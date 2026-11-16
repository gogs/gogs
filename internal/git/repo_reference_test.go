package git

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefShortName(t *testing.T) {
	tests := []struct {
		ref    string
		expVal string
	}{
		{
			ref:    "refs/heads/master",
			expVal: "master",
		},
		{
			ref:    "refs/tags/v1.0.0",
			expVal: "v1.0.0",
		},
		{
			ref:    "refs/pull/98",
			expVal: "refs/pull/98",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, RefShortName(test.ref))
		})
	}
}

func TestRepository_ShowRefVerify(t *testing.T) {
	ctx := context.Background()

	t.Run("reference does not exist", func(t *testing.T) {
		rev, err := testrepo.showRefVerify(ctx, "bad_reference")
		assert.NotNil(t, err)
		assert.Empty(t, rev)
	})

	rev, err := testrepo.showRefVerify(ctx, "refs/heads/release-1.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", rev)
}

func TestRepository_BranchCommitID(t *testing.T) {
	ctx := context.Background()

	t.Run("branch does not exist", func(t *testing.T) {
		rev, err := testrepo.BranchCommitID(ctx, "bad_branch")
		assert.NotNil(t, err)
		assert.Empty(t, rev)
	})

	rev, err := testrepo.BranchCommitID(ctx, "release-1.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", rev)
}

func TestRepository_TagCommitID(t *testing.T) {
	ctx := context.Background()

	t.Run("tag does not exist", func(t *testing.T) {
		rev, err := testrepo.TagCommitID(ctx, "bad_tag")
		assert.NotNil(t, err)
		assert.Empty(t, rev)
	})

	rev, err := testrepo.TagCommitID(ctx, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", rev)
}

func TestRepository_HasReference(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		ref    string
		expVal bool
	}{
		{
			ref:    RefsHeads + "master",
			expVal: true,
		},
		{
			ref:    RefsTags + "v1.0.0",
			expVal: true,
		},
		{
			ref:    "master",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, testrepo.hasReference(ctx, test.ref))
		})
	}
}

func TestRepository_HasBranch(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		ref    string
		expVal bool
	}{
		{
			ref:    "master",
			expVal: true,
		},
		{
			ref:    RefsHeads + "master",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, testrepo.HasBranch(ctx, test.ref))
		})
	}
}

func TestRepository_HasBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("has branches", func(t *testing.T) {
		got, err := testrepo.HasBranches(ctx)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("empty", func(t *testing.T) {
		path := t.TempDir()
		require.NoError(t, Init(ctx, path, InitOptions{Bare: true}))

		got, err := RepoHasBranches(ctx, path)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("HEAD points to a missing branch", func(t *testing.T) {
		r, cleanup, err := setupTempRepo()
		require.NoError(t, err)
		defer cleanup()

		_, err = r.SymbolicRef(ctx, SymbolicRefOptions{Ref: RefsHeads + "missing"})
		require.NoError(t, err)

		got, err := r.HasBranches(ctx)
		require.NoError(t, err)
		assert.True(t, got)
	})
}

func TestRepository_HasTag(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		ref    string
		expVal bool
	}{
		{
			ref:    "v1.0.0",
			expVal: true,
		},
		{
			ref:    RefsTags + "v1.0.0",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, testrepo.HasTag(ctx, test.ref))
		})
	}
}

func TestRepository_SymbolicRef(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Get HEAD
	ref, err := r.SymbolicRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, RefsHeads+"master", ref)

	// Set a symbolic reference
	_, err = r.SymbolicRef(ctx, SymbolicRefOptions{
		Name: "TEST_REF",
		Ref:  RefsHeads + "develop",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get the symbolic reference we just set
	ref, err = r.SymbolicRef(ctx, SymbolicRefOptions{
		Name: "TEST_REF",
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, RefsHeads+"develop", ref)
}

func TestRepository_ShowRef(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		opt     showRefOptions
		expRefs []*Reference
	}{
		{
			opt: showRefOptions{
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
			opt: showRefOptions{
				Tags:     true,
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
			refs, err := testrepo.showRef(ctx, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expRefs, refs)
		})
	}
}

func TestRepository_Branches(t *testing.T) {
	ctx := context.Background()
	expBranches := map[string]bool{
		"master":      true,
		"develop":     true,
		"release-1.0": true,
	}
	branches, err := testrepo.Branches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		delete(expBranches, b)
	}

	if len(expBranches) > 0 {
		t.Fatalf("expect to be empty but got %v", expBranches)
	}
}

func TestRepository_BranchesByCommitterDate(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)

	got, err := r.BranchesByCommitterDate(ctx)
	require.NoError(t, err)
	assert.Empty(t, got)

	commitAs := func(branch, name, email, date string) string {
		t.Helper()
		envs := []string{
			"GIT_AUTHOR_NAME=Author",
			"GIT_AUTHOR_EMAIL=author@example.com",
			"GIT_AUTHOR_DATE=" + date,
			"GIT_COMMITTER_NAME=" + name,
			"GIT_COMMITTER_EMAIL=" + email,
			"GIT_COMMITTER_DATE=" + date,
		}
		stdout, err := exec(ctx, r.Path(), []string{"commit-tree", "-m", branch, "0eedd79eba4394bbef888c804e899731644367fe^{tree}"}, envs)
		require.NoError(t, err)
		commitID := strings.TrimSpace(string(stdout))
		_, err = exec(ctx, r.Path(), []string{"update-ref", "--end-of-options", RefsHeads + branch, commitID}, nil)
		require.NoError(t, err)
		return commitID
	}
	oldID := commitAs("old", "Old Committer", "old@example.com", "2001-02-03T04:05:06Z")
	nestedID := commitAs("feature/search", "Alice Doe", "alice@example.com", "2030-01-02T03:04:05+08:00")
	middleID := commitAs("main", "Bob", "bob@example.com", "2020-06-07T08:09:10Z")

	got, err = r.BranchesByCommitterDate(ctx)
	require.NoError(t, err)
	want := []*Branch{
		{
			Name:     "feature/search",
			CommitID: nestedID,
			Committer: &Signature{
				Name:  "Alice Doe",
				Email: "alice@example.com",
				When:  time.Date(2030, 1, 2, 3, 4, 5, 0, time.FixedZone("", 8*60*60)),
			},
		},
		{
			Name:      "main",
			CommitID:  middleID,
			Committer: &Signature{Name: "Bob", Email: "bob@example.com", When: time.Date(2020, 6, 7, 8, 9, 10, 0, time.UTC)},
		},
		{
			Name:      "old",
			CommitID:  oldID,
			Committer: &Signature{Name: "Old Committer", Email: "old@example.com", When: time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)},
		},
	}
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i].Name, got[i].Name)
		assert.Equal(t, want[i].CommitID, got[i].CommitID)
		assert.Equal(t, want[i].Committer.Name, got[i].Committer.Name)
		assert.Equal(t, want[i].Committer.Email, got[i].Committer.Email)
		assert.True(t, want[i].Committer.When.Equal(got[i].Committer.When), "%s: want %v, got %v", want[i].Name, want[i].Committer.When, got[i].Committer.When)
	}
}

func TestRepository_DeleteBranch(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		opt DeleteBranchOptions
	}{
		{
			opt: DeleteBranchOptions{
				Force: false,
			},
		},
		{
			opt: DeleteBranchOptions{
				Force: true,
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			branch := strconv.Itoa(int(time.Now().UnixNano()))
			createBranch(t, r, branch)
			assert.True(t, r.hasReference(ctx, RefsHeads+branch))

			err := r.DeleteBranch(ctx, branch, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.False(t, r.hasReference(ctx, RefsHeads+branch))
		})
	}
}
