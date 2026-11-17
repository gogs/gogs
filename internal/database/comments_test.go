package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

func TestComments(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &CommentsStore{
		db: newTestDB(t, "CommentsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *CommentsStore)
	}{
		{"Create", commentsCreate},
		{"createStatus", commentsCreateStatus},
		{"createRef", commentsCreateRef},
		{"GetByID", commentsGetByID},
		{"List", commentsList},
		{"Update", commentsUpdate},
		{"DeleteByID", commentsDeleteByID},
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

func commentsCreate(t *testing.T, ctx context.Context, s *CommentsStore) {
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "cm-create")
	mentioned := newTestIssueUser(t, ctx, s.db, "cm-create-mentioned")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{IssueComment: true})
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	other := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Other")
	attachments := newAttachmentsStore(s.db)
	unbound, err := attachments.Create(ctx, 1, "a.txt", strings.NewReader("a"))
	require.NoError(t, err)

	comment, err := s.Create(ctx, logger, repo.Owner, repo, issue, "Hi @cm-create-mentioned", []string{unbound.UUID})
	require.NoError(t, err)
	assert.Equal(t, CommentTypeComment, comment.Type)
	assert.Equal(t, repo.OwnerID, comment.Poster.ID)
	assert.Equal(t, issue.ID, comment.Issue.ID)
	assert.Equal(t, []string{unbound.UUID}, attachmentUUIDs(comment.Attachments))
	assert.Positive(t, comment.CreatedUnix)
	assert.Equal(t, comment.CreatedUnix, comment.Created.Unix())
	assert.Equal(t, comment.CreatedUnix, comment.Updated.Unix())

	got, err := attachments.GetByUUID(ctx, unbound.UUID)
	require.NoError(t, err)
	assert.Equal(t, issue.ID, got.IssueID)
	assert.Equal(t, comment.ID, got.CommentID)

	numComments := func(issueID int64) int {
		t.Helper()
		issue, err := newIssuesStore(s.db).getRaw(ctx, nil, "id = ?", issueID)
		require.NoError(t, err)
		return issue.NumComments
	}
	assert.Equal(t, 1, numComments(issue.ID))
	assert.Equal(t, 0, numComments(other.ID))

	var mentions []int64
	err = s.db.Model(new(IssueUser)).Where("repo_id = ? AND issue_id = ? AND is_mentioned = ?", repo.ID, issue.ID, true).Pluck("uid", &mentions).Error
	require.NoError(t, err)
	assert.Equal(t, []int64{mentioned.ID}, mentions)

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, 1)
	assert.Contains(t, payloads[0], `"action": "created"`)
	assert.NotContains(t, payloads[0], `"created_at": "0001-01-01`, "the comment has its creation time")
	assert.Contains(t, payloads[0], `"body": "Hi @cm-create-mentioned"`)
}

func commentsCreateStatus(t *testing.T, ctx context.Context, s *CommentsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "cm-status")
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	pull := &Issue{RepoID: repo.ID, Index: 99, PosterID: repo.OwnerID, IsPull: true}
	require.NoError(t, s.db.Create(pull).Error)

	closed := func() []int {
		t.Helper()
		got, err := newReposStore(s.db).GetByID(ctx, repo.ID)
		require.NoError(t, err)
		return []int{got.NumClosedIssues, got.NumClosedPulls}
	}

	issue.IsClosed = true
	c, err := s.createStatus(ctx, logger, repo.Owner, repo, issue)
	require.NoError(t, err)
	assert.Equal(t, CommentTypeClose, c.Type)
	assert.Equal(t, []int{1, 0}, closed())

	pull.IsClosed = true
	c, err = s.createStatus(ctx, logger, repo.Owner, repo, pull)
	require.NoError(t, err)
	assert.Equal(t, CommentTypeClose, c.Type)
	assert.Equal(t, []int{1, 1}, closed())

	issue.IsClosed = false
	c, err = s.createStatus(ctx, logger, repo.Owner, repo, issue)
	require.NoError(t, err)
	assert.Equal(t, CommentTypeReopen, c.Type)
	assert.Equal(t, []int{0, 1}, closed())

	// Comments of other types do not count as comments.
	got, err := newIssuesStore(s.db).getRaw(ctx, nil, "id = ?", issue.ID)
	require.NoError(t, err)
	assert.Zero(t, got.NumComments)
}

func commentsCreateRef(t *testing.T, ctx context.Context, s *CommentsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "cm-ref")
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	other := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Other")

	refs := func(issueID int64) []string {
		t.Helper()
		var shas []string
		err := s.db.Model(new(Comment)).Where("issue_id = ? AND type = ?", issueID, CommentTypeCommitRef).Order("id").Pluck("commit_sha", &shas).Error
		require.NoError(t, err)
		return shas
	}

	for _, sha := range []string{"abc", "abc", "def"} {
		err := s.createRef(ctx, logger, repo.Owner, repo, issue, "Fix it", sha)
		require.NoError(t, err)
	}
	err := s.createRef(ctx, logger, repo.Owner, repo, other, "Fix it", "abc")
	require.NoError(t, err)
	assert.Equal(t, []string{"abc", "def"}, refs(issue.ID))
	assert.Equal(t, []string{"abc"}, refs(other.ID))

	err = s.createRef(ctx, logger, repo.Owner, repo, issue, "Fix it", "")
	require.Error(t, err)
}

func commentsGetByID(t *testing.T, ctx context.Context, s *CommentsStore) {
	repo := newTestIssueRepo(t, ctx, s.db, "cm-get")
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	comment := &Comment{Type: CommentTypeComment, PosterID: repo.OwnerID, IssueID: issue.ID, Content: "Hi"}
	require.NoError(t, s.db.Create(comment).Error)

	got, err := s.GetByID(ctx, comment.ID)
	require.NoError(t, err)
	assert.Equal(t, "Hi", got.Content)
	assert.Equal(t, repo.OwnerID, got.Poster.ID)
	assert.Equal(t, issue.ID, got.Issue.ID)
	assert.Equal(t, repo.ID, got.Issue.Repo.ID)
	assert.NotNil(t, got.Attachments)

	_, err = s.GetByID(ctx, 9999)
	assert.True(t, IsErrCommentNotExist(err), "got %v", err)

	// Comments whose poster no longer exists get the ghost user.
	err = s.db.Model(comment).Update("poster_id", 9999).Error
	require.NoError(t, err)
	got, err = s.GetByID(ctx, comment.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(-1), got.PosterID)
	assert.Equal(t, NewGhostUser().Name, got.Poster.Name)
}

func commentsList(t *testing.T, ctx context.Context, s *CommentsStore) {
	repo := newTestIssueRepo(t, ctx, s.db, "cm-list")
	otherRepo := newTestIssueRepo(t, ctx, s.db, "cm-list-other")
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	second := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Second")
	otherIssue := newTestIssue(t, ctx, newIssuesStore(s.db), otherRepo, "Other")

	create := func(issueID, created, updated int64) *Comment {
		t.Helper()
		c := &Comment{PosterID: repo.OwnerID, IssueID: issueID, CreatedUnix: created, UpdatedUnix: updated}
		require.NoError(t, s.db.Create(c).Error)
		return c
	}
	// Comments created in the same second sort by ID.
	c3 := create(issue.ID, 300, 300)
	c1 := create(issue.ID, 100, 500)
	c2a := create(second.ID, 200, 200)
	c2b := create(issue.ID, 200, 200)
	c2c := create(issue.ID, 200, 200)
	create(otherIssue.ID, 50, 600)

	commentIDs := func(comments []*Comment) []int64 {
		ids := make([]int64, 0, len(comments))
		for _, c := range comments {
			ids = append(ids, c.ID)
		}
		return ids
	}

	got, err := s.ListByIssueID(ctx, issue.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{c1.ID, c2b.ID, c2c.ID, c3.ID}, commentIDs(got))
	assert.Equal(t, issue.ID, got[0].Issue.ID, "attributes are loaded")

	got, err = s.ListByIssueID(ctx, issue.ID, 300)
	require.NoError(t, err)
	assert.Equal(t, []int64{c1.ID, c3.ID}, commentIDs(got))

	got, err = s.ListByRepoID(ctx, repo.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{c1.ID, c2a.ID, c2b.ID, c2c.ID, c3.ID}, commentIDs(got))

	got, err = s.ListByRepoID(ctx, repo.ID, 201)
	require.NoError(t, err)
	assert.Equal(t, []int64{c1.ID, c3.ID}, commentIDs(got))

	got, err = s.ListByIssueID(ctx, 9999, 0)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func commentsUpdate(t *testing.T, ctx context.Context, s *CommentsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "cm-update")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{IssueComment: true})
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	comment, err := s.Create(ctx, logger, repo.Owner, repo, issue, "Old", nil)
	require.NoError(t, err)
	comment, err = s.GetByID(ctx, comment.ID)
	require.NoError(t, err)

	comment.Content = "New"
	err = s.Update(ctx, logger, repo.Owner, comment, "Old")
	require.NoError(t, err)

	got, err := s.GetByID(ctx, comment.ID)
	require.NoError(t, err)
	assert.Equal(t, "New", got.Content)

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, 2)
	assert.Contains(t, payloads[1], `"action": "edited"`)
	assert.Contains(t, payloads[1], `"from": "Old"`)
	assert.Contains(t, payloads[1], `"body": "New"`)
}

func commentsDeleteByID(t *testing.T, ctx context.Context, s *CommentsStore) {
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "cm-delete")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{IssueComment: true})
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Crash")
	attachments := newAttachmentsStore(s.db)
	a, err := attachments.Create(ctx, 1, "a.txt", strings.NewReader("a"))
	require.NoError(t, err)
	kept, err := attachments.Create(ctx, 1, "b.txt", strings.NewReader("b"))
	require.NoError(t, err)

	comment, err := s.Create(ctx, logger, repo.Owner, repo, issue, "Bye", []string{a.UUID})
	require.NoError(t, err)
	keptComment, err := s.Create(ctx, logger, repo.Owner, repo, issue, "Stay", []string{kept.UUID})
	require.NoError(t, err)
	issue.IsClosed = true
	status, err := s.createStatus(ctx, logger, repo.Owner, repo, issue)
	require.NoError(t, err)

	numComments := func() int {
		t.Helper()
		got, err := newIssuesStore(s.db).getRaw(ctx, nil, "id = ?", issue.ID)
		require.NoError(t, err)
		return got.NumComments
	}
	require.Equal(t, 2, numComments())

	err = s.DeleteByID(ctx, logger, repo.Owner, comment.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, comment.ID)
	assert.True(t, IsErrCommentNotExist(err), "got %v", err)
	assert.Equal(t, 1, numComments())
	_, err = attachments.GetByUUID(ctx, a.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)
	_, err = os.Stat(a.LocalPath())
	assert.True(t, os.IsNotExist(err), "got %v", err)
	_, err = attachments.GetByUUID(ctx, kept.UUID)
	require.NoError(t, err)
	_, err = os.Stat(kept.LocalPath())
	require.NoError(t, err)

	// Deleting a comment of another type does not change the number of
	// comments.
	err = s.DeleteByID(ctx, logger, repo.Owner, status.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, numComments())

	err = s.DeleteByID(ctx, logger, repo.Owner, 9999)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, keptComment.ID)
	require.NoError(t, err)

	payloads := hookPayloads(t, s.db)
	require.Len(t, payloads, 4)
	assert.Contains(t, payloads[2], `"action": "deleted"`)
	assert.Contains(t, payloads[2], `"body": "Bye"`)
}
