package git

import (
	"bytes"
	"context"
	"strconv"

	"github.com/cockroachdb/errors"
)

// GrepOptions contains optional arguments for searching file contents.
//
// Docs: https://git-scm.com/docs/git-grep
type GrepOptions struct {
	// The maximum number of matching lines to return in total. Zero means no
	// limit.
	MaxResults int
	// The maximum number of matching lines to return for each file. Zero means
	// no limit.
	MaxResultsPerFile int
}

// GrepResult is a line that matches a search.
type GrepResult struct {
	// The path of the file from the root of the tree.
	Path string
	// The number of the line, counting from 1.
	Line int
	// The content of the line, without the line ending.
	Text string
}

// Grep searches the text files in the tree of the given revision for lines
// that contain the keyword, ignoring case. The keyword is matched literally,
// not as a pattern. Binary files are skipped. Results are ordered by path, then
// by line. The second return value reports whether results were left out
// because of opts.MaxResults.
func (r *Repository) Grep(ctx context.Context, rev, keyword string, opts ...GrepOptions) ([]*GrepResult, bool, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.Grep")
	defer span.End()

	var opt GrepOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"grep", "--null", "--line-number", "--full-name", "-I", "--fixed-strings", "--ignore-case"}
	if opt.MaxResultsPerFile > 0 {
		args = append(args, "--max-count="+strconv.Itoa(opt.MaxResultsPerFile))
	}
	args = append(args, "-e", keyword, "--end-of-options", rev, "--")

	// Stop Git once enough results arrived, rather than reading every match of
	// a common keyword in a large repository.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := &grepWriter{prefix: []byte(rev + ":"), max: opt.MaxResults, cancel: cancel}
	err := pipe(ctx, r.path, args, nil, w)
	switch {
	case w.err != nil:
		return nil, false, w.err
	case w.truncated:
		return w.results, true, nil
	case err == nil:
		return w.results, false, nil
	case isExitStatus(err, 1):
		// Git exits with 1 when nothing matches.
		return nil, false, nil
	}
	return nil, false, err
}

// grepWriter parses the output of "git grep --null --line-number" as it
// arrives, where each line is "<rev>:<path>\0<line>\0<text>\n".
type grepWriter struct {
	prefix    []byte
	max       int
	cancel    context.CancelFunc
	buf       []byte
	results   []*GrepResult
	truncated bool
	err       error
}

func (w *grepWriter) Write(p []byte) (int, error) {
	if w.truncated || w.err != nil {
		// Drain what Git wrote before it was stopped.
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for {
		end := bytes.IndexByte(w.buf, '\n')
		if end < 0 {
			break
		}
		line := w.buf[:end]
		w.buf = w.buf[end+1:]

		if w.max > 0 && len(w.results) == w.max {
			w.truncated = true
			w.cancel()
			return len(p), nil
		}
		result, err := parseGrepLine(w.prefix, line)
		if err != nil {
			w.err = err
			w.cancel()
			return len(p), nil
		}
		w.results = append(w.results, result)
	}
	return len(p), nil
}

func parseGrepLine(prefix, line []byte) (*GrepResult, error) {
	fields := bytes.SplitN(line, []byte{0}, 3)
	if len(fields) != 3 || !bytes.HasPrefix(fields[0], prefix) {
		return nil, errors.Newf("unexpected grep output %q", line)
	}
	number, err := strconv.Atoi(string(fields[1]))
	if err != nil {
		return nil, errors.Wrapf(err, "parse line number in %q", line)
	}
	return &GrepResult{
		Path: string(fields[0][len(prefix):]),
		Line: number,
		Text: string(bytes.TrimSuffix(fields[2], []byte{'\r'})),
	}, nil
}
