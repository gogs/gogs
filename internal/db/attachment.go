// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"time"

	gouuid "github.com/satori/go.uuid"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
)

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID        int64
	UUID      string `xorm:"uuid UNIQUE"`
	IssueID   int64  `xorm:"INDEX"`
	CommentID int64
	ReleaseID int64 `xorm:"INDEX"`
	Name      string

	Created     time.Time `xorm:"-" json:"-"`
	CreatedUnix int64
}

func (a *Attachment) BeforeInsert() {
	a.CreatedUnix = time.Now().Unix()
}

func (a *Attachment) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		a.Created = time.Unix(a.CreatedUnix, 0).Local()
	}
}

// AttachmentLocalPath returns where attachment is stored in local file system based on given UUID.
func AttachmentLocalPath(uuid string) string {
	return path.Join(conf.Attachment.Path, uuid[0:1], uuid[1:2], uuid)
}

// LocalPath returns where attachment is stored in local file system.
func (attach *Attachment) LocalPath() string {
	return AttachmentLocalPath(attach.UUID)
}

// NewAttachment creates a new attachment object.
func NewAttachment(name string, buf []byte, file multipart.File) (_ *Attachment, err error) {
	attach := &Attachment{
		UUID: gouuid.NewV4().String(),
		Name: name,
	}

	localPath := attach.LocalPath()
	if err = os.MkdirAll(path.Dir(localPath), os.ModePerm); err != nil {
		return nil, fmt.Errorf("MkdirAll: %v", err)
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, fmt.Errorf("Create: %v", err)
	}
	defer fw.Close()

	if _, err = fw.Write(buf); err != nil {
		return nil, fmt.Errorf("Write: %v", err)
	} else if _, err = io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("Copy: %v", err)
	}

	if _, err := x.Insert(attach); err != nil {
		return nil, err
	}

	return attach, nil
}

var _ errutil.NotFound = (*ErrAttachmentNotExist)(nil)

type ErrAttachmentNotExist struct {
	args map[string]interface{}
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

func getAttachmentByUUID(e Engine, uuid string) (*Attachment, error) {
	attach := &Attachment{UUID: uuid}
	has, err := e.Get(attach)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAttachmentNotExist{args: map[string]interface{}{"uuid": uuid}}
	}
	return attach, nil
}

func getAttachmentsByUUIDs(e Engine, uuids []string) ([]*Attachment, error) {
	if len(uuids) == 0 {
		return []*Attachment{}, nil
	}

	// Silently drop invalid uuids.
	attachments := make([]*Attachment, 0, len(uuids))
	return attachments, e.In("uuid", uuids).Find(&attachments)
}

// GetAttachmentByUUID returns attachment by given UUID.
func GetAttachmentByUUID(uuid string) (*Attachment, error) {
	return getAttachmentByUUID(x, uuid)
}

func getAttachmentsByIssueID(e Engine, issueID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, e.Where("issue_id = ? AND comment_id = 0", issueID).Find(&attachments)
}

// GetAttachmentsByIssueID returns all attachments of an issue.
func GetAttachmentsByIssueID(issueID int64) ([]*Attachment, error) {
	return getAttachmentsByIssueID(x, issueID)
}

func getAttachmentsByCommentID(e Engine, commentID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 5)
	return attachments, e.Where("comment_id=?", commentID).Find(&attachments)
}

// GetAttachmentsByCommentID returns all attachments of a comment.
func GetAttachmentsByCommentID(commentID int64) ([]*Attachment, error) {
	return getAttachmentsByCommentID(x, commentID)
}

func getAttachmentsByReleaseID(e Engine, releaseID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, e.Where("release_id = ?", releaseID).Find(&attachments)
}

// GetAttachmentsByReleaseID returns all attachments of a release.
func GetAttachmentsByReleaseID(releaseID int64) ([]*Attachment, error) {
	return getAttachmentsByReleaseID(x, releaseID)
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

		if _, err := x.Delete(a); err != nil {
			return i, err
		}
	}

	return len(attachments), nil
}

// DeleteAttachmentsByIssue deletes all attachments associated with the given issue.
func DeleteAttachmentsByIssue(issueId int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByIssueID(issueId)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}

// DeleteAttachmentsByComment deletes all attachments associated with the given comment.
func DeleteAttachmentsByComment(commentId int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByCommentID(commentId)
	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}
