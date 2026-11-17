package database

import (
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// CommentType defines whether a comment is just a simple comment, an action (like close) or a reference.
type CommentType int

const (
	// Plain comment, can be associated with a commit (CommitID > 0) and a line (Line > 0).
	CommentTypeComment CommentType = iota
	CommentTypeReopen
	CommentTypeClose

	// References.
	CommentTypeIssueRef
	// Reference from a commit (not part of a pull request).
	CommentTypeCommitRef
	// Reference from a comment.
	CommentTypeCommentRef
	// Reference from a pull request.
	CommentTypePullRef
)

type CommentTag int

const (
	CommentTagNone CommentTag = iota
	CommentTagPoster
	CommentTagWriter
	CommentTagOwner
)

// Comment represents a comment in commit and issue page.
type Comment struct {
	ID              int64
	Type            CommentType
	PosterID        int64
	Poster          *User  `xorm:"-" json:"-" gorm:"-"`
	IssueID         int64  `xorm:"INDEX"`
	Issue           *Issue `xorm:"-" json:"-" gorm:"-"`
	CommitID        int64
	Line            int64
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-" json:"-" gorm:"-"`

	Created     time.Time `xorm:"-" json:"-" gorm:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" json:"-" gorm:"-"`
	UpdatedUnix int64

	// Reference issue in commit message
	CommitSHA string `xorm:"VARCHAR(64)"`

	Attachments []*Attachment `xorm:"-" json:"-" gorm:"-"`

	// For view issue page.
	ShowTag CommentTag `xorm:"-" json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (c *Comment) BeforeCreate(tx *gorm.DB) error {
	if c.CreatedUnix == 0 {
		c.CreatedUnix = tx.NowFunc().Unix()
		c.UpdatedUnix = c.CreatedUnix
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (c *Comment) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (c *Comment) AfterFind(_ *gorm.DB) error {
	c.Created = time.Unix(c.CreatedUnix, 0).Local()
	c.Updated = time.Unix(c.UpdatedUnix, 0).Local()
	return nil
}

func (c *Comment) HTMLURL() string {
	return fmt.Sprintf("%s#issuecomment-%d", c.Issue.HTMLURL(), c.ID)
}

// APIFormat returns the API format of the comment. It assumes the following
// fields have been assigned with valid values:
// Required - Poster, Issue
func (c *Comment) APIFormat() *apiv1types.IssueComment {
	return &apiv1types.IssueComment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  c.Poster.APIFormat(),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func CommentHashTag(id int64) string {
	return "issuecomment-" + strconv.FormatInt(id, 10)
}

// HashTag returns unique hash tag for comment.
func (c *Comment) HashTag() string {
	return CommentHashTag(c.ID)
}

// EventTag returns unique event hash tag for comment.
func (c *Comment) EventTag() string {
	return "event-" + strconv.FormatInt(c.ID, 10)
}

type CreateCommentOptions struct {
	Type  CommentType
	Doer  *User
	Repo  *Repository
	Issue *Issue

	CommitID    int64
	CommitSHA   string
	LineNum     int64
	Content     string
	Attachments []string // UUIDs of attachments
}

var _ errx.NotFound = (*ErrCommentNotExist)(nil)

type ErrCommentNotExist struct {
	args map[string]any
}

func IsErrCommentNotExist(err error) bool {
	_, ok := err.(ErrCommentNotExist)
	return ok
}

func (err ErrCommentNotExist) Error() string {
	return fmt.Sprintf("comment does not exist: %v", err.args)
}

func (ErrCommentNotExist) NotFound() bool {
	return true
}
