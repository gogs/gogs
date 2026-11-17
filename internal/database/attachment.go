package database

import (
	"fmt"
	"path"
	"time"

	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
)

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID        int64
	UUID      string `xorm:"uuid UNIQUE"`
	IssueID   int64  `xorm:"INDEX"`
	CommentID int64
	ReleaseID int64 `xorm:"INDEX"`
	// UploaderID is the user who uploaded the file, the only one who can
	// download it before it is attached to an issue, a comment or a release.
	UploaderID int64
	Name       string

	Created     time.Time `xorm:"-" json:"-" gorm:"-"`
	CreatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (a *Attachment) BeforeCreate(tx *gorm.DB) error {
	if a.CreatedUnix == 0 {
		a.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (a *Attachment) AfterFind(_ *gorm.DB) error {
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

var _ errx.NotFound = (*ErrAttachmentNotExist)(nil)

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
