package git

import (
	"context"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
)

// UpdateServerInfo updates the auxiliary info file on the server side for the
// repository in given path.
func UpdateServerInfo(ctx context.Context, path string) error {
	args := []string{"update-server-info", "--end-of-options"}
	_, err := exec(ctx, path, args, nil)
	return err
}

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
	// The additional environment variables to run the command with.
	Envs []string
}

// serviceIdleTimeout is how long a service may go without reading from or
// writing to the client before it is killed. Git sends keepalive packets while
// it prepares a pack or runs hooks, so a transfer that makes progress never
// stays idle that long.
var serviceIdleTimeout = 10 * time.Minute

// RunService runs the service for the repository in given path, e.g., "git
// upload-pack <path>". Instead of a fixed timeout, which would cut off a large
// transfer, the service is killed once it has been idle for too long.
func RunService(ctx context.Context, service Service, path string, opts ...ServiceOptions) error {
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

	c := newCommand(ctx, "", args, opt.Envs)
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
	err := ignoreWaitDelay(c, runCommand(c))
	if context.Cause(ctx) == ErrExecTimeout {
		return ErrExecTimeout
	}
	return err
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
