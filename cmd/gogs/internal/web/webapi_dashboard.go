package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
)

// requireSignIn turns away the same viewers as the sign-in requirement of the
// server-rendered pages: anonymous visitors, users prohibited from signing in,
// and users who have not confirmed their email when that is required.
func requireSignIn(c flamego.Context, user *database.User) {
	w := c.ResponseWriter()
	switch {
	case user == nil:
		writeErrorResponse(w, http.StatusUnauthorized, errors.New("authentication required"))
	case user.ProhibitLogin:
		writeErrorResponse(w, http.StatusForbidden, errors.New("login prohibited"))
	case !user.IsActive && conf.Auth.RequireEmailConfirmation:
		writeErrorResponse(w, http.StatusForbidden, errors.New("account not activated"))
	}
}

// dashboardOwner is the user or organization whose dashboard is requested.
type dashboardOwner struct {
	*database.User
}

// withDashboardOwner maps the organization named by the `org` path parameter
// as *dashboardOwner, or the signed-in user when there is none. Like the
// server-rendered dashboard, only members see the dashboard of an
// organization.
func withDashboardOwner(c flamego.Context, user *database.User) {
	name := c.Param("org")
	if name == "" {
		c.Map(&dashboardOwner{user})
		return
	}

	w := c.ResponseWriter()
	org, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil && !database.IsErrUserNotExist(err) {
		log.Error("withDashboardOwner: get user by username %q: %v", name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get organization"))
		return
	}
	if err != nil || !org.IsOrganization() || !(user.IsAdmin || org.IsOwnedBy(user.ID) || org.IsOrgMember(user.ID)) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("organization does not exist"))
		return
	}
	c.Map(&dashboardOwner{org})
}

type dashboardRepo struct {
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"isPrivate"`
	IsFork    bool   `json:"isFork"`
	IsMirror  bool   `json:"isMirror"`
	StarCount int    `json:"starCount"`
}

type dashboardOrg struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	RepoCount int    `json:"repoCount"`
}

type dashboardMirror struct {
	Name      string `json:"name"`
	IsPrivate bool   `json:"isPrivate"`
	// IntervalHours is how often the mirror syncs.
	IntervalHours int `json:"intervalHours"`
}

// dashboardContext is the dashboard owner, and the organizations of the
// signed-in user to switch the dashboard to, whichever dashboard is requested.
type dashboardContext struct {
	// Kind is "user" or "organization", the kind of the dashboard owner.
	Kind          string         `json:"kind"`
	ID            int64          `json:"id"`
	Name          string         `json:"name"`
	AvatarURL     string         `json:"avatarURL"`
	Organizations []dashboardOrg `json:"organizations"`
}

func newDashboardContext(ctx context.Context, user *database.User, owner *dashboardOwner) (*dashboardContext, error) {
	orgs, err := database.Handle.Organizations().List(ctx, database.ListOrgsOptions{
		MemberID:              user.ID,
		IncludePrivateMembers: true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "list organizations")
	}
	dc := &dashboardContext{
		Kind:          "user",
		ID:            owner.ID,
		Name:          owner.Name,
		AvatarURL:     owner.AvatarURLPath(),
		Organizations: make([]dashboardOrg, 0, len(orgs)),
	}
	if owner.IsOrganization() {
		dc.Kind = "organization"
	}
	for _, org := range orgs {
		dc.Organizations = append(dc.Organizations, dashboardOrg{
			Name:      org.Name,
			AvatarURL: org.AvatarURLPath(),
			RepoCount: org.NumRepos,
		})
	}
	return dc, nil
}

type dashboard struct {
	*dashboardContext
	Repos     []dashboardRepo `json:"repos"`
	RepoCount int64           `json:"repoCount"`
	// HasMoreRepos is true when the owner has more repositories than listed.
	HasMoreRepos bool `json:"hasMoreRepos"`
	// CollaborativeRepos are only listed on the dashboard of the signed-in
	// user.
	CollaborativeRepos []dashboardRepo   `json:"collaborativeRepos,omitempty"`
	Mirrors            []dashboardMirror `json:"mirrors"`
}

func toDashboardRepos(repos []*database.Repository) []dashboardRepo {
	items := make([]dashboardRepo, 0, len(repos))
	for _, repo := range repos {
		items = append(items, dashboardRepo{
			Owner:     repo.Owner.Name,
			Name:      repo.Name,
			IsPrivate: repo.IsPrivate,
			IsFork:    repo.IsFork,
			IsMirror:  repo.IsMirror,
			StarCount: repo.NumStars,
		})
	}
	return items
}

func getDashboard(c flamego.Context, user *database.User, owner *dashboardOwner) (statusCode int, resp *dashboard, err error) {
	ctx := c.Request().Context()
	dc, err := newDashboardContext(ctx, user, owner)
	if err != nil {
		log.Error("getDashboard: get context of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get context")
	}

	var (
		repos, mirrors []*database.Repository
		repoCount      int64
	)
	if owner.IsOrganization() {
		repos, repoCount, err = owner.GetUserRepositories(user.ID, 1, conf.UI.User.RepoPagingNum)
		if err == nil {
			mirrors, err = owner.GetUserMirrorRepositories(user.ID)
		}
	} else {
		repos, err = database.GetUserRepositories(&database.UserRepoOptions{
			UserID:   owner.ID,
			Private:  true,
			Page:     1,
			PageSize: conf.UI.User.RepoPagingNum,
		})
		repoCount = int64(owner.NumRepos)
		if err == nil {
			mirrors, err = database.GetUserMirrorRepositories(owner.ID)
		}
	}
	if err != nil {
		log.Error("getDashboard: get repositories of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repositories")
	}
	for _, repo := range repos {
		repo.Owner = owner.User
	}

	resp = &dashboard{
		dashboardContext: dc,
		Repos:            toDashboardRepos(repos),
		RepoCount:        repoCount,
		HasMoreRepos:     owner.NumRepos > conf.UI.User.RepoPagingNum,
		Mirrors:          make([]dashboardMirror, 0, len(mirrors)),
	}
	if err = database.MirrorRepositoryList(mirrors).LoadAttributes(); err != nil {
		log.Error("getDashboard: load attributes of mirrors of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load mirror attributes")
	}
	for _, repo := range mirrors {
		mirror := dashboardMirror{Name: repo.Name, IsPrivate: repo.IsPrivate}
		if repo.Mirror != nil {
			mirror.IntervalHours = repo.Interval
		}
		resp.Mirrors = append(resp.Mirrors, mirror)
	}

	if owner.IsOrganization() {
		return http.StatusOK, resp, nil
	}
	collaborativeRepos, err := database.Handle.Repositories().GetByCollaboratorID(ctx, user.ID, conf.UI.User.RepoPagingNum, "updated_unix DESC")
	if err == nil {
		err = database.RepositoryList(collaborativeRepos).LoadAttributes()
	}
	if err != nil {
		log.Error("getDashboard: get collaborative repositories of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get collaborative repositories")
	}
	resp.CollaborativeRepos = toDashboardRepos(collaborativeRepos)
	return http.StatusOK, resp, nil
}

func getDashboardActivity(c flamego.Context, l i18n.Locale, user *database.User, owner *dashboardOwner) (statusCode int, resp *activityFeed, err error) {
	ctx := c.Request().Context()
	afterID, _ := strconv.ParseInt(c.Query("afterID"), 10, 64)
	var actions []*database.Action
	if owner.IsOrganization() {
		actions, err = database.Handle.Actions().ListByOrganization(ctx, owner.ID, user.ID, afterID)
	} else {
		actions, err = database.Handle.Actions().ListByUser(ctx, owner.ID, user.ID, afterID, false)
	}
	if err != nil {
		log.Error("getDashboardActivity: list actions of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list actions")
	}
	feed, err := toActivityFeed(ctx, l, actions)
	if err != nil {
		log.Error("getDashboardActivity: convert actions of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "convert actions")
	}
	return http.StatusOK, feed, nil
}

type dashboardIssueLabel struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	TextColor string `json:"textColor"`
}

type dashboardIssueUser struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
}

type dashboardIssue struct {
	// Repo is the full name of the repository, e.g., "owner/name".
	Repo         string                `json:"repo"`
	Index        int64                 `json:"index"`
	Title        string                `json:"title"`
	IsClosed     bool                  `json:"isClosed"`
	CommentCount int                   `json:"commentCount"`
	Labels       []dashboardIssueLabel `json:"labels"`
	Poster       dashboardIssueUser    `json:"poster"`
	Assignee     *dashboardIssueUser   `json:"assignee,omitempty"`
	CreatedAt    time.Time             `json:"createdAt"`
}

type dashboardIssueRepo struct {
	ID int64 `json:"id"`
	// FullName is the full name of the repository, e.g., "owner/name".
	FullName string `json:"fullName"`
	// Count is the number of issues or pull requests in the requested state.
	Count int `json:"count"`
}

type dashboardIssueCounts struct {
	YourRepos int64 `json:"yourRepos"`
	Assigned  int64 `json:"assigned"`
	Created   int64 `json:"created"`
	Open      int64 `json:"open"`
	Closed    int64 `json:"closed"`
}

type dashboardIssues struct {
	*dashboardContext
	*listPage[dashboardIssue]
	// Type is the filter in effect, one of "your_repositories", "assigned" and
	// "created_by". Organizations only have "your_repositories".
	Type   string               `json:"type"`
	Counts dashboardIssueCounts `json:"counts"`
	// Repos are the repositories of the owner that have issues or pull requests
	// in the requested state, listed for the "your_repositories" filter only.
	Repos []dashboardIssueRepo `json:"repos"`
}

func getDashboardIssues(c flamego.Context, user *database.User, owner *dashboardOwner) (statusCode int, resp *dashboardIssues, err error) {
	return listDashboardIssues(c, user, owner, false)
}

func getDashboardPulls(c flamego.Context, user *database.User, owner *dashboardOwner) (statusCode int, resp *dashboardIssues, err error) {
	return listDashboardIssues(c, user, owner, true)
}

// listDashboardIssues lists the issues or pull requests of the dashboard owner,
// filtered and sorted like the server-rendered page by the `type`, `repo`,
// `state`, `sort` and `page` query parameters.
func listDashboardIssues(c flamego.Context, user *database.User, owner *dashboardOwner, isPull bool) (statusCode int, resp *dashboardIssues, err error) {
	filterMode := database.FilterModeYourRepos
	if !owner.IsOrganization() {
		switch mode := database.FilterMode(c.Query("type")); mode {
		case database.FilterModeAssign, database.FilterModeCreate:
			filterMode = mode
		}
	}
	isClosed := c.Query("state") == "closed"
	page := pageParam(c)

	repoID, _ := strconv.ParseInt(c.Query("repo"), 10, 64)
	if repoID > 0 {
		repo, err := database.GetRepositoryByID(repoID)
		if err != nil && !database.IsErrRepoNotExist(err) {
			log.Error("listDashboardIssues: get repository %d: %v", repoID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
		}
		if err != nil || (!repo.IsOwnedBy(owner.ID) && !repo.HasAccess(owner.ID)) {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
	}

	var repos []*database.Repository
	if owner.IsOrganization() {
		repos, _, err = owner.GetUserRepositories(user.ID, 1, owner.NumRepos)
	} else {
		repos, err = database.GetUserRepositories(&database.UserRepoOptions{
			UserID:   owner.ID,
			Private:  true,
			Page:     1,
			PageSize: owner.NumRepos,
		})
	}
	if err != nil {
		log.Error("listDashboardIssues: get repositories of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repositories")
	}

	dc, err := newDashboardContext(c.Request().Context(), user, owner)
	if err != nil {
		log.Error("listDashboardIssues: get context of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get context")
	}
	resp = &dashboardIssues{
		dashboardContext: dc,
		Type:             string(filterMode),
		Repos:            []dashboardIssueRepo{},
	}
	repoIDs := make([]int64, 0, len(repos))
	for _, repo := range repos {
		repoIDs = append(repoIDs, repo.ID)
		if filterMode != database.FilterModeYourRepos {
			continue
		}

		var count int
		switch {
		case isPull && isClosed:
			count = repo.NumClosedPulls
		case isPull:
			count = repo.NumOpenPulls
		case !repo.EnableIssues || repo.EnableExternalTracker:
		case isClosed:
			count = repo.NumClosedIssues
		default:
			count = repo.NumOpenIssues
		}
		if count > 0 {
			resp.Repos = append(resp.Repos, dashboardIssueRepo{
				ID:       repo.ID,
				FullName: owner.Name + "/" + repo.Name,
				Count:    count,
			})
		}
	}
	if !isPull {
		repoIDs, err = database.FilterRepositoryWithIssues(repoIDs)
		if err != nil {
			log.Error("listDashboardIssues: filter repositories with issues: %v", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "filter repositories")
		}
	}

	opts := &database.IssuesOptions{
		RepoID:   repoID,
		Page:     page,
		IsClosed: isClosed,
		IsPull:   isPull,
		SortType: c.Query("sort"),
	}
	switch filterMode {
	case database.FilterModeYourRepos:
		opts.RepoIDs = repoIDs
		if opts.RepoIDs == nil {
			// Match no repositories rather than every one.
			opts.RepoIDs = []int64{-1}
		}
	case database.FilterModeAssign:
		opts.AssigneeID = owner.ID
	case database.FilterModeCreate:
		opts.PosterID = owner.ID
	}
	issues, err := database.Issues(opts)
	if err != nil {
		log.Error("listDashboardIssues: list issues of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list issues")
	}

	items := make([]dashboardIssue, 0, len(issues))
	for _, issue := range issues {
		item := dashboardIssue{
			Repo:         issue.Repo.FullName(),
			Index:        issue.Index,
			Title:        issue.Title,
			IsClosed:     issue.IsClosed,
			CommentCount: issue.NumComments,
			Labels:       make([]dashboardIssueLabel, 0, len(issue.Labels)),
			Poster:       dashboardIssueUser{Name: issue.Poster.Name, AvatarURL: issue.Poster.AvatarURLPath()},
			CreatedAt:    issue.Created.UTC(),
		}
		for _, label := range issue.Labels {
			item.Labels = append(item.Labels, toIssueLabel(label))
		}
		if issue.Assignee != nil {
			item.Assignee = &dashboardIssueUser{Name: issue.Assignee.Name, AvatarURL: issue.Assignee.AvatarURLPath()}
		}
		items = append(items, item)
	}

	stats := database.GetUserIssueStats(repoID, owner.ID, repoIDs, filterMode, isPull)
	resp.Counts = dashboardIssueCounts{
		YourRepos: stats.YourReposCount,
		Assigned:  stats.AssignCount,
		Created:   stats.CreateCount,
		Open:      stats.OpenCount,
		Closed:    stats.ClosedCount,
	}
	total := stats.OpenCount
	if isClosed {
		total = stats.ClosedCount
	}
	resp.listPage = newListPage(items, page, total, conf.UI.IssuePagingNum)
	return http.StatusOK, resp, nil
}
