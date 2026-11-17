package database

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"
	"xorm.io/xorm"
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
	IssueID    int64       `xorm:"INDEX NOT NULL"`
	ReviewerID int64       `xorm:"INDEX NOT NULL"`
	State      ReviewState `xorm:"NOT NULL DEFAULT 0"`
	// CommitSHA is the head commit of the pull request when the review was
	// submitted.
	CommitSHA     string `xorm:"VARCHAR(64)"`
	CreatedUnix   int64
	SubmittedUnix int64 `xorm:"NOT NULL DEFAULT 0"`
}

func (r *Review) BeforeInsert() {
	r.CreatedUnix = time.Now().Unix()
}

// ReviewRequest is an open request for a user to review a pull request. It is
// deleted when the user submits a review.
type ReviewRequest struct {
	ID          int64
	IssueID     int64 `xorm:"UNIQUE(review_request) NOT NULL"`
	ReviewerID  int64 `xorm:"UNIQUE(review_request) NOT NULL"`
	RequesterID int64 `xorm:"NOT NULL"`
	CreatedUnix int64
}

func (r *ReviewRequest) BeforeInsert() {
	r.CreatedUnix = time.Now().Unix()
}

// GetSubmittedReviewsByIssueID returns the submitted reviews of the pull
// request, oldest first.
func GetSubmittedReviewsByIssueID(issueID int64) ([]*Review, error) {
	reviews := make([]*Review, 0, 5)
	return reviews, x.Where("issue_id = ? AND state <> ?", issueID, ReviewStatePending).
		Asc("submitted_unix").Asc("id").Find(&reviews)
}

func getPendingReview(e Engine, issueID, reviewerID int64) (*Review, error) {
	review := new(Review)
	has, err := e.Where("issue_id = ? AND reviewer_id = ? AND state = ?", issueID, reviewerID, ReviewStatePending).Get(review)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, nil
	}
	return review, nil
}

// GetPendingReview returns the review the reviewer has not submitted yet, or
// nil when there is none.
func GetPendingReview(issueID, reviewerID int64) (*Review, error) {
	return getPendingReview(x, issueID, reviewerID)
}

// CountPendingReviewComments returns the number of comments of the pending
// review.
func CountPendingReviewComments(reviewID int64) (int64, error) {
	return x.Where("review_id = ? AND is_pending = ?", reviewID, true).Count(new(Comment))
}

// GetCodeCommentsByIssueID returns the code comments of the pull request that
// the viewer can see, i.e., the submitted ones and the viewer's pending ones,
// oldest first.
func GetCodeCommentsByIssueID(issueID, viewerID int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	err := x.Where("issue_id = ? AND type = ?", issueID, CommentTypeCode).
		And("is_pending = ? OR poster_id = ?", false, viewerID).
		Asc("created_unix").Asc("id").Find(&comments)
	if err != nil {
		return nil, err
	}
	return comments, loadCommentsAttributes(x, comments)
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
func CreateCodeComment(logger *logx.Logger, opts CreateCodeCommentOptions) (*Comment, error) {
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return nil, err
	}

	var review *Review
	var err error
	if opts.Pending {
		review, err = getOrCreatePendingReview(sess, opts.Issue.ID, opts.Doer.ID)
		if err != nil {
			return nil, err
		}
	} else {
		// Like on GitHub, a single comment is a review of its own, without a
		// summary.
		now := time.Now().Unix()
		review = &Review{
			IssueID:       opts.Issue.ID,
			ReviewerID:    opts.Doer.ID,
			State:         ReviewStateComment,
			CommitSHA:     opts.CommitSHA,
			SubmittedUnix: now,
		}
		if _, err = sess.Insert(review); err != nil {
			return nil, errors.Wrap(err, "create review")
		}
		// The code comment notifies about the review, so its timeline item is
		// inserted without notifying again.
		if _, err = sess.Insert(&Comment{
			Type:     CommentTypeReview,
			PosterID: opts.Doer.ID,
			IssueID:  opts.Issue.ID,
			ReviewID: review.ID,
		}); err != nil {
			return nil, errors.Wrap(err, "create review comment")
		}
	}

	comment, err := createComment(logger, sess, &CreateCommentOptions{
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
	if err != nil {
		return nil, errors.Wrap(err, "create code comment")
	}
	return comment, sess.Commit()
}

func getOrCreatePendingReview(e *xorm.Session, issueID, reviewerID int64) (*Review, error) {
	review, err := getPendingReview(e, issueID, reviewerID)
	if err != nil {
		return nil, errors.Wrap(err, "get pending review")
	} else if review != nil {
		return review, nil
	}
	review = &Review{IssueID: issueID, ReviewerID: reviewerID, State: ReviewStatePending}
	if _, err = e.Insert(review); err != nil {
		return nil, errors.Wrap(err, "create pending review")
	}
	return review, nil
}

// CreateReply replies to the thread that the root comment starts. A reply to a
// code comment can be added to the doer's pending review.
func CreateReply(logger *logx.Logger, doer *User, repo *Repository, issue *Issue, root *Comment, content string, attachments []string, pending bool) (*Comment, error) {
	if root.ReplyToID != 0 || root.IssueID != issue.ID || (root.Type != CommentTypeComment && root.Type != CommentTypeCode) {
		return nil, errors.New("only the first comment of a thread can be replied to")
	}
	if root.Type != CommentTypeCode {
		pending = false
	}

	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return nil, err
	}
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
		review, err := getOrCreatePendingReview(sess, issue.ID, doer.ID)
		if err != nil {
			return nil, err
		}
		opts.ReviewID = review.ID
	}
	comment, err := createComment(logger, sess, opts)
	if err != nil {
		return nil, errors.Wrap(err, "create reply")
	}
	return comment, sess.Commit()
}

// SubmitReview submits the reviewer's pending review with its comments, or a
// new review without comments when there is none.
func SubmitReview(logger *logx.Logger, doer *User, repo *Repository, issue *Issue, state ReviewState, content, commitSHA string) (*Review, error) {
	if state == ReviewStatePending {
		return nil, errors.New("a review cannot be submitted as pending")
	}
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return nil, err
	}

	review, err := getOrCreatePendingReview(sess, issue.ID, doer.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	review.State = state
	review.CommitSHA = commitSHA
	review.SubmittedUnix = now
	if _, err = sess.ID(review.ID).Cols("state", "commit_sha", "submitted_unix").Update(review); err != nil {
		return nil, errors.Wrap(err, "update review")
	}

	// The comments show from when they were submitted, like on GitHub.
	res, err := sess.Exec("UPDATE `comment` SET is_pending = ?, created_unix = ?, updated_unix = ? WHERE review_id = ? AND is_pending = ?",
		false, now, now, review.ID, true)
	if err != nil {
		return nil, errors.Wrap(err, "submit comments")
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err = sess.Exec("UPDATE `issue` SET num_comments = num_comments + ? WHERE id = ?", n, issue.ID); err != nil {
			return nil, errors.Wrap(err, "count comments")
		}
	}
	if _, err = sess.Delete(&ReviewRequest{IssueID: issue.ID, ReviewerID: doer.ID}); err != nil {
		return nil, errors.Wrap(err, "delete review request")
	}
	if _, err = createComment(logger, sess, &CreateCommentOptions{
		Type:     CommentTypeReview,
		Doer:     doer,
		Repo:     repo,
		Issue:    issue,
		Content:  content,
		ReviewID: review.ID,
	}); err != nil {
		return nil, errors.Wrap(err, "create review comment")
	}
	return review, sess.Commit()
}

// DiscardPendingReview deletes the reviewer's pending review and its comments.
func DiscardPendingReview(issueID, reviewerID int64) error {
	review, err := GetPendingReview(issueID, reviewerID)
	if err != nil {
		return errors.Wrap(err, "get pending review")
	} else if review == nil {
		return nil
	}

	var comments []*Comment
	if err = x.Where("review_id = ? AND is_pending = ?", review.ID, true).Find(&comments); err != nil {
		return errors.Wrap(err, "find pending comments")
	}
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}
	for _, c := range comments {
		// Replies the reviewer has not submitted cannot have replies.
		if _, err = sess.Where("id = ? OR reply_to_id = ?", c.ID, c.ID).Delete(new(Comment)); err != nil {
			return errors.Wrap(err, "delete comment")
		}
	}
	if _, err = sess.ID(review.ID).Delete(new(Review)); err != nil {
		return errors.Wrap(err, "delete review")
	}
	if err = sess.Commit(); err != nil {
		return err
	}
	for _, c := range comments {
		if err = Handle.Attachments().DeleteByCommentID(context.TODO(), c.ID, true); err != nil {
			return errors.Wrapf(err, "delete attachments of comment %d", c.ID)
		}
	}
	return nil
}

// SetThreadResolved resolves or unresolves the thread that the root comment
// starts.
func SetThreadResolved(root *Comment, doerID int64, resolved bool) error {
	if resolved {
		root.ResolvedByID, root.ResolvedUnix = doerID, time.Now().Unix()
	} else {
		root.ResolvedByID, root.ResolvedUnix = 0, 0
	}
	_, err := x.ID(root.ID).Cols("resolved_by_id", "resolved_unix").Update(root)
	return err
}

// GetReviewRequestsByIssueID returns the open review requests of the pull
// request, oldest first.
func GetReviewRequestsByIssueID(issueID int64) ([]*ReviewRequest, error) {
	requests := make([]*ReviewRequest, 0, 5)
	return requests, x.Where("issue_id = ?", issueID).Asc("id").Find(&requests)
}

// RequestReview asks the reviewer to review the pull request, which shows in
// its timeline. It does nothing when the request is open already.
func RequestReview(logger *logx.Logger, doer *User, repo *Repository, issue *Issue, reviewer *User) error {
	return changeReviewRequest(logger, doer, repo, issue, reviewer, true)
}

// RemoveReviewRequest withdraws the request for the reviewer to review the
// pull request. It does nothing when there is no open request.
func RemoveReviewRequest(logger *logx.Logger, doer *User, repo *Repository, issue *Issue, reviewer *User) error {
	return changeReviewRequest(logger, doer, repo, issue, reviewer, false)
}

func changeReviewRequest(logger *logx.Logger, doer *User, repo *Repository, issue *Issue, reviewer *User, requested bool) error {
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	has, err := sess.Exist(&ReviewRequest{IssueID: issue.ID, ReviewerID: reviewer.ID})
	if err != nil {
		return errors.Wrap(err, "check review request")
	} else if has == requested {
		return nil
	}

	typ := CommentTypeReviewRequest
	if requested {
		_, err = sess.Insert(&ReviewRequest{IssueID: issue.ID, ReviewerID: reviewer.ID, RequesterID: doer.ID})
	} else {
		typ = CommentTypeReviewRequestRemoved
		_, err = sess.Delete(&ReviewRequest{IssueID: issue.ID, ReviewerID: reviewer.ID})
	}
	if err != nil {
		return errors.Wrap(err, "change review request")
	}
	if _, err = createComment(logger, sess, &CreateCommentOptions{
		Type:         typ,
		Doer:         doer,
		Repo:         repo,
		Issue:        issue,
		TargetUserID: reviewer.ID,
	}); err != nil {
		return errors.Wrap(err, "create timeline event")
	}
	return sess.Commit()
}
