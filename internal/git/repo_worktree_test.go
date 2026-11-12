package git

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_WorktreeAdd(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	t.Run("detached HEAD", func(t *testing.T) {
		path := tempPath()
		defer func() { _ = os.RemoveAll(path) }()

		sha, err := r.RevParse(ctx, "master")
		require.NoError(t, err)

		err = r.WorktreeAdd(ctx, path, sha)
		assert.NoError(t, err)
	})

	t.Run("new branch", func(t *testing.T) {
		path := tempPath()
		defer func() { _ = os.RemoveAll(path) }()

		err := r.WorktreeAdd(ctx, path, "master", WorktreeAddOptions{
			Branch: "test-worktree-branch",
		})
		assert.NoError(t, err)
	})
}

func TestRepository_WorktreeRemove(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	t.Run("remove worktree", func(t *testing.T) {
		path := tempPath()

		err := r.WorktreeAdd(ctx, path, "master", WorktreeAddOptions{
			Branch: "test-remove-branch",
		})
		require.NoError(t, err)

		err = r.WorktreeRemove(ctx, path)
		assert.NoError(t, err)
	})

	t.Run("force remove", func(t *testing.T) {
		path := tempPath()

		err := r.WorktreeAdd(ctx, path, "master", WorktreeAddOptions{
			Branch: "test-force-remove",
		})
		require.NoError(t, err)

		// Write an untracked file to make the worktree dirty.
		err = os.WriteFile(path+"/dirty-file", []byte("dirty"), 0600)
		require.NoError(t, err)

		err = r.WorktreeRemove(ctx, path, WorktreeRemoveOptions{Force: true})
		assert.NoError(t, err)
	})
}
