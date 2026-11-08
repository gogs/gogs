package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository(t *testing.T) {
	path := os.TempDir()
	r := &Repository{
		path: path,
	}

	assert.Equal(t, path, r.Path())
}

func TestInit(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		opt InitOptions
	}{
		{
			opt: InitOptions{},
		},
		{
			opt: InitOptions{
				Bare: true,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			path := tempPath()
			defer func() {
				_ = os.RemoveAll(path)
			}()

			if err := Init(ctx, path, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	_, err := Open(testrepo.Path())
	assert.Nil(t, err)

	_, err = Open(tempPath())
	assert.Equal(t, os.ErrNotExist, err)
}

func TestClone(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		opt CloneOptions
	}{
		{
			opt: CloneOptions{},
		},
		{
			opt: CloneOptions{
				Mirror: true,
				Bare:   true,
				Quiet:  true,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			path := tempPath()
			defer func() {
				_ = os.RemoveAll(path)
			}()

			if err := Clone(ctx, testrepo.Path(), path, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func setupTempRepo() (_ *Repository, cleanup func(), err error) {
	ctx := context.Background()
	path := tempPath()
	cleanup = func() {
		_ = os.RemoveAll(path)
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	if err = Clone(ctx, testrepo.Path(), path); err != nil {
		return nil, cleanup, err
	}

	r, err := Open(path)
	if err != nil {
		return nil, cleanup, err
	}
	return r, cleanup, nil
}

func TestRepository_Push(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		remote string
		branch string
		opt    PushOptions
	}{
		{
			remote: "origin",
			branch: "master",
			opt:    PushOptions{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Push(ctx, test.remote, test.branch, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepository_ApplyCheck(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	baseID := commitFiles(t, r, "master", map[string]string{"a.txt": "a b\ntwo\nc d\n"})
	headID := commitFiles(t, r, "master", map[string]string{"a.txt": "a b\nTWO\nc d\n"})

	patch, err := r.DiffBinary(ctx, baseID, headID)
	require.NoError(t, err)
	patchPath := filepath.Join(t.TempDir(), "a.patch")
	require.NoError(t, os.WriteFile(patchPath, patch, 0600))

	// The context lines of the patch now only differ in whitespace.
	require.NoError(t, r.ReadTree(ctx, baseID))
	blobID, err := r.HashObject(ctx, strings.NewReader("a  b\ntwo\nc  d\n"))
	require.NoError(t, err)
	require.NoError(t, r.AddToIndex(ctx, IndexEntry{Mode: EntryBlob, ID: blobID, Path: "a.txt"}))

	assert.Error(t, r.ApplyCheck(ctx, patchPath))
	assert.NoError(t, r.ApplyCheck(ctx, patchPath, ApplyCheckOptions{IgnoreWhitespace: true}))
}

func TestRepository_RevParse(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		rev    string
		expID  string
		expErr error
	}{
		{
			rev:    "4e59b72",
			expID:  "4e59b72440188e7c2578299fc28ea425fbe9aece",
			expErr: nil,
		},
		{
			rev:    "release-1.0",
			expID:  "0eedd79eba4394bbef888c804e899731644367fe",
			expErr: nil,
		},
		{
			rev:    "RELEASE_1.0",
			expID:  "2a52e96389d02209b451ae1ddf45d645b42d744c",
			expErr: nil,
		},
		{
			rev:    "refs/heads/release-1.0",
			expID:  "0eedd79eba4394bbef888c804e899731644367fe",
			expErr: nil,
		},
		{
			rev:    "refs/tags/RELEASE_1.0",
			expID:  "2a52e96389d02209b451ae1ddf45d645b42d744c",
			expErr: nil,
		},

		{
			rev:    "refs/tags/404",
			expID:  "",
			expErr: ErrRevisionNotExist,
		},

		// A revision starting with a dash must be rejected to prevent argument
		// injection into "git rev-parse".
		{
			rev:    "--absolute-git-dir",
			expID:  "",
			expErr: ErrRevisionNotExist,
		},
		{
			rev:    "-h",
			expID:  "",
			expErr: ErrRevisionNotExist,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			id, err := testrepo.RevParse(ctx, test.rev)
			assert.Equal(t, test.expErr, err)
			assert.Equal(t, test.expID, id)
		})
	}
}

func TestRepository_CountObjects(t *testing.T) {
	ctx := context.Background()
	// Make sure it does not blow up
	_, err := testrepo.CountObjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepository_Fsck(t *testing.T) {
	ctx := context.Background()
	// Make sure it does not blow up
	err := testrepo.Fsck(ctx, FsckOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRepository_GC(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, r.GC(ctx, GCOptions{Args: []string{"--quiet"}}))
	assert.Error(t, r.GC(ctx, GCOptions{Args: []string{"--no-such-option"}}))
}
