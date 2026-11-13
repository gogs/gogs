package web

import (
	"net/http"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
)

var errForkBaseNotFound = errors.New("repository not found")

// forkBaseRepo returns the repository to fork. Like the server-rendered page,
// repositories that are empty or that the viewer cannot read are not found.
func forkBaseRepo(viewer *database.User, id int64) (*database.Repository, error) {
	repo, err := database.GetRepositoryByID(id)
	if database.IsErrRepoNotExist(err) {
		return nil, errForkBaseNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "get repository")
	}
	if !repo.CanBeForked() || !repo.HasAccess(viewer.ID) {
		return nil, errForkBaseNotFound
	}
	if err = repo.GetOwner(); err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	return repo, nil
}

// forkBase is the repository to fork.
type forkBase struct {
	*database.Repository
}

// withForkBaseRepo maps the repository named by the `id` path parameter as
// *forkBase.
func withForkBaseRepo(c flamego.Context, user *database.User) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	repo, err := forkBaseRepo(user, id)
	if errors.Is(err, errForkBaseNotFound) {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, err)
		return
	} else if err != nil {
		log.Error("withForkBaseRepo: get repository %d: %v", id, err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, err)
		return
	}
	c.Map(&forkBase{Repository: repo})
}

type repoForkBaseInfo struct {
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"isPrivate"`
	IsUnlisted  bool   `json:"isUnlisted"`
}

type repoForkOptions struct {
	Base repoForkBaseInfo `json:"base"`
	// Owners are the viewer and the organizations they own.
	Owners []repoNewOwner `json:"owners"`
}

func getRepoFork(user *database.User, base *forkBase) (statusCode int, resp any, err error) {
	owners, err := repoNewOwners(user, user)
	if err != nil {
		log.Error("getRepoFork: list owners for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, &repoForkOptions{
		Base: repoForkBaseInfo{
			Owner:       base.Owner.Name,
			Name:        base.Name,
			Description: base.Description,
			IsPrivate:   base.IsPrivate,
			IsUnlisted:  base.IsUnlisted,
		},
		Owners: owners,
	}, nil
}

type repoForkRequest struct {
	OwnerID     int64  `json:"ownerID"`
	RepoName    string `json:"name" validate:"required,alphadashdot,max=100"`
	Description string `json:"description" validate:"max=512"`
}

func postRepoFork(c flamego.Context, l i18n.Locale, user *database.User, base *forkBase, req repoForkRequest) (statusCode int, resp any, err error) {
	owner, err := repoOwner(c.Request().Context(), user, req.OwnerID)
	// Unlike creating a repository, forking to an organization takes owning
	// it, even for site admins.
	if errors.Is(err, errRepoOwnerNotAllowed) || (err == nil && owner.IsOrganization() && !owner.IsOwnedBy(user.ID)) {
		return http.StatusForbidden, repoOwnerError(l), nil
	} else if err != nil {
		log.Error("postRepoFork: get owner %d: %v", req.OwnerID, err)
		return http.StatusInternalServerError, nil, err
	}

	fork, has, err := database.HasForkedRepo(owner.ID, base.ID)
	if err != nil {
		log.Error("postRepoFork: check fork of repository %d by owner %d: %v", base.ID, owner.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "check forked repository")
	} else if has {
		// The owner already has a fork, which the viewer is sent to instead.
		return http.StatusOK, &repoCreated{Owner: owner.Name, Name: fork.Name}, nil
	}
	if owner.ID == base.OwnerID {
		msg := l.Tr("repo.settings.cannot_fork_to_same_owner")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"ownerID": &msg}}, nil
	}

	fork, err = database.ForkRepository(c.Request().Context(), user, owner, base.Repository, req.RepoName, req.Description)
	if err != nil {
		if database.IsErrRepoAlreadyExist(err) {
			msg := l.Tr("repo.settings.new_owner_has_same_repo")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
		}
		if code, resp := repoCreateError(l, err); resp != nil {
			return code, resp, nil
		}
		log.Error("postRepoFork: fork repository %d to %s/%s: %v", base.ID, owner.Name, req.RepoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "fork repository")
	}
	return http.StatusCreated, &repoCreated{Owner: owner.Name, Name: fork.Name}, nil
}
