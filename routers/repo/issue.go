// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
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
		ctx.SetCookie("redirect_to", "/"+url.QueryEscape(ctx.Req.RequestURI))
		ctx.Redirect("/user/login")
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

	mid, _ := base.StrTo(ctx.Query("milestone")).Int64()
	page, _ := base.StrTo(ctx.Query("page")).Int()

	// Get issues.
	issues, err := models.GetIssues(assigneeId, ctx.Repo.Repository.Id, posterId, mid, page,
		isShowClosed, ctx.Query("labels"), ctx.Query("sortType"))
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
	ctx.Data["ViewType"] = viewType
	ctx.Data["Issues"] = issues
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
		ctx.Data["ShowCount"] = issueStats.ClosedCount
	} else {
		ctx.Data["ShowCount"] = issueStats.OpenCount
	}
	ctx.HTML(200, "issue/list")
}

func CreateIssue(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false

	us, err := models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(GetCollaborators)", err)
		return
	}
	ctx.Data["Collaborators"] = us
	ctx.HTML(200, "issue/create")
}

func CreateIssuePost(ctx *middleware.Context, params martini.Params, form auth.CreateIssueForm) {
	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false

	us, err := models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(GetCollaborators)", err)
		return
	}
	ctx.Data["Collaborators"] = us

	if ctx.HasError() {
		ctx.HTML(200, "issue/create")
		return
	}

	// Only collaborators can assign.
	if !ctx.Repo.IsOwner {
		form.AssigneeId = 0
	}
	issue := &models.Issue{
		Index:       int64(ctx.Repo.Repository.NumIssues) + 1,
		Name:        form.IssueName,
		RepoId:      ctx.Repo.Repository.Id,
		PosterId:    ctx.User.Id,
		MilestoneId: form.MilestoneId,
		AssigneeId:  form.AssigneeId,
		Labels:      form.Labels,
		Content:     form.Content,
	}
	if err := models.NewIssue(issue); err != nil {
		ctx.Handle(500, "issue.CreateIssue(NewIssue)", err)
		return
	} else if err := models.NewIssueUserPairs(issue.RepoId, issue.Id, ctx.Repo.Owner.Id,
		ctx.User.Id, form.AssigneeId, ctx.Repo.Repository.Name); err != nil {
		ctx.Handle(500, "issue.CreateIssue(NewIssueUserPairs)", err)
		return
	}

	// Update mentions.
	ms := base.MentionPattern.FindAllString(issue.Content, -1)
	if len(ms) > 0 {
		for i := range ms {
			ms[i] = ms[i][1:]
		}

		ids := models.GetUserIdsByNames(ms)
		if err := models.UpdateIssueUserPairsByMentions(ids, issue.Id); err != nil {
			ctx.Handle(500, "issue.CreateIssue(UpdateIssueUserPairsByMentions)", err)
			return
		}
	}

	act := &models.Action{
		ActUserId:    ctx.User.Id,
		ActUserName:  ctx.User.Name,
		ActEmail:     ctx.User.Email,
		OpType:       models.OP_CREATE_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Name),
		RepoId:       ctx.Repo.Repository.Id,
		RepoUserName: ctx.Repo.Owner.Name,
		RepoName:     ctx.Repo.Repository.Name,
		RefName:      ctx.Repo.BranchName,
		IsPrivate:    ctx.Repo.Repository.IsPrivate,
	}
	// Notify watchers.
	if err := models.NotifyWatchers(act); err != nil {
		ctx.Handle(500, "issue.CreateIssue(NotifyWatchers)", err)
		return
	}

	// Mail watchers and mentions.
	if base.Service.NotifyMail {
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			ctx.Handle(500, "issue.CreateIssue(SendIssueNotifyMail)", err)
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
			ctx.Handle(500, "issue.CreateIssue(SendIssueMentionMail)", err)
			return
		}
	}
	log.Trace("%d Issue created: %d", ctx.Repo.Repository.Id, issue.Id)

	ctx.Redirect(fmt.Sprintf("/%s/%s/issues/%d", params["username"], params["reponame"], issue.Index))
}

func ViewIssue(ctx *middleware.Context, params martini.Params) {
	idx, _ := base.StrTo(params["index"]).Int64()
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

	us, err := models.GetCollaborators(strings.TrimPrefix(ctx.Repo.RepoLink, "/"))
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(GetCollaborators)", err)
		return
	}
	ctx.Data["Collaborators"] = us

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
		comments[i].Content = string(base.RenderMarkdown([]byte(comments[i].Content), ctx.Repo.RepoLink))
	}

	ctx.Data["Title"] = issue.Name
	ctx.Data["Issue"] = issue
	ctx.Data["Comments"] = comments
	ctx.Data["IsIssueOwner"] = ctx.Repo.IsOwner || (ctx.IsSigned && issue.PosterId == ctx.User.Id)
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false
	ctx.HTML(200, "issue/view")
}

func UpdateIssue(ctx *middleware.Context, params martini.Params, form auth.CreateIssueForm) {
	idx, err := base.StrTo(params["index"]).Int()
	if err != nil {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, int64(idx))
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateIssue", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssue(GetIssueByIndex)", err)
		}
		return
	}

	if ctx.User.Id != issue.PosterId && !ctx.Repo.IsOwner {
		ctx.Handle(404, "issue.UpdateIssue", nil)
		return
	}

	issue.Name = form.IssueName
	issue.MilestoneId = form.MilestoneId
	issue.AssigneeId = form.AssigneeId
	issue.Labels = form.Labels
	issue.Content = form.Content
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

func UpdateAssignee(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(403)
		return
	}

	idx, err := base.StrTo(ctx.Query("issue")).Int64()
	if err != nil {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, idx)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateAssignee", err)
		} else {
			ctx.Handle(500, "issue.UpdateAssignee(GetIssueByIndex)", err)
		}
		return
	}

	aid, _ := base.StrTo(ctx.Query("assigneeid")).Int64()
	// Not check for invalid assignne id and give responsibility to owners.
	issue.AssigneeId = aid
	if err = models.UpdateIssueUserPairByAssignee(aid, issue.Id); err != nil {
		ctx.Handle(500, "issue.UpdateAssignee(UpdateIssueUserPairByAssignee): %v", err)
		return
	} else if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "issue.UpdateAssignee(UpdateIssue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func Comment(ctx *middleware.Context, params martini.Params) {
	index, err := base.StrTo(ctx.Query("issueIndex")).Int64()
	if err != nil {
		ctx.Handle(404, "issue.Comment(get index)", err)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, index)
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.Comment", err)
		} else {
			ctx.Handle(200, "issue.Comment(get issue)", err)
		}
		return
	}

	// TODO: check collaborators
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
				ctx.Handle(500, "issue.Comment(UpdateIssue)", err)
				return
			} else if err = models.UpdateIssueUserPairsByStatus(issue.Id, issue.IsClosed); err != nil {
				ctx.Handle(500, "issue.Comment(UpdateIssueUserPairsByStatus)", err)
				return
			}

			cmtType := models.IT_CLOSE
			if !issue.IsClosed {
				cmtType = models.IT_REOPEN
			}

			if err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.Id, issue.Id, 0, 0, cmtType, ""); err != nil {
				ctx.Handle(200, "issue.Comment(create status change comment)", err)
				return
			}
			log.Trace("%s Issue(%d) status changed: %v", ctx.Req.RequestURI, issue.Id, !issue.IsClosed)
		}
	}

	var ms []string
	content := ctx.Query("content")
	if len(content) > 0 {
		switch params["action"] {
		case "new":
			if err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.Id, issue.Id, 0, 0, models.IT_PLAIN, content); err != nil {
				ctx.Handle(500, "issue.Comment(create comment)", err)
				return
			}

			// Update mentions.
			ms = base.MentionPattern.FindAllString(issue.Content, -1)
			if len(ms) > 0 {
				for i := range ms {
					ms[i] = ms[i][1:]
				}

				ids := models.GetUserIdsByNames(ms)
				if err := models.UpdateIssueUserPairsByMentions(ids, issue.Id); err != nil {
					ctx.Handle(500, "issue.CreateIssue(UpdateIssueUserPairsByMentions)", err)
					return
				}
			}

			log.Trace("%s Comment created: %d", ctx.Req.RequestURI, issue.Id)
		default:
			ctx.Handle(404, "issue.Comment", err)
			return
		}
	}

	// Notify watchers.
	if err = models.NotifyWatchers(&models.Action{ActUserId: ctx.User.Id, ActUserName: ctx.User.Name, ActEmail: ctx.User.Email,
		OpType: models.OP_COMMENT_ISSUE, Content: fmt.Sprintf("%d|%s", issue.Index, strings.Split(content, "\n")[0]),
		RepoId: ctx.Repo.Repository.Id, RepoName: ctx.Repo.Repository.Name, RefName: ""}); err != nil {
		ctx.Handle(500, "issue.CreateIssue(NotifyWatchers)", err)
		return
	}

	// Mail watchers and mentions.
	if base.Service.NotifyMail {
		issue.Content = content
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			ctx.Handle(500, "issue.Comment(SendIssueNotifyMail)", err)
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
			ctx.Handle(500, "issue.Comment(SendIssueMentionMail)", err)
			return
		}
	}

	ctx.Redirect(fmt.Sprintf("%s/issues/%d", ctx.Repo.RepoLink, index))
}

func Milestones(ctx *middleware.Context) {
	ctx.Data["Title"] = "Milestones"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	ctx.HTML(200, "issue/milestone")
}

func NewMilestones(ctx *middleware.Context) {
	ctx.Data["Title"] = "New Milestones"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = true

	ctx.HTML(200, "issue/milestone_new")
}
