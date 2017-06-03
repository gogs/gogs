// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"fmt"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/setting"
)

const (
	DASHBOARD = "user/dashboard/dashboard"
	NEWS_FEED = "user/dashboard/feeds"
	ISSUES    = "user/dashboard/issues"
	PROFILE   = "user/profile"
	ORG_HOME  = "org/home"
)

// getDashboardContextUser finds out dashboard is viewing as which context user.
func getDashboardContextUser(c *context.Context) *models.User {
	ctxUser := c.User
	orgName := c.Params(":org")
	if len(orgName) > 0 {
		// Organization.
		org, err := models.GetUserByName(orgName)
		if err != nil {
			c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
			return nil
		}
		ctxUser = org
	}
	c.Data["ContextUser"] = ctxUser

	if err := c.User.GetOrganizations(true); err != nil {
		c.Handle(500, "GetOrganizations", err)
		return nil
	}
	c.Data["Orgs"] = c.User.Orgs

	return ctxUser
}

// retrieveFeeds loads feeds from database by given context user.
// The user could be organization so it is not always the logged in user,
// which is why we have to explicitly pass the context user ID.
func retrieveFeeds(c *context.Context, ctxUser *models.User, userID int64, isProfile bool) {
	actions, err := models.GetFeeds(ctxUser, userID, c.QueryInt64("after_id"), isProfile)
	if err != nil {
		c.Handle(500, "GetFeeds", err)
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
				if errors.IsUserNotExist(err) {
					continue
				}
				c.Handle(500, "GetUserByName", err)
				return
			}
			unameAvatars[act.ActUserName] = u.RelAvatarLink()
		}

		act.ActAvatar = unameAvatars[act.ActUserName]
		feeds = append(feeds, act)
	}
	c.Data["Feeds"] = feeds
	if len(feeds) > 0 {
		afterID := feeds[len(feeds)-1].ID
		c.Data["AfterID"] = afterID
		c.Header().Set("X-AJAX-URL", fmt.Sprintf("%s?after_id=%d", c.Data["Link"], afterID))
	}
}

func Dashboard(c *context.Context) {
	ctxUser := getDashboardContextUser(c)
	if c.Written() {
		return
	}

	retrieveFeeds(c, ctxUser, c.User.ID, false)
	if c.Written() {
		return
	}

	if c.Req.Header.Get("X-AJAX") == "true" {
		c.HTML(200, NEWS_FEED)
		return
	}

	c.Data["Title"] = ctxUser.DisplayName() + " - " + c.Tr("dashboard")
	c.Data["PageIsDashboard"] = true
	c.Data["PageIsNews"] = true

	// Only user can have collaborative repositories.
	if !ctxUser.IsOrganization() {
		collaborateRepos, err := c.User.GetAccessibleRepositories(setting.UI.User.RepoPagingNum)
		if err != nil {
			c.Handle(500, "GetAccessibleRepositories", err)
			return
		} else if err = models.RepositoryList(collaborateRepos).LoadAttributes(); err != nil {
			c.Handle(500, "RepositoryList.LoadAttributes", err)
			return
		}
		c.Data["CollaborativeRepos"] = collaborateRepos
	}

	var err error
	var repos, mirrors []*models.Repository
	var repoCount int64
	if ctxUser.IsOrganization() {
		repos, repoCount, err = ctxUser.GetUserRepositories(c.User.ID, 1, setting.UI.User.RepoPagingNum)
		if err != nil {
			c.Handle(500, "GetUserRepositories", err)
			return
		}

		mirrors, err = ctxUser.GetUserMirrorRepositories(c.User.ID)
		if err != nil {
			c.Handle(500, "GetUserMirrorRepositories", err)
			return
		}
	} else {
		if err = ctxUser.GetRepositories(1, setting.UI.User.RepoPagingNum); err != nil {
			c.Handle(500, "GetRepositories", err)
			return
		}
		repos = ctxUser.Repos
		repoCount = int64(ctxUser.NumRepos)

		mirrors, err = ctxUser.GetMirrorRepositories()
		if err != nil {
			c.Handle(500, "GetMirrorRepositories", err)
			return
		}
	}
	c.Data["Repos"] = repos
	c.Data["RepoCount"] = repoCount
	c.Data["MaxShowRepoNum"] = setting.UI.User.RepoPagingNum

	if err := models.MirrorRepositoryList(mirrors).LoadAttributes(); err != nil {
		c.Handle(500, "MirrorRepositoryList.LoadAttributes", err)
		return
	}
	c.Data["MirrorCount"] = len(mirrors)
	c.Data["Mirrors"] = mirrors

	c.HTML(200, DASHBOARD)
}

func Issues(c *context.Context) {
	isPullList := c.Params(":type") == "pulls"
	if isPullList {
		c.Data["Title"] = c.Tr("pull_requests")
		c.Data["PageIsPulls"] = true
	} else {
		c.Data["Title"] = c.Tr("issues")
		c.Data["PageIsIssues"] = true
	}

	ctxUser := getDashboardContextUser(c)
	if c.Written() {
		return
	}

	var (
		sortType   = c.Query("sort")
		filterMode = models.FILTER_MODE_YOUR_REPOS
	)

	// Note: Organization does not have view type and filter mode.
	if !ctxUser.IsOrganization() {
		viewType := c.Query("type")
		types := []string{
			string(models.FILTER_MODE_YOUR_REPOS),
			string(models.FILTER_MODE_ASSIGN),
			string(models.FILTER_MODE_CREATE),
		}
		if !com.IsSliceContainsStr(types, viewType) {
			viewType = string(models.FILTER_MODE_YOUR_REPOS)
		}
		filterMode = models.FilterMode(viewType)
	}

	page := c.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	repoID := c.QueryInt64("repo")
	isShowClosed := c.Query("state") == "closed"

	// Get repositories.
	var (
		err         error
		repos       []*models.Repository
		userRepoIDs []int64
		showRepos   = make([]*models.Repository, 0, 10)
	)
	if ctxUser.IsOrganization() {
		repos, _, err = ctxUser.GetUserRepositories(c.User.ID, 1, ctxUser.NumRepos)
		if err != nil {
			c.Handle(500, "GetRepositories", err)
			return
		}
	} else {
		if err := ctxUser.GetRepositories(1, c.User.NumRepos); err != nil {
			c.Handle(500, "GetRepositories", err)
			return
		}
		repos = ctxUser.Repos
	}

	userRepoIDs = make([]int64, 0, len(repos))
	for _, repo := range repos {
		userRepoIDs = append(userRepoIDs, repo.ID)

		if filterMode != models.FILTER_MODE_YOUR_REPOS {
			continue
		}

		if isPullList {
			if isShowClosed && repo.NumClosedPulls == 0 ||
				!isShowClosed && repo.NumOpenPulls == 0 {
				continue
			}
		} else {
			if !repo.EnableIssues || repo.EnableExternalTracker ||
				isShowClosed && repo.NumClosedIssues == 0 ||
				!isShowClosed && repo.NumOpenIssues == 0 {
				continue
			}
		}

		showRepos = append(showRepos, repo)
	}

	// Filter repositories if the page shows issues.
	if !isPullList {
		userRepoIDs, err = models.FilterRepositoryWithIssues(userRepoIDs)
		if err != nil {
			c.Handle(500, "FilterRepositoryWithIssues", err)
			return
		}
	}

	issueOptions := &models.IssuesOptions{
		RepoID:   repoID,
		Page:     page,
		IsClosed: isShowClosed,
		IsPull:   isPullList,
		SortType: sortType,
	}
	switch filterMode {
	case models.FILTER_MODE_YOUR_REPOS:
		// Get all issues from repositories from this user.
		if userRepoIDs == nil {
			issueOptions.RepoIDs = []int64{-1}
		} else {
			issueOptions.RepoIDs = userRepoIDs
		}

	case models.FILTER_MODE_ASSIGN:
		// Get all issues assigned to this user.
		issueOptions.AssigneeID = ctxUser.ID

	case models.FILTER_MODE_CREATE:
		// Get all issues created by this user.
		issueOptions.PosterID = ctxUser.ID
	}

	issues, err := models.Issues(issueOptions)
	if err != nil {
		c.Handle(500, "Issues", err)
		return
	}

	if repoID > 0 {
		repo, err := models.GetRepositoryByID(repoID)
		if err != nil {
			c.Handle(500, "GetRepositoryByID", fmt.Errorf("[#%d] %v", repoID, err))
			return
		}

		if err = repo.GetOwner(); err != nil {
			c.Handle(500, "GetOwner", fmt.Errorf("[#%d] %v", repoID, err))
			return
		}

		// Check if user has access to given repository.
		if !repo.IsOwnedBy(ctxUser.ID) && !repo.HasAccess(ctxUser.ID) {
			c.Handle(404, "Issues", fmt.Errorf("#%d", repoID))
			return
		}
	}

	for _, issue := range issues {
		if err = issue.Repo.GetOwner(); err != nil {
			c.Handle(500, "GetOwner", fmt.Errorf("[#%d] %v", issue.RepoID, err))
			return
		}
	}

	issueStats := models.GetUserIssueStats(repoID, ctxUser.ID, userRepoIDs, filterMode, isPullList)

	var total int
	if !isShowClosed {
		total = int(issueStats.OpenCount)
	} else {
		total = int(issueStats.ClosedCount)
	}

	c.Data["Issues"] = issues
	c.Data["Repos"] = showRepos
	c.Data["Page"] = paginater.New(total, setting.UI.IssuePagingNum, page, 5)
	c.Data["IssueStats"] = issueStats
	c.Data["ViewType"] = string(filterMode)
	c.Data["SortType"] = sortType
	c.Data["RepoID"] = repoID
	c.Data["IsShowClosed"] = isShowClosed

	if isShowClosed {
		c.Data["State"] = "closed"
	} else {
		c.Data["State"] = "open"
	}

	c.HTML(200, ISSUES)
}

func ShowSSHKeys(c *context.Context, uid int64) {
	keys, err := models.ListPublicKeys(uid)
	if err != nil {
		c.Handle(500, "ListPublicKeys", err)
		return
	}

	var buf bytes.Buffer
	for i := range keys {
		buf.WriteString(keys[i].OmitEmail())
		buf.WriteString("\n")
	}
	c.PlainText(200, buf.Bytes())
}

func showOrgProfile(c *context.Context) {
	c.SetParams(":org", c.Params(":username"))
	context.HandleOrgAssignment(c)
	if c.Written() {
		return
	}

	org := c.Org.Organization
	c.Data["Title"] = org.FullName

	page := c.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	var (
		repos []*models.Repository
		count int64
		err   error
	)
	if c.IsLogged && !c.User.IsAdmin {
		repos, count, err = org.GetUserRepositories(c.User.ID, page, setting.UI.User.RepoPagingNum)
		if err != nil {
			c.Handle(500, "GetUserRepositories", err)
			return
		}
		c.Data["Repos"] = repos
	} else {
		showPrivate := c.IsLogged && c.User.IsAdmin
		repos, err = models.GetUserRepositories(&models.UserRepoOptions{
			UserID:   org.ID,
			Private:  showPrivate,
			Page:     page,
			PageSize: setting.UI.User.RepoPagingNum,
		})
		if err != nil {
			c.Handle(500, "GetRepositories", err)
			return
		}
		c.Data["Repos"] = repos
		count = models.CountUserRepositories(org.ID, showPrivate)
	}
	c.Data["Page"] = paginater.New(int(count), setting.UI.User.RepoPagingNum, page, 5)

	if err := org.GetMembers(); err != nil {
		c.Handle(500, "GetMembers", err)
		return
	}
	c.Data["Members"] = org.Members

	c.Data["Teams"] = org.Teams

	c.HTML(200, ORG_HOME)
}

func Email2User(c *context.Context) {
	u, err := models.GetUserByEmail(c.Query("email"))
	if err != nil {
		c.NotFoundOrServerError("GetUserByEmail", errors.IsUserNotExist, err)
		return
	}
	c.Redirect(setting.AppSubURL + "/user/" + u.Name)
}
