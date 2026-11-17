package database

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"unknwon.dev/x/logx"
	"xorm.io/core"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

// setupLegacyEngine points the XORM engine and the database handle at one new
// SQLite database, for code that still uses both.
func setupLegacyEngine(t *testing.T) {
	if !conf.UseSQLite3 {
		t.Skip("only runs on SQLite")
	}
	oldDatabase, oldX, oldHandle := conf.Database, x, Handle
	t.Cleanup(func() {
		conf.Database, x, Handle = oldDatabase, oldX, oldHandle
	})

	conf.Database = conf.DatabaseOpts{Type: "sqlite3", Path: filepath.Join(t.TempDir(), "gogs.db")}
	var err error
	x, err = getEngine()
	require.NoError(t, err)
	t.Cleanup(func() { _ = x.Close() })
	x.SetMapper(core.GonicMapper{})
	require.NoError(t, x.Sync2(legacyTables...))

	db, err := dbx.OpenDB(conf.Database, &gorm.Config{
		SkipDefaultTransaction: true,
		NamingStrategy:         schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(Tables...))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	Handle = &DB{db: db}
}

func TestReviews(t *testing.T) {
	setupLegacyEngine(t)
	logger := logx.NewNoopLogger()

	insert := func(t *testing.T, beans ...any) {
		t.Helper()
		_, err := x.Insert(beans...)
		require.NoError(t, err)
	}
	alice := &User{Name: "alice", LowerName: "alice", Email: "alice@example.com"}
	bob := &User{Name: "bob", LowerName: "bob", Email: "bob@example.com"}
	carol := &User{Name: "carol", LowerName: "carol", Email: "carol@example.com"}
	insert(t, alice, bob, carol)
	repo := &Repository{OwnerID: alice.ID, Owner: alice, LowerName: "repo", Name: "repo"}
	insert(t, repo)
	issue := &Issue{RepoID: repo.ID, Repo: repo, Index: 1, PosterID: alice.ID, Poster: alice, Title: "Rate limit", IsPull: true}
	insert(t, issue)
	insert(t, &PullRequest{IssueID: issue.ID, Index: issue.Index, HeadRepoID: repo.ID, BaseRepoID: repo.ID, HeadBranch: "rate-limit", BaseBranch: "main"})

	numComments := func(t *testing.T) int {
		t.Helper()
		got, err := getRawIssueByID(x, issue.ID)
		require.NoError(t, err)
		return got.NumComments
	}
	visibleIDs := func(t *testing.T) []int64 {
		t.Helper()
		comments, err := GetCommentsByIssueID(issue.ID)
		require.NoError(t, err)
		ids := []int64{}
		for _, c := range comments {
			ids = append(ids, c.ID)
		}
		return ids
	}
	codeCommentIDs := func(t *testing.T, viewerID int64) []int64 {
		t.Helper()
		comments, err := GetCodeCommentsByIssueID(issue.ID, viewerID)
		require.NoError(t, err)
		ids := []int64{}
		for _, c := range comments {
			ids = append(ids, c.ID)
		}
		return ids
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
	draft, err := CreateCodeComment(logger, codeComment(bob, "The limiter map never shrinks.", true))
	require.NoError(t, err)
	assert.True(t, draft.IsPending)
	assert.Empty(t, visibleIDs(t))
	assert.Empty(t, codeCommentIDs(t, alice.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, bob.ID))
	assert.Equal(t, 0, numComments(t))
	participants, err := GetParticipantsByIssueID(x, issue.ID)
	require.NoError(t, err)
	assert.Empty(t, participants)

	// Carol's single comment is a submitted review of its own.
	single, err := CreateCodeComment(logger, codeComment(carol, "Spoofable client IP.", false))
	require.NoError(t, err)
	assert.False(t, single.IsPending)
	assert.Equal(t, []int64{single.ID}, codeCommentIDs(t, alice.ID))
	assert.Equal(t, 1, numComments(t))
	reviews, err := GetSubmittedReviewsByIssueID(issue.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, ReviewStateComment, reviews[0].State)
	assert.Equal(t, carol.ID, reviews[0].ReviewerID)

	// Bob replies to Carol's thread as part of his pending review, and a
	// reply to a reply is refused.
	reply, err := CreateReply(logger, bob, repo, issue, single, "Agreed.", nil, true)
	require.NoError(t, err)
	assert.True(t, reply.IsPending)
	assert.Equal(t, draft.ReviewID, reply.ReviewID)
	_, err = CreateReply(logger, alice, repo, issue, reply, "No nesting.", nil, false)
	assert.Error(t, err)

	require.NoError(t, RequestReview(logger, alice, repo, issue, bob))
	requests, err := GetReviewRequestsByIssueID(issue.ID)
	require.NoError(t, err)
	require.Len(t, requests, 1)

	review, err := SubmitReview(logger, bob, repo, issue, ReviewStateRequestChanges, "Fix both before this lands.", "head1")
	require.NoError(t, err)
	assert.Equal(t, draft.ReviewID, review.ID)
	assert.ElementsMatch(t, []int64{single.ID, draft.ID, reply.ID}, codeCommentIDs(t, alice.ID))
	// Carol's comment, Bob's comment and Bob's reply.
	assert.Equal(t, 3, numComments(t))
	requests, err = GetReviewRequestsByIssueID(issue.ID)
	require.NoError(t, err)
	assert.Empty(t, requests, "submitting a review answers the request")
	pending, err := GetPendingReview(issue.ID, bob.ID)
	require.NoError(t, err)
	assert.Nil(t, pending)

	// Deleting the first comment of a thread deletes its replies.
	require.NoError(t, DeleteCommentByID(logger, carol, single.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, alice.ID))
	assert.Equal(t, 1, numComments(t))

	// Discarding a pending review leaves the submitted comments alone.
	_, err = CreateCodeComment(logger, codeComment(alice, "Will fix.", true))
	require.NoError(t, err)
	require.NoError(t, DiscardPendingReview(issue.ID, alice.ID))
	assert.Equal(t, []int64{draft.ID}, codeCommentIDs(t, alice.ID))
	pending, err = GetPendingReview(issue.ID, alice.ID)
	require.NoError(t, err)
	assert.Nil(t, pending)
}
