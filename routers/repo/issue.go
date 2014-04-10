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
	ctx.Data["ViewType"] = "all"

	milestoneId, _ := base.StrTo(ctx.Query("milestone")).Int()
	page, _ := base.StrTo(ctx.Query("page")).Int()

	ctx.Data["IssueCreatedCount"] = 0

	var posterId int64 = 0
	isCreatedBy := ctx.Query("type") == "created_by"
	if isCreatedBy {
		if !ctx.IsSigned {
			ctx.SetCookie("redirect_to", "/"+url.QueryEscape(ctx.Req.RequestURI))
			ctx.Redirect("/user/login/", 302)
			return
		}
		ctx.Data["ViewType"] = "created_by"
	}

	// Get issues.
	issues, err := models.GetIssues(0, ctx.Repo.Repository.Id, posterId, int64(milestoneId), page,
		ctx.Query("state") == "closed", false, ctx.Query("labels"), ctx.Query("sortType"))
	if err != nil {
		ctx.Handle(200, "issue.Issues: %v", err)
		return
	}

	if ctx.IsSigned {
		posterId = ctx.User.Id
	}
	var createdByCount int

	showIssues := make([]models.Issue, 0, len(issues))
	// Get posters.
	for i := range issues {
		u, err := models.GetUserById(issues[i].PosterId)
		if err != nil {
			ctx.Handle(200, "issue.Issues(get poster): %v", err)
			return
		}
		if isCreatedBy && u.Id != posterId {
			continue
		}
		if u.Id == posterId {
			createdByCount++
		}
		issues[i].Poster = u
		showIssues = append(showIssues, issues[i])
	}

	ctx.Data["Issues"] = showIssues
	ctx.Data["IssueCount"] = ctx.Repo.Repository.NumIssues
	ctx.Data["OpenCount"] = ctx.Repo.Repository.NumOpenIssues
	ctx.Data["ClosedCount"] = ctx.Repo.Repository.NumClosedIssues
	ctx.Data["IssueCreatedCount"] = createdByCount
	ctx.Data["IsShowClosed"] = ctx.Query("state") == "closed"
	ctx.HTML(200, "issue/list")
}

func CreateIssue(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false
	ctx.HTML(200, "issue/create")
}

func CreateIssuePost(ctx *middleware.Context, params martini.Params, form auth.CreateIssueForm) {
	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true
	ctx.Data["IsRepoToolbarIssuesList"] = false

	if ctx.HasError() {
		ctx.HTML(200, "issue/create")
		return
	}

	issue, err := models.CreateIssue(ctx.User.Id, ctx.Repo.Repository.Id, form.MilestoneId, form.AssigneeId,
		ctx.Repo.Repository.NumIssues, form.IssueName, form.Labels, form.Content, false)
	if err != nil {
		ctx.Handle(500, "issue.CreateIssue(CreateIssue)", err)
		return
	}

	// Notify watchers.
	if err = models.NotifyWatchers(&models.Action{ActUserId: ctx.User.Id, ActUserName: ctx.User.Name, ActEmail: ctx.User.Email,
		OpType: models.OP_CREATE_ISSUE, Content: fmt.Sprintf("%d|%s", issue.Index, issue.Name),
		RepoId: ctx.Repo.Repository.Id, RepoName: ctx.Repo.Repository.Name, RefName: ""}); err != nil {
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
		ms := base.MentionPattern.FindAllString(issue.Content, -1)
		newTos := make([]string, 0, len(ms))
		for _, m := range ms {
			if com.IsSliceContainsStr(tos, m[1:]) {
				continue
			}

			newTos = append(newTos, m[1:])
		}
		if err = mailer.SendIssueMentionMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository,
			issue, models.GetUserEmailsByNames(newTos)); err != nil {
			ctx.Handle(500, "issue.CreateIssue(SendIssueMentionMail)", err)
			return
		}
	}
	log.Trace("%d Issue created: %d", ctx.Repo.Repository.Id, issue.Id)

	ctx.Redirect(fmt.Sprintf("/%s/%s/issues/%d", params["username"], params["reponame"], issue.Index))
}

func ViewIssue(ctx *middleware.Context, params martini.Params) {
	index, err := base.StrTo(params["index"]).Int()
	if err != nil {
		ctx.Handle(404, "issue.ViewIssue", err)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, int64(index))
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.ViewIssue", err)
		} else {
			ctx.Handle(200, "issue.ViewIssue", err)
		}
		return
	}

	// Get posters.
	u, err := models.GetUserById(issue.PosterId)
	if err != nil {
		ctx.Handle(200, "issue.ViewIssue(get poster): %v", err)
		return
	}
	issue.Poster = u
	issue.RenderedContent = string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink))

	// Get comments.
	comments, err := models.GetIssueComments(issue.Id)
	if err != nil {
		ctx.Handle(200, "issue.ViewIssue(get comments): %v", err)
		return
	}

	// Get posters.
	for i := range comments {
		u, err := models.GetUserById(comments[i].PosterId)
		if err != nil {
			ctx.Handle(200, "issue.ViewIssue(get poster): %v", err)
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
	index, err := base.StrTo(params["index"]).Int()
	if err != nil {
		ctx.Handle(404, "issue.UpdateIssue", err)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.Id, int64(index))
	if err != nil {
		if err == models.ErrIssueNotExist {
			ctx.Handle(404, "issue.UpdateIssue", err)
		} else {
			ctx.Handle(200, "issue.UpdateIssue(get issue)", err)
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
		ctx.Handle(200, "issue.UpdateIssue(update issue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok":      true,
		"title":   issue.Name,
		"content": string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink)),
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
				ctx.Handle(200, "issue.Comment(update issue status)", err)
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

	content := ctx.Query("content")
	if len(content) > 0 {
		switch params["action"] {
		case "new":
			if err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.Id, issue.Id, 0, 0, models.IT_PLAIN, content); err != nil {
				ctx.Handle(500, "issue.Comment(create comment)", err)
				return
			}
			log.Trace("%s Comment created: %d", ctx.Req.RequestURI, issue.Id)
		default:
			ctx.Handle(404, "issue.Comment", err)
			return
		}
	}

	ctx.Redirect(fmt.Sprintf("/%s/%s/issues/%d", ctx.User.Name, ctx.Repo.Repository.Name, index))
}
