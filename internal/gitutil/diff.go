// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io"
	"sync"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
)

// DiffSection is a wrapper to git.DiffSection with helper methods.
type DiffSection struct {
	*git.DiffSection

	initOnce sync.Once
	dmp      *diffmatchpatch.DiffMatchPatch
}

// ComputedInlineDiffFor computes inline diff for the given line.
func (s *DiffSection) ComputedInlineDiffFor(line *git.DiffLine) template.HTML {
	fallback := template.HTML(html.EscapeString(line.Content))
	if conf.Git.DisableDiffHighlight {
		return fallback
	}

	// Find equivalent diff line, ignore when not found.
	var diff1, diff2 string
	switch line.Type {
	case git.DiffLineAdd:
		compareLine := s.Line(git.DiffLineDelete, line.RightLine)
		if compareLine == nil {
			return fallback
		}

		diff1 = compareLine.Content
		diff2 = line.Content

	case git.DiffLineDelete:
		compareLine := s.Line(git.DiffLineAdd, line.LeftLine)
		if compareLine == nil {
			return fallback
		}

		diff1 = line.Content
		diff2 = compareLine.Content

	default:
		return fallback
	}

	s.initOnce.Do(func() {
		s.dmp = diffmatchpatch.New()
		s.dmp.DiffEditCost = 100
	})

	diffs := s.dmp.DiffMain(diff1[1:], diff2[1:], true)
	diffs = s.dmp.DiffCleanupEfficiency(diffs)

	return diffsToHTML(diffs, line.Type)
}

func diffsToHTML(diffs []diffmatchpatch.Diff, lineType git.DiffLineType) template.HTML {
	buf := bytes.NewBuffer(nil)

	// Reproduce signs which are cutted for inline diff before.
	switch lineType {
	case git.DiffLineAdd:
		buf.WriteByte('+')
	case git.DiffLineDelete:
		buf.WriteByte('-')
	}
	buf.WriteByte(' ')

	const (
		addedCodePrefix   = `<span class="added-code">`
		removedCodePrefix = `<span class="removed-code">`
		codeTagSuffix     = `</span>`
	)

	for i := range diffs {
		switch {
		case diffs[i].Type == diffmatchpatch.DiffInsert && lineType == git.DiffLineAdd:
			buf.WriteString(addedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.WriteString(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffDelete && lineType == git.DiffLineDelete:
			buf.WriteString(removedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.WriteString(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffEqual:
			buf.WriteString(html.EscapeString(diffs[i].Text))
		}
	}

	return template.HTML(buf.Bytes())
}

// DiffFile is a wrapper to git.DiffFile with helper methods.
type DiffFile struct {
	*git.DiffFile
	Sections []*DiffSection
}

// HighlightClass returns the detected highlight class for the file.
func (diffFile *DiffFile) HighlightClass() string {
	return highlight.FileNameToHighlightClass(diffFile.Name)
}

// Diff is a wrapper to git.Diff with helper methods.
type Diff struct {
	*git.Diff
	Files []*DiffFile
}

// NewDiff returns a new wrapper of given git.Diff.
func NewDiff(oldDiff *git.Diff) *Diff {
	newDiff := &Diff{
		Diff:  oldDiff,
		Files: make([]*DiffFile, oldDiff.NumFiles()),
	}

	// FIXME: detect encoding while parsing.
	var buf bytes.Buffer
	for i := range oldDiff.Files {
		buf.Reset()

		newDiff.Files[i] = &DiffFile{
			DiffFile: oldDiff.Files[i],
			Sections: make([]*DiffSection, oldDiff.Files[i].NumSections()),
		}

		for j := range oldDiff.Files[i].Sections {
			newDiff.Files[i].Sections[j] = &DiffSection{
				DiffSection: oldDiff.Files[i].Sections[j],
			}

			for k := range newDiff.Files[i].Sections[j].Lines {
				buf.WriteString(newDiff.Files[i].Sections[j].Lines[k].Content)
				buf.WriteString("\n")
			}
		}

		charsetLabel, err := tool.DetectEncoding(buf.Bytes())
		if charsetLabel != "UTF-8" && err == nil {
			encoding, _ := charset.Lookup(charsetLabel)
			if encoding != nil {
				d := encoding.NewDecoder()
				for j := range newDiff.Files[i].Sections {
					for k := range newDiff.Files[i].Sections[j].Lines {
						if c, _, err := transform.String(d, newDiff.Files[i].Sections[j].Lines[k].Content); err == nil {
							newDiff.Files[i].Sections[j].Lines[k].Content = c
						}
					}
				}
			}
		}
	}

	return newDiff
}

// ParseDiff parses the diff from given io.Reader.
func ParseDiff(r io.Reader, maxFiles, maxFileLines, maxLineChars int) (*Diff, error) {
	done := make(chan git.SteamParseDiffResult)
	go git.StreamParseDiff(r, done, maxFiles, maxFileLines, maxLineChars)

	result := <-done
	if result.Err != nil {
		return nil, fmt.Errorf("stream parse diff: %v", result.Err)
	}
	return NewDiff(result.Diff), nil
}

// RepoDiff parses the diff on given revisions of given repository.
func RepoDiff(repo *git.Repository, rev string, maxFiles, maxFileLines, maxLineChars int, opts ...git.DiffOptions) (*Diff, error) {
	diff, err := repo.Diff(rev, maxFiles, maxFileLines, maxLineChars, opts...)
	if err != nil {
		return nil, fmt.Errorf("get diff: %v", err)
	}
	return NewDiff(diff), nil
}
