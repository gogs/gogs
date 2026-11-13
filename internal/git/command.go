package git

import (
	"bytes"
	"context"
	"io"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// waitDelay bounds how long waiting for a command takes after it was killed or
// exited while its output stays idle, e.g., when a process it started still
// holds the output open.
var waitDelay = time.Minute

// WithTimeout returns a copy of ctx that times out after the given duration. A
// timeout of zero or less returns ctx unchanged, i.e., no timeout.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// newCommand returns a git command that is killed when ctx is done. runCommand
// makes sure every process it started is killed along with it.
func newCommand(ctx context.Context, dir string, args []string, envs []string) *osexec.Cmd {
	c := osexec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	if len(envs) > 0 {
		c.Env = append(os.Environ(), envs...)
	}
	c.WaitDelay = waitDelay
	return c
}

// runCommand starts the command and waits for it to complete. It records the
// run as a span named after the Git subcommand, without the other arguments,
// which may contain credentials, e.g., in a remote URL. The span is passed on
// to Git through the TRACEPARENT and TRACESTATE environment variables, so that
// hooks Git runs can record their spans under it.
//
// Output is copied through pipes of our own rather than the ones os/exec
// creates, because os/exec drops what is left in them a fixed delay after the
// command exited, even while a slow writer, e.g., a client, is still taking it.
func runCommand(ctx context.Context, c *osexec.Cmd) (err error) {
	ctx, span := tracer.Start(ctx, "git "+subcommand(c.Args[1:]), trace.WithAttributes(attribute.String("git.dir", c.Dir)))
	defer func() {
		if c.ProcessState != nil {
			span.SetAttributes(attribute.Int("process.exit.code", c.ProcessState.ExitCode()))
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	if len(carrier) > 0 && c.Env == nil {
		c.Env = os.Environ()
	}
	for key, value := range carrier {
		c.Env = append(c.Env, strings.ToUpper(key)+"="+value)
	}

	var copiers []*outputCopier
	for _, w := range []*io.Writer{&c.Stdout, &c.Stderr} {
		if *w == nil {
			continue
		}
		if _, ok := (*w).(*os.File); ok {
			continue
		}
		pr, pw, err := os.Pipe()
		if err != nil {
			closeReaders(copiers)
			return errors.Wrap(err, "create pipe")
		}
		defer func() { _ = pw.Close() }()
		copiers = append(copiers, &outputCopier{r: pr, w: *w, done: make(chan struct{})})
		*w = pw
	}

	tree, err := newProcessTree(c)
	if err != nil {
		closeReaders(copiers)
		return errors.Wrap(err, "set up process tree")
	}
	defer tree.close()

	if err := c.Start(); err != nil {
		closeReaders(copiers)
		return err
	}

	// Close our copies of the write ends, so that the copies of the output end
	// once Git and every process it started closed theirs.
	for _, w := range []io.Writer{c.Stdout, c.Stderr} {
		if pw, ok := w.(*os.File); ok {
			_ = pw.Close()
		}
	}

	if err := tree.attach(c); err != nil {
		_ = c.Process.Kill()
		_ = c.Wait()
		closeReaders(copiers)
		return errors.Wrap(err, "attach process tree")
	}
	for _, o := range copiers {
		go o.copy()
	}

	err = c.Wait()
	for _, o := range copiers {
		o.wait(waitDelay)
	}
	if err != nil {
		return err
	}
	for _, o := range copiers {
		if o.err != nil {
			return o.err
		}
	}
	return nil
}

// subcommand returns the Git subcommand in the arguments, e.g., "log".
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-c" || args[i] == "-C":
			// The global option takes the next argument as its value.
			i++
		case !strings.HasPrefix(args[i], "-"):
			return args[i]
		}
	}
	return ""
}

func closeReaders(copiers []*outputCopier) {
	for _, o := range copiers {
		_ = o.r.Close()
	}
}

// outputCopier copies the output of a command from a pipe to a writer.
type outputCopier struct {
	r    *os.File
	w    io.Writer
	done chan struct{}
	err  error

	writing    atomic.Bool
	lastActive atomic.Int64 // In Unix nanoseconds.
}

func (o *outputCopier) copy() {
	defer close(o.done)
	defer func() { _ = o.r.Close() }()

	buf := make([]byte, 32*1024)
	for {
		n, err := o.r.Read(buf)
		if n > 0 {
			o.writing.Store(true)
			_, o.err = o.w.Write(buf[:n])
			o.writing.Store(false)
			o.lastActive.Store(time.Now().UnixNano())
			if o.err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// wait waits for the copy to end after the command exited. When the output
// stays idle for the given delay, e.g., because a process the command started
// holds it open, it stops the copy. It never stops a copy that is writing.
func (o *outputCopier) wait(delay time.Duration) {
	o.lastActive.Store(time.Now().UnixNano())
	for {
		next := delay
		if !o.writing.Load() {
			idle := time.Since(time.Unix(0, o.lastActive.Load()))
			if idle >= delay {
				_ = o.r.Close()
				<-o.done
				return
			}
			next = delay - idle
		}

		timer := time.NewTimer(next)
		select {
		case <-o.done:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
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

// run executes a git command in the given directory, reading stdin from the
// given io.Reader if not nil and writing stdout to the given io.Writer, and
// returns what the command printed to stderr. The command is bound only by the
// deadline and cancellation of the context. It returns ErrExecTimeout if the
// execution was timed out.
func run(ctx context.Context, dir string, args []string, envs []string, stdin io.Reader, stdout io.Writer) ([]byte, error) {
	stderr := new(bytes.Buffer)
	c := newCommand(ctx, dir, args, envs)
	c.Stdin = stdin
	c.Stdout = stdout
	c.Stderr = stderr
	err := runCommand(ctx, c)
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

// exec executes a git command in the given directory and returns stdout as
// bytes. Stderr is included in the error message on failure.
func exec(ctx context.Context, dir string, args []string, envs []string) ([]byte, error) {
	return execInput(ctx, dir, args, envs, nil)
}

// execInput is like exec but feeds stdin to the command.
func execInput(ctx context.Context, dir string, args []string, envs []string, stdin io.Reader) ([]byte, error) {
	stdout := new(bytes.Buffer)
	_, err := run(ctx, dir, args, envs, stdin, stdout)

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

	_, err := run(ctx, dir, args, envs, nil, w)
	return err
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
		limit := min(int64(len(p)), w.N)
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
