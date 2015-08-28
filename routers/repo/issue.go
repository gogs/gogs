// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	ISSUES     base.TplName = "repo/issue/list"
	ISSUE_NEW  base.TplName = "repo/issue/new"
	ISSUE_VIEW base.TplName = "repo/issue/view"

	LABELS base.TplName = "repo/issue/labels"

	MILESTONE      base.TplName = "repo/issue/milestones"
	MILESTONE_NEW  base.TplName = "repo/issue/milestone_new"
	MILESTONE_EDIT base.TplName = "repo/issue/milestone_edit"
)

var (
	ErrFileTypeForbidden = errors.New("File type is not allowed")
	ErrTooManyFiles      = errors.New("Maximum number of files to upload exceeded")
)

func RetrieveLabels(ctx *middleware.Context) {
	labels, err := models.GetLabelsByRepoID(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "RetrieveLabels.GetLabels: %v", err)
		return
	}
	for _, l := range labels {
		l.CalOpenIssues()
	}
	ctx.Data["Labels"] = labels
	ctx.Data["NumLabels"] = len(labels)
}

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.issues")
	ctx.Data["PageIsIssueList"] = true

	viewType := ctx.Query("type")
	sortType := ctx.Query("sort")
	types := []string{"assigned", "created_by", "mentioned"}
	if !com.IsSliceContainsStr(types, viewType) {
		viewType = "all"
	}

	// Must sign in to see issues about you.
	if viewType != "all" && !ctx.IsSigned {
		ctx.SetCookie("redirect_to", "/"+url.QueryEscape(setting.AppSubUrl+ctx.Req.RequestURI), 0, setting.AppSubUrl)
		ctx.Redirect(setting.AppSubUrl + "/user/login")
		return
	}

	var (
		assigneeID = ctx.QueryInt64("assignee")
		posterID   int64
	)
	filterMode := models.FM_ALL
	switch viewType {
	case "assigned":
		filterMode = models.FM_ASSIGN
		assigneeID = ctx.User.Id
	case "created_by":
		filterMode = models.FM_CREATE
		posterID = ctx.User.Id
	case "mentioned":
		filterMode = models.FM_MENTION
	}

	var uid int64 = -1
	if ctx.IsSigned {
		uid = ctx.User.Id
	}

	repo := ctx.Repo.Repository
	selectLabels := ctx.Query("labels")
	milestoneID := ctx.QueryInt64("milestone")
	isShowClosed := ctx.Query("state") == "closed"
	issueStats := models.GetIssueStats(repo.ID, uid, com.StrTo(selectLabels).MustInt64(), milestoneID, assigneeID, filterMode)

	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(issueStats.OpenCount)
	} else {
		total = int(issueStats.ClosedCount)
	}
	ctx.Data["Page"] = paginater.New(total, setting.IssuePagingNum, page, 5)

	// Get issues.
	issues, err := models.Issues(uid, assigneeID, repo.ID, posterID, milestoneID,
		nil, page, isShowClosed, filterMode == models.FM_MENTION, selectLabels, sortType)
	if err != nil {
		ctx.Handle(500, "Issues: %v", err)
		return
	}

	// Get issue-user relations.
	pairs, err := models.GetIssueUsers(repo.ID, posterID, isShowClosed)
	if err != nil {
		ctx.Handle(500, "GetIssueUsers: %v", err)
		return
	}

	// Get posters.
	for i := range issues {
		if err = issues[i].GetPoster(); err != nil {
			ctx.Handle(500, "GetPoster", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if err = issues[i].GetLabels(); err != nil {
			ctx.Handle(500, "GetLabels", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if !ctx.IsSigned {
			issues[i].IsRead = true
			continue
		}

		// Check read status.
		idx := models.PairsContains(pairs, issues[i].ID, ctx.User.Id)
		if idx > -1 {
			issues[i].IsRead = pairs[idx].IsRead
		} else {
			issues[i].IsRead = true
		}
	}
	ctx.Data["Issues"] = issues

	// Get milestones.
	ctx.Data["Milestones"], err = models.GetAllRepoMilestones(repo.ID)
	if err != nil {
		ctx.Handle(500, "GetAllRepoMilestones: %v", err)
		return
	}

	// Get assignees.
	ctx.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		ctx.Handle(500, "GetAssignees: %v", err)
		return
	}

	ctx.Data["IssueStats"] = issueStats
	ctx.Data["SelectLabels"] = com.StrTo(selectLabels).MustInt64()
	ctx.Data["ViewType"] = viewType
	ctx.Data["SortType"] = sortType
	ctx.Data["MilestoneID"] = milestoneID
	ctx.Data["AssigneeID"] = assigneeID
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
	} else {
		ctx.Data["State"] = "open"
	}

	ctx.HTML(200, ISSUES)
}

func renderAttachmentSettings(ctx *middleware.Context) {
	ctx.Data["IsAttachmentEnabled"] = setting.AttachmentEnabled
	ctx.Data["AttachmentAllowedTypes"] = setting.AttachmentAllowedTypes
	ctx.Data["AttachmentMaxFiles"] = setting.AttachmentMaxFiles
}

func NewIssue(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.issues.new")
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	if ctx.Repo.IsAdmin() {
		var (
			repo = ctx.Repo.Repository
			err  error
		)
		ctx.Data["Labels"], err = models.GetLabelsByRepoID(repo.ID)
		if err != nil {
			ctx.Handle(500, "GetLabelsByRepoID: %v", err)
			return
		}

		ctx.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}
		ctx.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}

		ctx.Data["Assignees"], err = repo.GetAssignees()
		if err != nil {
			ctx.Handle(500, "GetAssignees: %v", err)
			return
		}
	}

	ctx.HTML(200, ISSUE_NEW)
}

func NewIssuePost(ctx *middleware.Context, form auth.CreateIssueForm) {
	ctx.Data["Title"] = ctx.Tr("repo.issues.new")
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	var (
		repo        = ctx.Repo.Repository
		labelIDs    []int64
		milestoneID int64
		assigneeID  int64
		attachments []string
	)
	if ctx.Repo.IsAdmin() {
		// Check labels.
		labelIDs = base.StringsToInt64s(strings.Split(form.LabelIDs, ","))
		labelIDMark := base.Int64sToMap(labelIDs)
		labels, err := models.GetLabelsByRepoID(repo.ID)
		if err != nil {
			ctx.Handle(500, "GetLabelsByRepoID: %v", err)
			return
		}
		hasSelected := false
		for i := range labels {
			if labelIDMark[labels[i].ID] {
				labels[i].IsChecked = true
				hasSelected = true
			}
		}
		ctx.Data["HasSelectedLabel"] = hasSelected
		ctx.Data["label_ids"] = form.LabelIDs
		ctx.Data["Labels"] = labels

		// Check milestone.
		milestoneID = form.MilestoneID
		if milestoneID > 0 {
			ctx.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
			if err != nil {
				ctx.Handle(500, "GetMilestones: %v", err)
				return
			}
			ctx.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
			if err != nil {
				ctx.Handle(500, "GetMilestones: %v", err)
				return
			}
			ctx.Data["Milestone"], err = repo.GetMilestoneByID(milestoneID)
			if err != nil {
				ctx.Handle(500, "GetMilestoneByID: %v", err)
				return
			}
			ctx.Data["milestone_id"] = milestoneID
		}

		// Check assignee.
		assigneeID = form.AssigneeID
		if assigneeID > 0 {
			ctx.Data["Assignees"], err = repo.GetAssignees()
			if err != nil {
				ctx.Handle(500, "GetAssignees: %v", err)
				return
			}
			ctx.Data["Assignee"], err = repo.GetAssigneeByID(assigneeID)
			if err != nil {
				ctx.Handle(500, "GetAssigneeByID: %v", err)
				return
			}
			ctx.Data["assignee_id"] = assigneeID
		}
	}

	if setting.AttachmentEnabled {
		attachments = form.Attachments
	}

	if ctx.HasError() {
		ctx.HTML(200, ISSUE_NEW)
		return
	}

	issue := &models.Issue{
		RepoID:      ctx.Repo.Repository.ID,
		Index:       int64(repo.NumIssues) + 1,
		Name:        form.Title,
		PosterID:    ctx.User.Id,
		Poster:      ctx.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		Content:     form.Content,
	}
	if err := models.NewIssue(repo, issue, labelIDs, attachments); err != nil {
		ctx.Handle(500, "NewIssue", err)
		return
	}

	// Update mentions.
	mentions := base.MentionPattern.FindAllString(issue.Content, -1)
	if len(mentions) > 0 {
		for i := range mentions {
			mentions[i] = mentions[i][1:]
		}

		if err := models.UpdateMentions(mentions, issue.ID); err != nil {
			ctx.Handle(500, "UpdateMentions", err)
			return
		}
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			ctx.Handle(500, "SendIssueNotifyMail", err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(mentions))
		for _, m := range mentions {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			ctx.Handle(500, "SendIssueMentionMail", err)
			return
		}
	}

	log.Trace("Issue created: %d/%d", ctx.Repo.Repository.ID, issue.ID)
	ctx.Redirect(ctx.Repo.RepoLink + "/issues/" + com.ToStr(issue.Index))
}

func UploadIssueAttachment(ctx *middleware.Context) {
	if !setting.AttachmentEnabled {
		ctx.Error(404, "attachment is not enabled")
		return
	}

	allowedTypes := strings.Split(setting.AttachmentAllowedTypes, ",")
	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		ctx.Error(500, fmt.Sprintf("FormFile: %v", err))
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
		ctx.Error(400, ErrFileTypeForbidden.Error())
		return
	}

	attach, err := models.NewAttachment(header.Filename, buf, file)
	if err != nil {
		ctx.Error(500, fmt.Sprintf("NewAttachment: %v", err))
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	ctx.JSON(200, map[string]string{
		"uuid": attach.UUID,
	})
}

func ViewIssue(ctx *middleware.Context) {
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "GetIssueByIndex", err)
		} else {
			ctx.Handle(500, "GetIssueByIndex", err)
		}
		return
	}
	ctx.Data["Title"] = issue.Name

	if err = issue.GetPoster(); err != nil {
		ctx.Handle(500, "GetPoster", err)
		return
	}
	issue.RenderedContent = string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink))

	repo := ctx.Repo.Repository

	// Metas.
	// Check labels.
	if err = issue.GetLabels(); err != nil {
		ctx.Handle(500, "GetLabels", err)
		return
	}
	labelIDMark := make(map[int64]bool)
	for i := range issue.Labels {
		labelIDMark[issue.Labels[i].ID] = true
	}
	labels, err := models.GetLabelsByRepoID(repo.ID)
	if err != nil {
		ctx.Handle(500, "GetLabelsByRepoID: %v", err)
		return
	}
	hasSelected := false
	for i := range labels {
		if labelIDMark[labels[i].ID] {
			labels[i].IsChecked = true
			hasSelected = true
		}
	}
	ctx.Data["HasSelectedLabel"] = hasSelected
	ctx.Data["Labels"] = labels

	// Check milestone and assignee.
	if ctx.Repo.IsAdmin() {
		ctx.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}
		ctx.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}

		ctx.Data["Assignees"], err = repo.GetAssignees()
		if err != nil {
			ctx.Handle(500, "GetAssignees: %v", err)
			return
		}
	}

	if ctx.IsSigned {
		// Update issue-user.
		if err = issue.ReadBy(ctx.User.Id); err != nil {
			ctx.Handle(500, "ReadBy", err)
			return
		}
	}

	var (
		tag     models.CommentTag
		ok      bool
		marked  = make(map[int64]models.CommentTag)
		comment *models.Comment
	)
	// Render comments.
	for _, comment = range issue.Comments {
		if comment.Type == models.COMMENT_TYPE_COMMENT {
			comment.RenderedContent = string(base.RenderMarkdown([]byte(comment.Content), ctx.Repo.RepoLink))

			// Check tag.
			tag, ok = marked[comment.PosterID]
			if ok {
				comment.ShowTag = tag
				continue
			}

			if repo.IsOwnedBy(comment.PosterID) ||
				(repo.Owner.IsOrganization() && repo.Owner.IsOwnedBy(comment.PosterID)) {
				comment.ShowTag = models.COMMENT_TAG_OWNER
			} else if comment.Poster.IsAdminOfRepo(repo) {
				comment.ShowTag = models.COMMENT_TAG_ADMIN
			} else if comment.PosterID == issue.PosterID {
				comment.ShowTag = models.COMMENT_TAG_POSTER
			}

			marked[comment.PosterID] = comment.ShowTag
		}
	}

	ctx.Data["Issue"] = issue
	ctx.Data["IsIssueOwner"] = ctx.Repo.IsAdmin() || (ctx.IsSigned && issue.IsPoster(ctx.User.Id))
	ctx.Data["SignInLink"] = setting.AppSubUrl + "/user/login"
	ctx.HTML(200, ISSUE_VIEW)
}

func getActionIssue(ctx *middleware.Context) *models.Issue {
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Error(404, "GetIssueByIndex")
		} else {
			ctx.Handle(500, "GetIssueByIndex", err)
		}
		return nil
	}
	return issue
}

func UpdateIssueTitle(ctx *middleware.Context) {
	issue := getActionIssue(ctx)
	if ctx.Written() {
		return
	}

	if !ctx.IsSigned || (ctx.User.Id != issue.PosterID && !ctx.Repo.IsAdmin()) {
		ctx.Error(403)
		return
	}

	issue.Name = ctx.Query("title")
	if len(issue.Name) == 0 {
		ctx.Error(204)
		return
	}

	if err := models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "UpdateIssue", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"title": issue.Name,
	})
}

func UpdateIssueContent(ctx *middleware.Context) {
	issue := getActionIssue(ctx)
	if ctx.Written() {
		return
	}

	if !ctx.IsSigned || (ctx.User.Id != issue.PosterID && !ctx.Repo.IsAdmin()) {
		ctx.Error(403)
		return
	}

	issue.Content = ctx.Query("content")
	if err := models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "UpdateIssue", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"content": string(base.RenderMarkdown([]byte(issue.Content), ctx.Query("context"))),
	})
}

func UpdateIssueLabel(ctx *middleware.Context) {
	issue := getActionIssue(ctx)
	if ctx.Written() {
		return
	}

	if ctx.Query("action") == "clear" {
		if err := issue.ClearLabels(); err != nil {
			ctx.Handle(500, "ClearLabels", err)
			return
		}
	} else {
		isAttach := ctx.Query("action") == "attach"
		label, err := models.GetLabelByID(ctx.QueryInt64("id"))
		if err != nil {
			if models.IsErrLabelNotExist(err) {
				ctx.Error(404, "GetLabelByID")
			} else {
				ctx.Handle(500, "GetLabelByID", err)
			}
			return
		}

		if isAttach && !issue.HasLabel(label.ID) {
			if err = issue.AddLabel(label); err != nil {
				ctx.Handle(500, "AddLabel", err)
				return
			}
		} else if !isAttach && issue.HasLabel(label.ID) {
			if err = issue.RemoveLabel(label); err != nil {
				ctx.Handle(500, "RemoveLabel", err)
				return
			}
		}
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateIssueMilestone(ctx *middleware.Context) {
	issue := getActionIssue(ctx)
	if ctx.Written() {
		return
	}

	oldMid := issue.MilestoneID
	mid := ctx.QueryInt64("id")
	if oldMid == mid {
		ctx.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	// Not check for invalid milestone id and give responsibility to owners.
	issue.MilestoneID = mid
	if err := models.ChangeMilestoneAssign(oldMid, issue); err != nil {
		ctx.Handle(500, "ChangeMilestoneAssign", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateIssueAssignee(ctx *middleware.Context) {
	issue := getActionIssue(ctx)
	if ctx.Written() {
		return
	}

	aid := ctx.QueryInt64("id")
	if issue.AssigneeID == aid {
		ctx.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	// Not check for invalid assignee id and give responsibility to owners.
	issue.AssigneeID = aid
	if err := models.UpdateIssueUserByAssignee(issue); err != nil {
		ctx.Handle(500, "UpdateIssueUserByAssignee: %v", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func NewComment(ctx *middleware.Context, form auth.CreateCommentForm) {
	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "GetIssueByIndex", err)
		} else {
			ctx.Handle(500, "GetIssueByIndex", err)
		}
		return
	}

	var attachments []string
	if setting.AttachmentEnabled {
		attachments = form.Attachments
	}

	if ctx.HasError() {
		ctx.Flash.Error(ctx.Data["ErrorMsg"].(string))
		ctx.Redirect(fmt.Sprintf("%s/issues/%d", ctx.Repo.RepoLink, issue.Index))
		return
	}

	// Check if issue owner/poster changes the status of issue.
	if (ctx.Repo.IsOwner() || (ctx.IsSigned && issue.IsPoster(ctx.User.Id))) &&
		(form.Status == "reopen" || form.Status == "close") {
		issue.Repo = ctx.Repo.Repository
		if err = issue.ChangeStatus(ctx.User, form.Status == "close"); err != nil {
			ctx.Handle(500, "ChangeStatus", err)
			return
		}
		log.Trace("%s Issue[%d] status changed: %v", ctx.Req.RequestURI, issue.ID, !issue.IsClosed)
	}

	// Fix #321: Allow empty comments, as long as we have attachments.
	if len(form.Content) == 0 && len(attachments) == 0 {
		ctx.Redirect(fmt.Sprintf("%s/issues/%d", ctx.Repo.RepoLink, issue.Index))
		return
	}

	comment, err := models.CreateIssueComment(ctx.User, ctx.Repo.Repository, issue, form.Content, attachments)
	if err != nil {
		ctx.Handle(500, "CreateIssueComment", err)
		return
	}

	// Update mentions.
	mentions := base.MentionPattern.FindAllString(comment.Content, -1)
	if len(mentions) > 0 {
		for i := range mentions {
			mentions[i] = mentions[i][1:]
		}

		if err := models.UpdateMentions(mentions, issue.ID); err != nil {
			ctx.Handle(500, "UpdateMentions", err)
			return
		}
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		issue.Content = form.Content
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			ctx.Handle(500, "SendIssueNotifyMail", err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(mentions))
		for _, m := range mentions {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			ctx.Handle(500, "SendIssueMentionMail", err)
			return
		}
	}

	log.Trace("Comment created: %d/%d/%d", ctx.Repo.Repository.ID, issue.ID, comment.ID)
	ctx.Redirect(fmt.Sprintf("%s/issues/%d#%s", ctx.Repo.RepoLink, issue.Index, comment.HashTag()))
}

func UpdateCommentContent(ctx *middleware.Context) {
	comment, err := models.GetCommentByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrCommentNotExist(err) {
			ctx.Error(404, "GetCommentByID")
		} else {
			ctx.Handle(500, "GetCommentByID", err)
		}
		return
	}

	if !ctx.IsSigned || (ctx.User.Id != comment.PosterID && !ctx.Repo.IsAdmin()) {
		ctx.Error(403)
		return
	} else if comment.Type != models.COMMENT_TYPE_COMMENT {
		ctx.Error(204)
		return
	}

	comment.Content = ctx.Query("content")
	if len(comment.Content) == 0 {
		ctx.JSON(200, map[string]interface{}{
			"content": "",
		})
		return
	}
	if err := models.UpdateComment(comment); err != nil {
		ctx.Handle(500, "UpdateComment", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"content": string(base.RenderMarkdown([]byte(comment.Content), ctx.Query("context"))),
	})
}

func Labels(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.labels")
	ctx.Data["PageIsLabels"] = true
	ctx.Data["RequireMinicolors"] = true
	ctx.HTML(200, LABELS)
}

func NewLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	ctx.Data["Title"] = ctx.Tr("repo.labels")
	ctx.Data["PageIsLabels"] = true

	if ctx.HasError() {
		ctx.Flash.Error(ctx.Data["ErrorMsg"].(string))
		ctx.Redirect(ctx.Repo.RepoLink + "/labels")
		return
	}

	l := &models.Label{
		RepoID: ctx.Repo.Repository.ID,
		Name:   form.Title,
		Color:  form.Color,
	}
	if err := models.NewLabel(l); err != nil {
		ctx.Handle(500, "NewLabel", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/labels")
}

func UpdateLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	l, err := models.GetLabelByID(form.ID)
	if err != nil {
		switch {
		case models.IsErrLabelNotExist(err):
			ctx.Error(404)
		default:
			ctx.Handle(500, "UpdateLabel", err)
		}
		return
	}

	l.Name = form.Title
	l.Color = form.Color
	if err := models.UpdateLabel(l); err != nil {
		ctx.Handle(500, "UpdateLabel", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/labels")
}

func DeleteLabel(ctx *middleware.Context) {
	if err := models.DeleteLabel(ctx.Repo.Repository.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteLabel: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.issues.label_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/labels",
	})
	return
}

func Milestones(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones")
	ctx.Data["PageIsMilestones"] = true

	isShowClosed := ctx.Query("state") == "closed"
	openCount, closedCount := models.MilestoneStats(ctx.Repo.Repository.ID)
	ctx.Data["OpenCount"] = openCount
	ctx.Data["ClosedCount"] = closedCount

	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(openCount)
	} else {
		total = int(closedCount)
	}
	ctx.Data["Page"] = paginater.New(total, setting.IssuePagingNum, page, 5)

	miles, err := models.GetMilestones(ctx.Repo.Repository.ID, page, isShowClosed)
	if err != nil {
		ctx.Handle(500, "GetMilestones", err)
		return
	}
	for _, m := range miles {
		m.RenderedContent = string(base.RenderMarkdown([]byte(m.Content), ctx.Repo.RepoLink))
		m.CalOpenIssues()
	}
	ctx.Data["Milestones"] = miles

	if isShowClosed {
		ctx.Data["State"] = "closed"
	} else {
		ctx.Data["State"] = "open"
	}

	ctx.Data["IsShowClosed"] = isShowClosed
	ctx.HTML(200, MILESTONE)
}

func NewMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.new")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())
	ctx.HTML(200, MILESTONE_NEW)
}

func NewMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.new")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_NEW)
		return
	}

	if len(form.Deadline) == 0 {
		form.Deadline = "9999-12-31"
	}
	deadline, err := time.Parse("2006-01-02", form.Deadline)
	if err != nil {
		ctx.Data["Err_Deadline"] = true
		ctx.RenderWithErr(ctx.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &form)
		return
	}

	if err = models.NewMilestone(&models.Milestone{
		RepoID:   ctx.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Content,
		Deadline: deadline,
	}); err != nil {
		ctx.Handle(500, "NewMilestone", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.milestones.create_success", form.Title))
	ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
}

func EditMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.edit")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["PageIsEditMilestone"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", nil)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}
	ctx.Data["title"] = m.Name
	ctx.Data["content"] = m.Content
	if len(m.DeadlineString) > 0 {
		ctx.Data["deadline"] = m.DeadlineString
	}
	ctx.HTML(200, MILESTONE_NEW)
}

func EditMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.edit")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["PageIsEditMilestone"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_NEW)
		return
	}

	if len(form.Deadline) == 0 {
		form.Deadline = "9999-12-31"
	}
	deadline, err := time.Parse("2006-01-02", form.Deadline)
	if err != nil {
		ctx.Data["Err_Deadline"] = true
		ctx.RenderWithErr(ctx.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &form)
		return
	}

	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", nil)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}
	m.Name = form.Title
	m.Content = form.Content
	m.Deadline = deadline
	if err = models.UpdateMilestone(m); err != nil {
		ctx.Handle(500, "UpdateMilestone", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.milestones.edit_success", m.Name))
	ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
}

func ChangeMilestonStatus(ctx *middleware.Context) {
	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", err)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}

	switch ctx.Params(":action") {
	case "open":
		if m.IsClosed {
			if err = models.ChangeMilestoneStatus(m, false); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones?state=open")
	case "close":
		if !m.IsClosed {
			m.ClosedDate = time.Now()
			if err = models.ChangeMilestoneStatus(m, true); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones?state=closed")
	default:
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
	}
}

func DeleteMilestone(ctx *middleware.Context) {
	if err := models.DeleteMilestoneByID(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteMilestoneByID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.milestones.deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/milestones",
	})
}

func PullRequest2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/pr2/list")
}
