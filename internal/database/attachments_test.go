package database

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osx"
)

func TestAttachments(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &AttachmentsStore{
		db: newTestDB(t, "AttachmentsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *AttachmentsStore)
	}{
		{"Create", attachmentsCreate},
		{"GetByUUID", attachmentsGetByUUID},
		{"Bind", attachmentsBind},
		{"bindToRelease", attachmentsBindToRelease},
		{"List", attachmentsList},
		{"Delete", attachmentsDelete},
		{"DeleteByCommentID", attachmentsDeleteByCommentID},
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

var mockAttachmentPath sync.Mutex

// setMockAttachmentPath stores attachments in a new temporary directory until
// the test ends.
func setMockAttachmentPath(t *testing.T) {
	t.Helper()

	mockAttachmentPath.Lock()
	before := conf.Attachment.Path
	conf.Attachment.Path = t.TempDir()
	t.Cleanup(func() {
		conf.Attachment.Path = before
		mockAttachmentPath.Unlock()
	})
}

// newTestAttachment creates an attachment of given name and content, and
// attaches it to the given issue, comment and release.
func newTestAttachment(t *testing.T, ctx context.Context, s *AttachmentsStore, name string, issueID, commentID, releaseID int64) *Attachment {
	t.Helper()

	a, err := s.Create(ctx, 1, name, strings.NewReader(name))
	require.NoError(t, err)
	a.IssueID, a.CommentID, a.ReleaseID = issueID, commentID, releaseID
	err = s.db.Model(a).Select("issue_id", "comment_id", "release_id").Updates(a).Error
	require.NoError(t, err)
	return a
}

func attachmentUUIDs(attachments []*Attachment) []string {
	uuids := make([]string, 0, len(attachments))
	for _, a := range attachments {
		uuids = append(uuids, a.UUID)
	}
	return uuids
}

func attachmentsCreate(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)

	a, err := s.Create(ctx, 7, "notes.txt", strings.NewReader("hello world"))
	require.NoError(t, err)
	assert.NotZero(t, a.ID)
	assert.True(t, strings.HasPrefix(a.LocalPath(), conf.Attachment.Path), "stored at %q", a.LocalPath())
	content, err := os.ReadFile(a.LocalPath())
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))

	got, err := s.GetByUUID(ctx, a.UUID)
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, int64(7), got.UploaderID)
	assert.Equal(t, "notes.txt", got.Name)
	assert.Zero(t, got.IssueID+got.CommentID+got.ReleaseID)
	assert.NotZero(t, got.CreatedUnix)
	assert.Equal(t, got.CreatedUnix, got.Created.Unix())

	b, err := s.Create(ctx, 7, "notes.txt", strings.NewReader(""))
	require.NoError(t, err)
	assert.NotEqual(t, a.UUID, b.UUID)
}

func attachmentsGetByUUID(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	a := newTestAttachment(t, ctx, s, "a.txt", 0, 0, 0)

	got, err := s.GetByUUID(ctx, a.UUID)
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.ID)

	// PostgreSQL stores UUIDs natively, so neither a malformed UUID nor an
	// unknown one may fail with anything but not found.
	for _, uuid := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", ""} {
		_, err = s.GetByUUID(ctx, uuid)
		assert.True(t, IsErrAttachmentNotExist(err), "%q: got %v", uuid, err)
	}
}

func attachmentsBind(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	unbound1 := newTestAttachment(t, ctx, s, "unbound1", 0, 0, 0)
	unbound2 := newTestAttachment(t, ctx, s, "unbound2", 0, 0, 0)
	untouched := newTestAttachment(t, ctx, s, "untouched", 0, 0, 0)
	ofIssue := newTestAttachment(t, ctx, s, "of-issue", 9, 0, 0)
	ofRelease := newTestAttachment(t, ctx, s, "of-release", 0, 0, 9)

	err := s.Bind(ctx, 3, 4, []string{unbound1.UUID, ofIssue.UUID, ofRelease.UUID, "not-a-uuid"})
	require.NoError(t, err)
	err = s.Bind(ctx, 5, 0, []string{unbound2.UUID})
	require.NoError(t, err)
	err = s.Bind(ctx, 6, 0, nil)
	require.NoError(t, err)

	for _, tc := range []struct {
		uuid                          string
		issueID, commentID, releaseID int64
	}{
		{unbound1.UUID, 3, 4, 0},
		{unbound2.UUID, 5, 0, 0},
		{untouched.UUID, 0, 0, 0},
		// Attachments that are already attached cannot be claimed.
		{ofIssue.UUID, 9, 0, 0},
		{ofRelease.UUID, 0, 0, 9},
	} {
		got, err := s.GetByUUID(ctx, tc.uuid)
		require.NoError(t, err)
		assert.Equal(t, []int64{tc.issueID, tc.commentID, tc.releaseID}, []int64{got.IssueID, got.CommentID, got.ReleaseID}, got.Name)
	}
}

func attachmentsBindToRelease(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	unbound := newTestAttachment(t, ctx, s, "unbound", 0, 0, 0)
	untouched := newTestAttachment(t, ctx, s, "untouched", 0, 0, 0)
	ofIssue := newTestAttachment(t, ctx, s, "of-issue", 9, 0, 0)

	err := s.bindToRelease(ctx, 3, []string{unbound.UUID, ofIssue.UUID, "not-a-uuid"})
	require.NoError(t, err)

	got, err := s.ListByReleaseID(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{unbound.UUID}, attachmentUUIDs(got))
	got1, err := s.GetByUUID(ctx, untouched.UUID)
	require.NoError(t, err)
	assert.Zero(t, got1.ReleaseID)
	got1, err = s.GetByUUID(ctx, ofIssue.UUID)
	require.NoError(t, err)
	assert.Zero(t, got1.ReleaseID)
}

func attachmentsList(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	issue1 := newTestAttachment(t, ctx, s, "issue1", 1, 0, 0)
	issue2 := newTestAttachment(t, ctx, s, "issue2", 1, 0, 0)
	comment := newTestAttachment(t, ctx, s, "comment", 1, 2, 0)
	newTestAttachment(t, ctx, s, "other-issue", 2, 1, 0)
	release := newTestAttachment(t, ctx, s, "release", 0, 0, 1)
	newTestAttachment(t, ctx, s, "unbound", 0, 0, 0)

	got, err := s.ListByIssueID(ctx, 1)
	require.NoError(t, err)
	// Attachments of comments of the issue are left out.
	assert.Equal(t, []string{issue1.UUID, issue2.UUID}, attachmentUUIDs(got))

	got, err = s.ListByCommentID(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{comment.UUID}, attachmentUUIDs(got))

	got, err = s.ListByReleaseID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{release.UUID}, attachmentUUIDs(got))

	got, err = s.ListByIssueID(ctx, 404)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)

	// An ID of 0 lists no attachments, rather than those not attached to one.
	for name, list := range map[string]func(context.Context, int64) ([]*Attachment, error){
		"issue": s.ListByIssueID, "comment": s.ListByCommentID, "release": s.ListByReleaseID,
	} {
		got, err = list(ctx, 0)
		require.NoError(t, err, name)
		assert.Empty(t, got, name)
	}
}

func attachmentsDelete(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	removed := newTestAttachment(t, ctx, s, "removed", 0, 0, 1)
	kept := newTestAttachment(t, ctx, s, "kept", 0, 0, 1)
	other := newTestAttachment(t, ctx, s, "other", 0, 0, 1)

	// The row is deleted by ID even when other fields are stale.
	removed.ReleaseID = 2
	err := s.Delete(ctx, true, removed)
	require.NoError(t, err)
	assert.False(t, osx.Exist(removed.LocalPath()), "file not removed")
	_, err = s.GetByUUID(ctx, removed.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)

	err = s.Delete(ctx, false, kept)
	require.NoError(t, err)
	assert.True(t, osx.IsFile(kept.LocalPath()), "file removed")
	_, err = s.GetByUUID(ctx, kept.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)

	// A file that is already gone does not stop deleting the attachment.
	gone := newTestAttachment(t, ctx, s, "gone", 0, 0, 1)
	err = os.Remove(gone.LocalPath())
	require.NoError(t, err)
	err = s.Delete(ctx, true, gone)
	require.NoError(t, err)
	_, err = s.GetByUUID(ctx, gone.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)

	got, err := s.GetByUUID(ctx, other.UUID)
	require.NoError(t, err)
	assert.Equal(t, other.ID, got.ID)
	assert.True(t, osx.IsFile(other.LocalPath()), "file of other attachment removed")

	err = s.Delete(ctx, true)
	require.NoError(t, err)
}

func attachmentsDeleteByCommentID(t *testing.T, ctx context.Context, s *AttachmentsStore) {
	setMockAttachmentPath(t)
	comment1 := newTestAttachment(t, ctx, s, "comment1", 1, 2, 0)
	comment2 := newTestAttachment(t, ctx, s, "comment2", 1, 2, 0)
	issue := newTestAttachment(t, ctx, s, "issue", 1, 0, 0)
	otherComment := newTestAttachment(t, ctx, s, "other-comment", 1, 3, 0)

	// An ID of 0 deletes nothing, rather than the attachments of no comment.
	err := s.DeleteByCommentID(ctx, 0, true)
	require.NoError(t, err)
	_, err = s.GetByUUID(ctx, issue.UUID)
	require.NoError(t, err)
	require.True(t, osx.IsFile(issue.LocalPath()), "file of issue attachment removed")

	err = s.DeleteByCommentID(ctx, 2, true)
	require.NoError(t, err)
	for _, a := range []*Attachment{comment1, comment2} {
		_, err = s.GetByUUID(ctx, a.UUID)
		assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)
		assert.False(t, osx.Exist(a.LocalPath()), "file of %s not removed", a.Name)
	}
	for _, a := range []*Attachment{issue, otherComment} {
		_, err = s.GetByUUID(ctx, a.UUID)
		require.NoError(t, err)
		assert.True(t, osx.IsFile(a.LocalPath()), "file of %s removed", a.Name)
	}

	err = s.DeleteByCommentID(ctx, 3, false)
	require.NoError(t, err)
	_, err = s.GetByUUID(ctx, otherComment.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "got %v", err)
	assert.True(t, osx.IsFile(otherComment.LocalPath()), "file removed")
}
