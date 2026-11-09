package web

import (
	"net/http"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
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

type dashboard struct {
	// Kind is "user" or "organization", the kind of the dashboard owner.
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	// Organizations are those of the signed-in user, to switch the dashboard
	// to, whichever dashboard is requested.
	Organizations []dashboardOrg  `json:"organizations"`
	Repos         []dashboardRepo `json:"repos"`
	RepoCount     int64           `json:"repoCount"`
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
	orgs, err := database.Handle.Organizations().List(ctx, database.ListOrgsOptions{
		MemberID:              user.ID,
		IncludePrivateMembers: true,
	})
	if err != nil {
		log.Error("getDashboard: list organizations of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list organizations")
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
		Kind:          "user",
		ID:            owner.ID,
		Name:          owner.Name,
		AvatarURL:     owner.AvatarURLPath(),
		Organizations: make([]dashboardOrg, 0, len(orgs)),
		Repos:         toDashboardRepos(repos),
		RepoCount:     repoCount,
		HasMoreRepos:  owner.NumRepos > conf.UI.User.RepoPagingNum,
		Mirrors:       make([]dashboardMirror, 0, len(mirrors)),
	}
	for _, org := range orgs {
		resp.Organizations = append(resp.Organizations, dashboardOrg{
			Name:      org.Name,
			AvatarURL: org.AvatarURLPath(),
			RepoCount: org.NumRepos,
		})
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
		resp.Kind = "organization"
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
