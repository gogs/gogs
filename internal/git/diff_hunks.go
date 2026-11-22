package git

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
)

// DiffHunk is a hunk of the diff of one file.
type DiffHunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	// Lines are the lines of the hunk with their " ", "-" or "+" prefix.
	Lines []string
}

// DiffFileHunks returns the hunks of the diff of the file at path between the
// base and head revisions, with the given number of lines of context. When
// the file was renamed from oldPath, the diff is of the old file at base to
// the file at head. Otherwise renames are not followed, so a renamed file
// shows as added at its new path.
func (r *Repository) DiffFileHunks(ctx context.Context, base, head, oldPath, path string, contextLines int) ([]*DiffHunk, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.DiffFileHunks")
	defer span.End()

	// Paths are literal, so a path cannot select other files with the magic
	// of pathspecs, e.g., ":(glob)**".
	diff := func(renames bool, paths ...string) ([]byte, error) {
		args := []string{"-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "-U" + strconv.Itoa(contextLines)}
		if renames {
			args = append(args, "--find-renames")
		} else {
			args = append(args, "--no-renames")
		}
		args = append(args, "--end-of-options", base, head, "--")
		for _, p := range paths {
			args = append(args, ":(literal)"+p)
		}
		return exec(ctx, r.path, args, nil)
	}

	if oldPath != "" && oldPath != path {
		stdout, err := diff(true, oldPath, path)
		if err != nil {
			return nil, err
		}
		// Only the diff of a rename from oldPath to path counts. Two files
		// that changed on their own are diffed apart, and path alone is
		// diffed below.
		lines := append([]byte("\n"), stdout...)
		if bytes.Count(lines, []byte("\ndiff --git ")) == 1 &&
			bytes.Contains(lines, []byte("\nrename from "+oldPath+"\n")) &&
			bytes.Contains(lines, []byte("\nrename to "+path+"\n")) {
			return ParseDiffHunks(stdout), nil
		}
	}
	stdout, err := diff(false, path)
	if err != nil {
		return nil, err
	}
	return ParseDiffHunks(stdout), nil
}

// ParseDiffHunks parses the hunks of the diff of one file.
func ParseDiffHunks(diff []byte) []*DiffHunk {
	var hunks []*DiffHunk
	var cur *DiffHunk
	scanner := bufio.NewScanner(bytes.NewReader(diff))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@ ") {
			cur = parseHunkHeader(line)
			if cur != nil {
				hunks = append(hunks, cur)
			}
			continue
		}
		if cur == nil || line == "" {
			continue
		}
		switch line[0] {
		case ' ', '-', '+':
			cur.Lines = append(cur.Lines, line)
		}
	}
	return hunks
}

// parseHunkHeader parses a header such as "@@ -12,3 +12,5 @@ func main() {".
func parseHunkHeader(line string) *DiffHunk {
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return nil
	}
	var h DiffHunk
	var ok bool
	if h.OldStart, h.OldLines, ok = parseHunkRange(fields[1][1:]); !ok {
		return nil
	}
	if h.NewStart, h.NewLines, ok = parseHunkRange(fields[2][1:]); !ok {
		return nil
	}
	return &h
}

func parseHunkRange(s string) (start, count int, ok bool) {
	startStr, countStr, hasCount := strings.Cut(s, ",")
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return 0, 0, false
	}
	count = 1
	if hasCount {
		if count, err = strconv.Atoi(countStr); err != nil {
			return 0, 0, false
		}
	}
	return start, count, true
}

// HunkContaining returns the hunk that shows the lines from start to end on the
// old side (when old is true) or the new side of the diff, or nil when no
// single hunk shows them all.
func HunkContaining(hunks []*DiffHunk, old bool, start, end int) *DiffHunk {
	for _, h := range hunks {
		first, count := h.NewStart, h.NewLines
		if old {
			first, count = h.OldStart, h.OldLines
		}
		if count > 0 && start >= first && end < first+count {
			return h
		}
	}
	return nil
}

// LinesEndingAt returns up to limit lines of the hunk, with their prefix, that
// end at the given line of the old side (when old is true) or the new side.
func (h *DiffHunk) LinesEndingAt(old bool, line, limit int) []string {
	oldLine, newLine := h.OldStart, h.NewStart
	for i, l := range h.Lines {
		var at int
		switch l[0] {
		case ' ':
			at = newLine
			if old {
				at = oldLine
			}
			oldLine++
			newLine++
		case '-':
			at = -1
			if old {
				at = oldLine
			}
			oldLine++
		case '+':
			at = -1
			if !old {
				at = newLine
			}
			newLine++
		}
		if at == line {
			return h.Lines[max(0, i+1-limit) : i+1]
		}
	}
	return nil
}

// MapLineRange maps the lines from start to end of the old revision of a
// file to the new revision, given the hunks of the diff between them without
// context lines. It reports false when the diff changes any of the lines or
// adds lines between them.
func MapLineRange(hunks []*DiffHunk, start, end int) (newStart, newEnd int, ok bool) {
	offset := 0
	for _, h := range hunks {
		if h.OldLines == 0 {
			// Lines are added after OldStart.
			if h.OldStart < start {
				offset += h.NewLines
				continue
			}
			if h.OldStart < end {
				return 0, 0, false
			}
			break
		}
		last := h.OldStart + h.OldLines - 1
		if last < start {
			offset += h.NewLines - h.OldLines
			continue
		}
		if h.OldStart <= end {
			return 0, 0, false
		}
		break
	}
	return start + offset, end + offset, true
}
