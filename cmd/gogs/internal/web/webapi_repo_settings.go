package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
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
func (rc *repoAdminContext) viewerCanManage(user *database.User, allowSiteAdmin bool) bool {
	if rc.viewerAccess < database.AccessModeOwner {
		return false
	}
	if rc.Owner.IsOrganization() && !(allowSiteAdmin && user.IsAdmin) {
		return rc.Owner.IsOwnedBy(user.ID)
	}
	return true
}

// withRepoAdmin injects the repoAdminContext of the repository, which only its
// admins and site admins can see. Like the server-rendered pages, the settings
// of the repository are not found for others.
func withRepoAdmin(c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.Viewer.CanAdminister() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}
	repo, err := database.GetRepositoryByID(repoCtx.Repo.ID)
	if err != nil {
		log.Error("withRepoAdmin: get repository %d: %v", repoCtx.Repo.ID, err)
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
	PullsAllowRebase      bool   `json:"pullsAllowRebase"`
}

type repoSettings struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Private     bool   `json:"private"`
	Unlisted    bool   `json:"unlisted"`
	// IsFork is true for forks, which keep the visibility of their base
	// repository.
	IsFork    bool   `json:"isFork"`
	ForkCount int    `json:"forkCount"`
	AvatarURL string `json:"avatarURL"`
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
}

func getRepoSettings(user *database.User, rc *repoAdminContext) (statusCode int, resp any, err error) {
	repo := rc.Repo
	avatarURL := repo.RelAvatarLink()
	settings := &repoSettings{
		Name:            repo.Name,
		Description:     repo.Description,
		Website:         repo.Website,
		Private:         repo.IsPrivate,
		Unlisted:        repo.IsUnlisted,
		IsFork:          repo.IsFork,
		ForkCount:       repo.NumForks,
		AvatarURL:       avatarURL,
		HasCustomAvatar: avatarURL != "",
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
			PullsAllowRebase:      repo.PullsAllowRebase,
		},
		CanEnablePulls:        repo.CanEnablePulls(),
		OwnerIsOrganization:   rc.Owner.IsOrganization(),
		ViewerIsOwner:         rc.viewerAccess >= database.AccessModeOwner,
		ViewerCanEditGitHooks: user.CanEditGitHook(),
	}
	if avatarURL == "" {
		// The repository shows the avatar of its owner until it has its own.
		settings.AvatarURL = tool.AppendAvatarSize(rc.Owner.AvatarURLPath(), 200)
	}
	if repo.IsMirror {
		mirror, err := database.GetMirrorByRepoID(repo.ID)
		if err != nil {
			log.Error("getRepoSettings: get mirror of repository %d: %v", repo.ID, err)
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

// patchRepoSettings renames the repository and changes its description,
// website and visibility.
func patchRepoSettings(c flamego.Context, l i18n.Locale, user *database.User, rc *repoAdminContext, req repoSettingsRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	oldName := repo.Name
	renamed := repo.LowerName != strings.ToLower(req.RepoName)
	if renamed {
		if err = database.ChangeRepositoryName(rc.Owner, repo.Name, req.RepoName); err != nil {
			var msg string
			switch {
			case database.IsErrRepoAlreadyExist(err):
				msg = l.Tr("form.repo_name_been_taken")
				return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			case database.IsErrNameNotAllowed(err):
				msg = l.Tr("repo.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
				return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			}
			log.Error("patchRepoSettings: rename repository %d to %q: %v", repo.ID, req.RepoName, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "rename repository")
		}
	}
	// The name can change in case only.
	repo.Name = req.RepoName
	repo.Description = req.Description
	repo.Website = req.Website

	private, unlisted := req.Private, req.Unlisted
	if repo.IsFork {
		private, unlisted = repo.BaseRepo.IsPrivate, repo.BaseRepo.IsUnlisted
	}
	visibilityChanged := repo.IsPrivate != private || repo.IsUnlisted != unlisted
	repo.IsPrivate, repo.IsUnlisted = private, unlisted
	if err = database.UpdateRepository(repo, visibilityChanged); err != nil {
		log.Error("patchRepoSettings: update repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}

	if renamed {
		if err := database.Handle.Actions().RenameRepo(c.Request().Context(), user, rc.Owner, oldName, repo); err != nil {
			log.Error("patchRepoSettings: create rename action of repository %d: %v", repo.ID, err)
		}
	}
	return http.StatusOK, &repoNameResponse{Owner: rc.Owner.Name, Name: repo.Name}, nil
}

func putRepoAvatar(c flamego.Context, l i18n.Locale, rc *repoAdminContext) (statusCode int, resp any, err error) {
	data, code, err := readAvatarUpload(c.Request().Request, l)
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("putRepoAvatar: repository %d: %v", rc.Repo.ID, err)
		}
		return code, nil, err
	}
	if err = rc.Repo.UploadAvatar(data); err != nil {
		log.Error("putRepoAvatar: save avatar of repository %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save avatar")
	}
	rc.Repo.UseCustomAvatar = true
	if err = database.UpdateRepository(rc.Repo, false); err != nil {
		log.Error("putRepoAvatar: update repository %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoAvatar(rc *repoAdminContext) (statusCode int, resp any, err error) {
	if rc.Repo.RelAvatarLink() == "" {
		return http.StatusNoContent, nil, nil
	}
	if err = rc.Repo.DeleteAvatar(); err != nil {
		log.Error("deleteRepoAvatar: delete avatar of repository %d: %v", rc.Repo.ID, err)
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
func putRepoMirror(r *http.Request, l i18n.Locale, user *database.User, rc *repoAdminContext, req repoMirrorRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !repo.IsMirror {
		return http.StatusNotFound, nil, errors.New("repository is not a mirror")
	}
	mirror, err := database.GetMirrorByRepoID(repo.ID)
	if err != nil {
		log.Error("putRepoMirror: get mirror of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get mirror")
	}

	// Like the server-rendered page, an interval below an hour leaves the
	// schedule as it was.
	if req.Interval > 0 {
		mirror.EnablePrune = req.EnablePrune
		mirror.Interval = req.Interval
		mirror.NextSync = time.Now().Add(time.Duration(req.Interval) * time.Hour)
		if err = database.UpdateMirror(mirror); err != nil {
			log.Error("putRepoMirror: update mirror of repository %d: %v", repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "update mirror")
		}
	}

	remoteAddr, err := form.ParseRemoteAddr(form.ParseRemoteAddrOptions{
		CloneAddr: req.Address,
		User:      user,
	})
	if err != nil {
		code, msg, ok := cloneAddrError(l, l.Tr("repo.mirror_address"), err)
		if !ok {
			log.Error("putRepoMirror: parse remote address: %v", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parse remote address")
		}
		return code, &bindingErrorResponse{Fields: fieldErrors{"address": &msg}}, nil
	}
	if err = mirror.SaveAddress(r.Context(), remoteAddr); err != nil {
		log.Error("putRepoMirror: save address of mirror %d: %v", repo.ID, err)
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
func putRepoAdvanced(rc *repoAdminContext, req repoAdvancedSettings) (statusCode int, resp any, err error) {
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
	repo.PullsAllowRebase = req.PullsAllowRebase
	if err = database.UpdateRepository(repo, false); err != nil {
		log.Error("putRepoAdvanced: update repository %d: %v", repo.ID, err)
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
	msg := l.Tr("form.enterred_invalid_repo_name")
	return &bindingErrorResponse{Fields: fieldErrors{"repoName": &msg}}
}

// postRepoConvert turns a mirror into a regular repository.
func postRepoConvert(l i18n.Locale, user *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !rc.viewerCanManage(user, false) || !repo.IsMirror {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}

	repo.IsMirror = false
	if _, err = database.CleanUpMigrateInfo(repo); err != nil {
		log.Error("postRepoConvert: clean up migration of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "clean up migrate info")
	}
	if err = database.DeleteMirrorByRepoID(repo.ID); err != nil {
		log.Error("postRepoConvert: delete mirror of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete mirror")
	}
	return http.StatusNoContent, nil, nil
}

type repoTransferRequest struct {
	RepoName string `json:"repoName" validate:"required"`
	NewOwner string `json:"newOwner" validate:"required"`
}

// postRepoTransfer gives the repository to another user or organization.
func postRepoTransfer(c flamego.Context, l i18n.Locale, user *database.User, rc *repoAdminContext, req repoTransferRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !rc.viewerCanManage(user, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if !database.Handle.Users().IsUsernameUsed(c.Request().Context(), req.NewOwner, rc.Owner.ID) {
		msg := l.Tr("form.enterred_invalid_owner_name")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"newOwner": &msg}}, nil
	}

	if err = database.TransferOwnership(user, req.NewOwner, repo); err != nil {
		if database.IsErrRepoAlreadyExist(err) {
			msg := l.Tr("repo.settings.new_owner_has_same_repo")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"newOwner": &msg}}, nil
		}
		log.Error("postRepoTransfer: transfer repository %d to %q: %v", repo.ID, req.NewOwner, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "transfer ownership")
	}
	return http.StatusOK, &repoNameResponse{Owner: repo.Owner.Name, Name: repo.Name}, nil
}

// deleteRepo deletes the repository with everything in it.
func deleteRepo(l i18n.Locale, user *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !rc.viewerCanManage(user, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if err = database.DeleteRepository(rc.Owner.ID, repo.ID); err != nil {
		log.Error("deleteRepo: delete repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete repository")
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoWiki erases the wiki of the repository and turns it off.
func deleteRepoWiki(l i18n.Locale, user *database.User, rc *repoAdminContext, req repoConfirmRequest) (statusCode int, resp any, err error) {
	repo := rc.Repo
	if !rc.viewerCanManage(user, true) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if resp := confirmRepoName(l, repo, req.RepoName); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	repo.DeleteWiki()
	repo.EnableWiki = false
	if err = database.UpdateRepository(repo, false); err != nil {
		log.Error("deleteRepoWiki: update repository %d: %v", repo.ID, err)
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

func getRepoCollaborators(rc *repoAdminContext) (statusCode int, resp any, err error) {
	collaborators, err := rc.Repo.GetCollaborators()
	if err != nil {
		log.Error("getRepoCollaborators: list collaborators of repository %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get collaborators")
	}
	items := make([]repoCollaborator, 0, len(collaborators))
	for _, u := range collaborators {
		items = append(items, repoCollaborator{
			Name:      u.Name,
			FullName:  u.FullName,
			AvatarURL: tool.AppendAvatarSize(u.AvatarURLPath(), 80),
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
func postRepoCollaborator(c flamego.Context, l i18n.Locale, user *database.User, rc *repoAdminContext, req repoCollaboratorRequest) (statusCode int, resp any, err error) {
	if strings.EqualFold(req.Username, rc.Owner.Name) {
		return http.StatusNoContent, nil, nil
	}
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), req.Username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			msg := l.Tr("form.user_not_exist")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"username": &msg}}, nil
		}
		log.Error("postRepoCollaborator: get user %q: %v", req.Username, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user")
	}
	if u.IsOrganization() {
		msg := l.Tr("repo.settings.org_not_allowed_to_be_collaborator")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"username": &msg}}, nil
	}

	if err = rc.Repo.AddCollaborator(u); err != nil {
		log.Error("postRepoCollaborator: add user %d to repository %d: %v", u.ID, rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add collaborator")
	}
	if conf.User.EnableEmailNotification {
		if err := email.SendCollaboratorMail(database.NewMailerUser(u), database.NewMailerUser(user), database.NewMailerRepo(rc.Repo)); err != nil {
			log.Error("postRepoCollaborator: send collaborator mail to user %d: %v", u.ID, err)
		}
	}
	return http.StatusCreated, nil, nil
}

// repoCollaboratorUser returns the user named in the route, or the status
// code and the error to answer with.
func repoCollaboratorUser(c flamego.Context, caller string) (_ *database.User, statusCode int, err error) {
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, errors.New("user does not exist")
		}
		log.Error("%s: get user %q: %v", caller, c.Param("username"), err)
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user")
	}
	return u, http.StatusOK, nil
}

type repoCollaboratorModeRequest struct {
	Mode string `json:"mode" validate:"required,oneof=read write admin"`
}

// patchRepoCollaborator changes the access of a collaborator, up to the access
// of the viewer.
func patchRepoCollaborator(c flamego.Context, rc *repoAdminContext, req repoCollaboratorModeRequest) (statusCode int, resp any, err error) {
	u, code, err := repoCollaboratorUser(c, "patchRepoCollaborator")
	if err != nil {
		return code, nil, err
	}
	if err = rc.Repo.ChangeCollaborationAccessMode(rc.viewerAccess, u.ID, database.ParseAccessMode(req.Mode)); err != nil {
		log.Error("patchRepoCollaborator: change access of user %d to repository %d: %v", u.ID, rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change collaboration access mode")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoCollaborator(c flamego.Context, rc *repoAdminContext) (statusCode int, resp any, err error) {
	u, code, err := repoCollaboratorUser(c, "deleteRepoCollaborator")
	if err != nil {
		return code, nil, err
	}
	if err = rc.Repo.DeleteCollaboration(u.ID); err != nil {
		log.Error("deleteRepoCollaborator: remove user %d from repository %d: %v", u.ID, rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete collaboration")
	}
	return http.StatusNoContent, nil, nil
}

func getRepoDeployKeys(rc *repoAdminContext) (statusCode int, resp any, err error) {
	keys, err := database.ListDeployKeys(rc.Repo.ID)
	if err != nil {
		log.Error("getRepoDeployKeys: list deploy keys of repository %d: %v", rc.Repo.ID, err)
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
func postRepoDeployKey(l i18n.Locale, rc *repoAdminContext, req userSSHKeyRequest) (statusCode int, resp any, err error) {
	content, err := database.CheckPublicKeyString(req.Content)
	if err != nil {
		msg := l.Tr("form.invalid_ssh_key", err.Error())
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if _, err = database.AddDeployKey(rc.Repo.ID, req.SSHTitle, content); err != nil {
		var msg string
		switch {
		case database.IsErrKeyAlreadyExist(err), database.IsErrDeployKeyAlreadyExist(err):
			msg = l.Tr("repo.settings.key_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
		case database.IsErrDeployKeyNameAlreadyUsed(err):
			msg = l.Tr("repo.settings.key_name_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}, nil
		}
		log.Error("postRepoDeployKey: add deploy key to repository %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add deploy key")
	}
	return http.StatusCreated, nil, nil
}

// deleteRepoDeployKey deletes a deploy key of the repository. Keys of other
// repositories are not found.
func deleteRepoDeployKey(c flamego.Context, user *database.User, rc *repoAdminContext) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	key, err := database.GetDeployKeyByID(id)
	if err != nil {
		if database.IsErrDeployKeyNotExist(err) {
			return http.StatusNotFound, nil, errors.New("key not found")
		}
		log.Error("deleteRepoDeployKey: get deploy key %d: %v", id, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get deploy key")
	}
	if key.RepoID != rc.Repo.ID {
		return http.StatusNotFound, nil, errors.New("key not found")
	}
	if err = database.DeleteDeployKey(user, key.ID); err != nil {
		log.Error("deleteRepoDeployKey: delete deploy key %d: %v", key.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete deploy key")
	}
	return http.StatusNoContent, nil, nil
}
