package user

import (
	"bytes"
	"net/http"
	"slices"

	"github.com/unknwon/paginater"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

const (
	tmplUserDashboardIssues = "user/dashboard/issues"
)

// getDashboardContextUser finds out dashboard is viewing as which context user.
func getDashboardContextUser(c *context.Context) *database.User {
	ctxUser := c.User
	orgName := c.Params(":org")
	if len(orgName) > 0 {
		// Organization.
		org, err := database.Handle.Users().GetByUsername(c.Req.Context(), orgName)
		if err != nil {
			c.NotFoundOrError(err, "get user by name")
			return nil
		}
		ctxUser = org
	}
	c.Data["ContextUser"] = ctxUser

	orgs, err := database.Handle.Organizations().List(
		c.Req.Context(),
		database.ListOrgsOptions{
			MemberID:              c.User.ID,
			IncludePrivateMembers: true,
		},
	)
	if err != nil {
		c.Error(err, "list organizations")
		return nil
	}
	c.Data["Orgs"] = orgs

	return ctxUser
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
		filterMode = database.FilterModeYourRepos
	)

	// Note: Organization does not have view type and filter mode.
	if !ctxUser.IsOrganization() {
		viewType := c.Query("type")
		types := []string{
			string(database.FilterModeYourRepos),
			string(database.FilterModeAssign),
			string(database.FilterModeCreate),
		}
		if !slices.Contains(types, viewType) {
			viewType = string(database.FilterModeYourRepos)
		}
		filterMode = database.FilterMode(viewType)
	}

	page := max(c.QueryInt("page"), 1)

	repoID := c.QueryInt64("repo")
	isShowClosed := c.Query("state") == "closed"

	// Get repositories.
	var (
		err         error
		repos       []*database.Repository
		userRepoIDs []int64
		showRepos   = make([]*database.Repository, 0, 10)
	)
	if ctxUser.IsOrganization() {
		repos, _, err = ctxUser.GetUserRepositories(c.User.ID, 1, ctxUser.NumRepos)
		if err != nil {
			c.Error(err, "get repositories")
			return
		}
	} else {
		repos, err = database.GetUserRepositories(
			&database.UserRepoOptions{
				UserID:   ctxUser.ID,
				Private:  true,
				Page:     1,
				PageSize: ctxUser.NumRepos,
			},
		)
		if err != nil {
			c.Error(err, "get repositories")
			return
		}
	}

	userRepoIDs = make([]int64, 0, len(repos))
	for _, repo := range repos {
		userRepoIDs = append(userRepoIDs, repo.ID)

		if filterMode != database.FilterModeYourRepos {
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
		userRepoIDs, err = database.FilterRepositoryWithIssues(userRepoIDs)
		if err != nil {
			c.Error(err, "filter repositories with issues")
			return
		}
	}

	issueOptions := &database.IssuesOptions{
		RepoID:   repoID,
		Page:     page,
		IsClosed: isShowClosed,
		IsPull:   isPullList,
		SortType: sortType,
	}
	switch filterMode {
	case database.FilterModeYourRepos:
		// Get all issues from repositories from this user.
		if userRepoIDs == nil {
			issueOptions.RepoIDs = []int64{-1}
		} else {
			issueOptions.RepoIDs = userRepoIDs
		}

	case database.FilterModeAssign:
		// Get all issues assigned to this user.
		issueOptions.AssigneeID = ctxUser.ID

	case database.FilterModeCreate:
		// Get all issues created by this user.
		issueOptions.PosterID = ctxUser.ID
	}

	issues, err := database.Issues(issueOptions)
	if err != nil {
		c.Error(err, "list issues")
		return
	}

	if repoID > 0 {
		repo, err := database.GetRepositoryByID(repoID)
		if err != nil {
			c.Error(err, "get repository by ID")
			return
		}

		if err = repo.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}

		// Check if user has access to given repository.
		if !repo.IsOwnedBy(ctxUser.ID) && !repo.HasAccess(ctxUser.ID) {
			c.NotFound()
			return
		}
	}

	for _, issue := range issues {
		if err = issue.Repo.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}
	}

	issueStats := database.GetUserIssueStats(repoID, ctxUser.ID, userRepoIDs, filterMode, isPullList)

	var total int
	if !isShowClosed {
		total = int(issueStats.OpenCount)
	} else {
		total = int(issueStats.ClosedCount)
	}

	c.Data["Issues"] = issues
	c.Data["Repos"] = showRepos
	c.Data["Page"] = paginater.New(total, conf.UI.IssuePagingNum, page, 5)
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

	c.Success(tmplUserDashboardIssues)
}

func ShowSSHKeys(c *context.Context, uid int64) {
	keys, err := database.ListPublicKeys(uid)
	if err != nil {
		c.Error(err, "list public keys")
		return
	}

	var buf bytes.Buffer
	for i := range keys {
		buf.WriteString(keys[i].OmitEmail())
		buf.WriteString("\n")
	}
	c.PlainText(http.StatusOK, buf.String())
}

func Email2User(c *context.Context) {
	u, err := database.Handle.Users().GetByEmail(c.Req.Context(), c.Query("email"))
	if err != nil {
		c.NotFoundOrError(err, "get user by email")
		return
	}
	c.Redirect(conf.Server.Subpath + "/user/" + u.Name)
}
