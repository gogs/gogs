package git

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

var (
	// logOutput is the writer to write logs. When not set, no log will be produced.
	logOutput io.Writer
	// logPrefix is the prefix prepend to each log entry.
	logPrefix = "[git-module] "
)

// SetOutput sets the output writer for logs.
func SetOutput(output io.Writer) {
	logOutput = output
}

// SetPrefix sets the prefix to be prepended to each log entry.
func SetPrefix(prefix string) {
	logPrefix = prefix
}

func logf(format string, args ...interface{}) {
	if logOutput == nil {
		return
	}

	_, _ = fmt.Fprint(logOutput, logPrefix)
	_, _ = fmt.Fprintf(logOutput, format, args...)
	_, _ = fmt.Fprintln(logOutput)
}

var (
	// gitVersion stores the Git binary version.
	// NOTE: To check Git version should call BinVersion not this global variable.
	gitVersion    string
	gitVersionMu  sync.Mutex
	gitVersionSet bool
)

// BinVersion returns current Git binary version that is used by this module.
// Successful results are cached; failed attempts are not cached, allowing
// retries with a fresh context.
func BinVersion(ctx context.Context) (string, error) {
	gitVersionMu.Lock()
	defer gitVersionMu.Unlock()

	if gitVersionSet {
		return gitVersion, nil
	}

	stdout, err := exec(ctx, "", []string{"version"}, nil)
	if err != nil {
		return "", err
	}

	fields := strings.Fields(string(stdout))
	if len(fields) < 3 {
		return "", fmt.Errorf("not enough output: %s", stdout)
	}

	// Handle special case on Windows.
	i := strings.Index(fields[2], "windows")
	if i >= 1 {
		gitVersion = fields[2][:i-1]
	} else {
		gitVersion = fields[2]
	}

	gitVersionSet = true
	return gitVersion, nil
}
