package web

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/ptrx"
)

// repoContext is the request-scoped viewer of the repository. Viewer can be an
// authenticated or anonymous user.
type repoContext struct {
	Owner *database.User
	Repo  *database.Repository

	ViewerID     int64
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
func withRepoContext(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	ownerName := c.Param("owner")
	repoName := c.Param("repo")

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
		log.Error("repoContext: get user by username %q: %v", ownerName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get owner")
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
		log.Error("repoContext: get repo by name %q/%q: %v", ownerName, repoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repo")
	}

	viewer := ptrx.Deref(user, database.User{})
	var viewerAccess database.AccessMode
	if viewer.IsAdmin {
		viewerAccess = database.AccessModeOwner
	} else {
		viewerAccess = database.Handle.Permissions().AccessMode(
			ctx,
			viewer.ID,
			repo.ID,
			database.AccessModeOptions{
				OwnerID: owner.ID,
				Private: repo.IsPrivate,
			},
		)
	}

	c.Map(&repoContext{
		Owner:        owner,
		Repo:         repo,
		ViewerID:     viewer.ID,
		viewerAccess: viewerAccess,
	})
	return StatusNextHandler, nil, nil
}
