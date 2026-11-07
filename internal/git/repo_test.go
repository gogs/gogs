package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
