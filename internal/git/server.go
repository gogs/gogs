package git

import (
	"context"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
)

// Service is a Git service that serves fetches and pushes.
type Service string

// A list of services that clients can request.
const (
	ServiceUploadPack    Service = "upload-pack"
	ServiceUploadArchive Service = "upload-archive"
	ServiceReceivePack   Service = "receive-pack"
)

// ServiceOptions contains optional arguments for running a service.
//
// Docs: https://git-scm.com/docs/git-upload-pack
type ServiceOptions struct {
	// Indicates whether to quit after a single request/response exchange, as the
	// smart HTTP protocol requires.
	StatelessRPC bool
	// Indicates whether to only advertise references and exit, as the "info/refs"
	// endpoint of the smart HTTP protocol requires.
	AdvertiseRefs bool
	// The streams connected to the client.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// The protocol the client asks for, e.g., "version=2". Anything other than a
	// protocol version Git supports makes the service use protocol v0.
	Protocol string
	// The additional environment variables to run the command with.
	Envs []string
}

// IsSupportedProtocol returns true if the protocol a client asks for, e.g.,
// "version=2", is one that services accept.
func IsSupportedProtocol(protocol string) bool {
	return protocol == "version=1" || protocol == "version=2"
}

// protocolEnv returns the GIT_PROTOCOL environment variable for the protocol
// the client asks for. It is always set so that a value inherited from the
// environment, e.g., one that sshd accepted, never reaches Git unchecked.
func protocolEnv(protocol string) string {
	if !IsSupportedProtocol(protocol) {
		return "GIT_PROTOCOL="
	}
	return "GIT_PROTOCOL=" + protocol
}

// serviceIdleTimeout is how long a service may go without reading from or
// writing to the client before it is killed. Git sends keepalive packets while
// it prepares or processes a pack, so a transfer that makes progress never stays
// idle that long. Git sends nothing while hooks run, so a hook that prints
// nothing for this long is killed along with the service.
var serviceIdleTimeout = 30 * time.Minute

// RunService runs the service for the repository in given path, e.g., "git
// upload-pack <path>". Instead of a fixed timeout, which would cut off a large
// transfer, the service is killed once it has been idle for too long.
func RunService(ctx context.Context, service Service, path string, opts ...ServiceOptions) error {
	ctx, span := tracer.Start(ctx, "git.RunService")
	defer span.End()

	var opt ServiceOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{string(service)}
	if opt.StatelessRPC {
		args = append(args, "--stateless-rpc")
	}
	if opt.AdvertiseRefs {
		args = append(args, "--advertise-refs")
	}
	args = append(args, path)

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	idleTimeout := serviceIdleTimeout
	var lastActive atomic.Int64
	touch := func() { lastActive.Store(time.Now().UnixNano()) }
	touch()
	go func() {
		ticker := time.NewTicker(idleTimeout / 10)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastActive.Load())) > idleTimeout {
					cancel(ErrExecTimeout)
					return
				}
			}
		}
	}()

	envs := append(append([]string(nil), opt.Envs...), protocolEnv(opt.Protocol))
	c := newCommand(ctx, "", args, envs)
	if opt.Stdin != nil {
		// Copy the input through a pipe of our own, because waiting for the command
		// also waits for the input that os/exec copies, and reading from the client
		// may block long after Git exited.
		pr, pw, err := os.Pipe()
		if err != nil {
			return errors.Wrap(err, "create pipe")
		}
		defer func() { _ = pr.Close() }()
		go func() {
			_, _ = io.Copy(pw, activityReader{r: opt.Stdin, touch: touch})
			_ = pw.Close()
		}()
		c.Stdin = pr
	}
	c.Stdout = activityWriter{w: opt.Stdout, touch: touch}
	c.Stderr = activityWriter{w: opt.Stderr, touch: touch}
	err := runCommand(ctx, c)
	if context.Cause(ctx) == ErrExecTimeout {
		return ErrExecTimeout
	}
	return err
}

// RefUpdate is an update of a reference by a push.
type RefUpdate struct {
	OldID string
	NewID string
	// The full name of the reference, e.g., "refs/heads/main".
	RefName string
}

// RunReceivePack runs "git receive-pack" for the repository in given path like
// RunService, and returns the reference updates of the push that were applied,
// which is what Git passes to the post-receive hook. The updates are returned
// even with an error, because Git may have applied some of them before it
// failed.
func RunReceivePack(ctx context.Context, path string, opts ServiceOptions) ([]RefUpdate, error) {
	commands := &receivePackCommandsReader{r: opts.Stdin}
	if opts.Stdin != nil {
		opts.Stdin = commands
	}
	err := RunService(ctx, ServiceReceivePack, path, opts)

	applied, checkErr := appliedRefUpdates(ctx, path, commands.updates())
	if err != nil {
		return applied, err
	}
	return applied, checkErr
}

// appliedRefUpdates returns the updates that the references in the repository
// in given path reflect. Git checks the old ID of every update it applies, so
// an update whose reference now has the new ID is one that Git applied.
func appliedRefUpdates(ctx context.Context, path string, updates []RefUpdate) ([]RefUpdate, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	args := []string{"for-each-ref", "--format=%(objectname) %(refname)"}
	for _, update := range updates {
		args = append(args, update.RefName)
	}
	stdout, err := exec(ctx, path, args, nil)
	if err != nil {
		return nil, errors.Wrap(err, "list references")
	}
	current := make(map[string]string, len(updates))
	for line := range strings.SplitSeq(strings.TrimSpace(string(stdout)), "\n") {
		id, name, ok := strings.Cut(line, " ")
		if ok {
			current[name] = id
		}
	}

	applied := make([]RefUpdate, 0, len(updates))
	for _, update := range updates {
		id, exists := current[update.RefName]
		if (IsEmptyID(update.NewID) && !exists) || id == update.NewID {
			applied = append(applied, update)
		}
	}
	return applied, nil
}

// receivePackCommandsReader passes on what a client sends to "git
// receive-pack" and records the reference updates it asks for, which come as
// pkt-lines of "<old-id> <new-id> <ref-name>" before the first flush-pkt.
//
// Docs: https://git-scm.com/docs/pack-protocol#_reference_update_request_and_packfile_transfer
type receivePackCommandsReader struct {
	r io.Reader

	// The bytes of a pkt-line that has not been read in full yet.
	pending []byte

	mu       sync.Mutex
	done     bool
	commands []RefUpdate
}

func (r *receivePackCommandsReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.done && n > 0 {
		r.pending = append(r.pending, p[:n]...)
		r.parse()
	}
	return n, err
}

// parse records the commands in complete pkt-lines of pending, and stops at the
// flush-pkt that ends them or at anything that is not a pkt-line.
func (r *receivePackCommandsReader) parse() {
	for len(r.pending) >= 4 {
		size, err := strconv.ParseUint(string(r.pending[:4]), 16, 16)
		// A size below 4 is the flush-pkt or another special packet.
		if err != nil || size < 4 {
			r.done = true
			r.pending = nil
			return
		}
		if len(r.pending) < int(size) {
			break
		}

		line := string(r.pending[4:size])
		r.pending = r.pending[size:]
		// The first command carries the capabilities of the client after a NUL.
		line, _, _ = strings.Cut(line, "\x00")
		fields := strings.Fields(line)
		// Other lines, e.g., "shallow <id>", are not commands. Git rejects reference
		// names with wildcards, which would also make the lookup of the applied
		// updates match other references.
		if len(fields) == 3 && isID(fields[0]) && isID(fields[1]) &&
			strings.HasPrefix(fields[2], "refs/") && !strings.ContainsAny(fields[2], `*?[\`) {
			r.commands = append(r.commands, RefUpdate{OldID: fields[0], NewID: fields[1], RefName: fields[2]})
		}
	}
}

func (r *receivePackCommandsReader) updates() []RefUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.commands
}

// isID returns true if s is a hex object ID of any object format.
func isID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

type activityReader struct {
	r     io.Reader
	touch func()
}

func (r activityReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.touch()
	return n, err
}

type activityWriter struct {
	w     io.Writer
	touch func()
}

func (w activityWriter) Write(p []byte) (int, error) {
	w.touch()
	if w.w == nil {
		return len(p), nil
	}
	return w.w.Write(p)
}
