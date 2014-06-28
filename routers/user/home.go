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

const (
	DASHBOARD base.TplName = "user/dashboard"
	PROFILE   base.TplName = "user/profile"
	ISSUES    base.TplName = "user/issues"
	PULLS     base.TplName = "user/pulls"
	STARS     base.TplName = "user/stars"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Dashboard"
	ctx.Data["PageIsUserDashboard"] = true

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.Dashboard(GetOrganizations)", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs
	ctx.Data["ContextUser"] = ctx.User

	var err error
	ctx.Data["MyRepos"], err = models.GetRepositories(ctx.User.Id, true)
	if err != nil {
		ctx.Handle(500, "home.Dashboard(GetRepositories)", err)
		return
	}

	ctx.Data["CollaborativeRepos"], err = models.GetCollaborativeRepos(ctx.User.Name)
	if err != nil {
		ctx.Handle(500, "home.Dashboard(GetCollaborativeRepos)", err)
		return
	}

	actions, err := models.GetFeeds(ctx.User.Id, 0, false)
	if err != nil {
		ctx.Handle(500, "home.Dashboard(GetFeeds)", err)
		return
	}

	// Check access of private repositories.
	feeds := make([]*models.Action, 0, len(actions))
	for _, act := range actions {
		if act.IsPrivate {
			if has, _ := models.HasAccess(ctx.User.Name, act.RepoUserName+"/"+act.RepoName,
				models.READABLE); !has {
				continue
			}
		}
		feeds = append(feeds, act)
	}
	ctx.Data["Feeds"] = feeds
	ctx.HTML(200, DASHBOARD)
}

func Profile(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Profile"
	ctx.Data["PageIsUserProfile"] = true

	u, err := models.GetUserByName(params["username"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "user.Profile(GetUserByName)", err)
		} else {
			ctx.Handle(500, "user.Profile(GetUserByName)", err)
		}
		return
	}

	if u.IsOrganization() {
		ctx.Redirect("/org/" + u.Name)
		return
	}

	// For security reason, hide e-mail address for anonymous visitors.
	if !ctx.IsSigned {
		u.Email = ""
	}
	ctx.Data["Owner"] = u

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab
	switch tab {
	case "activity":
		ctx.Data["Feeds"], err = models.GetFeeds(u.Id, 0, true)
		if err != nil {
			ctx.Handle(500, "user.Profile(GetFeeds)", err)
			return
		}
	default:
		ctx.Data["Repos"], err = models.GetRepositories(u.Id, ctx.IsSigned && ctx.User.Id == u.Id)
		if err != nil {
			ctx.Handle(500, "user.Profile(GetRepositories)", err)
			return
		}
	}

	ctx.HTML(200, PROFILE)
}

func Email2User(ctx *middleware.Context) {
	u, err := models.GetUserByEmail(ctx.Query("email"))
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "user.Email2User(GetUserByEmail)", err)
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
				models.READABLE); !has {
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

	var filterMode int
	switch viewType {
	case "assigned":
		filterMode = models.FM_ASSIGN
	case "created_by":
		filterMode = models.FM_CREATE
	}

	repoId, _ := base.StrTo(ctx.Query("repoid")).Int64()
	issueStats := models.GetUserIssueStats(ctx.User.Id, filterMode)

	// Get all repositories.
	repos, err := models.GetRepositories(ctx.User.Id, true)
	if err != nil {
		ctx.Handle(500, "user.Issues(GetRepositories)", err)
		return
	}

	repoIds := make([]int64, 0, len(repos))
	showRepos := make([]*models.Repository, 0, len(repos))
	for _, repo := range repos {
		if repo.NumIssues == 0 {
			continue
		}

		repoIds = append(repoIds, repo.Id)
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

	if repoId > 0 {
		repoIds = []int64{repoId}
	}

	page, _ := base.StrTo(ctx.Query("page")).Int()

	// Get all issues.
	var ius []*models.IssueUser
	switch viewType {
	case "assigned":
		fallthrough
	case "created_by":
		ius, err = models.GetIssueUserPairsByMode(ctx.User.Id, repoId, isShowClosed, page, filterMode)
	default:
		ius, err = models.GetIssueUserPairsByRepoIds(repoIds, isShowClosed, page)
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
				log.Warn("user.Issues(GetIssueById #%d): issue not exist", ius[i].IssueId)
				continue
			} else {
				ctx.Handle(500, fmt.Sprintf("user.Issues(GetIssueById #%d)", ius[i].IssueId), err)
				return
			}
		}

		issues[i].Repo, err = models.GetRepositoryById(issues[i].RepoId)
		if err != nil {
			if err == models.ErrRepoNotExist {
				log.Warn("user.Issues(GetRepositoryById #%d): repository not exist", issues[i].RepoId)
				continue
			} else {
				ctx.Handle(500, fmt.Sprintf("user.Issues(GetRepositoryById #%d)", issues[i].RepoId), err)
				return
			}
		}

		if err = issues[i].Repo.GetOwner(); err != nil {
			ctx.Handle(500, "user.Issues(GetOwner)", err)
			return
		}

		if err = issues[i].GetPoster(); err != nil {
			ctx.Handle(500, "user.Issues(GetUserById)", err)
			return
		}
	}

	ctx.Data["RepoId"] = repoId
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
	ctx.HTML(200, ISSUES)
}

func Pulls(ctx *middleware.Context) {
	ctx.HTML(200, PULLS)
}

func Stars(ctx *middleware.Context) {
	ctx.HTML(200, STARS)
}
