package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	api "github.com/gogs/go-gogs-client"
	"github.com/unknwon/com"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/markup"
)

// CommentType defines whether a comment is just a simple comment, an action (like close) or a reference.
type CommentType int

const (
	// Plain comment, can be associated with a commit (CommitID > 0) and a line (LineNum > 0)
	CommentTypeComment CommentType = iota
	CommentTypeReopen
	CommentTypeClose

	// References.
	CommentTypeIssueRef
	// Reference from a commit (not part of a pull request)
	CommentTypeCommitRef
	// Reference from a comment
	CommentTypeCommentRef
	// Reference from a pull request
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
	Poster          *User  `gorm:"-" json:"-"`
	IssueID         int64  `gorm:"index"`
	Issue           *Issue `gorm:"-" json:"-"`
	CommitID        int64
	Line            int64
	Content         string `gorm:"type:text"`
	RenderedContent string `gorm:"-" json:"-"`

	Created     time.Time `gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `gorm:"-" json:"-"`
	UpdatedUnix int64

	// Reference issue in commit message
	CommitSHA string `gorm:"type:varchar(40)"`

	Attachments []*Attachment `gorm:"-" json:"-"`

	// For view issue page.
	ShowTag CommentTag `gorm:"-" json:"-"`
}

func (c *Comment) BeforeCreate(tx *gorm.DB) error {
	if c.CreatedUnix == 0 {
		c.CreatedUnix = tx.NowFunc().Unix()
	}
	if c.UpdatedUnix == 0 {
		c.UpdatedUnix = c.CreatedUnix
	}
	return nil
}

func (c *Comment) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

func (c *Comment) AfterFind(tx *gorm.DB) error {
	c.Created = time.Unix(c.CreatedUnix, 0).Local()
	c.Updated = time.Unix(c.UpdatedUnix, 0).Local()
	return nil
}

func (c *Comment) loadAttributes(tx *gorm.DB) (err error) {
	if c.Poster == nil {
		c.Poster, err = Handle.Users().GetByID(context.TODO(), c.PosterID)
		if err != nil {
			if IsErrUserNotExist(err) {
				c.PosterID = -1
				c.Poster = NewGhostUser()
			} else {
				return errors.Newf("getUserByID.(Poster) [%d]: %v", c.PosterID, err)
			}
		}
	}

	if c.Issue == nil {
		c.Issue, err = getRawIssueByID(tx, c.IssueID)
		if err != nil {
			return errors.Newf("getIssueByID [%d]: %v", c.IssueID, err)
		}
		if c.Issue.Repo == nil {
			c.Issue.Repo, err = getRepositoryByID(tx, c.Issue.RepoID)
			if err != nil {
				return errors.Newf("getRepositoryByID [%d]: %v", c.Issue.RepoID, err)
			}
		}
	}

	if c.Attachments == nil {
		c.Attachments, err = getAttachmentsByCommentID(tx, c.ID)
		if err != nil {
			return errors.Newf("getAttachmentsByCommentID [%d]: %v", c.ID, err)
		}
	}

	return nil
}

func (c *Comment) LoadAttributes() error {
	return c.loadAttributes(db)
}

func (c *Comment) HTMLURL() string {
	return fmt.Sprintf("%s#issuecomment-%d", c.Issue.HTMLURL(), c.ID)
}

// This method assumes following fields have been assigned with valid values:
// Required - Poster, Issue
func (c *Comment) APIFormat() *api.Comment {
	return &api.Comment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  c.Poster.APIFormat(),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func CommentHashTag(id int64) string {
	return "issuecomment-" + com.ToStr(id)
}

// HashTag returns unique hash tag for comment.
func (c *Comment) HashTag() string {
	return CommentHashTag(c.ID)
}

// EventTag returns unique event hash tag for comment.
func (c *Comment) EventTag() string {
	return "event-" + com.ToStr(c.ID)
}

// mailParticipants sends new comment emails to repository watchers
// and mentioned people.
func (c *Comment) mailParticipants(tx *gorm.DB, opType ActionType, issue *Issue) (err error) {
	mentions := markup.FindAllMentions(c.Content)
	if err = updateIssueMentions(tx, c.IssueID, mentions); err != nil {
		return errors.Newf("UpdateIssueMentions [%d]: %v", c.IssueID, err)
	}

	switch opType {
	case ActionCommentIssue:
		issue.Content = c.Content
	case ActionCloseIssue:
		issue.Content = fmt.Sprintf("Closed #%d", issue.Index)
	case ActionReopenIssue:
		issue.Content = fmt.Sprintf("Reopened #%d", issue.Index)
	}
	if err = mailIssueCommentToParticipants(issue, c.Poster, mentions); err != nil {
		log.Error("mailIssueCommentToParticipants: %v", err)
	}

	return nil
}

func createComment(tx *gorm.DB, opts *CreateCommentOptions) (_ *Comment, err error) {
	comment := &Comment{
		Type:      opts.Type,
		PosterID:  opts.Doer.ID,
		Poster:    opts.Doer,
		IssueID:   opts.Issue.ID,
		CommitID:  opts.CommitID,
		CommitSHA: opts.CommitSHA,
		Line:      opts.LineNum,
		Content:   opts.Content,
	}
	if err = tx.Create(comment).Error; err != nil {
		return nil, err
	}

	// Compose comment action, could be plain comment, close or reopen issue/pull request.
	// This object will be used to notify watchers in the end of function.
	act := &Action{
		ActUserID:    opts.Doer.ID,
		ActUserName:  opts.Doer.Name,
		Content:      fmt.Sprintf("%d|%s", opts.Issue.Index, strings.Split(opts.Content, "\n")[0]),
		RepoID:       opts.Repo.ID,
		RepoUserName: opts.Repo.Owner.Name,
		RepoName:     opts.Repo.Name,
		IsPrivate:    opts.Repo.IsPrivate,
	}

	// Check comment type.
	switch opts.Type {
	case CommentTypeComment:
		act.OpType = ActionCommentIssue

		if err = tx.Exec("UPDATE `issue` SET num_comments=num_comments+1 WHERE id=?", opts.Issue.ID).Error; err != nil {
			return nil, err
		}

		// Check attachments
		attachments := make([]*Attachment, 0, len(opts.Attachments))
		for _, uuid := range opts.Attachments {
			attach, err := getAttachmentByUUID(tx, uuid)
			if err != nil {
				if IsErrAttachmentNotExist(err) {
					continue
				}
				return nil, errors.Newf("getAttachmentByUUID [%s]: %v", uuid, err)
			}
			attachments = append(attachments, attach)
		}

		for i := range attachments {
			attachments[i].IssueID = opts.Issue.ID
			attachments[i].CommentID = comment.ID
			if err = tx.Model(attachments[i]).Where("id = ?", attachments[i].ID).Updates(map[string]any{
				"issue_id":   attachments[i].IssueID,
				"comment_id": attachments[i].CommentID,
			}).Error; err != nil {
				return nil, errors.Newf("update attachment [%d]: %v", attachments[i].ID, err)
			}
		}

	case CommentTypeReopen:
		act.OpType = ActionReopenIssue
		if opts.Issue.IsPull {
			act.OpType = ActionReopenPullRequest
		}

		if opts.Issue.IsPull {
			err = tx.Exec("UPDATE `repository` SET num_closed_pulls=num_closed_pulls-1 WHERE id=?", opts.Repo.ID).Error
		} else {
			err = tx.Exec("UPDATE `repository` SET num_closed_issues=num_closed_issues-1 WHERE id=?", opts.Repo.ID).Error
		}
		if err != nil {
			return nil, err
		}

	case CommentTypeClose:
		act.OpType = ActionCloseIssue
		if opts.Issue.IsPull {
			act.OpType = ActionClosePullRequest
		}

		if opts.Issue.IsPull {
			err = tx.Exec("UPDATE `repository` SET num_closed_pulls=num_closed_pulls+1 WHERE id=?", opts.Repo.ID).Error
		} else {
			err = tx.Exec("UPDATE `repository` SET num_closed_issues=num_closed_issues+1 WHERE id=?", opts.Repo.ID).Error
		}
		if err != nil {
			return nil, err
		}
	}

	if err = tx.Exec("UPDATE `issue` SET updated_unix = ? WHERE id = ?", tx.NowFunc().Unix(), opts.Issue.ID).Error; err != nil {
		return nil, errors.Newf("update issue 'updated_unix': %v", err)
	}

	// Notify watchers for whatever action comes in, ignore if no action type.
	if act.OpType > 0 {
		if err = notifyWatchers(tx, act); err != nil {
			log.Error("notifyWatchers: %v", err)
		}
		if err = comment.mailParticipants(tx, act.OpType, opts.Issue); err != nil {
			log.Error("MailParticipants: %v", err)
		}
	}

	return comment, comment.loadAttributes(tx)
}

func createStatusComment(tx *gorm.DB, doer *User, repo *Repository, issue *Issue) (*Comment, error) {
	cmtType := CommentTypeClose
	if !issue.IsClosed {
		cmtType = CommentTypeReopen
	}
	return createComment(tx, &CreateCommentOptions{
		Type:  cmtType,
		Doer:  doer,
		Repo:  repo,
		Issue: issue,
	})
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

// CreateComment creates comment of issue or commit.
func CreateComment(opts *CreateCommentOptions) (comment *Comment, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		comment, err = createComment(tx, opts)
		return err
	})
	return comment, err
}

// CreateIssueComment creates a plain issue comment.
func CreateIssueComment(doer *User, repo *Repository, issue *Issue, content string, attachments []string) (*Comment, error) {
	comment, err := CreateComment(&CreateCommentOptions{
		Type:        CommentTypeComment,
		Doer:        doer,
		Repo:        repo,
		Issue:       issue,
		Content:     content,
		Attachments: attachments,
	})
	if err != nil {
		return nil, errors.Newf("CreateComment: %v", err)
	}

	comment.Issue = issue
	if err = PrepareWebhooks(repo, HookEventTypeIssueComment, &api.IssueCommentPayload{
		Action:     api.HOOK_ISSUE_COMMENT_CREATED,
		Issue:      issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: repo.APIFormatLegacy(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks [comment_id: %d]: %v", comment.ID, err)
	}

	return comment, nil
}

// CreateRefComment creates a commit reference comment to issue.
func CreateRefComment(doer *User, repo *Repository, issue *Issue, content, commitSHA string) error {
	if commitSHA == "" {
		return errors.Newf("cannot create reference with empty commit SHA")
	}

	// Check if same reference from same commit has already existed.
	var count int64
	err := db.Model(new(Comment)).Where("type = ? AND issue_id = ? AND commit_sha = ?",
		CommentTypeCommitRef, issue.ID, commitSHA).Count(&count).Error
	if err != nil {
		return errors.Newf("check reference comment: %v", err)
	} else if count > 0 {
		return nil
	}

	_, err = CreateComment(&CreateCommentOptions{
		Type:      CommentTypeCommitRef,
		Doer:      doer,
		Repo:      repo,
		Issue:     issue,
		CommitSHA: commitSHA,
		Content:   content,
	})
	return err
}

var _ errutil.NotFound = (*ErrCommentNotExist)(nil)

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

// GetCommentByID returns the comment by given ID.
func GetCommentByID(id int64) (*Comment, error) {
	c := new(Comment)
	err := db.Where("id = ?", id).First(c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotExist{args: map[string]any{"commentID": id}}
		}
		return nil, err
	}
	return c, c.LoadAttributes()
}

// FIXME: use CommentList to improve performance.
func loadCommentsAttributes(tx *gorm.DB, comments []*Comment) (err error) {
	for i := range comments {
		if err = comments[i].loadAttributes(tx); err != nil {
			return errors.Newf("loadAttributes [%d]: %v", comments[i].ID, err)
		}
	}

	return nil
}

func getCommentsByIssueIDSince(tx *gorm.DB, issueID, since int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	query := tx.Where("issue_id = ?", issueID).Order("created_unix ASC")
	if since > 0 {
		query = query.Where("updated_unix >= ?", since)
	}

	if err := query.Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, loadCommentsAttributes(tx, comments)
}

func getCommentsByRepoIDSince(tx *gorm.DB, repoID, since int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	query := tx.Joins("INNER JOIN issue ON issue.id = comment.issue_id").
		Where("issue.repo_id = ?", repoID).
		Order("comment.created_unix ASC")
	if since > 0 {
		query = query.Where("comment.updated_unix >= ?", since)
	}
	if err := query.Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, loadCommentsAttributes(tx, comments)
}

func getCommentsByIssueID(tx *gorm.DB, issueID int64) ([]*Comment, error) {
	return getCommentsByIssueIDSince(tx, issueID, -1)
}

// GetCommentsByIssueID returns all comments of an issue.
func GetCommentsByIssueID(issueID int64) ([]*Comment, error) {
	return getCommentsByIssueID(db, issueID)
}

// GetCommentsByIssueIDSince returns a list of comments of an issue since a given time point.
func GetCommentsByIssueIDSince(issueID, since int64) ([]*Comment, error) {
	return getCommentsByIssueIDSince(db, issueID, since)
}

// GetCommentsByRepoIDSince returns a list of comments for all issues in a repo since a given time point.
func GetCommentsByRepoIDSince(repoID, since int64) ([]*Comment, error) {
	return getCommentsByRepoIDSince(db, repoID, since)
}

// UpdateComment updates information of comment.
func UpdateComment(doer *User, c *Comment, oldContent string) (err error) {
	if err = db.Model(c).Where("id = ?", c.ID).Updates(c).Error; err != nil {
		return err
	}

	if err = c.Issue.LoadAttributes(); err != nil {
		log.Error("Issue.LoadAttributes [issue_id: %d]: %v", c.IssueID, err)
	} else if err = PrepareWebhooks(c.Issue.Repo, HookEventTypeIssueComment, &api.IssueCommentPayload{
		Action:  api.HOOK_ISSUE_COMMENT_EDITED,
		Issue:   c.Issue.APIFormat(),
		Comment: c.APIFormat(),
		Changes: &api.ChangesPayload{
			Body: &api.ChangesFromPayload{
				From: oldContent,
			},
		},
		Repository: c.Issue.Repo.APIFormatLegacy(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks [comment_id: %d]: %v", c.ID, err)
	}

	return nil
}

// DeleteCommentByID deletes the comment by given ID.
func DeleteCommentByID(doer *User, id int64) error {
	comment, err := GetCommentByID(id)
	if err != nil {
		if IsErrCommentNotExist(err) {
			return nil
		}
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", comment.ID).Delete(new(Comment)).Error; err != nil {
			return err
		}

		if comment.Type == CommentTypeComment {
			if err := tx.Exec("UPDATE `issue` SET num_comments = num_comments - 1 WHERE id = ?", comment.IssueID).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errors.Newf("transaction: %v", err)
	}

	_, err = DeleteAttachmentsByComment(comment.ID, true)
	if err != nil {
		log.Error("Failed to delete attachments by comment[%d]: %v", comment.ID, err)
	}

	if err = comment.Issue.LoadAttributes(); err != nil {
		log.Error("Issue.LoadAttributes [issue_id: %d]: %v", comment.IssueID, err)
	} else if err = PrepareWebhooks(comment.Issue.Repo, HookEventTypeIssueComment, &api.IssueCommentPayload{
		Action:     api.HOOK_ISSUE_COMMENT_DELETED,
		Issue:      comment.Issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: comment.Issue.Repo.APIFormatLegacy(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks [comment_id: %d]: %v", comment.ID, err)
	}
	return nil
}
