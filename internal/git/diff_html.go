package git

import (
	"bytes"
	"html"
	"html/template"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
)

// ComputedInlineDiffFor computes inline diff for the given line.
func (s *DiffSection) ComputedInlineDiffFor(line *DiffLine) template.HTML {
	fallback := template.HTML(html.EscapeString(line.Content))
	if conf.Git.DisableDiffHighlight {
		return fallback
	}

	// Find equivalent diff line, ignore when not found.
	var diff1, diff2 string
	switch line.Type {
	case DiffLineAdd:
		compareLine := s.Line(DiffLineDelete, line.RightLine)
		if compareLine == nil {
			return fallback
		}

		diff1 = compareLine.Content
		diff2 = line.Content

	case DiffLineDelete:
		compareLine := s.Line(DiffLineAdd, line.LeftLine)
		if compareLine == nil {
			return fallback
		}

		diff1 = line.Content
		diff2 = compareLine.Content

	default:
		return fallback
	}

	dmp := diffmatchpatch.New()
	dmp.DiffEditCost = 100
	diffs := dmp.DiffMain(diff1[1:], diff2[1:], true)
	diffs = dmp.DiffCleanupEfficiency(diffs)

	return diffsToHTML(diffs, line.Type)
}

func diffsToHTML(diffs []diffmatchpatch.Diff, lineType DiffLineType) template.HTML {
	buf := bytes.NewBuffer(nil)

	// Reproduce signs which are cut for inline diff before.
	switch lineType {
	case DiffLineAdd:
		buf.WriteByte('+')
	case DiffLineDelete:
		buf.WriteByte('-')
	}

	const (
		addedCodePrefix   = `<span class="added-code">`
		removedCodePrefix = `<span class="removed-code">`
		codeTagSuffix     = `</span>`
	)

	for i := range diffs {
		switch {
		case diffs[i].Type == diffmatchpatch.DiffInsert && lineType == DiffLineAdd:
			buf.WriteString(addedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.WriteString(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffDelete && lineType == DiffLineDelete:
			buf.WriteString(removedCodePrefix)
			buf.WriteString(html.EscapeString(diffs[i].Text))
			buf.WriteString(codeTagSuffix)
		case diffs[i].Type == diffmatchpatch.DiffEqual:
			buf.WriteString(html.EscapeString(diffs[i].Text))
		}
	}

	return template.HTML(buf.Bytes())
}

// HighlightClass returns the detected highlight class for the file.
func (f *DiffFile) HighlightClass() string {
	return highlight.FileNameToHighlightClass(f.Name)
}

// decodeCharsets converts the content of each file in the diff to UTF-8 when
// the file is detected to be in another encoding.
func decodeCharsets(logger *logx.Logger, diff *Diff) {
	var buf bytes.Buffer
	for _, file := range diff.Files {
		buf.Reset()
		for _, section := range file.Sections {
			for _, line := range section.Lines {
				buf.WriteString(line.Content)
				buf.WriteString("\n")
			}
		}

		charsetLabel, err := tool.DetectEncoding(logger, buf.Bytes())
		if charsetLabel == "UTF-8" || err != nil {
			continue
		}
		encoding, _ := charset.Lookup(charsetLabel)
		if encoding == nil {
			continue
		}
		d := encoding.NewDecoder()
		for _, section := range file.Sections {
			for _, line := range section.Lines {
				if c, _, err := transform.String(d, line.Content); err == nil {
					line.Content = c
				}
			}
		}
	}
}
