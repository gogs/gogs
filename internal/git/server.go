package git

import (
	"context"
	"encoding/hex"
	"io"
	"os"
	"slices"
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
// RunService, and returns the reference updates of the push that Git applied,
// which is what Git passes to the post-receive hook. The updates are returned
// even with an error, because Git may have applied some of them before it
// failed.
func RunReceivePack(ctx context.Context, path string, opts ServiceOptions) ([]RefUpdate, error) {
	push := &receivePackPush{inAdvertisement: !opts.StatelessRPC}
	if opts.Stdin != nil {
		opts.Stdin = receivePackRequestReader{ctx: ctx, path: path, r: opts.Stdin, push: push}
	}
	opts.Stdout = receivePackResponseWriter{w: opts.Stdout, push: push}
	err := RunService(ctx, ServiceReceivePack, path, opts)

	updates, applied, reported := push.result()
	var checkErr error
	if !reported {
		applied, checkErr = appliedRefUpdates(ctx, path, updates)
	}
	applied = push.withoutPhantomDeletes(applied)
	if err != nil {
		return applied, err
	}
	return applied, checkErr
}

// refUpdatesBatchSize is how many reference names are looked up per Git
// command, which keeps the arguments of a push of many references under the
// size limit of the operating system.
var refUpdatesBatchSize = 1000

// appliedRefUpdates returns the updates that the references in the repository
// in given path reflect. It is only used when Git did not report which updates
// it applied, because another push may change the references in the meantime.
func appliedRefUpdates(ctx context.Context, path string, updates []RefUpdate) ([]RefUpdate, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(updates))
	for _, update := range updates {
		names = append(names, update.RefName)
	}
	current, err := refIDs(ctx, path, names)
	if err != nil {
		return nil, err
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

// refIDs returns the IDs of the references with given names that exist in the
// repository in given path.
func refIDs(ctx context.Context, path string, names []string) (map[string]string, error) {
	// A name also matches the references below it, e.g., "refs/heads/a" matches
	// "refs/heads/a/b", so the result is matched by the exact name.
	ids := make(map[string]string, len(names))
	for batch := range slices.Chunk(names, refUpdatesBatchSize) {
		args := append([]string{"for-each-ref", "--format=%(objectname) %(refname)", "--end-of-options"}, batch...)
		stdout, err := exec(ctx, path, args, nil)
		if err != nil {
			return nil, errors.Wrap(err, "list references")
		}
		for line := range strings.SplitSeq(strings.TrimSpace(string(stdout)), "\n") {
			id, name, ok := strings.Cut(line, " ")
			if ok {
				ids[name] = id
			}
		}
	}
	return ids, nil
}

// receivePackPush records a push from the pkt-lines that pass between the
// client and "git receive-pack": the reference updates the client asks for,
// and the report of Git on which of them it applied.
//
// Docs: https://git-scm.com/docs/pack-protocol#_pushing_data_to_a_server
type receivePackPush struct {
	mu sync.Mutex

	// The bytes of a pkt-line from the client that was not read in full yet.
	request     []byte
	requestDone bool
	commands    []RefUpdate
	// The capabilities that the client asks for.
	capabilities []string
	// The references to delete that existed before Git read the commands, or nil
	// when they are unknown.
	existedBeforeDelete map[string]string

	// The bytes of a pkt-line from Git that was not written in full yet.
	response []byte
	// Whether Git still writes the advertisement of references, which comes
	// first unless the service is stateless.
	inAdvertisement bool
	// The bytes of the report that were unwrapped from the sideband but not
	// parsed yet.
	report     []byte
	reportDone bool
	// Whether the response is broken, i.e., there is no report to trust.
	responseBroken bool
	// The references that Git reported it updated.
	updated map[string]bool
}

type receivePackRequestReader struct {
	ctx  context.Context
	path string
	r    io.Reader
	push *receivePackPush
}

func (r receivePackRequestReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n == 0 {
		return n, err
	}

	r.push.mu.Lock()
	wasDone := r.push.requestDone
	r.push.parseRequest(p[:n])
	var deleted []string
	if !wasDone && r.push.requestDone {
		for _, command := range r.push.commands {
			if IsEmptyID(command.NewID) {
				deleted = append(deleted, command.RefName)
			}
		}
	}
	r.push.mu.Unlock()

	// Git has not read the end of the commands yet, so it has not deleted any of
	// the references.
	if len(deleted) > 0 {
		existed, lookupErr := refIDs(r.ctx, r.path, deleted)
		if lookupErr == nil {
			r.push.mu.Lock()
			r.push.existedBeforeDelete = existed
			r.push.mu.Unlock()
		}
	}
	return n, err
}

type receivePackResponseWriter struct {
	w    io.Writer
	push *receivePackPush
}

func (w receivePackResponseWriter) Write(p []byte) (int, error) {
	w.push.mu.Lock()
	w.push.parseResponse(p)
	w.push.mu.Unlock()
	if w.w == nil {
		return len(p), nil
	}
	return w.w.Write(p)
}

// readPktLine returns the payload of the first pkt-line in b and its length in
// bytes, which is 0 when b does not hold a whole pkt-line yet. A length of 4
// with a nil payload is a flush-pkt or another special packet.
func readPktLine(b []byte) (payload []byte, n int, err error) {
	if len(b) < 4 {
		return nil, 0, nil
	}
	size, err := strconv.ParseUint(string(b[:4]), 16, 16)
	if err != nil {
		return nil, 0, errors.New("invalid pkt-line length")
	}
	if size < 4 {
		return nil, 4, nil
	}
	if len(b) < int(size) {
		return nil, 0, nil
	}
	return b[4:size], int(size), nil
}

// parseRequest records the commands, i.e., the pkt-lines of "<old-id> <new-id>
// <ref-name>" before the first flush-pkt, and the capabilities after the NUL of
// the first pkt-line.
func (p *receivePackPush) parseRequest(b []byte) {
	if p.requestDone {
		return
	}
	p.request = append(p.request, b...)
	for {
		payload, n, err := readPktLine(p.request)
		if err != nil || (n > 0 && payload == nil) {
			p.requestDone = true
			p.request = nil
			return
		} else if n == 0 {
			return
		}
		p.request = p.request[n:]

		line, capabilities, hasCapabilities := strings.Cut(string(payload), "\x00")
		if hasCapabilities && p.capabilities == nil {
			p.capabilities = strings.Fields(capabilities)
		}
		// Other lines, e.g., "shallow <id>" and those of a push certificate, are not
		// commands. Git rejects reference names with wildcards, which would also
		// make the lookup of the applied updates match other references.
		fields := strings.Fields(line)
		if len(fields) == 3 && isID(fields[0]) && isID(fields[1]) &&
			strings.HasPrefix(fields[2], "refs/") && !strings.ContainsAny(fields[2], `*?[\`) {
			p.commands = append(p.commands, RefUpdate{OldID: fields[0], NewID: fields[1], RefName: fields[2]})
		}
	}
}

// parseResponse records the references in the report of Git, i.e., the
// pkt-lines of "ok <ref-name>" before the flush-pkt that ends it, which Git
// wraps in sideband packets of band 1 when the client asks for a sideband.
func (p *receivePackPush) parseResponse(b []byte) {
	if p.reportDone || p.responseBroken {
		return
	}
	p.response = append(p.response, b...)

	// "git receive-pack" only supports the 64k sideband.
	useSideband := slices.Contains(p.capabilities, "side-band-64k")
	for {
		payload, n, err := readPktLine(p.response)
		if err != nil {
			p.responseBroken = true
			return
		} else if n == 0 {
			return
		}
		raw := p.response[:n]
		p.response = p.response[n:]

		switch {
		case p.inAdvertisement:
			p.inAdvertisement = payload != nil
			continue
		case !useSideband:
			p.report = append(p.report, raw...)
		case payload == nil:
			// The sideband ended before the report did.
			p.responseBroken = true
			return
		case len(payload) > 0 && payload[0] == 1:
			p.report = append(p.report, payload[1:]...)
		case len(payload) > 0 && payload[0] == 3:
			p.responseBroken = true
			return
		default:
			// Progress messages
			continue
		}
		p.parseReport()
		if p.reportDone || p.responseBroken {
			return
		}
	}
}

// parseReport records the references in the complete pkt-lines of the report.
func (p *receivePackPush) parseReport() {
	for {
		payload, n, err := readPktLine(p.report)
		if err != nil {
			p.responseBroken = true
			return
		} else if n == 0 {
			return
		}
		p.report = p.report[n:]
		if payload == nil {
			p.reportDone = true
			p.report = nil
			return
		}

		// Other lines are "unpack <result>", "ng <ref-name> <reason>", and the
		// "option" lines of report-status-v2.
		if name, ok := strings.CutPrefix(strings.TrimSuffix(string(payload), "\n"), "ok "); ok {
			if p.updated == nil {
				p.updated = make(map[string]bool)
			}
			p.updated[name] = true
		}
	}
}

// result returns the reference updates that the client asked for, and when Git
// reported which of them it applied, those updates.
func (p *receivePackPush) result() (updates, applied []RefUpdate, reported bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.reportDone {
		return p.commands, nil, false
	}
	for _, update := range p.commands {
		if p.updated[update.RefName] {
			applied = append(applied, update)
		}
	}
	return p.commands, applied, true
}

// withoutPhantomDeletes returns the updates without the deletions of references
// that did not exist before the push. Git reports them as applied when the old
// ID is not an object in the repository, but leaves them out of the
// post-receive hook. All updates are returned when it is unknown which
// references existed.
func (p *receivePackPush) withoutPhantomDeletes(updates []RefUpdate) []RefUpdate {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.existedBeforeDelete == nil {
		return updates
	}

	existing := updates[:0:0]
	for _, update := range updates {
		if _, existed := p.existedBeforeDelete[update.RefName]; existed || !IsEmptyID(update.NewID) {
			existing = append(existing, update)
		}
	}
	return existing
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
