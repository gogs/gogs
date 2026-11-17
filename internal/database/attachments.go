package database

import (
	"context"
	"io"
	"os"
	"path"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AttachmentsStore is the storage layer for files attached to issues, comments
// and releases.
type AttachmentsStore struct {
	db *gorm.DB
}

func newAttachmentsStore(db *gorm.DB) *AttachmentsStore {
	return &AttachmentsStore{db: db}
}

// Create stores the content of r as a new attachment uploaded by the user with
// given ID, which is not attached to anything yet.
func (s *AttachmentsStore) Create(ctx context.Context, uploaderID int64, name string, r io.Reader) (*Attachment, error) {
	attach := &Attachment{
		UUID:       uuid.New().String(),
		UploaderID: uploaderID,
		Name:       name,
	}

	localPath := attach.LocalPath()
	err := os.MkdirAll(path.Dir(localPath), os.ModePerm)
	if err != nil {
		return nil, errors.Wrap(err, "create directory")
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, errors.Wrap(err, "create file")
	}
	defer func() { _ = fw.Close() }()

	if _, err = io.Copy(fw, r); err != nil {
		return nil, errors.Wrap(err, "write file")
	}
	if err = s.db.WithContext(ctx).Create(attach).Error; err != nil {
		return nil, err
	}
	return attach, nil
}

// GetByUUID returns the attachment with given UUID. It returns
// ErrAttachmentNotExist when not found.
func (s *AttachmentsStore) GetByUUID(ctx context.Context, uuid string) (*Attachment, error) {
	if !isValidUUID(uuid) {
		return nil, ErrAttachmentNotExist{args: map[string]any{"uuid": uuid}}
	}

	attach := new(Attachment)
	err := s.db.WithContext(ctx).Where("uuid = ?", uuid).First(attach).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttachmentNotExist{args: map[string]any{"uuid": uuid}}
		}
		return nil, err
	}
	return attach, nil
}

// unboundAttachmentCond matches attachments that are not yet linked to any
// issue, comment, or release. Attachments carry no owner, so only unbound ones
// may be linked to prevent claiming attachments of other repositories by UUID.
const unboundAttachmentCond = "issue_id = 0 AND comment_id = 0 AND release_id = 0"

// bindToRelease attaches the unattached attachments with given UUIDs to the
// release with given ID.
func (s *AttachmentsStore) bindToRelease(ctx context.Context, releaseID int64, uuids []string) error {
	uuids = slices.DeleteFunc(slices.Clone(uuids), func(s string) bool { return !isValidUUID(s) })
	if len(uuids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(new(Attachment)).Where(unboundAttachmentCond).Where("uuid IN ?", uuids).Update("release_id", releaseID).Error
}

// Bind attaches the unattached attachments with given UUIDs to the issue, and
// to the comment when commentID is not 0, e.g., files added while editing
// them. UUIDs of attachments that are already attached are skipped.
func (s *AttachmentsStore) Bind(ctx context.Context, issueID, commentID int64, uuids []string) error {
	uuids = slices.DeleteFunc(slices.Clone(uuids), func(s string) bool { return !isValidUUID(s) })
	if len(uuids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Model(new(Attachment)).Where(unboundAttachmentCond).Where("uuid IN ?", uuids).
		Updates(map[string]any{"issue_id": issueID, "comment_id": commentID}).Error
}

// ListByIssueID returns the attachments of the issue with given ID, excluding
// those of its comments.
func (s *AttachmentsStore) ListByIssueID(ctx context.Context, issueID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, s.db.WithContext(ctx).Where("issue_id = ? AND comment_id = 0", issueID).Order("id").Find(&attachments).Error
}

// ListByCommentID returns the attachments of the comment with given ID.
func (s *AttachmentsStore) ListByCommentID(ctx context.Context, commentID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, s.db.WithContext(ctx).Where("comment_id = ?", commentID).Order("id").Find(&attachments).Error
}

// ListByReleaseID returns the attachments of the release with given ID.
func (s *AttachmentsStore) ListByReleaseID(ctx context.Context, releaseID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, s.db.WithContext(ctx).Where("release_id = ?", releaseID).Order("id").Find(&attachments).Error
}

// Delete deletes given attachments, and their files when remove is true.
func (s *AttachmentsStore) Delete(ctx context.Context, remove bool, attachments ...*Attachment) error {
	for _, a := range attachments {
		if remove {
			err := os.Remove(a.LocalPath())
			if err != nil && !os.IsNotExist(err) {
				return errors.Wrapf(err, "remove file of attachment %d", a.ID)
			}
		}

		// Delete by ID alone, because the other fields may be stale, e.g., a release
		// detaches its files before deleting those that were left out.
		err := s.db.WithContext(ctx).Delete(new(Attachment), a.ID).Error
		if err != nil {
			return errors.Wrapf(err, "delete attachment %d", a.ID)
		}
	}
	return nil
}

// DeleteByCommentID deletes the attachments of the comment with given ID, and
// their files when remove is true.
func (s *AttachmentsStore) DeleteByCommentID(ctx context.Context, commentID int64, remove bool) error {
	attachments, err := s.ListByCommentID(ctx, commentID)
	if err != nil {
		return errors.Wrap(err, "list attachments")
	}
	return s.Delete(ctx, remove, attachments...)
}
