package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/git"
)

func TestPullRequestParents(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &PullRequestParentsStore{
		db: newTestDB(t, "PullRequestParentsStore"),
	}

	// 4 depends on 2 and 3, which both depend on 1.
	require.NoError(t, s.Set(ctx, 2, []int64{1}))
	require.NoError(t, s.Set(ctx, 3, []int64{1}))
	require.NoError(t, s.Set(ctx, 4, []int64{3, 2, 3}))

	parents, err := s.ListParentIDs(ctx, 4)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 3}, parents)
	children, err := s.ListChildIDs(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 3}, children)

	for _, test := range []struct {
		name    string
		pullID  int64
		parents []int64
	}{
		{name: "itself", pullID: 4, parents: []int64{4}},
		{name: "direct child", pullID: 3, parents: []int64{4}},
		{name: "indirect child", pullID: 1, parents: []int64{5, 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before, err := s.ListParentIDs(ctx, test.pullID)
			require.NoError(t, err)

			err = s.Set(ctx, test.pullID, test.parents)
			assert.ErrorIs(t, err, ErrPullRequestParentCycle)

			after, err := s.ListParentIDs(ctx, test.pullID)
			require.NoError(t, err)
			assert.Equal(t, before, after, "parents should be untouched")
		})
	}

	t.Run("replace with none", func(t *testing.T) {
		require.NoError(t, s.Set(ctx, 4, nil))
		parents, err := s.ListParentIDs(ctx, 4)
		require.NoError(t, err)
		assert.Empty(t, parents)
	})
}

func TestPullRequest_updateSyntheticBase(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=alice", "-c", "user.email=alice@example.com"}, args...)...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return strings.TrimSpace(string(output))
	}
	commit := func(branch, file, content string) {
		t.Helper()
		run("checkout", "--quiet", branch)
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
		run("add", file)
		run("commit", "--quiet", "--message="+file)
	}

	run("init", "--quiet", "--initial-branch=main")
	run("commit", "--quiet", "--allow-empty", "--message=initial")
	run("branch", "b")
	run("branch", "c")
	run("branch", "e")
	commit("b", "b.txt", "b\n")
	commit("c", "c.txt", "c\n")
	commit("e", "c.txt", "e\n")
	// "d" depends on both "b" and "c", and adds its own change on top.
	run("checkout", "--quiet", "-b", "d", "b")
	run("merge", "--quiet", "--no-edit", "c")
	commit("d", "d.txt", "d\n")

	gitRepo, err := git.Open(dir)
	require.NoError(t, err)
	pr := &PullRequest{Index: 4, HeadBranch: "d"}
	parentB := &PullRequest{Index: 2, HeadBranch: "b"}
	parentC := &PullRequest{Index: 3, HeadBranch: "c"}

	t.Run("one parent", func(t *testing.T) {
		base, err := pr.updateSyntheticBase(ctx, gitRepo, []*PullRequest{parentB})
		require.NoError(t, err)
		assert.Equal(t, run("rev-parse", "b"), base, "the base should be the head of the only parent")
	})

	t.Run("two parents", func(t *testing.T) {
		base, err := pr.updateSyntheticBase(ctx, gitRepo, []*PullRequest{parentB, parentC})
		require.NoError(t, err)

		assert.Equal(t, base, run("rev-parse", "refs/pull/4/base"), "the base should be pinned")
		assert.Equal(t, run("rev-parse", "b"), run("rev-parse", base+"^1"))
		assert.Equal(t, run("rev-parse", "c"), run("rev-parse", base+"^2"))
		assert.Equal(t, "d.txt", run("diff", "--name-only", base, "d"), "only the own change should be reviewed")

		again, err := pr.updateSyntheticBase(ctx, gitRepo, []*PullRequest{parentB, parentC})
		require.NoError(t, err)
		assert.Equal(t, base, again, "the same parents should give the same base")
	})

	t.Run("conflicting parents", func(t *testing.T) {
		before := run("rev-parse", "refs/pull/4/base")

		parentE := &PullRequest{Index: 5, HeadBranch: "e"}
		_, err := pr.updateSyntheticBase(ctx, gitRepo, []*PullRequest{parentC, parentE})
		assert.ErrorIs(t, err, ErrPullRequestParentsConflict)
		assert.Contains(t, err.Error(), "#5")

		assert.Equal(t, before, run("rev-parse", "refs/pull/4/base"), "the pinned base should be untouched")
	})
}

func TestSortPullRequestStack(t *testing.T) {
	// #9 has no parents, #2 and #7 depend on #9, #1 depends on #2 and #7, and
	// #5 is unrelated. Sorting by number alone would put #1 first.
	pulls := []*PullRequest{
		{ID: 1, Index: 1}, {ID: 2, Index: 2}, {ID: 5, Index: 5}, {ID: 7, Index: 7}, {ID: 9, Index: 9},
	}
	parents := map[int64][]int64{
		1: {2, 7},
		2: {9},
		7: {9},
		// A parent outside of the given pull requests is ignored.
		5: {404},
	}
	var got []int64
	for _, p := range sortPullRequestStack(pulls, parents) {
		got = append(got, p.Index)
	}
	assert.Equal(t, []int64{5, 9, 2, 7, 1}, got)
}

func TestPullRequestStack_relatives(t *testing.T) {
	merged := &PullRequest{ID: 1, Index: 1, HasMerged: true}
	b := &PullRequest{ID: 2, Index: 2}
	c := &PullRequest{ID: 3, Index: 3}
	d := &PullRequest{ID: 4, Index: 4}
	e := &PullRequest{ID: 5, Index: 5}
	s := &PullRequestStack{
		Pulls: []*PullRequest{merged, b, c, d, e},
		// b depends on the merged #1, d on b and c, and e on d.
		Parents: map[int64][]int64{2: {1}, 4: {2, 3}, 5: {4}},
	}

	var ancestors []int64
	for _, p := range s.UnmergedAncestors(e.ID) {
		ancestors = append(ancestors, p.Index)
	}
	assert.Equal(t, []int64{2, 3, 4}, ancestors, "merged ancestors should be left out, parents first")
	assert.Empty(t, s.UnmergedAncestors(c.ID))

	assert.Equal(t, map[int64]bool{4: true, 5: true}, s.Descendants(b.ID))
	assert.Empty(t, s.Descendants(e.ID))
}

// stackTestRepo creates a repository where "b" and "c" branch off "main", "d"
// merges both and adds d.txt, and "e" changes c.txt differently from "c".
func stackTestRepo(t *testing.T) (*git.Repository, func(args ...string) string, func(branch, file, content string)) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=alice", "-c", "user.email=alice@example.com"}, args...)...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return strings.TrimSpace(string(output))
	}
	commit := func(branch, file, content string) {
		t.Helper()
		run("checkout", "--quiet", branch)
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644))
		run("add", file)
		run("commit", "--quiet", "--message="+file)
	}

	run("init", "--quiet", "--initial-branch=main")
	run("commit", "--quiet", "--allow-empty", "--message=initial")
	run("branch", "b")
	run("branch", "c")
	run("branch", "e")
	commit("b", "b.txt", "b\n")
	commit("c", "c.txt", "c\n")
	commit("e", "c.txt", "e\n")
	run("checkout", "--quiet", "-b", "d", "b")
	run("merge", "--quiet", "--no-edit", "c")
	commit("d", "d.txt", "d\n")

	gitRepo, err := git.Open(dir)
	require.NoError(t, err)
	return gitRepo, run, commit
}

func TestPullRequest_updateSyntheticBase_mergedParent(t *testing.T) {
	ctx := context.Background()
	gitRepo, run, _ := stackTestRepo(t)

	// #2 was merged at the head of "b", whose branch was deleted afterwards.
	parentB := &PullRequest{Index: 2, HeadBranch: "b", HasMerged: true, MergedCommitID: run("rev-parse", "b")}
	run("branch", "--delete", "--force", "b")
	parentC := &PullRequest{Index: 3, HeadBranch: "c"}
	pr := &PullRequest{Index: 4, HeadBranch: "d"}

	base, err := pr.updateSyntheticBase(ctx, gitRepo, []*PullRequest{parentB, parentC})
	require.NoError(t, err)
	assert.Equal(t, parentB.MergedCommitID, run("rev-parse", base+"^1"))
	assert.Equal(t, "d.txt", run("diff", "--name-only", base, "d"), "only the own change should be reviewed")
}

func TestPullRequestStack_Behind(t *testing.T) {
	ctx := context.Background()
	gitRepo, run, commit := stackTestRepo(t)
	parentB := &PullRequest{ID: 2, Index: 2, HeadBranch: "b", Issue: &Issue{}}
	parentC := &PullRequest{ID: 3, Index: 3, HeadBranch: "c", Issue: &Issue{}}
	pr := &PullRequest{ID: 4, Index: 4, HeadBranch: "d", Issue: &Issue{}}
	stack := &PullRequestStack{
		Pulls:   []*PullRequest{parentB, parentC, pr},
		Parents: map[int64][]int64{4: {2, 3}},
	}
	behindIndexes := func() []int64 {
		t.Helper()
		behind, err := stack.Behind(ctx, gitRepo, "main")
		require.NoError(t, err)
		var indexes []int64
		for _, p := range behind[pr.ID] {
			indexes = append(indexes, p.Index)
		}
		return indexes
	}

	assert.Empty(t, behindIndexes())

	commit("b", "b.txt", "b v2\n")
	assert.Equal(t, []int64{2}, behindIndexes())

	t.Run("parent head in the base branch", func(t *testing.T) {
		// The new head of "b" is now in "main" but still not in "d".
		run("branch", "--force", "main", "b")
		assert.Equal(t, []int64{2}, behindIndexes())
	})

	t.Run("merged parent", func(t *testing.T) {
		// A merged parent is in the base branch already, whatever its branch.
		parentB.HasMerged = true
		assert.Empty(t, behindIndexes())
	})
}

func TestMergeParentsInto(t *testing.T) {
	ctx := context.Background()
	gitRepo, run, commit := stackTestRepo(t)
	sig := &git.Signature{Name: "alice", Email: "alice@example.com"}
	parentB := &PullRequest{Index: 2, HeadBranch: "b"}
	parentC := &PullRequest{Index: 3, HeadBranch: "c"}
	pr := &PullRequest{Index: 4, HeadBranch: "d"}
	parents := []*PullRequest{parentB, parentC}

	headID := run("rev-parse", "d")
	got, err := mergeParentsInto(ctx, gitRepo, headID, "d", parents, sig)
	require.NoError(t, err)
	assert.Equal(t, headID, got, "a branch that has every parent should be left alone")

	commit("b", "b.txt", "b v2\n")
	got, err = mergeParentsInto(ctx, gitRepo, headID, "d", parents, sig)
	require.NoError(t, err)
	assert.Equal(t, headID, run("rev-parse", got+"^1"), "the update should build on the branch")
	assert.Equal(t, run("rev-parse", "b"), run("rev-parse", got+"^2"))
	assert.Equal(t, "Merge #2 (b) into d", run("log", "-1", "--format=%s", got))
	assert.Equal(t, "b v2", run("show", got+":b.txt"))
	assert.Equal(t, "d", run("show", got+":d.txt"))

	run("update-ref", "refs/heads/d", got)
	base, err := pr.updateSyntheticBase(ctx, gitRepo, parents)
	require.NoError(t, err)
	assert.Equal(t, "d.txt", run("diff", "--name-only", base, "d"), "only the own change should be reviewed after the update")

	t.Run("conflict", func(t *testing.T) {
		parentE := &PullRequest{Index: 5, HeadBranch: "e"}
		_, err := mergeParentsInto(ctx, gitRepo, run("rev-parse", "d"), "d", []*PullRequest{parentE}, sig)
		assert.ErrorIs(t, err, ErrPullRequestUpdateConflict)
		assert.Contains(t, err.Error(), "#5")
	})
}

func TestCheckStackMerge(t *testing.T) {
	ctx := context.Background()
	gitRepo, run, _ := stackTestRepo(t)
	b := &PullRequest{Index: 2, HeadBranch: "b"}
	c := &PullRequest{Index: 3, HeadBranch: "c"}
	d := &PullRequest{Index: 4, HeadBranch: "d"}
	e := &PullRequest{Index: 5, HeadBranch: "e"}
	heads := func(pulls ...*PullRequest) []string {
		ids := make([]string, len(pulls))
		for i, p := range pulls {
			ids[i] = run("rev-parse", p.HeadBranch)
		}
		return ids
	}
	mainID := run("rev-parse", "main")
	refsBefore := run("for-each-ref")

	assert.NoError(t, checkStackMerge(ctx, gitRepo, mainID, []*PullRequest{b, c, d}, heads(b, c, d)))

	// "e" merges cleanly into main alone, but not after "c".
	assert.NoError(t, checkStackMerge(ctx, gitRepo, mainID, []*PullRequest{e}, heads(e)))
	err := checkStackMerge(ctx, gitRepo, mainID, []*PullRequest{b, c, e}, heads(b, c, e))
	assert.ErrorIs(t, err, ErrPullRequestStackNotMergeable)
	assert.Contains(t, err.Error(), "#5")

	assert.Equal(t, refsBefore, run("for-each-ref"), "checking should not move any branch")
}

func TestSetPullRequestParents_baseMismatch(t *testing.T) {
	pr := &PullRequest{ID: 3, Index: 3, BaseBranch: "main"}
	err := SetPullRequestParents(context.Background(), pr, []*PullRequest{
		{ID: 1, Index: 1, BaseBranch: "main"},
		{ID: 2, Index: 2, BaseBranch: "release"},
	})
	assert.ErrorIs(t, err, ErrPullRequestParentBaseMismatch)
	assert.ErrorContains(t, err, "#2 targets release")
}
