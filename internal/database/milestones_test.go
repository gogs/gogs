package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

func TestMilestones(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &MilestonesStore{
		db: newTestDB(t, "MilestonesStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *MilestonesStore)
	}{
		{"Create", milestonesCreate},
		{"GetByID", milestonesGetByID},
		{"List", milestonesList},
		{"Count", milestonesCount},
		{"CountIssues", milestonesCountIssues},
		{"Update", milestonesUpdate},
		{"ChangeStatus", milestonesChangeStatus},
		{"DeleteByID", milestonesDeleteByID},
		{"updateIssueStats", milestonesUpdateIssueStats},
		{"AssignIssue", milestonesAssignIssue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

// newTestMilestoneRepo creates a user of given name and a repository it owns,
// and returns the repository with its owner.
//
// NOTE: Owners must have names that no other test uses, because tests that
// delete users remove their directories under the repository root that is
// mocked at that time.
func newTestMilestoneRepo(t *testing.T, ctx context.Context, s *MilestonesStore, ownerName string) *Repository {
	t.Helper()

	owner, err := newUsersStore(s.db).Create(ctx, ownerName, ownerName+"@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := newReposStore(s.db).Create(ctx, owner.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo.Owner = owner
	return repo
}

func newTestMilestone(t *testing.T, ctx context.Context, s *MilestonesStore, repoID int64, name string, isClosed bool) *Milestone {
	t.Helper()

	m := &Milestone{RepoID: repoID, Name: name, IsClosed: isClosed}
	err := s.Create(ctx, m)
	require.NoError(t, err)
	require.NotZero(t, m.ID)
	return m
}

func newTestMilestoneIssue(t *testing.T, s *MilestonesStore, repoID, index, milestoneID int64, isClosed, isPull bool) *Issue {
	t.Helper()

	issue := &Issue{RepoID: repoID, Index: index, MilestoneID: milestoneID, IsClosed: isClosed, IsPull: isPull}
	err := s.db.Create(issue).Error
	require.NoError(t, err)
	return issue
}

func milestoneIDs(milestones []*Milestone) []int64 {
	ids := make([]int64, 0, len(milestones))
	for _, m := range milestones {
		ids = append(ids, m.ID)
	}
	return ids
}

func milestonesCreate(t *testing.T, ctx context.Context, s *MilestonesStore) {
	repo := newTestMilestoneRepo(t, ctx, s, "ms-create")
	deadline := time.Unix(1_900_000_000, 0)
	m := &Milestone{RepoID: repo.ID, Name: "v1", Content: "First", Deadline: deadline}
	err := s.Create(ctx, m)
	require.NoError(t, err)
	newTestMilestone(t, ctx, s, repo.ID, "v2", false)

	got, err := s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "v1", got.Name)
	assert.Equal(t, "First", got.Content)
	assert.Equal(t, deadline.Unix(), got.Deadline.Unix())

	gotRepo, err := newReposStore(s.db).GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, gotRepo.NumMilestones)
}

func milestonesGetByID(t *testing.T, ctx context.Context, s *MilestonesStore) {
	m := newTestMilestone(t, ctx, s, 1, "v1", false)

	got, err := s.GetByID(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)

	// An ID of 0 matches no milestone, and milestones of other repositories
	// are not found.
	_, err = s.GetByID(ctx, 1, 0)
	assert.True(t, IsErrMilestoneNotExist(err), "got %v", err)
	_, err = s.GetByID(ctx, 2, m.ID)
	assert.True(t, IsErrMilestoneNotExist(err), "got %v", err)
	_, err = s.GetByID(ctx, 0, m.ID)
	assert.True(t, IsErrMilestoneNotExist(err), "got %v", err)
}

func milestonesList(t *testing.T, ctx context.Context, s *MilestonesStore) {
	ui := conf.UI
	ui.IssuePagingNum = 2
	conf.SetMockUI(t, ui)

	open1 := newTestMilestone(t, ctx, s, 1, "open1", false)
	closed1 := newTestMilestone(t, ctx, s, 1, "closed1", true)
	open2 := newTestMilestone(t, ctx, s, 1, "open2", false)
	open3 := newTestMilestone(t, ctx, s, 1, "open3", false)
	newTestMilestone(t, ctx, s, 2, "other", false)

	got, err := s.ListByRepoID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{open1.ID, closed1.ID, open2.ID, open3.ID}, milestoneIDs(got))

	got, err = s.List(ctx, 1, false, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{open1.ID, open2.ID, open3.ID}, milestoneIDs(got))
	got, err = s.List(ctx, 1, false, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{open1.ID, open2.ID}, milestoneIDs(got))
	got, err = s.List(ctx, 1, false, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{open3.ID}, milestoneIDs(got))
	got, err = s.List(ctx, 1, true, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{closed1.ID}, milestoneIDs(got))
}

func milestonesCount(t *testing.T, ctx context.Context, s *MilestonesStore) {
	newTestMilestone(t, ctx, s, 1, "open1", false)
	newTestMilestone(t, ctx, s, 1, "open2", false)
	newTestMilestone(t, ctx, s, 1, "closed1", true)
	newTestMilestone(t, ctx, s, 2, "other", true)

	open, closed, err := s.Count(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 1}, []int64{open, closed})

	open, closed, err = s.Count(ctx, 404)
	require.NoError(t, err)
	assert.Equal(t, []int64{0, 0}, []int64{open, closed})
}

func milestonesCountIssues(t *testing.T, ctx context.Context, s *MilestonesStore) {
	m := newTestMilestone(t, ctx, s, 1, "v1", false)
	newTestMilestoneIssue(t, s, 1, 1, m.ID, false, false)
	newTestMilestoneIssue(t, s, 1, 2, m.ID, false, false)
	newTestMilestoneIssue(t, s, 1, 3, m.ID, true, false)
	newTestMilestoneIssue(t, s, 1, 4, m.ID, false, true)
	newTestMilestoneIssue(t, s, 1, 5, 9999, false, false)

	for _, tc := range []struct {
		isClosed, includePulls bool
		want                   int64
	}{
		{false, false, 2},
		{false, true, 3},
		{true, false, 1},
		{true, true, 1},
	} {
		got, err := s.CountIssues(ctx, m.ID, tc.isClosed, tc.includePulls)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "isClosed: %v, includePulls: %v", tc.isClosed, tc.includePulls)
	}
}

func milestonesUpdate(t *testing.T, ctx context.Context, s *MilestonesStore) {
	m := newTestMilestone(t, ctx, s, 1, "v1", false)
	other := newTestMilestone(t, ctx, s, 1, "v2", false)

	got, err := s.GetByID(ctx, 1, m.ID)
	require.NoError(t, err)
	got.Name = "v1.0"
	got.Content = ""
	got.NumIssues = 4
	got.NumClosedIssues = 1
	err = s.Update(ctx, got)
	require.NoError(t, err)

	got, err = s.GetByID(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "v1.0", got.Name)
	assert.Equal(t, 25, got.Completeness)
	got, err = s.GetByID(ctx, 1, other.ID)
	require.NoError(t, err)
	assert.Equal(t, "v2", got.Name)
}

func milestonesChangeStatus(t *testing.T, ctx context.Context, s *MilestonesStore) {
	repo := newTestMilestoneRepo(t, ctx, s, "ms-status")
	m := newTestMilestone(t, ctx, s, repo.ID, "v1", false)
	newTestMilestone(t, ctx, s, repo.ID, "v2", true)
	// The numbers are counted again, and other columns of the repository are
	// left alone.
	err := s.db.Exec("UPDATE repository SET num_milestones = 9, num_closed_milestones = 9, description = ? WHERE id = ?", "Kept", repo.ID).Error
	require.NoError(t, err)

	got, err := s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	err = s.ChangeStatus(ctx, got, true)
	require.NoError(t, err)

	got, err = s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	assert.True(t, got.IsClosed)
	assert.Equal(t, s.db.NowFunc().Unix(), got.ClosedDateUnix)
	gotRepo, err := newReposStore(s.db).GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 2}, []int{gotRepo.NumMilestones, gotRepo.NumClosedMilestones})
	assert.Equal(t, "Kept", gotRepo.Description)

	// Closing a closed milestone keeps its closed date.
	closedDate := time.Unix(1_000_000_000, 0)
	got.ClosedDate = closedDate
	err = s.Update(ctx, got)
	require.NoError(t, err)
	err = s.ChangeStatus(ctx, got, true)
	require.NoError(t, err)
	got, err = s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	assert.Equal(t, closedDate.Unix(), got.ClosedDateUnix)

	err = s.ChangeStatus(ctx, got, false)
	require.NoError(t, err)
	got, err = s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	assert.False(t, got.IsClosed)
	gotRepo, err = newReposStore(s.db).GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 1}, []int{gotRepo.NumMilestones, gotRepo.NumClosedMilestones})
}

func milestonesDeleteByID(t *testing.T, ctx context.Context, s *MilestonesStore) {
	repo := newTestMilestoneRepo(t, ctx, s, "ms-delete")
	m := newTestMilestone(t, ctx, s, repo.ID, "v1", true)
	other := newTestMilestone(t, ctx, s, repo.ID, "v2", false)
	issue := newTestMilestoneIssue(t, s, repo.ID, 1, m.ID, false, false)
	otherIssue := newTestMilestoneIssue(t, s, repo.ID, 2, other.ID, false, false)
	for _, iu := range []*IssueUser{
		{UserID: 1, IssueID: issue.ID, RepoID: repo.ID, MilestoneID: m.ID},
		{UserID: 1, IssueID: otherIssue.ID, RepoID: repo.ID, MilestoneID: other.ID},
	} {
		err := s.db.Create(iu).Error
		require.NoError(t, err)
	}

	issueMilestoneIDs := func() []int64 {
		var issueIDs, userIDs []int64
		err := s.db.Model(new(Issue)).Order("id").Pluck("milestone_id", &issueIDs).Error
		require.NoError(t, err)
		err = s.db.Model(new(IssueUser)).Order("id").Pluck("milestone_id", &userIDs).Error
		require.NoError(t, err)
		return append(issueIDs, userIDs...)
	}

	// A milestone of another repository is left alone.
	err := s.DeleteByID(ctx, 9999, m.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{m.ID, other.ID, m.ID, other.ID}, issueMilestoneIDs())

	err = s.DeleteByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, repo.ID, m.ID)
	assert.True(t, IsErrMilestoneNotExist(err), "got %v", err)
	assert.Equal(t, []int64{0, other.ID, 0, other.ID}, issueMilestoneIDs())
	gotRepo, err := newReposStore(s.db).GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 0}, []int{gotRepo.NumMilestones, gotRepo.NumClosedMilestones})

	err = s.DeleteByID(ctx, repo.ID, m.ID)
	require.NoError(t, err)
}

func milestonesUpdateIssueStats(t *testing.T, ctx context.Context, s *MilestonesStore) {
	m := newTestMilestone(t, ctx, s, 1, "v1", false)
	other := newTestMilestone(t, ctx, s, 1, "v2", false)
	for _, ms := range []*Milestone{m, other} {
		err := s.db.Model(ms).Update("num_issues", 2).Error
		require.NoError(t, err)
	}

	issue := &Issue{RepoID: 1, MilestoneID: m.ID, IsClosed: true}
	err := s.updateIssueStats(ctx, issue)
	require.NoError(t, err)
	got, err := s.GetByID(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 1, 50}, []int{got.NumIssues, got.NumClosedIssues, got.Completeness})

	issue.IsClosed = false
	err = s.updateIssueStats(ctx, issue)
	require.NoError(t, err)
	got, err = s.GetByID(ctx, 1, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{2, 0, 0}, []int{got.NumIssues, got.NumClosedIssues, got.Completeness})

	got, err = s.GetByID(ctx, 1, other.ID)
	require.NoError(t, err)
	assert.Zero(t, got.NumClosedIssues)

	err = s.updateIssueStats(ctx, &Issue{RepoID: 1, IsClosed: true})
	require.NoError(t, err)
}

func milestonesAssignIssue(t *testing.T, ctx context.Context, s *MilestonesStore) {
	logger := logx.NewNoopLogger()
	repo := newTestMilestoneRepo(t, ctx, s, "ms-assign")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Issues: true})

	m1 := newTestMilestone(t, ctx, s, repo.ID, "v1", false)
	m2 := newTestMilestone(t, ctx, s, repo.ID, "v2", false)
	err := s.db.Model(m1).Updates(map[string]any{"num_issues": 1, "num_closed_issues": 1}).Error
	require.NoError(t, err)
	created := newTestMilestoneIssue(t, s, repo.ID, 1, m1.ID, true, false)
	err = s.db.Create(&IssueUser{UserID: repo.OwnerID, IssueID: created.ID, RepoID: repo.ID, MilestoneID: m1.ID}).Error
	require.NoError(t, err)

	issue := new(Issue)
	err = s.db.First(issue, created.ID).Error
	require.NoError(t, err)
	issue.Repo = repo
	issue.Poster = repo.Owner

	counts := func(id int64) []int {
		m, err := s.GetByID(ctx, repo.ID, id)
		require.NoError(t, err)
		return []int{m.NumIssues, m.NumClosedIssues}
	}
	assigned := func() []int64 {
		var issueIDs, userIDs []int64
		err := s.db.Model(new(Issue)).Pluck("milestone_id", &issueIDs).Error
		require.NoError(t, err)
		err = s.db.Model(new(IssueUser)).Pluck("milestone_id", &userIDs).Error
		require.NoError(t, err)
		return append(issueIDs, userIDs...)
	}
	payloads := func() []string {
		var payloads []string
		err := s.db.Model(new(HookTask)).Order("id").Pluck("payload_content", &payloads).Error
		require.NoError(t, err)
		return payloads
	}

	issue.MilestoneID = m2.ID
	err = s.AssignIssue(ctx, logger, repo.Owner, issue, m1.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 0}, counts(m1.ID))
	assert.Equal(t, []int{1, 1}, counts(m2.ID))
	assert.Equal(t, []int64{m2.ID, m2.ID}, assigned())
	require.NotNil(t, issue.Milestone)
	assert.Equal(t, m2.ID, issue.Milestone.ID)
	got := payloads()
	require.Len(t, got, 1)
	assert.Contains(t, got[0], `"action": "milestoned"`)

	issue.MilestoneID = 0
	err = s.AssignIssue(ctx, logger, repo.Owner, issue, m2.ID)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 0}, counts(m2.ID))
	assert.Equal(t, []int64{0, 0}, assigned())
	assert.Nil(t, issue.Milestone)
	got = payloads()
	require.Len(t, got, 2)
	assert.Contains(t, got[1], `"action": "demilestoned"`)

	// A milestone of another repository cannot be assigned.
	issue.MilestoneID = 9999
	err = s.AssignIssue(ctx, logger, repo.Owner, issue, 0)
	assert.True(t, IsErrMilestoneNotExist(err), "got %v", err)
	assert.Equal(t, []int64{0, 0}, assigned())
}
