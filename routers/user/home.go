// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"

	"github.com/Unknwon/com"
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
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

	actions, err := models.GetFeeds(ctx.User.Id, 0, false)
	if err != nil {
		ctx.Handle(500, "user.Dashboard", err)
		return
	}

	feeds := make([]*models.Action, 0, len(actions))
	for _, act := range actions {
		if act.IsPrivate {
			if has, _ := models.HasAccess(ctx.User.Name, act.RepoUserName+"/"+act.RepoName,
				models.AU_READABLE); !has {
				continue
			}
		}
		feeds = append(feeds, act)
	}

	ctx.Data["Feeds"] = feeds
	ctx.HTML(200, "user/dashboard")
}

func Profile(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Profile"

	user, err := models.GetUserByName(params["username"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "user.Profile", err)
		} else {
			ctx.Handle(500, "user.Profile", err)
		}
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
		return
	}

	feeds := make([]string, 0, len(actions))
	for _, act := range actions {
		if act.IsPrivate {
			if has, _ := models.HasAccess(ctx.User.Name, act.RepoUserName+"/"+act.RepoName,
				models.AU_READABLE); !has {
				continue
			}
		}
		feeds = append(feeds, fmt.Sprintf(TPL_FEED, base.ActionIcon(act.OpType),
			base.TimeSince(act.Created), base.ActionDesc(act)))
	}
	ctx.JSON(200, &feeds)
}

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = "Your Issues"

	viewType := ctx.Query("type")
	types := []string{"assigned", "created_by"}
	if !com.IsSliceContainsStr(types, viewType) {
		viewType = "all"
	}

	isShowClosed := ctx.Query("state") == "closed"

	var assigneeId, posterId int64
	var filterMode int
	switch viewType {
	case "assigned":
		assigneeId = ctx.User.Id
		filterMode = models.FM_ASSIGN
	case "created_by":
		posterId = ctx.User.Id
		filterMode = models.FM_CREATE
	}
	_, _ = assigneeId, posterId

	rid, _ := base.StrTo(ctx.Query("repoid")).Int64()
	issueStats := models.GetUserIssueStats(ctx.User.Id, filterMode)

	// Get all repositories.
	repos, err := models.GetRepositories(ctx.User, true)
	if err != nil {
		ctx.Handle(500, "user.Issues(GetRepositories)", err)
		return
	}

	rids := make([]int64, 0, len(repos))
	showRepos := make([]*models.Repository, 0, len(repos))
	for _, repo := range repos {
		if repo.NumIssues == 0 {
			continue
		}

		rids = append(rids, repo.Id)
		repo.NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
		issueStats.AllCount += int64(repo.NumOpenIssues)

		if isShowClosed {
			if repo.NumClosedIssues > 0 {
				if filterMode == models.FM_CREATE {
					repo.NumClosedIssues = int(models.GetIssueCountByPoster(ctx.User.Id, repo.Id, isShowClosed))
				}
				showRepos = append(showRepos, repo)
			}
		} else {
			if repo.NumOpenIssues > 0 {
				if filterMode == models.FM_CREATE {
					repo.NumOpenIssues = int(models.GetIssueCountByPoster(ctx.User.Id, repo.Id, isShowClosed))
				}
				showRepos = append(showRepos, repo)
			}
		}
	}

	if rid > 0 {
		rids = []int64{rid}
	}

	page, _ := base.StrTo(ctx.Query("page")).Int()

	// Get all issues.
	var ius []*models.IssueUser
	switch viewType {
	case "assigned":
		fallthrough
	case "created_by":
		ius, err = models.GetIssueUserPairsByMode(ctx.User.Id, rid, isShowClosed, page, filterMode)
	default:
		ius, err = models.GetIssueUserPairsByRepoIds(rids, isShowClosed, page)
	}
	if err != nil {
		ctx.Handle(500, "user.Issues(GetAllIssueUserPairs)", err)
		return
	}

	issues := make([]*models.Issue, len(ius))
	for i := range ius {
		issues[i], err = models.GetIssueById(ius[i].IssueId)
		if err != nil {
			if err == models.ErrIssueNotExist {
				log.Error("user.Issues(#%d): issue not exist", ius[i].IssueId)
				continue
			} else {
				ctx.Handle(500, "user.Issues(GetIssue)", err)
				return
			}
		}

		issues[i].Repo, err = models.GetRepositoryById(issues[i].RepoId)
		if err != nil {
			ctx.Handle(500, "user.Issues(GetRepositoryById)", err)
			return
		}

		issues[i].Poster, err = models.GetUserById(issues[i].PosterId)
		if err != nil {
			ctx.Handle(500, "user.Issues(GetUserById)", err)
			return
		}
	}

	ctx.Data["RepoId"] = rid
	ctx.Data["Repos"] = showRepos
	ctx.Data["Issues"] = issues
	ctx.Data["ViewType"] = viewType
	ctx.Data["IssueStats"] = issueStats
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
		ctx.Data["ShowCount"] = issueStats.ClosedCount
	} else {
		ctx.Data["ShowCount"] = issueStats.OpenCount
	}
	ctx.HTML(200, "issue/user")
}

func Pulls(ctx *middleware.Context) {
	ctx.HTML(200, "user/pulls")
}

func Stars(ctx *middleware.Context) {
	ctx.HTML(200, "user/stars")
}
