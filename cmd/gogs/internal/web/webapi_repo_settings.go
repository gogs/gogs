package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/urlx"
)

// repoAdminContext is the repository whose settings a repository admin
// changes.
type repoAdminContext struct {
	Owner *database.User
	// Repo has its owner and base repository loaded, which the functions that
	// change it need.
	Repo         *database.Repository
	viewerAccess database.AccessMode
}

// viewerCanManage returns whether the viewer may convert, transfer or delete
// the repository, or erase its wiki. Only its owners may, and for an
// organization, only the owners of the organization, or site admins when
// allowSiteAdmin is true.
func (rc *repoAdminContext) viewerCanManage(ctx context.Context, viewer *database.User, allowSiteAdmin bool) bool {
	if rc.viewerAccess < database.AccessModeOwner {
		return false
	}
	if rc.Owner.IsOrganization() && !(allowSiteAdmin && viewer.IsAdmin) {
		return database.Handle.Organizations().IsOwner(ctx, rc.Owner.ID, viewer.ID)
	}
	return true
}

// withRepoAdmin injects the repoAdminContext of the repository, which only its
// admins and site admins can see. The settings of the repository are not found
// for others.
func withRepoAdmin(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.Viewer.CanAdminister() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}
	repos := database.Handle.Repositories()
	repo, err := repos.GetByID(c.Request().Context(), repoCtx.Repo.ID)
	if err == nil {
		err = repos.LoadAttributes(c.Request().Context(), repo)
	}
	if err != nil {
		logger.Error("Failed to get repository", "repoID", repoCtx.Repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get repository"))
		return
	}
	c.Map(&repoAdminContext{Owner: repo.Owner, Repo: repo, viewerAccess: repoCtx.Viewer.access})
}

type repoMirrorSettings struct {
	// Address includes the credentials, which only admins of the repository
	// see.
	Address     string `json:"address"`
	Interval    int    `json:"interval"`
	EnablePrune bool   `json:"enablePrune"`
	// LastSyncedAt is only sent for mirrors that synced since they were
	// created.
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
}

type repoAdvancedSettings struct {
	EnableWiki            bool   `json:"enableWiki"`
	AllowPublicWiki       bool   `json:"allowPublicWiki"`
	EnableExternalWiki    bool   `json:"enableExternalWiki"`
	ExternalWikiURL       string `json:"externalWikiURL"`
	EnableIssues          bool   `json:"enableIssues"`
	AllowPublicIssues     bool   `json:"allowPublicIssues"`
	EnableExternalTracker bool   `json:"enableExternalTracker"`
	ExternalTrackerURL    string `json:"externalTrackerURL"`
	ExternalTrackerFormat string `json:"externalTrackerFormat"`
	ExternalTrackerStyle  string `json:"externalTrackerStyle" validate:"omitempty,oneof=numeric alphanumeric"`
	EnablePulls           bool   `json:"enablePulls"`
	PullsIgnoreWhitespace bool   `json:"pullsIgnoreWhitespace"`
	PullsAllowMerge       bool   `json:"pullsAllowMerge"`
	PullsAllowRebase      bool   `json:"pullsAllowRebase"`
	PullsAllowSquash      bool   `json:"pullsAllowSquash"`
	PullsDeleteBranch     bool   `json:"pullsDeleteBranch"`
	// PullsMergeMessage and PullsSquashMessage are the presets of the
	// messages merge commits and squashing start with, empty for the
	// default message.
	PullsMergeMessage  string `json:"pullsMergeMessage" validate:"omitempty,oneof=title title_description"`
	PullsSquashMessage string `json:"pullsSquashMessage" validate:"omitempty,oneof=title title_description title_commits"`
}

type repoSettings struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Private     bool   `json:"private"`
	Unlisted    bool   `json:"unlisted"`
	IsFork      bool   `json:"isFork"`
	// VisibilityFollowsBase is true for forks that keep the visibility of their
	// base repository, i.e., unless the site forces private repositories.
	VisibilityFollowsBase bool   `json:"visibilityFollowsBase"`
	ForkCount             int    `json:"forkCount"`
	AvatarURL             string `json:"avatarURL"`
	// HasCustomAvatar is true when an avatar was uploaded, which can be
	// deleted.
	HasCustomAvatar bool                 `json:"hasCustomAvatar"`
	Advanced        repoAdvancedSettings `json:"advanced"`
	// CanEnablePulls is false when the repository cannot have pull requests,
	// e.g., a mirror.
	CanEnablePulls bool `json:"canEnablePulls"`
	// Mirror is only sent for mirrors.
	Mirror              *repoMirrorSettings `json:"mirror,omitempty"`
	OwnerIsOrganization bool                `json:"ownerIsOrganization"`
	// ViewerIsOwner is true when the viewer owns the repository and sees its
	// danger zone.
	ViewerIsOwner         bool `json:"viewerIsOwner"`
	ViewerCanEditGitHooks bool `json:"viewerCanEditGitHooks"`
	// ForcePrivate is true when the site forces private repositories and the
	// viewer cannot make this one public.
	ForcePrivate bool `json:"forcePrivate"`
	// CanBePublic and CanBeListed are false for forks whose base repository is
	// private or unlisted.
	CanBePublic bool `json:"canBePublic"`
	CanBeListed bool `json:"canBeListed"`
	// CanAllowPublicWiki and CanAllowPublicIssues are false for forks whose base
	// repository does not let them allow public access.
	CanAllowPublicWiki   bool `json:"canAllowPublicWiki"`
	CanAllowPublicIssues bool `json:"canAllowPublicIssues"`
}

func getRepoSettings(c flamego.Context, logger *logx.Logger, viewer *database.User, rc *repoAdminContext) (statusCode int, resp any, err error) {
	repo := rc.Repo
	avatarURL := repo.RelAvatarLink()
	settings := &repoSettings{
		Name:                  repo.Name,
		Description:           repo.Description,
		Website:               repo.Website,
		Private:               repo.IsPrivate,
		Unlisted:              repo.IsUnlisted,
		IsFork:                repo.IsFork,
		VisibilityFollowsBase: visibilityFollowsBase(repo),
		ForcePrivate:          isForcedPrivate(viewer, repo),
		ForkCount:             repo.NumForks,
		AvatarURL:             avatarURL,
		HasCustomAvatar:       avatarURL != "",
		Advanced: repoAdvancedSettings{
			EnableWiki:            repo.EnableWiki,
			AllowPublicWiki:       repo.AllowPublicWiki,
			EnableExternalWiki:    repo.EnableExternalWiki,
			ExternalWikiURL:       repo.ExternalWikiURL,
			EnableIssues:          repo.EnableIssues,
			AllowPublicIssues:     repo.AllowPublicIssues,
			EnableExternalTracker: repo.EnableExternalTracker,
			ExternalTrackerURL:    repo.ExternalTrackerURL,
			ExternalTrackerFormat: repo.ExternalTrackerFormat,
			ExternalTrackerStyle:  repo.ExternalTrackerStyle,
			EnablePulls:           repo.EnablePulls,
			PullsIgnoreWhitespace: repo.PullsIgnoreWhitespace,
			PullsAllowMerge:       repo.PullsAllowMerge,
			PullsAllowRebase:      repo.PullsAllowRebase,
			PullsAllowSquash:      repo.PullsAllowSquash,
			PullsDeleteBranch:     repo.PullsDeleteBranch,
			PullsMergeMessage:     repo.PullsMergeMessage,
			PullsSquashMessage:    repo.PullsSquashMessage,
		},
		CanEnablePulls:        repo.CanEnablePulls(),
		OwnerIsOrganization:   rc.Owner.IsOrganization(),
		ViewerIsOwner:         rc.viewerAccess >= database.AccessModeOwner,
		ViewerCanEditGitHooks: viewer.CanEditGitHook(),
		CanBePublic:           !repo.IsFork || !repo.BaseRepo.IsPrivate,
		CanBeListed:           !repo.IsFork || !repo.BaseRepo.IsUnlisted,
		CanAllowPublicWiki:    !repo.IsFork || repo.BaseRepo.ForksMayAllowPublicWiki(),
		CanAllowPublicIssues:  !repo.IsFork || repo.BaseRepo.ForksMayAllowPublicIssues(),
	}
	if avatarURL == "" {
		// The repository shows the avatar of its owner until it has its own.
		settings.AvatarURL = rc.Owner.AvatarURLPath()
	}
	if repo.IsMirror {
		mirror, err := database.Handle.Mirrors().GetByRepoID(c.Request().Context(), repo.ID)
		if err != nil {
			logger.Error("Failed to get mirror", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get mirror")
		}
		settings.Mirror = &repoMirrorSettings{
			Address:     mirror.RawAddress(),
			Interval:    mirror.Interval,
			EnablePrune: mirror.EnablePrune,
		}
		if mirror.LastSyncUnix > 0 {
			settings.Mirror.LastSyncedAt = &mirror.LastSync
		}
	}
	return http.StatusOK, settings, nil
}

type repoSettingsRequest struct {
	RepoName    string `json:"name" validate:"required,alphadashdot,max=100"`
	Description string `json:"description" validate:"max=512"`
	Website     string `json:"website" validate:"omitempty,url,max=100"`
	Private     bool   `json:"private"`
	Unlisted    bool   `json:"unlisted"`
}

type repoNameResponse struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// visibilityFollowsBase reports whether the repository is a fork that keeps the
// visibility of its base repository. When the site forces private repositories,
// forks have their own visibility, which saving never makes more visible than
// their base.
func visibilityFollowsBase(repo *database.Repository) bool {
	return repo.IsFork && !conf.Repository.ForcePrivate
}

// isForcedPrivate reports whether the viewer must keep the repository private
// because the site forces private repositories. Only site admins can make such
// a repository public.
func isForcedPrivate(viewer *database.User, repo *database.Repository) bool {
	return conf.Repository.ForcePrivate && repo.IsPrivate && !viewer.IsAdmin
}

// patchRepoSettings renames the repository and changes its description,
// website and visibility.
func patchRepoSettings(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoSettingsRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	oldName := repo.Name
	renamed := repo.LowerName != strings.ToLower(req.RepoName)
	if renamed {
		if err = database.Handle.Repositories().Rename(ctx, rc.Owner, repo.Name, req.RepoName); err != nil {
			var msg string
			switch {
			case database.IsErrRepoAlreadyExist(err):
				msg = l.Tr("form::repo_name_been_taken")
				return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			case database.IsErrNameNotAllowed(err):
				msg = l.Tr("repo::form.name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value())
				return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			}
			logger.Error("Failed to rename repository", "repoID", repo.ID, "newName", req.RepoName, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "rename repository")
		}
	}
	// The name can change in case only.
	repo.Name = req.RepoName
	repo.Description = req.Description
	repo.Website = req.Website

	private, unlisted := req.Private, req.Unlisted
	if visibilityFollowsBase(repo) {
		private, unlisted = repo.BaseRepo.IsPrivate, repo.BaseRepo.IsUnlisted
	}
	if isForcedPrivate(viewer, repo) {
		private = true
	}
	visibilityChanged := repo.IsPrivate != private || repo.IsUnlisted != unlisted
	repo.IsPrivate, repo.IsUnlisted = private, unlisted
	if err = database.Handle.Repositories().Update(ctx, logger, repo, visibilityChanged); err != nil {
		logger.Error("Failed to update repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}

	if renamed {
		if err := database.Handle.Actions().RenameRepo(c.Request().Context(), viewer, rc.Owner, oldName, repo); err != nil {
			logger.Error("Failed to create rename action", "repoID", repo.ID, "error", err)
		}
	}
	return http.StatusOK, &repoNameResponse{Owner: rc.Owner.Name, Name: repo.Name}, nil
}

func putRepoAvatar(c flamego.Context, logger *logx.Logger, l i18n.Locale, rc *repoAdminContext) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	data, code, err := readAvatarUpload(c.Request().Request, l)
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to read avatar upload", "repoID", rc.Repo.ID, "error", err)
		}
		return code, nil, err
	}
	if err = rc.Repo.UploadAvatar(data); err != nil {
		logger.Error("Failed to save avatar", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save avatar")
	}
	rc.Repo.UseCustomAvatar = true
	if err = database.Handle.Repositories().Update(ctx, logger, rc.Repo, false); err != nil {
		logger.Error("Failed to update repository", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoAvatar(logger *logx.Logger, rc *repoAdminContext) (statusCode int, resp any, err error) {
	if rc.Repo.RelAvatarLink() == "" {
		return http.StatusNoContent, nil, nil
	}
	if err = rc.Repo.DeleteAvatar(logger); err != nil {
		logger.Error("Failed to delete avatar", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete avatar")
	}
	return http.StatusNoContent, nil, nil
}

type repoMirrorRequest struct {
	Address     string `json:"address" validate:"required"`
	Interval    int    `json:"interval"`
	EnablePrune bool   `json:"enablePrune"`
}

// putRepoMirror changes where and how often a mirror syncs from.
func putRepoMirror(r *http.Request, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoMirrorRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !repo.IsMirror {
		return http.StatusNotFound, nil, errors.New("repository is not a mirror")
	}
	mirror, err := database.Handle.Mirrors().GetByRepoID(r.Context(), repo.ID)
	if err != nil {
		logger.Error("Failed to get mirror", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get mirror")
	}

	remoteAddr, err := form.ParseRemoteAddr(form.ParseRemoteAddrOptions{
		CloneAddr: req.Address,
		User:      viewer,
	})
	if err != nil {
		code, msg, ok := cloneAddrError(l, l.Tr("repo::mirror_address"), err)
		if !ok {
			logger.Error("Failed to parse remote address", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parse remote address")
		}
		return code, &bindingErrorResponse{Fields: fieldErrors{"address": &msg}}, nil
	}

	// An interval below an hour leaves the schedule as it was.
	if req.Interval > 0 {
		mirror.EnablePrune = req.EnablePrune
		mirror.Interval = req.Interval
		mirror.NextSync = time.Now().Add(time.Duration(req.Interval) * time.Hour)
		if err = database.Handle.Mirrors().Update(r.Context(), mirror); err != nil {
			logger.Error("Failed to update mirror", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "update mirror")
		}
	}

	if err = mirror.SaveAddress(r.Context(), remoteAddr); err != nil {
		logger.Error("Failed to save mirror address", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save address")
	}
	return http.StatusNoContent, nil, nil
}

// postRepoMirrorSync queues a sync of the mirror, which runs in the
// background.
func postRepoMirrorSync(rc *repoAdminContext) (statusCode int, resp any, err error) {
	if !rc.Repo.IsMirror {
		return http.StatusNotFound, nil, errors.New("repository is not a mirror")
	}
	go database.MirrorQueue.Add(rc.Repo.ID)
	return http.StatusAccepted, nil, nil
}

// putRepoAdvanced turns the wiki, the issues and the pull requests of the
// repository on or off.
func putRepoAdvanced(c flamego.Context, logger *logx.Logger, l i18n.Locale, rc *repoAdminContext, req repoAdvancedSettings) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if req.EnablePulls && !req.PullsAllowMerge && !req.PullsAllowSquash && !req.PullsAllowRebase {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: l.Tr("repo::settings.pulls.no_merge_method")}, nil
	}
	// Browsers are sent to these URLs, which must not run script, e.g., a
	// "javascript:" URL.
	if req.EnableWiki && req.EnableExternalWiki && !urlx.IsHTTP(req.ExternalWikiURL) {
		msg := l.Tr("repo::settings.external_wiki_url") + l.Tr("form::url_error")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"externalWikiURL": &msg}}, nil
	}
	if req.EnableIssues && req.EnableExternalTracker && !urlx.IsHTTP(req.ExternalTrackerURL) {
		msg := l.Tr("repo::settings.external_tracker_url") + l.Tr("form::url_error")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"externalTrackerURL": &msg}}, nil
	}
	repo := rc.Repo
	repo.EnableWiki = req.EnableWiki
	repo.AllowPublicWiki = req.AllowPublicWiki && req.EnableWiki && !req.EnableExternalWiki
	repo.EnableExternalWiki = req.EnableExternalWiki
	repo.ExternalWikiURL = req.ExternalWikiURL
	repo.EnableIssues = req.EnableIssues
	repo.AllowPublicIssues = req.AllowPublicIssues && req.EnableIssues && !req.EnableExternalTracker
	repo.EnableExternalTracker = req.EnableExternalTracker
	repo.ExternalTrackerURL = req.ExternalTrackerURL
	repo.ExternalTrackerFormat = req.ExternalTrackerFormat
	repo.ExternalTrackerStyle = req.ExternalTrackerStyle
	repo.EnablePulls = req.EnablePulls
	repo.PullsIgnoreWhitespace = req.PullsIgnoreWhitespace
	repo.PullsAllowMerge = req.PullsAllowMerge
	repo.PullsAllowRebase = req.PullsAllowRebase
	repo.PullsAllowSquash = req.PullsAllowSquash
	repo.PullsDeleteBranch = req.PullsDeleteBranch
	repo.PullsMergeMessage = req.PullsMergeMessage
	repo.PullsSquashMessage = req.PullsSquashMessage
	if err = database.Handle.Repositories().Update(ctx, logger, repo, false); err != nil {
		logger.Error("Failed to update repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}
	return http.StatusNoContent, nil, nil
}

// repoConfirmRequest carries the name of the repository, which the viewer
// types to confirm an action that cannot be taken back.
type repoConfirmRequest struct {
	RepoName string `json:"repoName" validate:"required"`
}

// confirmRepoName returns the response for a confirmation that does not name
// the repository, or nil when it does.
func confirmRepoName(l i18n.Locale, repo *database.Repository, name string) *bindingErrorResponse {
	if repo.Name == name {
		return nil
	}
	msg := l.Tr("form::enterred_invalid_repo_name")
	return &bindingErrorResponse{Fields: fieldErrors{"repoName": &msg}}
}

// postRepoConvert turns a mirror into a regular repository.
func postRepoConvert(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	if !rc.viewerCanManage(ctx, viewer, false) || !repo.IsMirror {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}

	repo.IsMirror = false
	if err = database.Handle.Repositories().CleanUpMigrateInfo(ctx, logger, repo); err != nil {
		logger.Error("Failed to clean up migrate info", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "clean up migrate info")
	}
	// The repository is no longer a mirror, so its mirror row has to go even
	// when the client goes away.
	if err = database.Handle.Mirrors().DeleteByRepoID(context.WithoutCancel(c.Request().Context()), repo.ID); err != nil {
		logger.Error("Failed to delete mirror", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete mirror")
	}
	return http.StatusNoContent, nil, nil
}

type repoTransferRequest struct {
	RepoName string `json:"repoName" validate:"required"`
	NewOwner string `json:"newOwner" validate:"required"`
}

// postRepoTransfer gives the repository to another user or organization.
func postRepoTransfer(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoTransferRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	if !rc.viewerCanManage(ctx, viewer, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if !database.Handle.Users().IsUsernameUsed(c.Request().Context(), req.NewOwner, rc.Owner.ID) {
		msg := l.Tr("form::enterred_invalid_owner_name")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"newOwner": &msg}}, nil
	}

	if err = database.Handle.Repositories().Transfer(ctx, viewer, req.NewOwner, repo); err != nil {
		if database.IsErrRepoAlreadyExist(err) {
			msg := l.Tr("repo::settings.new_owner_has_same_repo")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"newOwner": &msg}}, nil
		}
		logger.Error("Failed to transfer repository", "repoID", repo.ID, "newOwnerName", req.NewOwner, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "transfer ownership")
	}
	return http.StatusOK, &repoNameResponse{Owner: repo.Owner.Name, Name: repo.Name}, nil
}

// deleteRepo deletes the repository with everything in it.
func deleteRepo(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	if !rc.viewerCanManage(ctx, viewer, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if err = database.Handle.Repositories().Delete(ctx, logger, rc.Owner.ID, repo.ID); err != nil {
		logger.Error("Failed to delete repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete repository")
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoWiki erases the wiki of the repository and turns it off.
func deleteRepoWiki(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	if !rc.viewerCanManage(ctx, viewer, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	repo.DeleteWiki(logger)
	repo.EnableWiki = false
	if err = database.Handle.Repositories().Update(ctx, logger, repo, false); err != nil {
		logger.Error("Failed to update repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}
	return http.StatusNoContent, nil, nil
}

type repoCollaborator struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName"`
	AvatarURL string `json:"avatarURL"`
	// Mode is "read", "write" or "admin".
	Mode string `json:"mode"`
}

type repoCollaborators struct {
	Items []repoCollaborator `json:"items"`
}

func getRepoCollaborators(c flamego.Context, logger *logx.Logger, rc *repoAdminContext) (statusCode int, resp any, err error) {
	collaborators, err := database.Handle.Collaborations().List(c.Request().Context(), rc.Repo.ID)
	if err != nil {
		logger.Error("Failed to list collaborators", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get collaborators")
	}
	items := make([]repoCollaborator, 0, len(collaborators))
	for _, u := range collaborators {
		items = append(items, repoCollaborator{
			Name:      u.Name,
			FullName:  u.FullName,
			AvatarURL: u.AvatarURLPath(),
			Mode:      u.Collaboration.Mode.String(),
		})
	}
	return http.StatusOK, &repoCollaborators{Items: items}, nil
}

type repoCollaboratorRequest struct {
	Username string `json:"username" validate:"required"`
}

// postRepoCollaborator gives a user write access to the repository. Adding the
// owner or an existing collaborator changes nothing.
func postRepoCollaborator(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, rc *repoAdminContext, req repoCollaboratorRequest) (statusCode int, resp any, err error) {
	if strings.EqualFold(req.Username, rc.Owner.Name) {
		return http.StatusNoContent, nil, nil
	}
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), req.Username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			msg := l.Tr("form::user_not_exist")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"username": &msg}}, nil
		}
		logger.Error("Failed to get user", "userName", req.Username, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user")
	}
	if u.IsOrganization() {
		msg := l.Tr("repo::settings.org_not_allowed_to_be_collaborator")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"username": &msg}}, nil
	}

	if err = database.Handle.Collaborations().Add(c.Request().Context(), rc.Repo, u.ID); err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, err
		}
		logger.Error("Failed to add collaborator", "collaboratorID", u.ID, "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add collaborator")
	}
	if conf.User.EnableEmailNotification {
		if err := email.SendCollaboratorMail(logger, database.NewMailerUser(u), database.NewMailerUser(viewer), database.NewMailerRepo(rc.Repo)); err != nil {
			logger.Error("Failed to send collaborator mail", "collaboratorID", u.ID, "error", err)
		}
	}
	return http.StatusCreated, nil, nil
}

// repoCollaboratorUser returns the user named in the route, or the status
// code and the error to answer with.
func repoCollaboratorUser(c flamego.Context, logger *logx.Logger) (_ *database.User, statusCode int, err error) {
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, errors.New("user does not exist")
		}
		logger.Error("Failed to get user", "userName", c.Param("username"), "error", err)
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user")
	}
	return u, http.StatusOK, nil
}

type repoCollaboratorModeRequest struct {
	Mode string `json:"mode" validate:"required,oneof=read write admin"`
}

// patchRepoCollaborator changes the access of a collaborator, up to the access
// of the viewer.
func patchRepoCollaborator(c flamego.Context, logger *logx.Logger, rc *repoAdminContext, req repoCollaboratorModeRequest) (statusCode int, resp any, err error) {
	u, code, err := repoCollaboratorUser(c, logger)
	if err != nil {
		return code, nil, err
	}
	if err = database.Handle.Collaborations().ChangeMode(c.Request().Context(), rc.Repo, rc.viewerAccess, u.ID, database.ParseAccessMode(req.Mode)); err != nil {
		logger.Error("Failed to change collaboration access mode", "collaboratorID", u.ID, "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change collaboration access mode")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoCollaborator(c flamego.Context, logger *logx.Logger, rc *repoAdminContext) (statusCode int, resp any, err error) {
	u, code, err := repoCollaboratorUser(c, logger)
	if err != nil {
		return code, nil, err
	}
	if err = database.Handle.Collaborations().Delete(c.Request().Context(), rc.Repo, u.ID); err != nil {
		logger.Error("Failed to delete collaboration", "collaboratorID", u.ID, "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete collaboration")
	}
	return http.StatusNoContent, nil, nil
}

func getRepoDeployKeys(c flamego.Context, logger *logx.Logger, rc *repoAdminContext) (statusCode int, resp any, err error) {
	keys, err := database.Handle.DeployKeys().List(c.Request().Context(), rc.Repo.ID)
	if err != nil {
		logger.Error("Failed to list deploy keys", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list deploy keys")
	}
	items := make([]userSSHKey, 0, len(keys))
	for _, k := range keys {
		item := userSSHKey{ID: k.ID, Name: k.Name, Fingerprint: k.Fingerprint, CreatedAt: k.Created, IsRecentlyUsed: k.HasRecentActivity}
		if k.HasUsed {
			item.LastUsedAt = &k.Updated
		}
		items = append(items, item)
	}
	return http.StatusOK, &userSSHKeys{Items: items}, nil
}

// postRepoDeployKey gives an SSH key read access to the repository.
func postRepoDeployKey(c flamego.Context, logger *logx.Logger, l i18n.Locale, rc *repoAdminContext, req userSSHKeyRequest) (statusCode int, resp any, err error) {
	content, err := database.CheckPublicKeyString(logger, req.Content)
	if database.IsErrKeyCheckFailed(err) {
		logger.Error("Failed to check deploy key", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "check deploy key")
	} else if errors.Is(err, database.ErrSSHDisabled) {
		msg := l.Tr("settings::ssh_disabled")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	} else if err != nil {
		msg := l.Tr("form::invalid_ssh_key", "error", err)
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if _, err = database.Handle.DeployKeys().Add(c.Request().Context(), logger, rc.Repo.ID, req.SSHTitle, content); err != nil {
		var msg string
		switch {
		case database.IsErrKeyAlreadyExist(err), database.IsErrDeployKeyAlreadyExist(err):
			msg = l.Tr("repo::settings.key_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
		case database.IsErrDeployKeyNameAlreadyUsed(err):
			msg = l.Tr("repo::settings.key_name_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}, nil
		}
		logger.Error("Failed to add deploy key", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add deploy key")
	}
	return http.StatusCreated, nil, nil
}

// deleteRepoDeployKey deletes a deploy key of the repository. Keys of other
// repositories are not found.
func deleteRepoDeployKey(c flamego.Context, logger *logx.Logger, viewer *database.User, rc *repoAdminContext) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	key, err := database.Handle.DeployKeys().GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrDeployKeyNotExist(err) {
			return http.StatusNotFound, nil, errors.New("key not found")
		}
		logger.Error("Failed to get deploy key", "keyID", id, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get deploy key")
	}
	if key.RepoID != rc.Repo.ID {
		return http.StatusNotFound, nil, errors.New("key not found")
	}
	if err = database.Handle.DeployKeys().Delete(c.Request().Context(), logger, viewer, key.ID); err != nil {
		logger.Error("Failed to delete deploy key", "keyID", key.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete deploy key")
	}
	return http.StatusNoContent, nil, nil
}
