// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Issues(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Issues"
	ctx.Data["IsRepoToolbarIssues"] = true

	milestoneId, _ := base.StrTo(params["milestone"]).Int()
	page, _ := base.StrTo(params["page"]).Int()

	var err error
	ctx.Data["Issues"], err = models.GetIssues(0, ctx.Repo.Repository.Id, 0,
		int64(milestoneId), page, params["state"] == "closed", false, params["labels"], params["sortType"])
	if err != nil {
		ctx.Handle(200, "issue.Issues: %v", err)
		return
	}

	if len(params["branchname"]) == 0 {
		params["branchname"] = "master"
	}
	ctx.Data["Branchname"] = params["branchname"]
	ctx.HTML(200, "issue/repo")
}

func CreateIssue(ctx *middleware.Context, params martini.Params, form auth.CreateIssueForm) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "issue.CreateIssue", nil)
		return
	}

	ctx.Data["Title"] = "Create issue"
	ctx.Data["IsRepoToolbarIssues"] = true

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "issue/create")
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, "issue/create")
		return
	}

	issue, err := models.CreateIssue(ctx.User.Id, form.RepoId, form.MilestoneId, form.AssigneeId,
		form.IssueName, form.Labels, form.Content, false)
	if err == nil {
		log.Trace("%s Issue created: %d", form.RepoId, issue.Id)
		ctx.Redirect(fmt.Sprintf("/%s/%s/issues/%d", params["username"], params["reponame"], issue.Index))
		return
	}
	ctx.Handle(200, "issue.CreateIssue", err)
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

	ctx.Data["Title"] = issue.Name
	ctx.Data["Issue"] = issue
	ctx.HTML(200, "issue/view")
}

func UpdateIssue(ctx *middleware.Context, params martini.Params, form auth.CreateIssueForm) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "issue.UpdateIssue", nil)
		return
	}

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
			ctx.Handle(200, "issue.UpdateIssue", err)
		}
		return
	}

	issue.Name = form.IssueName
	issue.MilestoneId = form.MilestoneId
	issue.AssigneeId = form.AssigneeId
	issue.Labels = form.Labels
	issue.Content = form.Content
	if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(200, "issue.UpdateIssue", err)
		return
	}

	ctx.Data["Title"] = issue.Name
	ctx.Data["Issue"] = issue
}
