package web

import (
	"net/http"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

type userSettings struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	Website  string `json:"website"`
	Location string `json:"location"`
	// IsLocal is false for users who sign in through an external
	// authentication source. They cannot change their username or password.
	IsLocal   bool   `json:"isLocal"`
	AvatarURL string `json:"avatarURL"`
	// HasCustomAvatar is true when an avatar was uploaded, which can be
	// deleted.
	HasCustomAvatar bool `json:"hasCustomAvatar"`
	// AvatarEmail is the email the avatar is looked up by on Gravatar. It is
	// only sent when the site uses Gravatar.
	AvatarEmail *string `json:"avatarEmail,omitempty"`
}

func getUserSettings(user *database.User) (statusCode int, resp *userSettings, err error) {
	settings := &userSettings{
		Name:            user.Name,
		FullName:        user.FullName,
		Website:         user.Website,
		Location:        user.Location,
		IsLocal:         user.IsLocal(),
		AvatarURL:       tool.AppendAvatarSize(user.AvatarURLPath(), 200),
		HasCustomAvatar: user.UseCustomAvatar,
	}
	if !conf.Picture.DisableGravatar {
		settings.AvatarEmail = &user.AvatarEmail
	}
	return http.StatusOK, settings, nil
}

type userSettingsRequest struct {
	UserName string `json:"name" validate:"required,alphadashdot,max=35"`
	FullName string `json:"fullName" validate:"max=100"`
	Website  string `json:"website" validate:"omitempty,url,max=100"`
	Location string `json:"location" validate:"max=50"`
}

type userNameResponse struct {
	Name string `json:"name"`
}

// patchUserSettings changes the profile of the signed-in user, and the
// username of local users.
func patchUserSettings(c flamego.Context, l i18n.Locale, user *database.User, req userSettingsRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	name := user.Name
	// Users from an external authentication source keep the name it gives them.
	if user.IsLocal() && user.Name != req.UserName {
		if err = database.Handle.Users().ChangeUsername(ctx, user.ID, req.UserName); err != nil {
			var msg string
			switch {
			case database.IsErrUserAlreadyExist(err):
				msg = l.Tr("form.username_been_taken")
				return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			case database.IsErrNameNotAllowed(err):
				msg = l.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
				return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			}
			log.Error("patchUserSettings: rename user %d to %q: %v", user.ID, req.UserName, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "rename user")
		}
		name = req.UserName
	}

	err = database.Handle.Users().Update(ctx, user.ID, database.UpdateUserOptions{
		FullName: &req.FullName,
		Website:  &req.Website,
		Location: &req.Location,
	})
	if err != nil {
		log.Error("patchUserSettings: update user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}
	return http.StatusOK, &userNameResponse{Name: name}, nil
}

func putUserAvatar(c flamego.Context, l i18n.Locale, user *database.User) (statusCode int, resp any, err error) {
	r := c.Request().Request
	data, code, err := readAvatarUpload(r, l)
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("putUserAvatar: user %d: %v", user.ID, err)
		}
		return code, nil, err
	}
	if err = database.Handle.Users().UseCustomAvatar(r.Context(), user.ID, data); err != nil {
		log.Error("putUserAvatar: save avatar of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save avatar")
	}
	return http.StatusNoContent, nil, nil
}

func deleteUserAvatar(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	if err = database.Handle.Users().DeleteCustomAvatar(c.Request().Context(), user.ID); err != nil {
		log.Error("deleteUserAvatar: delete avatar of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete avatar")
	}
	return http.StatusNoContent, nil, nil
}

type userGravatarRequest struct {
	AvatarEmail string `json:"email" validate:"required,email,max=254"`
}

// putUserGravatar shows the Gravatar of the given email as the avatar of the
// signed-in user, in place of any uploaded avatar.
func putUserGravatar(c flamego.Context, user *database.User, req userGravatarRequest) (statusCode int, resp any, err error) {
	if conf.Picture.DisableGravatar {
		return http.StatusNotFound, nil, errors.New("gravatar is disabled")
	}
	ctx := c.Request().Context()
	avatar := cryptox.MD5(req.AvatarEmail)
	err = database.Handle.Users().Update(ctx, user.ID, database.UpdateUserOptions{
		Avatar:      &avatar,
		AvatarEmail: &req.AvatarEmail,
	})
	if err != nil {
		log.Error("putUserGravatar: update user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}
	if err = database.Handle.Users().DeleteCustomAvatar(ctx, user.ID); err != nil {
		log.Error("putUserGravatar: delete avatar of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete avatar")
	}
	return http.StatusNoContent, nil, nil
}

type userPasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required,max=255"`
	Password    string `json:"password" validate:"required,max=255"`
	Retype      string `json:"retype"`
}

func putUserPassword(c flamego.Context, l i18n.Locale, user *database.User, req userPasswordRequest) (statusCode int, resp any, err error) {
	if !user.IsLocal() {
		return http.StatusForbidden, nil, errors.New(l.Tr("settings.password_change_disabled"))
	}
	if !userx.ValidatePassword(user.Password, user.Salt, req.OldPassword) {
		msg := l.Tr("settings.password_incorrect")
		return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"oldPassword": &msg}}, nil
	}
	if req.Password != req.Retype {
		msg := l.Tr("form.password_not_match")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"retype": &msg}}, nil
	}
	err = database.Handle.Users().Update(c.Request().Context(), user.ID, database.UpdateUserOptions{Password: &req.Password})
	if err != nil {
		log.Error("putUserPassword: update user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}
	return http.StatusNoContent, nil, nil
}

type userSettingsRepo struct {
	ID        int64  `json:"id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"isPrivate"`
	IsFork    bool   `json:"isFork"`
	IsMirror  bool   `json:"isMirror"`
	Size      string `json:"size"`
	// IsOwner is true for repositories of the user, which they cannot leave.
	IsOwner bool `json:"isOwner"`
}

type userSettingsRepos struct {
	Items []userSettingsRepo `json:"items"`
}

// getUserSettingsRepos lists the repositories of the signed-in user and the
// ones they collaborate on.
func getUserSettingsRepos(user *database.User) (statusCode int, resp *userSettingsRepos, err error) {
	repos, err := database.GetUserAndCollaborativeRepositories(user.ID)
	if err != nil {
		log.Error("getUserSettingsRepos: list repositories of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}
	if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
		log.Error("getUserSettingsRepos: load attributes: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load attributes")
	}
	items := make([]userSettingsRepo, 0, len(repos))
	for _, repo := range repos {
		items = append(items, userSettingsRepo{
			ID:        repo.ID,
			Owner:     repo.Owner.Name,
			Name:      repo.Name,
			IsPrivate: repo.IsPrivate,
			IsFork:    repo.IsFork,
			IsMirror:  repo.IsMirror,
			Size:      tool.FileSize(repo.Size),
			IsOwner:   repo.OwnerID == user.ID,
		})
	}
	return http.StatusOK, &userSettingsRepos{Items: items}, nil
}

// deleteUserSettingsRepo makes the signed-in user leave a repository they
// collaborate on.
func deleteUserSettingsRepo(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	repo, err := database.GetRepositoryByID(id)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository not found")
		}
		log.Error("deleteUserSettingsRepo: get repository %d: %v", id, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	// Answer the same for repositories the user does not collaborate on, so
	// the IDs of private repositories cannot be probed.
	if !database.IsCollaborator(repo.ID, user.ID) {
		return http.StatusNotFound, nil, errors.New("repository not found")
	}
	if err = repo.DeleteCollaboration(user.ID); err != nil {
		log.Error("deleteUserSettingsRepo: remove user %d from repository %d: %v", user.ID, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete collaboration")
	}
	return http.StatusNoContent, nil, nil
}

type userSettingsOrg struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
}

type userSettingsOrgs struct {
	Items []userSettingsOrg `json:"items"`
}

// getUserSettingsOrgs lists every organization the signed-in user is a
// member of, including the ones their membership is private in.
func getUserSettingsOrgs(user *database.User) (statusCode int, resp *userSettingsOrgs, err error) {
	orgs, err := database.GetOrgsByUserID(user.ID, true)
	if err != nil {
		log.Error("getUserSettingsOrgs: list organizations of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list organizations")
	}
	items := make([]userSettingsOrg, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, userSettingsOrg{Name: org.Name, AvatarURL: org.AvatarURLPath()})
	}
	return http.StatusOK, &userSettingsOrgs{Items: items}, nil
}

type userDeleteRequest struct {
	Password string `json:"password" validate:"required"`
}

// deleteUser deletes the account of the signed-in user once they confirm
// their password. Users who still own repositories or belong to
// organizations cannot be deleted.
func deleteUser(c flamego.Context, l i18n.Locale, user *database.User, req userDeleteRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	_, err = database.Handle.Users().Authenticate(ctx, user.Name, req.Password, user.LoginSource)
	if err != nil {
		if auth.IsErrBadCredentials(err) {
			msg := l.Tr("form.enterred_invalid_password")
			return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"password": &msg}}, nil
		}
		log.Error("deleteUser: authenticate user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
	}
	if err = database.Handle.Users().DeleteByID(ctx, user.ID, false); err != nil {
		switch {
		case database.IsErrUserOwnRepos(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("form.still_own_repo")}, nil
		case database.IsErrUserHasOrgs(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("form.still_has_org")}, nil
		}
		log.Error("deleteUser: delete user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete user")
	}
	log.Trace("Account deleted: %s", user.Name)
	return http.StatusNoContent, nil, nil
}
