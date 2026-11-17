package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

func TestReviews(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &ReviewsStore{
		db: newTestDB(t, "ReviewsStore"),
	}
	// Comments build links and find mentions from the server, SSH and UI
	// settings, which other suites mock.
	conf.SetMockServer(t, conf.ServerOpts{})
	conf.SetMockSSH(t, conf.SSHOpts{})
	conf.SetMockUI(t, conf.UIOpts{})
	logger := logx.NewNoopLogger()

	repo := newTestIssueRepo(t, ctx, s.db, "rv-alice")
	alice := repo.Owner
	bob := newTestIssueUser(t, ctx, s.db, "rv-bob")
	carol := newTestIssueUser(t, ctx, s.db, "rv-carol")
	issue := newTestIssue(t, ctx, newIssuesStore(s.db), repo, "Rate limit")
	comments := newCommentsStore(s.db)

	numComments := func(t *testing.T) int {
		t.Helper()
		got, err := newIssuesStore(s.db).GetByID(ctx, issue.ID)
		require.NoError(t, err)
		return got.NumComments
	}
	ids := func(comments []*Comment) []int64 {
		ids := []int64{}
		for _, c := range comments {
			ids = append(ids, c.ID)
		}
		return ids
	}
	visibleIDs := func(t *testing.T) []int64 {
		t.Helper()
		got, err := comments.ListByIssueID(ctx, issue.ID, 0)
		require.NoError(t, err)
		return ids(got)
	}
	codeCommentIDs := func(t *testing.T, viewerID int64) []int64 {
		t.Helper()
		got, err := s.ListCodeComments(ctx, issue.ID, viewerID)
		require.NoError(t, err)
		return ids(got)
	}
	codeComment := func(doer *User, content string, pending bool) CreateCodeCommentOptions {
		return CreateCodeCommentOptions{
			Doer: doer, Repo: repo, Issue: issue, Content: content,
			TreePath: "http.go", Side: "new", StartSide: "new", StartLine: 40, Line: 41,
			CommitSHA: "head1", BaseSHA: "base1", Pending: pending,
		}
	}

	// Bob's pending comment and reply are only his until he submits his
	// review.
	draft, err := s.CreateCodeComment(ctx, logger, codeComment(bob, "The limiter map never shrinks.", true))
	require.NoError(t, err)
	assert.True(t, draft.IsPending)
	assert.Empty(t, visibleIDs(t))
	assert.Empty(t, codeCommentIDs(t, alice.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, bob.ID))
	assert.Equal(t, 0, numComments(t))
	participants, err := newIssuesStore(s.db).listParticipants(ctx, issue.ID)
	require.NoError(t, err)
	assert.Empty(t, participants)

	// Carol's single comment is a submitted review of its own.
	single, err := s.CreateCodeComment(ctx, logger, codeComment(carol, "Spoofable client IP.", false))
	require.NoError(t, err)
	assert.False(t, single.IsPending)
	assert.Equal(t, []int64{single.ID}, codeCommentIDs(t, alice.ID))
	assert.Equal(t, 1, numComments(t))
	reviews, err := s.ListSubmitted(ctx, issue.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, ReviewStateComment, reviews[0].State)
	assert.Equal(t, carol.ID, reviews[0].ReviewerID)

	// Bob replies to Carol's thread as part of his pending review, and a
	// reply to a reply is refused.
	reply, err := s.CreateReply(ctx, logger, bob, repo, issue, single, "Agreed.", nil, true)
	require.NoError(t, err)
	assert.True(t, reply.IsPending)
	assert.Equal(t, draft.ReviewID, reply.ReviewID)
	_, err = s.CreateReply(ctx, logger, alice, repo, issue, reply, "No nesting.", nil, false)
	assert.Error(t, err)

	require.NoError(t, s.SetRequested(ctx, logger, alice, repo, issue, bob, true))
	requests, err := s.ListRequests(ctx, issue.ID)
	require.NoError(t, err)
	require.Len(t, requests, 1)

	review, err := s.Submit(ctx, logger, bob, repo, issue, ReviewStateRequestChanges, "Fix both before this lands.", "head1")
	require.NoError(t, err)
	assert.Equal(t, draft.ReviewID, review.ID)
	assert.ElementsMatch(t, []int64{single.ID, draft.ID, reply.ID}, codeCommentIDs(t, alice.ID))
	// Carol's comment, Bob's comment and Bob's reply.
	assert.Equal(t, 3, numComments(t))
	requests, err = s.ListRequests(ctx, issue.ID)
	require.NoError(t, err)
	assert.Empty(t, requests, "submitting a review answers the request")
	pending, err := s.GetPending(ctx, issue.ID, bob.ID)
	require.NoError(t, err)
	assert.Nil(t, pending)

	// Resolving a thread does not count as editing its first comment.
	require.NoError(t, s.SetThreadResolved(ctx, single, alice.ID, true))
	got, err := comments.GetByID(ctx, single.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, got.ResolvedByID)
	assert.Equal(t, single.UpdatedUnix, got.UpdatedUnix)

	// Deleting the first comment of a thread deletes its replies.
	require.NoError(t, comments.DeleteByID(ctx, logger, carol, single.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, alice.ID))
	assert.Equal(t, 1, numComments(t))

	// Discarding a pending review leaves the submitted comments alone.
	_, err = s.CreateCodeComment(ctx, logger, codeComment(alice, "Will fix.", true))
	require.NoError(t, err)
	require.NoError(t, s.DiscardPending(ctx, issue.ID, alice.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, alice.ID))
	pending, err = s.GetPending(ctx, issue.ID, alice.ID)
	require.NoError(t, err)
	assert.Nil(t, pending)
}
