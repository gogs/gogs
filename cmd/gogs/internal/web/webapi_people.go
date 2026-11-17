package web

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
)

// toPeople converts a page of users listed as followers, stargazers or
// watchers, with the website, location and join date the server-rendered
// cards showed.
func toPeople(users []*database.User) []exploreUser {
	items := make([]exploreUser, 0, len(users))
	for _, u := range users {
		item := toExploreUser(u)
		item.Website = u.Website
		items = append(items, item)
	}
	return items
}

func getUserFollowers(logger *logx.Logger, c flamego.Context, puser *profileUser) (statusCode int, resp *listPage[exploreUser], err error) {
	// Organizations have no followers.
	if puser.IsOrganization() {
		return http.StatusNotFound, nil, errors.New("user does not exist")
	}

	page := pageParam(c)
	users, err := database.Handle.Users().ListFollowers(c.Request().Context(), puser.ID, page, database.ItemsPerPage)
	if err != nil {
		logger.Error("Failed to list followers of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list followers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(puser.NumFollowers), database.ItemsPerPage), nil
}

func getUserFollowing(logger *logx.Logger, c flamego.Context, puser *profileUser) (statusCode int, resp *listPage[exploreUser], err error) {
	// Organizations have no followers.
	if puser.IsOrganization() {
		return http.StatusNotFound, nil, errors.New("user does not exist")
	}

	page := pageParam(c)
	users, err := database.Handle.Users().ListFollowings(c.Request().Context(), puser.ID, page, database.ItemsPerPage)
	if err != nil {
		logger.Error("Failed to list followings of user", "userID", puser.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list followings")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(puser.NumFollowing), database.ItemsPerPage), nil
}

func getRepoStargazers(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *listPage[exploreUser], err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	repo := repoCtx.Repo
	page := pageParam(c)
	users, err := database.Handle.Repositories().ListStargazers(c.Request().Context(), repo.ID, page)
	if err != nil {
		logger.Error("Failed to list stargazers of repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list stargazers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(repo.NumStars), database.ItemsPerPage), nil
}

func getRepoWatchers(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *listPage[exploreUser], err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	repo := repoCtx.Repo
	page := pageParam(c)
	users, err := database.Handle.Repositories().ListWatchers(c.Request().Context(), repo.ID, page)
	if err != nil {
		logger.Error("Failed to list watchers of repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list watchers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(repo.NumWatches), database.ItemsPerPage), nil
}

// deleteRepoWatcher lets site admins make another user stop watching the
// repository, like the unwatch button of the server-rendered watchers page.
func deleteRepoWatcher(logger *logx.Logger, c flamego.Context, viewer *database.User, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	if viewer == nil || !viewer.IsAdmin {
		return http.StatusForbidden, nil, errors.New("site admin required")
	}

	ctx := c.Request().Context()
	name := c.Param("username")
	watcher, err := database.Handle.Users().GetByUsername(ctx, name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, errors.New("user does not exist")
		}
		logger.Error("Failed to get user by username", "username", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user")
	}

	repo := repoCtx.Repo
	if err = database.Handle.Repositories().Unwatch(ctx, watcher.ID, repo.ID); err != nil {
		logger.Error("Failed to unwatch repository", "repoID", repo.ID, "userID", watcher.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "unwatch repo")
	}
	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		logger.Error("Failed to reload repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoWatchResponse{WatchCount: updated.NumWatches}, nil
}
