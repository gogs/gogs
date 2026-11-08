package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRepository_CommitThroughIndex(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)

	parent, err := testrepo.BranchCommit(ctx, "master")
	require.NoError(t, err)
	require.NoError(t, r.ReadTree(ctx, parent.ID.String()))

	blobID, err := r.HashObject(ctx, strings.NewReader("hello\n"), HashObjectOptions{Path: "new/dir/hello.sh"})
	require.NoError(t, err)
	require.NoError(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryExec, ID: blobID, Path: "new/dir/hello.sh"}))
	require.NoError(t, r.RemoveFromIndex(ctx, "README.txt"))

	t.Run("invalid changes", func(t *testing.T) {
		// "run.sh" is a file, so nothing can be added under it.
		assert.Error(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryBlob, ID: blobID, Path: "run.sh/x"}))
		assert.Error(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryBlob, ID: blobID, Path: ".git/config"}))
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
		Parents: []string{parent.ID.String()},
		Author:  author,
	})
	require.NoError(t, err)

	raw, err := r.rawCommit(ctx, commitID)
	require.NoError(t, err)
	assert.Equal(t, parent.ID.String(), raw.parent)
	assert.Equal(t, "Add hello\n", raw.message)
	assert.Equal(t, "alice", raw.author.Name)
	assert.Equal(t, "alice@example.com", raw.author.Email)
	assert.Equal(t, "2024-01-02T03:04:05+05:30", raw.author.When.Format(time.RFC3339))

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

func TestRepository_DiffCached(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)

	parent, err := testrepo.BranchCommit(ctx, "master")
	require.NoError(t, err)
	require.NoError(t, r.ReadTree(ctx, parent.ID.String()))
	for _, path := range []string{"README.txt", "pom.xml"} {
		blobID, err := r.HashObject(ctx, strings.NewReader("changed\n"))
		require.NoError(t, err)
		require.NoError(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryBlob, ID: blobID, Path: path}))
	}

	diff, err := r.DiffCached(ctx, parent.ID.String(), "README.txt", 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, diff.Files, 1)
	assert.Equal(t, "README.txt", diff.Files[0].Name)
	assert.Equal(t, 1, diff.Files[0].NumAdditions())
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
	require.NoError(t, r.Checkout(ctx, "feature", CheckoutOptions{BaseBranch: "master"}))
	commitFile(t, r, "FEATURE", "feature")
	featureID, err := r.BranchCommitID(ctx, "feature")
	require.NoError(t, err)
	require.NoError(t, r.Checkout(ctx, "conflict", CheckoutOptions{BaseBranch: "master"}))
	commitFile(t, r, "README.txt", "conflict")
	conflictID, err := r.BranchCommitID(ctx, "conflict")
	require.NoError(t, err)
	require.NoError(t, r.Checkout(ctx, "master"))
	commitFile(t, r, "README.txt", "master")
	newMasterID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)

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
		blobID, err := r.HashObject(ctx, strings.NewReader(content))
		require.NoError(t, err)
		require.NoError(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryBlob, ID: blobID, Path: name}))
		tree, err := r.WriteTree(ctx)
		require.NoError(t, err)
		id, err := r.CommitTree(ctx, tree, message, CommitTreeOptions{
			Parents: []string{parent},
			Author: &Signature{
				Name:  "bob",
				Email: "bob@example.com",
				When:  time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", -7*3600)),
			},
		})
		require.NoError(t, err)
		return id
	}

	base, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)
	upstream := commitOnto(base, "UPSTREAM", "upstream\n", "Add UPSTREAM\n")

	t.Run("replays commits on top of the upstream", func(t *testing.T) {
		first := commitOnto(base, "A", "a\n", "Add A\n\nWith a body.\n")
		second := commitOnto(first, "B", "b\n", "Add B")

		got, err := r.Rebase(ctx, upstream, second)
		require.NoError(t, err)

		rebasedSecond, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "Add B", rebasedSecond.message)
		assert.Equal(t, "2024-01-02T03:04:05-07:00", rebasedSecond.author.When.Format(time.RFC3339))
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
		got, err := r.Rebase(ctx, upstream, head)
		require.NoError(t, err)
		assert.Equal(t, head, got)
	})

	t.Run("drops commits already in the upstream", func(t *testing.T) {
		same := commitOnto(base, "UPSTREAM", "upstream\n", "Add UPSTREAM again\n")
		head := commitOnto(same, "D", "d\n", "Add D\n")

		got, err := r.Rebase(ctx, upstream, head)
		require.NoError(t, err)
		rebased, err := r.rawCommit(ctx, got)
		require.NoError(t, err)
		assert.Equal(t, "Add D\n", rebased.message)
		assert.Equal(t, upstream, rebased.parent)
	})

	t.Run("conflict", func(t *testing.T) {
		head := commitOnto(base, "UPSTREAM", "different\n", "Change UPSTREAM\n")
		_, err := r.Rebase(ctx, upstream, head)
		assert.ErrorIs(t, err, ErrMergeConflict)
	})
}
