package web

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
)

// repoContext is the request-scoped viewer of the repository. Viewer is nil for
// anonymous users.
type repoContext struct {
	Owner *database.User
	Repo  *database.Repository

	Viewer       *database.User
	viewerAccess database.AccessMode
}

func (c *repoContext) ViewerCanRead() bool {
	return c.viewerAccess >= database.AccessModeRead
}

func (c *repoContext) ViewerCanWrite() bool {
	return c.viewerAccess >= database.AccessModeWrite
}

func (c *repoContext) ViewerCanAdminister() bool {
	return c.viewerAccess >= database.AccessModeAdmin
}

// withRepoContext injects the repoContext of the repository derived from the
// route.
func withRepoContext(c flamego.Context, viewer *database.User) {
	ctx := c.Request().Context()
	w := c.ResponseWriter()
	ownerName := c.Param("owner")
	repoName := c.Param("repo")

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
			return
		}
		log.Error("repoContext: get user by username %q: %v", ownerName, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get owner"))
		return
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
			return
		}
		log.Error("repoContext: get repo by name %q/%q: %v", ownerName, repoName, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get repo"))
		return
	}

	// Disabled users are effectively visiting as anonymous users.
	if viewer != nil && viewer.IsDisabled() {
		viewer = nil
	}

	c.Map(&repoContext{
		Owner:        owner,
		Repo:         repo,
		Viewer:       viewer,
		viewerAccess: viewerAccessMode(ctx, viewer, owner, repo),
	})
}

// viewerAccessMode returns the access mode of the viewer to the repository. A
// nil viewer is an anonymous user.
func viewerAccessMode(ctx context.Context, viewer *database.User, owner *database.User, repo *database.Repository) database.AccessMode {
	opts := database.AccessModeOptions{
		OwnerID: owner.ID,
		Private: repo.IsPrivate,
	}
	if viewer == nil {
		return database.Handle.Permissions().AccessMode(ctx, 0, repo.ID, opts)
	}
	if viewer.IsAdmin {
		return database.AccessModeOwner
	}
	return database.Handle.Permissions().AccessMode(ctx, viewer.ID, repo.ID, opts)
}
