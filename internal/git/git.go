package git

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel"
)

// tracer creates the spans of the functions and methods of the package, and of
// the Git processes they run.
var tracer = otel.Tracer("gogs.io/gogs/internal/git")

var (
	// logOutput is the writer to write logs. When not set, no log will be produced.
	logOutput io.Writer
	// logPrefix is the prefix prepend to each log entry.
	logPrefix = "[git] "
)

// SetOutput sets the output writer for logs.
func SetOutput(output io.Writer) {
	logOutput = output
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

// BinVersion returns current Git binary version that is used by this package.
// Successful results are cached. Failed attempts are not cached, allowing
// retries with a fresh context.
func BinVersion(ctx context.Context) (string, error) {
	ctx, span := tracer.Start(ctx, "git.BinVersion")
	defer span.End()

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
		return "", errors.Newf("not enough output: %s", stdout)
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

// ConfigGet returns the value of given key in Git configuration. It returns an
// empty string when the key is not set.
//
// Docs: https://git-scm.com/docs/git-config
func ConfigGet(ctx context.Context, key string) (string, error) {
	ctx, span := tracer.Start(ctx, "git.ConfigGet")
	defer span.End()

	stdout, err := exec(ctx, "", []string{"config", "--get", "--end-of-options", key}, nil)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// SetGlobalConfig sets given key to the value in the global Git configuration
// of the current user.
//
// Docs: https://git-scm.com/docs/git-config
func SetGlobalConfig(ctx context.Context, key, value string) error {
	ctx, span := tracer.Start(ctx, "git.SetGlobalConfig")
	defer span.End()

	_, err := exec(ctx, "", []string{"config", "--global", "--end-of-options", key, value}, nil)
	return err
}
