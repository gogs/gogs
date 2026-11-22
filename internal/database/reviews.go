package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/api/v1types"
)

// ReviewState is the outcome of a review of a pull request.
type ReviewState int

const (
	// ReviewStatePending is a review that is not submitted yet, whose comments
	// only its reviewer can see.
	ReviewStatePending ReviewState = iota
	ReviewStateComment
	ReviewStateApprove
	ReviewStateRequestChanges
)

// Review is a review of a pull request. Its summary is the content of the
// comment of type CommentTypeReview that shows it in the timeline, and its code
// comments have its ID as their ReviewID.
type Review struct {
	ID         int64
	IssueID    int64       `gorm:"index:IDX_review_issue_id;not null"`
	ReviewerID int64       `gorm:"index:IDX_review_reviewer_id;not null"`
	State      ReviewState `gorm:"not null;default:0"`
	// CommitSHA is the head commit of the pull request when the review was
	// submitted.
	CommitSHA     string `gorm:"type:VARCHAR(64)"`
	CreatedUnix   int64
	SubmittedUnix int64 `gorm:"not null;default:0"`
}

// BeforeCreate implements the GORM create hook.
func (r *Review) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// ReviewRequest is an open request for a user to review a pull request. It is
// deleted when the user submits a review.
type ReviewRequest struct {
	ID          int64
	IssueID     int64 `gorm:"uniqueIndex:UQE_review_request_review_request;not null"`
	ReviewerID  int64 `gorm:"uniqueIndex:UQE_review_request_review_request;not null"`
	RequesterID int64 `gorm:"not null"`
	CreatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (r *ReviewRequest) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// ReviewsStore is the storage layer for reviews of pull requests, their code
// comments and review requests.
type ReviewsStore struct {
	db *gorm.DB
}

func newReviewsStore(db *gorm.DB) *ReviewsStore {
	return &ReviewsStore{db: db}
}

// ListSubmitted returns the submitted reviews of the pull request, oldest
// first.
func (s *ReviewsStore) ListSubmitted(ctx context.Context, issueID int64) ([]*Review, error) {
	reviews := make([]*Review, 0, 5)
	return reviews, s.db.WithContext(ctx).
		Where("issue_id = ? AND state <> ?", issueID, ReviewStatePending).
		Order("submitted_unix ASC").Order("id ASC").
		Find(&reviews).Error
}

// GetPending returns the review the reviewer has not submitted yet, or nil
// when there is none.
func (s *ReviewsStore) GetPending(ctx context.Context, issueID, reviewerID int64) (*Review, error) {
	review := new(Review)
	err := s.db.WithContext(ctx).
		Where("issue_id = ? AND reviewer_id = ? AND state = ?", issueID, reviewerID, ReviewStatePending).
		First(review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return review, nil
}

// CountPendingComments returns the number of comments of the pending review.
func (s *ReviewsStore) CountPendingComments(ctx context.Context, reviewID int64) (int64, error) {
	var count int64
	return count, s.db.WithContext(ctx).Model(new(Comment)).
		Where("review_id = ? AND is_pending = ?", reviewID, true).
		Count(&count).Error
}

// ListCodeComments returns the code comments of the pull request that the
// viewer can see, i.e., the submitted ones and the viewer's pending ones,
// oldest first.
func (s *ReviewsStore) ListCodeComments(ctx context.Context, issueID, viewerID int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	err := s.db.WithContext(ctx).
		Where("issue_id = ? AND type = ?", issueID, CommentTypeCode).
		Where("is_pending = ? OR poster_id = ?", false, viewerID).
		Order("created_unix ASC").Order("id ASC").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, newCommentsStore(s.db).LoadAttributes(ctx, comments...)
}

// CreateCodeCommentOptions describes a new thread of code comments on lines
// of the diff of a pull request.
type CreateCodeCommentOptions struct {
	Doer        *User
	Repo        *Repository
	Issue       *Issue
	Content     string
	Attachments []string

	TreePath  string
	Side      string
	StartSide string
	StartLine int64
	Line      int64
	// CommitSHA is the head commit and BaseSHA the merge base the lines are
	// in.
	CommitSHA string
	BaseSHA   string
	DiffHunk  string
	// Pending adds the comment to the doer's pending review, which is created
	// when there is none. Otherwise the comment is submitted at once as a
	// review of its own.
	Pending bool
}

// CreateCodeComment starts a thread of code comments.
func (s *ReviewsStore) CreateCodeComment(ctx context.Context, logger *logx.Logger, opts CreateCodeCommentOptions) (*Comment, error) {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	var comment *Comment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var review *Review
		var err error
		if opts.Pending {
			review, err = newReviewsStore(tx).getOrCreatePending(ctx, opts.Issue.ID, opts.Doer.ID)
			if err != nil {
				return err
			}
		} else {
			// Like on GitHub, a single comment is a review of its own, without a
			// summary.
			review = &Review{
				IssueID:       opts.Issue.ID,
				ReviewerID:    opts.Doer.ID,
				State:         ReviewStateComment,
				CommitSHA:     opts.CommitSHA,
				SubmittedUnix: tx.NowFunc().Unix(),
			}
			if err = tx.Create(review).Error; err != nil {
				return errors.Wrap(err, "create review")
			}
			// The code comment notifies about the review, so its timeline item
			// is inserted without notifying again.
			err = tx.Create(&Comment{
				Type:     CommentTypeReview,
				PosterID: opts.Doer.ID,
				IssueID:  opts.Issue.ID,
				ReviewID: review.ID,
			}).Error
			if err != nil {
				return errors.Wrap(err, "create review comment")
			}
		}

		comment, err = newCommentsStore(tx).create(ctx, logger, &CreateCommentOptions{
			Type:        CommentTypeCode,
			Doer:        opts.Doer,
			Repo:        opts.Repo,
			Issue:       opts.Issue,
			Content:     opts.Content,
			Attachments: opts.Attachments,
			CommitSHA:   opts.CommitSHA,
			LineNum:     opts.Line,
			ReviewID:    review.ID,
			IsPending:   opts.Pending,
			TreePath:    opts.TreePath,
			Side:        opts.Side,
			StartSide:   opts.StartSide,
			StartLine:   opts.StartLine,
			BaseSHA:     opts.BaseSHA,
			DiffHunk:    opts.DiffHunk,
		})
		return errors.Wrap(err, "create code comment")
	})
	return comment, err
}

func (s *ReviewsStore) getOrCreatePending(ctx context.Context, issueID, reviewerID int64) (*Review, error) {
	review, err := s.GetPending(ctx, issueID, reviewerID)
	if err != nil {
		return nil, errors.Wrap(err, "get pending review")
	} else if review != nil {
		return review, nil
	}
	review = &Review{IssueID: issueID, ReviewerID: reviewerID, State: ReviewStatePending}
	if err = s.db.WithContext(ctx).Create(review).Error; err != nil {
		return nil, errors.Wrap(err, "create pending review")
	}
	return review, nil
}

// CreateReply replies to the thread that the root comment starts. A reply to a
// code comment can be added to the doer's pending review.
func (s *ReviewsStore) CreateReply(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, root *Comment, content string, attachments []string, pending bool) (*Comment, error) {
	if root.ReplyToID != 0 || root.IssueID != issue.ID || (root.Type != CommentTypeComment && root.Type != CommentTypeCode) {
		return nil, errors.New("only the first comment of a thread can be replied to")
	}
	if root.Type != CommentTypeCode {
		pending = false
	}

	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	var comment *Comment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		opts := &CreateCommentOptions{
			Type:        root.Type,
			Doer:        doer,
			Repo:        repo,
			Issue:       issue,
			Content:     content,
			Attachments: attachments,
			ReplyToID:   root.ID,
			IsPending:   pending,
		}
		if pending {
			review, err := newReviewsStore(tx).getOrCreatePending(ctx, issue.ID, doer.ID)
			if err != nil {
				return err
			}
			opts.ReviewID = review.ID
		}
		var err error
		comment, err = newCommentsStore(tx).create(ctx, logger, opts)
		return errors.Wrap(err, "create reply")
	})
	if err != nil {
		return nil, err
	}

	// A reply to a comment is a comment to integrations. Code comments send no
	// webhooks, as when a thread starts.
	if comment.Type == CommentTypeComment {
		comment.Issue = issue
		err = newWebhooksStore(s.db).Prepare(ctx, logger, repo, HookEventTypeIssueComment, &v1types.WebhookIssueCommentPayload{
			Action:     v1types.WebhookIssueCommentCreated,
			Issue:      issue.APIFormat(),
			Comment:    comment.APIFormat(),
			Repository: repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
		if err != nil {
			logger.Error("Failed to prepare webhooks", "commentID", comment.ID, "error", err)
		}
	}
	return comment, nil
}

// Submit submits the reviewer's pending review with its comments, or a new
// review without comments when there is none.
func (s *ReviewsStore) Submit(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, state ReviewState, content, commitSHA string) (*Review, error) {
	if state == ReviewStatePending {
		return nil, errors.New("a review cannot be submitted as pending")
	}

	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	var review *Review
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		review, err = newReviewsStore(tx).getOrCreatePending(ctx, issue.ID, doer.ID)
		if err != nil {
			return err
		}
		now := tx.NowFunc().Unix()
		review.State = state
		review.CommitSHA = commitSHA
		review.SubmittedUnix = now
		err = tx.Model(review).Select("state", "commit_sha", "submitted_unix").Updates(review).Error
		if err != nil {
			return errors.Wrap(err, "update review")
		}

		// The comments show from when they were submitted, like on GitHub.
		res := tx.Exec("UPDATE comment SET is_pending = ?, created_unix = ?, updated_unix = ? WHERE review_id = ? AND is_pending = ?",
			false, now, now, review.ID, true)
		if res.Error != nil {
			return errors.Wrap(res.Error, "submit comments")
		}
		if res.RowsAffected > 0 {
			err = tx.Exec("UPDATE issue SET num_comments = num_comments + ? WHERE id = ?", res.RowsAffected, issue.ID).Error
			if err != nil {
				return errors.Wrap(err, "count comments")
			}
		}
		err = tx.Where("issue_id = ? AND reviewer_id = ?", issue.ID, doer.ID).Delete(new(ReviewRequest)).Error
		if err != nil {
			return errors.Wrap(err, "delete review request")
		}
		_, err = newCommentsStore(tx).create(ctx, logger, &CreateCommentOptions{
			Type:     CommentTypeReview,
			Doer:     doer,
			Repo:     repo,
			Issue:    issue,
			Content:  content,
			ReviewID: review.ID,
		})
		return errors.Wrap(err, "create review comment")
	})
	return review, err
}

// DiscardPending deletes the reviewer's pending review and its comments.
func (s *ReviewsStore) DiscardPending(ctx context.Context, issueID, reviewerID int64) error {
	review, err := s.GetPending(ctx, issueID, reviewerID)
	if err != nil {
		return errors.Wrap(err, "get pending review")
	} else if review == nil {
		return nil
	}

	var comments []*Comment
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("review_id = ? AND is_pending = ?", review.ID, true).Find(&comments).Error
		if err != nil {
			return errors.Wrap(err, "find pending comments")
		}
		for _, c := range comments {
			// Replies the reviewer has not submitted cannot have replies.
			err := tx.Where("id = ? OR reply_to_id = ?", c.ID, c.ID).Delete(new(Comment)).Error
			if err != nil {
				return errors.Wrap(err, "delete comment")
			}
		}
		return errors.Wrap(tx.Delete(new(Review), review.ID).Error, "delete review")
	})
	if err != nil {
		return err
	}
	for _, c := range comments {
		if err = newAttachmentsStore(s.db).DeleteByCommentID(ctx, c.ID, true); err != nil {
			return errors.Wrapf(err, "delete attachments of comment %d", c.ID)
		}
	}
	return nil
}

// SetThreadResolved resolves or unresolves the thread that the root comment
// starts.
func (s *ReviewsStore) SetThreadResolved(ctx context.Context, root *Comment, doerID int64, resolved bool) error {
	if resolved {
		root.ResolvedByID, root.ResolvedUnix = doerID, s.db.NowFunc().Unix()
	} else {
		root.ResolvedByID, root.ResolvedUnix = 0, 0
	}
	// UpdateColumns leaves updated_unix alone, since resolving does not edit
	// the comment.
	return s.db.WithContext(ctx).Model(&Comment{ID: root.ID}).UpdateColumns(map[string]any{
		"resolved_by_id": root.ResolvedByID,
		"resolved_unix":  root.ResolvedUnix,
	}).Error
}

// ListRequests returns the open review requests of the pull request, oldest
// first.
func (s *ReviewsStore) ListRequests(ctx context.Context, issueID int64) ([]*ReviewRequest, error) {
	requests := make([]*ReviewRequest, 0, 5)
	return requests, s.db.WithContext(ctx).Where("issue_id = ?", issueID).Order("id ASC").Find(&requests).Error
}

// SetRequested asks the reviewer to review the pull request when requested is
// true, or withdraws the request, which shows in its timeline. It does nothing
// when the request is already open or withdrawn.
func (s *ReviewsStore) SetRequested(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, reviewer *User, requested bool) error {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		err := tx.Model(new(ReviewRequest)).Where("issue_id = ? AND reviewer_id = ?", issue.ID, reviewer.ID).Count(&count).Error
		if err != nil {
			return errors.Wrap(err, "check review request")
		} else if (count > 0) == requested {
			return nil
		}

		typ := CommentTypeReviewRequest
		if requested {
			err = tx.Create(&ReviewRequest{IssueID: issue.ID, ReviewerID: reviewer.ID, RequesterID: doer.ID}).Error
		} else {
			typ = CommentTypeReviewRequestRemoved
			err = tx.Where("issue_id = ? AND reviewer_id = ?", issue.ID, reviewer.ID).Delete(new(ReviewRequest)).Error
		}
		if err != nil {
			return errors.Wrap(err, "change review request")
		}
		_, err = newCommentsStore(tx).create(ctx, logger, &CreateCommentOptions{
			Type:         typ,
			Doer:         doer,
			Repo:         repo,
			Issue:        issue,
			TargetUserID: reviewer.ID,
		})
		return errors.Wrap(err, "create timeline event")
	})
}
