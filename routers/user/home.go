// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	DASHBOARD base.TplName = "user/dashboard/dashboard"
	ISSUES    base.TplName = "user/dashboard/issues"
	STARS     base.TplName = "user/stars"
	PROFILE   base.TplName = "user/profile"
	ORG_HOME  base.TplName = "org/home"
)

func getDashboardContextUser(ctx *middleware.Context) *models.User {
	ctxUser := ctx.User
	orgName := ctx.Params(":org")
	if len(orgName) > 0 {
		// Organization.
		org, err := models.GetUserByName(orgName)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.Handle(404, "GetUserByName", err)
			} else {
				ctx.Handle(500, "GetUserByName", err)
			}
			return nil
		}
		ctxUser = org
	}
	ctx.Data["ContextUser"] = ctxUser

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "GetOrganizations", err)
		return nil
	}
	ctx.Data["Orgs"] = ctx.User.Orgs

	return ctxUser
}

func retrieveFeeds(ctx *middleware.Context, uid, offset int64, isProfile bool) {
	actions, err := models.GetFeeds(uid, offset, isProfile)
	if err != nil {
		ctx.Handle(500, "GetFeeds", err)
		return
	}

	// Check access of private repositories.
	feeds := make([]*models.Action, 0, len(actions))
	unameAvatars := make(map[string]string)
	for _, act := range actions {
		// Cache results to reduce queries.
		_, ok := unameAvatars[act.ActUserName]
		if !ok {
			u, err := models.GetUserByName(act.ActUserName)
			if err != nil {
				if models.IsErrUserNotExist(err) {
					continue
				}
				ctx.Handle(500, "GetUserByName", err)
				return
			}
			unameAvatars[act.ActUserName] = u.AvatarLink()
		}

		act.ActAvatar = unameAvatars[act.ActUserName]
		feeds = append(feeds, act)
	}
	ctx.Data["Feeds"] = feeds
}

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("dashboard")
	ctx.Data["PageIsDashboard"] = true
	ctx.Data["PageIsNews"] = true

	ctxUser := getDashboardContextUser(ctx)
	if ctx.Written() {
		return
	}

	if !ctxUser.IsOrganization() {
		collaborateRepos, err := ctx.User.GetAccessibleRepositories()
		if err != nil {
			ctx.Handle(500, "GetAccessibleRepositories", err)
			return
		}

		for i := range collaborateRepos {
			if err = collaborateRepos[i].GetOwner(); err != nil {
				ctx.Handle(500, "GetOwner: "+collaborateRepos[i].Name, err)
				return
			}
		}
		ctx.Data["CollaborateCount"] = len(collaborateRepos)
		ctx.Data["CollaborativeRepos"] = collaborateRepos
	}

	repos, err := models.GetRepositories(ctxUser.Id, true)
	if err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}
	ctx.Data["Repos"] = repos

	// Get mirror repositories.
	mirrors := make([]*models.Repository, 0, 5)
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

	retrieveFeeds(ctx, ctx.User.Id, 0, false)
	if ctx.Written() {
		return
	}
	ctx.HTML(200, DASHBOARD)
}

func Issues(ctx *middleware.Context) {
	isPullList := ctx.Params(":type") == "pulls"
	if isPullList {
		ctx.Data["Title"] = ctx.Tr("pull_requests")
		ctx.Data["PageIsPulls"] = true
	} else {
		ctx.Data["Title"] = ctx.Tr("issues")
		ctx.Data["PageIsIssues"] = true
	}

	ctxUser := getDashboardContextUser(ctx)
	if ctx.Written() {
		return
	}

	// Organization does not have view type and filter mode.
	var (
		viewType   string
		sortType   = ctx.Query("sort")
		filterMode = models.FM_ALL
		assigneeID int64
		posterID   int64
	)
	if ctxUser.IsOrganization() {
		viewType = "all"
	} else {
		viewType = ctx.Query("type")
		types := []string{"assigned", "created_by"}
		if !com.IsSliceContainsStr(types, viewType) {
			viewType = "all"
		}

		switch viewType {
		case "assigned":
			filterMode = models.FM_ASSIGN
			assigneeID = ctxUser.Id
		case "created_by":
			filterMode = models.FM_CREATE
			posterID = ctxUser.Id
		}
	}

	repoID := ctx.QueryInt64("repo")
	isShowClosed := ctx.Query("state") == "closed"

	// Get repositories.
	repos, err := models.GetRepositories(ctxUser.Id, true)
	if err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}

	allCount := 0
	repoIDs := make([]int64, 0, len(repos))
	showRepos := make([]*models.Repository, 0, len(repos))
	for _, repo := range repos {
		if (isPullList && repo.NumPulls == 0) ||
			(!isPullList && repo.NumIssues == 0) {
			continue
		}

		repoIDs = append(repoIDs, repo.ID)

		if isPullList {
			allCount += repo.NumOpenPulls
			repo.NumOpenIssues = repo.NumOpenPulls
			repo.NumClosedIssues = repo.NumClosedPulls
		} else {
			allCount += repo.NumOpenIssues
		}

		if filterMode != models.FM_ALL {
			// Calculate repository issue count with filter mode.
			numOpen, numClosed := repo.IssueStats(ctxUser.Id, filterMode, isPullList)
			repo.NumOpenIssues, repo.NumClosedIssues = int(numOpen), int(numClosed)
		}

		if repo.ID == repoID ||
			(isShowClosed && repo.NumClosedIssues > 0) ||
			(!isShowClosed && repo.NumOpenIssues > 0) {
			showRepos = append(showRepos, repo)
		}
	}
	ctx.Data["Repos"] = showRepos

	issueStats := models.GetUserIssueStats(repoID, ctxUser.Id, repoIDs, filterMode, isPullList)
	issueStats.AllCount = int64(allCount)

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
	issues, err := models.Issues(&models.IssuesOptions{
		UserID:     ctxUser.Id,
		AssigneeID: assigneeID,
		RepoID:     repoID,
		PosterID:   posterID,
		RepoIDs:    repoIDs,
		Page:       page,
		IsClosed:   isShowClosed,
		IsPull:     isPullList,
		SortType:   sortType,
	})
	if err != nil {
		ctx.Handle(500, "Issues: %v", err)
		return
	}

	// Get posters and repository.
	for i := range issues {
		issues[i].Repo, err = models.GetRepositoryByID(issues[i].RepoID)
		if err != nil {
			ctx.Handle(500, "GetRepositoryByID", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if err = issues[i].Repo.GetOwner(); err != nil {
			ctx.Handle(500, "GetOwner", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if err = issues[i].GetPoster(); err != nil {
			ctx.Handle(500, "GetPoster", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}
	}
	ctx.Data["Issues"] = issues

	ctx.Data["IssueStats"] = issueStats
	ctx.Data["ViewType"] = viewType
	ctx.Data["SortType"] = sortType
	ctx.Data["RepoID"] = repoID
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
	} else {
		ctx.Data["State"] = "open"
	}

	ctx.HTML(200, ISSUES)
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
		buf.WriteString("\n")
	}
	ctx.PlainText(200, buf.Bytes())
}

func showOrgProfile(ctx *middleware.Context) {
	ctx.SetParams(":org", ctx.Params(":username"))
	middleware.HandleOrgAssignment(ctx)
	if ctx.Written() {
		return
	}

	org := ctx.Org.Organization
	ctx.Data["Title"] = org.FullName

	repos, err := models.GetRepositories(org.Id, ctx.IsSigned && org.IsOrgMember(ctx.User.Id))
	if err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}
	ctx.Data["Repos"] = repos

	if err = org.GetMembers(); err != nil {
		ctx.Handle(500, "GetMembers", err)
		return
	}
	ctx.Data["Members"] = org.Members

	if err = org.GetTeams(); err != nil {
		ctx.Handle(500, "GetTeams", err)
		return
	}
	ctx.Data["Teams"] = org.Teams

	ctx.HTML(200, ORG_HOME)
}

func Profile(ctx *middleware.Context) {
	ctx.Data["Title"] = "Profile"
	ctx.Data["PageIsUserProfile"] = true

	uname := ctx.Params(":username")
	// Special handle for FireFox requests favicon.ico.
	if uname == "favicon.ico" {
		ctx.Redirect(setting.AppSubUrl + "/img/favicon.png")
		return
	} else if strings.HasSuffix(uname, ".png") {
		ctx.Error(404)
		return
	}

	isShowKeys := false
	if strings.HasSuffix(uname, ".keys") {
		isShowKeys = true
		uname = strings.TrimSuffix(uname, ".keys")
	}

	u, err := models.GetUserByName(uname)
	if err != nil {
		if models.IsErrUserNotExist(err) {
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
		showOrgProfile(ctx)
		return
	}
	ctx.Data["Owner"] = u

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab
	switch tab {
	case "activity":
		retrieveFeeds(ctx, u.Id, 0, true)
		if ctx.Written() {
			return
		}
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
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByEmail", err)
		} else {
			ctx.Handle(500, "GetUserByEmail", err)
		}
		return
	}
	ctx.Redirect(setting.AppSubUrl + "/user/" + u.Name)
}
