package git

import (
	"html/template"
	"testing"

	dmp "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"
)

func TestDiffsToHTML(t *testing.T) {
	tests := []struct {
		diffs    []dmp.Diff
		lineType DiffLineType
		expHTML  template.HTML
	}{
		{
			diffs: []dmp.Diff{
				{Type: dmp.DiffEqual, Text: "foo "},
				{Type: dmp.DiffInsert, Text: "bar"},
				{Type: dmp.DiffDelete, Text: " baz"},
				{Type: dmp.DiffEqual, Text: " biz"},
			},
			lineType: DiffLineAdd,
			expHTML:  template.HTML(`+foo <span class="added-code">bar</span> biz`),
		},
		{
			diffs: []dmp.Diff{
				{Type: dmp.DiffEqual, Text: "foo "},
				{Type: dmp.DiffDelete, Text: "bar"},
				{Type: dmp.DiffInsert, Text: " baz"},
				{Type: dmp.DiffEqual, Text: " biz"},
			},
			lineType: DiffLineDelete,
			expHTML:  template.HTML(`-foo <span class="removed-code">bar</span> biz`),
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expHTML, diffsToHTML(test.diffs, test.lineType))
		})
	}
}
