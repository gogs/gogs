package web

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/issuetemplate"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/pathx"
)

// withRepoIssues lets through viewers who can see the issues of the
// repository. Like the server-rendered pages, the issues are not found when the
// repository has no issue tracker of its own.
func withRepoIssues(c flamego.Context, repoCtx *repoContext) {
	repo := repoCtx.Repo
	if !repo.EnableIssues || repo.EnableExternalTracker || (!repoCtx.Viewer.CanRead() && !repo.CanGuestViewIssues()) {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoIssue injects the issue in the route. A pull request is not found,
// since it has its own page.
func withRepoIssue(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) {
	injectRepoIssue(c, logger, repoCtx, false)
}

// injectRepoIssue injects the issue or the pull request in the route, as isPull
// asks.
func injectRepoIssue(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, isPull bool) {
	w := c.ResponseWriter()
	repo := repoCtx.Repo
	index, _ := strconv.ParseInt(c.Param("index"), 10, 64)
	if index <= 0 {
		writeErrorResponse(w, http.StatusNotFound, errors.New("issue does not exist"))
		return
	}
	issue, err := database.GetIssueByIndex(repo.ID, index)
	if err != nil {
		if database.IsErrIssueNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("issue does not exist"))
			return
		}
		logger.Error("Failed to get issue", "issueIndex", index, "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get issue"))
		return
	}
	if issue.IsPull != isPull || (isPull && issue.PullRequest == nil) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("issue does not exist"))
		return
	}
	// Changes to the issue notify webhooks about the repository and its owner.
	repo.Owner = repoCtx.Owner
	issue.Repo = repo
	c.Map(issue)
}

// viewerCanEditIssue returns whether the viewer can change the title, the
// content and the state of the issue: writers and whoever opened it.
func viewerCanEditIssue(repoCtx *repoContext, issue *database.Issue) bool {
	return repoCtx.Viewer.CanWrite() || (repoCtx.Viewer.ID() > 0 && issue.IsPoster(repoCtx.Viewer.ID()))
}

type issueUser struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	// IsGhost marks a deleted user, who has no profile.
	IsGhost bool `json:"isGhost,omitempty"`
}

func toIssueUser(u *database.User) issueUser {
	return issueUser{Name: u.Name, AvatarURL: u.AvatarURLPath(), IsGhost: u.ID <= 0}
}

type issueAttachment struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	IsImage bool   `json:"isImage"`
}

// attachmentURL is the absolute URL of an attachment, which content links to
// so it works wherever the content is shown, e.g., in email notifications.
func attachmentURL(uuid string) string {
	return conf.Server.ExternalURL + "attachments/" + uuid
}

// linksAttachment reports whether the content links to the attachment, i.e.,
// shows it in place like on GitHub.
func linksAttachment(content, uuid string) bool {
	return strings.Contains(content, "/attachments/"+uuid)
}

func isImageName(name string) bool {
	return strings.HasPrefix(mime.TypeByExtension(filepath.Ext(name)), "image/")
}

// toIssueAttachments returns the attachments the content does not link to,
// e.g., files attached before they were linked in place, which are listed
// after the content.
func toIssueAttachments(attachments []*database.Attachment, content string) []issueAttachment {
	items := make([]issueAttachment, 0, len(attachments))
	for _, a := range attachments {
		if linksAttachment(content, a.UUID) {
			continue
		}
		items = append(items, issueAttachment{
			Name:    a.Name,
			URL:     conf.Server.Subpath + "/attachments/" + a.UUID,
			IsImage: isImageName(a.Name),
		})
	}
	return items
}

// updateLinkedAttachments attaches the new files to the issue or comment, and
// deletes the attached files the content no longer links to. Files that were
// never linked, e.g., attached before files were linked in place, are kept.
func updateLinkedAttachments(issueID, commentID int64, attached []*database.Attachment, oldContent, newContent string, uuids []string) error {
	if err := database.BindAttachments(issueID, commentID, uuids); err != nil {
		return errors.Wrap(err, "attach files")
	}
	var unlinked []*database.Attachment
	for _, a := range attached {
		if linksAttachment(oldContent, a.UUID) && !linksAttachment(newContent, a.UUID) {
			unlinked = append(unlinked, a)
		}
	}
	if _, err := database.DeleteAttachments(unlinked, true); err != nil {
		return errors.Wrap(err, "delete unlinked files")
	}
	return nil
}

type issueComment struct {
	ID int64 `json:"id"`
	// Type is "comment", "reopen", "close", "commit_ref", "review", "code",
	// "review_request" or "review_request_removed".
	Type      string    `json:"type"`
	Poster    issueUser `json:"poster"`
	CreatedAt time.Time `json:"createdAt"`
	// ContentHTML is the rendered content of a comment or the summary of a
	// review, or the link to the commit of a commit reference.
	ContentHTML string `json:"contentHTML,omitempty"`
	// Content is the Markdown of a comment, only sent to viewers who can edit
	// it.
	Content string `json:"content,omitempty"`
	// Tag is how the poster of a comment relates to the repository: "owner",
	// "writer", "poster" or empty.
	Tag           string            `json:"tag,omitempty"`
	Attachments   []issueAttachment `json:"attachments,omitempty"`
	ViewerCanEdit bool              `json:"viewerCanEdit,omitempty"`
	// Replies are the replies to the first comment of a thread, oldest first.
	Replies []issueComment `json:"replies,omitempty"`
	// Pending marks the viewer's own comment of a review not submitted yet.
	Pending bool `json:"pending,omitempty"`
	// Thread is where the first comment of a thread of code comments is.
	Thread *issueThread `json:"thread,omitempty"`
	// Review is the review a "review" item shows.
	Review *issueReview `json:"review,omitempty"`
	// Target is who a review was requested from.
	Target *issueUser `json:"target,omitempty"`
}

// issueThread is where a thread of code comments is in the diff of a pull
// request, and whether it is resolved.
type issueThread struct {
	// ReviewID is the review the thread started in.
	ReviewID int64  `json:"reviewID"`
	Path     string `json:"path"`
	// Side and StartSide are "old" or "new".
	Side      string `json:"side"`
	StartSide string `json:"startSide"`
	StartLine int64  `json:"startLine"`
	Line      int64  `json:"line"`
	// Outdated marks a thread whose lines changed since it started, which
	// only shows in the conversation.
	Outdated bool `json:"outdated"`
	// DiffHunk is the last lines of the diff up to Line when the thread
	// started, with their prefix.
	DiffHunk   string     `json:"diffHunk"`
	ResolvedBy *issueUser `json:"resolvedBy,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	// ViewerCanResolve is whether the viewer can resolve or unresolve it.
	ViewerCanResolve bool `json:"viewerCanResolve,omitempty"`
}

type issueReview struct {
	ID int64 `json:"id"`
	// State is "comment", "approve" or "request_changes".
	State     string `json:"state"`
	CommitSHA string `json:"commitSHA"`
}

var reviewStates = map[database.ReviewState]string{
	database.ReviewStateComment:        "comment",
	database.ReviewStateApprove:        "approve",
	database.ReviewStateRequestChanges: "request_changes",
}

var issueCommentTypes = map[database.CommentType]string{
	database.CommentTypeComment:              "comment",
	database.CommentTypeReopen:               "reopen",
	database.CommentTypeClose:                "close",
	database.CommentTypeCommitRef:            "commit_ref",
	database.CommentTypeReview:               "review",
	database.CommentTypeCode:                 "code",
	database.CommentTypeReviewRequest:        "review_request",
	database.CommentTypeReviewRequestRemoved: "review_request_removed",
}

type issueAttachmentSettings struct {
	AllowedTypes []string `json:"allowedTypes"`
	// MaxSize is the largest file in bytes.
	MaxSize  int64 `json:"maxSize"`
	MaxFiles int   `json:"maxFiles"`
}

type repoIssueViewMilestone struct {
	repoIssueMilestone
	// Completeness is the percentage of the issues of the milestone that are
	// closed.
	Completeness int `json:"completeness"`
}

// issueMetaOptions are the labels, milestones and assignees that writers can
// set on issues.
type issueMetaOptions struct {
	LabelOptions     []dashboardIssueLabel      `json:"labelOptions,omitempty"`
	MilestoneOptions []repoIssueFilterMilestone `json:"milestoneOptions,omitempty"`
	AssigneeOptions  []repoIssueAssignee        `json:"assigneeOptions,omitempty"`
}

func listIssueMetaOptions(repo *database.Repository) (opts issueMetaOptions, err error) {
	labels, err := database.GetLabelsByRepoID(repo.ID)
	if err != nil {
		return opts, errors.Wrap(err, "list labels")
	}
	opts.LabelOptions = make([]dashboardIssueLabel, 0, len(labels))
	for _, label := range labels {
		opts.LabelOptions = append(opts.LabelOptions, toIssueLabel(label))
	}

	milestones, err := database.GetMilestonesByRepoID(repo.ID)
	if err != nil {
		return opts, errors.Wrap(err, "list milestones")
	}
	opts.MilestoneOptions = make([]repoIssueFilterMilestone, 0, len(milestones))
	for _, m := range milestones {
		opts.MilestoneOptions = append(opts.MilestoneOptions, repoIssueFilterMilestone{
			repoIssueMilestone: repoIssueMilestone{ID: m.ID, Name: m.Name},
			IsClosed:           m.IsClosed,
		})
	}

	assignees, err := repo.GetAssignees()
	if err != nil {
		return opts, errors.Wrap(err, "list assignees")
	}
	opts.AssigneeOptions = make([]repoIssueAssignee, 0, len(assignees))
	for _, u := range assignees {
		opts.AssigneeOptions = append(opts.AssigneeOptions, repoIssueAssignee{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURLPath()})
	}
	return opts, nil
}

// attachmentLimits are the configured limits of the files attached to issues
// and comments, or to releases.
type attachmentLimits struct {
	Enabled      bool
	AllowedTypes []string
	// MaxSize is the largest file in megabytes.
	MaxSize  int64
	MaxFiles int
}

func issueAttachmentLimits() attachmentLimits {
	return attachmentLimits{
		Enabled:      conf.Attachment.Enabled,
		AllowedTypes: conf.Attachment.AllowedTypes,
		MaxSize:      conf.Attachment.MaxSize,
		MaxFiles:     conf.Attachment.MaxFiles,
	}
}

// attachmentSettings returns how files can be attached, or nil when
// attachments are disabled.
func attachmentSettings(lim attachmentLimits) *issueAttachmentSettings {
	if !lim.Enabled {
		return nil
	}
	return &issueAttachmentSettings{
		AllowedTypes: lim.AllowedTypes,
		MaxSize:      lim.MaxSize << 20,
		MaxFiles:     lim.MaxFiles,
	}
}

type repoIssueView struct {
	Index       int64             `json:"index"`
	Title       string            `json:"title"`
	IsClosed    bool              `json:"isClosed"`
	Poster      issueUser         `json:"poster"`
	CreatedAt   time.Time         `json:"createdAt"`
	ContentHTML string            `json:"contentHTML"`
	Content     string            `json:"content"`
	Attachments []issueAttachment `json:"attachments"`
	Comments    []issueComment    `json:"comments"`

	Labels    []dashboardIssueLabel   `json:"labels"`
	Milestone *repoIssueViewMilestone `json:"milestone,omitempty"`
	Assignee  *repoIssueAssignee      `json:"assignee,omitempty"`
	// The options are only sent to writers.
	issueMetaOptions
	Participants []issueUser `json:"participants"`

	// Attachment is how files can be attached to comments, only sent to
	// authenticated viewers when attachments are enabled.
	Attachment *issueAttachmentSettings `json:"attachment,omitempty"`
	// ViewerCanEdit is whether the viewer can change the title, the content
	// and the state of the issue.
	ViewerCanEdit  bool `json:"viewerCanEdit"`
	ViewerCanWrite bool `json:"viewerCanWrite"`
}

func getRepoIssue(c flamego.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoIssueView, err error) {
	resp, err = toRepoIssueView(c.Request().Context(), logger, viewer, repoCtx, issue)
	if err != nil {
		logger.Error("Failed to describe issue", "issueID", issue.ID, "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, resp, nil
}

// toRepoIssueView describes the issue or the pull request with its comments,
// and marks it read by the viewer.
func toRepoIssueView(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (resp *repoIssueView, err error) {
	repo := repoCtx.Repo
	metas := repo.ComposeMetas()
	resp = &repoIssueView{
		Index:       issue.Index,
		Title:       issue.Title,
		IsClosed:    issue.IsClosed,
		Poster:      toIssueUser(issue.Poster),
		CreatedAt:   issue.Created.UTC(),
		ContentHTML: embedPermalinks(ctx, logger, repoCtx.Owner.Name, repo.Name, string(markup.Markdown(issue.Content, repo.Link(), metas))),
		Content:     issue.Content,
		Attachments: toIssueAttachments(issue.Attachments, issue.Content),
		Comments:    make([]issueComment, 0, len(issue.Comments)),
		Labels:      make([]dashboardIssueLabel, 0, len(issue.Labels)),

		ViewerCanEdit:  viewer != nil && viewerCanEditIssue(repoCtx, issue),
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}
	for _, label := range issue.Labels {
		resp.Labels = append(resp.Labels, toIssueLabel(label))
	}
	if issue.Milestone != nil {
		m := issue.Milestone
		resp.Milestone = &repoIssueViewMilestone{repoIssueMilestone: repoIssueMilestone{ID: m.ID, Name: m.Name}}
		if m.NumIssues > 0 {
			resp.Milestone.Completeness = m.NumClosedIssues * 100 / m.NumIssues
		}
	}
	if issue.Assignee != nil {
		resp.Assignee = &repoIssueAssignee{ID: issue.Assignee.ID, Name: issue.Assignee.Name, AvatarURL: issue.Assignee.AvatarURLPath()}
	}

	cv := newCommentViewer(ctx, logger, viewer, repoCtx, issue)
	var reviews map[int64]*database.Review
	if issue.IsPull {
		submitted, err := database.GetSubmittedReviewsByIssueID(issue.ID)
		if err != nil {
			return nil, errors.Wrap(err, "list reviews")
		}
		reviews = make(map[int64]*database.Review, len(submitted))
		for _, r := range submitted {
			reviews[r.ID] = r
		}
	}

	// Comments are oldest first, so the first comment of a thread comes
	// before its replies.
	roots := make(map[int64]int)
	for _, comment := range issue.Comments {
		typ, ok := issueCommentTypes[comment.Type]
		// Like the server-rendered page, references from comments and pull
		// requests are not shown. Code comments show in the threads of their
		// review.
		if !ok || comment.Type == database.CommentTypeCode {
			continue
		}
		if comment.ReplyToID > 0 {
			if i, ok := roots[comment.ReplyToID]; ok {
				resp.Comments[i].Replies = append(resp.Comments[i].Replies, cv.toItem(comment))
			}
			continue
		}

		var item issueComment
		switch comment.Type {
		case database.CommentTypeCommitRef:
			item = issueComment{ID: comment.ID, Type: typ, Poster: toIssueUser(comment.Poster), CreatedAt: comment.Created.UTC()}
			item.ContentHTML = markup.Sanitize(comment.Content)
		case database.CommentTypeReview:
			review := reviews[comment.ReviewID]
			if review == nil {
				continue
			}
			item = cv.toItem(comment)
			item.Review = &issueReview{ID: review.ID, State: reviewStates[review.State], CommitSHA: review.CommitSHA}
		case database.CommentTypeReviewRequest, database.CommentTypeReviewRequestRemoved:
			target, err := database.Handle.Users().GetByID(ctx, comment.TargetUserID)
			if err != nil {
				if !database.IsErrUserNotExist(err) {
					return nil, errors.Wrap(err, "get requested reviewer")
				}
				target = database.NewGhostUser()
			}
			targetUser := toIssueUser(target)
			item = issueComment{ID: comment.ID, Type: typ, Poster: toIssueUser(comment.Poster), CreatedAt: comment.Created.UTC(), Target: &targetUser}
		case database.CommentTypeComment:
			item = cv.toItem(comment)
			roots[comment.ID] = len(resp.Comments)
		default:
			item = issueComment{ID: comment.ID, Type: typ, Poster: toIssueUser(comment.Poster), CreatedAt: comment.Created.UTC()}
		}
		resp.Comments = append(resp.Comments, item)
	}
	participants := cv.participants
	resp.Participants = make([]issueUser, 0, len(participants))
	for _, u := range participants {
		resp.Participants = append(resp.Participants, toIssueUser(u))
	}

	if resp.ViewerCanWrite {
		if resp.issueMetaOptions, err = listIssueMetaOptions(repo); err != nil {
			return nil, errors.Wrap(err, "list what can be set on issues")
		}
	}

	if viewer != nil {
		resp.Attachment = attachmentSettings(issueAttachmentLimits())
		if err = issue.ReadBy(viewer.ID); err != nil {
			return nil, errors.Wrap(err, "mark as read")
		}
	}
	return resp, nil
}

// commentViewer describes comments to the viewer, and collects the users who
// took part in the conversation.
type commentViewer struct {
	ctx          context.Context
	logger       *logx.Logger
	viewer       *database.User
	repoCtx      *repoContext
	issue        *database.Issue
	metas        map[string]string
	tags         map[int64]string
	participants []*database.User
}

func newCommentViewer(ctx context.Context, logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue) *commentViewer {
	return &commentViewer{
		ctx:          ctx,
		logger:       logger,
		viewer:       viewer,
		repoCtx:      repoCtx,
		issue:        issue,
		metas:        repoCtx.Repo.ComposeMetas(),
		tags:         make(map[int64]string),
		participants: []*database.User{issue.Poster},
	}
}

// toItem describes a comment with content: a comment, a reply, a code
// comment or the summary of a review.
func (cv *commentViewer) toItem(comment *database.Comment) issueComment {
	repo := cv.repoCtx.Repo
	item := issueComment{
		ID:        comment.ID,
		Type:      issueCommentTypes[comment.Type],
		Poster:    toIssueUser(comment.Poster),
		CreatedAt: comment.Created.UTC(),
		Pending:   comment.IsPending,
	}
	if comment.Content != "" {
		item.ContentHTML = embedPermalinks(cv.ctx, cv.logger, cv.repoCtx.Owner.Name, repo.Name, string(markup.Markdown(comment.Content, repo.Link(), cv.metas)))
	}
	item.Attachments = toIssueAttachments(comment.Attachments, comment.Content)
	// Like the server-rendered page, the poster and admins can change a
	// comment. The summary of a review is changed with the review.
	if comment.Type != database.CommentTypeReview && cv.viewer != nil &&
		(comment.PosterID == cv.viewer.ID || cv.repoCtx.Viewer.CanAdminister()) {
		item.ViewerCanEdit = true
		item.Content = comment.Content
	}

	tag, ok := cv.tags[comment.PosterID]
	if !ok {
		switch {
		case repo.IsOwnedBy(comment.PosterID) || (cv.repoCtx.Owner.IsOrganization() && cv.repoCtx.Owner.IsOwnedBy(comment.PosterID)):
			tag = "owner"
		case cv.canWrite(comment.PosterID):
			tag = "writer"
		case comment.PosterID == cv.issue.PosterID:
			tag = "poster"
		}
		cv.tags[comment.PosterID] = tag
	}
	item.Tag = tag

	if !comment.IsPending && !slices.ContainsFunc(cv.participants, func(u *database.User) bool { return u.ID == comment.Poster.ID }) {
		cv.participants = append(cv.participants, comment.Poster)
	}
	return item
}

// canWrite returns whether the user can write to the repository.
func (cv *commentViewer) canWrite(userID int64) bool {
	repo := cv.repoCtx.Repo
	return database.Handle.Permissions().Authorize(cv.ctx, cv.logger, userID, repo.ID, database.AccessModeWrite,
		database.AccessModeOptions{OwnerID: repo.OwnerID, Private: repo.IsPrivate})
}

// limitIssueContent caps the size of the Markdown sent to the issue page,
// which outgrows the limit of the other forms.
func limitIssueContent(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 1<<20) // 1 MiB
}

// requireIssueEditor lets through the viewers who can change the title, the
// content and the state of the issue.
func requireIssueEditor(c flamego.Context, repoCtx *repoContext, issue *database.Issue) {
	if !viewerCanEditIssue(repoCtx, issue) {
		writeErrorResponse(c.ResponseWriter(), http.StatusForbidden, errors.New("only writers and the poster can change the issue"))
	}
}

type repoIssueRequest struct {
	Title   *string `json:"title"`
	Content *string `json:"content"`
	// Attachments are the UUIDs of the files uploaded while editing the
	// content.
	Attachments []string `json:"attachments"`
}

func patchRepoIssue(logger *logx.Logger, l i18n.Locale, viewer *database.User, issue *database.Issue, req repoIssueRequest) (statusCode int, resp any, err error) {
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if resp := textFieldError(l, "title", "repo.milestones.title", title, 255); resp != nil {
			return http.StatusBadRequest, resp, nil
		}
		if title != issue.Title {
			if err = issue.ChangeTitle(logger, viewer, title); err != nil {
				logger.Error("Failed to change issue title", "issueID", issue.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "change title")
			}
		}
	}
	if req.Content != nil && *req.Content != issue.Content {
		if statusCode, err = checkNewAttachments(issueAttachmentLimits(), req.Attachments); err != nil {
			if statusCode == http.StatusInternalServerError {
				logger.Error("Failed to check new attachments", "error", err)
			}
			return statusCode, nil, err
		}
		oldContent := issue.Content
		if err = issue.ChangeContent(logger, viewer, *req.Content); err != nil {
			logger.Error("Failed to change issue content", "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "change content")
		}
		attached, err := database.GetAttachmentsByIssueID(issue.ID)
		if err == nil {
			err = updateLinkedAttachments(issue.ID, 0, attached, oldContent, issue.Content, req.Attachments)
		}
		if err != nil {
			logger.Error("Failed to update issue attachments", "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, err
		}
	}
	return http.StatusNoContent, nil, nil
}

func setRepoIssueClosed(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue, closed bool) (statusCode int, resp any, err error) {
	if issue.IsPull {
		pull := issue.PullRequest
		if pull.HasMerged {
			return http.StatusNotFound, nil, errors.New("pull request has been merged")
		}
		// Like the server-rendered page, a pull request only reopens when no
		// other one merges the same branches, and it is checked for conflicts
		// again.
		if !closed {
			other, err := database.GetUnmergedPullRequest(pull.HeadRepoID, pull.BaseRepoID, pull.HeadBranch, pull.BaseBranch)
			if err != nil && !database.IsErrPullRequestNotExist(err) {
				logger.Error("Failed to get open pull request with the same branches", "pullID", pull.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "get open pull request")
			} else if other != nil {
				return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo.pulls.open_unmerged_pull_exists", other.Index)}, nil
			}
			if err = pull.UpdatePatch(c.Request().Context(), logger); err != nil {
				logger.Error("Failed to update pull request patch", "pullID", pull.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "update patch")
			}
			pull.AddToTaskQueue(logger)
		}
	}
	if err = issue.ChangeStatus(logger, viewer, repoCtx.Repo, closed); err != nil {
		logger.Error("Failed to change issue state", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change state")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoIssueClosed(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return setRepoIssueClosed(c, logger, l, viewer, repoCtx, issue, true)
}

func deleteRepoIssueClosed(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return setRepoIssueClosed(c, logger, l, viewer, repoCtx, issue, false)
}

type repoIssueLabelsRequest struct {
	IDs []int64 `json:"ids"`
}

// putRepoIssueLabels replaces the labels of the issue.
func putRepoIssueLabels(logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueLabelsRequest) (statusCode int, resp any, err error) {
	labels := make([]*database.Label, 0, len(req.IDs))
	for _, id := range req.IDs {
		label, err := database.GetLabelOfRepoByID(repoCtx.Repo.ID, id)
		if err != nil {
			if database.IsErrLabelNotExist(err) {
				return http.StatusBadRequest, nil, errors.Newf("label %d does not exist", id)
			}
			logger.Error("Failed to get label", "labelID", id, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get label")
		}
		labels = append(labels, label)
	}

	for _, label := range slices.Clone(issue.Labels) {
		if slices.ContainsFunc(labels, func(l *database.Label) bool { return l.ID == label.ID }) {
			continue
		}
		if err = issue.RemoveLabel(logger, viewer, label); err != nil {
			logger.Error("Failed to remove label from issue", "labelID", label.ID, "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "remove label")
		}
	}
	for _, label := range labels {
		if issue.HasLabel(label.ID) {
			continue
		}
		if err = issue.AddLabel(logger, viewer, label); err != nil {
			logger.Error("Failed to add label to issue", "labelID", label.ID, "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "add label")
		}
	}
	return http.StatusNoContent, nil, nil
}

type repoIssueIDRequest struct {
	// ID is the milestone or the assignee to set, or 0 to clear it.
	ID int64 `json:"id"`
}

// checkIssueMilestone returns the status code and the error to answer with
// when the milestone, unless 0, is not one of the repository. Unlike the
// server-rendered pages, a milestone of another repository is turned away.
func checkIssueMilestone(repo *database.Repository, id int64) (statusCode int, err error) {
	if id == 0 {
		return 0, nil
	}
	if _, err = database.GetMilestoneByRepoID(repo.ID, id); err != nil {
		if database.IsErrMilestoneNotExist(err) {
			return http.StatusBadRequest, errors.New("milestone does not exist")
		}
		return http.StatusInternalServerError, errors.Wrapf(err, "get milestone %d", id)
	}
	return 0, nil
}

// checkIssueAssignee returns the status code and the error to answer with when
// issues of the repository cannot be assigned to the user, unless 0. Unlike
// the server-rendered pages, only those the issue can be assigned to are.
func checkIssueAssignee(repo *database.Repository, id int64) (statusCode int, err error) {
	if id == 0 {
		return 0, nil
	}
	assignees, err := repo.GetAssignees()
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "list assignees of repository %d", repo.ID)
	}
	if !slices.ContainsFunc(assignees, func(u *database.User) bool { return u.ID == id }) {
		return http.StatusBadRequest, errors.New("assignee does not exist")
	}
	return 0, nil
}

func putRepoIssueMilestone(logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueIDRequest) (statusCode int, resp any, err error) {
	if req.ID == issue.MilestoneID {
		return http.StatusNoContent, nil, nil
	}
	if statusCode, err = checkIssueMilestone(repoCtx.Repo, req.ID); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check issue milestone", "error", err)
		}
		return statusCode, nil, err
	}
	oldID := issue.MilestoneID
	issue.MilestoneID = req.ID
	if err = database.ChangeMilestoneAssign(logger, viewer, issue, oldID); err != nil {
		logger.Error("Failed to change issue milestone", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change milestone")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoIssueAssignee(logger *logx.Logger, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueIDRequest) (statusCode int, resp any, err error) {
	if req.ID == issue.AssigneeID {
		return http.StatusNoContent, nil, nil
	}
	if statusCode, err = checkIssueAssignee(repoCtx.Repo, req.ID); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check issue assignee", "error", err)
		}
		return statusCode, nil, err
	}
	if err = issue.ChangeAssignee(logger, viewer, req.ID); err != nil {
		logger.Error("Failed to change issue assignee", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change assignee")
	}
	return http.StatusNoContent, nil, nil
}

type repoIssueCommentRequest struct {
	Content string `json:"content"`
	// Attachments are the UUIDs of the uploaded files to attach.
	Attachments []string `json:"attachments"`
	// ReplyTo is the first comment of the thread to reply to.
	ReplyTo int64 `json:"replyTo"`
	// Pending adds a reply to a code comment to the viewer's pending review.
	Pending bool `json:"pending"`
}

// checkNewAttachments returns the status code and the error to answer with
// when the files cannot be attached. Unlike the server-rendered pages, only
// files that are not attached yet can be, so an issue or a comment cannot take
// over those of others.
func checkNewAttachments(lim attachmentLimits, uuids []string) (statusCode int, err error) {
	if len(uuids) == 0 {
		return 0, nil
	}
	if !lim.Enabled {
		return http.StatusBadRequest, errors.New("attachments are disabled")
	}
	if len(uuids) > lim.MaxFiles {
		return http.StatusBadRequest, errors.Newf("at most %d files can be attached", lim.MaxFiles)
	}
	for _, uuid := range uuids {
		a, err := database.GetAttachmentByUUID(uuid)
		if err != nil {
			if database.IsErrAttachmentNotExist(err) {
				return http.StatusBadRequest, errors.New("attachment does not exist")
			}
			return http.StatusInternalServerError, errors.Wrapf(err, "get attachment %q", uuid)
		}
		if a.IssueID > 0 || a.CommentID > 0 || a.ReleaseID > 0 {
			return http.StatusBadRequest, errors.New("attachment does not exist")
		}
	}
	return 0, nil
}

func postRepoIssueComment(logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueCommentRequest) (statusCode int, resp any, err error) {
	// Like the server-rendered page, a comment can be only attachments.
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		msg := l.Tr("form.Content") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if statusCode, err = checkNewAttachments(issueAttachmentLimits(), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check new attachments", "error", err)
		}
		return statusCode, nil, err
	}

	if req.ReplyTo > 0 {
		root, err := database.GetCommentByID(req.ReplyTo)
		if err != nil && !database.IsErrCommentNotExist(err) {
			logger.Error("Failed to get comment", "commentID", req.ReplyTo, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get comment")
		}
		// Replies never reply to replies, and nobody else sees a pending
		// comment.
		if root == nil || root.IssueID != issue.ID || root.ReplyToID != 0 ||
			(root.Type != database.CommentTypeComment && root.Type != database.CommentTypeCode) ||
			(root.IsPending && root.PosterID != viewer.ID) {
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo.issues.reply_not_allowed")}, nil
		}
		comment, err := database.CreateReply(logger, viewer, repoCtx.Repo, issue, root, req.Content, req.Attachments, req.Pending || root.IsPending)
		if err != nil {
			logger.Error("Failed to create reply", "issueID", issue.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "create reply")
		}
		return http.StatusCreated, map[string]int64{"id": comment.ID}, nil
	}

	comment, err := database.CreateIssueComment(logger, viewer, repoCtx.Repo, issue, req.Content, req.Attachments)
	if err != nil {
		logger.Error("Failed to create issue comment", "issueID", issue.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create comment")
	}
	return http.StatusCreated, map[string]int64{"id": comment.ID}, nil
}

// withIssueComment injects the comment of the issue in the route, which only
// its poster and admins can change, like on the server-rendered page.
func withIssueComment(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, issue *database.Issue) {
	w := c.ResponseWriter()
	comment, err := database.GetCommentByID(idParam(c))
	if err != nil {
		if database.IsErrCommentNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("comment does not exist"))
			return
		}
		logger.Error("Failed to get comment", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get comment"))
		return
	}
	if comment.IssueID != issue.ID || (comment.Type != database.CommentTypeComment && comment.Type != database.CommentTypeCode) ||
		(comment.IsPending && comment.PosterID != repoCtx.Viewer.ID()) ||
		(comment.PosterID != repoCtx.Viewer.ID() && !repoCtx.Viewer.CanAdminister()) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("comment does not exist"))
		return
	}
	c.Map(comment)
}

func patchRepoIssueComment(logger *logx.Logger, l i18n.Locale, viewer *database.User, comment *database.Comment, req repoIssueCommentRequest) (statusCode int, resp any, err error) {
	if strings.TrimSpace(req.Content) == "" {
		msg := l.Tr("form.Content") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if statusCode, err = checkNewAttachments(issueAttachmentLimits(), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check new attachments", "error", err)
		}
		return statusCode, nil, err
	}
	oldContent := comment.Content
	comment.Content = req.Content
	if err = database.UpdateComment(logger, viewer, comment, oldContent); err != nil {
		logger.Error("Failed to update comment", "commentID", comment.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update comment")
	}
	attached, err := database.GetAttachmentsByCommentID(comment.ID)
	if err == nil {
		err = updateLinkedAttachments(comment.IssueID, comment.ID, attached, oldContent, comment.Content, req.Attachments)
	}
	if err != nil {
		logger.Error("Failed to update comment attachments", "commentID", comment.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoIssueComment(logger *logx.Logger, viewer *database.User, comment *database.Comment) (statusCode int, resp any, err error) {
	if err = database.DeleteCommentByID(logger, viewer, comment.ID); err != nil {
		logger.Error("Failed to delete comment", "commentID", comment.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete comment")
	}
	return http.StatusNoContent, nil, nil
}

type markdownRequest struct {
	Text string `json:"text"`
	// Wiki resolves relative links against the wiki instead of the
	// repository.
	Wiki bool `json:"wiki"`
	// Branch and Path render the text as the Markdown file at Path in Branch,
	// with relative links resolved against its directory.
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

// postRepoMarkdown renders Markdown the way the issues of the repository are,
// the way its wiki pages are, or the way one of its files is.
func postRepoMarkdown(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, req markdownRequest) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	var html []byte
	switch {
	case req.Wiki:
		html = markup.MarkdownFile(req.Text, repo.Link()+"/wiki", repo.ComposeMetas())
	case req.Path != "":
		fileLink := repo.Link() + "/src/" + escapePath(req.Branch) + "/" + escapePath(pathx.Clean(req.Path))
		html = markup.MarkdownFile(req.Text, path.Dir(fileLink), repo.ComposeMetas())
	default:
		html = []byte(embedPermalinks(c.Request().Context(), logger, repoCtx.Owner.Name, repo.Name, string(markup.Markdown(req.Text, repo.Link(), repo.ComposeMetas()))))
	}
	return http.StatusOK, map[string]string{"html": string(html)}, nil
}

// limitAttachmentUpload caps the size of an uploaded attachment.
func limitAttachmentUpload(lim attachmentLimits) flamego.Handler {
	return func(c flamego.Context) {
		r := c.Request().Request
		r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, lim.MaxSize<<20+64<<10) // Room for the multipart framing.
	}
}

// uploadedFile is a file uploaded as the "file" form field that passed the
// size and type limits. Head is its first bytes, already read from File.
type uploadedFile struct {
	File   multipart.File
	Header *multipart.FileHeader
	Head   []byte
}

// readUploadedFile reads the file uploaded as the "file" form field, checking
// it against the limits. The caller closes the file.
func readUploadedFile(c flamego.Context, l i18n.Locale, lim attachmentLimits) (_ *uploadedFile, statusCode int, err error) {
	if !lim.Enabled {
		return nil, http.StatusNotFound, errors.New("uploads are disabled")
	}
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, http.StatusRequestEntityTooLarge, errors.Newf("file exceeds the size limit of %d MB", lim.MaxSize)
		}
		return nil, http.StatusBadRequest, errors.Wrap(err, "read file")
	}
	if header.Size > lim.MaxSize<<20 {
		_ = file.Close()
		return nil, http.StatusRequestEntityTooLarge, errors.Newf("file exceeds the size limit of %d MB", lim.MaxSize)
	}

	buf := make([]byte, 1024)
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		_ = file.Close()
		return nil, http.StatusBadRequest, errors.Wrap(err, "read file")
	}
	buf = buf[:n]
	fileType := http.DetectContentType(buf)
	if !slices.ContainsFunc(lim.AllowedTypes, func(t string) bool {
		t = strings.TrimSpace(t)
		return t == "*/*" || t == fileType
	}) {
		_ = file.Close()
		return nil, http.StatusBadRequest, errors.New(l.Tr("dropzone.invalid_input_type"))
	}
	return &uploadedFile{File: file, Header: header, Head: buf}, http.StatusOK, nil
}

// storeAttachment stores the file uploaded as the "file" form field, and
// returns its UUID.
func storeAttachment(c flamego.Context, l i18n.Locale, viewer *database.User, lim attachmentLimits) (statusCode int, resp any, err error) {
	f, statusCode, err := readUploadedFile(c, l, lim)
	if err != nil {
		return statusCode, nil, err
	}
	defer func() { _ = f.File.Close() }()

	attachment, err := database.NewAttachment(viewer.ID, f.Header.Filename, f.Head, f.File)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "store attachment")
	}
	return http.StatusCreated, map[string]string{"uuid": attachment.UUID, "url": attachmentURL(attachment.UUID)}, nil
}

// postRepoIssueAttachment stores a file to attach to an issue or a comment.
func postRepoIssueAttachment(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User) (statusCode int, resp any, err error) {
	statusCode, resp, err = storeAttachment(c, l, viewer, issueAttachmentLimits())
	if statusCode == http.StatusInternalServerError {
		logger.Error("Failed to store attachment", "error", err)
	}
	return statusCode, resp, err
}

type issueTemplateSummary struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	About string `json:"about"`
}

type issueTemplateOption struct {
	Label string `json:"label"`
	// LabelHTML is the rendered inline Markdown of the label of a checkbox,
	// which supports bold, italics and links like on GitHub.
	LabelHTML string `json:"labelHTML,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

type issueTemplateField struct {
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
	// DescriptionHTML is the rendered Markdown of the description, and HTML
	// the rendered Markdown of a markdown element.
	DescriptionHTML string                `json:"descriptionHTML,omitempty"`
	HTML            string                `json:"html,omitempty"`
	Placeholder     string                `json:"placeholder,omitempty"`
	Value           string                `json:"value,omitempty"`
	Render          string                `json:"render,omitempty"`
	Multiple        bool                  `json:"multiple,omitempty"`
	Options         []issueTemplateOption `json:"options,omitempty"`
	Default         *int                  `json:"default,omitempty"`
	Required        bool                  `json:"required,omitempty"`
	// Accept is the comma-separated file extensions an upload takes.
	Accept string `json:"accept,omitempty"`
}

type issueTemplate struct {
	issueTemplateSummary
	Title     string   `json:"title"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
	// Body is the content of a Markdown template, and Form the fields of an
	// issue form.
	Body string               `json:"body"`
	Form []issueTemplateField `json:"form,omitempty"`
}

type issueContactLink struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	About string `json:"about"`
}

type repoNewIssueOptions struct {
	// The templates are only sent to viewers who can read the repository.
	Templates          []issueTemplateSummary `json:"templates"`
	ContactLinks       []issueContactLink     `json:"contactLinks"`
	BlankIssuesEnabled bool                   `json:"blankIssuesEnabled"`
	// Template is the template asked for with the "template" query parameter.
	Template *issueTemplate `json:"template,omitempty"`
	// Content is what a blank issue starts with, i.e., the single issue
	// template.
	Content string `json:"content"`
	// The options are only sent to writers.
	issueMetaOptions
	Attachment     *issueAttachmentSettings `json:"attachment,omitempty"`
	ViewerCanWrite bool                     `json:"viewerCanWrite"`
}

// loadIssueTemplates returns the issue templates on the default branch of the
// repository, if the viewer can read it.
func loadIssueTemplates(ctx context.Context, repoCtx *repoContext) (*issuetemplate.IssueTemplates, error) {
	none := &issuetemplate.IssueTemplates{Config: issuetemplate.Config{BlankIssuesEnabled: true}}
	if !repoCtx.Viewer.CanRead() || repoCtx.Repo.IsEmpty {
		return none, nil
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		return nil, errors.Wrap(err, "open repository")
	}
	commit, err := gitRepo.BranchCommit(ctx, repoCtx.Repo.DefaultBranch)
	if err != nil {
		// Like the server-rendered page, a broken default branch has no templates.
		return none, nil
	}
	ts, err := issuetemplate.LoadIssueTemplates(ctx, commit)
	if err != nil {
		return nil, errors.Wrap(err, "load issue templates")
	}
	return ts, nil
}

func toIssueTemplate(repo *database.Repository, t *issuetemplate.Template) *issueTemplate {
	markdown := func(text string) string {
		if strings.TrimSpace(text) == "" {
			return ""
		}
		return string(markup.Markdown(text, repo.Link(), repo.ComposeMetas()))
	}
	resp := &issueTemplate{
		issueTemplateSummary: issueTemplateSummary{File: t.File, Name: t.Name, About: t.About},
		Title:                t.Title,
		Labels:               append([]string{}, t.Labels...),
		Assignees:            append([]string{}, t.Assignees...),
		Body:                 t.Body,
	}
	for _, el := range t.Form {
		field := issueTemplateField{
			Type:            el.Type,
			Label:           el.Attributes.Label,
			DescriptionHTML: markdown(el.Attributes.Description),
			Placeholder:     el.Attributes.Placeholder,
			Value:           el.Attributes.Value,
			Render:          el.Attributes.Render,
			Multiple:        el.Attributes.Multiple,
			Default:         el.Attributes.Default,
			Required:        el.Validations.Required,
			Accept:          el.Validations.Accept,
		}
		if el.Type == issuetemplate.ElementMarkdown {
			field.HTML = markdown(el.Attributes.Value)
			field.Value = ""
		}
		for _, o := range el.Attributes.Options {
			opt := issueTemplateOption{Label: o.Label, Required: o.Required}
			if el.Type == issuetemplate.ElementCheckboxes {
				// A label is a single paragraph, shown inline next to its checkbox.
				html := strings.TrimSpace(markdown(o.Label))
				opt.LabelHTML = strings.TrimSuffix(strings.TrimPrefix(html, "<p>"), "</p>")
			}
			field.Options = append(field.Options, opt)
		}
		resp.Form = append(resp.Form, field)
	}
	return resp
}

func getRepoNewIssue(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoNewIssueOptions, err error) {
	repo := repoCtx.Repo
	ts, err := loadIssueTemplates(c.Request().Context(), repoCtx)
	if err != nil {
		logger.Error("Failed to load issue templates", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	resp = &repoNewIssueOptions{
		Templates:          make([]issueTemplateSummary, 0, len(ts.Templates)),
		ContactLinks:       make([]issueContactLink, 0, len(ts.Config.ContactLinks)),
		BlankIssuesEnabled: ts.Config.BlankIssuesEnabled,
		Content:            ts.Default,
		Attachment:         attachmentSettings(issueAttachmentLimits()),
		ViewerCanWrite:     repoCtx.Viewer.CanWrite(),
	}
	for _, t := range ts.Templates {
		resp.Templates = append(resp.Templates, issueTemplateSummary{File: t.File, Name: t.Name, About: t.About})
	}
	for _, link := range ts.Config.ContactLinks {
		resp.ContactLinks = append(resp.ContactLinks, issueContactLink(link))
	}
	if t := ts.Find(c.Query("template")); t != nil {
		resp.Template = toIssueTemplate(repo, t)
	}
	if resp.ViewerCanWrite {
		if resp.issueMetaOptions, err = listIssueMetaOptions(repo); err != nil {
			logger.Error("Failed to list issue metadata options", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, err
		}
	}
	return http.StatusOK, resp, nil
}

type repoNewIssueRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	// LabelIDs, MilestoneID and AssigneeID are only set by writers, like on
	// the server-rendered page.
	LabelIDs    []int64 `json:"labelIDs"`
	MilestoneID int64   `json:"milestoneID"`
	AssigneeID  int64   `json:"assigneeID"`
	// Attachments are the UUIDs of the uploaded files to attach.
	Attachments []string `json:"attachments"`
	// Template is the file of the issue template the issue is opened with.
	// The content of an issue form is made from Fields, which has the values
	// of its elements in order.
	Template string     `json:"template"`
	Fields   [][]string `json:"fields"`
}

// templateLabelsAndAssignee returns the labels and the assignee the template
// sets that exist in the repository.
func templateLabelsAndAssignee(repo *database.Repository, t *issuetemplate.Template) (labelIDs []int64, assigneeID int64, err error) {
	if len(t.Labels) > 0 {
		labels, err := database.GetLabelsByRepoID(repo.ID)
		if err != nil {
			return nil, 0, errors.Wrap(err, "list labels")
		}
		for _, label := range labels {
			if slices.ContainsFunc(t.Labels, func(name string) bool { return strings.EqualFold(name, label.Name) }) {
				labelIDs = append(labelIDs, label.ID)
			}
		}
	}
	if len(t.Assignees) > 0 {
		assignees, err := repo.GetAssignees()
		if err != nil {
			return nil, 0, errors.Wrap(err, "list assignees")
		}
		// An issue has a single assignee, the first one the template names.
		for _, name := range t.Assignees {
			i := slices.IndexFunc(assignees, func(u *database.User) bool { return strings.EqualFold(u.Name, name) })
			if i >= 0 {
				assigneeID = assignees[i].ID
				break
			}
		}
	}
	return labelIDs, assigneeID, nil
}

// newIssueFromRequest checks the form to open an issue or a pull request, and
// returns the issue to create with its labels. A non-nil resp or err is what to
// answer with instead.
func newIssueFromRequest(l i18n.Locale, viewer *database.User, repoCtx *repoContext, req repoNewIssueRequest) (issue *database.Issue, labelIDs []int64, statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	title := strings.TrimSpace(req.Title)
	if resp := textFieldError(l, "title", "repo.milestones.title", title, 255); resp != nil {
		return nil, nil, http.StatusBadRequest, resp, nil
	}
	if statusCode, err = checkNewAttachments(issueAttachmentLimits(), req.Attachments); err != nil {
		return nil, nil, statusCode, nil, err
	}

	issue = &database.Issue{
		RepoID:   repo.ID,
		Title:    title,
		PosterID: viewer.ID,
		Poster:   viewer,
		Content:  req.Content,
	}
	// Like the server-rendered page, what only writers can set is left out for
	// others.
	if repoCtx.Viewer.CanWrite() {
		for _, id := range req.LabelIDs {
			if _, err = database.GetLabelOfRepoByID(repo.ID, id); err != nil {
				if database.IsErrLabelNotExist(err) {
					return nil, nil, http.StatusBadRequest, nil, errors.Newf("label %d does not exist", id)
				}
				return nil, nil, http.StatusInternalServerError, nil, errors.Wrapf(err, "get label %d", id)
			}
		}
		if statusCode, err = checkIssueMilestone(repo, req.MilestoneID); err == nil {
			statusCode, err = checkIssueAssignee(repo, req.AssigneeID)
		}
		if err != nil {
			return nil, nil, statusCode, nil, err
		}
		labelIDs = req.LabelIDs
		issue.MilestoneID = req.MilestoneID
		issue.AssigneeID = req.AssigneeID
	}
	return issue, labelIDs, 0, nil, nil
}

func postRepoIssue(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, req repoNewIssueRequest) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	// The title is checked before the fields of an issue form, so its error
	// shows first.
	if resp := textFieldError(l, "title", "repo.milestones.title", strings.TrimSpace(req.Title), 255); resp != nil {
		return http.StatusBadRequest, resp, nil
	}

	var tmpl *issuetemplate.Template
	if req.Template != "" {
		ts, err := loadIssueTemplates(c.Request().Context(), repoCtx)
		if err != nil {
			logger.Error("Failed to load issue templates", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, err
		}
		if tmpl = ts.Find(req.Template); tmpl == nil {
			return http.StatusBadRequest, nil, errors.Newf("issue template %q does not exist", req.Template)
		}
		if tmpl.IsForm() {
			// The files of upload elements are uploaded but not attached yet.
			var files func(id string) (issuetemplate.File, bool)
			if issueAttachmentLimits().Enabled {
				files = func(id string) (issuetemplate.File, bool) {
					a, err := database.GetAttachmentByUUID(id)
					if err != nil || a.IssueID > 0 || a.CommentID > 0 || a.ReleaseID > 0 {
						return issuetemplate.File{}, false
					}
					return issuetemplate.File{Name: a.Name, URL: attachmentURL(a.UUID), IsImage: isImageName(a.Name)}, true
				}
			}
			req.Content, err = tmpl.RenderForm(req.Fields, files)
			if fieldErrs := issuetemplate.FieldErrors(nil); errors.As(err, &fieldErrs) {
				fields := fieldErrors{}
				for _, fieldErr := range fieldErrs {
					msg := fieldErr.Label + ": " + fieldErr.Message
					if fieldErr.Missing {
						msg = fieldErr.Label + l.Tr("form.require_error")
					}
					fields["field-"+strconv.Itoa(fieldErr.Index)] = &msg
				}
				return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
			} else if err != nil {
				return http.StatusBadRequest, nil, err
			}
			for i, el := range tmpl.Form {
				if el.Type == issuetemplate.ElementUpload && files != nil {
					req.Attachments = append(req.Attachments, req.Fields[i]...)
				}
			}
		}
	}
	issue, labelIDs, statusCode, resp, err := newIssueFromRequest(l, viewer, repoCtx, req)
	if resp != nil || err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check new issue", "repoID", repo.ID, "error", err)
		}
		return statusCode, resp, err
	}
	if tmpl != nil && !repoCtx.Viewer.CanWrite() {
		// Like on GitHub, the labels and assignees of the template apply to
		// issues of anyone. Writers choose them on the form instead.
		if labelIDs, issue.AssigneeID, err = templateLabelsAndAssignee(repo, tmpl); err != nil {
			logger.Error("Failed to get template labels and assignee", "error", err)
			return http.StatusInternalServerError, nil, err
		}
	}

	// Creating the issue notifies watchers and webhooks about the repository
	// and its owner.
	repo.Owner = repoCtx.Owner
	if err = database.NewIssue(logger, repo, issue, labelIDs, req.Attachments); err != nil {
		logger.Error("Failed to create issue", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create issue")
	}
	return http.StatusCreated, map[string]int64{"index": issue.Index}, nil
}
