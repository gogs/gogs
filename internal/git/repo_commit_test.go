package git

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_escapePath(t *testing.T) {
	tests := []struct {
		path    string
		expPath string
	}{
		{
			path:    "",
			expPath: "",
		},
		{
			path:    "normal",
			expPath: "normal",
		},
		{
			path:    ":normal",
			expPath: "\\:normal",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expPath, escapePath(test.path))
		})
	}
}

func TestRepository_CatFileCommit(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid revision", func(t *testing.T) {
		c, err := testrepo.CatFileCommit(ctx, "bad_revision")
		assert.Equal(t, ErrRevisionNotExist, err)
		assert.Nil(t, c)
	})

	c, err := testrepo.CatFileCommit(ctx, "d58e3ef9f123eea6857161c79275ee22b228f659")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "d58e3ef9f123eea6857161c79275ee22b228f659", c.ID.String())
	assert.Equal(t, "Add a symlink\n", c.Message)
}

func TestRepository_BranchCommit(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid branch", func(t *testing.T) {
		c, err := testrepo.BranchCommit(ctx, "refs/heads/release-1.0")
		assert.Equal(t, ErrRevisionNotExist, err)
		assert.Nil(t, c)
	})

	c, err := testrepo.BranchCommit(ctx, "release-1.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", c.ID.String())
	assert.Equal(t, "Rename shell script\n", c.Message)
}

func TestRepository_TagCommit(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid branch", func(t *testing.T) {
		c, err := testrepo.BranchCommit(ctx, "refs/tags/v1.0.0")
		assert.Equal(t, ErrRevisionNotExist, err)
		assert.Nil(t, c)
	})

	c, err := testrepo.BranchCommit(ctx, "release-1.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", c.ID.String())
	assert.Equal(t, "Rename shell script\n", c.Message)
}

func TestRepository_Log(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		rev          string
		opt          LogOptions
		expCommitIDs []string
	}{
		{
			rev: "0eedd79eba4394bbef888c804e899731644367fe",
			opt: LogOptions{
				Since: time.Unix(1581250680, 0),
			},
			expCommitIDs: []string{
				"0eedd79eba4394bbef888c804e899731644367fe",
				"4e59b72440188e7c2578299fc28ea425fbe9aece",
			},
		},
		{
			rev: "0eedd79eba4394bbef888c804e899731644367fe",
			opt: LogOptions{
				Since: time.Now().AddDate(100, 0, 0),
			},
			expCommitIDs: []string{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			commits, err := testrepo.Log(ctx, test.rev, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestRepository_CommitByRevision(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid revision", func(t *testing.T) {
		c, err := testrepo.CommitByRevision(ctx, "bad_revision")
		assert.Equal(t, ErrRevisionNotExist, err)
		assert.Nil(t, c)
	})

	tests := []struct {
		rev   string
		opt   CommitByRevisionOptions
		expID string
	}{
		{
			rev:   "4e59b72",
			expID: "4e59b72440188e7c2578299fc28ea425fbe9aece",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			c, err := testrepo.CommitByRevision(ctx, test.rev, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expID, c.ID.String())
		})
	}
}

func TestRepository_CommitsSince(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		rev          string
		since        time.Time
		opt          CommitsSinceOptions
		expCommitIDs []string
	}{
		{
			rev:   "0eedd79eba4394bbef888c804e899731644367fe",
			since: time.Unix(1581250680, 0),
			expCommitIDs: []string{
				"0eedd79eba4394bbef888c804e899731644367fe",
				"4e59b72440188e7c2578299fc28ea425fbe9aece",
			},
		},
		{
			rev:          "0eedd79eba4394bbef888c804e899731644367fe",
			since:        time.Now().AddDate(100, 0, 0),
			expCommitIDs: []string{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			commits, err := testrepo.CommitsSince(ctx, test.rev, test.since, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestRepository_DiffNameOnly(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		base     string
		head     string
		opt      DiffNameOnlyOptions
		expFiles []string
	}{
		{
			base:     "ef7bebf8bdb1919d947afe46ab4b2fb4278039b3",
			head:     "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			expFiles: []string{"fix.txt"},
		},
		{
			base: "45a30ea9afa413e226ca8614179c011d545ca883",
			head: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			opt: DiffNameOnlyOptions{
				NeedsMergeBase: true,
			},
			expFiles: []string{"fix.txt", "pom.xml", "src/test/java/com/github/AppTest.java"},
		},

		{
			base: "45a30ea9afa413e226ca8614179c011d545ca883",
			head: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			opt: DiffNameOnlyOptions{
				Path: "src",
			},
			expFiles: []string{"src/test/java/com/github/AppTest.java"},
		},
		{
			base: "45a30ea9afa413e226ca8614179c011d545ca883",
			head: "978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
			opt: DiffNameOnlyOptions{
				Path: "resources",
			},
			expFiles: []string{},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			files, err := testrepo.DiffNameOnly(ctx, test.base, test.head, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expFiles, files)
		})
	}
}

func TestRepository_RevListCount(t *testing.T) {
	ctx := context.Background()

	t.Run("no refspecs", func(t *testing.T) {
		count, err := testrepo.RevListCount(ctx, []string{})
		assert.Equal(t, errors.New("must have at least one refspec"), err)
		assert.Zero(t, count)
	})

	tests := []struct {
		refspecs []string
		opt      RevListCountOptions
		expCount int64
	}{
		{
			refspecs: []string{"755fd577edcfd9209d0ac072eed3b022cbe4d39b"},
			expCount: 1,
		},
		{
			refspecs: []string{"f5ed01959cffa4758ca0a49bf4c34b138d7eab0a"},
			expCount: 5,
		},
		{
			refspecs: []string{"978fb7f6388b49b532fbef8b856681cfa6fcaa0a"},
			expCount: 27,
		},

		{
			refspecs: []string{"7c5ee6478d137417ae602140c615e33aed91887c"},
			opt: RevListCountOptions{
				Path: "README.txt",
			},
			expCount: 3,
		},
		{
			refspecs: []string{"7c5ee6478d137417ae602140c615e33aed91887c"},
			opt: RevListCountOptions{
				Path: "resources",
			},
			expCount: 1,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			count, err := testrepo.RevListCount(ctx, test.refspecs, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCount, count)
		})
	}
}

func TestRepository_RevList(t *testing.T) {
	ctx := context.Background()

	t.Run("no refspecs", func(t *testing.T) {
		commits, err := testrepo.RevList(ctx, []string{})
		assert.Equal(t, errors.New("must have at least one refspec"), err)
		assert.Nil(t, commits)
	})

	tests := []struct {
		refspecs     []string
		opt          RevListOptions
		expCommitIDs []string
	}{
		{
			refspecs: []string{"45a30ea9afa413e226ca8614179c011d545ca883...978fb7f6388b49b532fbef8b856681cfa6fcaa0a"},
			expCommitIDs: []string{
				"978fb7f6388b49b532fbef8b856681cfa6fcaa0a",
				"ef7bebf8bdb1919d947afe46ab4b2fb4278039b3",
				"ebbbf773431ba07510251bb03f9525c7bab2b13a",
			},
		},
		{
			refspecs: []string{"45a30ea9afa413e226ca8614179c011d545ca883...978fb7f6388b49b532fbef8b856681cfa6fcaa0a"},
			opt: RevListOptions{
				Path: "src",
			},
			expCommitIDs: []string{
				"ebbbf773431ba07510251bb03f9525c7bab2b13a",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			commits, err := testrepo.RevList(ctx, test.refspecs, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expCommitIDs, commitsToIDs(commits))
		})
	}
}

func TestRepository_LatestCommitTime(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		opt     LatestCommitTimeOptions
		expTime time.Time
	}{
		{
			opt: LatestCommitTimeOptions{
				Branch: "release-1.0",
			},
			expTime: time.Unix(1581256638, 0),
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			got, err := testrepo.LatestCommitTime(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expTime.Unix(), got.Unix())
		})
	}
}
