package git

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// BlameCommit is a commit that last changed some lines of a file.
type BlameCommit struct {
	ID        OID
	Author    *Signature
	Committer *Signature
	Summary   string
	// PreviousID is the commit before this one that has the file, at
	// PreviousPath, which differs from the path after a rename. It is nil when
	// the commit added the file.
	PreviousID   OID
	PreviousPath string
}

// BlameHunk is a run of consecutive lines that the same commit last changed.
type BlameHunk struct {
	Commit *BlameCommit
	// StartLine is the number of the first line of the hunk, counting from 1.
	StartLine int
	Lines     []string
}

// Blame returns which commit last changed each line of the file at the path in
// the given revision, as runs of lines in the order of the file.
func (r *Repository) Blame(ctx context.Context, rev, path string) ([]*BlameHunk, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.Blame")
	defer span.End()

	stdout, err := exec(ctx, r.path, []string{"blame", "--porcelain", rev, "--", path}, nil)
	if err != nil {
		if strings.Contains(err.Error(), "no such path") || strings.Contains(err.Error(), "bad revision") {
			return nil, ErrRevisionNotExist
		}
		return nil, err
	}
	return parseBlame(stdout)
}

// parseBlame parses the output of "git blame --porcelain", which prints the
// details of a commit only the first time one of its lines appears.
func parseBlame(data []byte) ([]*BlameHunk, error) {
	commits := make(map[string]*BlameCommit)
	var hunks []*BlameHunk
	var current *BlameCommit
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), len(data)+1)
	for scanner.Scan() {
		line := scanner.Text()
		if content, ok := strings.CutPrefix(line, "\t"); ok {
			if current == nil {
				return nil, errors.New("line content before its commit")
			}
			last := len(hunks) - 1
			if last >= 0 && hunks[last].Commit == current {
				hunks[last].Lines = append(hunks[last].Lines, content)
				continue
			}
			startLine := 1
			if last >= 0 {
				startLine = hunks[last].StartLine + len(hunks[last].Lines)
			}
			hunks = append(hunks, &BlameHunk{Commit: current, StartLine: startLine, Lines: []string{content}})
			continue
		}

		key, value, _ := strings.Cut(line, " ")
		if len(key) == 40 || len(key) == 64 {
			if commit, ok := commits[key]; ok {
				current = commit
				continue
			}
			id, err := newIDFromString(key)
			if err == nil {
				current = &BlameCommit{ID: id, Author: &Signature{}, Committer: &Signature{}}
				commits[key] = current
				continue
			}
		}
		if current == nil {
			return nil, errors.Newf("unexpected line %q", line)
		}
		switch key {
		case "author":
			current.Author.Name = value
		case "author-mail":
			current.Author.Email = strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")
		case "author-time":
			current.Author.When = blameTime(value)
		case "author-tz":
			current.Author.When = blameZone(current.Author.When, value)
		case "committer":
			current.Committer.Name = value
		case "committer-mail":
			current.Committer.Email = strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")
		case "committer-time":
			current.Committer.When = blameTime(value)
		case "committer-tz":
			current.Committer.When = blameZone(current.Committer.When, value)
		case "summary":
			current.Summary = value
		case "previous":
			sha, path, _ := strings.Cut(value, " ")
			id, err := newIDFromString(sha)
			if err != nil {
				return nil, errors.Wrapf(err, "parse previous commit %q", sha)
			}
			current.PreviousID = id
			current.PreviousPath = unquotePath(path)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "scan")
	}
	return hunks, nil
}

// blameTime returns the time of the Unix timestamp. "git blame" prints its
// zone on the next line.
func blameTime(value string) time.Time {
	sec, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// blameZone returns the time in the zone of the offset, e.g., "-0700".
func blameZone(when time.Time, offset string) time.Time {
	if len(offset) != 5 {
		return when
	}
	hours, err1 := strconv.Atoi(offset[1:3])
	minutes, err2 := strconv.Atoi(offset[3:])
	if err1 != nil || err2 != nil {
		return when
	}
	seconds := hours*3600 + minutes*60
	if offset[0] == '-' {
		seconds = -seconds
	}
	return when.In(time.FixedZone("", seconds))
}

// unquotePath returns the path that Git quoted for having special characters,
// e.g., a tab or a non-ASCII character, as is.
func unquotePath(path string) string {
	if !strings.HasPrefix(path, `"`) {
		return path
	}
	if unquoted, err := strconv.Unquote(path); err == nil {
		return unquoted
	}
	return path
}
