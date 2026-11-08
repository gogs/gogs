package git

import (
	"bytes"
	"context"
	"io"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// waitDelay bounds how long waiting for a command takes after it was killed or
// exited, e.g., when a process it started still holds its output open.
var waitDelay = time.Minute

// WithTimeout returns a copy of ctx that times out after the given duration. A
// timeout of zero or less returns ctx unchanged, i.e., no timeout.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// newCommand returns a git command that is killed along with every process it
// started when ctx is done.
func newCommand(ctx context.Context, dir string, args []string, envs []string) *osexec.Cmd {
	c := osexec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	if len(envs) > 0 {
		c.Env = append(os.Environ(), envs...)
	}
	c.WaitDelay = waitDelay
	killProcessGroup(c)
	return c
}

// exitError is returned when Git exits with a non-zero status. Its message
// includes what Git printed to stderr, if anything.
type exitError struct {
	*osexec.ExitError
	stderr []byte
}

func (e *exitError) Error() string {
	if len(e.stderr) == 0 {
		return e.ExitError.Error()
	}
	return e.ExitError.Error() + ": " + string(e.stderr)
}

func (e *exitError) Unwrap() error {
	return e.ExitError
}

// run executes a git command in the given directory, writing stdout to the
// given io.Writer, and returns what the command printed to stderr. The command
// is bound only by the deadline and cancellation of the context. It returns
// ErrExecTimeout if the execution was timed out.
func run(ctx context.Context, dir string, args []string, envs []string, stdout io.Writer) ([]byte, error) {
	stderr := new(bytes.Buffer)
	c := newCommand(ctx, dir, args, envs)
	c.Stdout = stdout
	c.Stderr = stderr
	err := ignoreWaitDelay(c, c.Run())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return nil, ErrExecTimeout
			}
			return nil, ctxErr
		}

		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return nil, &exitError{ExitError: exitErr, stderr: bytes.TrimSpace(stderr.Bytes())}
		}
		return nil, err
	}
	return stderr.Bytes(), nil
}

// ignoreWaitDelay returns nil instead of err when the command succeeded and
// only a process it started, e.g., a job in a hook, kept its output open past
// the wait delay.
func ignoreWaitDelay(c *osexec.Cmd, err error) error {
	if errors.Is(err, osexec.ErrWaitDelay) && c.ProcessState != nil && c.ProcessState.Success() {
		return nil
	}
	return err
}

// exec executes a git command in the given directory and returns stdout as
// bytes. Stderr is included in the error message on failure.
func exec(ctx context.Context, dir string, args []string, envs []string) ([]byte, error) {
	stdout := new(bytes.Buffer)
	_, err := run(ctx, dir, args, envs, stdout)

	// Log (partial) stdout even on error, so failed commands produce a useful log
	// entry rather than an empty one.
	if logOutput != nil {
		data := stdout.Bytes()
		if len(data) > 512 {
			data = append(data[:512:512], "... (more omitted)"...)
		}
		log(dir, args, data)
	}

	if err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// pipe executes a git command in the given directory, streaming stdout to the
// given io.Writer.
func pipe(ctx context.Context, dir string, args []string, envs []string, stdout io.Writer) error {
	w := stdout
	if logOutput != nil {
		buf := new(bytes.Buffer)
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

	_, err := run(ctx, dir, args, envs, w)
	return err
}

// authorEnvs returns environment variables that record the signature as the author.
// Git otherwise falls back to user.name, user.email, or the system account.
func authorEnvs(author *Signature) []string {
	return []string{
		"GIT_AUTHOR_NAME=" + author.Name,
		"GIT_AUTHOR_EMAIL=" + author.Email,
	}
}

// committerEnvs returns environment variables that record the signature as the committer.
// Git otherwise falls back to user.name, user.email, or the system account.
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

// isExitStatus reports whether err represents a specific process exit status
// code.
func isExitStatus(err error, code int) bool {
	var exitErr *osexec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}
