// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	DASHBOARD base.TplName = "user/dashboard/dashboard"
	PULLS     base.TplName = "user/dashboard/pulls"
	ISSUES    base.TplName = "user/issues"
	STARS     base.TplName = "user/stars"
	PROFILE   base.TplName = "user/profile"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("dashboard")
	ctx.Data["PageIsDashboard"] = true
	ctx.Data["PageIsNews"] = true

	var ctxUser *models.User
	// Check context type.
	orgName := ctx.Params(":org")
	if len(orgName) > 0 {
		// Organization.
		org, err := models.GetUserByName(orgName)
		if err != nil {
			if err == models.ErrUserNotExist {
				ctx.Handle(404, "GetUserByName", err)
			} else {
				ctx.Handle(500, "GetUserByName", err)
			}
			return
		}
		ctxUser = org
	} else {
		// Normal user.
		ctxUser = ctx.User
		collaborates, err := models.GetCollaborativeRepos(ctxUser.Name)
		if err != nil {
			ctx.Handle(500, "GetCollaborativeRepos", err)
			return
		}
		ctx.Data["CollaborateCount"] = len(collaborates)
		ctx.Data["CollaborativeRepos"] = collaborates
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	repos, err := models.GetRepositories(ctxUser.Id, true)
	if err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}
	ctx.Data["Repos"] = repos

	// Get mirror repositories.
	mirrors := make([]*models.Repository, 0, len(repos)/2)
	for _, repo := range repos {
		if repo.IsMirror {
			if err = repo.GetMirror(); err != nil {
				ctx.Handle(500, "GetMirror: "+repo.Name, err)
				return
			}
			mirrors = append(mirrors, repo)
		}
	}
	ctx.Data["MirrorCount"] = len(mirrors)
	ctx.Data["Mirrors"] = mirrors

	// Get feeds.
	actions, err := models.GetFeeds(ctxUser.Id, 0, false)
	if err != nil {
		ctx.Handle(500, "GetFeeds", err)
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
		// FIXME: cache results?
		u, err := models.GetUserByName(act.ActUserName)
		if err != nil {
			if err == models.ErrUserNotExist {
				continue
			}
			ctx.Handle(500, "GetUserByName", err)
			return
		}
		act.ActAvatar = u.AvatarLink()
		feeds = append(feeds, act)
	}
	ctx.Data["Feeds"] = feeds
	ctx.HTML(200, DASHBOARD)
}

func Pulls(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("pull_requests")
	ctx.Data["PageIsDashboard"] = true
	ctx.Data["PageIsPulls"] = true

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return
	}
	ctx.Data["ContextUser"] = ctx.User

	ctx.HTML(200, PULLS)
}

func ShowSSHKeys(ctx *middleware.Context, uid int64) {
	keys, err := models.ListPublicKeys(uid)
	if err != nil {
		ctx.Handle(500, "ListPublicKeys", err)
		return
	}

	var buf bytes.Buffer
	for i := range keys {
		buf.WriteString(keys[i].OmitEmail())
	}
	ctx.RenderData(200, buf.Bytes())
}

func Profile(ctx *middleware.Context) {
	ctx.Data["Title"] = "Profile"
	ctx.Data["PageIsUserProfile"] = true

	uname := ctx.Params(":username")
	// Special handle for FireFox requests favicon.ico.
	if uname == "favicon.ico" {
		ctx.Redirect(setting.AppSubUrl + "/img/favicon.png")
		return
	}

	isShowKeys := false
	if strings.HasSuffix(uname, ".keys") {
		isShowKeys = true
		uname = strings.TrimSuffix(uname, ".keys")
	}

	u, err := models.GetUserByName(uname)
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "GetUserByName", err)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return
	}

	// Show SSH keys.
	if isShowKeys {
		ShowSSHKeys(ctx, u.Id)
		return
	}

	if u.IsOrganization() {
		ctx.Redirect(setting.AppSubUrl + "/org/" + u.Name)
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
		actions, err := models.GetFeeds(u.Id, 0, false)
		if err != nil {
			ctx.Handle(500, "GetFeeds", err)
			return
		}
		feeds := make([]*models.Action, 0, len(actions))
		for _, act := range actions {
			if act.IsPrivate {
				if !ctx.IsSigned {
					continue
				}
				if has, _ := models.HasAccess(ctx.User.Name, act.RepoUserName+"/"+act.RepoName,
					models.READABLE); !has {
					continue
				}
			}
			// FIXME: cache results?
			u, err := models.GetUserByName(act.ActUserName)
			if err != nil {
				if err == models.ErrUserNotExist {
					continue
				}
				ctx.Handle(500, "GetUserByName", err)
				return
			}
			act.ActAvatar = u.AvatarLink()
			feeds = append(feeds, act)
		}
		ctx.Data["Feeds"] = feeds
	default:
		ctx.Data["Repos"], err = models.GetRepositories(u.Id, ctx.IsSigned && ctx.User.Id == u.Id)
		if err != nil {
			ctx.Handle(500, "GetRepositories", err)
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
	ctx.Redirect(setting.AppSubUrl + "/user/" + u.Name)
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

	repoId, _ := com.StrTo(ctx.Query("repoid")).Int64()
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

	page, _ := com.StrTo(ctx.Query("page")).Int()

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
