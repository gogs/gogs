package web

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
)

var errRepoOwnerNotAllowed = errors.New("not allowed to create repositories for this owner")

// repoOwner returns the user or organization that a repository is created
// for. It is the viewer when ownerID is zero, their own ID or no user, and
// otherwise an organization they own. Site admins
// may use any organization.
func repoOwner(ctx context.Context, viewer *database.User, ownerID int64) (*database.User, error) {
	if ownerID == 0 || ownerID == viewer.ID {
		return viewer, nil
	}
	owner, err := database.Handle.Users().GetByID(ctx, ownerID)
	if database.IsErrUserNotExist(err) {
		return viewer, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	if !owner.IsOrganization() || !(viewer.IsAdmin || database.Handle.Organizations().IsOwner(ctx, owner.ID, viewer.ID)) {
		return nil, errRepoOwnerNotAllowed
	}
	return owner, nil
}

type repoNewOwner struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	AvatarURL      string `json:"avatarURL"`
	IsOrganization bool   `json:"isOrganization"`
}

type repoNewOptions struct {
	// Owners are the viewer and the organizations they own, along with the
	// requested organization when a site admin asks for one they do not own.
	Owners  []repoNewOwner `json:"owners"`
	OwnerID int64          `json:"ownerID"`
	// Private is the visibility of the viewer's last repository.
	Private bool `json:"private"`
	// ForcePrivate is true when the site makes every new repository private.
	ForcePrivate   bool     `json:"forcePrivate"`
	CanImportLocal bool     `json:"canImportLocal"`
	Gitignores     []string `json:"gitignores"`
	Licenses       []string `json:"licenses"`
	Readmes        []string `json:"readmes"`
}

// repoNewOwners lists the owners the viewer can create repositories for: the
// viewer and the organizations they own, and then the given owner when it is
// none of them, e.g., an organization a site admin asks for.
func repoNewOwners(ctx context.Context, viewer, owner *database.User) ([]repoNewOwner, error) {
	orgs, err := database.Handle.Organizations().ListOwnedBy(ctx, viewer.ID)
	if err != nil {
		return nil, errors.Wrap(err, "get owned organizations")
	}
	owners := []repoNewOwner{{ID: viewer.ID, Name: viewer.Name, AvatarURL: viewer.AvatarURLPath()}}
	listed := owner.ID == viewer.ID
	for _, org := range orgs {
		owners = append(owners, repoNewOwner{ID: org.ID, Name: org.Name, AvatarURL: org.AvatarURLPath(), IsOrganization: true})
		listed = listed || org.ID == owner.ID
	}
	if !listed {
		owners = append(owners, repoNewOwner{ID: owner.ID, Name: owner.Name, AvatarURL: owner.AvatarURLPath(), IsOrganization: owner.IsOrganization()})
	}
	return owners, nil
}

func getRepoNew(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	orgID, _ := strconv.ParseInt(c.Query("org"), 10, 64)
	owner, err := repoOwner(c.Request().Context(), viewer, orgID)
	if errors.Is(err, errRepoOwnerNotAllowed) {
		// Not 403, which the page takes for a viewer who may not sign in.
		return http.StatusNotFound, nil, err
	} else if err != nil {
		logger.Error("Failed to get owner", "ownerID", orgID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	owners, err := repoNewOwners(ctx, viewer, owner)
	if err != nil {
		logger.Error("Failed to list owners", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, &repoNewOptions{
		Owners:         owners,
		OwnerID:        owner.ID,
		Private:        viewer.LastRepoVisibility,
		ForcePrivate:   conf.Repository.ForcePrivate,
		CanImportLocal: viewer.CanImportLocal(),
		Gitignores:     database.Gitignores,
		Licenses:       database.Licenses,
		Readmes:        database.Readmes,
	}, nil
}

type repoCreated struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// repoCreateError returns the response for a repository that could not be
// created for a reason the viewer can fix, or nil when err is about something
// else.
func repoCreateError(l i18n.Locale, err error) (int, *bindingErrorResponse) {
	switch {
	case database.IsErrReachLimitOfRepo(err):
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("repo::form.reach_limit_of_creation", "limit", err.(database.ErrReachLimitOfRepo).Limit)}
	case database.IsErrRepoAlreadyExist(err):
		return repoNameTakenError(l)
	case database.IsErrNameNotAllowed(err):
		msg := l.Tr("repo::form.name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value())
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}
	}
	return 0, nil
}

func repoNameTakenError(l i18n.Locale) (int, *bindingErrorResponse) {
	msg := l.Tr("form::repo_name_been_taken")
	return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}
}

// checkRepoName returns the response for a name the owner already has a
// repository of. CreateRepository finds the directory of that repository first
// and reports it as an unexpected error.
func checkRepoName(ctx context.Context, logger *logx.Logger, l i18n.Locale, owner *database.User, name string) (statusCode int, resp any, err error) {
	taken, err := database.Handle.Repositories().Exists(ctx, owner, name)
	if err != nil {
		logger.Error("Failed to check repository existence", "ownerName", owner.Name, "repoName", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "check repository existence")
	} else if taken {
		code, resp := repoNameTakenError(l)
		return code, resp, nil
	}
	return 0, nil, nil
}

// repoOwnerError returns the response for an owner the viewer cannot create
// repositories for, which the form never offers.
func repoOwnerError(l i18n.Locale) *bindingErrorResponse {
	msg := l.Tr("form::invalid_request")
	return &bindingErrorResponse{Fields: fieldErrors{"ownerID": &msg}}
}

type repoCreateRequest struct {
	OwnerID     int64  `json:"ownerID"`
	RepoName    string `json:"name" validate:"required,alphadashdot,max=100"`
	Description string `json:"description" validate:"max=512"`
	Private     bool   `json:"private"`
	Unlisted    bool   `json:"unlisted"`
	// AutoInit makes the first commit with the chosen files. The files are
	// ignored without it.
	AutoInit   bool     `json:"autoInit"`
	Gitignores []string `json:"gitignores"`
	License    string   `json:"license"`
	Readme     string   `json:"readme"`
	// ObjectFormat is the hash algorithm of the repository, SHA-1 when empty.
	ObjectFormat string `json:"objectFormat" validate:"omitempty,oneof=sha1 sha256"`
}

func postRepoCreate(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req repoCreateRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	owner, err := repoOwner(c.Request().Context(), viewer, req.OwnerID)
	if errors.Is(err, errRepoOwnerNotAllowed) {
		return http.StatusForbidden, repoOwnerError(l), nil
	} else if err != nil {
		logger.Error("Failed to get owner", "ownerID", req.OwnerID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if code, resp, err := checkRepoName(ctx, logger, l, owner, req.RepoName); code != 0 {
		return code, resp, err
	}

	opts := database.CreateRepoOptionsLegacy{
		Name:         req.RepoName,
		Description:  req.Description,
		IsPrivate:    req.Private || conf.Repository.ForcePrivate,
		IsUnlisted:   req.Unlisted,
		AutoInit:     req.AutoInit,
		ObjectFormat: git.ObjectFormat(req.ObjectFormat),
	}
	if req.AutoInit {
		// Unknown files would fail the first commit after the repository is
		// created, so they are turned away first. The form only offers known
		// ones.
		fields := fieldErrors{}
		msg := l.Tr("form::invalid_request")
		for _, name := range req.Gitignores {
			if !slices.Contains(database.Gitignores, name) {
				fields["gitignores"] = &msg
			}
		}
		if req.License != "" && !slices.Contains(database.Licenses, req.License) {
			fields["license"] = &msg
		}
		if !slices.Contains(database.Readmes, req.Readme) {
			fields["readme"] = &msg
		}
		if len(fields) > 0 {
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
		}
		opts.Gitignores = strings.Join(req.Gitignores, ",")
		opts.License = req.License
		opts.Readme = req.Readme
	}

	repo, err := database.Handle.Repositories().Create(c.Request().Context(), logger, viewer, owner, opts)
	if err != nil {
		if repo != nil {
			if errDelete := database.Handle.Repositories().Delete(ctx, logger, owner.ID, repo.ID); errDelete != nil {
				logger.Error("Failed to delete repository", "repoID", repo.ID, "error", errDelete)
			}
		}
		if code, resp := repoCreateError(l, err); resp != nil {
			return code, resp, nil
		}
		logger.Error("Failed to create repository", "ownerName", owner.Name, "repoName", req.RepoName, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create repository")
	}
	return http.StatusCreated, &repoCreated{Owner: owner.Name, Name: repo.Name}, nil
}

type repoMigrateRequest struct {
	CloneAddr    string `json:"cloneAddr" validate:"required"`
	AuthUsername string `json:"authUsername"`
	AuthPassword string `json:"authPassword"`
	OwnerID      int64  `json:"ownerID"`
	RepoName     string `json:"name" validate:"required,alphadashdot,max=100"`
	Mirror       bool   `json:"mirror"`
	Private      bool   `json:"private"`
	Unlisted     bool   `json:"unlisted"`
	Description  string `json:"description" validate:"max=512"`
}

// cloneAddrError returns the status code and the message for an address to
// clone from that cannot be used, or false when err is about something else.
// label names the field of the address.
func cloneAddrError(l i18n.Locale, label string, err error) (statusCode int, msg string, ok bool) {
	var addrErr database.ErrInvalidCloneAddr
	if !errors.As(err, &addrErr) {
		return 0, "", false
	}
	switch {
	case addrErr.IsURLError:
		return http.StatusBadRequest, label + l.Tr("form::url_error"), true
	case addrErr.IsPermissionDenied:
		return http.StatusForbidden, l.Tr("repo::migrate.permission_denied"), true
	case addrErr.IsInvalidPath:
		return http.StatusBadRequest, l.Tr("repo::migrate.invalid_local_path"), true
	case addrErr.IsBlockedLocalAddress:
		return http.StatusForbidden, l.Tr("repo::migrate.clone_address_resolved_to_blocked_local_address"), true
	case addrErr.IsSSHAddress:
		return http.StatusBadRequest, l.Tr("repo::migrate.ssh_address_not_supported"), true
	}
	return 0, "", false
}

func postRepoMigrate(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req repoMigrateRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	owner, err := repoOwner(c.Request().Context(), viewer, req.OwnerID)
	if errors.Is(err, errRepoOwnerNotAllowed) {
		return http.StatusForbidden, repoOwnerError(l), nil
	} else if err != nil {
		logger.Error("Failed to get owner", "ownerID", req.OwnerID, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if code, resp, err := checkRepoName(ctx, logger, l, owner, req.RepoName); code != 0 {
		return code, resp, err
	}

	remoteAddr, err := form.ParseRemoteAddr(form.ParseRemoteAddrOptions{
		CloneAddr:    req.CloneAddr,
		User:         viewer,
		AuthUsername: req.AuthUsername,
		AuthPassword: req.AuthPassword,
	})
	if err != nil {
		code, msg, ok := cloneAddrError(l, l.Tr("form::CloneAddr"), err)
		if !ok {
			logger.Error("Failed to parse remote address", "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parse remote address")
		}
		return code, &bindingErrorResponse{Fields: fieldErrors{"cloneAddr": &msg}}, nil
	}

	repo, err := database.Handle.Repositories().Migrate(c.Request().Context(), logger, viewer, owner, database.MigrateRepoOptions{
		Name:        req.RepoName,
		Description: req.Description,
		IsPrivate:   req.Private || conf.Repository.ForcePrivate,
		IsUnlisted:  req.Unlisted,
		IsMirror:    req.Mirror,
		RemoteAddr:  remoteAddr,
	})
	if err == nil {
		return http.StatusCreated, &repoCreated{Owner: owner.Name, Name: repo.Name}, nil
	}
	if repo != nil {
		if errDelete := database.Handle.Repositories().Delete(ctx, logger, owner.ID, repo.ID); errDelete != nil {
			logger.Error("Failed to delete repository", "repoID", repo.ID, "error", errDelete)
		}
	}
	if code, resp := repoCreateError(l, err); resp != nil {
		return code, resp, nil
	}

	// The reasons Git gives help the viewer fix the address or the
	// credentials, once the credentials are masked.
	reason := database.HandleMirrorCredentials(err.Error(), true)
	switch {
	case strings.Contains(err.Error(), "Authentication failed") ||
		strings.Contains(err.Error(), "could not read Username"):
		msg := l.Tr("form::auth_failed", "error", reason)
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"authUsername": &msg, "authPassword": nil}}, nil
	case strings.Contains(err.Error(), "fatal:"):
		logger.Error("Failed to migrate repository", "ownerName", owner.Name, "repoName", req.RepoName, "error", reason)
		msg := l.Tr("repo::migrate.failed", "error", reason)
		return http.StatusBadGateway, &bindingErrorResponse{Fields: fieldErrors{"cloneAddr": &msg}}, nil
	}
	logger.Error("Failed to migrate repository", "ownerName", owner.Name, "repoName", req.RepoName, "error", reason)
	return http.StatusInternalServerError, nil, errors.New("migrate repository")
}
