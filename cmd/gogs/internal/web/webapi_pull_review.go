package web

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
)

// pullReviewer is the state of a user's review of a pull request.
type pullReviewer struct {
	User issueUser `json:"user"`
	// State is "requested" while a requested review is not submitted, or the
	// latest of "approved" and "changes_requested", or "commented".
	State string `json:"state"`
	// Stale marks an approval of an older head commit.
	Stale bool `json:"stale,omitempty"`
	// Counts is whether the review counts toward merging, i.e., the reviewer
	// can write to the repository.
	Counts bool `json:"counts"`
}

// pullReviewStatus is what the reviews of a pull request decide.
type pullReviewStatus struct {
	Reviewers []pullReviewer `json:"reviewers"`
	// Decision is "approved", "changes_requested", "review_required", or
	// empty when nothing is decided.
	Decision string `json:"reviewDecision,omitempty"`
	// RequiredApprovals is how many approvals the protection of the base
	// branch requires.
	RequiredApprovals int `json:"requiredApprovals,omitempty"`
	// Approvals is how many approvals count toward RequiredApprovals.
	Approvals int `json:"approvals"`
	// MergeBlockers are why the pull request cannot merge yet: "approvals",
	// "changes_requested" and "unresolved_conversations".
	MergeBlockers []string `json:"mergeBlockers"`
	// ViewerPendingReview counts the comments of the viewer's review that is
	// not submitted yet.
	ViewerPendingReview *pendingReviewInfo `json:"viewerPendingReview,omitempty"`
	// ReviewerOptions are the users a review can be requested from, only
	// sent to viewers who can request reviews.
	ReviewerOptions []issueUser `json:"reviewerOptions,omitempty"`
}

type pendingReviewInfo struct {
	CommentCount int64 `json:"commentCount"`
}

// describeReviews decides the reviews of the pull request at the head commit,
// against the protection of its base branch. The threads are those the
// viewer can see.
func describeReviews(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, head string, threads []issueComment) (*pullReviewStatus, error) {
	repo := repoCtx.Repo
	status := &pullReviewStatus{Reviewers: []pullReviewer{}, MergeBlockers: []string{}}
	reviews, err := database.Handle.Reviews().ListSubmitted(ctx, issue.ID)
	if err != nil {
		return nil, errors.Wrap(err, "list reviews")
	}
	requests, err := database.Handle.Reviews().ListRequests(ctx, issue.ID)
	if err != nil {
		return nil, errors.Wrap(err, "list review requests")
	}
	protect, err := database.Handle.ProtectBranches().GetByName(ctx, repo.ID, issue.PullRequest.BaseBranch)
	if err != nil && !database.IsErrBranchNotExist(err) {
		return nil, errors.Wrap(err, "get branch protection")
	}

	type state struct {
		userID    int64
		state     string
		commitSHA string
	}
	var order []int64
	latest := make(map[int64]*state)
	for _, r := range reviews {
		s, ok := latest[r.ReviewerID]
		if !ok {
			s = &state{userID: r.ReviewerID, state: "commented"}
			latest[r.ReviewerID] = s
			order = append(order, r.ReviewerID)
		}
		// A comment does not change an earlier approval or request for
		// changes.
		switch r.State {
		case database.ReviewStateApprove:
			s.state, s.commitSHA = "approved", r.CommitSHA
		case database.ReviewStateRequestChanges:
			s.state, s.commitSHA = "changes_requested", r.CommitSHA
		}
	}
	requested := make(map[int64]bool, len(requests))
	for _, r := range requests {
		requested[r.ReviewerID] = true
		if _, ok := latest[r.ReviewerID]; !ok {
			latest[r.ReviewerID] = &state{userID: r.ReviewerID}
			order = append(order, r.ReviewerID)
		}
	}

	dismissStale := protect != nil && protect.Protected && protect.DismissStaleApprovals
	approvals, changesRequested := 0, false
	for _, id := range order {
		s := latest[id]
		user, err := database.Handle.Users().GetByID(ctx, id)
		if err != nil {
			if !database.IsErrUserNotExist(err) {
				return nil, errors.Wrap(err, "get reviewer")
			}
			continue
		}
		r := pullReviewer{
			User:   toIssueUser(user),
			State:  s.state,
			Stale:  s.state == "approved" && s.commitSHA != head,
			Counts: id != issue.PosterID && canWriteRepo(ctx, logger, repo, id),
		}
		if requested[id] {
			r.State = "requested"
		}
		if r.Counts {
			switch s.state {
			case "approved":
				if !r.Stale || !dismissStale {
					approvals++
				}
			case "changes_requested":
				changesRequested = true
			}
		}
		status.Reviewers = append(status.Reviewers, r)
	}

	status.Approvals = approvals
	if protect != nil && protect.Protected && protect.RequiredApprovals > 0 {
		status.RequiredApprovals = protect.RequiredApprovals
		if approvals < protect.RequiredApprovals {
			status.MergeBlockers = append(status.MergeBlockers, "approvals")
		}
		if changesRequested {
			status.MergeBlockers = append(status.MergeBlockers, "changes_requested")
		}
		if protect.RequireResolvedConversations && slices.ContainsFunc(threads, func(t issueComment) bool {
			return !t.Pending && t.Thread.ResolvedBy == nil
		}) {
			status.MergeBlockers = append(status.MergeBlockers, "unresolved_conversations")
		}
	}
	switch {
	case changesRequested:
		status.Decision = "changes_requested"
	case status.RequiredApprovals > 0 && approvals < status.RequiredApprovals:
		status.Decision = "review_required"
	case approvals > 0:
		status.Decision = "approved"
	}

	if viewer != nil {
		pending, err := database.Handle.Reviews().GetPending(ctx, issue.ID, viewer.ID)
		if err != nil {
			return nil, errors.Wrap(err, "get pending review")
		}
		if pending != nil {
			n, err := database.Handle.Reviews().CountPendingComments(ctx, pending.ID)
			if err != nil {
				return nil, errors.Wrap(err, "count pending comments")
			}
			// A pending review whose comments were all deleted is not one the
			// viewer is still writing.
			if n > 0 {
				status.ViewerPendingReview = &pendingReviewInfo{CommentCount: n}
			}
		}
		if viewerCanRequestReviews(repoCtx) {
			if status.ReviewerOptions, err = listReviewerOptions(ctx, repo, issue); err != nil {
				return nil, errors.Wrap(err, "list reviewer options")
			}
		}
	}
	return status, nil
}

func canWriteRepo(ctx context.Context, logger *logx.Logger, repo *database.Repository, userID int64) bool {
	return database.Handle.Permissions().Authorize(ctx, logger, userID, repo.ID, database.AccessModeWrite,
		database.AccessModeOptions{OwnerID: repo.OwnerID, Private: repo.IsPrivate})
}

// viewerCanRequestReviews returns whether the viewer can ask others to review
// the pull request: writers, like for assigning, since the options list who
// can write to the repository.
func viewerCanRequestReviews(repoCtx *repoContext) bool {
	return repoCtx.Viewer.CanWrite()
}

// listReviewerOptions returns the users who can be asked to review the pull
// request: those who can write to the repository, except who opened it.
func listReviewerOptions(ctx context.Context, repo *database.Repository, issue *database.Issue) ([]issueUser, error) {
	users, err := database.Handle.Repositories().ListWriters(ctx, repo)
	if err != nil {
		return nil, err
	}
	opts := make([]issueUser, 0, len(users))
	for _, u := range users {
		if u.ID != issue.PosterID {
			opts = append(opts, toIssueUser(u))
		}
	}
	return opts, nil
}

// describeThreads describes the threads of code comments of the pull request
// that the viewer can see, with their lines in the diff between base and head
// of the given repository. A thread whose lines changed since it started is
// outdated, and keeps the lines it started on.
func describeThreads(ctx context.Context, cv *commentViewer, gitRepo *git.Repository, base, head string) ([]issueComment, error) {
	issue := cv.issue
	viewerID := int64(0)
	if cv.viewer != nil {
		viewerID = cv.viewer.ID
	}
	comments, err := database.Handle.Reviews().ListCodeComments(ctx, issue.ID, viewerID)
	if err != nil {
		return nil, errors.Wrap(err, "list code comments")
	}
	canResolve := cv.viewer != nil && viewerCanEditIssue(cv.repoCtx, issue)

	// Diffs are shared by the threads on the same file at the same commits.
	type diffKey struct{ from, to, path string }
	diffs := make(map[diffKey][]*git.DiffHunk)
	mapLines := func(from, to, path string, start, end int64) (int64, int64, bool) {
		if from == to {
			return start, end, true
		}
		if gitRepo == nil {
			return 0, 0, false
		}
		key := diffKey{from, to, path}
		hunks, ok := diffs[key]
		if !ok {
			hunks, err = gitRepo.DiffFileHunks(ctx, from, to, "", path, 0)
			if err != nil {
				// The commit may be gone, e.g., after a force push.
				cv.logger.Debug("Failed to diff thread lines", "from", from, "to", to, "path", path, "error", err)
				hunks = nil
			}
			diffs[key] = hunks
			if err != nil {
				return 0, 0, false
			}
		}
		s, e, ok := git.MapLineRange(hunks, int(start), int(end))
		return int64(s), int64(e), ok
	}

	threads := []issueComment{}
	index := make(map[int64]int)
	for _, c := range comments {
		if c.ReplyToID > 0 {
			if i, ok := index[c.ReplyToID]; ok {
				threads[i].Replies = append(threads[i].Replies, cv.toItem(c))
			}
			continue
		}
		item := cv.toItem(c)
		t := &issueThread{
			ReviewID:         c.ReviewID,
			Path:             c.TreePath,
			Side:             c.Side,
			StartSide:        c.StartSide,
			StartLine:        c.StartLine,
			Line:             c.Line,
			DiffHunk:         c.DiffHunk,
			ViewerCanResolve: canResolve && !c.IsPending,
		}
		if c.ResolvedByID > 0 {
			resolver, err := database.Handle.Users().GetByID(ctx, c.ResolvedByID)
			if err != nil {
				if !database.IsErrUserNotExist(err) {
					return nil, errors.Wrap(err, "get resolver")
				}
				resolver = database.NewGhostUser()
			}
			u := toIssueUser(resolver)
			at := time.Unix(c.ResolvedUnix, 0).UTC()
			t.ResolvedBy, t.ResolvedAt = &u, &at
		}

		// The lines of a range are mapped on each side they are on.
		from, to := c.CommitSHA, head
		if c.Side == "old" {
			from, to = c.BaseSHA, base
		}
		startFrom, startTo := c.CommitSHA, head
		if c.StartSide == "old" {
			startFrom, startTo = c.BaseSHA, base
		}
		if c.Side == c.StartSide {
			if s, e, ok := mapLines(from, to, c.TreePath, c.StartLine, c.Line); ok {
				t.StartLine, t.Line = s, e
			} else {
				t.Outdated = true
			}
		} else {
			s, _, okStart := mapLines(startFrom, startTo, c.TreePath, c.StartLine, c.StartLine)
			_, e, okEnd := mapLines(from, to, c.TreePath, c.Line, c.Line)
			if okStart && okEnd {
				t.StartLine, t.Line = s, e
			} else {
				t.Outdated = true
			}
		}
		item.Thread = t
		index[c.ID] = len(threads)
		threads = append(threads, item)
	}
	return threads, nil
}

// requireHeadSHA returns the response for a change made on another head
// commit than the current one, which the viewer has not seen.
func requireHeadSHA(l i18n.Locale, rng *pullRange, headSHA string) (statusCode int, resp any) {
	if rng == nil {
		return http.StatusNotFound, &bindingErrorResponse{Error: l.Tr("repo::pulls.data_broken")}
	}
	if headSHA != rng.Head {
		return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.head_changed")}
	}
	return 0, nil
}

type repoPullCodeCommentRequest struct {
	Path string `json:"path" validate:"required"`
	// OldPath is the path of the file at the base when the pull request
	// renamed it.
	OldPath   string `json:"oldPath"`
	Side      string `json:"side" validate:"oneof=old new"`
	StartSide string `json:"startSide" validate:"oneof=old new"`
	StartLine int64  `json:"startLine" validate:"min=1"`
	Line      int64  `json:"line" validate:"min=1"`
	HeadSHA   string `json:"headSHA" validate:"required"`
	Content   string `json:"content"`
	// Attachments are the UUIDs of the uploaded files to attach.
	Attachments []string `json:"attachments"`
	// Pending adds the comment to the viewer's pending review instead of
	// submitting it at once.
	Pending bool `json:"pending"`
}

// postRepoPullCodeComment starts a thread on lines of the diff of the pull
// request.
func postRepoPullCodeComment(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoPullCodeCommentRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		msg := l.Tr("form::Content") + l.Tr("form::require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if statusCode, err = checkNewAttachments(ctx, issueAttachmentLimits(), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check new attachments", "error", err)
		}
		return statusCode, nil, err
	}
	if issue.PullRequest.HasMerged || issue.IsClosed {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.closed")}, nil
	}
	_, rng, _, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if statusCode, resp := requireHeadSHA(l, rng, req.HeadSHA); statusCode != 0 {
		return statusCode, resp, nil
	}

	// Comments are only on the lines the diff shows, which are in its hunks.
	hunks, err := rng.GitRepo.DiffFileHunks(ctx, rng.Base, rng.Head, req.OldPath, req.Path, 3)
	if err != nil {
		logger.Error("Failed to diff file", "path", req.Path, "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "diff file")
	}
	end := git.HunkContaining(hunks, req.Side == "old", int(req.Line), int(req.Line))
	start := git.HunkContaining(hunks, req.StartSide == "old", int(req.StartLine), int(req.StartLine))
	if end == nil || start != end || (req.Side == req.StartSide && req.StartLine > req.Line) {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.line_not_in_diff")}, nil
	}

	comment, err := database.Handle.Reviews().CreateCodeComment(ctx, logger, database.CreateCodeCommentOptions{
		Doer:        viewer,
		Repo:        repoCtx.Repo,
		Issue:       issue,
		Content:     req.Content,
		Attachments: req.Attachments,
		TreePath:    req.Path,
		Side:        req.Side,
		StartSide:   req.StartSide,
		StartLine:   req.StartLine,
		Line:        req.Line,
		CommitSHA:   rng.Head,
		BaseSHA:     rng.Base,
		DiffHunk:    strings.Join(end.LinesEndingAt(req.Side == "old", int(req.Line), 4), "\n"),
		Pending:     req.Pending,
	})
	if err != nil {
		logger.Error("Failed to create code comment", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create code comment")
	}
	return http.StatusCreated, map[string]int64{"id": comment.ID}, nil
}

type repoPullReviewRequest struct {
	// State is "comment", "approve" or "request_changes".
	State   string `json:"state" validate:"oneof=comment approve request_changes"`
	Content string `json:"content"`
	HeadSHA string `json:"headSHA" validate:"required"`
}

// postRepoPullReview submits the viewer's review of the pull request.
func postRepoPullReview(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoPullReviewRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	state := map[string]database.ReviewState{
		"comment":         database.ReviewStateComment,
		"approve":         database.ReviewStateApprove,
		"request_changes": database.ReviewStateRequestChanges,
	}[req.State]
	if state != database.ReviewStateComment && issue.IsPoster(viewer.ID) {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.own_pull")}, nil
	}
	if issue.PullRequest.HasMerged || issue.IsClosed {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo::pulls.review.closed")}, nil
	}
	_, rng, _, err := describePull(ctx, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe pull request", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if statusCode, resp := requireHeadSHA(l, rng, req.HeadSHA); statusCode != 0 {
		return statusCode, resp, nil
	}
	if state == database.ReviewStateComment && strings.TrimSpace(req.Content) == "" {
		pending, err := database.Handle.Reviews().GetPending(ctx, issue.ID, viewer.ID)
		if err != nil {
			logger.Error("Failed to get pending review", "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get pending review")
		}
		n := int64(0)
		if pending != nil {
			if n, err = database.Handle.Reviews().CountPendingComments(ctx, pending.ID); err != nil {
				logger.Error("Failed to count pending comments", "reviewID", pending.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "count pending comments")
			}
		}
		if n == 0 {
			msg := l.Tr("form::Content") + l.Tr("form::require_error")
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
		}
	}

	if _, err = database.Handle.Reviews().Submit(ctx, logger, viewer, repoCtx.Repo, issue, state, req.Content, rng.Head); err != nil {
		logger.Error("Failed to submit review", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "submit review")
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoPullPendingReview discards the viewer's pending review.
func deleteRepoPullPendingReview(c flamego.Context, logger *logx.Logger, viewer *database.User, issue *database.Issue) (statusCode int, resp any, err error) {
	if err = database.Handle.Reviews().DiscardPending(c.Request().Context(), issue.ID, viewer.ID); err != nil {
		logger.Error("Failed to discard pending review", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "discard pending review")
	}
	return http.StatusNoContent, nil, nil
}

// withThreadRoot injects the first comment of a thread of code comments of
// the pull request, which writers and whoever opened it can resolve.
func withThreadRoot(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, issue *database.Issue) {
	w := c.ResponseWriter()
	comment, err := database.Handle.Comments().GetByID(c.Request().Context(), idParam(c))
	if err != nil {
		if database.IsErrCommentNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("thread does not exist"))
			return
		}
		logger.Error("Failed to get comment", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get comment"))
		return
	}
	if comment.IssueID != issue.ID || comment.Type != database.CommentTypeCode || comment.ReplyToID != 0 || comment.IsPending {
		writeErrorResponse(w, http.StatusNotFound, errors.New("thread does not exist"))
		return
	}
	if !viewerCanEditIssue(repoCtx, issue) {
		writeErrorResponse(w, http.StatusForbidden, errors.New("only writers and the poster can resolve threads"))
		return
	}
	c.Map(comment)
}

func setRepoPullThreadResolved(c flamego.Context, logger *logx.Logger, viewer *database.User, comment *database.Comment, resolved bool) (statusCode int, resp any, err error) {
	if err = database.Handle.Reviews().SetThreadResolved(c.Request().Context(), comment, viewer.ID, resolved); err != nil {
		logger.Error("Failed to change thread resolution", "commentID", comment.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change thread resolution")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoPullThreadResolved(c flamego.Context, logger *logx.Logger, viewer *database.User, comment *database.Comment) (statusCode int, resp any, err error) {
	return setRepoPullThreadResolved(c, logger, viewer, comment, true)
}

func deleteRepoPullThreadResolved(c flamego.Context, logger *logx.Logger, viewer *database.User, comment *database.Comment) (statusCode int, resp any, err error) {
	return setRepoPullThreadResolved(c, logger, viewer, comment, false)
}

// changeRepoPullReviewer requests a review from the user, or withdraws the
// request.
func changeRepoPullReviewer(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, requested bool) (statusCode int, resp any, err error) {
	if !viewerCanRequestReviews(repoCtx) {
		return http.StatusForbidden, nil, errors.New("only writers can request reviews")
	}
	reviewer, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, errors.New("user does not exist")
		}
		logger.Error("Failed to get user", "username", c.Param("username"), "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user")
	}
	if requested {
		var options []issueUser
		options, err = listReviewerOptions(c.Request().Context(), repoCtx.Repo, issue)
		if err != nil {
			logger.Error("Failed to list reviewer options", "repoID", repoCtx.Repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "list reviewer options")
		}
		if !slices.ContainsFunc(options, func(u issueUser) bool { return u.Name == reviewer.Name }) {
			return http.StatusBadRequest, nil, errors.New("a review cannot be requested from the user")
		}
		err = database.Handle.Reviews().SetRequested(c.Request().Context(), logger, viewer, repoCtx.Repo, issue, reviewer, true)
	} else {
		err = database.Handle.Reviews().SetRequested(c.Request().Context(), logger, viewer, repoCtx.Repo, issue, reviewer, false)
	}
	if err != nil {
		logger.Error("Failed to change review request", "issueID", issue.ID, "reviewerID", reviewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change review request")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoPullReviewer(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return changeRepoPullReviewer(c, logger, viewer, repoCtx, issue, true)
}

func deleteRepoPullReviewer(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return changeRepoPullReviewer(c, logger, viewer, repoCtx, issue, false)
}
