package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/errx"
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
	Poster          *User  `json:"-" gorm:"-"`
	IssueID         int64  `gorm:"index:IDX_comment_issue_id"`
	Issue           *Issue `json:"-" gorm:"-"`
	CommitID        int64
	Line            int64
	Content         string `gorm:"type:TEXT"`
	RenderedContent string `json:"-" gorm:"-"`

	Created     time.Time `json:"-" gorm:"-"`
	CreatedUnix int64
	Updated     time.Time `json:"-" gorm:"-"`
	UpdatedUnix int64

	// Reference issue in commit message
	CommitSHA string `gorm:"type:VARCHAR(64)"`

	Attachments []*Attachment `json:"-" gorm:"-"`

	// For view issue page.
	ShowTag CommentTag `json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (c *Comment) BeforeCreate(tx *gorm.DB) error {
	if c.CreatedUnix == 0 {
		c.CreatedUnix = tx.NowFunc().Unix()
		c.UpdatedUnix = c.CreatedUnix
	}
	// Webhook payloads and API responses use the comment just created.
	c.Created = time.Unix(c.CreatedUnix, 0).Local()
	c.Updated = time.Unix(c.UpdatedUnix, 0).Local()
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

func (c *Comment) HTMLURL(externalURL string) string {
	return fmt.Sprintf("%s#issuecomment-%d", c.Issue.HTMLURL(externalURL), c.ID)
}

// APIFormat returns the API format of the comment. It assumes the following
// fields have been assigned with valid values:
// Required - Poster, Issue
func (c *Comment) APIFormat(links LinkOptions) *v1types.IssueComment {
	return &v1types.IssueComment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(links.ExternalURL),
		Poster:  c.Poster.APIFormat(links),
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

// CommentsStore is the storage layer for comments of issues and pull
// requests.
type CommentsStore struct {
	db *gorm.DB
}

func newCommentsStore(db *gorm.DB) *CommentsStore {
	return &CommentsStore{db: db}
}

// create creates the comment of given options, updates the numbers that
// depend on its type, notifies the watchers of the repository and mails the
// participants of the issue.
func (s *CommentsStore) create(ctx context.Context, logger *logx.Logger, notify NotifyOptions, opts *CreateCommentOptions) (*Comment, error) {
	db := s.db.WithContext(ctx)
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
	err := db.Create(comment).Error
	if err != nil {
		return nil, errors.Wrap(err, "insert comment")
	}

	act := &Action{
		ActUserID:    opts.Doer.ID,
		ActUserName:  opts.Doer.Name,
		Content:      fmt.Sprintf("%d|%s", opts.Issue.Index, strings.Split(opts.Content, "\n")[0]),
		RepoID:       opts.Repo.ID,
		RepoUserName: opts.Repo.Owner.Name,
		RepoName:     opts.Repo.Name,
		IsPrivate:    opts.Repo.IsPrivate,
	}

	closedColumn := "num_closed_issues"
	if opts.Issue.IsPull {
		closedColumn = "num_closed_pulls"
	}
	switch opts.Type {
	case CommentTypeComment:
		act.OpType = ActionCommentIssue
		err = db.Exec("UPDATE issue SET num_comments = num_comments + 1 WHERE id = ?", opts.Issue.ID).Error
		if err != nil {
			return nil, errors.Wrap(err, "count comment")
		}
		err = newAttachmentsStore(s.db).Bind(ctx, opts.Issue.ID, comment.ID, opts.Attachments)
		if err != nil {
			return nil, errors.Wrap(err, "attach files")
		}

	case CommentTypeReopen:
		act.OpType = ActionReopenIssue
		if opts.Issue.IsPull {
			act.OpType = ActionReopenPullRequest
		}
		err = db.Exec("UPDATE repository SET "+closedColumn+" = "+closedColumn+" - 1 WHERE id = ?", opts.Repo.ID).Error
		if err != nil {
			return nil, errors.Wrap(err, "count reopened")
		}

	case CommentTypeClose:
		act.OpType = ActionCloseIssue
		if opts.Issue.IsPull {
			act.OpType = ActionClosePullRequest
		}
		err = db.Exec("UPDATE repository SET "+closedColumn+" = "+closedColumn+" + 1 WHERE id = ?", opts.Repo.ID).Error
		if err != nil {
			return nil, errors.Wrap(err, "count closed")
		}
	}

	err = db.Exec("UPDATE issue SET updated_unix = ? WHERE id = ?", db.NowFunc().Unix(), opts.Issue.ID).Error
	if err != nil {
		return nil, errors.Wrap(err, "update issue")
	}

	// Only plain comments and closing or reopening are announced to watchers.
	if act.OpType > 0 {
		if err = newActionsStore(s.db).notifyWatchers(ctx, act); err != nil {
			logger.Error("Failed to notify watchers", "error", err)
		}
		if err = s.mailParticipants(ctx, logger, notify, comment, act.OpType, opts.Issue); err != nil {
			logger.Error("Failed to mail participants", "error", err)
		}
	}
	return comment, s.LoadAttributes(ctx, comment)
}

// mailParticipants marks the users the comment mentions as mentioned, and
// sends emails about the comment to them and to the participants of the
// issue.
func (s *CommentsStore) mailParticipants(ctx context.Context, logger *logx.Logger, notify NotifyOptions, c *Comment, opType ActionType, issue *Issue) error {
	mentions := findLowerMentions(notify.Markup, c.Content)
	err := newIssuesStore(s.db).updateMentions(ctx, issue, mentions)
	if err != nil {
		return errors.Wrap(err, "update mentions")
	}

	switch opType {
	case ActionCommentIssue:
		issue.Content = c.Content
	case ActionCloseIssue:
		issue.Content = fmt.Sprintf("Closed #%d", issue.Index)
	case ActionReopenIssue:
		issue.Content = fmt.Sprintf("Reopened #%d", issue.Index)
	}
	if err = mailIssueCommentToParticipants(logger, notify, s.db.WithContext(ctx), issue, c.Poster, mentions); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}
	return nil
}

// createStatus creates the comment that the issue is closed or reopened.
func (s *CommentsStore) createStatus(ctx context.Context, logger *logx.Logger, notify NotifyOptions, doer *User, repo *Repository, issue *Issue) (*Comment, error) {
	typ := CommentTypeClose
	if !issue.IsClosed {
		typ = CommentTypeReopen
	}
	return s.create(ctx, logger, notify, &CreateCommentOptions{
		Type:  typ,
		Doer:  doer,
		Repo:  repo,
		Issue: issue,
	})
}

// Create creates a comment of given content on the issue of given repository
// as the doer, attaches the unattached attachments with given UUIDs to it, and
// prepares webhooks.
func (s *CommentsStore) Create(ctx context.Context, logger *logx.Logger, notify NotifyOptions, doer *User, repo *Repository, issue *Issue, content string, uuids []string) (*Comment, error) {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	var comment *Comment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		comment, err = newCommentsStore(tx).create(ctx, logger, notify, &CreateCommentOptions{
			Type:        CommentTypeComment,
			Doer:        doer,
			Repo:        repo,
			Issue:       issue,
			Content:     content,
			Attachments: uuids,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	comment.Issue = issue
	links := notify.Links
	err = newWebhooksStore(s.db).Prepare(ctx, logger, links.ExternalURL, repo, HookEventTypeIssueComment, &v1types.WebhookIssueCommentPayload{
		Action:     v1types.WebhookIssueCommentCreated,
		Issue:      issue.APIFormat(links),
		Comment:    comment.APIFormat(links),
		Repository: repo.APIFormatLegacy(logger, links, nil),
		Sender:     doer.APIFormat(links),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", comment.ID, "error", err)
	}
	return comment, nil
}

// createRef creates a comment on the issue that the commit with given SHA
// references it, unless the commit already did.
func (s *CommentsStore) createRef(ctx context.Context, logger *logx.Logger, notify NotifyOptions, doer *User, repo *Repository, issue *Issue, content, commitSHA string) error {
	if commitSHA == "" {
		return errors.New("cannot create reference with empty commit SHA")
	}

	var count int64
	err := s.db.WithContext(ctx).Model(new(Comment)).
		Where("type = ? AND issue_id = ? AND commit_sha = ?", CommentTypeCommitRef, issue.ID, commitSHA).
		Count(&count).Error
	if err != nil {
		return errors.Wrap(err, "count references")
	} else if count > 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := newCommentsStore(tx).create(ctx, logger, notify, &CreateCommentOptions{
			Type:      CommentTypeCommitRef,
			Doer:      doer,
			Repo:      repo,
			Issue:     issue,
			CommitSHA: commitSHA,
			Content:   content,
		})
		return err
	})
}

// GetByID returns the comment with given ID and its attributes. It returns
// ErrCommentNotExist when not found.
func (s *CommentsStore) GetByID(ctx context.Context, id int64) (*Comment, error) {
	c := new(Comment)
	err := s.db.WithContext(ctx).Where("id = ?", id).First(c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommentNotExist{args: map[string]any{"commentID": id}}
		}
		return nil, err
	}
	return c, s.LoadAttributes(ctx, c)
}

// LoadAttributes sets the poster, the issue with its repository and the
// attachments of given comments that are not set yet. Comments whose poster
// no longer exists get the ghost user.
func (s *CommentsStore) LoadAttributes(ctx context.Context, comments ...*Comment) (err error) {
	for _, c := range comments {
		if c.Poster == nil {
			c.Poster, err = newUsersStore(s.db).GetByID(ctx, c.PosterID)
			if err != nil {
				if !IsErrUserNotExist(err) {
					return errors.Wrapf(err, "get poster %d", c.PosterID)
				}
				c.PosterID = -1
				c.Poster = NewGhostUser()
			}
		}

		if c.Issue == nil {
			c.Issue, err = newIssuesStore(s.db).getRaw(ctx, map[string]any{"issueID": c.IssueID}, "id = ?", c.IssueID)
			if err != nil {
				return errors.Wrapf(err, "get issue %d", c.IssueID)
			}
		}
		if c.Issue.Repo == nil {
			c.Issue.Repo, err = newReposStore(s.db).getWithAttributes(ctx, c.Issue.RepoID)
			if err != nil {
				return errors.Wrapf(err, "get repository %d", c.Issue.RepoID)
			}
		}

		if c.Attachments == nil {
			c.Attachments, err = newAttachmentsStore(s.db).ListByCommentID(ctx, c.ID)
			if err != nil {
				return errors.Wrap(err, "list attachments")
			}
		}
	}
	return nil
}

// ListByIssueID returns the comments of the issue with given ID that are
// updated at or after given Unix time, oldest first, and their attributes.
func (s *CommentsStore) ListByIssueID(ctx context.Context, issueID, since int64) ([]*Comment, error) {
	db := s.db.WithContext(ctx).Where("issue_id = ?", issueID)
	if since > 0 {
		db = db.Where("updated_unix >= ?", since)
	}
	comments := make([]*Comment, 0, 10)
	err := db.Order("created_unix ASC").Order("id ASC").Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, s.LoadAttributes(ctx, comments...)
}

// ListByRepoID returns the comments of all issues of the repository with
// given ID that are updated at or after given Unix time, oldest first, and
// their attributes.
func (s *CommentsStore) ListByRepoID(ctx context.Context, repoID, since int64) ([]*Comment, error) {
	db := s.db.WithContext(ctx).Select("comment.*").
		Joins("INNER JOIN issue ON issue.id = comment.issue_id").
		Where("issue.repo_id = ?", repoID)
	if since > 0 {
		db = db.Where("comment.updated_unix >= ?", since)
	}
	comments := make([]*Comment, 0, 10)
	err := db.Order("comment.created_unix ASC").Order("comment.id ASC").Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, s.LoadAttributes(ctx, comments...)
}

// Update saves all fields of the comment edited by the doer, whose content was
// oldContent, and prepares webhooks.
func (s *CommentsStore) Update(ctx context.Context, logger *logx.Logger, links LinkOptions, doer *User, c *Comment, oldContent string) error {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	err := s.db.WithContext(ctx).Model(c).Select("*").Updates(c).Error
	if err != nil {
		return err
	}

	if err = newIssuesStore(s.db).LoadAttributes(ctx, c.Issue); err != nil {
		logger.Error("Failed to load issue attributes", "issueID", c.IssueID, "error", err)
		return nil
	}
	err = newWebhooksStore(s.db).Prepare(ctx, logger, links.ExternalURL, c.Issue.Repo, HookEventTypeIssueComment, &v1types.WebhookIssueCommentPayload{
		Action:  v1types.WebhookIssueCommentEdited,
		Issue:   c.Issue.APIFormat(links),
		Comment: c.APIFormat(links),
		Changes: &v1types.WebhookChangesPayload{
			Body: &v1types.WebhookChangesFromPayload{From: oldContent},
		},
		Repository: c.Issue.Repo.APIFormatLegacy(logger, links, nil),
		Sender:     doer.APIFormat(links),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", c.ID, "error", err)
	}
	return nil
}

// DeleteByID deletes the comment with given ID and its attachments as the
// doer, and prepares webhooks. The files of the attachments are under the
// attachment path. Deleting a comment that does not exist is not an error.
func (s *CommentsStore) DeleteByID(ctx context.Context, logger *logx.Logger, links LinkOptions, attachmentPath string, doer *User, id int64) error {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	comment, err := s.GetByID(ctx, id)
	if err != nil {
		if IsErrCommentNotExist(err) {
			return nil
		}
		return err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Delete(new(Comment), comment.ID).Error
		if err != nil {
			return errors.Wrap(err, "delete comment")
		}
		if comment.Type == CommentTypeComment {
			return tx.Exec("UPDATE issue SET num_comments = num_comments - 1 WHERE id = ?", comment.IssueID).Error
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = newAttachmentsStore(s.db).DeleteByCommentID(ctx, attachmentPath, comment.ID, true)
	if err != nil {
		logger.Error("Failed to delete attachments of comment", "commentID", comment.ID, "error", err)
	}

	if err = newIssuesStore(s.db).LoadAttributes(ctx, comment.Issue); err != nil {
		logger.Error("Failed to load issue attributes", "issueID", comment.IssueID, "error", err)
		return nil
	}
	err = newWebhooksStore(s.db).Prepare(ctx, logger, links.ExternalURL, comment.Issue.Repo, HookEventTypeIssueComment, &v1types.WebhookIssueCommentPayload{
		Action:     v1types.WebhookIssueCommentDeleted,
		Issue:      comment.Issue.APIFormat(links),
		Comment:    comment.APIFormat(links),
		Repository: comment.Issue.Repo.APIFormatLegacy(logger, links, nil),
		Sender:     doer.APIFormat(links),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", comment.ID, "error", err)
	}
	return nil
}
