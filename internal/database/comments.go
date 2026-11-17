package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

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
func (s *CommentsStore) create(ctx context.Context, logger *logx.Logger, opts *CreateCommentOptions) (*Comment, error) {
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

	// Only comments and changes of status are announced.
	if act.OpType > 0 {
		if err = newActionsStore(s.db).notifyWatchers(ctx, act); err != nil {
			logger.Error("Failed to notify watchers", "error", err)
		}
		if err = s.mailParticipants(ctx, logger, comment, act.OpType, opts.Issue); err != nil {
			logger.Error("Failed to mail participants", "error", err)
		}
	}
	return comment, s.LoadAttributes(ctx, comment)
}

// mailParticipants marks the users the comment mentions as mentioned, and
// sends emails about the comment to them and to the participants of the
// issue.
func (s *CommentsStore) mailParticipants(ctx context.Context, logger *logx.Logger, c *Comment, opType ActionType, issue *Issue) error {
	mentions := findLowerMentions(c.Content)
	err := newIssuesStore(s.db).updateMentions(ctx, issue.RepoID, c.IssueID, mentions)
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
	if err = mailIssueCommentToParticipants(logger, s.db.WithContext(ctx), issue, c.Poster, mentions); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}
	return nil
}

// createStatus creates the comment that the issue is closed or reopened.
func (s *CommentsStore) createStatus(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue) (*Comment, error) {
	typ := CommentTypeClose
	if !issue.IsClosed {
		typ = CommentTypeReopen
	}
	return s.create(ctx, logger, &CreateCommentOptions{
		Type:  typ,
		Doer:  doer,
		Repo:  repo,
		Issue: issue,
	})
}

// Create creates a comment of given content on the issue of given repository
// as the doer, attaches the unattached attachments with given UUIDs to it, and
// prepares webhooks.
func (s *CommentsStore) Create(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, content string, uuids []string) (*Comment, error) {
	var comment *Comment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) (err error) {
		comment, err = newCommentsStore(tx).create(ctx, logger, &CreateCommentOptions{
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
	err = newWebhooksStore(s.db).Prepare(ctx, logger, repo, HookEventTypeIssueComment, &apiv1types.WebhookIssueCommentPayload{
		Action:     apiv1types.WebhookIssueCommentCreated,
		Issue:      issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: repo.APIFormatLegacy(logger, nil),
		Sender:     doer.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", comment.ID, "error", err)
	}
	return comment, nil
}

// createRef creates a comment on the issue that the commit with given SHA
// references it, unless the commit already did.
func (s *CommentsStore) createRef(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, content, commitSHA string) error {
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
		_, err := newCommentsStore(tx).create(ctx, logger, &CreateCommentOptions{
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

// Update saves all fields of the comment, whose content was given old
// content, as the doer, and prepares webhooks.
func (s *CommentsStore) Update(ctx context.Context, logger *logx.Logger, doer *User, c *Comment, oldContent string) error {
	err := s.db.WithContext(ctx).Model(c).Select("*").Updates(c).Error
	if err != nil {
		return err
	}

	if err = newIssuesStore(s.db).LoadAttributes(ctx, c.Issue); err != nil {
		logger.Error("Failed to load issue attributes", "issueID", c.IssueID, "error", err)
		return nil
	}
	err = newWebhooksStore(s.db).Prepare(ctx, logger, c.Issue.Repo, HookEventTypeIssueComment, &apiv1types.WebhookIssueCommentPayload{
		Action:  apiv1types.WebhookIssueCommentEdited,
		Issue:   c.Issue.APIFormat(),
		Comment: c.APIFormat(),
		Changes: &apiv1types.WebhookChangesPayload{
			Body: &apiv1types.WebhookChangesFromPayload{From: oldContent},
		},
		Repository: c.Issue.Repo.APIFormatLegacy(logger, nil),
		Sender:     doer.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", c.ID, "error", err)
	}
	return nil
}

// DeleteByID deletes the comment with given ID and its attachments as the
// doer, and prepares webhooks. Deleting a comment that does not exist is not
// an error.
func (s *CommentsStore) DeleteByID(ctx context.Context, logger *logx.Logger, doer *User, id int64) error {
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

	err = newAttachmentsStore(s.db).DeleteByCommentID(ctx, comment.ID, true)
	if err != nil {
		logger.Error("Failed to delete attachments of comment", "commentID", comment.ID, "error", err)
	}

	if err = newIssuesStore(s.db).LoadAttributes(ctx, comment.Issue); err != nil {
		logger.Error("Failed to load issue attributes", "issueID", comment.IssueID, "error", err)
		return nil
	}
	err = newWebhooksStore(s.db).Prepare(ctx, logger, comment.Issue.Repo, HookEventTypeIssueComment, &apiv1types.WebhookIssueCommentPayload{
		Action:     apiv1types.WebhookIssueCommentDeleted,
		Issue:      comment.Issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: comment.Issue.Repo.APIFormatLegacy(logger, nil),
		Sender:     doer.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "commentID", comment.ID, "error", err)
	}
	return nil
}
