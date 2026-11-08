package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_MergeBase(t *testing.T) {
	ctx := context.Background()

	t.Run("bad revision", func(t *testing.T) {
		// "bad_revision" doesn't exist, so git fails with exit status 128 (fatal),
		// not exit status 1 (no merge base).
		mb, err := testrepo.MergeBase(ctx, "0eedd79eba4394bbef888c804e899731644367fe", "bad_revision")
		assert.Error(t, err)
		assert.Empty(t, mb)
	})

	tests := []struct {
		base          string
		head          string
		wantMergeBase string
	}{
		{
			base:          "4e59b72440188e7c2578299fc28ea425fbe9aece",
			head:          "0eedd79eba4394bbef888c804e899731644367fe",
			wantMergeBase: "4e59b72440188e7c2578299fc28ea425fbe9aece",
		},
		{
			base:          "master",
			head:          "release-1.0",
			wantMergeBase: "0eedd79eba4394bbef888c804e899731644367fe",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			mb, err := testrepo.MergeBase(ctx, test.base, test.head)
			require.NoError(t, err)
			assert.Equal(t, test.wantMergeBase, mb)
		})
	}
}

func TestRepository_PullRequestMeta(t *testing.T) {
	ctx := context.Background()

	base, cleanupBase, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanupBase()
	head, cleanupHead, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanupHead()

	mergeBase, err := base.RevParse(ctx, "master")
	require.NoError(t, err)

	// Two commits that change three files on the head branch, plus a commit on
	// the base branch that must not be counted.
	require.NoError(t, head.Checkout(ctx, "feature", CheckoutOptions{BaseBranch: "master"}))
	committer := &Signature{Name: "alice", Email: "alice@example.com", When: time.Now()}
	for i, files := range [][]string{{"a.txt", "b.txt"}, {"c.txt"}} {
		for _, f := range files {
			require.NoError(t, os.WriteFile(filepath.Join(head.Path(), f), []byte(f), 0o644))
		}
		require.NoError(t, head.Add(ctx, AddOptions{All: true}))
		require.NoError(t, head.Commit(ctx, committer, fmt.Sprintf("head %d", i)))
	}
	require.NoError(t, os.WriteFile(filepath.Join(base.Path(), "base.txt"), []byte("base"), 0o644))
	require.NoError(t, base.Add(ctx, AddOptions{All: true}))
	require.NoError(t, base.Commit(ctx, committer, "base"))

	t.Run("fork", func(t *testing.T) {
		snapshot := func() string {
			refs, err := exec(ctx, head.Path(), []string{"for-each-ref"}, nil)
			require.NoError(t, err)
			config, err := exec(ctx, head.Path(), []string{"config", "--local", "--list"}, nil)
			require.NoError(t, err)
			return string(refs) + string(config)
		}
		before := snapshot()

		meta, err := head.PullRequestMeta(ctx, base.Path(), "feature", "master")
		require.NoError(t, err)
		assert.Equal(t, mergeBase, meta.MergeBase)
		require.Len(t, meta.Commits, 2)
		assert.Equal(t, "head 1", meta.Commits[0].Summary())
		assert.Equal(t, 3, meta.NumFiles)

		assert.Equal(t, before, snapshot(), "refs and config of the head repository should be untouched")
		baseCommitID, err := base.BranchCommitID(ctx, "master")
		require.NoError(t, err)
		_, err = exec(ctx, head.Path(), []string{"cat-file", "-e", baseCommitID}, nil)
		assert.Error(t, err, "objects of the base repository should not be copied")
	})

	t.Run("base branch does not exist", func(t *testing.T) {
		_, err := head.PullRequestMeta(ctx, base.Path(), "feature", "404")
		assert.ErrorIs(t, err, ErrReferenceNotExist)
	})

	t.Run("base path contains path list separator and quotes", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip(`Windows does not allow ":" or '"' in file names`)
		}

		root := tempPath()
		defer func() { _ = os.RemoveAll(root) }()
		basePath := filepath.Join(root, `repos:root "x"`, "base.git")
		require.NoError(t, Clone(ctx, base.Path(), basePath, CloneOptions{Bare: true}))

		meta, err := head.PullRequestMeta(ctx, basePath, "feature", "master")
		require.NoError(t, err)
		assert.Equal(t, mergeBase, meta.MergeBase)
		assert.Len(t, meta.Commits, 2)
		assert.Equal(t, 3, meta.NumFiles)
	})

	t.Run("same repository", func(t *testing.T) {
		meta, err := head.PullRequestMeta(ctx, head.Path(), "feature", "master")
		require.NoError(t, err)
		assert.Equal(t, mergeBase, meta.MergeBase)
		assert.Len(t, meta.Commits, 2)
		assert.Equal(t, 3, meta.NumFiles)
	})

	t.Run("no merge base", func(t *testing.T) {
		_, err := exec(ctx, head.Path(), []string{"checkout", "--orphan", "lonely"}, nil)
		require.NoError(t, err)
		require.NoError(t, head.Commit(ctx, committer, "lonely"))

		_, err = head.PullRequestMeta(ctx, base.Path(), "lonely", "master")
		assert.True(t, IsErrNoMergeBase(err), "got %v", err)
	})
}
