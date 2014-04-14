// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Dashboard"
	ctx.Data["PageIsUserDashboard"] = true
	repos, err := models.GetRepositories(&models.User{Id: ctx.User.Id}, true)
	if err != nil {
		ctx.Handle(500, "user.Dashboard", err)
		return
	}
	ctx.Data["MyRepos"] = repos

	feeds, err := models.GetFeeds(ctx.User.Id, 0, false)
	if err != nil {
		ctx.Handle(500, "user.Dashboard", err)
		return
	}
	ctx.Data["Feeds"] = feeds
	ctx.HTML(200, "user/dashboard")
}

func Profile(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Profile"

	// TODO: Need to check view self or others.
	user, err := models.GetUserByName(params["username"])
	if err != nil {
		ctx.Handle(500, "user.Profile", err)
		return
	}

	ctx.Data["Owner"] = user

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab

	switch tab {
	case "activity":
		feeds, err := models.GetFeeds(user.Id, 0, true)
		if err != nil {
			ctx.Handle(500, "user.Profile", err)
			return
		}
		ctx.Data["Feeds"] = feeds
	default:
		repos, err := models.GetRepositories(user, ctx.IsSigned && ctx.User.Id == user.Id)
		if err != nil {
			ctx.Handle(500, "user.Profile", err)
			return
		}
		ctx.Data["Repos"] = repos
	}

	ctx.Data["PageIsUserProfile"] = true
	ctx.HTML(200, "user/profile")
}

func Email2User(ctx *middleware.Context) {
	u, err := models.GetUserByEmail(ctx.Query("email"))
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "user.Email2User", err)
		} else {
			ctx.Handle(500, "user.Email2User(GetUserByEmail)", err)
		}
		return
	}

	ctx.Redirect("/user/" + u.Name)
}

const (
	TPL_FEED = `<i class="icon fa fa-%s"></i>
                        <div class="info"><span class="meta">%s</span><br>%s</div>`
)

func Feeds(ctx *middleware.Context, form auth.FeedsForm) {
	actions, err := models.GetFeeds(form.UserId, form.Page*20, false)
	if err != nil {
		ctx.JSON(500, err)
	}

	feeds := make([]string, len(actions))
	for i := range actions {
		feeds[i] = fmt.Sprintf(TPL_FEED, base.ActionIcon(actions[i].OpType),
			base.TimeSince(actions[i].Created), base.ActionDesc(actions[i]))
	}
	ctx.JSON(200, &feeds)
}

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = "Your Issues"
	ctx.Data["ViewType"] = "all"

	page, _ := base.StrTo(ctx.Query("page")).Int()
	repoId, _ := base.StrTo(ctx.Query("repoid")).Int64()

	ctx.Data["RepoId"] = repoId

	var posterId int64 = 0
	if ctx.Query("type") == "created_by" {
		posterId = ctx.User.Id
		ctx.Data["ViewType"] = "created_by"
	}

	// Get all repositories.
	repos, err := models.GetRepositories(ctx.User, true)
	if err != nil {
		ctx.Handle(200, "user.Issues(get repositories)", err)
		return
	}

	showRepos := make([]models.Repository, 0, len(repos))

	isShowClosed := ctx.Query("state") == "closed"
	var closedIssueCount, createdByCount, allIssueCount int

	// Get all issues.
	allIssues := make([]models.Issue, 0, 5*len(repos))
	for i, repo := range repos {
		issues, err := models.GetIssues(0, repo.Id, posterId, 0, page, isShowClosed, false, "", "")
		if err != nil {
			ctx.Handle(200, "user.Issues(get issues)", err)
			return
		}

		allIssueCount += repo.NumIssues
		closedIssueCount += repo.NumClosedIssues

		// Set repository information to issues.
		for j := range issues {
			issues[j].Repo = &repos[i]
		}
		allIssues = append(allIssues, issues...)

		repos[i].NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
		if repos[i].NumOpenIssues > 0 {
			showRepos = append(showRepos, repos[i])
		}
	}

	showIssues := make([]models.Issue, 0, len(allIssues))
	ctx.Data["IsShowClosed"] = isShowClosed

	// Get posters and filter issues.
	for i := range allIssues {
		u, err := models.GetUserById(allIssues[i].PosterId)
		if err != nil {
			ctx.Handle(200, "user.Issues(get poster): %v", err)
			return
		}
		allIssues[i].Poster = u
		if u.Id == ctx.User.Id {
			createdByCount++
		}

		if repoId > 0 && repoId != allIssues[i].Repo.Id {
			continue
		}

		if isShowClosed == allIssues[i].IsClosed {
			showIssues = append(showIssues, allIssues[i])
		}
	}

	ctx.Data["Repos"] = showRepos
	ctx.Data["Issues"] = showIssues
	ctx.Data["AllIssueCount"] = allIssueCount
	ctx.Data["ClosedIssueCount"] = closedIssueCount
	ctx.Data["OpenIssueCount"] = allIssueCount - closedIssueCount
	ctx.Data["CreatedByCount"] = createdByCount
	ctx.HTML(200, "issue/user")
}

func Pulls(ctx *middleware.Context) {
	ctx.HTML(200, "user/pulls")
}

func Stars(ctx *middleware.Context) {
	ctx.HTML(200, "user/stars")
}
