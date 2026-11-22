package web

import (
	"net/http"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/osx"
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
	// HasCustomAvatar is true when the avatar file exists, which can be deleted.
	// It may also hold an identicon saved by an earlier version, or an upload
	// that an earlier version stopped showing without deleting it.
	HasCustomAvatar bool `json:"hasCustomAvatar"`
}

func getUserSettings(viewer *database.User) (statusCode int, resp *userSettings, err error) {
	settings := &userSettings{
		Name:            viewer.Name,
		FullName:        viewer.FullName,
		Website:         viewer.Website,
		Location:        viewer.Location,
		IsLocal:         viewer.IsLocal(),
		AvatarURL:       viewer.AvatarURLPath(),
		HasCustomAvatar: osx.IsFile(userx.CustomAvatarPath(viewer.ID)),
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

// patchUserSettings changes the profile of the authenticated user, and the
// username of local users.
func patchUserSettings(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req userSettingsRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	name := viewer.Name
	// Users from an external authentication source keep the name it gives them.
	if viewer.IsLocal() && viewer.Name != req.UserName {
		if err = database.Handle.Users().ChangeUsername(ctx, viewer.ID, req.UserName); err != nil {
			var msg string
			switch {
			case database.IsErrUserAlreadyExist(err):
				msg = l.Tr("form::username_been_taken")
				return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			case database.IsErrNameNotAllowed(err):
				msg = l.Tr("user::form.name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value())
				return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
			}
			logger.Error("Failed to rename user", "userID", viewer.ID, "newName", req.UserName, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "rename user")
		}
		name = req.UserName
	}

	err = database.Handle.Users().Update(ctx, viewer.ID, database.UpdateUserOptions{
		FullName: &req.FullName,
		Website:  &req.Website,
		Location: &req.Location,
	})
	if err != nil {
		logger.Error("Failed to update user", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}
	return http.StatusOK, &userNameResponse{Name: name}, nil
}

func putUserAvatar(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User) (statusCode int, resp any, err error) {
	r := c.Request().Request
	data, code, err := readAvatarUpload(r, l)
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to read avatar upload", "userID", viewer.ID, "error", err)
		}
		return code, nil, err
	}
	if err = database.Handle.Users().UseCustomAvatar(r.Context(), viewer.ID, data); err != nil {
		logger.Error("Failed to save avatar", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save avatar")
	}
	return http.StatusNoContent, nil, nil
}

func deleteUserAvatar(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	if err = database.Handle.Users().DeleteCustomAvatar(c.Request().Context(), viewer.ID); err != nil {
		logger.Error("Failed to delete avatar", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete avatar")
	}
	return http.StatusNoContent, nil, nil
}

type userPasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required,max=255"`
	Password    string `json:"password" validate:"required,max=255"`
	Retype      string `json:"retype"`
}

func putUserPassword(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req userPasswordRequest) (statusCode int, resp any, err error) {
	if !viewer.IsLocal() {
		return http.StatusForbidden, nil, errors.New(l.Tr("settings::password_change_disabled"))
	}
	if !userx.ValidatePassword(viewer.Password, viewer.Salt, req.OldPassword) {
		msg := l.Tr("settings::password_incorrect")
		return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"oldPassword": &msg}}, nil
	}
	if req.Password != req.Retype {
		msg := l.Tr("form::password_not_match")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"retype": &msg}}, nil
	}
	err = database.Handle.Users().Update(c.Request().Context(), viewer.ID, database.UpdateUserOptions{Password: &req.Password})
	if err != nil {
		logger.Error("Failed to update user", "userID", viewer.ID, "error", err)
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

// getUserSettingsRepos lists the repositories of the authenticated user and the
// ones they collaborate on.
func getUserSettingsRepos(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userSettingsRepos, err error) {
	repos, err := database.GetUserAndCollaborativeRepositories(viewer.ID)
	if err != nil {
		logger.Error("Failed to list repositories", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}
	if err = database.Handle.Repositories().LoadAttributes(c.Request().Context(), repos...); err != nil {
		logger.Error("Failed to load repository attributes", "error", err)
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
			Size:      fileSize(repo.Size),
			IsOwner:   repo.OwnerID == viewer.ID,
		})
	}
	return http.StatusOK, &userSettingsRepos{Items: items}, nil
}

// deleteUserSettingsRepo makes the authenticated user leave a repository they
// collaborate on.
func deleteUserSettingsRepo(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	repos := database.Handle.Repositories()
	repo, err := repos.GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository not found")
		}
		logger.Error("Failed to get repository", "repoID", id, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = repos.LoadAttributes(c.Request().Context(), repo); err != nil {
		logger.Error("Failed to load repository attributes", "repoID", id, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load repository attributes")
	}
	// Answer the same for repositories the user does not collaborate on, so
	// the IDs of private repositories cannot be probed.
	if !database.Handle.Collaborations().IsCollaborator(c.Request().Context(), logger, repo.ID, viewer.ID) {
		return http.StatusNotFound, nil, errors.New("repository not found")
	}
	if err = database.Handle.Collaborations().Delete(c.Request().Context(), repo, viewer.ID); err != nil {
		logger.Error("Failed to delete collaboration", "userID", viewer.ID, "repoID", repo.ID, "error", err)
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

// getUserSettingsOrgs lists every organization the authenticated user is a
// member of, including the ones their membership is private in.
func getUserSettingsOrgs(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userSettingsOrgs, err error) {
	ctx := c.Request().Context()
	orgs, err := database.Handle.Organizations().List(ctx, database.ListOrgsOptions{MemberID: viewer.ID, IncludePrivateMembers: true})
	if err != nil {
		logger.Error("Failed to list organizations", "userID", viewer.ID, "error", err)
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

// deleteUser deletes the account of the authenticated user once they confirm
// their password. Users who still own repositories or belong to
// organizations cannot be deleted.
func deleteUser(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req userDeleteRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	_, err = database.Handle.Users().Authenticate(ctx, logger, viewer.Name, req.Password, viewer.LoginSource)
	if err != nil {
		if auth.IsErrBadCredentials(err) {
			msg := l.Tr("form::enterred_invalid_password")
			return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"password": &msg}}, nil
		}
		logger.Error("Failed to authenticate user", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
	}
	if err = database.Handle.Users().DeleteByID(ctx, viewer.ID, false); err != nil {
		switch {
		case database.IsErrUserOwnRepos(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("form::still_own_repo")}, nil
		case database.IsErrUserHasOrgs(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("form::still_has_org")}, nil
		}
		logger.Error("Failed to delete user", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete user")
	}
	logger.Debug("Account deleted", "userName", viewer.Name)
	return http.StatusNoContent, nil, nil
}
