package git

import (
	"context"
	"strconv"
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
		require.NoError(t, Init(ctx, path, InitBare()))

		got, err := RepoHasBranches(ctx, path)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("HEAD points to a missing branch", func(t *testing.T) {
		r, cleanup, err := setupTempRepo()
		require.NoError(t, err)
		defer cleanup()

		_, err = r.SymbolicRef(ctx, SymbolicRefRef(RefsHeads+"missing"))
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
	_, err = r.SymbolicRef(ctx, SymbolicRefName("TEST_REF"), SymbolicRefRef(RefsHeads+"develop"))
	if err != nil {
		t.Fatal(err)
	}

	// Get the symbolic reference we just set
	ref, err = r.SymbolicRef(ctx, SymbolicRefName("TEST_REF"))
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

func TestRepository_DeleteBranch(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	tests := []struct {
		opts []DeleteBranchOption
	}{
		{
			opts: nil,
		},
		{
			opts: []DeleteBranchOption{DeleteBranchForce()},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			branch := strconv.Itoa(int(time.Now().UnixNano()))
			createBranch(t, r, branch)
			assert.True(t, r.hasReference(ctx, RefsHeads+branch))

			err := r.DeleteBranch(ctx, branch, test.opts...)
			if err != nil {
				t.Fatal(err)
			}

			assert.False(t, r.hasReference(ctx, RefsHeads+branch))
		})
	}
}
