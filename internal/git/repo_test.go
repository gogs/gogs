package git

import (
	"context"
	"os"
	"path/filepath"
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
		{
			opt: CloneOptions{
				Branch: "develop",
			},
		},
		{
			opt: CloneOptions{
				Depth: 1,
			},
		},
		{
			opt: CloneOptions{
				Branch: "develop",
				Depth:  1,
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

func TestRepository_Fetch(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		opt FetchOptions
	}{
		{
			opt: FetchOptions{},
		},
		{
			opt: FetchOptions{
				Prune: true,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Fetch(ctx, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepository_Pull(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		opt PullOptions
	}{
		{
			opt: PullOptions{},
		},
		{
			opt: PullOptions{
				Rebase: true,
			},
		},
		{
			opt: PullOptions{
				All: true,
			},
		},
		{
			opt: PullOptions{
				Remote: "origin",
				Branch: "master",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Pull(ctx, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
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
		{
			remote: "origin",
			branch: "master",
			opt:    PushOptions{SetUpstream: true},
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

func TestRepository_Checkout(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		branch string
		opt    CheckoutOptions
	}{
		{
			branch: "develop",
			opt:    CheckoutOptions{},
		},
		{
			branch: "a-new-branch",
			opt: CheckoutOptions{
				BaseBranch: "master",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Checkout(ctx, test.branch, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepository_Reset(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		rev string
		opt ResetOptions
	}{
		{
			rev: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			opt: ResetOptions{
				Hard: true,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Reset(ctx, test.rev, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepository_Move(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		src string
		dst string
		opt MoveOptions
	}{
		{
			src: "run.sh",
			dst: "runme.sh",
			opt: MoveOptions{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			// Make sure it does not blow up
			if err := r.Move(ctx, test.src, test.dst, test.opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRepository_Add(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Generate a file
	fpath := filepath.Join(r.Path(), "TESTFILE")
	err = os.WriteFile(fpath, []byte("something"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Make sure it does not blow up
	if err := r.Add(ctx, AddOptions{
		All:       true,
		Pathspecs: []string{"TESTFILE"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRepository_Commit(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	committer := &Signature{
		Name:  "alice",
		Email: "alice@example.com",
	}
	author := &Signature{
		Name:  "bob",
		Email: "bob@example.com",
	}
	message := "Add a file"

	t.Run("nothing to commit", func(t *testing.T) {
		if err = r.Commit(ctx, committer, message, CommitOptions{
			Author: author,
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("nothing to commit with a file named MERGE_HEAD", func(t *testing.T) {
		fpath := filepath.Join(r.Path(), "MERGE_HEAD")
		require.NoError(t, os.WriteFile(fpath, []byte("MERGE_HEAD"), 0600))
		require.NoError(t, r.Add(ctx, AddOptions{All: true}))
		require.NoError(t, r.Commit(ctx, committer, "Add MERGE_HEAD"))

		assert.NoError(t, r.Commit(ctx, committer, message))
	})

	t.Run("committer is also the author", func(t *testing.T) {
		// Generate a file and add to index
		fpath := filepath.Join(r.Path(), "COMMITTER_IS_AUTHOR")
		err = os.WriteFile(fpath, []byte("something"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		if err := r.Add(ctx, AddOptions{
			All: true,
		}); err != nil {
			t.Fatal(err)
		}

		// Make sure it does not blow up
		if err = r.Commit(ctx, committer, message); err != nil {
			t.Fatal(err)
		}

		// Verify the result
		c, err := r.CatFileCommit(ctx, "master")
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, committer.Name, c.Committer.Name)
		assert.Equal(t, committer.Email, c.Committer.Email)
		assert.Equal(t, committer.Name, c.Author.Name)
		assert.Equal(t, committer.Email, c.Author.Email)
		assert.Equal(t, message+"\n", c.Message)
	})

	t.Run("committer is not the author", func(t *testing.T) {
		// Generate a file and add to index
		fpath := filepath.Join(r.Path(), "COMMITTER_IS_NOT_AUTHOR")
		err = os.WriteFile(fpath, []byte("something"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		if err := r.Add(ctx, AddOptions{
			All: true,
		}); err != nil {
			t.Fatal(err)
		}

		// Make sure it does not blow up
		if err = r.Commit(ctx, committer, message, CommitOptions{Author: author}); err != nil {
			t.Fatal(err)
		}

		// Verify the result
		c, err := r.CatFileCommit(ctx, "master")
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, committer.Name, c.Committer.Name)
		assert.Equal(t, committer.Email, c.Committer.Email)
		assert.Equal(t, author.Name, c.Author.Name)
		assert.Equal(t, author.Email, c.Author.Email)
		assert.Equal(t, message+"\n", c.Message)
	})

	stageFile := func(t *testing.T, name string) {
		require.NoError(t, os.WriteFile(filepath.Join(r.Path(), name), []byte(name), 0o600))
		require.NoError(t, r.Add(ctx, AddOptions{All: true}))
	}

	t.Run("empty message", func(t *testing.T) {
		stageFile(t, "EMPTY_MESSAGE")
		err := r.Commit(ctx, committer, "")
		assert.ErrorContains(t, err, "empty commit message")
	})

	t.Run("rejected by hook", func(t *testing.T) {
		stageFile(t, "REJECTED_BY_HOOK")
		hookPath := filepath.Join(r.Path(), ".git", "hooks", "pre-commit")
		require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\necho rejected >&2\nexit 1\n"), 0o700))
		defer func() { _ = os.Remove(hookPath) }()

		err := r.Commit(ctx, committer, message)
		assert.ErrorContains(t, err, "rejected")
	})

	t.Run("initial commit", func(t *testing.T) {
		path := tempPath()
		defer func() { _ = os.RemoveAll(path) }()
		require.NoError(t, Init(ctx, path))
		empty, err := Open(path)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(path, "README"), []byte("README"), 0o600))
		require.NoError(t, empty.Add(ctx, AddOptions{All: true}))
		require.NoError(t, empty.Commit(ctx, committer, message))

		_, err = empty.RevParse(ctx, "HEAD")
		assert.NoError(t, err)
	})

	t.Run("committer from Git configuration", func(t *testing.T) {
		fpath := filepath.Join(r.Path(), "COMMITTER_FROM_CONFIG")
		require.NoError(t, os.WriteFile(fpath, []byte("something"), 0600))
		require.NoError(t, r.Add(ctx, AddOptions{All: true}))

		err := r.Commit(ctx, nil, message, CommitOptions{
			Author: author,
			CommandOptions: CommandOptions{
				Envs: []string{
					"GIT_CONFIG_COUNT=2",
					"GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=carol",
					"GIT_CONFIG_KEY_1=user.email", "GIT_CONFIG_VALUE_1=carol@example.com",
				},
			},
		})
		require.NoError(t, err)

		c, err := r.CatFileCommit(ctx, "master")
		require.NoError(t, err)
		assert.Equal(t, "carol", c.Committer.Name)
		assert.Equal(t, "carol@example.com", c.Committer.Email)
		assert.Equal(t, author.Name, c.Author.Name)
		assert.Equal(t, author.Email, c.Author.Email)
	})
}

// commitFile writes the file with given content and commits it to the current
// branch of the repository.
func commitFile(t *testing.T, r *Repository, name, content string) {
	t.Helper()

	ctx := context.Background()
	require.NoError(t, os.WriteFile(filepath.Join(r.Path(), name), []byte(content), 0600))
	require.NoError(t, r.Add(ctx, AddOptions{All: true}))
	sig := &Signature{Name: "alice", Email: "alice@example.com"}
	require.NoError(t, r.Commit(ctx, sig, "Update "+name))
}

func TestRepository_Merge(t *testing.T) {
	// Merging needs a committer, which Gogs sets in the global Git configuration at
	// startup but test machines may not have.
	t.Setenv("GIT_COMMITTER_NAME", "alice")
	t.Setenv("GIT_COMMITTER_EMAIL", "alice@example.com")

	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, r.Checkout(ctx, "feature", CheckoutOptions{BaseBranch: "master"}))
	commitFile(t, r, "FEATURE", "feature")
	featureID, err := r.BranchCommitID(ctx, "feature")
	require.NoError(t, err)
	require.NoError(t, r.Checkout(ctx, "master"))
	masterID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)

	t.Run("no fast-forward without commit", func(t *testing.T) {
		err := r.Merge(ctx, "feature", MergeOptions{NoFastForward: true, NoCommit: true})
		require.NoError(t, err)

		gotID, err := r.BranchCommitID(ctx, "master")
		require.NoError(t, err)
		assert.Equal(t, masterID, gotID)
		assert.FileExists(t, filepath.Join(r.Path(), ".git", "MERGE_HEAD"))

		require.NoError(t, r.Reset(ctx, masterID, ResetOptions{Hard: true}))
	})

	t.Run("commit merge with unchanged tree", func(t *testing.T) {
		// Apply the same change to master in a different commit, so merging feature
		// leaves the tree as is.
		sig := &Signature{Name: "alice", Email: "alice@example.com"}
		require.NoError(t, os.WriteFile(filepath.Join(r.Path(), "FEATURE"), []byte("feature"), 0600))
		require.NoError(t, r.Add(ctx, AddOptions{All: true}))
		require.NoError(t, r.Commit(ctx, sig, "Cherry-pick FEATURE"))
		defer func() { require.NoError(t, r.Reset(ctx, masterID, ResetOptions{Hard: true})) }()

		require.NoError(t, r.Merge(ctx, "feature", MergeOptions{NoFastForward: true, NoCommit: true}))
		require.NoError(t, r.Commit(ctx, sig, "Merge feature"))

		c, err := r.CatFileCommit(ctx, "master")
		require.NoError(t, err)
		require.Equal(t, 2, c.ParentsCount())
		parentID, err := c.ParentID(1)
		require.NoError(t, err)
		assert.Equal(t, featureID, parentID.String())
	})

	t.Run("fast-forward", func(t *testing.T) {
		require.NoError(t, r.Merge(ctx, "feature"))

		gotID, err := r.BranchCommitID(ctx, "master")
		require.NoError(t, err)
		assert.Equal(t, featureID, gotID)
	})
}

func TestRepository_Rebase(t *testing.T) {
	// Rebasing needs a committer, which Gogs sets in the global Git configuration
	// at startup but test machines may not have.
	t.Setenv("GIT_COMMITTER_NAME", "alice")
	t.Setenv("GIT_COMMITTER_EMAIL", "alice@example.com")

	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, r.Checkout(ctx, "feature", CheckoutOptions{BaseBranch: "master"}))
	commitFile(t, r, "FEATURE", "feature")
	require.NoError(t, r.Checkout(ctx, "master"))
	commitFile(t, r, "MASTER", "master")
	masterID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)

	require.NoError(t, r.Rebase(ctx, "master", "feature"))

	head, err := r.CatFileCommit(ctx, "HEAD")
	require.NoError(t, err)
	assert.Equal(t, "Update FEATURE\n", head.Message)
	parentID, err := head.ParentID(0)
	require.NoError(t, err)
	assert.Equal(t, masterID, parentID.String())
}

func TestRepository_ApplyCheck(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	require.NoError(t, err)
	defer cleanup()

	commitFile(t, r, "a.txt", "a b\ntwo\nc d\n")
	baseID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)
	commitFile(t, r, "a.txt", "a b\nTWO\nc d\n")
	headID, err := r.BranchCommitID(ctx, "master")
	require.NoError(t, err)

	patch, err := r.DiffBinary(ctx, baseID, headID)
	require.NoError(t, err)
	patchPath := filepath.Join(t.TempDir(), "a.patch")
	require.NoError(t, os.WriteFile(patchPath, patch, 0600))

	// The context lines of the patch now only differ in whitespace.
	require.NoError(t, r.Reset(ctx, baseID, ResetOptions{Hard: true}))
	require.NoError(t, os.WriteFile(filepath.Join(r.Path(), "a.txt"), []byte("a  b\ntwo\nc  d\n"), 0600))

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
	_, err := testrepo.CountObjects(ctx, CountObjectsOptions{})
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
