// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

// DiffLineType represents the type of a line in diff.
type DiffLineType uint8

const (
	DIFF_LINE_PLAIN DiffLineType = iota + 1
	DIFF_LINE_ADD
	DIFF_LINE_DEL
	DIFF_LINE_SECTION
)

// DiffFileType represents the file status in diff.
type DiffFileType uint8

const (
	DIFF_FILE_ADD DiffFileType = iota + 1
	DIFF_FILE_CHANGE
	DIFF_FILE_DEL
	DIFF_FILE_RENAME
)

// DiffLine represents a line in diff.
type DiffLine struct {
	LeftIdx  int
	RightIdx int
	Type     DiffLineType
	Content  string
}

func (d *DiffLine) GetType() int {
	return int(d.Type)
}

// DiffSection represents a section in diff.
type DiffSection struct {
	Name  string
	Lines []*DiffLine
}

// Line returns a specific line by type (add or del) and file line number from a section.
func (diffSection *DiffSection) Line(lineType DiffLineType, idx int) *DiffLine {
	var (
		difference    = 0
		addCount      = 0
		delCount      = 0
		matchDiffLine *DiffLine
	)

LOOP:
	for _, diffLine := range diffSection.Lines {
		switch diffLine.Type {
		case DIFF_LINE_ADD:
			addCount++
		case DIFF_LINE_DEL:
			delCount++
		default:
			if matchDiffLine != nil {
				break LOOP
			}
			difference = diffLine.RightIdx - diffLine.LeftIdx
			addCount = 0
			delCount = 0
		}

		switch lineType {
		case DIFF_LINE_DEL:
			if diffLine.RightIdx == 0 && diffLine.LeftIdx == idx-difference {
				matchDiffLine = diffLine
			}
		case DIFF_LINE_ADD:
			if diffLine.LeftIdx == 0 && diffLine.RightIdx == idx+difference {
				matchDiffLine = diffLine
			}
		}
	}

	if addCount == delCount {
		return matchDiffLine
	}
	return nil
}

// DiffFile represents a file in diff.
type DiffFile struct {
	Name               string
	OldName            string
	Index              string // 40-byte SHA, Changed/New: new SHA; Deleted: old SHA
	Addition, Deletion int
	Type               DiffFileType
	IsCreated          bool
	IsDeleted          bool
	IsBin              bool
	IsRenamed          bool
	IsSubmodule        bool
	Sections           []*DiffSection
	IsIncomplete       bool
}

func (diffFile *DiffFile) GetType() int {
	return int(diffFile.Type)
}

func (diffFile *DiffFile) NumSections() int {
	return len(diffFile.Sections)
}

// Diff contains all information of a specific diff output.
type Diff struct {
	TotalAddition, TotalDeletion int
	Files                        []*DiffFile
	IsIncomplete                 bool
}

func (diff *Diff) NumFiles() int {
	return len(diff.Files)
}

const _DIFF_HEAD = "diff --git "

// ParsePatch takes a reader and parses everything it receives in diff format.
func ParsePatch(done chan<- error, maxLines, maxLineCharacteres, maxFiles int, reader io.Reader) *Diff {
	var (
		diff = &Diff{Files: make([]*DiffFile, 0)}

		curFile    *DiffFile
		curSection = &DiffSection{
			Lines: make([]*DiffLine, 0, 10),
		}

		leftLine, rightLine int
		lineCount           int
		curFileLinesCount   int
	)
	input := bufio.NewReader(reader)
	isEOF := false
	for !isEOF {
		// TODO: would input.ReadBytes be more memory-efficient?
		line, err := input.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				isEOF = true
			} else {
				done <- fmt.Errorf("ReadString: %v", err)
				return nil
			}
		}

		if len(line) > 0 && line[len(line)-1] == '\n' {
			// Remove line break.
			line = line[:len(line)-1]
		}

		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") || len(line) == 0 {
			continue
		}

		curFileLinesCount++
		lineCount++

		// Diff data too large, we only show the first about maxlines lines
		if curFileLinesCount >= maxLines || len(line) >= maxLineCharacteres {
			curFile.IsIncomplete = true
		}

		switch {
		case line[0] == ' ':
			diffLine := &DiffLine{Type: DIFF_LINE_PLAIN, Content: line, LeftIdx: leftLine, RightIdx: rightLine}
			leftLine++
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		case line[0] == '@':
			curSection = &DiffSection{}
			curFile.Sections = append(curFile.Sections, curSection)
			ss := strings.Split(line, "@@")
			diffLine := &DiffLine{Type: DIFF_LINE_SECTION, Content: line}
			curSection.Lines = append(curSection.Lines, diffLine)

			// Parse line number.
			ranges := strings.Split(ss[1][1:], " ")
			leftLine, _ = strconv.Atoi(strings.Split(ranges[0], ",")[0][1:])
			if len(ranges) > 1 {
				rightLine, _ = strconv.Atoi(strings.Split(ranges[1], ",")[0])
			} else {
				rightLine = leftLine
			}
			continue
		case line[0] == '+':
			curFile.Addition++
			diff.TotalAddition++
			diffLine := &DiffLine{Type: DIFF_LINE_ADD, Content: line, RightIdx: rightLine}
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		case line[0] == '-':
			curFile.Deletion++
			diff.TotalDeletion++
			diffLine := &DiffLine{Type: DIFF_LINE_DEL, Content: line, LeftIdx: leftLine}
			if leftLine > 0 {
				leftLine++
			}
			curSection.Lines = append(curSection.Lines, diffLine)
		case strings.HasPrefix(line, "Binary"):
			curFile.IsBin = true
			continue
		}

		// Get new file.
		if strings.HasPrefix(line, _DIFF_HEAD) {
			middle := -1

			// Note: In case file name is surrounded by double quotes (it happens only in git-shell).
			// e.g. diff --git "a/xxx" "b/xxx"
			hasQuote := line[len(_DIFF_HEAD)] == '"'
			if hasQuote {
				middle = strings.Index(line, ` "b/`)
			} else {
				middle = strings.Index(line, " b/")
			}

			beg := len(_DIFF_HEAD)
			a := line[beg+2 : middle]
			b := line[middle+3:]
			if hasQuote {
				a = string(UnescapeChars([]byte(a[1 : len(a)-1])))
				b = string(UnescapeChars([]byte(b[1 : len(b)-1])))
			}

			curFile = &DiffFile{
				Name:     a,
				Type:     DIFF_FILE_CHANGE,
				Sections: make([]*DiffSection, 0, 10),
			}
			diff.Files = append(diff.Files, curFile)
			if len(diff.Files) >= maxFiles {
				diff.IsIncomplete = true
				io.Copy(ioutil.Discard, reader)
				break
			}
			curFileLinesCount = 0

			// Check file diff type and submodule.
		CHECK_TYPE:
			for {
				line, err := input.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						isEOF = true
					} else {
						done <- fmt.Errorf("ReadString: %v", err)
						return nil
					}
				}

				switch {
				case strings.HasPrefix(line, "new file"):
					curFile.Type = DIFF_FILE_ADD
					curFile.IsCreated = true
					curFile.IsSubmodule = strings.HasSuffix(line, " 160000\n")
				case strings.HasPrefix(line, "deleted"):
					curFile.Type = DIFF_FILE_DEL
					curFile.IsDeleted = true
					curFile.IsSubmodule = strings.HasSuffix(line, " 160000\n")
				case strings.HasPrefix(line, "index"):
					if curFile.IsDeleted {
						curFile.Index = line[6:46]
					} else if len(line) >= 88 {
						curFile.Index = line[49:88]
					} else {
						curFile.Index = curFile.Name
					}
					break CHECK_TYPE
				case strings.HasPrefix(line, "similarity index 100%"):
					curFile.Type = DIFF_FILE_RENAME
					curFile.IsRenamed = true
					curFile.OldName = curFile.Name
					curFile.Name = b
					curFile.Index = b
					break CHECK_TYPE
				case strings.HasPrefix(line, "old mode"):
					break CHECK_TYPE
				}
			}
		}
	}

	done <- nil
	return diff
}

// GetDiffRange returns a parsed diff object between given commits.
func GetDiffRange(repoPath, beforeCommitID, afterCommitID string, maxLines, maxLineCharacteres, maxFiles int) (*Diff, error) {
	repo, err := OpenRepository(repoPath)
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(afterCommitID)
	if err != nil {
		return nil, err
	}

	cmd := NewCommand()
	if len(beforeCommitID) == 0 {
		// First commit of repository
		if commit.ParentCount() == 0 {
			cmd.AddArguments("show", "--full-index", afterCommitID)
		} else {
			c, _ := commit.Parent(0)
			cmd.AddArguments("diff", "--full-index", "-M", c.ID.String(), afterCommitID)
		}
	} else {
		cmd.AddArguments("diff", "--full-index", "-M", beforeCommitID, afterCommitID)
	}

	stdout, w := io.Pipe()
	done := make(chan error)
	var diff *Diff
	go func() {
		diff = ParsePatch(done, maxLines, maxLineCharacteres, maxFiles, stdout)
	}()

	stderr := new(bytes.Buffer)
	err = cmd.RunInDirTimeoutPipeline(2*time.Minute, repoPath, w, stderr)
	w.Close() // Close writer to exit parsing goroutine
	if err != nil {
		return nil, concatenateError(err, stderr.String())
	}

	return diff, <-done
}

// RawDiffType represents the type of raw diff format.
type RawDiffType string

const (
	RAW_DIFF_NORMAL RawDiffType = "diff"
	RAW_DIFF_PATCH  RawDiffType = "patch"
)

// GetRawDiff dumps diff results of repository in given commit ID to io.Writer.
func GetRawDiff(repoPath, commitID string, diffType RawDiffType, writer io.Writer) error {
	repo, err := OpenRepository(repoPath)
	if err != nil {
		return fmt.Errorf("OpenRepository: %v", err)
	}

	commit, err := repo.GetCommit(commitID)
	if err != nil {
		return err
	}

	cmd := NewCommand()
	switch diffType {
	case RAW_DIFF_NORMAL:
		if commit.ParentCount() == 0 {
			cmd.AddArguments("show", commitID)
		} else {
			c, _ := commit.Parent(0)
			cmd.AddArguments("diff", "-M", c.ID.String(), commitID)
		}
	case RAW_DIFF_PATCH:
		if commit.ParentCount() == 0 {
			cmd.AddArguments("format-patch", "--no-signature", "--stdout", "--root", commitID)
		} else {
			c, _ := commit.Parent(0)
			query := fmt.Sprintf("%s...%s", commitID, c.ID.String())
			cmd.AddArguments("format-patch", "--no-signature", "--stdout", query)
		}
	default:
		return fmt.Errorf("invalid diffType: %s", diffType)
	}

	stderr := new(bytes.Buffer)
	if err = cmd.RunInDirPipeline(repoPath, writer, stderr); err != nil {
		return concatenateError(err, stderr.String())
	}
	return nil
}

// GetDiffCommit returns a parsed diff object of given commit.
func GetDiffCommit(repoPath, commitID string, maxLines, maxLineCharacteres, maxFiles int) (*Diff, error) {
	return GetDiffRange(repoPath, "", commitID, maxLines, maxLineCharacteres, maxFiles)
}
