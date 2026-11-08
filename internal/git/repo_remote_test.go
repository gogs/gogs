package git

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLsRemote(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		url     string
		opt     LsRemoteOptions
		expRefs []*Reference
	}{
		{
			url: testrepo.Path(),
			opt: LsRemoteOptions{
				Patterns: []string{"release-1.0"},
			},
			expRefs: []*Reference{
				{
					ID:      "0eedd79eba4394bbef888c804e899731644367fe",
					Refspec: "refs/heads/release-1.0",
				},
			},
		}, {
			url: testrepo.Path(),
			opt: LsRemoteOptions{
				Patterns: []string{"v1.0.0"},
			},
			expRefs: []*Reference{
				{
					ID:      "0eedd79eba4394bbef888c804e899731644367fe",
					Refspec: "refs/tags/v1.0.0",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			refs, err := LsRemote(ctx, test.url, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expRefs, refs)
		})
	}
}

func TestRemoteHEAD(t *testing.T) {
	ctx := context.Background()

	t.Run("default", func(t *testing.T) {
		ref, err := RemoteHEAD(ctx, testrepo.Path())
		require.NoError(t, err)
		assert.Equal(t, RefsHeads+"master", ref)
	})

	t.Run("HEAD points to another branch", func(t *testing.T) {
		r, cleanup, err := setupTempRepo()
		require.NoError(t, err)
		defer cleanup()

		// The clone also has "refs/remotes/origin/HEAD", which must not be mistaken
		// for HEAD.
		err = r.Checkout(ctx, "develop", CheckoutOptions{BaseBranch: "master"})
		require.NoError(t, err)

		ref, err := RemoteHEAD(ctx, r.Path())
		require.NoError(t, err)
		assert.Equal(t, RefsHeads+"develop", ref)
	})

	t.Run("HEAD points to a missing branch", func(t *testing.T) {
		r, cleanup, err := setupTempRepo()
		require.NoError(t, err)
		defer cleanup()

		_, err = r.SymbolicRef(ctx, SymbolicRefOptions{Ref: RefsHeads + "missing"})
		require.NoError(t, err)

		_, err = RemoteHEAD(ctx, r.Path())
		assert.Equal(t, ErrNotSymbolicRef, err)
	})

	t.Run("not a repository", func(t *testing.T) {
		_, err := RemoteHEAD(ctx, t.TempDir())
		assert.Error(t, err)
	})
}

func TestRepository_RemoteAdd(t *testing.T) {
	ctx := context.Background()
	path := tempPath()
	defer func() {
		_ = os.RemoveAll(path)
	}()

	err := Init(ctx, path, InitOptions{
		Bare: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// Add testrepo as the mirror remote and fetch right away
	err = r.RemoteAdd(ctx, "origin", testrepo.Path(), RemoteAddOptions{
		Fetch:       true,
		MirrorFetch: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check a non-default branch: release-1.0
	assert.True(t, r.hasReference(ctx, RefsHeads+"release-1.0"))
}

func TestRepository_RemoteRemove(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	err = r.RemoteRemove(ctx, "origin")
	assert.Nil(t, err)

	err = r.RemoteRemove(ctx, "origin")
	assert.Equal(t, ErrRemoteNotExist, err)
}

func TestRepository_RemoteUpdate(t *testing.T) {
	ctx := context.Background()
	upstream := tempPath()
	defer func() { _ = os.RemoveAll(upstream) }()
	require.NoError(t, Clone(ctx, testrepo.Path(), upstream, CloneOptions{Bare: true}))

	clone := func() *Repository {
		path := tempPath()
		t.Cleanup(func() { _ = os.RemoveAll(path) })
		require.NoError(t, Clone(ctx, upstream, path))
		r, err := Open(path)
		require.NoError(t, err)
		return r
	}
	r := clone()
	other := clone()

	require.NoError(t, other.Push(ctx, "origin", "master:refs/heads/new-branch"))
	output, err := r.RemoteUpdate(ctx)
	require.NoError(t, err)
	assert.Contains(t, output, "new-branch")

	require.NoError(t, other.Push(ctx, "origin", ":refs/heads/new-branch"))
	output, err = r.RemoteUpdate(ctx)
	require.NoError(t, err)
	assert.NotContains(t, output, "[deleted]")

	output, err = r.RemoteUpdate(ctx, RemoteUpdateOptions{Prune: true})
	require.NoError(t, err)
	assert.Contains(t, output, "[deleted]")
}
