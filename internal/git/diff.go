package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DiffLineType is the line type in diff.
type DiffLineType uint8

// A list of different line types.
const (
	DiffLinePlain DiffLineType = iota + 1
	DiffLineAdd
	DiffLineDelete
	DiffLineSection
)

// DiffFileType is the file status in diff.
type DiffFileType uint8

// A list of different file statuses.
const (
	DiffFileAdd DiffFileType = iota + 1
	DiffFileChange
	DiffFileDelete
	DiffFileRename
)

// DiffLine represents a line in diff.
type DiffLine struct {
	Type      DiffLineType // The type of the line
	Content   string       // The content of the line
	LeftLine  int          // The left line number
	RightLine int          // The right line number
}

// DiffSection represents a section in diff.
type DiffSection struct {
	Lines []*DiffLine // lines in the section

	numAdditions int
	numDeletions int
}

// NumLines returns the number of lines in the section.
func (s *DiffSection) NumLines() int {
	return len(s.Lines)
}

// Line returns a specific line by given type and line number in a section.
func (s *DiffSection) Line(typ DiffLineType, line int) *DiffLine {
	var (
		difference      = 0
		addCount        = 0
		delCount        = 0
		matchedDiffLine *DiffLine
	)

loop:
	for _, diffLine := range s.Lines {
		switch diffLine.Type {
		case DiffLineAdd:
			addCount++
		case DiffLineDelete:
			delCount++
		default:
			if matchedDiffLine != nil {
				break loop
			}
			difference = diffLine.RightLine - diffLine.LeftLine
			addCount = 0
			delCount = 0
		}

		switch typ {
		case DiffLineDelete:
			if diffLine.RightLine == 0 && diffLine.LeftLine == line-difference {
				matchedDiffLine = diffLine
			}
		case DiffLineAdd:
			if diffLine.LeftLine == 0 && diffLine.RightLine == line+difference {
				matchedDiffLine = diffLine
			}
		}
	}

	if addCount == delCount {
		return matchedDiffLine
	}
	return nil
}

// DiffFile represents a file in diff.
type DiffFile struct {
	// The name of the file.
	Name string
	// The type of the file.
	Type DiffFileType
	// The index (SHA1 hash) of the file. For a changed/new file, it is the new SHA,
	// and for a deleted file it becomes "000000".
	Index string
	// OldIndex is the old index (SHA1 hash) of the file.
	OldIndex string
	// The sections in the file.
	Sections []*DiffSection

	numAdditions int
	numDeletions int

	oldName string

	mode    EntryMode
	oldMode EntryMode

	isBinary     bool
	isSubmodule  bool
	isIncomplete bool
}

// NumSections returns the number of sections in the file.
func (f *DiffFile) NumSections() int {
	return len(f.Sections)
}

// NumAdditions returns the number of additions in the file.
func (f *DiffFile) NumAdditions() int {
	return f.numAdditions
}

// NumDeletions returns the number of deletions in the file.
func (f *DiffFile) NumDeletions() int {
	return f.numDeletions
}

// IsCreated returns true if the file is newly created.
func (f *DiffFile) IsCreated() bool {
	return f.Type == DiffFileAdd
}

// IsDeleted returns true if the file has been deleted.
func (f *DiffFile) IsDeleted() bool {
	return f.Type == DiffFileDelete
}

// IsRenamed returns true if the file has been renamed.
func (f *DiffFile) IsRenamed() bool {
	return f.Type == DiffFileRename
}

// OldName returns previous name before renaming.
func (f *DiffFile) OldName() string {
	return f.oldName
}

// Mode returns the mode of the file.
func (f *DiffFile) Mode() EntryMode {
	return f.mode
}

// OldMode returns the old mode of the file if it's changed.
func (f *DiffFile) OldMode() EntryMode {
	return f.oldMode
}

// IsBinary returns true if the file is in binary format.
func (f *DiffFile) IsBinary() bool {
	return f.isBinary
}

// IsSubmodule returns true if the file contains information of a submodule.
func (f *DiffFile) IsSubmodule() bool {
	return f.isSubmodule
}

// IsIncomplete returns true if the file is incomplete to the file diff.
func (f *DiffFile) IsIncomplete() bool {
	return f.isIncomplete
}

// Diff represents a Git diff.
type Diff struct {
	Files []*DiffFile // The files in the diff

	totalAdditions int
	totalDeletions int

	isIncomplete bool
}

// NumFiles returns the number of files in the diff.
func (d *Diff) NumFiles() int {
	return len(d.Files)
}

// TotalAdditions returns the total additions in the diff.
func (d *Diff) TotalAdditions() int {
	return d.totalAdditions
}

// TotalDeletions returns the total deletions in the diff.
func (d *Diff) TotalDeletions() int {
	return d.totalDeletions
}

// IsIncomplete returns true if the file is incomplete to the entire diff.
func (d *Diff) IsIncomplete() bool {
	return d.isIncomplete
}

// SteamParseDiffResult contains results of streaming parsing a diff.
type SteamParseDiffResult struct {
	Diff *Diff
	Err  error
}

type diffParser struct {
	*bufio.Reader
	maxFiles     int
	maxFileLines int
	maxLineChars int

	// The next line that hasn't been processed. It is used to determine what kind
	// of process should go in.
	buffer []byte
	isEOF  bool
}

func (p *diffParser) readLine() error {
	if p.buffer != nil {
		return nil
	}

	var err error
	p.buffer, err = p.ReadBytes('\n')
	if err != nil {
		if err != io.EOF {
			return fmt.Errorf("read string: %v", err)
		}

		p.isEOF = true
	}

	// Remove line break
	if len(p.buffer) > 0 && p.buffer[len(p.buffer)-1] == '\n' {
		p.buffer = p.buffer[:len(p.buffer)-1]
	}
	return nil
}

var diffHead = []byte("diff --git ")

func (p *diffParser) parseFileHeader() (*DiffFile, error) {
	line := string(p.buffer)
	p.buffer = nil

	// NOTE: In case file name is surrounded by double quotes (it happens only in
	// git-shell). e.g. diff --git "a/xxx" "b/xxx"
	var middle int
	hasQuote := line[len(diffHead)] == '"'
	if hasQuote {
		middle = strings.Index(line, ` "b/`)
	} else {
		middle = strings.Index(line, ` b/`)
	}

	beg := len(diffHead)
	a := line[beg+2 : middle]
	b := line[middle+3:]
	if hasQuote {
		a = string(UnescapeChars([]byte(a[1 : len(a)-1])))
		b = string(UnescapeChars([]byte(b[1 : len(b)-1])))
	}

	file := &DiffFile{
		Name:    a,
		oldName: b,
		Type:    DiffFileChange,
	}

	// Check file diff type and submodule
	var err error
checkType:
	for !p.isEOF {
		if err = p.readLine(); err != nil {
			return nil, err
		}

		line := string(p.buffer)
		p.buffer = nil

		if len(line) == 0 {
			continue
		}

		switch {
		case strings.HasPrefix(line, "new file"):
			file.Type = DiffFileAdd
			file.isSubmodule = strings.HasSuffix(line, " 160000")
			fields := strings.Fields(line)
			if len(fields) > 0 {
				mode, _ := strconv.ParseUint(fields[len(fields)-1], 8, 64)
				file.mode = EntryMode(mode)
				if file.oldMode == 0 {
					file.oldMode = file.mode
				}
			}
		case strings.HasPrefix(line, "deleted"):
			file.Type = DiffFileDelete
			file.isSubmodule = strings.HasSuffix(line, " 160000")
			fields := strings.Fields(line)
			if len(fields) > 0 {
				mode, _ := strconv.ParseUint(fields[len(fields)-1], 8, 64)
				file.mode = EntryMode(mode)
				if file.oldMode == 0 {
					file.oldMode = file.mode
				}
			}
		case strings.HasPrefix(line, "index"): // e.g. index ee791be..9997571 100644
			fields := strings.Fields(line[6:])
			shas := strings.Split(fields[0], "..")
			if len(shas) != 2 {
				return nil, errors.New("malformed index: expect two SHAs in the form of <old>..<new>")
			}

			file.OldIndex = shas[0]
			file.Index = shas[1]
			if len(fields) > 1 {
				mode, _ := strconv.ParseUint(fields[1], 8, 64)
				file.mode = EntryMode(mode)
				file.oldMode = EntryMode(mode)
			}
			break checkType
		case strings.HasPrefix(line, "similarity index "):
			file.Type = DiffFileRename
			file.oldName = a
			file.Name = b

			// No need to look for index if it's a pure rename
			if strings.HasSuffix(line, "100%") {
				break checkType
			}
		case strings.HasPrefix(line, "new mode"):
			fields := strings.Fields(line)
			if len(fields) > 0 {
				mode, _ := strconv.ParseUint(fields[len(fields)-1], 8, 64)
				file.mode = EntryMode(mode)
			}
		case strings.HasPrefix(line, "old mode"):
			fields := strings.Fields(line)
			if len(fields) > 0 {
				mode, _ := strconv.ParseUint(fields[len(fields)-1], 8, 64)
				file.oldMode = EntryMode(mode)
			}
		}
	}

	return file, nil
}

func (p *diffParser) parseSection() (_ *DiffSection, isIncomplete bool, _ error) {
	line := string(p.buffer)
	p.buffer = nil

	section := &DiffSection{
		Lines: []*DiffLine{
			{
				Type:    DiffLineSection,
				Content: line,
			},
		},
	}

	// Parse line number, e.g. @@ -0,0 +1,3 @@
	var leftLine, rightLine int
	ss := strings.Split(line, "@@")
	ranges := strings.Split(ss[1][1:], " ")
	leftLine, _ = strconv.Atoi(strings.Split(ranges[0], ",")[0][1:])
	if len(ranges) > 1 {
		rightLine, _ = strconv.Atoi(strings.Split(ranges[1], ",")[0])
	} else {
		rightLine = leftLine
	}

	var err error
	for !p.isEOF {
		if err = p.readLine(); err != nil {
			return nil, false, err
		}

		if len(p.buffer) == 0 {
			p.buffer = nil
			continue
		}

		// Make sure we're still in the section. If not, we're done with this section.
		if p.buffer[0] != ' ' &&
			p.buffer[0] != '+' &&
			p.buffer[0] != '-' {

			// No new line indicator
			if p.buffer[0] == '\\' &&
				bytes.HasPrefix(p.buffer, []byte(`\ No newline at end of file`)) {
				p.buffer = nil
				continue
			}
			return section, false, nil
		}

		line := string(p.buffer)
		p.buffer = nil

		// Too many characters in a single diff line
		if p.maxLineChars > 0 && len(line) > p.maxLineChars {
			return section, true, nil
		}

		switch line[0] {
		case ' ':
			section.Lines = append(section.Lines, &DiffLine{
				Type:      DiffLinePlain,
				Content:   line,
				LeftLine:  leftLine,
				RightLine: rightLine,
			})
			leftLine++
			rightLine++
		case '+':
			section.Lines = append(section.Lines, &DiffLine{
				Type:      DiffLineAdd,
				Content:   line,
				RightLine: rightLine,
			})
			section.numAdditions++
			rightLine++
		case '-':
			section.Lines = append(section.Lines, &DiffLine{
				Type:     DiffLineDelete,
				Content:  line,
				LeftLine: leftLine,
			})
			section.numDeletions++
			if leftLine > 0 {
				leftLine++
			}
		}
	}

	return section, false, nil
}

func (p *diffParser) parse() (*Diff, error) {
	diff := new(Diff)
	file := new(DiffFile)
	currentFileLines := 0

	var err error
	for !p.isEOF {
		if err = p.readLine(); err != nil {
			return nil, err
		}

		if len(p.buffer) == 0 ||
			bytes.HasPrefix(p.buffer, []byte("+++ ")) ||
			bytes.HasPrefix(p.buffer, []byte("--- ")) {
			p.buffer = nil
			continue
		}

		// Found new file
		if bytes.HasPrefix(p.buffer, diffHead) {
			// Check if reached maximum number of files
			if p.maxFiles > 0 && len(diff.Files) >= p.maxFiles {
				diff.isIncomplete = true
				_, _ = io.Copy(io.Discard, p)
				break
			}

			file, err = p.parseFileHeader()
			if err != nil {
				return nil, err
			}
			diff.Files = append(diff.Files, file)

			currentFileLines = 0
			continue
		}

		if file == nil || file.isIncomplete {
			p.buffer = nil
			continue
		}

		if bytes.HasPrefix(p.buffer, []byte("Binary")) {
			p.buffer = nil
			file.isBinary = true
			continue
		}

		// Loop until we found section header
		if p.buffer[0] != '@' {
			p.buffer = nil
			continue
		}

		// Too many diff lines for the file
		if p.maxFileLines > 0 && currentFileLines > p.maxFileLines {
			file.isIncomplete = true
			diff.isIncomplete = true
			continue
		}

		section, isIncomplete, err := p.parseSection()
		if err != nil {
			return nil, err
		}
		file.Sections = append(file.Sections, section)
		file.numAdditions += section.numAdditions
		file.numDeletions += section.numDeletions
		diff.totalAdditions += section.numAdditions
		diff.totalDeletions += section.numDeletions
		currentFileLines += section.NumLines()
		if isIncomplete {
			file.isIncomplete = true
			diff.isIncomplete = true
		}
	}

	return diff, nil
}

// StreamParseDiff parses the diff read from the given io.Reader. It does
// parse-on-read to minimize the time spent on huge diffs. It accepts a channel
// to notify and send error (if any) to the caller when the process is done.
// Therefore, this method should be called in a goroutine asynchronously.
func StreamParseDiff(r io.Reader, done chan<- SteamParseDiffResult, maxFiles, maxFileLines, maxLineChars int) {
	p := &diffParser{
		Reader:       bufio.NewReader(r),
		maxFiles:     maxFiles,
		maxFileLines: maxFileLines,
		maxLineChars: maxLineChars,
	}
	diff, err := p.parse()
	done <- SteamParseDiffResult{
		Diff: diff,
		Err:  err,
	}
}
