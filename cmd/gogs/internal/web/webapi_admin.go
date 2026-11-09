package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
)

// requireAdmin turns away users who are not site admins. Mount it after
// requireSignIn.
func requireAdmin(c flamego.Context, user *database.User) {
	if !user.IsAdmin {
		writeErrorResponse(c.ResponseWriter(), http.StatusForbidden, errors.New("admin required"))
	}
}

// adminListPage is one page of a list in the admin panel, with the number of
// all matches.
type adminListPage[T any] struct {
	*listPage[T]
	Total int64 `json:"total"`
}

func newAdminListPage[T any](items []T, page int, total int64, pageSize int) *adminListPage[T] {
	return &adminListPage[T]{listPage: newListPage(items, page, total, pageSize), Total: total}
}

// unixTime returns the time of a Unix timestamp column, which is read directly
// because users and repositories loaded through xorm leave the parsed time
// unset.
func unixTime(unix int64) time.Time {
	return time.Unix(unix, 0).UTC()
}

// searchAdminUsers lists users or organizations by ID, or searches them by
// name when the keyword is not empty, like the server-rendered admin pages.
func searchAdminUsers(ctx context.Context, typ database.UserType, keyword string, page, pageSize int) ([]*database.User, int64, error) {
	if keyword != "" {
		if typ == database.UserTypeOrganization {
			return database.Handle.Organizations().SearchByName(ctx, keyword, page, pageSize, "id ASC")
		}
		return database.Handle.Users().SearchByName(ctx, keyword, page, pageSize, "id ASC")
	}

	if typ == database.UserTypeOrganization {
		orgs, err := database.Organizations(page, pageSize)
		return orgs, database.CountOrganizations(), err
	}
	users, err := database.Handle.Users().List(ctx, page, pageSize)
	return users, database.Handle.Users().Count(ctx), err
}

type adminUser struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatarURL"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"isActive"`
	IsAdmin   bool      `json:"isAdmin"`
	RepoCount int       `json:"repoCount"`
	CreatedAt time.Time `json:"createdAt"`
}

func getAdminUsers(c flamego.Context) (statusCode int, resp *adminListPage[adminUser], err error) {
	page, pageSize := pageParam(c), conf.UI.Admin.UserPagingNum
	users, total, err := searchAdminUsers(c.Request().Context(), database.UserTypeIndividual, c.Query("q"), page, pageSize)
	if err != nil {
		log.Error("getAdminUsers: search users: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search users")
	}

	items := make([]adminUser, 0, len(users))
	for _, u := range users {
		items = append(items, adminUser{
			ID:        u.ID,
			Name:      u.Name,
			AvatarURL: u.AvatarURLPath(),
			Email:     u.Email,
			IsActive:  u.IsActive,
			IsAdmin:   u.IsAdmin,
			RepoCount: u.NumRepos,
			CreatedAt: unixTime(u.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

type adminOrg struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatarURL"`
	TeamCount   int       `json:"teamCount"`
	MemberCount int       `json:"memberCount"`
	RepoCount   int       `json:"repoCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

func getAdminOrgs(c flamego.Context) (statusCode int, resp *adminListPage[adminOrg], err error) {
	page, pageSize := pageParam(c), conf.UI.Admin.OrgPagingNum
	orgs, total, err := searchAdminUsers(c.Request().Context(), database.UserTypeOrganization, c.Query("q"), page, pageSize)
	if err != nil {
		log.Error("getAdminOrgs: search organizations: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search organizations")
	}

	items := make([]adminOrg, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, adminOrg{
			ID:          org.ID,
			Name:        org.Name,
			AvatarURL:   org.AvatarURLPath(),
			TeamCount:   org.NumTeams,
			MemberCount: org.NumMembers,
			RepoCount:   org.NumRepos,
			CreatedAt:   unixTime(org.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

type adminRepo struct {
	ID         int64  `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"isPrivate"`
	WatchCount int    `json:"watchCount"`
	StarCount  int    `json:"starCount"`
	IssueCount int    `json:"issueCount"`
	// Size is the size of the repository on disk, formatted like "1.2 MB".
	Size      string    `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

func getAdminRepos(c flamego.Context) (statusCode int, resp *adminListPage[adminRepo], err error) {
	page, pageSize := pageParam(c), conf.UI.Admin.RepoPagingNum
	var (
		repos []*database.Repository
		total int64
	)
	if keyword := c.Query("q"); keyword == "" {
		repos, err = database.Repositories(page, pageSize)
		total = database.CountRepositories(true)
	} else {
		repos, total, err = database.SearchRepositoryByName(&database.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  "id ASC",
			Private:  true,
			Page:     page,
			PageSize: pageSize,
		})
	}
	if err != nil {
		log.Error("getAdminRepos: list repositories: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}
	if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
		log.Error("getAdminRepos: load repository attributes: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load repository attributes")
	}

	items := make([]adminRepo, 0, len(repos))
	for _, repo := range repos {
		items = append(items, adminRepo{
			ID:         repo.ID,
			Owner:      repo.Owner.Name,
			Name:       repo.Name,
			IsPrivate:  repo.IsPrivate,
			WatchCount: repo.NumWatches,
			StarCount:  repo.NumStars,
			IssueCount: repo.NumIssues,
			Size:       tool.FileSize(repo.Size),
			CreatedAt:  unixTime(repo.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

func deleteAdminRepo(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	repo, err := database.GetRepositoryByID(id)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository not found")
		}
		log.Error("deleteAdminRepo: get repository %d: %v", id, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = database.DeleteRepository(repo.OwnerID, repo.ID); err != nil {
		log.Error("deleteAdminRepo: delete repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete repository")
	}
	log.Trace("Repository %d deleted by admin %q", repo.ID, user.Name)
	return http.StatusNoContent, nil, nil
}

type adminNotice struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

func getAdminNotices(c flamego.Context, l i18n.Locale) (statusCode int, resp *adminListPage[adminNotice], err error) {
	ctx := c.Request().Context()
	page, pageSize := pageParam(c), conf.UI.Admin.NoticePagingNum
	notices, err := database.Handle.Notices().List(ctx, page, pageSize)
	if err != nil {
		log.Error("getAdminNotices: list notices: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list notices")
	}

	items := make([]adminNotice, 0, len(notices))
	for _, n := range notices {
		items = append(items, adminNotice{
			ID:          n.ID,
			Type:        l.Tr(n.Type.TrStr()),
			Description: n.Description,
			CreatedAt:   unixTime(n.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, database.Handle.Notices().Count(ctx), pageSize), nil
}

type adminNoticesDeleteRequest struct {
	IDs []int64 `json:"ids" validate:"required,min=1,dive,min=1"`
}

func deleteAdminNotices(c flamego.Context, req adminNoticesDeleteRequest) (statusCode int, resp any, err error) {
	if err = database.Handle.Notices().DeleteByIDs(c.Request().Context(), req.IDs...); err != nil {
		log.Error("deleteAdminNotices: delete notices: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete notices")
	}
	return http.StatusNoContent, nil, nil
}

func deleteAllAdminNotices(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	if err = database.Handle.Notices().DeleteAll(c.Request().Context()); err != nil {
		log.Error("deleteAllAdminNotices: delete notices: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete all notices")
	}
	log.Trace("System notices deleted by admin %q", user.Name)
	return http.StatusNoContent, nil, nil
}
