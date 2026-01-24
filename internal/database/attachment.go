package database

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	gouuid "github.com/satori/go.uuid"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
)

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID        int64
	UUID      string `gorm:"column:uuid;uniqueIndex"`
	IssueID   int64  `gorm:"index"`
	CommentID int64
	ReleaseID int64 `gorm:"index"`
	Name      string

	Created     time.Time `gorm:"-" json:"-"`
	CreatedUnix int64
}

func (a *Attachment) BeforeCreate(tx *gorm.DB) error {
	if a.CreatedUnix == 0 {
		a.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

func (a *Attachment) AfterFind(tx *gorm.DB) error {
	a.Created = time.Unix(a.CreatedUnix, 0).Local()
	return nil
}

// AttachmentLocalPath returns where attachment is stored in local file system based on given UUID.
func AttachmentLocalPath(uuid string) string {
	return path.Join(conf.Attachment.Path, uuid[0:1], uuid[1:2], uuid)
}

// LocalPath returns where attachment is stored in local file system.
func (a *Attachment) LocalPath() string {
	return AttachmentLocalPath(a.UUID)
}

// NewAttachment creates a new attachment object.
func NewAttachment(name string, buf []byte, file multipart.File) (_ *Attachment, err error) {
	attach := &Attachment{
		UUID: gouuid.NewV4().String(),
		Name: name,
	}

	localPath := attach.LocalPath()
	if err = os.MkdirAll(path.Dir(localPath), os.ModePerm); err != nil {
		return nil, errors.Newf("MkdirAll: %v", err)
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, errors.Newf("Create: %v", err)
	}
	defer fw.Close()

	if _, err = fw.Write(buf); err != nil {
		return nil, errors.Newf("write: %v", err)
	} else if _, err = io.Copy(fw, file); err != nil {
		return nil, errors.Newf("copy: %v", err)
	}

	if err := db.Create(attach).Error; err != nil {
		return nil, err
	}

	return attach, nil
}

var _ errutil.NotFound = (*ErrAttachmentNotExist)(nil)

type ErrAttachmentNotExist struct {
	args map[string]any
}

func IsErrAttachmentNotExist(err error) bool {
	_, ok := err.(ErrAttachmentNotExist)
	return ok
}

func (err ErrAttachmentNotExist) Error() string {
	return fmt.Sprintf("attachment does not exist: %v", err.args)
}

func (ErrAttachmentNotExist) NotFound() bool {
	return true
}

func getAttachmentByUUID(e *gorm.DB, uuid string) (*Attachment, error) {
	attach := &Attachment{}
	err := e.Where("uuid = ?", uuid).First(attach).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAttachmentNotExist{args: map[string]any{"uuid": uuid}}
		}
		return nil, err
	}
	return attach, nil
}

func getAttachmentsByUUIDs(e *gorm.DB, uuids []string) ([]*Attachment, error) {
	if len(uuids) == 0 {
		return []*Attachment{}, nil
	}

	attachments := make([]*Attachment, 0, len(uuids))
	return attachments, e.Where("uuid IN ?", uuids).Find(&attachments).Error
}

// GetAttachmentByUUID returns attachment by given UUID.
func GetAttachmentByUUID(uuid string) (*Attachment, error) {
	return getAttachmentByUUID(db, uuid)
}

func getAttachmentsByIssueID(e *gorm.DB, issueID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, e.Where("issue_id = ? AND comment_id = 0", issueID).Find(&attachments).Error
}

// GetAttachmentsByIssueID returns all attachments of an issue.
func GetAttachmentsByIssueID(issueID int64) ([]*Attachment, error) {
	return getAttachmentsByIssueID(db, issueID)
}

func getAttachmentsByCommentID(e *gorm.DB, commentID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, e.Where("comment_id = ?", commentID).Find(&attachments).Error
}

// GetAttachmentsByCommentID returns all attachments of a comment.
func GetAttachmentsByCommentID(commentID int64) ([]*Attachment, error) {
	return getAttachmentsByCommentID(db, commentID)
}

func getAttachmentsByReleaseID(e *gorm.DB, releaseID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, e.Where("release_id = ?", releaseID).Find(&attachments).Error
}

// GetAttachmentsByReleaseID returns all attachments of a release.
func GetAttachmentsByReleaseID(releaseID int64) ([]*Attachment, error) {
	return getAttachmentsByReleaseID(db, releaseID)
}

// DeleteAttachment deletes the given attachment and optionally the associated file.
func DeleteAttachment(a *Attachment, remove bool) error {
	_, err := DeleteAttachments([]*Attachment{a}, remove)
	return err
}

// DeleteAttachments deletes the given attachments and optionally the associated files.
func DeleteAttachments(attachments []*Attachment, remove bool) (int, error) {
	for i, a := range attachments {
		if remove {
			if err := os.Remove(a.LocalPath()); err != nil {
				return i, err
			}
		}

		if err := db.Delete(a).Error; err != nil {
			return i, err
		}
	}

	return len(attachments), nil
}

// DeleteAttachmentsByIssue deletes all attachments associated with the given issue.
func DeleteAttachmentsByIssue(issueID int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByIssueID(issueID)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}

// DeleteAttachmentsByComment deletes all attachments associated with the given comment.
func DeleteAttachmentsByComment(commentID int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByCommentID(commentID)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}
