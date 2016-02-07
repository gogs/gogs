package models

import (
	dmp "github.com/sergi/go-diff/diffmatchpatch"
	"html/template"
	"testing"
)

func assertEqual(t *testing.T, s1 string, s2 template.HTML) {
	if s1 != string(s2) {
		t.Errorf("%s should be equal %s", s2, s1)
	}
}

func assertLineEqual(t *testing.T, d1 *DiffLine, d2 *DiffLine) {
	if d1 != d2 {
		t.Errorf("%v should be equal %v", d1, d2)
	}
}

func TestDiffToHTML(t *testing.T) {
	assertEqual(t, "foo <span class=\"added-code\">bar</span> biz", diffToHTML([]dmp.Diff{
		dmp.Diff{dmp.DiffEqual, "foo "},
		dmp.Diff{dmp.DiffInsert, "bar"},
		dmp.Diff{dmp.DiffDelete, " baz"},
		dmp.Diff{dmp.DiffEqual, " biz"},
	}, DIFF_LINE_ADD))

	assertEqual(t, "foo <span class=\"removed-code\">bar</span> biz", diffToHTML([]dmp.Diff{
		dmp.Diff{dmp.DiffEqual, "foo "},
		dmp.Diff{dmp.DiffDelete, "bar"},
		dmp.Diff{dmp.DiffInsert, " baz"},
		dmp.Diff{dmp.DiffEqual, " biz"},
	}, DIFF_LINE_DEL))
}

// test if GetLine is return the correct lines
func TestGetLine(t *testing.T) {
	ds := DiffSection{Lines: []*DiffLine{
		&DiffLine{LeftIdx: 28, RightIdx: 28, Type: DIFF_LINE_PLAIN},
		&DiffLine{LeftIdx: 29, RightIdx: 29, Type: DIFF_LINE_PLAIN},
		&DiffLine{LeftIdx: 30, RightIdx: 30, Type: DIFF_LINE_PLAIN},
		&DiffLine{LeftIdx: 31, RightIdx: 0, Type: DIFF_LINE_DEL},
		&DiffLine{LeftIdx: 0, RightIdx: 31, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 0, RightIdx: 32, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 32, RightIdx: 33, Type: DIFF_LINE_PLAIN},
		&DiffLine{LeftIdx: 33, RightIdx: 0, Type: DIFF_LINE_DEL},
		&DiffLine{LeftIdx: 34, RightIdx: 0, Type: DIFF_LINE_DEL},
		&DiffLine{LeftIdx: 35, RightIdx: 0, Type: DIFF_LINE_DEL},
		&DiffLine{LeftIdx: 36, RightIdx: 0, Type: DIFF_LINE_DEL},
		&DiffLine{LeftIdx: 0, RightIdx: 34, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 0, RightIdx: 35, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 0, RightIdx: 36, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 0, RightIdx: 37, Type: DIFF_LINE_ADD},
		&DiffLine{LeftIdx: 37, RightIdx: 38, Type: DIFF_LINE_PLAIN},
		&DiffLine{LeftIdx: 38, RightIdx: 39, Type: DIFF_LINE_PLAIN},
	}}

	assertLineEqual(t, ds.GetLine(DIFF_LINE_ADD, 31), ds.Lines[4])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_DEL, 31), ds.Lines[3])

	assertLineEqual(t, ds.GetLine(DIFF_LINE_ADD, 33), ds.Lines[11])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_ADD, 34), ds.Lines[12])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_ADD, 35), ds.Lines[13])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_ADD, 36), ds.Lines[14])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_DEL, 34), ds.Lines[7])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_DEL, 35), ds.Lines[8])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_DEL, 36), ds.Lines[9])
	assertLineEqual(t, ds.GetLine(DIFF_LINE_DEL, 37), ds.Lines[10])
}
