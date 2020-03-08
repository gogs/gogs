// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"html/template"
	"testing"

	dmp "github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"

	"github.com/gogs/git-module"
)

func Test_diffsToHTML(t *testing.T) {
	tests := []struct {
		diffs    []dmp.Diff
		lineType git.DiffLineType
		expHTML  template.HTML
	}{
		{
			diffs: []dmp.Diff{
				{Type: dmp.DiffEqual, Text: "foo "},
				{Type: dmp.DiffInsert, Text: "bar"},
				{Type: dmp.DiffDelete, Text: " baz"},
				{Type: dmp.DiffEqual, Text: " biz"},
			},
			lineType: git.DiffLineAdd,
			expHTML:  template.HTML(`+foo <span class="added-code">bar</span> biz`),
		},
		{
			diffs: []dmp.Diff{
				{Type: dmp.DiffEqual, Text: "foo "},
				{Type: dmp.DiffDelete, Text: "bar"},
				{Type: dmp.DiffInsert, Text: " baz"},
				{Type: dmp.DiffEqual, Text: " biz"},
			},
			lineType: git.DiffLineDelete,
			expHTML:  template.HTML(`-foo <span class="removed-code">bar</span> biz`),
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expHTML, diffsToHTML(test.diffs, test.lineType))
		})
	}
}
