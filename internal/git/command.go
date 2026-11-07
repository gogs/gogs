package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sourcegraph/run"
)

// CommandOptions contains additional options for running a Git command.
type CommandOptions struct {
	Envs []string
}

// DefaultTimeout is the default timeout duration for all commands. It is
// applied when the context does not already have a deadline.
const DefaultTimeout = time.Minute

// cmd builds a *run.Command for git with the given arguments, environment
// variables and working directory. DefaultTimeout will be applied if the context
// does not already have a deadline.
func cmd(ctx context.Context, dir string, args []string, envs []string) (*run.Command, context.CancelFunc) {
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, DefaultTimeout)
		cancel = timeoutCancel
	}

	// run.Cmd joins all parts into a single string and then shell-parses it. We must
	// quote each argument so that special characters (spaces, quotes, angle
	// brackets, etc.) are preserved correctly.
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, "git")
	for _, arg := range args {
		parts = append(parts, run.Arg(arg))
	}

	c := run.Cmd(ctx, parts...)
	if dir != "" {
		c = c.Dir(dir)
	}
	if len(envs) > 0 {
		c = c.Environ(append(os.Environ(), envs...))
	}
	return c, cancel
}

// exec executes a git command in the given directory and returns stdout as
// bytes. Stderr is included in the error message on failure. DefaultTimeout will
// be applied if the context does not already have a deadline. It returns
// ErrExecTimeout if the execution was timed out.
func exec(ctx context.Context, dir string, args []string, envs []string) ([]byte, error) {
	c, cancel := cmd(ctx, dir, args, envs)
	defer cancel()

	var logBuf *bytes.Buffer
	if logOutput != nil {
		logBuf = new(bytes.Buffer)
		logBuf.Grow(512)
		defer func() {
			log(dir, args, logBuf.Bytes())
		}()
	}

	// Use Stream to a buffer to preserve raw bytes (including NUL bytes from
	// commands like "ls-tree -z"). The String/Lines methods process output
	// line-by-line which corrupts binary-ish output.
	stdout := new(bytes.Buffer)
	err := c.StdOut().Run().Stream(stdout)

	// Capture (partial) stdout for logging even on error, so failed commands produce
	// a useful log entry rather than an empty one.
	if logOutput != nil {
		data := stdout.Bytes()
		limit := len(data)
		if limit > 512 {
			limit = 512
		}
		logBuf.Write(data[:limit])
		if len(data) > 512 {
			logBuf.WriteString("... (more omitted)")
		}
	}

	if err != nil {
		return nil, mapContextError(err, ctx)
	}
	return stdout.Bytes(), nil
}

// pipe executes a git command in the given directory, streaming stdout to the
// given io.Writer.
func pipe(ctx context.Context, dir string, args []string, envs []string, stdout io.Writer) error {
	c, cancel := cmd(ctx, dir, args, envs)
	defer cancel()

	var buf *bytes.Buffer
	w := stdout
	if logOutput != nil {
		buf = new(bytes.Buffer)
		buf.Grow(512)
		w = &limitDualWriter{
			W: buf,
			N: int64(buf.Cap()),
			w: stdout,
		}

		defer func() {
			log(dir, args, buf.Bytes())
		}()
	}

	streamErr := c.StdOut().Run().Stream(w)
	if streamErr != nil {
		return mapContextError(streamErr, ctx)
	}
	return nil
}

// committerEnvs returns environment variables for setting the Git committer.
func committerEnvs(committer *Signature) []string {
	return []string{
		"GIT_COMMITTER_NAME=" + committer.Name,
		"GIT_COMMITTER_EMAIL=" + committer.Email,
	}
}

// log logs a git command execution with its output.
func log(dir string, args []string, output []byte) {
	cmdStr := "git"
	if len(args) > 0 {
		quoted := make([]string, len(args))
		for i, a := range args {
			if strings.ContainsAny(a, " \t\n\"'\\<>") {
				quoted[i] = strconv.Quote(a)
			} else {
				quoted[i] = a
			}
		}
		cmdStr = "git " + strings.Join(quoted, " ")
	}
	if len(dir) == 0 {
		logf("%s\n%s", cmdStr, output)
	} else {
		logf("%s: %s\n%s", dir, cmdStr, output)
	}
}

// A limitDualWriter writes to W but limits the amount of data written to just N
// bytes. On the other hand, it passes everything to w.
type limitDualWriter struct {
	W        io.Writer // underlying writer
	N        int64     // max bytes remaining
	prompted bool

	w io.Writer
}

func (w *limitDualWriter) Write(p []byte) (int, error) {
	if w.N > 0 {
		limit := int64(len(p))
		if limit > w.N {
			limit = w.N
		}
		n, _ := w.W.Write(p[:limit])
		w.N -= int64(n)
	}

	if !w.prompted && w.N <= 0 {
		w.prompted = true
		_, _ = w.W.Write([]byte("... (more omitted)"))
	}

	return w.w.Write(p)
}

// mapContextError maps context errors to the appropriate sentinel errors used
// by this package.
func mapContextError(err error, ctx context.Context) error {
	if ctx == nil {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return ErrExecTimeout
		}
		return ctxErr
	}
	return err
}

// isExitStatus reports whether err represents a specific process exit status
// code, using the run.ExitCoder interface provided by sourcegraph/run.
func isExitStatus(err error, code int) bool {
	var exitCoder run.ExitCoder
	ok := errors.As(err, &exitCoder)
	return ok && exitCoder.ExitCode() == code
}
