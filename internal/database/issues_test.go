package database

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

func TestIssues(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// NOTE: The suite does not run in parallel because it mocks global
	// configuration that other suites read without locks.

	ctx := context.Background()
	s := &IssuesStore{
		db: newTestDB(t, "IssuesStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *IssuesStore)
	}{
		{"Create", issuesCreate},
		{"Get", issuesGet},
		{"List", issuesList},
		{"Suggest", issuesSuggest},
		{"Stats", issuesStats},
		{"UserStats", issuesUserStats},
		{"MarkRead", issuesMarkRead},
		{"UpdateAssignee", issuesUpdateAssignee},
		{"ChangeStatus", issuesChangeStatus},
		{"ChangeTitleAndContent", issuesChangeTitleAndContent},
		{"Labels", issuesLabels},
		{"updateMentions", issuesUpdateMentions},
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

// newTestIssueRepo creates a user of given name and a repository it owns, and
// returns the repository with its owner.
//
// NOTE: Owners must have names that no other test uses, because tests that
// delete users remove their directories under the repository root that is
// mocked at that time.
func newTestIssueRepo(t *testing.T, ctx context.Context, db *gorm.DB, ownerName string) *Repository {
	t.Helper()

	owner := newTestIssueUser(t, ctx, db, ownerName)
	repo, err := newReposStore(db).create(ctx, owner.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo.Owner = owner
	return repo
}

func newTestIssueUser(t *testing.T, ctx context.Context, db *gorm.DB, name string) *User {
	t.Helper()

	user, err := newUsersStore(db).Create(ctx, name, name+"@example.com", CreateUserOptions{})
	require.NoError(t, err)
	return user
}

// newTestIssue creates an issue of given title in the repository as its
// owner, and returns it with its attributes.
func newTestIssue(t *testing.T, ctx context.Context, s *IssuesStore, repo *Repository, title string) *Issue {
	t.Helper()

	repo.NumIssues = int(issueCount(t, s.db, repo.ID))
	issue := &Issue{RepoID: repo.ID, Title: title, PosterID: repo.OwnerID, Poster: repo.Owner}
	err := s.Create(ctx, logx.NewNoopLogger(), repo, issue, nil, nil)
	require.NoError(t, err)
	return issue
}

func issueCount(t *testing.T, db *gorm.DB, repoID int64) int64 {
	t.Helper()

	var count int64
	err := db.Model(new(Issue)).Where("repo_id = ?", repoID).Count(&count).Error
	require.NoError(t, err)
	return count
}

func issueIDs(issues []*Issue) []int64 {
	ids := make([]int64, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

func hookPayloads(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var payloads []string
	err := db.Model(new(HookTask)).Order("id").Pluck("payload_content", &payloads).Error
	require.NoError(t, err)
	return payloads
}

func issuesCreate(t *testing.T, ctx context.Context, s *IssuesStore) {
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "is-create")
	reader := newTestIssueUser(t, ctx, s.db, "is-create-reader")
	err := s.db.Create(&Access{UserID: reader.ID, RepoID: repo.ID, Mode: AccessModeRead}).Error
	require.NoError(t, err)
	mentioned := newTestIssueUser(t, ctx, s.db, "is-create-mentioned")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Issues: true})

	milestone := newTestMilestone(t, ctx, newMilestonesStore(s.db), repo.ID, "v1", false)
	bug := newTestLabel(t, ctx, newLabelsStore(s.db), repo.ID, "bug")
	otherLabel := newTestLabel(t, ctx, newLabelsStore(s.db), 9999, "bug")
	attachments := newAttachmentsStore(s.db)
	unbound, err := attachments.Create(ctx, 1, "a.txt", strings.NewReader("a"))
	require.NoError(t, err)
	bound := newTestAttachment(t, ctx, attachments, "b.txt", 9999, 0, 0)

	issue := &Issue{
		RepoID:      repo.ID,
		Title:       "  Crash  ",
		Content:     "Ping @IS-Create-Mentioned",
		PosterID:    repo.OwnerID,
		Poster:      repo.Owner,
		MilestoneID: milestone.ID,
		AssigneeID:  reader.ID,
	}
	err = s.Create(ctx, logger, repo, issue, []int64{bug.ID, otherLabel.ID}, []string{unbound.UUID, bound.UUID})
	require.NoError(t, err)

	got, err := s.GetByID(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, got.CreatedUnix, issue.Created.Unix())
	assert.Equal(t, got.CreatedUnix, issue.Updated.Unix())
	assert.Equal(t, int64(1), got.Index)
	assert.Equal(t, "Crash", got.Title)
	assert.Equal(t, milestone.ID, got.MilestoneID)
	assert.Equal(t, reader.ID, got.AssigneeID)
	assert.Equal(t, []int64{bug.ID}, labelIDs(got.Labels))
	assert.Equal(t, []string{unbound.UUID}, attachmentUUIDs(got.Attachments))

	gotRepo, err := newReposStore(s.db).GetByID(ctx, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, gotRepo.NumIssues)
	assert.Zero(t, gotRepo.NumPulls)

	gotMilestone, err := newMilestonesStore(s.db).GetByID(ctx, repo.ID, milestone.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, gotMilestone.NumIssues)
	gotLabel, err := newLabelsStore(s.db).GetByID(ctx, repo.ID, bug.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, gotLabel.NumIssues)
	gotLabel, err = newLabelsStore(s.db).GetByID(ctx, 9999, otherLabel.ID)
	require.NoError(t, err)
	assert.Zero(t, gotLabel.NumIssues)

	var ius []*IssueUser
	err = s.db.Where("issue_id = ?", issue.ID).Order("uid").Find(&ius).Error
	require.NoError(t, err)
	type relation struct {
		UserID                            int64
		IsPoster, IsAssigned, IsMentioned bool
	}
	var relations []relation
	for _, iu := range ius {
		assert.Equal(t, issue.ID, iu.IssueID)
		relations = append(relations, relation{iu.UserID, iu.IsPoster, iu.IsAssigned, iu.IsMentioned})
	}
	assert.Equal(t, []relation{
		{repo.OwnerID, true, false, false},
		{reader.ID, false, true, false},
		{mentioned.ID, false, false, true},
	}, relations)

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, 1)
	assert.Contains(t, payloads[0], `"action": "opened"`)
	assert.NotContains(t, payloads[0], `"created_at": "0001-01-01`, "the issue has its creation time")

	// A milestone and an assignee that do not exist are dropped, and a poster
	// who cannot be assigned relates to the issue as its poster only.
	repo.NumIssues = 1
	issue = &Issue{RepoID: repo.ID, Title: "Second", PosterID: mentioned.ID, Poster: mentioned, MilestoneID: 9999, AssigneeID: 9999}
	err = s.Create(ctx, logger, repo, issue, nil, nil)
	require.NoError(t, err)
	got, err = s.GetByID(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.Index)
	assert.Zero(t, got.MilestoneID)
	assert.Zero(t, got.AssigneeID)
	assert.Nil(t, got.Milestone)
	assert.Nil(t, got.Assignee)

	var posters []int64
	err = s.db.Model(new(IssueUser)).Where("issue_id = ? AND is_poster = ?", issue.ID, true).Pluck("uid", &posters).Error
	require.NoError(t, err)
	assert.Equal(t, []int64{mentioned.ID}, posters)
}

func issuesGet(t *testing.T, ctx context.Context, s *IssuesStore) {
	repo := newTestIssueRepo(t, ctx, s.db, "is-get")
	issue := newTestIssue(t, ctx, s, repo, "First")
	newTestIssue(t, ctx, s, repo, "Second")

	got, err := s.GetByIndex(ctx, repo.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, issue.ID, got.ID)
	assert.Equal(t, repo.ID, got.Repo.ID)
	assert.Equal(t, repo.OwnerID, got.Poster.ID)
	assert.NotNil(t, got.Labels)
	assert.NotNil(t, got.Comments)

	_, err = s.GetByIndex(ctx, repo.ID, 3)
	assert.True(t, IsErrIssueNotExist(err), "got %v", err)
	_, err = s.GetByIndex(ctx, 9999, 1)
	assert.True(t, IsErrIssueNotExist(err), "got %v", err)
	_, err = s.GetByID(ctx, 9999)
	assert.True(t, IsErrIssueNotExist(err), "got %v", err)

	// Issues whose poster no longer exists get the ghost user.
	err = s.db.Model(new(Issue)).Where("id = ?", issue.ID).Update("poster_id", 9999).Error
	require.NoError(t, err)
	got, err = s.GetByID(ctx, issue.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), got.PosterID)
	assert.Equal(t, NewGhostUser().Name, got.Poster.Name)

	for _, ref := range []string{"is-get/repo1", "is-get/repo1#0", "is-get/repo1#-1", "is-get/repo1#x"} {
		_, err = s.getByRef(ctx, ref)
		assert.True(t, IsErrIssueNotExist(err), "%q: got %v", ref, err)
	}
}

func issuesList(t *testing.T, ctx context.Context, s *IssuesStore) {
	ui := conf.UI
	ui.IssuePagingNum = 2
	conf.SetMockUI(t, ui)

	repo := newTestIssueRepo(t, ctx, s.db, "is-list")
	other := newTestIssueRepo(t, ctx, s.db, "is-list-other")
	bug := newTestLabel(t, ctx, newLabelsStore(s.db), repo.ID, "bug")
	ui1 := newTestLabel(t, ctx, newLabelsStore(s.db), repo.ID, "ui")

	// All issues are created in the same second of the test clock, so issues
	// sort by ID when they sort the same.
	i1 := newTestIssue(t, ctx, s, repo, "One")
	i2 := newTestIssue(t, ctx, s, repo, "Two")
	i3 := newTestIssue(t, ctx, s, repo, "Three")
	closed := newTestIssue(t, ctx, s, repo, "Closed")
	newTestIssue(t, ctx, s, other, "Other")
	pull := &Issue{RepoID: repo.ID, Index: 99, Title: "Pull", IsPull: true}
	require.NoError(t, s.db.Create(pull).Error)
	err := s.db.Model(new(Issue)).Where("id = ?", closed.ID).Update("is_closed", true).Error
	require.NoError(t, err)
	assignee := newTestIssueUser(t, ctx, s.db, "is-list-assignee")
	milestone := newTestMilestone(t, ctx, newMilestonesStore(s.db), repo.ID, "v1", false)
	err = s.db.Model(new(Issue)).Where("id = ?", i2.ID).Updates(map[string]any{"num_comments": 5, "assignee_id": assignee.ID, "milestone_id": milestone.ID, "poster_id": other.OwnerID}).Error
	require.NoError(t, err)
	err = newLabelsStore(s.db).AddToIssue(ctx, i1, bug, ui1)
	require.NoError(t, err)
	err = newLabelsStore(s.db).AddToIssue(ctx, i3, bug)
	require.NoError(t, err)

	list := func(opts IssuesOptions) []int64 {
		t.Helper()
		got, err := s.List(ctx, &opts)
		require.NoError(t, err)
		for _, issue := range got {
			assert.NotNil(t, issue.Repo, "attributes are loaded")
		}
		return issueIDs(got)
	}
	count := func(opts IssuesOptions) int64 {
		t.Helper()
		got, err := s.Count(ctx, &opts)
		require.NoError(t, err)
		return got
	}

	assert.Equal(t, []int64{i3.ID, i2.ID}, list(IssuesOptions{RepoID: repo.ID}))
	assert.Equal(t, []int64{i1.ID}, list(IssuesOptions{RepoID: repo.ID, Page: 2}))
	assert.Equal(t, []int64{i1.ID, i2.ID}, list(IssuesOptions{RepoID: repo.ID, SortType: "oldest"}))
	assert.Equal(t, []int64{i3.ID}, list(IssuesOptions{RepoID: repo.ID, SortType: "oldest", Page: 2}))
	assert.Equal(t, []int64{i2.ID, i3.ID}, list(IssuesOptions{RepoID: repo.ID, SortType: "mostcomment"}))
	assert.Equal(t, []int64{i1.ID, i3.ID}, list(IssuesOptions{RepoID: repo.ID, SortType: "leastcomment"}))
	assert.Equal(t, int64(3), count(IssuesOptions{RepoID: repo.ID}))

	assert.Equal(t, []int64{closed.ID}, list(IssuesOptions{RepoID: repo.ID, IsClosed: true}))
	assert.Equal(t, []int64{pull.ID}, list(IssuesOptions{RepoID: repo.ID, IsPull: true}))
	assert.Equal(t, []int64{i2.ID}, list(IssuesOptions{RepoID: repo.ID, AssigneeID: assignee.ID}))
	assert.Equal(t, []int64{i2.ID}, list(IssuesOptions{RepoID: repo.ID, PosterID: other.OwnerID}))
	assert.Equal(t, []int64{i2.ID}, list(IssuesOptions{RepoID: repo.ID, MilestoneID: milestone.ID}))

	// An issue with several of the labels is listed once.
	assert.Equal(t, []int64{i3.ID, i1.ID}, list(IssuesOptions{RepoID: repo.ID, Labels: joinIDs(bug.ID, ui1.ID)}))
	assert.Equal(t, int64(2), count(IssuesOptions{RepoID: repo.ID, Labels: joinIDs(bug.ID, ui1.ID)}))
	assert.Equal(t, []int64{i1.ID}, list(IssuesOptions{RepoID: repo.ID, Labels: joinIDs(ui1.ID)}))
	assert.Equal(t, int64(3), count(IssuesOptions{RepoID: repo.ID, Labels: "0"}))

	assert.Equal(t, int64(4), count(IssuesOptions{RepoIDs: []int64{repo.ID, other.ID}}))
	assert.Equal(t, int64(1), count(IssuesOptions{RepoIDs: []int64{other.ID}}))
	assert.Equal(t, int64(4), count(IssuesOptions{}))
	assert.Empty(t, list(IssuesOptions{RepoIDs: []int64{}}))
	assert.Zero(t, count(IssuesOptions{RepoIDs: []int64{}}))

	// Mentions are listed for the user only.
	err = s.updateMentions(ctx, i1.ID, []string{"is-list"})
	require.NoError(t, err)
	assert.Equal(t, []int64{i1.ID}, list(IssuesOptions{RepoID: repo.ID, IsMention: true, UserID: repo.OwnerID}))
	assert.Empty(t, list(IssuesOptions{RepoID: repo.ID, IsMention: true, UserID: other.OwnerID}))
}

func joinIDs(ids ...int64) string {
	s := ""
	for i, id := range ids {
		if i > 0 {
			s += ","
		}
		s += strconv.FormatInt(id, 10)
	}
	return s
}

func issuesSuggest(t *testing.T, ctx context.Context, s *IssuesStore) {
	for i, issue := range []*Issue{
		{RepoID: 1, Index: 1, Title: "Login fails", UpdatedUnix: 100},
		{RepoID: 1, Index: 12, Title: "Crash on start", UpdatedUnix: 300},
		{RepoID: 1, Index: 2, Title: "Pull to fix LOGIN", IsPull: true, UpdatedUnix: 200},
		{RepoID: 1, Index: 21, Title: "Docs", UpdatedUnix: 400},
		{RepoID: 2, Index: 1, Title: "Login elsewhere", UpdatedUnix: 500},
	} {
		issue.CreatedUnix = int64(i + 1)
		require.NoError(t, s.db.Create(issue).Error)
	}

	suggest := func(opts SuggestIssuesOptions) []string {
		t.Helper()
		opts.RepoID = 1
		opts.Limit = 10
		got, err := s.Suggest(ctx, opts)
		require.NoError(t, err)
		titles := make([]string, 0, len(got))
		for _, issue := range got {
			titles = append(titles, issue.Title)
		}
		return titles
	}

	assert.Equal(t, []string{"Docs", "Crash on start", "Pull to fix LOGIN", "Login fails"}, suggest(SuggestIssuesOptions{IncludeIssues: true, IncludePulls: true}))
	assert.Equal(t, []string{"Docs", "Crash on start", "Login fails"}, suggest(SuggestIssuesOptions{IncludeIssues: true}))
	assert.Equal(t, []string{"Pull to fix LOGIN"}, suggest(SuggestIssuesOptions{IncludePulls: true}))
	assert.Empty(t, suggest(SuggestIssuesOptions{}))
	assert.Equal(t, []string{"Pull to fix LOGIN", "Login fails"}, suggest(SuggestIssuesOptions{Keyword: " login ", IncludeIssues: true, IncludePulls: true}))

	// The issue whose number is the keyword comes first, then the ones whose
	// number starts with it.
	assert.Equal(t, []string{"Login fails", "Crash on start"}, suggest(SuggestIssuesOptions{Keyword: "1", IncludeIssues: true, IncludePulls: true}))
	assert.Equal(t, []string{"Pull to fix LOGIN", "Docs"}, suggest(SuggestIssuesOptions{Keyword: "2", IncludeIssues: true, IncludePulls: true}))
}

func issuesStats(t *testing.T, ctx context.Context, s *IssuesStore) {
	bug := newTestLabel(t, ctx, newLabelsStore(s.db), 1, "bug")
	ui := newTestLabel(t, ctx, newLabelsStore(s.db), 1, "ui")
	for i, issue := range []*Issue{
		{RepoID: 1, PosterID: 10, AssigneeID: 20, MilestoneID: 30},
		{RepoID: 1, PosterID: 10, IsClosed: true},
		{RepoID: 1, PosterID: 11, AssigneeID: 20},
		{RepoID: 1, PosterID: 10, IsPull: true},
		{RepoID: 2, PosterID: 10},
	} {
		issue.Index = int64(i + 1)
		require.NoError(t, s.db.Create(issue).Error)
		if i < 2 {
			require.NoError(t, newLabelsStore(s.db).AddToIssue(ctx, issue, bug, ui))
		}
		if i == 2 {
			require.NoError(t, s.db.Create(&IssueUser{IssueID: issue.ID, UserID: 10, IsMentioned: true}).Error)
		}
	}

	stats := func(opts IssueStatsOptions) []int64 {
		t.Helper()
		opts.RepoID = 1
		got, err := s.Stats(ctx, &opts)
		require.NoError(t, err)
		return []int64{got.OpenCount, got.ClosedCount}
	}
	assert.Equal(t, []int64{2, 1}, stats(IssueStatsOptions{FilterMode: FilterModeYourRepos}))
	assert.Equal(t, []int64{1, 0}, stats(IssueStatsOptions{FilterMode: FilterModeYourRepos, IsPull: true}))
	assert.Equal(t, []int64{1, 1}, stats(IssueStatsOptions{FilterMode: FilterModeYourRepos, Labels: joinIDs(bug.ID, ui.ID)}))
	assert.Equal(t, []int64{2, 1}, stats(IssueStatsOptions{FilterMode: FilterModeYourRepos, Labels: "0"}))
	assert.Equal(t, []int64{1, 0}, stats(IssueStatsOptions{FilterMode: FilterModeYourRepos, MilestoneID: 30}))
	assert.Equal(t, []int64{2, 0}, stats(IssueStatsOptions{FilterMode: FilterModeAssign, AssigneeID: 20}))
	assert.Equal(t, []int64{1, 1}, stats(IssueStatsOptions{FilterMode: FilterModeCreate, UserID: 10}))
	assert.Equal(t, []int64{1, 0}, stats(IssueStatsOptions{FilterMode: FilterModeMention, UserID: 10}))
	assert.Equal(t, []int64{0, 0}, stats(IssueStatsOptions{FilterMode: FilterModeMention, UserID: 11}))
	assert.Equal(t, []int64{0, 0}, stats(IssueStatsOptions{FilterMode: "unknown"}))
}

func issuesUserStats(t *testing.T, ctx context.Context, s *IssuesStore) {
	for i, issue := range []*Issue{
		{RepoID: 1, PosterID: 10, AssigneeID: 10},
		{RepoID: 1, PosterID: 10, IsClosed: true},
		{RepoID: 2, PosterID: 11, AssigneeID: 10},
		{RepoID: 2, PosterID: 11, AssigneeID: 10, IsClosed: true},
		{RepoID: 3, PosterID: 10},
		{RepoID: 1, PosterID: 10, AssigneeID: 10, IsPull: true},
	} {
		issue.Index = int64(i + 1)
		require.NoError(t, s.db.Create(issue).Error)
	}

	stats := func(repoID int64, repoIDs []int64, filterMode FilterMode) *IssueStats {
		t.Helper()
		got, err := s.UserStats(ctx, repoID, 10, repoIDs, filterMode, false)
		require.NoError(t, err)
		return got
	}

	got := stats(0, []int64{1, 2}, FilterModeYourRepos)
	assert.Equal(t, &IssueStats{OpenCount: 2, ClosedCount: 2, YourReposCount: 2, AssignCount: 2, CreateCount: 2}, got)

	got = stats(2, []int64{1, 2}, FilterModeAssign)
	assert.Equal(t, &IssueStats{OpenCount: 1, ClosedCount: 1, YourReposCount: 1, AssignCount: 1, CreateCount: 0}, got)

	got = stats(0, nil, FilterModeCreate)
	assert.Equal(t, &IssueStats{OpenCount: 2, ClosedCount: 1, YourReposCount: 0, AssignCount: 2, CreateCount: 2}, got)

	got, err := s.UserStats(ctx, 1, 10, nil, FilterModeAssign, true)
	require.NoError(t, err)
	assert.Equal(t, &IssueStats{OpenCount: 1, ClosedCount: 0, YourReposCount: 1, AssignCount: 1, CreateCount: 1}, got)
}

func issuesMarkRead(t *testing.T, ctx context.Context, s *IssuesStore) {
	for _, iu := range []*IssueUser{
		{IssueID: 1, RepoID: 1, UserID: 10},
		{IssueID: 1, RepoID: 1, UserID: 11},
		{IssueID: 2, RepoID: 1, UserID: 10},
		{IssueID: 3, RepoID: 1, UserID: 10, IsClosed: true},
	} {
		require.NoError(t, s.db.Create(iu).Error)
	}

	err := s.MarkRead(ctx, 10, 1)
	require.NoError(t, err)

	got, err := s.ListIssueUsers(ctx, 1, 10, false)
	require.NoError(t, err)
	read := map[int64]bool{}
	for _, iu := range got {
		read[iu.IssueID] = iu.IsRead
	}
	assert.Equal(t, map[int64]bool{1: true, 2: false}, read)

	got, err = s.ListIssueUsers(ctx, 1, 11, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].IsRead)

	got, err = s.ListIssueUsers(ctx, 1, 10, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(3), got[0].IssueID)
}

func issuesUpdateAssignee(t *testing.T, ctx context.Context, s *IssuesStore) {
	issue := &Issue{RepoID: 1, Index: 1, Title: "Old"}
	require.NoError(t, s.db.Create(issue).Error)
	other := &Issue{RepoID: 1, Index: 2}
	require.NoError(t, s.db.Create(other).Error)
	for _, iu := range []*IssueUser{
		{IssueID: issue.ID, UserID: 10, IsAssigned: true},
		{IssueID: issue.ID, UserID: 11},
		{IssueID: other.ID, UserID: 10, IsAssigned: true},
	} {
		require.NoError(t, s.db.Create(iu).Error)
	}
	assigned := func() []int64 {
		t.Helper()
		var userIDs []int64
		err := s.db.Model(new(IssueUser)).Where("is_assigned = ?", true).Order("issue_id, uid").Pluck("uid", &userIDs).Error
		require.NoError(t, err)
		return userIDs
	}

	issue.AssigneeID = 11
	issue.Title = "New"
	err := s.UpdateAssignee(ctx, issue)
	require.NoError(t, err)
	assert.Equal(t, []int64{11, 10}, assigned())
	got, err := s.getRaw(ctx, nil, "id = ?", issue.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(11), got.AssigneeID)
	assert.Equal(t, "New", got.Title)

	issue.AssigneeID = 0
	err = s.UpdateAssignee(ctx, issue)
	require.NoError(t, err)
	assert.Equal(t, []int64{10}, assigned())

	// Updating an issue that is not inserted is a no-op.
	err = s.Update(ctx, &Issue{Title: "Nothing"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), issueCount(t, s.db, 1))
}

func issuesChangeStatus(t *testing.T, ctx context.Context, s *IssuesStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "is-status")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Issues: true})
	milestone := newTestMilestone(t, ctx, newMilestonesStore(s.db), repo.ID, "v1", false)
	bug := newTestLabel(t, ctx, newLabelsStore(s.db), repo.ID, "bug")

	repo.NumIssues = 0
	issue := &Issue{RepoID: repo.ID, Title: "Crash", PosterID: repo.OwnerID, Poster: repo.Owner, MilestoneID: milestone.ID}
	err := s.Create(ctx, logger, repo, issue, []int64{bug.ID}, nil)
	require.NoError(t, err)
	before := len(hookPayloads(t, s.db))

	state := func() []any {
		t.Helper()
		gotRepo, err := newReposStore(s.db).GetByID(ctx, repo.ID)
		require.NoError(t, err)
		gotMilestone, err := newMilestonesStore(s.db).GetByID(ctx, repo.ID, milestone.ID)
		require.NoError(t, err)
		gotLabel, err := newLabelsStore(s.db).GetByID(ctx, repo.ID, bug.ID)
		require.NoError(t, err)
		gotIssue, err := s.getRaw(ctx, nil, "id = ?", issue.ID)
		require.NoError(t, err)
		var closedUsers int64
		err = s.db.Model(new(IssueUser)).Where("issue_id = ? AND is_closed = ?", issue.ID, true).Count(&closedUsers).Error
		require.NoError(t, err)
		return []any{gotIssue.IsClosed, gotRepo.NumClosedIssues, gotMilestone.NumClosedIssues, gotLabel.NumClosedIssues, closedUsers}
	}
	commentTypes := func() []CommentType {
		t.Helper()
		var types []CommentType
		err := s.db.Model(new(Comment)).Where("issue_id = ?", issue.ID).Order("id").Pluck("type", &types).Error
		require.NoError(t, err)
		return types
	}

	issue.Labels = nil
	err = s.ChangeStatus(ctx, logger, repo.Owner, repo, issue, true)
	require.NoError(t, err)
	assert.Equal(t, []any{true, 1, 1, 1, int64(1)}, state())
	assert.Equal(t, []CommentType{CommentTypeClose}, commentTypes())
	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, before+1)
	assert.Contains(t, payloads[before], `"action": "closed"`)

	// Changing to the status the issue has is a no-op.
	err = s.ChangeStatus(ctx, logger, repo.Owner, repo, issue, true)
	require.NoError(t, err)
	assert.Equal(t, []any{true, 1, 1, 1, int64(1)}, state())
	assert.Len(t, hookPayloads(t, s.db), before+1)

	err = s.ChangeStatus(ctx, logger, repo.Owner, repo, issue, false)
	require.NoError(t, err)
	assert.Equal(t, []any{false, 0, 0, 0, int64(0)}, state())
	assert.Equal(t, []CommentType{CommentTypeClose, CommentTypeReopen}, commentTypes())
	payloads = hookPayloads(t, s.db)
	require.Len(t, payloads, before+2)
	assert.Contains(t, payloads[before+1], `"action": "reopened"`)
}

func issuesChangeTitleAndContent(t *testing.T, ctx context.Context, s *IssuesStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "is-edit")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Issues: true})
	issue := newTestIssue(t, ctx, s, repo, "Old title")
	issue.Content = "Unsaved"
	err := s.db.Model(new(Issue)).Where("id = ?", issue.ID).Update("content", "Old body").Error
	require.NoError(t, err)
	before := len(hookPayloads(t, s.db))

	err = s.ChangeTitle(ctx, logger, repo.Owner, issue, "New title")
	require.NoError(t, err)
	got, err := s.getRaw(ctx, nil, "id = ?", issue.ID)
	require.NoError(t, err)
	assert.Equal(t, "New title", got.Title)
	assert.Equal(t, "Old body", got.Content, "only the title is saved")

	issue.Title = "Unsaved"
	err = s.ChangeContent(ctx, logger, repo.Owner, issue, "New body")
	require.NoError(t, err)
	got, err = s.getRaw(ctx, nil, "id = ?", issue.ID)
	require.NoError(t, err)
	assert.Equal(t, "New title", got.Title, "only the content is saved")
	assert.Equal(t, "New body", got.Content)

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, before+2)
	assert.Contains(t, payloads[before], `"action": "edited"`)
	assert.Contains(t, payloads[before], `"from": "Old title"`)
	assert.Contains(t, payloads[before+1], `"from": "Unsaved"`)

	assignee := newTestIssueUser(t, ctx, s.db, "is-edit-assignee")
	err = s.ChangeAssignee(ctx, logger, repo.Owner, issue, assignee.ID)
	require.NoError(t, err)
	require.NotNil(t, issue.Assignee)
	assert.Equal(t, assignee.ID, issue.Assignee.ID)
	err = s.ChangeAssignee(ctx, logger, repo.Owner, issue, 0)
	require.NoError(t, err)
	assert.Nil(t, issue.Assignee)
	payloads = hookPayloads(t, s.db)
	require.Len(t, payloads, before+4)
	assert.Contains(t, payloads[before+2], `"action": "assigned"`)
	assert.Contains(t, payloads[before+3], `"action": "unassigned"`)
}

func issuesLabels(t *testing.T, ctx context.Context, s *IssuesStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "is-labels")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Issues: true})
	labels := newLabelsStore(s.db)
	bug := newTestLabel(t, ctx, labels, repo.ID, "bug")
	ui := newTestLabel(t, ctx, labels, repo.ID, "ui")
	docs := newTestLabel(t, ctx, labels, repo.ID, "docs")
	issue := newTestIssue(t, ctx, s, repo, "Crash")
	before := len(hookPayloads(t, s.db))

	current := func() []int64 {
		t.Helper()
		got, err := labels.ListByIssueID(ctx, issue.ID)
		require.NoError(t, err)
		return labelIDs(got)
	}
	numIssues := func() []int {
		t.Helper()
		got, err := labels.ListByRepoID(ctx, repo.ID)
		require.NoError(t, err)
		nums := make([]int, 0, len(got))
		for _, l := range got {
			nums = append(nums, l.NumIssues)
		}
		return nums // bug, docs, ui
	}

	err := s.AddLabels(ctx, logger, repo.Owner, issue, bug, ui)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID, ui.ID}, current())
	err = s.RemoveLabel(ctx, logger, repo.Owner, issue, bug)
	require.NoError(t, err)
	assert.Equal(t, []int64{ui.ID}, current())
	assert.Equal(t, []int{0, 0, 1}, numIssues())

	err = s.ReplaceLabels(ctx, issue, bug, docs)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID, docs.ID}, current())
	assert.Equal(t, []int{1, 1, 0}, numIssues())

	err = s.ClearLabels(ctx, logger, repo.Owner, issue)
	require.NoError(t, err)
	assert.Empty(t, current())
	assert.Equal(t, []int{0, 0, 0}, numIssues())

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, before+3)
	assert.Contains(t, payloads[before], `"action": "label_updated"`)
	assert.Contains(t, payloads[before+1], `"action": "label_updated"`)
	assert.Contains(t, payloads[before+2], `"action": "label_cleared"`)
}

func issuesUpdateMentions(t *testing.T, ctx context.Context, s *IssuesStore) {
	alice := newTestIssueUser(t, ctx, s.db, "is-mention-alice")
	bob := newTestIssueUser(t, ctx, s.db, "is-mention-bob")
	carol := newTestIssueUser(t, ctx, s.db, "is-mention-carol")
	org := &User{Name: "is-mention-org", LowerName: "is-mention-org", Type: UserTypeOrganization, NumMembers: 2}
	require.NoError(t, s.db.Create(org).Error)
	emptyOrg := &User{Name: "is-mention-empty", LowerName: "is-mention-empty", Type: UserTypeOrganization}
	require.NoError(t, s.db.Create(emptyOrg).Error)
	for _, uid := range []int64{bob.ID, carol.ID} {
		require.NoError(t, s.db.Create(&OrgUser{UID: uid, OrgID: org.ID}).Error)
	}
	// Alice already relates to the issue and keeps her other flags.
	require.NoError(t, s.db.Create(&IssueUser{IssueID: 1, UserID: alice.ID, IsPoster: true}).Error)

	assert.Equal(t, []string{"is-mention-alice", "is-mention-org"}, findLowerMentions("Hi @Is-Mention-Alice and @IS-MENTION-ORG"))

	err := s.updateMentions(ctx, 1, []string{"is-mention-alice", "is-mention-org", "is-mention-empty", "nobody"})
	require.NoError(t, err)

	var ius []*IssueUser
	err = s.db.Where("issue_id = ?", 1).Order("uid").Find(&ius).Error
	require.NoError(t, err)
	got := map[int64][2]bool{}
	for _, iu := range ius {
		got[iu.UserID] = [2]bool{iu.IsMentioned, iu.IsPoster}
	}
	assert.Equal(t, map[int64][2]bool{
		alice.ID:    {true, true},
		bob.ID:      {true, false},
		carol.ID:    {true, false},
		org.ID:      {true, false},
		emptyOrg.ID: {true, false},
	}, got)

	err = s.updateMentions(ctx, 1, nil)
	require.NoError(t, err)
	var count int64
	require.NoError(t, s.db.Model(new(IssueUser)).Count(&count).Error)
	assert.Equal(t, int64(5), count)
}
