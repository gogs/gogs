package git

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// expectedErrors are answers rather than failures, e.g., a revision that does
// not exist, so they do not mark the span of an operation as failed.
var expectedErrors = []error{
	ErrReferenceNotExist,
	ErrParentNotExist,
	ErrSubmoduleNotExist,
	ErrRevisionNotExist,
	ErrRemoteNotExist,
	ErrNoMergeBase,
	ErrNotBlob,
	ErrNotSymbolicRef,
	ErrMergeConflict,
}

// endSpan ends the span of an operation that returned err. An expected error is
// recorded as the outcome of the operation, any other one as its failure.
func endSpan(span trace.Span, err error) {
	defer span.End()
	if err == nil {
		return
	}
	for _, expected := range expectedErrors {
		if errors.Is(err, expected) {
			span.SetAttributes(attribute.String("git.outcome", expected.Error()))
			return
		}
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// redactURL removes the credentials from a remote URL, e.g., of a mirror.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// commandArgs returns the arguments of a Git command to record on its span.
// Values that may hold credentials or private text are left out, i.e., of
// configuration options, commit messages, search patterns and paths in the
// repository, and so are the credentials of URLs.
func commandArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			// Everything after it is a path in the repository.
			out = append(out, arg)
			for range args[i+1:] {
				out = append(out, "<path>")
			}
			return out
		case arg == "-c" || arg == "-m" || arg == "--message" || arg == "-e":
			out = append(out, arg)
			if i+1 < len(args) {
				out = append(out, "<redacted>")
				i++
			}
		case strings.HasPrefix(arg, "--message="):
			out = append(out, "--message=<redacted>")
		case strings.Contains(arg, "://"):
			out = append(out, redactURL(arg))
		case !strings.HasPrefix(arg, "-") && strings.Contains(arg, ":") && !filepath.IsAbs(arg):
			// A revision and a path in it, e.g., "main:docs/README.md", since
			// references cannot contain colons. Absolute paths on Windows,
			// e.g., of the repository, have one after their drive letter.
			rev, _, _ := strings.Cut(arg, ":")
			out = append(out, rev+":<path>")
		default:
			out = append(out, arg)
		}
	}
	return out
}
