package git

import (
	"bytes"
	"context"
	"strconv"
	"strings"

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
	// The number of lines to return around each matching line.
	ContextLines int
	// The pathspecs that limit which files are searched, e.g.,
	// ":(glob)**/*.go". All files are searched when empty.
	Pathspecs []string
}

// GrepResult is a line that matches a search, or a line around one.
type GrepResult struct {
	// The path of the file from the root of the tree.
	Path string
	// The number of the line, counting from 1.
	Line int
	// The content of the line, without the line ending.
	Text string
	// Whether the line contains the keyword, rather than being around a line
	// that does.
	IsMatch bool
}

// Grep searches the text files in the tree of the given revision for lines
// that contain the keyword, ignoring case. The keyword is matched literally,
// not as a pattern. Binary files are skipped. Results are ordered by path, then
// by line. The second return value reports whether matching lines were left
// out because of opts.MaxResults.
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
	if opt.ContextLines > 0 {
		args = append(args, "--context="+strconv.Itoa(opt.ContextLines))
	}
	args = append(args, "-e", keyword, "--end-of-options", rev, "--")
	args = append(args, opt.Pathspecs...)

	// Stop Git once enough results arrived, rather than reading every match of
	// a common keyword in a large repository.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := &grepWriter{
		prefix:     []byte(rev + ":"),
		keyword:    strings.ToLower(keyword),
		max:        opt.MaxResults,
		maxPerFile: opt.MaxResultsPerFile,
		context:    opt.ContextLines,
		cancel:     cancel,
	}
	err := pipe(ctx, r.path, args, nil, w)
	switch {
	case w.err != nil:
		return nil, false, w.err
	case w.truncated:
		return w.trimmedResults(), true, nil
	case err == nil:
		return w.results, false, nil
	case isExitStatus(err, 1):
		// Git exits with 1 when nothing matches.
		return nil, false, nil
	}
	return nil, false, err
}

// grepWriter parses the output of "git grep --null --line-number" as it
// arrives, where each line is "<rev>:<path>\0<line>\0<text>\n", and groups of
// lines with context are separated by "--".
type grepWriter struct {
	prefix     []byte
	keyword    string
	max        int
	maxPerFile int
	context    int
	cancel     context.CancelFunc
	buf        []byte
	matches    int
	// fileMatches counts the matching lines of the file of the last result.
	fileMatches int
	results     []*GrepResult
	truncated   bool
	err         error
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
		if string(line) == "--" {
			continue
		}

		result, err := parseGrepLine(w.prefix, line)
		if err != nil {
			w.err = err
			w.cancel()
			return len(p), nil
		}
		if n := len(w.results); n == 0 || w.results[n-1].Path != result.Path {
			w.fileMatches = 0
		}
		// With --null, Git separates the line number of matching lines and of
		// lines around them alike, so tell them apart by their text. Lines after
		// the last match Git counts for a file are context, even when they
		// contain the keyword.
		result.IsMatch = strings.Contains(strings.ToLower(result.Text), w.keyword) &&
			(w.maxPerFile <= 0 || w.fileMatches < w.maxPerFile)
		if result.IsMatch {
			if w.max > 0 && w.matches == w.max {
				w.truncated = true
				w.cancel()
				return len(p), nil
			}
			w.matches++
			w.fileMatches++
		}
		w.results = append(w.results, result)
	}
	return len(p), nil
}

// trimmedResults drops the lines before the first matching line that was left
// out, which only belong to it.
func (w *grepWriter) trimmedResults() []*GrepResult {
	var last *GrepResult
	for _, result := range w.results {
		if result.IsMatch {
			last = result
		}
	}
	results := w.results
	for len(results) > 0 {
		tail := results[len(results)-1]
		if tail.IsMatch || (last != nil && tail.Path == last.Path && tail.Line <= last.Line+w.context) {
			break
		}
		results = results[:len(results)-1]
	}
	return results
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
