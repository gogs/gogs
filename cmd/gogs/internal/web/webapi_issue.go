package web

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
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
func withRepoIssue(c flamego.Context, repoCtx *repoContext) {
	injectRepoIssue(c, repoCtx, false)
}

// injectRepoIssue injects the issue or the pull request in the route, as isPull
// asks.
func injectRepoIssue(c flamego.Context, repoCtx *repoContext, isPull bool) {
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
		log.Error("injectRepoIssue: get issue %d of repository %d: %v", index, repo.ID, err)
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

func toIssueAttachments(attachments []*database.Attachment) []issueAttachment {
	items := make([]issueAttachment, 0, len(attachments))
	for _, a := range attachments {
		items = append(items, issueAttachment{
			Name:    a.Name,
			URL:     conf.Server.Subpath + "/attachments/" + a.UUID,
			IsImage: strings.HasPrefix(mime.TypeByExtension(filepath.Ext(a.Name)), "image/"),
		})
	}
	return items
}

type issueComment struct {
	ID int64 `json:"id"`
	// Type is "comment", "reopen", "close" or "commit_ref".
	Type      string    `json:"type"`
	Poster    issueUser `json:"poster"`
	CreatedAt time.Time `json:"createdAt"`
	// ContentHTML is the rendered content of a comment, or the link to the
	// commit of a commit reference.
	ContentHTML string `json:"contentHTML,omitempty"`
	// Content is the Markdown of a comment, only sent to viewers who can edit
	// it.
	Content string `json:"content,omitempty"`
	// Tag is how the poster of a comment relates to the repository: "owner",
	// "writer", "poster" or empty.
	Tag           string            `json:"tag,omitempty"`
	Attachments   []issueAttachment `json:"attachments,omitempty"`
	ViewerCanEdit bool              `json:"viewerCanEdit,omitempty"`
}

var issueCommentTypes = map[database.CommentType]string{
	database.CommentTypeComment:   "comment",
	database.CommentTypeReopen:    "reopen",
	database.CommentTypeClose:     "close",
	database.CommentTypeCommitRef: "commit_ref",
}

type issueAttachmentSettings struct {
	AllowedTypes []string `json:"allowedTypes"`
	// MaxSize is the largest file in bytes.
	MaxSize  int64 `json:"maxSize"`
	MaxFiles int   `json:"maxFiles"`
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

	Labels    []dashboardIssueLabel `json:"labels"`
	Milestone *repoIssueMilestone   `json:"milestone,omitempty"`
	Assignee  *repoIssueAssignee    `json:"assignee,omitempty"`
	// The options are only sent to writers.
	issueMetaOptions
	Participants []issueUser `json:"participants"`

	// Attachment is how files can be attached to comments, only sent to
	// signed-in viewers when attachments are enabled.
	Attachment *issueAttachmentSettings `json:"attachment,omitempty"`
	// ViewerCanEdit is whether the viewer can change the title, the content
	// and the state of the issue.
	ViewerCanEdit  bool `json:"viewerCanEdit"`
	ViewerCanWrite bool `json:"viewerCanWrite"`
}

func getRepoIssue(c flamego.Context, user *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp *repoIssueView, err error) {
	resp, err = toRepoIssueView(c.Request().Context(), user, repoCtx, issue)
	if err != nil {
		log.Error("getRepoIssue: describe issue %d of repository %d: %v", issue.ID, repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, resp, nil
}

// toRepoIssueView describes the issue or the pull request with its comments,
// and marks it read by the viewer.
func toRepoIssueView(ctx context.Context, user *database.User, repoCtx *repoContext, issue *database.Issue) (resp *repoIssueView, err error) {
	repo := repoCtx.Repo
	metas := repo.ComposeMetas()
	resp = &repoIssueView{
		Index:       issue.Index,
		Title:       issue.Title,
		IsClosed:    issue.IsClosed,
		Poster:      toIssueUser(issue.Poster),
		CreatedAt:   issue.Created.UTC(),
		ContentHTML: string(markup.Markdown(issue.Content, repo.Link(), metas)),
		Content:     issue.Content,
		Attachments: toIssueAttachments(issue.Attachments),
		Comments:    make([]issueComment, 0, len(issue.Comments)),
		Labels:      make([]dashboardIssueLabel, 0, len(issue.Labels)),

		ViewerCanEdit:  user != nil && viewerCanEditIssue(repoCtx, issue),
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}
	for _, label := range issue.Labels {
		resp.Labels = append(resp.Labels, toIssueLabel(label))
	}
	if issue.Milestone != nil {
		resp.Milestone = &repoIssueMilestone{ID: issue.Milestone.ID, Name: issue.Milestone.Name}
	}
	if issue.Assignee != nil {
		resp.Assignee = &repoIssueAssignee{ID: issue.Assignee.ID, Name: issue.Assignee.Name, AvatarURL: issue.Assignee.AvatarURLPath()}
	}

	tags := make(map[int64]string)
	participants := []*database.User{issue.Poster}
	for _, comment := range issue.Comments {
		typ, ok := issueCommentTypes[comment.Type]
		// Like the server-rendered page, references from comments and pull
		// requests are not shown.
		if !ok {
			continue
		}
		item := issueComment{
			ID:        comment.ID,
			Type:      typ,
			Poster:    toIssueUser(comment.Poster),
			CreatedAt: comment.Created.UTC(),
		}
		switch comment.Type {
		case database.CommentTypeCommitRef:
			item.ContentHTML = markup.Sanitize(comment.Content)
		case database.CommentTypeComment:
			item.ContentHTML = string(markup.Markdown(comment.Content, repo.Link(), metas))
			item.Attachments = toIssueAttachments(comment.Attachments)
			// Like the server-rendered page, the poster and admins can change a
			// comment.
			if user != nil && (comment.PosterID == user.ID || repoCtx.Viewer.CanAdminister()) {
				item.ViewerCanEdit = true
				item.Content = comment.Content
			}

			tag, ok := tags[comment.PosterID]
			if !ok {
				switch {
				case repo.IsOwnedBy(comment.PosterID) || (repoCtx.Owner.IsOrganization() && repoCtx.Owner.IsOwnedBy(comment.PosterID)):
					tag = "owner"
				case database.Handle.Permissions().Authorize(ctx, comment.PosterID, repo.ID, database.AccessModeWrite,
					database.AccessModeOptions{OwnerID: repo.OwnerID, Private: repo.IsPrivate}):
					tag = "writer"
				case comment.PosterID == issue.PosterID:
					tag = "poster"
				}
				tags[comment.PosterID] = tag
			}
			item.Tag = tag

			if !slices.ContainsFunc(participants, func(u *database.User) bool { return u.ID == comment.Poster.ID }) {
				participants = append(participants, comment.Poster)
			}
		}
		resp.Comments = append(resp.Comments, item)
	}
	resp.Participants = make([]issueUser, 0, len(participants))
	for _, u := range participants {
		resp.Participants = append(resp.Participants, toIssueUser(u))
	}

	if resp.ViewerCanWrite {
		if resp.issueMetaOptions, err = listIssueMetaOptions(repo); err != nil {
			return nil, errors.Wrap(err, "list what can be set on issues")
		}
	}

	if user != nil {
		resp.Attachment = attachmentSettings(issueAttachmentLimits())
		if err = issue.ReadBy(user.ID); err != nil {
			return nil, errors.Wrap(err, "mark as read")
		}
	}
	return resp, nil
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
}

func patchRepoIssue(l i18n.Locale, user *database.User, issue *database.Issue, req repoIssueRequest) (statusCode int, resp any, err error) {
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if resp := textFieldError(l, "title", "repo.milestones.title", title, 255); resp != nil {
			return http.StatusBadRequest, resp, nil
		}
		if title != issue.Title {
			if err = issue.ChangeTitle(user, title); err != nil {
				log.Error("patchRepoIssue: change title of issue %d: %v", issue.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "change title")
			}
		}
	}
	if req.Content != nil && *req.Content != issue.Content {
		if err = issue.ChangeContent(user, *req.Content); err != nil {
			log.Error("patchRepoIssue: change content of issue %d: %v", issue.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "change content")
		}
	}
	return http.StatusNoContent, nil, nil
}

func setRepoIssueClosed(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, issue *database.Issue, closed bool) (statusCode int, resp any, err error) {
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
				log.Error("setRepoIssueClosed: get open pull request like %d: %v", pull.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "get open pull request")
			} else if other != nil {
				return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("repo.pulls.open_unmerged_pull_exists", other.Index)}, nil
			}
			if err = pull.UpdatePatch(c.Request().Context()); err != nil {
				log.Error("setRepoIssueClosed: update patch of pull request %d: %v", pull.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "update patch")
			}
			pull.AddToTaskQueue()
		}
	}
	if err = issue.ChangeStatus(user, repoCtx.Repo, closed); err != nil {
		log.Error("setRepoIssueClosed: change state of issue %d: %v", issue.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change state")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoIssueClosed(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return setRepoIssueClosed(c, l, user, repoCtx, issue, true)
}

func deleteRepoIssueClosed(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	return setRepoIssueClosed(c, l, user, repoCtx, issue, false)
}

type repoIssueLabelsRequest struct {
	IDs []int64 `json:"ids"`
}

// putRepoIssueLabels replaces the labels of the issue.
func putRepoIssueLabels(user *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueLabelsRequest) (statusCode int, resp any, err error) {
	labels := make([]*database.Label, 0, len(req.IDs))
	for _, id := range req.IDs {
		label, err := database.GetLabelOfRepoByID(repoCtx.Repo.ID, id)
		if err != nil {
			if database.IsErrLabelNotExist(err) {
				return http.StatusBadRequest, nil, errors.Newf("label %d does not exist", id)
			}
			log.Error("putRepoIssueLabels: get label %d: %v", id, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get label")
		}
		labels = append(labels, label)
	}

	for _, label := range slices.Clone(issue.Labels) {
		if slices.ContainsFunc(labels, func(l *database.Label) bool { return l.ID == label.ID }) {
			continue
		}
		if err = issue.RemoveLabel(user, label); err != nil {
			log.Error("putRepoIssueLabels: remove label %d from issue %d: %v", label.ID, issue.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "remove label")
		}
	}
	for _, label := range labels {
		if issue.HasLabel(label.ID) {
			continue
		}
		if err = issue.AddLabel(user, label); err != nil {
			log.Error("putRepoIssueLabels: add label %d to issue %d: %v", label.ID, issue.ID, err)
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

func putRepoIssueMilestone(user *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueIDRequest) (statusCode int, resp any, err error) {
	if req.ID == issue.MilestoneID {
		return http.StatusNoContent, nil, nil
	}
	if statusCode, err = checkIssueMilestone(repoCtx.Repo, req.ID); err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("putRepoIssueMilestone: %v", err)
		}
		return statusCode, nil, err
	}
	oldID := issue.MilestoneID
	issue.MilestoneID = req.ID
	if err = database.ChangeMilestoneAssign(user, issue, oldID); err != nil {
		log.Error("putRepoIssueMilestone: change milestone of issue %d: %v", issue.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change milestone")
	}
	return http.StatusNoContent, nil, nil
}

func putRepoIssueAssignee(user *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueIDRequest) (statusCode int, resp any, err error) {
	if req.ID == issue.AssigneeID {
		return http.StatusNoContent, nil, nil
	}
	if statusCode, err = checkIssueAssignee(repoCtx.Repo, req.ID); err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("putRepoIssueAssignee: %v", err)
		}
		return statusCode, nil, err
	}
	if err = issue.ChangeAssignee(user, req.ID); err != nil {
		log.Error("putRepoIssueAssignee: change assignee of issue %d: %v", issue.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change assignee")
	}
	return http.StatusNoContent, nil, nil
}

type repoIssueCommentRequest struct {
	Content string `json:"content"`
	// Attachments are the UUIDs of the uploaded files to attach.
	Attachments []string `json:"attachments"`
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

func postRepoIssueComment(l i18n.Locale, user *database.User, repoCtx *repoContext, issue *database.Issue, req repoIssueCommentRequest) (statusCode int, resp any, err error) {
	// Like the server-rendered page, a comment can be only attachments.
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		msg := l.Tr("form.Content") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if statusCode, err = checkNewAttachments(issueAttachmentLimits(), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("postRepoIssueComment: %v", err)
		}
		return statusCode, nil, err
	}

	comment, err := database.CreateIssueComment(user, repoCtx.Repo, issue, req.Content, req.Attachments)
	if err != nil {
		log.Error("postRepoIssueComment: create comment on issue %d: %v", issue.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create comment")
	}
	return http.StatusCreated, map[string]int64{"id": comment.ID}, nil
}

// withIssueComment injects the comment of the issue in the route, which only
// its poster and admins can change, like on the server-rendered page.
func withIssueComment(c flamego.Context, repoCtx *repoContext, issue *database.Issue) {
	w := c.ResponseWriter()
	comment, err := database.GetCommentByID(idParam(c))
	if err != nil {
		if database.IsErrCommentNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("comment does not exist"))
			return
		}
		log.Error("withIssueComment: get comment: %v", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get comment"))
		return
	}
	if comment.IssueID != issue.ID || comment.Type != database.CommentTypeComment ||
		(comment.PosterID != repoCtx.Viewer.ID() && !repoCtx.Viewer.CanAdminister()) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("comment does not exist"))
		return
	}
	c.Map(comment)
}

func patchRepoIssueComment(l i18n.Locale, user *database.User, comment *database.Comment, req repoIssueCommentRequest) (statusCode int, resp any, err error) {
	if strings.TrimSpace(req.Content) == "" {
		msg := l.Tr("form.Content") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	oldContent := comment.Content
	comment.Content = req.Content
	if err = database.UpdateComment(user, comment, oldContent); err != nil {
		log.Error("patchRepoIssueComment: update comment %d: %v", comment.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update comment")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoIssueComment(user *database.User, comment *database.Comment) (statusCode int, resp any, err error) {
	if err = database.DeleteCommentByID(user, comment.ID); err != nil {
		log.Error("deleteRepoIssueComment: delete comment %d: %v", comment.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete comment")
	}
	return http.StatusNoContent, nil, nil
}

type markdownRequest struct {
	Text string `json:"text"`
}

// postRepoMarkdown renders Markdown the way the issues of the repository are.
func postRepoMarkdown(repoCtx *repoContext, req markdownRequest) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	return http.StatusOK, map[string]string{"html": string(markup.Markdown(req.Text, repo.Link(), repo.ComposeMetas()))}, nil
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
func storeAttachment(c flamego.Context, l i18n.Locale, lim attachmentLimits) (statusCode int, resp any, err error) {
	f, statusCode, err := readUploadedFile(c, l, lim)
	if err != nil {
		return statusCode, nil, err
	}
	defer func() { _ = f.File.Close() }()

	attachment, err := database.NewAttachment(f.Header.Filename, f.Head, f.File)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "store attachment")
	}
	return http.StatusCreated, map[string]string{"uuid": attachment.UUID}, nil
}

// postRepoIssueAttachment stores a file to attach to an issue or a comment.
func postRepoIssueAttachment(c flamego.Context, l i18n.Locale) (statusCode int, resp any, err error) {
	statusCode, resp, err = storeAttachment(c, l, issueAttachmentLimits())
	if statusCode == http.StatusInternalServerError {
		log.Error("postRepoIssueAttachment: %v", err)
	}
	return statusCode, resp, err
}

type repoNewIssueOptions struct {
	// DefaultBranch is where the issue template is read from, only sent to
	// viewers who can read the repository.
	DefaultBranch string `json:"defaultBranch,omitempty"`
	// The options are only sent to writers.
	issueMetaOptions
	Attachment     *issueAttachmentSettings `json:"attachment,omitempty"`
	ViewerCanWrite bool                     `json:"viewerCanWrite"`
}

func getRepoNewIssue(repoCtx *repoContext) (statusCode int, resp *repoNewIssueOptions, err error) {
	repo := repoCtx.Repo
	resp = &repoNewIssueOptions{
		Attachment:     attachmentSettings(issueAttachmentLimits()),
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}
	if repoCtx.Viewer.CanRead() && !repo.IsEmpty {
		resp.DefaultBranch = repo.DefaultBranch
	}
	if resp.ViewerCanWrite {
		if resp.issueMetaOptions, err = listIssueMetaOptions(repo); err != nil {
			log.Error("getRepoNewIssue: list what can be set on issues of repository %d: %v", repo.ID, err)
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
}

// newIssueFromRequest checks the form to open an issue or a pull request, and
// returns the issue to create with its labels. A non-nil resp or err is what to
// answer with instead.
func newIssueFromRequest(l i18n.Locale, user *database.User, repoCtx *repoContext, req repoNewIssueRequest) (issue *database.Issue, labelIDs []int64, statusCode int, resp any, err error) {
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
		PosterID: user.ID,
		Poster:   user,
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

func postRepoIssue(l i18n.Locale, user *database.User, repoCtx *repoContext, req repoNewIssueRequest) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	issue, labelIDs, statusCode, resp, err := newIssueFromRequest(l, user, repoCtx, req)
	if resp != nil || err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("postRepoIssue: %v", err)
		}
		return statusCode, resp, err
	}

	// Creating the issue notifies watchers and webhooks about the repository
	// and its owner.
	repo.Owner = repoCtx.Owner
	if err = database.NewIssue(repo, issue, labelIDs, req.Attachments); err != nil {
		log.Error("postRepoIssue: create issue in repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create issue")
	}
	return http.StatusCreated, map[string]int64{"index": issue.Index}, nil
}
