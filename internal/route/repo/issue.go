// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unknwon/com"
	"github.com/unknwon/paginater"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/database/errors"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/tool"
)

const (
	ISSUES     = "repo/issue/list"
	ISSUE_NEW  = "repo/issue/new"
	ISSUE_VIEW = "repo/issue/view"

	LABELS = "repo/issue/labels"

	MILESTONE      = "repo/issue/milestones"
	MILESTONE_NEW  = "repo/issue/milestone_new"
	MILESTONE_EDIT = "repo/issue/milestone_edit"

	ISSUE_TEMPLATE_KEY = "IssueTemplate"
)

var (
	ErrFileTypeForbidden = errors.New("File type is not allowed")
	ErrTooManyFiles      = errors.New("Maximum number of files to upload exceeded")

	IssueTemplateCandidates = []string{
		"ISSUE_TEMPLATE.md",
		".gogs/ISSUE_TEMPLATE.md",
		".github/ISSUE_TEMPLATE.md",
	}
)

func MustEnableIssues(c *context.Context) {
	if !c.Repo.Repository.EnableIssues {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalTracker {
		c.Redirect(c.Repo.Repository.ExternalTrackerURL)
		return
	}
}

func MustAllowPulls(c *context.Context) {
	if !c.Repo.Repository.AllowsPulls() {
		c.NotFound()
		return
	}

	// User can send pull request if owns a forked repository.
	if c.IsLogged && database.Repos.HasForkedBy(c.Req.Context(), c.Repo.Repository.ID, c.User.ID) {
		c.Repo.PullRequest.Allowed = true
		c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
	}
}

func RetrieveLabels(c *context.Context) {
	labels, err := database.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return
	}
	for _, l := range labels {
		l.CalOpenIssues()
	}
	c.Data["Labels"] = labels
	c.Data["NumLabels"] = len(labels)
}

func issues(c *context.Context, isPullList bool) {
	if isPullList {
		MustAllowPulls(c)
		if c.Written() {
			return
		}
		c.Data["Title"] = c.Tr("repo.pulls")
		c.Data["PageIsPullList"] = true

	} else {
		MustEnableIssues(c)
		if c.Written() {
			return
		}
		c.Data["Title"] = c.Tr("repo.issues")
		c.Data["PageIsIssueList"] = true
	}

	viewType := c.Query("type")
	sortType := c.Query("sort")
	types := []string{"assigned", "created_by", "mentioned"}
	if !com.IsSliceContainsStr(types, viewType) {
		viewType = "all"
	}

	// Must sign in to see issues about you.
	if viewType != "all" && !c.IsLogged {
		c.SetCookie("redirect_to", "/"+url.QueryEscape(conf.Server.Subpath+c.Req.RequestURI), 0, conf.Server.Subpath)
		c.Redirect(conf.Server.Subpath + "/user/login")
		return
	}

	var (
		assigneeID = c.QueryInt64("assignee")
		posterID   int64
	)
	filterMode := database.FILTER_MODE_YOUR_REPOS
	switch viewType {
	case "assigned":
		filterMode = database.FILTER_MODE_ASSIGN
		assigneeID = c.User.ID
	case "created_by":
		filterMode = database.FILTER_MODE_CREATE
		posterID = c.User.ID
	case "mentioned":
		filterMode = database.FILTER_MODE_MENTION
	}

	var uid int64 = -1
	if c.IsLogged {
		uid = c.User.ID
	}

	repo := c.Repo.Repository
	selectLabels := c.Query("labels")
	milestoneID := c.QueryInt64("milestone")
	isShowClosed := c.Query("state") == "closed"
	issueStats := database.GetIssueStats(&database.IssueStatsOptions{
		RepoID:      repo.ID,
		UserID:      uid,
		Labels:      selectLabels,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		FilterMode:  filterMode,
		IsPull:      isPullList,
	})

	page := c.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(issueStats.OpenCount)
	} else {
		total = int(issueStats.ClosedCount)
	}
	pager := paginater.New(total, conf.UI.IssuePagingNum, page, 5)
	c.Data["Page"] = pager

	issues, err := database.Issues(&database.IssuesOptions{
		UserID:      uid,
		AssigneeID:  assigneeID,
		RepoID:      repo.ID,
		PosterID:    posterID,
		MilestoneID: milestoneID,
		Page:        pager.Current(),
		IsClosed:    isShowClosed,
		IsMention:   filterMode == database.FILTER_MODE_MENTION,
		IsPull:      isPullList,
		Labels:      selectLabels,
		SortType:    sortType,
	})
	if err != nil {
		c.Error(err, "list issues")
		return
	}

	// Get issue-user relations.
	pairs, err := database.GetIssueUsers(repo.ID, posterID, isShowClosed)
	if err != nil {
		c.Error(err, "get issue-user relations")
		return
	}

	// Get posters.
	for i := range issues {
		if !c.IsLogged {
			issues[i].IsRead = true
			continue
		}

		// Check read status.
		idx := database.PairsContains(pairs, issues[i].ID, c.User.ID)
		if idx > -1 {
			issues[i].IsRead = pairs[idx].IsRead
		} else {
			issues[i].IsRead = true
		}
	}
	c.Data["Issues"] = issues

	// Get milestones.
	c.Data["Milestones"], err = database.GetMilestonesByRepoID(repo.ID)
	if err != nil {
		c.Error(err, "get milestone by repository ID")
		return
	}

	// Get assignees.
	c.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		c.Error(err, "get assignees")
		return
	}

	if viewType == "assigned" {
		assigneeID = 0 // Reset ID to prevent unexpected selection of assignee.
	}

	c.Data["IssueStats"] = issueStats
	c.Data["SelectLabels"] = com.StrTo(selectLabels).MustInt64()
	c.Data["ViewType"] = viewType
	c.Data["SortType"] = sortType
	c.Data["MilestoneID"] = milestoneID
	c.Data["AssigneeID"] = assigneeID
	c.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		c.Data["State"] = "closed"
	} else {
		c.Data["State"] = "open"
	}

	c.Success(ISSUES)
}

func Issues(c *context.Context) {
	issues(c, false)
}

func Pulls(c *context.Context) {
	issues(c, true)
}

func renderAttachmentSettings(c *context.Context) {
	c.Data["RequireDropzone"] = true
	c.Data["IsAttachmentEnabled"] = conf.Attachment.Enabled
	c.Data["AttachmentAllowedTypes"] = conf.Attachment.AllowedTypes
	c.Data["AttachmentMaxSize"] = conf.Attachment.MaxSize
	c.Data["AttachmentMaxFiles"] = conf.Attachment.MaxFiles
}

func RetrieveRepoMilestonesAndAssignees(c *context.Context, repo *database.Repository) {
	var err error
	c.Data["OpenMilestones"], err = database.GetMilestones(repo.ID, -1, false)
	if err != nil {
		c.Error(err, "get open milestones")
		return
	}
	c.Data["ClosedMilestones"], err = database.GetMilestones(repo.ID, -1, true)
	if err != nil {
		c.Error(err, "get closed milestones")
		return
	}

	c.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		c.Error(err, "get assignees")
		return
	}
}

func RetrieveRepoMetas(c *context.Context, repo *database.Repository) []*database.Label {
	if !c.Repo.IsWriter() {
		return nil
	}

	labels, err := database.GetLabelsByRepoID(repo.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return nil
	}
	c.Data["Labels"] = labels

	RetrieveRepoMilestonesAndAssignees(c, repo)
	if c.Written() {
		return nil
	}

	return labels
}

func getFileContentFromDefaultBranch(c *context.Context, filename string) (string, bool) {
	if c.Repo.Commit == nil {
		var err error
		c.Repo.Commit, err = c.Repo.GitRepo.BranchCommit(c.Repo.Repository.DefaultBranch)
		if err != nil {
			return "", false
		}
	}

	entry, err := c.Repo.Commit.TreeEntry(filename)
	if err != nil {
		return "", false
	}
	p, err := entry.Blob().Bytes()
	if err != nil {
		return "", false
	}
	return string(p), true
}

func setTemplateIfExists(c *context.Context, ctxDataKey string, possibleFiles []string) {
	for _, filename := range possibleFiles {
		content, found := getFileContentFromDefaultBranch(c, filename)
		if found {
			c.Data[ctxDataKey] = content
			return
		}
	}
}

func NewIssue(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.issues.new")
	c.Data["PageIsIssueList"] = true
	c.Data["RequireHighlightJS"] = true
	c.Data["RequireSimpleMDE"] = true
	c.Data["title"] = c.Query("title")
	c.Data["content"] = c.Query("content")
	setTemplateIfExists(c, ISSUE_TEMPLATE_KEY, IssueTemplateCandidates)
	renderAttachmentSettings(c)

	RetrieveRepoMetas(c, c.Repo.Repository)
	if c.Written() {
		return
	}

	c.Success(ISSUE_NEW)
}

func ValidateRepoMetas(c *context.Context, f form.NewIssue) ([]int64, int64, int64) {
	var (
		repo = c.Repo.Repository
		err  error
	)

	labels := RetrieveRepoMetas(c, c.Repo.Repository)
	if c.Written() {
		return nil, 0, 0
	}

	if !c.Repo.IsWriter() {
		return nil, 0, 0
	}

	// Check labels.
	labelIDs := tool.StringsToInt64s(strings.Split(f.LabelIDs, ","))
	labelIDMark := tool.Int64sToMap(labelIDs)
	hasSelected := false
	for i := range labels {
		if labelIDMark[labels[i].ID] {
			labels[i].IsChecked = true
			hasSelected = true
		}
	}
	c.Data["HasSelectedLabel"] = hasSelected
	c.Data["label_ids"] = f.LabelIDs
	c.Data["Labels"] = labels

	// Check milestone.
	milestoneID := f.MilestoneID
	if milestoneID > 0 {
		c.Data["Milestone"], err = repo.GetMilestoneByID(milestoneID)
		if err != nil {
			c.Error(err, "get milestone by ID")
			return nil, 0, 0
		}
		c.Data["milestone_id"] = milestoneID
	}

	// Check assignee.
	assigneeID := f.AssigneeID
	if assigneeID > 0 {
		c.Data["Assignee"], err = repo.GetAssigneeByID(assigneeID)
		if err != nil {
			c.Error(err, "get assignee by ID")
			return nil, 0, 0
		}
		c.Data["assignee_id"] = assigneeID
	}

	return labelIDs, milestoneID, assigneeID
}

func NewIssuePost(c *context.Context, f form.NewIssue) {
	c.Data["Title"] = c.Tr("repo.issues.new")
	c.Data["PageIsIssueList"] = true
	c.Data["RequireHighlightJS"] = true
	c.Data["RequireSimpleMDE"] = true
	renderAttachmentSettings(c)

	labelIDs, milestoneID, assigneeID := ValidateRepoMetas(c, f)
	if c.Written() {
		return
	}

	if c.HasError() {
		c.Success(ISSUE_NEW)
		return
	}

	var attachments []string
	if conf.Attachment.Enabled {
		attachments = f.Files
	}

	issue := &database.Issue{
		RepoID:      c.Repo.Repository.ID,
		Title:       f.Title,
		PosterID:    c.User.ID,
		Poster:      c.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		Content:     f.Content,
	}
	if err := database.NewIssue(c.Repo.Repository, issue, labelIDs, attachments); err != nil {
		c.Error(err, "new issue")
		return
	}

	log.Trace("Issue created: %d/%d", c.Repo.Repository.ID, issue.ID)
	c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("issues/%d", issue.Index)))
}

func uploadAttachment(c *context.Context, allowedTypes []string) {
	file, header, err := c.Req.FormFile("file")
	if err != nil {
		c.Error(err, "get file")
		return
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, _ := file.Read(buf)
	if n > 0 {
		buf = buf[:n]
	}
	fileType := http.DetectContentType(buf)

	allowed := false
	for _, t := range allowedTypes {
		t := strings.Trim(t, " ")
		if t == "*/*" || t == fileType {
			allowed = true
			break
		}
	}

	if !allowed {
		c.PlainText(http.StatusBadRequest, ErrFileTypeForbidden.Error())
		return
	}

	attach, err := database.NewAttachment(header.Filename, buf, file)
	if err != nil {
		c.Error(err, "new attachment")
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	c.JSONSuccess(map[string]string{
		"uuid": attach.UUID,
	})
}

func UploadIssueAttachment(c *context.Context) {
	if !conf.Attachment.Enabled {
		c.NotFound()
		return
	}

	uploadAttachment(c, conf.Attachment.AllowedTypes)
}

func viewIssue(c *context.Context, isPullList bool) {
	c.Data["RequireHighlightJS"] = true
	c.Data["RequireDropzone"] = true
	renderAttachmentSettings(c)

	index := c.ParamsInt64(":index")
	if index <= 0 {
		c.NotFound()
		return
	}

	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	c.Data["Title"] = issue.Title

	// Make sure type and URL matches.
	if !isPullList && issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("pulls/%d", issue.Index)))
		return
	} else if isPullList && !issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("issues/%d", issue.Index)))
		return
	}

	if issue.IsPull {
		MustAllowPulls(c)
		if c.Written() {
			return
		}
		c.Data["PageIsPullList"] = true
		c.Data["PageIsPullConversation"] = true
	} else {
		MustEnableIssues(c)
		if c.Written() {
			return
		}
		c.Data["PageIsIssueList"] = true
	}

	issue.RenderedContent = string(markup.Markdown(issue.Content, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))

	repo := c.Repo.Repository

	// Get more information if it's a pull request.
	if issue.IsPull {
		if issue.PullRequest.HasMerged {
			c.Data["DisableStatusChange"] = issue.PullRequest.HasMerged
			PrepareMergedViewPullInfo(c, issue)
		} else {
			PrepareViewPullInfo(c, issue)
		}
		if c.Written() {
			return
		}
	}

	// Metas.
	// Check labels.
	labelIDMark := make(map[int64]bool)
	for i := range issue.Labels {
		labelIDMark[issue.Labels[i].ID] = true
	}
	labels, err := database.GetLabelsByRepoID(repo.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return
	}
	hasSelected := false
	for i := range labels {
		if labelIDMark[labels[i].ID] {
			labels[i].IsChecked = true
			hasSelected = true
		}
	}
	c.Data["HasSelectedLabel"] = hasSelected
	c.Data["Labels"] = labels

	// Check milestone and assignee.
	if c.Repo.IsWriter() {
		RetrieveRepoMilestonesAndAssignees(c, repo)
		if c.Written() {
			return
		}
	}

	if c.IsLogged {
		// Update issue-user.
		if err = issue.ReadBy(c.User.ID); err != nil {
			c.Error(err, "mark read by")
			return
		}
	}

	var (
		tag          database.CommentTag
		ok           bool
		marked       = make(map[int64]database.CommentTag)
		comment      *database.Comment
		participants = make([]*database.User, 1, 10)
	)

	// Render comments and and fetch participants.
	participants[0] = issue.Poster
	for _, comment = range issue.Comments {
		if comment.Type == database.COMMENT_TYPE_COMMENT {
			comment.RenderedContent = string(markup.Markdown(comment.Content, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))

			// Check tag.
			tag, ok = marked[comment.PosterID]
			if ok {
				comment.ShowTag = tag
				continue
			}

			if repo.IsOwnedBy(comment.PosterID) ||
				(repo.Owner.IsOrganization() && repo.Owner.IsOwnedBy(comment.PosterID)) {
				comment.ShowTag = database.COMMENT_TAG_OWNER
			} else if database.Perms.Authorize(
				c.Req.Context(),
				comment.PosterID,
				repo.ID,
				database.AccessModeWrite,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			) {
				comment.ShowTag = database.COMMENT_TAG_WRITER
			} else if comment.PosterID == issue.PosterID {
				comment.ShowTag = database.COMMENT_TAG_POSTER
			}

			marked[comment.PosterID] = comment.ShowTag

			isAdded := false
			for j := range participants {
				if comment.Poster == participants[j] {
					isAdded = true
					break
				}
			}
			if !isAdded && !issue.IsPoster(comment.Poster.ID) {
				participants = append(participants, comment.Poster)
			}
		}
	}

	if issue.IsPull && issue.PullRequest.HasMerged {
		pull := issue.PullRequest
		branchProtected := false
		protectBranch, err := database.GetProtectBranchOfRepoByName(pull.BaseRepoID, pull.HeadBranch)
		if err != nil {
			if !database.IsErrBranchNotExist(err) {
				c.Error(err, "get protect branch of repository by name")
				return
			}
		} else {
			branchProtected = protectBranch.Protected
		}

		c.Data["IsPullBranchDeletable"] = pull.BaseRepoID == pull.HeadRepoID &&
			c.Repo.IsWriter() && c.Repo.GitRepo.HasBranch(pull.HeadBranch) &&
			!branchProtected

		c.Data["DeleteBranchLink"] = c.Repo.MakeURL(url.URL{
			Path:     "branches/delete/" + pull.HeadBranch,
			RawQuery: fmt.Sprintf("commit=%s&redirect_to=%s", pull.MergedCommitID, c.Data["Link"]),
		})
	}

	c.Data["Participants"] = participants
	c.Data["NumParticipants"] = len(participants)
	c.Data["Issue"] = issue
	c.Data["IsIssueOwner"] = c.Repo.IsWriter() || (c.IsLogged && issue.IsPoster(c.User.ID))
	c.Data["SignInLink"] = conf.Server.Subpath + "/user/login?redirect_to=" + c.Data["Link"].(string)
	c.Success(ISSUE_VIEW)
}

func ViewIssue(c *context.Context) {
	viewIssue(c, false)
}

func ViewPull(c *context.Context) {
	viewIssue(c, true)
}

func getActionIssue(c *context.Context) *database.Issue {
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return nil
	}

	// Prevent guests accessing pull requests
	if !c.Repo.HasAccess() && issue.IsPull {
		c.NotFound()
		return nil
	}

	return issue
}

func UpdateIssueTitle(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	if !c.IsLogged || (!issue.IsPoster(c.User.ID) && !c.Repo.IsWriter()) {
		c.Status(http.StatusForbidden)
		return
	}

	title := c.QueryTrim("title")
	if title == "" {
		c.Status(http.StatusNoContent)
		return
	}

	if err := issue.ChangeTitle(c.User, title); err != nil {
		c.Error(err, "change title")
		return
	}

	c.JSONSuccess(map[string]any{
		"title": issue.Title,
	})
}

func UpdateIssueContent(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	if !c.IsLogged || (c.User.ID != issue.PosterID && !c.Repo.IsWriter()) {
		c.Status(http.StatusForbidden)
		return
	}

	content := c.Query("content")
	if err := issue.ChangeContent(c.User, content); err != nil {
		c.Error(err, "change content")
		return
	}

	c.JSONSuccess(map[string]string{
		"content": string(markup.Markdown(issue.Content, c.Query("context"), c.Repo.Repository.ComposeMetas())),
	})
}

func UpdateIssueLabel(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	if c.Query("action") == "clear" {
		if err := issue.ClearLabels(c.User); err != nil {
			c.Error(err, "clear labels")
			return
		}
	} else {
		isAttach := c.Query("action") == "attach"
		label, err := database.GetLabelOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id"))
		if err != nil {
			c.NotFoundOrError(err, "get label by ID")
			return
		}

		if isAttach && !issue.HasLabel(label.ID) {
			if err = issue.AddLabel(c.User, label); err != nil {
				c.Error(err, "add label")
				return
			}
		} else if !isAttach && issue.HasLabel(label.ID) {
			if err = issue.RemoveLabel(c.User, label); err != nil {
				c.Error(err, "remove label")
				return
			}
		}
	}

	c.JSONSuccess(map[string]any{
		"ok": true,
	})
}

func UpdateIssueMilestone(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	oldMilestoneID := issue.MilestoneID
	milestoneID := c.QueryInt64("id")
	if oldMilestoneID == milestoneID {
		c.JSONSuccess(map[string]any{
			"ok": true,
		})
		return
	}

	// Not check for invalid milestone id and give responsibility to owners.
	issue.MilestoneID = milestoneID
	if err := database.ChangeMilestoneAssign(c.User, issue, oldMilestoneID); err != nil {
		c.Error(err, "change milestone assign")
		return
	}

	c.JSONSuccess(map[string]any{
		"ok": true,
	})
}

func UpdateIssueAssignee(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	assigneeID := c.QueryInt64("id")
	if issue.AssigneeID == assigneeID {
		c.JSONSuccess(map[string]any{
			"ok": true,
		})
		return
	}

	if err := issue.ChangeAssignee(c.User, assigneeID); err != nil {
		c.Error(err, "change assignee")
		return
	}

	c.JSONSuccess(map[string]any{
		"ok": true,
	})
}

func NewComment(c *context.Context, f form.CreateComment) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	var attachments []string
	if conf.Attachment.Enabled {
		attachments = f.Files
	}

	if c.HasError() {
		c.Flash.Error(c.Data["ErrorMsg"].(string))
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("issues/%d", issue.Index)))
		return
	}

	var err error
	var comment *database.Comment
	defer func() {
		// Check if issue admin/poster changes the status of issue.
		if (c.Repo.IsWriter() || (c.IsLogged && issue.IsPoster(c.User.ID))) &&
			(f.Status == "reopen" || f.Status == "close") &&
			!(issue.IsPull && issue.PullRequest.HasMerged) {

			// Duplication and conflict check should apply to reopen pull request.
			var pr *database.PullRequest

			if f.Status == "reopen" && issue.IsPull {
				pull := issue.PullRequest
				pr, err = database.GetUnmergedPullRequest(pull.HeadRepoID, pull.BaseRepoID, pull.HeadBranch, pull.BaseBranch)
				if err != nil {
					if !database.IsErrPullRequestNotExist(err) {
						c.Error(err, "get unmerged pull request")
						return
					}
				}

				// Regenerate patch and test conflict.
				if pr == nil {
					if err = issue.PullRequest.UpdatePatch(); err != nil {
						c.Error(err, "update patch")
						return
					}

					issue.PullRequest.AddToTaskQueue()
				}
			}

			if pr != nil {
				c.Flash.Info(c.Tr("repo.pulls.open_unmerged_pull_exists", pr.Index))
			} else {
				if err = issue.ChangeStatus(c.User, c.Repo.Repository, f.Status == "close"); err != nil {
					log.Error("ChangeStatus: %v", err)
				} else {
					log.Trace("Issue [%d] status changed to closed: %v", issue.ID, issue.IsClosed)
				}
			}
		}

		// Redirect to comment hashtag if there is any actual content.
		typeName := "issues"
		if issue.IsPull {
			typeName = "pulls"
		}

		location := url.URL{
			Path: fmt.Sprintf("%s/%d", typeName, issue.Index),
		}

		if comment != nil {
			location.Fragment = comment.HashTag()
		}

		c.RawRedirect(c.Repo.MakeURL(location))
	}()

	// Fix #321: Allow empty comments, as long as we have attachments.
	if f.Content == "" && len(attachments) == 0 {
		return
	}

	comment, err = database.CreateIssueComment(c.User, c.Repo.Repository, issue, f.Content, attachments)
	if err != nil {
		c.Error(err, "create issue comment")
		return
	}

	log.Trace("Comment created: %d/%d/%d", c.Repo.Repository.ID, issue.ID, comment.ID)
}

func UpdateCommentContent(c *context.Context) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.UserID() != comment.PosterID && !c.Repo.IsAdmin() {
		c.NotFound()
		return
	} else if comment.Type != database.COMMENT_TYPE_COMMENT {
		c.Status(http.StatusNoContent)
		return
	}

	oldContent := comment.Content
	comment.Content = c.Query("content")
	if comment.Content == "" {
		c.JSONSuccess(map[string]any{
			"content": "",
		})
		return
	}
	if err = database.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Error(err, "update comment")
		return
	}

	c.JSONSuccess(map[string]string{
		"content": string(markup.Markdown(comment.Content, c.Query("context"), c.Repo.Repository.ComposeMetas())),
	})
}

func DeleteComment(c *context.Context) {
	comment, err := database.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get comment by ID")
		return
	}

	if c.UserID() != comment.PosterID && !c.Repo.IsAdmin() {
		c.NotFound()
		return
	} else if comment.Type != database.COMMENT_TYPE_COMMENT {
		c.Status(http.StatusNoContent)
		return
	}

	if err = database.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Error(err, "delete comment by ID")
		return
	}

	c.Status(http.StatusOK)
}

func Labels(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.labels")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsLabels"] = true
	c.Data["RequireMinicolors"] = true
	c.Data["LabelTemplates"] = database.LabelTemplates
	c.Success(LABELS)
}

func InitializeLabels(c *context.Context, f form.InitializeLabels) {
	if c.HasError() {
		c.RawRedirect(c.Repo.MakeURL("labels"))
		return
	}
	list, err := database.GetLabelTemplateFile(f.TemplateName)
	if err != nil {
		c.Flash.Error(c.Tr("repo.issues.label_templates.fail_to_load_file", f.TemplateName, err))
		c.RawRedirect(c.Repo.MakeURL("labels"))
		return
	}

	labels := make([]*database.Label, len(list))
	for i := 0; i < len(list); i++ {
		labels[i] = &database.Label{
			RepoID: c.Repo.Repository.ID,
			Name:   list[i][0],
			Color:  list[i][1],
		}
	}
	if err := database.NewLabels(labels...); err != nil {
		c.Error(err, "new labels")
		return
	}
	c.RawRedirect(c.Repo.MakeURL("labels"))
}

func NewLabel(c *context.Context, f form.CreateLabel) {
	c.Data["Title"] = c.Tr("repo.labels")
	c.Data["PageIsLabels"] = true

	if c.HasError() {
		c.Flash.Error(c.Data["ErrorMsg"].(string))
		c.RawRedirect(c.Repo.MakeURL("labels"))
		return
	}

	l := &database.Label{
		RepoID: c.Repo.Repository.ID,
		Name:   f.Title,
		Color:  f.Color,
	}
	if err := database.NewLabels(l); err != nil {
		c.Error(err, "new labels")
		return
	}
	c.RawRedirect(c.Repo.MakeURL("labels"))
}

func UpdateLabel(c *context.Context, f form.CreateLabel) {
	l, err := database.GetLabelByID(f.ID)
	if err != nil {
		c.NotFoundOrError(err, "get label by ID")
		return
	}

	l.Name = f.Title
	l.Color = f.Color
	if err := database.UpdateLabel(l); err != nil {
		c.Error(err, "update label")
		return
	}
	c.RawRedirect(c.Repo.MakeURL("labels"))
}

func DeleteLabel(c *context.Context) {
	if err := database.DeleteLabel(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteLabel: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.issues.label_deletion_success"))
	}

	c.JSONSuccess(map[string]any{
		"redirect": c.Repo.MakeURL("labels"),
	})
}

func Milestones(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true

	isShowClosed := c.Query("state") == "closed"
	openCount, closedCount := database.MilestoneStats(c.Repo.Repository.ID)
	c.Data["OpenCount"] = openCount
	c.Data["ClosedCount"] = closedCount

	page := c.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(openCount)
	} else {
		total = int(closedCount)
	}
	c.Data["Page"] = paginater.New(total, conf.UI.IssuePagingNum, page, 5)

	miles, err := database.GetMilestones(c.Repo.Repository.ID, page, isShowClosed)
	if err != nil {
		c.Error(err, "get milestones")
		return
	}
	for _, m := range miles {
		m.NumOpenIssues = int(m.CountIssues(false, false))
		m.NumClosedIssues = int(m.CountIssues(true, false))
		if m.NumOpenIssues+m.NumClosedIssues > 0 {
			m.Completeness = m.NumClosedIssues * 100 / (m.NumOpenIssues + m.NumClosedIssues)
		}
		m.RenderedContent = string(markup.Markdown(m.Content, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))
	}
	c.Data["Milestones"] = miles

	if isShowClosed {
		c.Data["State"] = "closed"
	} else {
		c.Data["State"] = "open"
	}

	c.Data["IsShowClosed"] = isShowClosed
	c.Success(MILESTONE)
}

func NewMilestone(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones.new")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = conf.I18n.DateLang(c.Locale.Language())
	c.Success(MILESTONE_NEW)
}

func NewMilestonePost(c *context.Context, f form.CreateMilestone) {
	c.Data["Title"] = c.Tr("repo.milestones.new")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = conf.I18n.DateLang(c.Locale.Language())

	if c.HasError() {
		c.Success(MILESTONE_NEW)
		return
	}

	if f.Deadline == "" {
		f.Deadline = "9999-12-31"
	}
	deadline, err := time.ParseInLocation("2006-01-02", f.Deadline, time.Local)
	if err != nil {
		c.Data["Err_Deadline"] = true
		c.RenderWithErr(c.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &f)
		return
	}

	if err = database.NewMilestone(&database.Milestone{
		RepoID:   c.Repo.Repository.ID,
		Name:     f.Title,
		Content:  f.Content,
		Deadline: deadline,
	}); err != nil {
		c.Error(err, "new milestone")
		return
	}

	c.Flash.Success(c.Tr("repo.milestones.create_success", f.Title))
	c.RawRedirect(c.Repo.MakeURL("milestones"))
}

func EditMilestone(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones.edit")
	c.Data["PageIsMilestones"] = true
	c.Data["PageIsEditMilestone"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = conf.I18n.DateLang(c.Locale.Language())

	m, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}
	c.Data["title"] = m.Name
	c.Data["content"] = m.Content
	if len(m.DeadlineString) > 0 {
		c.Data["deadline"] = m.DeadlineString
	}
	c.Success(MILESTONE_NEW)
}

func EditMilestonePost(c *context.Context, f form.CreateMilestone) {
	c.Data["Title"] = c.Tr("repo.milestones.edit")
	c.Data["PageIsMilestones"] = true
	c.Data["PageIsEditMilestone"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = conf.I18n.DateLang(c.Locale.Language())

	if c.HasError() {
		c.Success(MILESTONE_NEW)
		return
	}

	if f.Deadline == "" {
		f.Deadline = "9999-12-31"
	}
	deadline, err := time.ParseInLocation("2006-01-02", f.Deadline, time.Local)
	if err != nil {
		c.Data["Err_Deadline"] = true
		c.RenderWithErr(c.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &f)
		return
	}

	m, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}
	m.Name = f.Title
	m.Content = f.Content
	m.Deadline = deadline
	if err = database.UpdateMilestone(m); err != nil {
		c.Error(err, "update milestone")
		return
	}

	c.Flash.Success(c.Tr("repo.milestones.edit_success", m.Name))
	c.RawRedirect(c.Repo.MakeURL("milestones"))
}

func ChangeMilestonStatus(c *context.Context) {
	m, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}

	location := url.URL{
		Path: "milestones",
	}

	switch c.Params(":action") {
	case "open":
		if m.IsClosed {
			if err = database.ChangeMilestoneStatus(m, false); err != nil {
				c.Error(err, "change milestone status to open")
				return
			}
		}
		location.RawQuery = "state=open"
	case "close":
		if !m.IsClosed {
			m.ClosedDate = time.Now()
			if err = database.ChangeMilestoneStatus(m, true); err != nil {
				c.Error(err, "change milestone status to closed")
				return
			}
		}
		location.RawQuery = "state=closed"
	}

	c.RawRedirect(c.Repo.MakeURL(location))
}

func DeleteMilestone(c *context.Context) {
	if err := database.DeleteMilestoneOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteMilestoneByRepoID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.milestones.deletion_success"))
	}

	c.JSONSuccess(map[string]any{
		"redirect": c.Repo.MakeURL("milestones"),
	})
}
