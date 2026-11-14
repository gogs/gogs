package git

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

// setupBareRepoWithAlternates returns an empty bare repository that reads
// objects from the test repository, the way Gogs builds commits.
func setupBareRepoWithAlternates(t *testing.T) *Repository {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, Init(context.Background(), path, InitOptions{Bare: true}))
	objects, err := filepath.Abs(filepath.Join(testrepo.Path(), "objects"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(path, "objects", "info", "alternates"), []byte(objects+"\n"), 0o600))
	r, err := Open(path)
	require.NoError(t, err)
	return r
}

// createBranch creates the branch at the commit of the master branch.
func createBranch(t *testing.T, r *Repository, branch string) {
	t.Helper()
	_, err := exec(context.Background(), r.Path(), []string{"branch", "--end-of-options", branch, "master"}, nil)
	require.NoError(t, err)
}

// commitFiles commits the files with given contents to the branch, which is
// created as an orphan if it does not exist, and returns the commit ID.
func commitFiles(t *testing.T, r *Repository, branch string, files map[string]string) string {
	t.Helper()

	ctx := context.Background()
	var parents []string
	if parent, err := r.BranchCommitID(ctx, branch); err == nil {
		parents = append(parents, parent)
		require.NoError(t, r.ReadTree(ctx, parent))
	} else {
		objectFormat, err := r.ObjectFormat(ctx)
		require.NoError(t, err)
		require.NoError(t, r.ReadTree(ctx, objectFormat.EmptyTreeID()))
	}

	var names []string
	for name, content := range files {
		require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: name, Content: strings.NewReader(content)}))
		names = append(names, name)
	}
	slices.Sort(names)

	tree, err := r.WriteTree(ctx)
	require.NoError(t, err)
	sig := &Signature{Name: "alice", Email: "alice@example.com"}
	id, err := r.CommitTree(ctx, tree, "Update "+strings.Join(names, ", ")+"\n", CommitTreeOptions{
		Parents:   parents,
		Author:    sig,
		Committer: sig,
	})
	require.NoError(t, err)
	_, err = exec(ctx, r.Path(), []string{"update-ref", "--end-of-options", RefsHeads + branch, id}, nil)
	require.NoError(t, err)
	return id
}

func TestRepository_CommitThroughIndex(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)

	parent, err := testrepo.BranchCommit(ctx, "master")
	require.NoError(t, err)
	require.NoError(t, r.ReadTree(ctx, parent.ID.String()))

	require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: "new/dir/hello.sh", Content: strings.NewReader("hello\n"), Executable: true}))
	require.NoError(t, r.RemoveFromIndex(ctx, "README.txt"))
	// A path that also exists in the Git directory of the bare repository.
	require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: "config", Content: strings.NewReader("hello\n")}))
	require.NoError(t, r.RemoveFromIndex(ctx, "config"))

	t.Run("committer from Git configuration", func(t *testing.T) {
		tree, err := r.WriteTree(ctx)
		require.NoError(t, err)

		t.Setenv("GIT_CONFIG_COUNT", "2")
		t.Setenv("GIT_CONFIG_KEY_0", "user.name")
		t.Setenv("GIT_CONFIG_VALUE_0", "carol")
		t.Setenv("GIT_CONFIG_KEY_1", "user.email")
		t.Setenv("GIT_CONFIG_VALUE_1", "carol@example.com")
		id, err := r.CommitTree(ctx, tree, "message\n", CommitTreeOptions{
			Author: &Signature{Name: "bob", Email: "bob@example.com"},
		})
		require.NoError(t, err)

		c, err := r.CatFileCommit(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, "carol", c.Committer.Name)
		assert.Equal(t, "carol@example.com", c.Committer.Email)
		assert.Equal(t, "bob", c.Author.Name)
		assert.Equal(t, "bob@example.com", c.Author.Email)
		assert.Equal(t, 0, c.ParentsCount())
	})

	t.Run("invalid changes", func(t *testing.T) {
		// "run.sh" is a file, so nothing can be added under it.
		for _, path := range []string{"run.sh/x", ".git/config", "../x"} {
			assert.Error(t, r.AddToIndex(ctx, IndexFile{Path: path, Content: strings.NewReader("x\n")}), path)
		}
		assert.Error(t, r.RemoveFromIndex(ctx, "does-not-exist"))
		// Paths are taken literally, not as patterns.
		assert.Error(t, r.RemoveFromIndex(ctx, "*.sh"))
		// Removing a directory needs to be explicit.
		assert.Error(t, r.RemoveFromIndex(ctx, "src"))
	})

	tree, err := r.WriteTree(ctx)
	require.NoError(t, err)
	author := &Signature{
		Name:  "alice",
		Email: "alice@example.com",
		When:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", 5*3600+30*60)),
	}
	commitID, err := r.CommitTree(ctx, tree, "Add hello\n", CommitTreeOptions{
		Parents:   []string{parent.ID.String()},
		Author:    author,
		Committer: author,
	})
	require.NoError(t, err)

	raw, err := r.rawCommit(ctx, commitID)
	require.NoError(t, err)
	assert.Equal(t, parent.ID.String(), raw.parent)
	assert.Equal(t, "Add hello\n", raw.message)
	assert.Equal(t, "alice <alice@example.com> 1704144845 +0530", raw.authorLine)

	commit, err := r.CatFileCommit(ctx, commitID)
	require.NoError(t, err)
	entry, err := commit.TreeEntry(ctx, "new/dir/hello.sh")
	require.NoError(t, err)
	assert.True(t, entry.IsExec())
	_, err = commit.TreeEntry(ctx, "README.txt")
	assert.True(t, IsErrRevisionNotExist(err))
	_, err = commit.TreeEntry(ctx, "run.sh")
	assert.NoError(t, err)
}

func TestRepository_AddToIndex(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	// Git for Windows sets core.autocrlf to true in its system configuration,
	// which would convert the line endings of files without attributes.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.autocrlf")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")

	commitFiles(t, r, "crlf", map[string]string{"a.txt": "one\r\ntwo\r\n"})
	parent := commitFiles(t, r, "crlf", map[string]string{".gitattributes": "*.txt text=auto\n"})

	require.NoError(t, r.ReadTree(ctx, parent))
	require.NoError(t, r.AddToIndex(ctx,
		IndexFile{Path: "a.txt", Content: strings.NewReader("one\r\nTWO\r\n")},
		IndexFile{Path: "new.txt", Content: strings.NewReader("new\r\n")},
		IndexFile{Path: "up/.gitattributes", Content: strings.NewReader("* text\r\n")},
		IndexFile{Path: "up/b.dat", Content: strings.NewReader("b\r\n")},
		IndexFile{Path: "c.dat", Content: strings.NewReader("c\r\n"), Executable: true},
	))

	for path, want := range map[string]string{
		// A file with CRLF line endings in the index keeps them.
		"a.txt":   "one\r\nTWO\r\n",
		"new.txt": "new\n",
		// Added ".gitattributes" files apply to all added files, themselves included.
		"up/.gitattributes": "* text\n",
		"up/b.dat":          "b\n",
		"c.dat":             "c\r\n",
	} {
		got, err := exec(ctx, r.Path(), []string{"cat-file", "blob", ":" + path}, nil)
		require.NoError(t, err)
		assert.Equal(t, want, string(got), path)
	}
	stage, err := exec(ctx, r.Path(), []string{"ls-files", "--stage", "--", "c.dat", "up/b.dat"}, nil)
	require.NoError(t, err)
	assert.Regexp(t, `^100755 \S+ 0\tc.dat\n100644 \S+ 0\tup/b.dat\n$`, string(stage))
}

func TestRepository_DiffCached(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)

	parent, err := testrepo.BranchCommit(ctx, "master")
	require.NoError(t, err)
	require.NoError(t, r.ReadTree(ctx, parent.ID.String()))
	for _, path := range []string{"README.txt", "pom.xml"} {
		require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: path, Content: strings.NewReader("changed\n")}))
	}

	diff, err := r.DiffCached(ctx, logx.NewNoopLogger(), parent.ID.String(), "README.txt", 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, diff.Files, 1)
	assert.Equal(t, "README.txt", diff.Files[0].Name)
	assert.Equal(t, 1, diff.Files[0].NumAdditions())

	t.Run("follows .gitattributes of the revision", func(t *testing.T) {
		r, cleanup, err := setupTempRepo()
		require.NoError(t, err)
		defer cleanup()

		rev := commitFiles(t, r, "attributes", map[string]string{".gitattributes": "*.dat -diff\n", "a.dat": "a\n"})
		require.NoError(t, r.ReadTree(ctx, rev))
		require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: "a.dat", Content: strings.NewReader("b\n")}))

		diff, err := r.DiffCached(ctx, logx.NewNoopLogger(), rev, "a.dat", 0, 0, 0)
		require.NoError(t, err)
		require.Len(t, diff.Files, 1)
		assert.True(t, diff.Files[0].IsBinary())
	})
}

func TestRepository_IsValidBranchName(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"feature", "feature/a", "v1.0"} {
		assert.True(t, testrepo.IsValidBranchName(ctx, name), name)
	}
	for _, name := range []string{"HEAD", "-x", "a..b", "@{-1}", "x.lock", ""} {
		assert.False(t, testrepo.IsValidBranchName(ctx, name), name)
	}
}

func TestCleanupMessage(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{message: "", want: ""},
		{message: " \n\t\n", want: ""},
		{message: "summary", want: "summary\n"},
		{message: "\n\nsummary  \n\n\n\nbody\t\r\n# not a comment\n\n", want: "summary\n\nbody\n# not a comment\n"},
		{message: "a\n \nb\n", want: "a\n\nb\n"},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, CleanupMessage(test.message), "message: %q", test.message)
	}
}

func TestRepository_MergeTree(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	masterID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)
	createBranch(t, r, "feature")
	featureID := commitFiles(t, r, "feature", map[string]string{"FEATURE": "feature"})
	createBranch(t, r, "conflict")
	conflictID := commitFiles(t, r, "conflict", map[string]string{"README.txt": "conflict"})
	newMasterID := commitFiles(t, r, "master", map[string]string{"README.txt": "master"})

	tree, err := r.MergeTree(ctx, newMasterID, featureID)
	require.NoError(t, err)
	stdout, err := exec(ctx, r.Path(), []string{"ls-tree", "--name-only", tree}, nil)
	require.NoError(t, err)
	assert.Contains(t, strings.Fields(string(stdout)), "FEATURE")
	blob, err := exec(ctx, r.Path(), []string{"cat-file", "blob", tree + ":README.txt"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "master", string(blob))

	_, err = r.MergeTree(ctx, newMasterID, conflictID)
	assert.ErrorIs(t, err, ErrMergeConflict)

	t.Run("follows .gitattributes of ours", func(t *testing.T) {
		lines := "1\n2\n3\n4\n5\n6\n7\n8\n9\n"
		base := commitFiles(t, r, "attributes", map[string]string{".gitattributes": "data.txt -merge\n", "data.txt": lines})
		_, err := exec(ctx, r.Path(), []string{"update-ref", RefsHeads + "attributes-theirs", base}, nil)
		require.NoError(t, err)
		ours := commitFiles(t, r, "attributes", map[string]string{"data.txt": strings.Replace(lines, "2", "X", 1)})
		theirs := commitFiles(t, r, "attributes-theirs", map[string]string{"data.txt": strings.Replace(lines, "8", "Y", 1)})

		_, err = r.MergeTree(ctx, ours, theirs)
		assert.ErrorIs(t, err, ErrMergeConflict)
	})

	ok, err := r.IsAncestor(ctx, masterID, featureID)
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = r.IsAncestor(ctx, featureID, newMasterID)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRepository_Rebase(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	// commitOnto creates a commit that changes the file on top of the parent,
	// authored in a time zone other than the local one.
	commitOnto := func(parent, name, content, message string) string {
		t.Helper()
		require.NoError(t, r.ReadTree(ctx, parent))
		require.NoError(t, r.AddToIndex(ctx, IndexFile{Path: name, Content: strings.NewReader(content)}))
		tree, err := r.WriteTree(ctx)
		require.NoError(t, err)
		sig := &Signature{
			Name:  "bob",
			Email: "bob@example.com",
			When:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", -7*3600)),
		}
		id, err := r.CommitTree(ctx, tree, message, CommitTreeOptions{
			Parents:   []string{parent},
			Author:    sig,
			Committer: sig,
		})
		require.NoError(t, err)
		return id
	}

	base, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)
	upstream := commitOnto(base, "UPSTREAM", "upstream\n", "Add UPSTREAM\n")
	committer := &Signature{Name: "carol", Email: "carol@example.com"}

	t.Run("requires a committer", func(t *testing.T) {
		_, err := r.Rebase(ctx, upstream, base, nil)
		assert.EqualError(t, err, "committer is required")
	})

	t.Run("replays commits on top of the upstream", func(t *testing.T) {
		first := commitOnto(base, "A", "a\n", "Add A\n\nWith a body.\n")
		second := commitOnto(first, "B", "b\n", "Add B")

		got, err := r.Rebase(ctx, upstream, second, committer)
		require.NoError(t, err)

		rebasedSecond, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "Add B", rebasedSecond.message)
		assert.Equal(t, "bob <bob@example.com> 1704189845 -0700", rebasedSecond.authorLine)
		c, err := r.CatFileCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, committer.Name, c.Committer.Name)
		assert.Equal(t, committer.Email, c.Committer.Email)
		rebasedFirst, err := r.rawCommit(ctx, rebasedSecond.parent)
		require.NoError(t, err)
		assert.Equal(t, "Add A\n\nWith a body.\n", rebasedFirst.message)
		assert.Equal(t, upstream, rebasedFirst.parent)

		stdout, err := exec(ctx, r.Path(), []string{"ls-tree", "--name-only", got}, nil)
		require.NoError(t, err)
		assert.Subset(t, strings.Fields(string(stdout)), []string{"A", "B", "UPSTREAM"})
	})

	t.Run("keeps commits already on top of the upstream", func(t *testing.T) {
		head := commitOnto(upstream, "C", "c\n", "Add C\n")
		got, err := r.Rebase(ctx, upstream, head, committer)
		require.NoError(t, err)
		assert.Equal(t, head, got)
	})

	t.Run("drops commits already in the upstream", func(t *testing.T) {
		same := commitOnto(base, "UPSTREAM", "upstream\n", "Add UPSTREAM again\n")
		head := commitOnto(same, "D", "d\n", "Add D\n")

		got, err := r.Rebase(ctx, upstream, head, committer)
		require.NoError(t, err)
		rebased, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "Add D\n", rebased.message)
		assert.Equal(t, upstream, rebased.parent)
	})

	t.Run("keeps authors as they are and re-encodes in UTF-8", func(t *testing.T) {
		first := commitOnto(base, "E", "e\n", "Add E\n")
		tree, err := r.RevParse(ctx, first+"^{tree}")
		require.NoError(t, err)

		// "张三" and "中文" in GBK.
		object := "tree " + tree + "\nparent " + base + "\n" +
			"author \"\xd5\xc5\xc8\xfd\" <z@example.com> 1600000000 +0800\n" +
			"committer c <c@example.com> 1600000000 +0000\nencoding GBK\n\n\xd6\xd0\xce\xc4\n"
		stdout, err := execInput(ctx, r.Path(), []string{"hash-object", "-t", "commit", "-w", "--stdin"}, nil, strings.NewReader(object))
		require.NoError(t, err)

		got, err := r.Rebase(ctx, upstream, strings.TrimSpace(string(stdout)), committer)
		require.NoError(t, err)
		raw, err := exec(ctx, r.Path(), []string{"cat-file", "commit", got}, nil)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "\nauthor \"张三\" <z@example.com> 1600000000 +0800\n")
		assert.NotContains(t, string(raw), "\nencoding ")
		assert.True(t, strings.HasSuffix(string(raw), "\n\n中文\n"), "got %q", raw)
	})

	// writeRaw writes the commit object as is.
	writeRaw := func(header, message string) string {
		t.Helper()
		first := commitOnto(base, "F", "f\n", "Add F\n")
		tree, err := r.RevParse(ctx, first+"^{tree}")
		require.NoError(t, err)
		object := "tree " + tree + "\nparent " + base + "\n" + header + "\n" + message
		stdout, err := execInput(ctx, r.Path(), []string{"hash-object", "-t", "commit", "-w", "--stdin", "--literally"}, nil, strings.NewReader(object))
		require.NoError(t, err)
		return strings.TrimSpace(string(stdout))
	}

	t.Run("uses the first author line", func(t *testing.T) {
		head := writeRaw("author first <f@example.com> 1 +0000\nauthor second <s@example.com> 2 +0000\ncommitter c <c@example.com> 1 +0000\n", "m\n")
		got, err := r.Rebase(ctx, upstream, head, committer)
		require.NoError(t, err)
		rebased, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "first <f@example.com> 1 +0000", rebased.authorLine)
	})

	t.Run("refuses commits without author", func(t *testing.T) {
		head := writeRaw("committer c <c@example.com> 1 +0000\n", "m\n")
		_, err := r.Rebase(ctx, upstream, head, committer)
		assert.ErrorContains(t, err, "no author")
	})

	t.Run("ignores log.showSignature", func(t *testing.T) {
		head := writeRaw("author a <a@example.com> 1 +0000\ncommitter c <c@example.com> 1 +0000\ngpgsig -----BEGIN PGP SIGNATURE-----\n \n bad\n -----END PGP SIGNATURE-----\n", "signed\n")
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", "log.showSignature")
		t.Setenv("GIT_CONFIG_VALUE_0", "true")

		got, err := r.Rebase(ctx, upstream, head, committer)
		require.NoError(t, err)
		rebased, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "signed\n", rebased.message)
	})

	t.Run("conflict", func(t *testing.T) {
		head := commitOnto(base, "UPSTREAM", "different\n", "Change UPSTREAM\n")
		_, err := r.Rebase(ctx, upstream, head, committer)
		assert.ErrorIs(t, err, ErrMergeConflict)
	})
}

func TestRepository_RebaseRootCommit(t *testing.T) {
	for _, objectFormat := range []ObjectFormat{ObjectFormatSHA1, ObjectFormatSHA256} {
		t.Run(string(objectFormat), func(t *testing.T) {
			ctx := context.Background()
			path := t.TempDir()
			require.NoError(t, Init(ctx, path, InitOptions{Bare: true, ObjectFormat: objectFormat}))
			r, err := Open(path)
			require.NoError(t, err)
			committer := &Signature{Name: "carol", Email: "carol@example.com"}

			upstream := commitFiles(t, r, "main", map[string]string{"README": "readme\n"})

			t.Run("replays a root commit", func(t *testing.T) {
				root := commitFiles(t, r, "unrelated", map[string]string{"OTHER": "other\n"})

				got, err := r.Rebase(ctx, upstream, root, committer)
				require.NoError(t, err)
				rebased, err := r.rawCommit(ctx, got)
				require.NoError(t, err)
				assert.Equal(t, upstream, rebased.parent)

				stdout, err := exec(ctx, r.Path(), []string{"ls-tree", "--name-only", got}, nil)
				require.NoError(t, err)
				assert.Equal(t, []string{"OTHER", "README"}, strings.Fields(string(stdout)))
			})

			t.Run("keeps an empty root commit", func(t *testing.T) {
				root, err := r.CommitTree(ctx, objectFormat.EmptyTreeID(), "Empty\n", CommitTreeOptions{
					Author:    committer,
					Committer: committer,
				})
				require.NoError(t, err)

				got, err := r.Rebase(ctx, upstream, root, committer)
				require.NoError(t, err)
				rebased, err := r.rawCommit(ctx, got)
				require.NoError(t, err)
				assert.Equal(t, upstream, rebased.parent)
				assert.Equal(t, "Empty\n", rebased.message)
			})
		})
	}
}
