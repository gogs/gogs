package web

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
)

// Viewer is the request-scoped user looking at a repository. User is nil for an
// anonymous viewer, including a disabled account.
type Viewer struct {
	*database.User

	access database.AccessMode
}

// newViewer returns the viewer for the request. Disabled users are effectively
// visiting as anonymous users.
func newViewer(ctx context.Context, user *database.User, owner *database.User, repo *database.Repository) *Viewer {
	v := &Viewer{}
	if user != nil && !user.IsDisabled() {
		v.User = user
	}
	if v.User != nil && v.IsAdmin {
		v.access = database.AccessModeOwner
		return v
	}
	v.access = database.Handle.Permissions().AccessMode(
		ctx,
		v.ID(),
		repo.ID,
		database.AccessModeOptions{
			OwnerID: owner.ID,
			Private: repo.IsPrivate,
		},
	)
	return v
}

// ID returns the viewer's user ID, or 0 when the viewer is anonymous.
func (v *Viewer) ID() int64 {
	if v == nil || v.User == nil {
		return 0
	}
	return v.User.ID
}

// Authenticated reports whether the viewer is an authenticated user.
func (v *Viewer) Authenticated() bool {
	return v != nil && v.User != nil
}

func (v *Viewer) CanRead() bool {
	return v.access >= database.AccessModeRead
}

func (v *Viewer) CanWrite() bool {
	return v.access >= database.AccessModeWrite
}

func (v *Viewer) CanAdminister() bool {
	return v.access >= database.AccessModeAdmin
}

// repoContext is the request-scoped view of a repository.
type repoContext struct {
	Owner  *database.User
	Repo   *database.Repository
	Viewer *Viewer
}

// withRepoContext injects the repoContext of the repository derived from the
// route.
func withRepoContext(c flamego.Context, user *database.User) {
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

	c.Map(&repoContext{
		Owner:  owner,
		Repo:   repo,
		Viewer: newViewer(ctx, user, owner, repo),
	})
}
