package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

func TestPullRequestsStore(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &PullRequestsStore{
		db: newTestDB(t, "PullRequestsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *PullRequestsStore)
	}{
		{"Get", pullRequestsGet},
		{"Unmerged", pullRequestsUnmerged},
		{"Update", pullRequestsUpdate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			// Issue webhook payloads and API formats build links from the
			// server and SSH settings, which other suites mock.
			conf.SetMockServer(t, conf.ServerOpts{})
			conf.SetMockSSH(t, conf.SSHOpts{})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

// newTestPullRequest creates a pull request of given branches and its issue
// with given index.
func newTestPullRequest(t *testing.T, s *PullRequestsStore, index, headRepoID int64, headBranch string, baseRepoID int64, baseBranch string, isClosed, hasMerged bool) *PullRequest {
	t.Helper()

	issue := &Issue{RepoID: baseRepoID, Index: index, IsPull: true, IsClosed: isClosed}
	require.NoError(t, s.db.Create(issue).Error)
	pr := &PullRequest{
		IssueID:    issue.ID,
		Index:      index,
		HeadRepoID: headRepoID,
		HeadBranch: headBranch,
		BaseRepoID: baseRepoID,
		BaseBranch: baseBranch,
		HasMerged:  hasMerged,
	}
	require.NoError(t, s.db.Create(pr).Error)
	return pr
}

func pullRequestIDs(prs []*PullRequest) []int64 {
	ids := make([]int64, 0, len(prs))
	for _, pr := range prs {
		ids = append(ids, pr.ID)
	}
	return ids
}

func pullRequestsGet(t *testing.T, ctx context.Context, s *PullRequestsStore) {
	base := newTestIssueRepo(t, ctx, s.db, "pr-get")
	head := newTestIssueRepo(t, ctx, s.db, "pr-get-head")
	// The issue of the pull request has an ID that no pull request has.
	require.NoError(t, s.db.Create(&Issue{RepoID: base.ID, Index: 99}).Error)
	pr := newTestPullRequest(t, s, 1, head.ID, "feature", base.ID, "main", false, false)

	got, err := s.GetByID(ctx, pr.ID)
	require.NoError(t, err)
	assert.Equal(t, head.ID, got.HeadRepo.ID)
	assert.Equal(t, base.ID, got.BaseRepo.ID)
	assert.Nil(t, got.Merger)

	got, err = s.GetByIssueID(ctx, pr.IssueID)
	require.NoError(t, err)
	assert.Equal(t, pr.ID, got.ID)

	_, err = s.GetByID(ctx, 9999)
	assert.True(t, IsErrPullRequestNotExist(err), "got %v", err)
	_, err = s.GetByIssueID(ctx, 9999)
	assert.True(t, IsErrPullRequestNotExist(err), "got %v", err)

	// A head repository that no longer exists is left unset, and a merger that
	// no longer exists is the ghost user.
	err = s.db.Model(pr).Updates(map[string]any{"head_repo_id": 9999, "has_merged": true, "merger_id": 9999}).Error
	require.NoError(t, err)
	got, err = s.GetByID(ctx, pr.ID)
	require.NoError(t, err)
	assert.Nil(t, got.HeadRepo)
	assert.Equal(t, base.ID, got.BaseRepo.ID)
	assert.Equal(t, int64(-1), got.MergerID)
	assert.Equal(t, NewGhostUser().Name, got.Merger.Name)

	err = s.db.Model(pr).Update("merger_id", base.OwnerID).Error
	require.NoError(t, err)
	got, err = s.GetByID(ctx, pr.ID)
	require.NoError(t, err)
	assert.Equal(t, base.OwnerID, got.Merger.ID)

	// A base repository that no longer exists is an error.
	err = s.db.Model(pr).Update("base_repo_id", 9999).Error
	require.NoError(t, err)
	_, err = s.GetByID(ctx, pr.ID)
	assert.Error(t, err)
}

func pullRequestsUnmerged(t *testing.T, ctx context.Context, s *PullRequestsStore) {
	open := newTestPullRequest(t, s, 1, 2, "feature", 1, "main", false, false)
	newTestPullRequest(t, s, 2, 2, "feature", 1, "main", true, false)
	newTestPullRequest(t, s, 3, 2, "feature", 1, "main", false, true)
	sameHead := newTestPullRequest(t, s, 4, 2, "feature", 1, "release", false, false)
	sameBase := newTestPullRequest(t, s, 5, 3, "feature", 1, "main", false, false)
	otherHead := newTestPullRequest(t, s, 6, 2, "other", 1, "main", false, false)
	newTestPullRequest(t, s, 1, 3, "x", 9, "main", false, false)

	got, err := s.GetUnmerged(ctx, 2, 1, "feature", "main")
	require.NoError(t, err)
	assert.Equal(t, open.ID, got.ID)
	assert.Nil(t, got.BaseRepo, "attributes are not loaded")

	_, err = s.GetUnmerged(ctx, 2, 1, "feature", "develop")
	assert.True(t, IsErrPullRequestNotExist(err), "got %v", err)
	_, err = s.GetUnmerged(ctx, 1, 2, "main", "feature")
	assert.True(t, IsErrPullRequestNotExist(err), "got %v", err)

	prs, err := s.listUnmergedByHead(ctx, 2, "feature")
	require.NoError(t, err)
	assert.Equal(t, []int64{open.ID, sameHead.ID}, pullRequestIDs(prs))

	prs, err = s.listUnmergedByBase(ctx, 1, "main")
	require.NoError(t, err)
	assert.Equal(t, []int64{open.ID, sameBase.ID, otherHead.ID}, pullRequestIDs(prs))

	prs, err = s.listUnmergedByHead(ctx, 1, "main")
	require.NoError(t, err)
	assert.Empty(t, prs)
}

func pullRequestsUpdate(t *testing.T, ctx context.Context, s *PullRequestsStore) {
	pr := newTestPullRequest(t, s, 1, 2, "feature", 1, "main", false, false)
	other := newTestPullRequest(t, s, 2, 2, "other", 1, "main", false, false)

	get := func(id int64) *PullRequest {
		t.Helper()
		got := new(PullRequest)
		require.NoError(t, s.db.First(got, id).Error)
		return got
	}

	pr.Status = PullRequestStatusMergeable
	pr.HeadBranch = "unsaved"
	err := s.updateStatus(ctx, pr)
	require.NoError(t, err)
	got := get(pr.ID)
	assert.Equal(t, PullRequestStatusMergeable, got.Status)
	assert.Equal(t, "feature", got.HeadBranch, "only the status is saved")
	assert.Equal(t, PullRequestStatusConflict, get(other.ID).Status)

	pr.MergeBase = "abc"
	err = s.Update(ctx, pr)
	require.NoError(t, err)
	got = get(pr.ID)
	assert.Equal(t, "abc", got.MergeBase)
	assert.Equal(t, "unsaved", got.HeadBranch)
	assert.Equal(t, "other", get(other.ID).HeadBranch)
}
