package git

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
				Heads:    true,
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
				Tags:     true,
				Patterns: []string{"v1.0.0"},
			},
			expRefs: []*Reference{
				{
					ID:      "0eedd79eba4394bbef888c804e899731644367fe",
					Refspec: "refs/tags/v1.0.0",
				},
			},
		}, {
			url: testrepo.Path(),
			opt: LsRemoteOptions{
				Refs:     true,
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

func TestIsURLAccessible(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		url    string
		expVal bool
	}{
		{
			url:    testrepo.Path(),
			expVal: true,
		}, {
			url:    os.TempDir(),
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, IsURLAccessible(ctx, test.url))
		})
	}
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
	assert.True(t, r.HasReference(ctx, RefsHeads+"release-1.0"))
}

func TestRepository_RemoteRemove(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	err = r.RemoteRemove(ctx, "origin", RemoteRemoveOptions{})
	assert.Nil(t, err)

	err = r.RemoteRemove(ctx, "origin", RemoteRemoveOptions{})
	assert.Equal(t, ErrRemoteNotExist, err)
}

func TestRepository_Remotes(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// 1 remote
	remotes, err := r.Remotes(ctx)
	assert.Nil(t, err)
	assert.Equal(t, []string{"origin"}, remotes)

	// 2 remotes
	err = r.RemoteAdd(ctx, "t", "t")
	assert.Nil(t, err)

	remotes, err = r.Remotes(ctx)
	assert.Nil(t, err)
	assert.Equal(t, []string{"origin", "t"}, remotes)
	assert.Len(t, remotes, 2)

	// 0 remotes
	err = r.RemoteRemove(ctx, "t")
	assert.Nil(t, err)
	err = r.RemoteRemove(ctx, "origin")
	assert.Nil(t, err)

	remotes, err = r.Remotes(ctx)
	assert.Nil(t, err)
	assert.Equal(t, []string{}, remotes)
	assert.Len(t, remotes, 0)
}

func TestRepository_RemoteURLFamily(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	err = r.RemoteSetURLDelete(ctx, "origin", ".*")
	assert.Equal(t, ErrNotDeleteNonPushURLs, err)

	err = r.RemoteSetURL(ctx, "notexist", "t")
	assert.Equal(t, ErrRemoteNotExist, err)

	err = r.RemoteSetURL(ctx, "notexist", "t", RemoteSetURLOptions{Regex: "t"})
	assert.Equal(t, ErrRemoteNotExist, err)

	// Default origin URL is not easily testable
	err = r.RemoteSetURL(ctx, "origin", "t")
	assert.Nil(t, err)
	urls, err := r.RemoteGetURL(ctx, "origin")
	assert.Nil(t, err)
	assert.Equal(t, []string{"t"}, urls)

	err = r.RemoteSetURLAdd(ctx, "origin", "e")
	assert.Nil(t, err)
	urls, err = r.RemoteGetURL(ctx, "origin", RemoteGetURLOptions{All: true})
	assert.Nil(t, err)
	assert.Equal(t, []string{"t", "e"}, urls)

	err = r.RemoteSetURL(ctx, "origin", "s", RemoteSetURLOptions{Regex: "e"})
	assert.Nil(t, err)
	urls, err = r.RemoteGetURL(ctx, "origin", RemoteGetURLOptions{All: true})
	assert.Nil(t, err)
	assert.Equal(t, []string{"t", "s"}, urls)

	err = r.RemoteSetURLDelete(ctx, "origin", "t")
	assert.Nil(t, err)
	urls, err = r.RemoteGetURL(ctx, "origin", RemoteGetURLOptions{All: true})
	assert.Nil(t, err)
	assert.Equal(t, []string{"s"}, urls)
}
