package git

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapLineRange(t *testing.T) {
	// Without context: 2 lines inserted after line 3, line 10 replaced by 3
	// lines, and lines 20 to 21 deleted.
	hunks := ParseDiffHunks([]byte(`diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3,0 +4,2 @@ func a() {
+x
+y
@@ -10 +12,3 @@ func b() {
-old
+new1
+new2
+new3
@@ -20,2 +23,0 @@ func c() {
-gone1
-gone2
`))
	require.Len(t, hunks, 3)

	tests := []struct {
		name               string
		start, end         int
		wantStart, wantEnd int
		wantOK             bool
	}{
		{name: "before every change", start: 1, end: 3, wantStart: 1, wantEnd: 3, wantOK: true},
		{name: "after the insertion", start: 4, end: 9, wantStart: 6, wantEnd: 11, wantOK: true},
		{name: "replaced line", start: 10, end: 10, wantOK: false},
		{name: "range ending on the replaced line", start: 8, end: 10, wantOK: false},
		{name: "insertion inside the range", start: 2, end: 5, wantOK: false},
		{name: "between replacement and deletion", start: 11, end: 19, wantStart: 15, wantEnd: 23, wantOK: true},
		{name: "deleted line", start: 21, end: 21, wantOK: false},
		{name: "after every change", start: 30, end: 30, wantStart: 32, wantEnd: 32, wantOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, ok := MapLineRange(hunks, test.start, test.end)
			assert.Equal(t, test.wantOK, ok)
			if test.wantOK {
				assert.Equal(t, test.wantStart, start)
				assert.Equal(t, test.wantEnd, end)
			}
		})
	}
}

func TestDiffHunk_LinesEndingAt(t *testing.T) {
	hunks := ParseDiffHunks([]byte(`@@ -5,4 +5,5 @@
 a
-b
+B
+C
 d
 e
`))
	require.Len(t, hunks, 1)
	h := hunks[0]

	assert.Equal(t, []string{" a", "-b", "+B", "+C"}, h.LinesEndingAt(false, 7, 4))
	assert.Equal(t, []string{"-b", "+B", "+C", " d"}, h.LinesEndingAt(false, 8, 4))
	assert.Equal(t, []string{" a", "-b"}, h.LinesEndingAt(true, 6, 4))
	assert.Nil(t, h.LinesEndingAt(false, 20, 4))

	assert.Same(t, h, HunkContaining(hunks, false, 6, 9))
	assert.Nil(t, HunkContaining(hunks, false, 6, 10))
	assert.Same(t, h, HunkContaining(hunks, true, 8, 8))
}

func TestRepository_DiffFileHunks(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := osexec.Command("git", append([]string{"-c", "user.name=a", "-c", "user.email=a@example.com"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}
	run("init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	run("add", ".")
	run("commit", "-q", "-m", "a")
	base := run("rev-parse", "HEAD")
	run("mv", "a.txt", "b.txt")
	lines[4] = "five"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	run("add", ".")
	run("commit", "-q", "-m", "b")
	head := run("rev-parse", "HEAD")

	repo, err := Open(dir)
	require.NoError(t, err)
	ctx := context.Background()

	// A renamed file diffs against its old path, and shows only the change.
	hunks, err := repo.DiffFileHunks(ctx, base, head, "a.txt", "b.txt", 0)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, []string{"-line 5", "+five"}, hunks[0].Lines)

	// Without the old path, the file is all new.
	hunks, err = repo.DiffFileHunks(ctx, base, head, "", "b.txt", 0)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, 0, hunks[0].OldLines)

	// Paths are not pathspec patterns.
	hunks, err = repo.DiffFileHunks(ctx, base, head, "", ":(glob)*.txt", 0)
	require.NoError(t, err)
	assert.Empty(t, hunks)
	hunks, err = repo.DiffFileHunks(ctx, base, head, "", "*.txt", 0)
	require.NoError(t, err)
	assert.Empty(t, hunks)
}
