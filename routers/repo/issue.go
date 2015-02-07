// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	ISSUES       base.TplName = "repo/issue/list"
	ISSUE_CREATE base.TplName = "repo/issue/create"
	ISSUE_VIEW   base.TplName = "repo/issue/view"

	MILESTONE      base.TplName = "repo/issue/milestone"
	MILESTONE_NEW  base.TplName = "repo/issue/milestone_new"
	MILESTONE_EDIT base.TplName = "repo/issue/milestone_edit"
)

var (
	ErrFileTypeForbidden = errors.New("File type is not allowed")
	ErrTooManyFiles      = errors.New("Maximum number of files to upload exceeded")
)

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = "Issues"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	viewType := ctx.Query("type")
	types := []string{"assigned", "created_by", "mentioned"}
	if !com.IsSliceContainsStr(types, viewType) {
		viewType = "all"
	}

	isShowClosed := ctx.Query("state") == "closed"

	if viewType != "all" && !ctx.IsSigned {
		ctx.SetCookie("redirect_to", "/"+url.QueryEscape(setting.AppSubUrl+ctx.Req.RequestURI), 0, setting.AppSubUrl)
		ctx.Redirect(setting.AppSubUrl + "/user/login")
		return
	}

	var assigneeId, posterId int64
	var filterMode int
	switch viewType {
	case "assigned":
		assigneeId = ctx.User.Id
		filterMode = models.FM_ASSIGN
	case "created_by":
		posterId = ctx.User.Id
		filterMode = models.FM_CREATE
	case "mentioned":
		filterMode = models.FM_MENTION
	}

	var mid int64
	midx, _ := com.StrTo(ctx.Query("milestone")).Int64()
	if midx > 0 {
		mile, err := models.GetMilestoneByIndex(ctx.Repo.Repository.Id, midx)
		if err != nil {
			ctx.Handle(500, "issue.Issues(GetMilestoneByIndex): %v", err)
			return
		}
		mid = mile.Id
	}

	selectLabels := ctx.Query("labels")
	labels, err := models.GetLabels(ctx.Repo.Repository.Id)
	if err != nil {
		ctx.Handle(500, "issue.Issues(GetLabels): %v", err)
		return
	}
	for _, l := range labels {
		l.CalOpenIssues()
	}
	ctx.Data["Labels"] = labels

	page, _ := com.StrTo(ctx.Query("page")).Int()

	// Get issues.
	issues, err := models.GetIssues(assigneeId, ctx.Repo.Repository.Id, posterId, mid, page,
		isShowClosed, selectLabels, ctx.Query("sortType"))
	if err != nil {
		ctx.Handle(500, "issue.Issues(GetIssues): %v", err)
		return
	}

	// Get issue-user pairs.
	pairs, err := models.GetIssueUserPairs(ctx.Repo.Repository.Id, posterId, isShowClosed)
	if err != nil {
		ctx.Handle(500, "issue.Issues(GetIssueUserPairs): %v", err)
		return
	}

	// Get posters.
	for i := range issues {
		if err = issues[i].GetLabels(); err != nil {
			ctx.Handle(500, "GetLabels", fmt.Errorf("[#%d]%v", issues[i].Id, err))
			return
		}

		idx := models.PairsContains(pairs, issues[i].Id)

		if filterMode == models.FM_MENTION && (idx == -1 || !pairs[idx].IsMentioned) {
			continue
		}

		if idx > -1 {
			issues[i].IsRead = pairs[idx].IsRead
		} else {
			issues[i].IsRead = true
		}

		if err = issues[i].GetPoster(); err != nil {
			ctx.Handle(500, "issue.Issues(GetPoster)", fmt.Errorf("[#%d]%v", issues[i].Id, err))
			return
		}
	}

	var uid int64 = -1
	if ctx.User != nil {
		uid = ctx.User.Id
	}
	issueStats := models.GetIssueStats(ctx.Repo.Repository.Id, uid, isShowClosed, filterMode)
	ctx.Data["IssueStats"] = issueStats
	ctx.Data["SelectLabels"], _ = com.StrTo(selectLabels).Int64()
	ctx.Data["ViewType"] = viewType
	ctx.Data["Issues"] = issues
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
		ctx.Data["ShowCount"] = issueStats.ClosedCount
	} else {
		ctx.Data["ShowCount"] = issueStats.OpenCount
	}
	ctx.HTML(200, ISSUES)
}

func CreateIssue(ctx *middleware.Context) {
	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false
	ctx.Data["AttachmentsEnabled"] = setting.AttachmentEnabled

	var err error
	// Get all milestones.
	ctx.Data["OpenMilestones"], err = models.GetMilestones(ctx.Repo.Repository.Id, false)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetMilestones.1): %v", err)
		return
	}
	ctx.Data["ClosedMilestones"], err = models.GetMilestones(ctx.Repo.Repository.Id, true)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetMilestones.2): %v", err)
		return
	}

	us, err := models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(GetCollaborators)", err)
		return
	}

	ctx.Data["AllowedTypes"] = setting.AttachmentAllowedTypes
	ctx.Data["Collaborators"] = us

	ctx.HTML(200, ISSUE_CREATE)
}

func CreateIssuePost(ctx *middleware.Context, form auth.CreateIssueForm) {
	send := func(status int, data interface{}, err error) {
		if err != nil {
			log.Error(4, "issue.CreateIssuePost(?): %s", err)

			ctx.JSON(status, map[string]interface{}{
				"ok":     false,
				"status": status,
				"error":  err.Error(),
			})
		} else {
			ctx.JSON(status, map[string]interface{}{
				"ok":     true,
				"status": status,
				"data":   data,
			})
		}
	}

	var err error
	// Get all milestones.
	_, err = models.GetMilestones(ctx.Repo.Repository.Id, false)
	if err != nil {
		send(500, nil, err)
		return
	}
	_, err = models.GetMilestones(ctx.Repo.Repository.Id, true)
	if err != nil {
		send(500, nil, err)
		return
	}

	_, err = models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		send(500, nil, err)
		return
	}

	if ctx.HasError() {
		send(400, nil, errors.New(ctx.Flash.ErrorMsg))
		return
	}

	// Only collaborators can assign.
	if !ctx.Repo.IsOwner {
		form.AssigneeId = 0
	}
	issue := &models.Issue{
		RepoId:      ctx.Repo.Repository.Id,
		Index:       int64(ctx.Repo.Repository.NumIssues) + 1,
		Name:        form.IssueName,
		PosterId:    ctx.User.Id,
		MilestoneId: form.MilestoneId,
		AssigneeId:  form.AssigneeId,
		LabelIds:    form.Labels,
		Content:     form.Content,
	}
	if err := models.NewIssue(issue); err != nil {
		send(500, nil, err)
		return
	} else if err := models.NewIssueUserPairs(issue.RepoId, issue.Id, ctx.Repo.Owner.Id,
		ctx.User.Id, form.AssigneeId, ctx.Repo.Repository.Name); err != nil {
		send(500, nil, err)
		return
	}

	if setting.AttachmentEnabled {
		uploadFiles(ctx, issue.Id, 0)
	}

	// Update mentions.
	ms := base.MentionPattern.FindAllString(issue.Content, -1)
	if len(ms) > 0 {
		for i := range ms {
			ms[i] = ms[i][1:]
		}

		if err := models.UpdateMentions(ms, issue.Id); err != nil {
			send(500, nil, err)
			return
		}
	}

	act := &models.Action{
		ActUserId:    ctx.User.Id,
		ActUserName:  ctx.User.Name,
		ActEmail:     ctx.User.Email,
		OpType:       models.CREATE_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Name),
		RepoId:       ctx.Repo.Repository.Id,
		RepoUserName: ctx.Repo.Owner.Name,
		RepoName:     ctx.Repo.Repository.Name,
		RefName:      ctx.Repo.BranchName,
		IsPrivate:    ctx.Repo.Repository.IsPrivate,
	}
	// Notify watchers.
	if err := models.NotifyWatchers(act); err != nil {
		send(500, nil, err)
		return
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			send(500, nil, err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(ms))
		for _, m := range ms {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			send(500, nil, err)
			return
		}
	}
	log.Trace("%d Issue created: %d", ctx.Repo.Repository.Id, issue.Id)

	send(200, fmt.Sprintf("%s/%s/%s/issues/%d", setting.AppSubUrl, ctx.Params(":username"), ctx.Params(":reponame"), issue.Index), nil)
}

func checkLabels(labels, allLabels []*models.Label) {
	for _, l := range labels {
		for _, l2 := range allLabels {
			if l.Id == l2.Id {
				l2.IsChecked = true
				break
			}
		}
	}
}

func ViewIssue(ctx *middleware.Context) {
	ctx.Data["AttachmentsEnabled"] = setting.AttachmentEnabled

	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx == 0 {
		ctx.Handle(404, "issue.ViewIssue", nil)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.ViewIssue(GetIssueByIndex)", err)
		} else {
			ctx.Handle(500, "issue.ViewIssue(GetIssueByIndex)", err)
		}
		return
	}

	// Get labels.
	if err = issue.GetLabels(); err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetLabels)", err)
		return
	}
	labels, err := models.GetLabels(ctx.Repo.Repository.Id)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetLabels.2)", err)
		return
	}
	checkLabels(issue.Labels, labels)
	ctx.Data["Labels"] = labels

	// Get assigned milestone.
	if issue.MilestoneId > 0 {
		ctx.Data["Milestone"], err = models.GetMilestoneById(issue.MilestoneId)
		if err != nil {
			if err == models.ErrMilestoneNotExist {
				log.Warn("issue.ViewIssue(GetMilestoneById): %v", err)
			} else {
				ctx.Handle(500, "issue.ViewIssue(GetMilestoneById)", err)
				return
			}
		}
	}

	// Get all milestones.
	ctx.Data["OpenMilestones"], err = models.GetMilestones(ctx.Repo.Repository.Id, false)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetMilestones.1): %v", err)
		return
	}
	ctx.Data["ClosedMilestones"], err = models.GetMilestones(ctx.Repo.Repository.Id, true)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetMilestones.2): %v", err)
		return
	}

	// Get all collaborators.
	ctx.Data["Collaborators"], err = models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(GetCollaborators)", err)
		return
	}

	if ctx.IsSigned {
		// Update issue-user.
		if err = models.UpdateIssueUserPairByRead(ctx.User.Id, issue.Id); err != nil {
			ctx.Handle(500, "issue.ViewIssue(UpdateIssueUserPairByRead): %v", err)
			return
		}
	}

	// Get poster and Assignee.
	if err = issue.GetPoster(); err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetPoster): %v", err)
		return
	} else if err = issue.GetAssignee(); err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetAssignee): %v", err)
		return
	}
	issue.RenderedContent = string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink))

	// Get comments.
	comments, err := models.GetIssueComments(issue.Id)
	if err != nil {
		ctx.Handle(500, "issue.ViewIssue(GetIssueComments): %v", err)
		return
	}

	// Get posters.
	for i := range comments {
		u, err := models.GetUserById(comments[i].PosterId)
		if err != nil {
			ctx.Handle(500, "issue.ViewIssue(GetUserById.2): %v", err)
			return
		}
		comments[i].Poster = u

		if comments[i].Type == models.COMMENT_TYPE_COMMENT {
			comments[i].Content = string(base.RenderMarkdown([]byte(comments[i].Content), ctx.Repo.RepoLink))
		}
	}

	ctx.Data["AllowedTypes"] = setting.AttachmentAllowedTypes

	ctx.Data["Title"] = issue.Name
	ctx.Data["Issue"] = issue
	ctx.Data["Comments"] = comments
	ctx.Data["IsIssueOwner"] = ctx.Repo.IsOwner || (ctx.IsSigned && issue.PosterId == ctx.User.Id)
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false
	ctx.HTML(200, ISSUE_VIEW)
}

func UpdateIssue(ctx *middleware.Context, form auth.CreateIssueForm) {
	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx <= 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateIssue", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssue(GetIssueByIndex)", err)
		}
		return
	}

	if ctx.User.Id != issue.PosterId && !ctx.Repo.IsOwner {
		ctx.Error(403)
		return
	}

	issue.Name = form.IssueName
	//issue.MilestoneId = form.MilestoneId
	//issue.AssigneeId = form.AssigneeId
	//issue.LabelIds = form.Labels
	issue.Content = form.Content
	// try get content from text, ignore conflict with preview ajax
	if form.Content == "" {
		issue.Content = ctx.Query("text")
	}
	if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssue(UpdateIssue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok":      true,
		"title":   issue.Name,
		"content": string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink)),
	})
}

func UpdateIssueLabel(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(403)
		return
	}

	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx <= 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateIssueLabel(GetIssueByIndex)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueLabel(GetIssueByIndex)", err)
		}
		return
	}

	isAttach := ctx.Query("action") == "attach"
	labelStrId := ctx.Query("id")
	labelId := com.StrTo(labelStrId).MustInt64()
	label, err := models.GetLabelById(labelId)
	if err != nil {
		if err == models.ErrLabelNotExist {
			ctx.Handle(404, "issue.UpdateIssueLabel(GetLabelById)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueLabel(GetLabelById)", err)
		}
		return
	}

	isHad := strings.Contains(issue.LabelIds, "$"+labelStrId+"|")
	isNeedUpdate := false
	if isAttach {
		if !isHad {
			issue.LabelIds += "$" + labelStrId + "|"
			isNeedUpdate = true
		}
	} else {
		if isHad {
			issue.LabelIds = strings.Replace(issue.LabelIds, "$"+labelStrId+"|", "", -1)
			isNeedUpdate = true
		}
	}

	if isNeedUpdate {
		if err = models.UpdateIssue(issue); err != nil {
			ctx.Handle(500, "issue.UpdateIssueLabel(UpdateIssue)", err)
			return
		}

		if isAttach {
			label.NumIssues++
			if issue.IsClosed {
				label.NumClosedIssues++
			}
		} else {
			label.NumIssues--
			if issue.IsClosed {
				label.NumClosedIssues--
			}
		}
		if err = models.UpdateLabel(label); err != nil {
			ctx.Handle(500, "issue.UpdateIssueLabel(UpdateLabel)", err)
			return
		}
	}
	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateIssueMilestone(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(403)
		return
	}

	issueId := com.StrTo(ctx.Query("issue")).MustInt64()
	if issueId == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueById(issueId)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateIssueMilestone(GetIssueById)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueMilestone(GetIssueById)", err)
		}
		return
	}

	oldMid := issue.MilestoneId
	mid := com.StrTo(ctx.Query("milestoneid")).MustInt64()
	if oldMid == mid {
		ctx.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	// Not check for invalid milestone id and give responsibility to owners.
	issue.MilestoneId = mid
	if err = models.ChangeMilestoneAssign(oldMid, mid, issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssueMilestone(ChangeMilestoneAssign)", err)
		return
	} else if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssueMilestone(UpdateIssue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateAssignee(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(403)
		return
	}

	issueId := com.StrTo(ctx.Query("issue")).MustInt64()
	if issueId == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueById(issueId)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "GetIssueById", err)
		} else {
			ctx.Handle(500, "GetIssueById", err)
		}
		return
	}

	aid := com.StrTo(ctx.Query("assigneeid")).MustInt64()
	// Not check for invalid assignee id and give responsibility to owners.
	issue.AssigneeId = aid
	if err = models.UpdateIssueUserPairByAssignee(aid, issue.Id); err != nil {
		ctx.Handle(500, "UpdateIssueUserPairByAssignee: %v", err)
		return
	} else if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "UpdateIssue", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func uploadFiles(ctx *middleware.Context, issueId, commentId int64) {
	if !setting.AttachmentEnabled {
		return
	}

	allowedTypes := strings.Split(setting.AttachmentAllowedTypes, "|")
	attachments := ctx.Req.MultipartForm.File["attachments"]

	if len(attachments) > setting.AttachmentMaxFiles {
		ctx.Handle(400, "issue.Comment", ErrTooManyFiles)
		return
	}

	for _, header := range attachments {
		file, err := header.Open()

		if err != nil {
			ctx.Handle(500, "issue.Comment(header.Open)", err)
			return
		}

		defer file.Close()

		buf := make([]byte, 1024)
		n, _ := file.Read(buf)
		if n > 0 {
			buf = buf[:n]
		}
		fileType := http.DetectContentType(buf)
		fmt.Println(fileType)

		allowed := false

		for _, t := range allowedTypes {
			t := strings.Trim(t, " ")

			if t == "*/*" || t == fileType {
				allowed = true
				break
			}
		}

		if !allowed {
			ctx.Handle(400, "issue.Comment", ErrFileTypeForbidden)
			return
		}

		out, err := ioutil.TempFile(setting.AttachmentPath, "attachment_")

		if err != nil {
			ctx.Handle(500, "ioutil.TempFile", err)
			return
		}

		defer out.Close()

		out.Write(buf)
		_, err = io.Copy(out, file)
		if err != nil {
			ctx.Handle(500, "io.Copy", err)
			return
		}

		_, err = models.CreateAttachment(issueId, commentId, header.Filename, out.Name())
		if err != nil {
			ctx.Handle(500, "CreateAttachment", err)
			return
		}
	}
}

func Comment(ctx *middleware.Context) {
	send := func(status int, data interface{}, err error) {
		if err != nil {
			log.Error(4, "issue.Comment(?): %s", err)

			ctx.JSON(status, map[string]interface{}{
				"ok":     false,
				"status": status,
				"error":  err.Error(),
			})
		} else {
			ctx.JSON(status, map[string]interface{}{
				"ok":     true,
				"status": status,
				"data":   data,
			})
		}
	}

	index := com.StrTo(ctx.Query("issueIndex")).MustInt64()
	if index == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, index)
	if err != nil {
		if err == models.ErrIssueNotExist {
			send(404, nil, err)
		} else {
			send(200, nil, err)
		}

		return
	}

	// Check if issue owner changes the status of issue.
	var newStatus string
	if ctx.Repo.IsOwner || issue.PosterId == ctx.User.Id {
		newStatus = ctx.Query("change_status")
	}
	if len(newStatus) > 0 {
		if (strings.Contains(newStatus, "Reopen") && issue.IsClosed) ||
			(strings.Contains(newStatus, "Close") && !issue.IsClosed) {
			issue.IsClosed = !issue.IsClosed
			if err = models.UpdateIssue(issue); err != nil {
				send(500, nil, err)
				return
			} else if err = models.UpdateIssueUserPairsByStatus(issue.Id, issue.IsClosed); err != nil {
				send(500, nil, err)
				return
			}

			// Change open/closed issue counter for the associated milestone
			if issue.MilestoneId > 0 {
				if err = models.ChangeMilestoneIssueStats(issue); err != nil {
					send(500, nil, err)
				}
			}

			cmtType := models.COMMENT_TYPE_CLOSE
			if !issue.IsClosed {
				cmtType = models.COMMENT_TYPE_REOPEN
			}

			if _, err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.Id, issue.Id, 0, 0, cmtType, "", nil); err != nil {
				send(200, nil, err)
				return
			}
			log.Trace("%s Issue(%d) status changed: %v", ctx.Req.RequestURI, issue.Id, !issue.IsClosed)
		}
	}

	var comment *models.Comment

	var ms []string
	content := ctx.Query("content")
	// Fix #321. Allow empty comments, as long as we have attachments.
	if len(content) > 0 || len(ctx.Req.MultipartForm.File["attachments"]) > 0 {
		switch ctx.Params(":action") {
		case "new":
			if comment, err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.Id, issue.Id, 0, 0, models.COMMENT_TYPE_COMMENT, content, nil); err != nil {
				send(500, nil, err)
				return
			}

			// Update mentions.
			ms = base.MentionPattern.FindAllString(issue.Content, -1)
			if len(ms) > 0 {
				for i := range ms {
					ms[i] = ms[i][1:]
				}

				if err := models.UpdateMentions(ms, issue.Id); err != nil {
					send(500, nil, err)
					return
				}
			}

			log.Trace("%s Comment created: %d", ctx.Req.RequestURI, issue.Id)
		default:
			ctx.Handle(404, "issue.Comment", err)
			return
		}
	}

	if comment != nil {
		uploadFiles(ctx, issue.Id, comment.Id)
	}

	// Notify watchers.
	act := &models.Action{
		ActUserId:    ctx.User.Id,
		ActUserName:  ctx.User.LowerName,
		ActEmail:     ctx.User.Email,
		OpType:       models.COMMENT_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, strings.Split(content, "\n")[0]),
		RepoId:       ctx.Repo.Repository.Id,
		RepoUserName: ctx.Repo.Owner.LowerName,
		RepoName:     ctx.Repo.Repository.LowerName,
	}
	if err = models.NotifyWatchers(act); err != nil {
		send(500, nil, err)
		return
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		issue.Content = content
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			send(500, nil, err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(ms))
		for _, m := range ms {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			send(500, nil, err)
			return
		}
	}

	send(200, fmt.Sprintf("%s/issues/%d", ctx.Repo.RepoLink, index), nil)
}

func NewLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	if ctx.HasError() {
		Issues(ctx)
		return
	}

	l := &models.Label{
		RepoId: ctx.Repo.Repository.Id,
		Name:   form.Title,
		Color:  form.Color,
	}
	if err := models.NewLabel(l); err != nil {
		ctx.Handle(500, "issue.NewLabel(NewLabel)", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/issues")
}

func UpdateLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	id := com.StrTo(ctx.Query("id")).MustInt64()
	if id == 0 {
		ctx.Error(404)
		return
	}

	l := &models.Label{
		Id:    id,
		Name:  form.Title,
		Color: form.Color,
	}
	if err := models.UpdateLabel(l); err != nil {
		ctx.Handle(500, "issue.UpdateLabel(UpdateLabel)", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/issues")
}

func DeleteLabel(ctx *middleware.Context) {
	removes := ctx.Query("remove")
	if len(strings.TrimSpace(removes)) == 0 {
		ctx.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	strIds := strings.Split(removes, ",")
	for _, strId := range strIds {
		if err := models.DeleteLabel(ctx.Repo.Repository.Id, strId); err != nil {
			ctx.Handle(500, "issue.DeleteLabel(DeleteLabel)", err)
			return
		}
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func Milestones(ctx *middleware.Context) {
	ctx.Data["Title"] = "Milestones"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	isShowClosed := ctx.Query("state") == "closed"

	miles, err := models.GetMilestones(ctx.Repo.Repository.Id, isShowClosed)
	if err != nil {
		ctx.Handle(500, "issue.Milestones(GetMilestones)", err)
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
	ctx.HTML(200, MILESTONE)
}

func NewMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Milestone"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true
	ctx.HTML(200, MILESTONE_NEW)
}

func NewMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = "New Milestone"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_NEW)
		return
	}

	var deadline time.Time
	var err error
	if len(form.Deadline) == 0 {
		form.Deadline = "12/31/9999"
	}
	deadline, err = time.Parse("01/02/2006", form.Deadline)
	if err != nil {
		ctx.Handle(500, "issue.NewMilestonePost(time.Parse)", err)
		return
	}

	mile := &models.Milestone{
		RepoId:   ctx.Repo.Repository.Id,
		Index:    int64(ctx.Repo.Repository.NumMilestones) + 1,
		Name:     form.Title,
		Content:  form.Content,
		Deadline: deadline,
	}
	if err = models.NewMilestone(mile); err != nil {
		ctx.Handle(500, "issue.NewMilestonePost(NewMilestone)", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/issues/milestones")
}

func UpdateMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = "Update Milestone"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx == 0 {
		ctx.Handle(404, "issue.UpdateMilestone", nil)
		return
	}

	mile, err := models.GetMilestoneByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrMilestoneNotExist {
			ctx.Handle(404, "issue.UpdateMilestone(GetMilestoneByIndex)", err)
		} else {
			ctx.Handle(500, "issue.UpdateMilestone(GetMilestoneByIndex)", err)
		}
		return
	}

	action := ctx.Params(":action")
	if len(action) > 0 {
		switch action {
		case "open":
			if mile.IsClosed {
				if err = models.ChangeMilestoneStatus(mile, false); err != nil {
					ctx.Handle(500, "issue.UpdateMilestone(ChangeMilestoneStatus)", err)
					return
				}
			}
		case "close":
			if !mile.IsClosed {
				mile.ClosedDate = time.Now()
				if err = models.ChangeMilestoneStatus(mile, true); err != nil {
					ctx.Handle(500, "issue.UpdateMilestone(ChangeMilestoneStatus)", err)
					return
				}
			}
		case "delete":
			if err = models.DeleteMilestone(mile); err != nil {
				ctx.Handle(500, "issue.UpdateMilestone(DeleteMilestone)", err)
				return
			}
		}
		ctx.Redirect(ctx.Repo.RepoLink + "/issues/milestones")
		return
	}

	mile.DeadlineString = mile.Deadline.UTC().Format("01/02/2006")
	if mile.DeadlineString == "12/31/9999" {
		mile.DeadlineString = ""
	}
	ctx.Data["Milestone"] = mile

	ctx.HTML(200, MILESTONE_EDIT)
}

func UpdateMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = "Update Milestone"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx == 0 {
		ctx.Handle(404, "issue.UpdateMilestonePost", nil)
		return
	}

	mile, err := models.GetMilestoneByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrMilestoneNotExist {
			ctx.Handle(404, "issue.UpdateMilestonePost(GetMilestoneByIndex)", err)
		} else {
			ctx.Handle(500, "issue.UpdateMilestonePost(GetMilestoneByIndex)", err)
		}
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_EDIT)
		return
	}

	var deadline time.Time
	if len(form.Deadline) == 0 {
		form.Deadline = "12/31/9999"
	}
	deadline, err = time.Parse("01/02/2006", form.Deadline)
	if err != nil {
		ctx.Handle(500, "issue.UpdateMilestonePost(time.Parse)", err)
		return
	}

	mile.Name = form.Title
	mile.Content = form.Content
	mile.Deadline = deadline
	if err = models.UpdateMilestone(mile); err != nil {
		ctx.Handle(500, "issue.UpdateMilestonePost(UpdateMilestone)", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/issues/milestones")
}

func IssueGetAttachment(ctx *middleware.Context) {
	id := com.StrTo(ctx.Params(":id")).MustInt64()
	if id == 0 {
		ctx.Error(404)
		return
	}

	attachment, err := models.GetAttachmentById(id)

	if err != nil {
		ctx.Handle(404, "issue.IssueGetAttachment(models.GetAttachmentById)", err)
		return
	}

	// Fix #312. Attachments with , in their name are not handled correctly by Google Chrome.
	// We must put the name in " manually.
	ctx.ServeFile(attachment.Path, "\""+attachment.Name+"\"")
}

// testing route handler for new issue ui page
// todo : move to Issue() function
func Issues2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/issue2/list")
}

func PullRequest2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/pr2/list")
}

func Labels2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/issue2/labels")
}

func Milestones2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/milestone2/list")
}
