package web

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

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

func getUserFollowers(c flamego.Context, puser *profileUser) (statusCode int, resp *listPage[exploreUser], err error) {
	page := pageParam(c)
	users, err := database.Handle.Users().ListFollowers(c.Request().Context(), puser.ID, page, database.ItemsPerPage)
	if err != nil {
		log.Error("getUserFollowers: list followers of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list followers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(puser.NumFollowers), database.ItemsPerPage), nil
}

func getUserFollowing(c flamego.Context, puser *profileUser) (statusCode int, resp *listPage[exploreUser], err error) {
	page := pageParam(c)
	users, err := database.Handle.Users().ListFollowings(c.Request().Context(), puser.ID, page, database.ItemsPerPage)
	if err != nil {
		log.Error("getUserFollowing: list followings of user %d: %v", puser.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list followings")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(puser.NumFollowing), database.ItemsPerPage), nil
}

func getRepoStargazers(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *listPage[exploreUser], err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	repo := repoCtx.Repo
	page := pageParam(c)
	users, err := repo.GetStargazers(page)
	if err != nil {
		log.Error("getRepoStargazers: list stargazers of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list stargazers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(repo.NumStars), database.ItemsPerPage), nil
}

func getRepoWatchers(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *listPage[exploreUser], err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	repo := repoCtx.Repo
	page := pageParam(c)
	users, err := repo.GetWatchers(page)
	if err != nil {
		log.Error("getRepoWatchers: list watchers of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list watchers")
	}
	return http.StatusOK, newListPage(toPeople(users), page, int64(repo.NumWatches), database.ItemsPerPage), nil
}

// deleteRepoWatcher lets site admins make another user stop watching the
// repository, like the unwatch button of the server-rendered watchers page.
func deleteRepoWatcher(c flamego.Context, user *database.User, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	if user == nil || !user.IsAdmin {
		return http.StatusForbidden, nil, errors.New("site admin required")
	}

	ctx := c.Request().Context()
	name := c.Param("username")
	watcher, err := database.Handle.Users().GetByUsername(ctx, name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, errors.New("user does not exist")
		}
		log.Error("deleteRepoWatcher: get user by username %q: %v", name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user")
	}

	repo := repoCtx.Repo
	if err = database.WatchRepo(watcher.ID, repo.ID, false); err != nil {
		log.Error("deleteRepoWatcher: unwatch repo %d for user %d: %v", repo.ID, watcher.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "unwatch repo")
	}
	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		log.Error("deleteRepoWatcher: reload repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoWatchResponse{WatchCount: updated.NumWatches}, nil
}
