// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

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

	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/template/highlight"
	"github.com/gogs/gogs/pkg/tool"
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
	case git.DIFF_LINE_ADD:
		buf.WriteByte('+')
	case git.DIFF_LINE_DEL:
		buf.WriteByte('-')
	}

	for i := range diffs {
		switch {
		case diffs[i].Type == diffmatchpatch.DiffInsert && lineType == git.DIFF_LINE_ADD:
			buf.Write(addedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.Write(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffDelete && lineType == git.DIFF_LINE_DEL:
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
	if setting.Git.DisableDiffHighlight {
		return template.HTML(html.EscapeString(diffLine.Content[1:]))
	}
	var (
		compareDiffLine *git.DiffLine
		diff1           string
		diff2           string
	)

	// try to find equivalent diff line. ignore, otherwise
	switch diffLine.Type {
	case git.DIFF_LINE_ADD:
		compareDiffLine = diffSection.Line(git.DIFF_LINE_DEL, diffLine.RightIdx)
		if compareDiffLine == nil {
			return template.HTML(html.EscapeString(diffLine.Content))
		}
		diff1 = compareDiffLine.Content
		diff2 = diffLine.Content
	case git.DIFF_LINE_DEL:
		compareDiffLine = diffSection.Line(git.DIFF_LINE_ADD, diffLine.LeftIdx)
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

func NewDiff(gitDiff *git.Diff) *Diff {
	diff := &Diff{
		Diff:  gitDiff,
		Files: make([]*DiffFile, gitDiff.NumFiles()),
	}

	// FIXME: detect encoding while parsing.
	var buf bytes.Buffer
	for i := range gitDiff.Files {
		buf.Reset()

		diff.Files[i] = &DiffFile{
			DiffFile: gitDiff.Files[i],
			Sections: make([]*DiffSection, gitDiff.Files[i].NumSections()),
		}

		for j := range gitDiff.Files[i].Sections {
			diff.Files[i].Sections[j] = &DiffSection{
				DiffSection: gitDiff.Files[i].Sections[j],
			}

			for k := range diff.Files[i].Sections[j].Lines {
				buf.WriteString(diff.Files[i].Sections[j].Lines[k].Content)
				buf.WriteString("\n")
			}
		}

		charsetLabel, err := tool.DetectEncoding(buf.Bytes())
		if charsetLabel != "UTF-8" && err == nil {
			encoding, _ := charset.Lookup(charsetLabel)
			if encoding != nil {
				d := encoding.NewDecoder()
				for j := range diff.Files[i].Sections {
					for k := range diff.Files[i].Sections[j].Lines {
						if c, _, err := transform.String(d, diff.Files[i].Sections[j].Lines[k].Content); err == nil {
							diff.Files[i].Sections[j].Lines[k].Content = c
						}
					}
				}
			}
		}
	}

	return diff
}

func ParsePatch(maxLines, maxLineCharacteres, maxFiles int, reader io.Reader) (*Diff, error) {
	done := make(chan error)
	var gitDiff *git.Diff
	go func() {
		gitDiff = git.ParsePatch(done, maxLines, maxLineCharacteres, maxFiles, reader)
	}()

	if err := <-done; err != nil {
		return nil, fmt.Errorf("ParsePatch: %v", err)
	}
	return NewDiff(gitDiff), nil
}

func GetDiffRange(repoPath, beforeCommitID, afterCommitID string, maxLines, maxLineCharacteres, maxFiles int) (*Diff, error) {
	gitDiff, err := git.GetDiffRange(repoPath, beforeCommitID, afterCommitID, maxLines, maxLineCharacteres, maxFiles)
	if err != nil {
		return nil, fmt.Errorf("GetDiffRange: %v", err)
	}
	return NewDiff(gitDiff), nil
}

func GetDiffCommit(repoPath, commitID string, maxLines, maxLineCharacteres, maxFiles int) (*Diff, error) {
	gitDiff, err := git.GetDiffCommit(repoPath, commitID, maxLines, maxLineCharacteres, maxFiles)
	if err != nil {
		return nil, fmt.Errorf("GetDiffCommit: %v", err)
	}
	return NewDiff(gitDiff), nil
}
