package web

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"charm.land/log/v2"
	"github.com/flamego/flamego"
	"github.com/flamego/flamego/inject"
)

// recovery is a copy of [flamego.Recovery] except always responds in plain
// text.
func recovery() flamego.Handler {
	var (
		dunno     = []byte("???")
		centerDot = []byte("·")
		dot       = []byte(".")
		slash     = []byte("/")
	)

	// source returns a space-trimmed slice of the n'th line.
	source := func(lines [][]byte, n int) []byte {
		n-- // In a stack trace, lines are 1-indexed but our array is 0-indexed.
		if n < 0 || n >= len(lines) {
			return dunno
		}
		return bytes.TrimSpace(lines[n])
	}

	// function returns, if possible, the name of the function containing the PC.
	function := func(pc uintptr) []byte {
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			return dunno
		}
		name := []byte(fn.Name())
		// The name includes the path name to the package, which is unnecessary since
		// the file name is already included. Plus, it has center dots. That is, we see:
		//	runtime/debug.*T·ptrmethod
		// and want:
		//	*T.ptrmethod
		// Also the package path might contains dot (e.g. code.google.com/...), so first
		// eliminate the path prefix.
		if lastSlash := bytes.LastIndex(name, slash); lastSlash >= 0 {
			name = name[lastSlash+1:]
		}
		if period := bytes.Index(name, dot); period >= 0 {
			name = name[period+1:]
		}
		name = bytes.ReplaceAll(name, centerDot, dot)
		return name
	}

	// stack returns a nicely formatted stack frame, skipping skip frames.
	stack := func(skip int) []byte {
		buf := new(bytes.Buffer)
		// As we loop, we open files and read them. These variables record the currently
		// loaded file.
		var lines [][]byte
		var lastFile string
		for i := skip; ; i++ { // Skip the expected number of frames.
			pc, file, line, ok := runtime.Caller(i)
			if !ok {
				break
			}
			// Print this much at least.  If we can't find the source, it won't show.
			_, _ = fmt.Fprintf(buf, "%s:%d (0x%x)\n", file, line, pc)
			if file != lastFile {
				data, err := os.ReadFile(file)
				if err != nil {
					continue
				}
				lines = bytes.Split(data, []byte{'\n'})
				lastFile = file
			}
			_, _ = fmt.Fprintf(buf, "\t%s: %s\n", function(pc), source(lines, line))
		}
		return buf.Bytes()
	}

	return flamego.LoggerInvoker(func(c flamego.Context, logger *log.Logger) {
		defer func() {
			if err := recover(); err != nil {
				stack := bytes.TrimRight(stack(3), "\n")
				logger.Error(fmt.Sprintf("PANIC: %s\n%s", err, stack))

				val := c.Value(inject.InterfaceOf((*http.ResponseWriter)(nil)))
				w := val.Interface().(http.ResponseWriter)

				// Respond with panic message only in development mode
				var body []byte
				if flamego.Env() == flamego.EnvTypeDev {
					body = []byte(fmt.Sprintf("PANIC: %s\n%s", err, stack))
				} else {
					body = []byte(http.StatusText(http.StatusInternalServerError))
				}

				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(body)
			}
		}()

		c.Next()
	})
}
