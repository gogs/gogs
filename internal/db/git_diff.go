// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
)

type DiffSection struct {
	*git.DiffSection
}

var (
	addedCodePrefix   = []byte("<span class=\"added-code\">")
	removedCodePrefix = []byte("<span class=\"removed-code\">")
	codeTagSuffix     = []byte("</span>")
)

func diffToHTML(diffs []diffmatchpatch.Diff, lineType git.DiffLineType) template.HTML {
	buf := bytes.NewBuffer(nil)

	// Reproduce signs which are cutted for inline diff before.
	switch lineType {
	case git.DiffLineAdd:
		buf.WriteByte('+')
	case git.DiffLineDelete:
		buf.WriteByte('-')
	}

	for i := range diffs {
		switch {
		case diffs[i].Type == diffmatchpatch.DiffInsert && lineType == git.DiffLineAdd:
			buf.Write(addedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.Write(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffDelete && lineType == git.DiffLineDelete:
			buf.Write(removedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.Write(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffEqual:
			buf.WriteString(html.EscapeString(diffs[i].Text))
		}
	}

	return template.HTML(buf.Bytes())
}

var diffMatchPatch = diffmatchpatch.New()

func init() {
	diffMatchPatch.DiffEditCost = 100
}

// ComputedInlineDiffFor computes inline diff for the given line.
func (diffSection *DiffSection) ComputedInlineDiffFor(diffLine *git.DiffLine) template.HTML {
	if conf.Git.DisableDiffHighlight {
		return template.HTML(html.EscapeString(diffLine.Content[1:]))
	}
	var (
		compareDiffLine *git.DiffLine
		diff1           string
		diff2           string
	)

	// try to find equivalent diff line. ignore, otherwise
	switch diffLine.Type {
	case git.DiffLineAdd:
		compareDiffLine = diffSection.Line(git.DiffLineDelete, diffLine.RightLine)
		if compareDiffLine == nil {
			return template.HTML(html.EscapeString(diffLine.Content))
		}
		diff1 = compareDiffLine.Content
		diff2 = diffLine.Content
	case git.DiffLineDelete:
		compareDiffLine = diffSection.Line(git.DiffLineAdd, diffLine.LeftLine)
		if compareDiffLine == nil {
			return template.HTML(html.EscapeString(diffLine.Content))
		}
		diff1 = diffLine.Content
		diff2 = compareDiffLine.Content
	default:
		return template.HTML(html.EscapeString(diffLine.Content))
	}

	diffRecord := diffMatchPatch.DiffMain(diff1[1:], diff2[1:], true)
	diffRecord = diffMatchPatch.DiffCleanupEfficiency(diffRecord)

	return diffToHTML(diffRecord, diffLine.Type)
}

type DiffFile struct {
	*git.DiffFile
	Sections []*DiffSection
}

func (diffFile *DiffFile) HighlightClass() string {
	return highlight.FileNameToHighlightClass(diffFile.Name)
}

type Diff struct {
	*git.Diff
	Files []*DiffFile
}

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

func ParseDiff(r io.Reader, maxFiles, maxFileLines, maxLineChars int) (*Diff, error) {
	done := make(chan git.SteamParseDiffResult)
	go git.StreamParseDiff(r, done, maxFiles, maxFileLines, maxLineChars)

	result := <-done
	if result.Err != nil {
		return nil, fmt.Errorf("stream parse diff: %v", result.Err)
	}
	return NewDiff(result.Diff), nil
}

func RepoDiff(gitRepo *git.Repository, rev string, maxFiles, maxFileLines, maxLineChars int, opts ...git.DiffOptions) (*Diff, error) {
	gitDiff, err := gitRepo.Diff(rev, maxFiles, maxFileLines, maxLineChars, opts...)
	if err != nil {
		return nil, fmt.Errorf("get diff: %v", err)
	}
	return NewDiff(gitDiff), nil
}
