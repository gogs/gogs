// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/markup"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/tool"
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
		c.Handle(404, "MustEnableIssues", nil)
		return
	}

	if c.Repo.Repository.EnableExternalTracker {
		c.Redirect(c.Repo.Repository.ExternalTrackerURL)
		return
	}
}

func MustAllowPulls(c *context.Context) {
	if !c.Repo.Repository.AllowsPulls() {
		c.Handle(404, "MustAllowPulls", nil)
		return
	}

	// User can send pull request if owns a forked repository.
	if c.IsLogged && c.User.HasForkedRepo(c.Repo.Repository.ID) {
		c.Repo.PullRequest.Allowed = true
		c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
	}
}

func RetrieveLabels(c *context.Context) {
	labels, err := models.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "RetrieveLabels.GetLabels", err)
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
		c.SetCookie("redirect_to", "/"+url.QueryEscape(setting.AppSubURL+c.Req.RequestURI), 0, setting.AppSubURL)
		c.Redirect(setting.AppSubURL + "/user/login")
		return
	}

	var (
		assigneeID = c.QueryInt64("assignee")
		posterID   int64
	)
	filterMode := models.FILTER_MODE_YOUR_REPOS
	switch viewType {
	case "assigned":
		filterMode = models.FILTER_MODE_ASSIGN
		assigneeID = c.User.ID
	case "created_by":
		filterMode = models.FILTER_MODE_CREATE
		posterID = c.User.ID
	case "mentioned":
		filterMode = models.FILTER_MODE_MENTION
	}

	var uid int64 = -1
	if c.IsLogged {
		uid = c.User.ID
	}

	repo := c.Repo.Repository
	selectLabels := c.Query("labels")
	milestoneID := c.QueryInt64("milestone")
	isShowClosed := c.Query("state") == "closed"
	issueStats := models.GetIssueStats(&models.IssueStatsOptions{
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
	pager := paginater.New(total, setting.UI.IssuePagingNum, page, 5)
	c.Data["Page"] = pager

	issues, err := models.Issues(&models.IssuesOptions{
		UserID:      uid,
		AssigneeID:  assigneeID,
		RepoID:      repo.ID,
		PosterID:    posterID,
		MilestoneID: milestoneID,
		Page:        pager.Current(),
		IsClosed:    isShowClosed,
		IsMention:   filterMode == models.FILTER_MODE_MENTION,
		IsPull:      isPullList,
		Labels:      selectLabels,
		SortType:    sortType,
	})
	if err != nil {
		c.Handle(500, "Issues", err)
		return
	}

	// Get issue-user relations.
	pairs, err := models.GetIssueUsers(repo.ID, posterID, isShowClosed)
	if err != nil {
		c.Handle(500, "GetIssueUsers", err)
		return
	}

	// Get posters.
	for i := range issues {
		if !c.IsLogged {
			issues[i].IsRead = true
			continue
		}

		// Check read status.
		idx := models.PairsContains(pairs, issues[i].ID, c.User.ID)
		if idx > -1 {
			issues[i].IsRead = pairs[idx].IsRead
		} else {
			issues[i].IsRead = true
		}
	}
	c.Data["Issues"] = issues

	// Get milestones.
	c.Data["Milestones"], err = models.GetMilestonesByRepoID(repo.ID)
	if err != nil {
		c.Handle(500, "GetAllRepoMilestones", err)
		return
	}

	// Get assignees.
	c.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		c.Handle(500, "GetAssignees", err)
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

	c.HTML(200, ISSUES)
}

func Issues(c *context.Context) {
	issues(c, false)
}

func Pulls(c *context.Context) {
	issues(c, true)
}

func renderAttachmentSettings(c *context.Context) {
	c.Data["RequireDropzone"] = true
	c.Data["IsAttachmentEnabled"] = setting.AttachmentEnabled
	c.Data["AttachmentAllowedTypes"] = setting.AttachmentAllowedTypes
	c.Data["AttachmentMaxSize"] = setting.AttachmentMaxSize
	c.Data["AttachmentMaxFiles"] = setting.AttachmentMaxFiles
}

func RetrieveRepoMilestonesAndAssignees(c *context.Context, repo *models.Repository) {
	var err error
	c.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
	if err != nil {
		c.Handle(500, "GetMilestones", err)
		return
	}
	c.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
	if err != nil {
		c.Handle(500, "GetMilestones", err)
		return
	}

	c.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		c.Handle(500, "GetAssignees", err)
		return
	}
}

func RetrieveRepoMetas(c *context.Context, repo *models.Repository) []*models.Label {
	if !c.Repo.IsWriter() {
		return nil
	}

	labels, err := models.GetLabelsByRepoID(repo.ID)
	if err != nil {
		c.Handle(500, "GetLabelsByRepoID", err)
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
	var r io.Reader
	var bytes []byte

	if c.Repo.Commit == nil {
		var err error
		c.Repo.Commit, err = c.Repo.GitRepo.GetBranchCommit(c.Repo.Repository.DefaultBranch)
		if err != nil {
			return "", false
		}
	}

	entry, err := c.Repo.Commit.GetTreeEntryByPath(filename)
	if err != nil {
		return "", false
	}
	r, err = entry.Blob().Data()
	if err != nil {
		return "", false
	}
	bytes, err = ioutil.ReadAll(r)
	if err != nil {
		return "", false
	}
	return string(bytes), true
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
	setTemplateIfExists(c, ISSUE_TEMPLATE_KEY, IssueTemplateCandidates)
	renderAttachmentSettings(c)

	RetrieveRepoMetas(c, c.Repo.Repository)
	if c.Written() {
		return
	}

	c.HTML(200, ISSUE_NEW)
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
			c.Handle(500, "GetMilestoneByID", err)
			return nil, 0, 0
		}
		c.Data["milestone_id"] = milestoneID
	}

	// Check assignee.
	assigneeID := f.AssigneeID
	if assigneeID > 0 {
		c.Data["Assignee"], err = repo.GetAssigneeByID(assigneeID)
		if err != nil {
			c.Handle(500, "GetAssigneeByID", err)
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
		c.HTML(200, ISSUE_NEW)
		return
	}

	var attachments []string
	if setting.AttachmentEnabled {
		attachments = f.Files
	}

	issue := &models.Issue{
		RepoID:      c.Repo.Repository.ID,
		Title:       f.Title,
		PosterID:    c.User.ID,
		Poster:      c.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		Content:     f.Content,
	}
	if err := models.NewIssue(c.Repo.Repository, issue, labelIDs, attachments); err != nil {
		c.Handle(500, "NewIssue", err)
		return
	}

	log.Trace("Issue created: %d/%d", c.Repo.Repository.ID, issue.ID)
	c.Redirect(c.Repo.RepoLink + "/issues/" + com.ToStr(issue.Index))
}

func uploadAttachment(c *context.Context, allowedTypes []string) {
	file, header, err := c.Req.FormFile("file")
	if err != nil {
		c.Error(500, fmt.Sprintf("FormFile: %v", err))
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
		c.Error(400, ErrFileTypeForbidden.Error())
		return
	}

	attach, err := models.NewAttachment(header.Filename, buf, file)
	if err != nil {
		c.Error(500, fmt.Sprintf("NewAttachment: %v", err))
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	c.JSON(200, map[string]string{
		"uuid": attach.UUID,
	})
}

func UploadIssueAttachment(c *context.Context) {
	if !setting.AttachmentEnabled {
		c.NotFound()
		return
	}

	uploadAttachment(c, strings.Split(setting.AttachmentAllowedTypes, ","))
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

	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
		return
	}
	c.Data["Title"] = issue.Title

	// Make sure type and URL matches.
	if !isPullList && issue.IsPull {
		c.Redirect(c.Repo.RepoLink + "/pulls/" + com.ToStr(issue.Index))
		return
	} else if isPullList && !issue.IsPull {
		c.Redirect(c.Repo.RepoLink + "/issues/" + com.ToStr(issue.Index))
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
	labels, err := models.GetLabelsByRepoID(repo.ID)
	if err != nil {
		c.Handle(500, "GetLabelsByRepoID", err)
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
			c.Handle(500, "ReadBy", err)
			return
		}
	}

	var (
		tag          models.CommentTag
		ok           bool
		marked       = make(map[int64]models.CommentTag)
		comment      *models.Comment
		participants = make([]*models.User, 1, 10)
	)

	// Render comments and and fetch participants.
	participants[0] = issue.Poster
	for _, comment = range issue.Comments {
		if comment.Type == models.COMMENT_TYPE_COMMENT {
			comment.RenderedContent = string(markup.Markdown(comment.Content, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))

			// Check tag.
			tag, ok = marked[comment.PosterID]
			if ok {
				comment.ShowTag = tag
				continue
			}

			if repo.IsOwnedBy(comment.PosterID) ||
				(repo.Owner.IsOrganization() && repo.Owner.IsOwnedBy(comment.PosterID)) {
				comment.ShowTag = models.COMMENT_TAG_OWNER
			} else if comment.Poster.IsWriterOfRepo(repo) {
				comment.ShowTag = models.COMMENT_TAG_WRITER
			} else if comment.PosterID == issue.PosterID {
				comment.ShowTag = models.COMMENT_TAG_POSTER
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
		c.Data["IsPullBranchDeletable"] = pull.BaseRepoID == pull.HeadRepoID &&
			c.Repo.IsWriter() && c.Repo.GitRepo.IsBranchExist(pull.HeadBranch)

		deleteBranchUrl := c.Repo.RepoLink + "/branches/delete/" + pull.HeadBranch
		c.Data["DeleteBranchLink"] = fmt.Sprintf("%s?commit=%s&redirect_to=%s", deleteBranchUrl, pull.MergedCommitID, c.Data["Link"])
	}

	c.Data["Participants"] = participants
	c.Data["NumParticipants"] = len(participants)
	c.Data["Issue"] = issue
	c.Data["IsIssueOwner"] = c.Repo.IsWriter() || (c.IsLogged && issue.IsPoster(c.User.ID))
	c.Data["SignInLink"] = setting.AppSubURL + "/user/login?redirect_to=" + c.Data["Link"].(string)
	c.HTML(200, ISSUE_VIEW)
}

func ViewIssue(c *context.Context) {
	viewIssue(c, false)
}

func ViewPull(c *context.Context) {
	viewIssue(c, true)
}

func getActionIssue(c *context.Context) *models.Issue {
	issue, err := models.GetIssueByIndex(c.Repo.Repository.ID, c.ParamsInt64(":index"))
	if err != nil {
		c.NotFoundOrServerError("GetIssueByIndex", errors.IsIssueNotExist, err)
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
		c.Error(403)
		return
	}

	title := c.QueryTrim("title")
	if len(title) == 0 {
		c.Error(204)
		return
	}

	if err := issue.ChangeTitle(c.User, title); err != nil {
		c.Handle(500, "ChangeTitle", err)
		return
	}

	c.JSON(200, map[string]interface{}{
		"title": issue.Title,
	})
}

func UpdateIssueContent(c *context.Context) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	if !c.IsLogged || (c.User.ID != issue.PosterID && !c.Repo.IsWriter()) {
		c.Error(403)
		return
	}

	content := c.Query("content")
	if err := issue.ChangeContent(c.User, content); err != nil {
		c.Handle(500, "ChangeContent", err)
		return
	}

	c.JSON(200, map[string]string{
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
			c.Handle(500, "ClearLabels", err)
			return
		}
	} else {
		isAttach := c.Query("action") == "attach"
		label, err := models.GetLabelOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id"))
		if err != nil {
			if models.IsErrLabelNotExist(err) {
				c.Error(404, "GetLabelByID")
			} else {
				c.Handle(500, "GetLabelByID", err)
			}
			return
		}

		if isAttach && !issue.HasLabel(label.ID) {
			if err = issue.AddLabel(c.User, label); err != nil {
				c.Handle(500, "AddLabel", err)
				return
			}
		} else if !isAttach && issue.HasLabel(label.ID) {
			if err = issue.RemoveLabel(c.User, label); err != nil {
				c.Handle(500, "RemoveLabel", err)
				return
			}
		}
	}

	c.JSON(200, map[string]interface{}{
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
		c.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	// Not check for invalid milestone id and give responsibility to owners.
	issue.MilestoneID = milestoneID
	if err := models.ChangeMilestoneAssign(c.User, issue, oldMilestoneID); err != nil {
		c.Handle(500, "ChangeMilestoneAssign", err)
		return
	}

	c.JSON(200, map[string]interface{}{
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
		c.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	if err := issue.ChangeAssignee(c.User, assigneeID); err != nil {
		c.Handle(500, "ChangeAssignee", err)
		return
	}

	c.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func NewComment(c *context.Context, f form.CreateComment) {
	issue := getActionIssue(c)
	if c.Written() {
		return
	}

	var attachments []string
	if setting.AttachmentEnabled {
		attachments = f.Files
	}

	if c.HasError() {
		c.Flash.Error(c.Data["ErrorMsg"].(string))
		c.Redirect(fmt.Sprintf("%s/issues/%d", c.Repo.RepoLink, issue.Index))
		return
	}

	var err error
	var comment *models.Comment
	defer func() {
		// Check if issue admin/poster changes the status of issue.
		if (c.Repo.IsWriter() || (c.IsLogged && issue.IsPoster(c.User.ID))) &&
			(f.Status == "reopen" || f.Status == "close") &&
			!(issue.IsPull && issue.PullRequest.HasMerged) {

			// Duplication and conflict check should apply to reopen pull request.
			var pr *models.PullRequest

			if f.Status == "reopen" && issue.IsPull {
				pull := issue.PullRequest
				pr, err = models.GetUnmergedPullRequest(pull.HeadRepoID, pull.BaseRepoID, pull.HeadBranch, pull.BaseBranch)
				if err != nil {
					if !models.IsErrPullRequestNotExist(err) {
						c.ServerError("GetUnmergedPullRequest", err)
						return
					}
				}

				// Regenerate patch and test conflict.
				if pr == nil {
					if err = issue.PullRequest.UpdatePatch(); err != nil {
						c.ServerError("UpdatePatch", err)
						return
					}

					issue.PullRequest.AddToTaskQueue()
				}
			}

			if pr != nil {
				c.Flash.Info(c.Tr("repo.pulls.open_unmerged_pull_exists", pr.Index))
			} else {
				if err = issue.ChangeStatus(c.User, c.Repo.Repository, f.Status == "close"); err != nil {
					log.Error(2, "ChangeStatus: %v", err)
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
		if comment != nil {
			c.Redirect(fmt.Sprintf("%s/%s/%d#%s", c.Repo.RepoLink, typeName, issue.Index, comment.HashTag()))
		} else {
			c.Redirect(fmt.Sprintf("%s/%s/%d", c.Repo.RepoLink, typeName, issue.Index))
		}
	}()

	// Fix #321: Allow empty comments, as long as we have attachments.
	if len(f.Content) == 0 && len(attachments) == 0 {
		return
	}

	comment, err = models.CreateIssueComment(c.User, c.Repo.Repository, issue, f.Content, attachments)
	if err != nil {
		c.ServerError("CreateIssueComment", err)
		return
	}

	log.Trace("Comment created: %d/%d/%d", c.Repo.Repository.ID, issue.ID, comment.ID)
}

func UpdateCommentContent(c *context.Context) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetCommentByID", models.IsErrCommentNotExist, err)
		return
	}

	if c.UserID() != comment.PosterID && !c.Repo.IsAdmin() {
		c.Error(404)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.Error(204)
		return
	}

	oldContent := comment.Content
	comment.Content = c.Query("content")
	if len(comment.Content) == 0 {
		c.JSON(200, map[string]interface{}{
			"content": "",
		})
		return
	}
	if err = models.UpdateComment(c.User, comment, oldContent); err != nil {
		c.Handle(500, "UpdateComment", err)
		return
	}

	c.JSON(200, map[string]string{
		"content": string(markup.Markdown(comment.Content, c.Query("context"), c.Repo.Repository.ComposeMetas())),
	})
}

func DeleteComment(c *context.Context) {
	comment, err := models.GetCommentByID(c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetCommentByID", models.IsErrCommentNotExist, err)
		return
	}

	if c.UserID() != comment.PosterID && !c.Repo.IsAdmin() {
		c.Error(404)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		c.Error(204)
		return
	}

	if err = models.DeleteCommentByID(c.User, comment.ID); err != nil {
		c.Handle(500, "DeleteCommentByID", err)
		return
	}

	c.Status(200)
}

func Labels(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.labels")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsLabels"] = true
	c.Data["RequireMinicolors"] = true
	c.Data["LabelTemplates"] = models.LabelTemplates
	c.HTML(200, LABELS)
}

func InitializeLabels(c *context.Context, f form.InitializeLabels) {
	if c.HasError() {
		c.Redirect(c.Repo.RepoLink + "/labels")
		return
	}
	list, err := models.GetLabelTemplateFile(f.TemplateName)
	if err != nil {
		c.Flash.Error(c.Tr("repo.issues.label_templates.fail_to_load_file", f.TemplateName, err))
		c.Redirect(c.Repo.RepoLink + "/labels")
		return
	}

	labels := make([]*models.Label, len(list))
	for i := 0; i < len(list); i++ {
		labels[i] = &models.Label{
			RepoID: c.Repo.Repository.ID,
			Name:   list[i][0],
			Color:  list[i][1],
		}
	}
	if err := models.NewLabels(labels...); err != nil {
		c.Handle(500, "NewLabels", err)
		return
	}
	c.Redirect(c.Repo.RepoLink + "/labels")
}

func NewLabel(c *context.Context, f form.CreateLabel) {
	c.Data["Title"] = c.Tr("repo.labels")
	c.Data["PageIsLabels"] = true

	if c.HasError() {
		c.Flash.Error(c.Data["ErrorMsg"].(string))
		c.Redirect(c.Repo.RepoLink + "/labels")
		return
	}

	l := &models.Label{
		RepoID: c.Repo.Repository.ID,
		Name:   f.Title,
		Color:  f.Color,
	}
	if err := models.NewLabels(l); err != nil {
		c.Handle(500, "NewLabel", err)
		return
	}
	c.Redirect(c.Repo.RepoLink + "/labels")
}

func UpdateLabel(c *context.Context, f form.CreateLabel) {
	l, err := models.GetLabelByID(f.ID)
	if err != nil {
		switch {
		case models.IsErrLabelNotExist(err):
			c.Error(404)
		default:
			c.Handle(500, "UpdateLabel", err)
		}
		return
	}

	l.Name = f.Title
	l.Color = f.Color
	if err := models.UpdateLabel(l); err != nil {
		c.Handle(500, "UpdateLabel", err)
		return
	}
	c.Redirect(c.Repo.RepoLink + "/labels")
}

func DeleteLabel(c *context.Context) {
	if err := models.DeleteLabel(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteLabel: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.issues.label_deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/labels",
	})
	return
}

func Milestones(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true

	isShowClosed := c.Query("state") == "closed"
	openCount, closedCount := models.MilestoneStats(c.Repo.Repository.ID)
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
	c.Data["Page"] = paginater.New(total, setting.UI.IssuePagingNum, page, 5)

	miles, err := models.GetMilestones(c.Repo.Repository.ID, page, isShowClosed)
	if err != nil {
		c.Handle(500, "GetMilestones", err)
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
	c.HTML(200, MILESTONE)
}

func NewMilestone(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones.new")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = setting.DateLang(c.Locale.Language())
	c.HTML(200, MILESTONE_NEW)
}

func NewMilestonePost(c *context.Context, f form.CreateMilestone) {
	c.Data["Title"] = c.Tr("repo.milestones.new")
	c.Data["PageIsIssueList"] = true
	c.Data["PageIsMilestones"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = setting.DateLang(c.Locale.Language())

	if c.HasError() {
		c.HTML(200, MILESTONE_NEW)
		return
	}

	if len(f.Deadline) == 0 {
		f.Deadline = "9999-12-31"
	}
	deadline, err := time.ParseInLocation("2006-01-02", f.Deadline, time.Local)
	if err != nil {
		c.Data["Err_Deadline"] = true
		c.RenderWithErr(c.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &f)
		return
	}

	if err = models.NewMilestone(&models.Milestone{
		RepoID:   c.Repo.Repository.ID,
		Name:     f.Title,
		Content:  f.Content,
		Deadline: deadline,
	}); err != nil {
		c.Handle(500, "NewMilestone", err)
		return
	}

	c.Flash.Success(c.Tr("repo.milestones.create_success", f.Title))
	c.Redirect(c.Repo.RepoLink + "/milestones")
}

func EditMilestone(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.milestones.edit")
	c.Data["PageIsMilestones"] = true
	c.Data["PageIsEditMilestone"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = setting.DateLang(c.Locale.Language())

	m, err := models.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			c.Handle(404, "", nil)
		} else {
			c.Handle(500, "GetMilestoneByRepoID", err)
		}
		return
	}
	c.Data["title"] = m.Name
	c.Data["content"] = m.Content
	if len(m.DeadlineString) > 0 {
		c.Data["deadline"] = m.DeadlineString
	}
	c.HTML(200, MILESTONE_NEW)
}

func EditMilestonePost(c *context.Context, f form.CreateMilestone) {
	c.Data["Title"] = c.Tr("repo.milestones.edit")
	c.Data["PageIsMilestones"] = true
	c.Data["PageIsEditMilestone"] = true
	c.Data["RequireDatetimepicker"] = true
	c.Data["DateLang"] = setting.DateLang(c.Locale.Language())

	if c.HasError() {
		c.HTML(200, MILESTONE_NEW)
		return
	}

	if len(f.Deadline) == 0 {
		f.Deadline = "9999-12-31"
	}
	deadline, err := time.ParseInLocation("2006-01-02", f.Deadline, time.Local)
	if err != nil {
		c.Data["Err_Deadline"] = true
		c.RenderWithErr(c.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &f)
		return
	}

	m, err := models.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			c.Handle(404, "", nil)
		} else {
			c.Handle(500, "GetMilestoneByRepoID", err)
		}
		return
	}
	m.Name = f.Title
	m.Content = f.Content
	m.Deadline = deadline
	if err = models.UpdateMilestone(m); err != nil {
		c.Handle(500, "UpdateMilestone", err)
		return
	}

	c.Flash.Success(c.Tr("repo.milestones.edit_success", m.Name))
	c.Redirect(c.Repo.RepoLink + "/milestones")
}

func ChangeMilestonStatus(c *context.Context) {
	m, err := models.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			c.Handle(404, "", err)
		} else {
			c.Handle(500, "GetMilestoneByRepoID", err)
		}
		return
	}

	switch c.Params(":action") {
	case "open":
		if m.IsClosed {
			if err = models.ChangeMilestoneStatus(m, false); err != nil {
				c.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		c.Redirect(c.Repo.RepoLink + "/milestones?state=open")
	case "close":
		if !m.IsClosed {
			m.ClosedDate = time.Now()
			if err = models.ChangeMilestoneStatus(m, true); err != nil {
				c.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		c.Redirect(c.Repo.RepoLink + "/milestones?state=closed")
	default:
		c.Redirect(c.Repo.RepoLink + "/milestones")
	}
}

func DeleteMilestone(c *context.Context) {
	if err := models.DeleteMilestoneOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteMilestoneByRepoID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.milestones.deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/milestones",
	})
}
