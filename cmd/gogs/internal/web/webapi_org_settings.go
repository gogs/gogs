package web

import (
	"io"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
)

type orgCreateRequest struct {
	OrgName string `json:"orgName" validate:"required,alphadashdot,max=35"`
}

type orgNameResponse struct {
	Name string `json:"name"`
}

// orgNameError returns the response for an organization name that cannot be
// used, or nil when err is about something else.
func orgNameError(l i18n.Locale, field string, err error) (int, *bindingErrorResponse) {
	var code int
	var msg string
	switch {
	case database.IsErrUserAlreadyExist(err):
		code, msg = http.StatusUnprocessableEntity, l.Tr("form.org_name_been_taken")
	case database.IsErrNameNotAllowed(err):
		code, msg = http.StatusBadRequest, l.Tr("org.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
	default:
		return 0, nil
	}
	return code, &bindingErrorResponse{Fields: fieldErrors{field: &msg}}
}

func postOrg(l i18n.Locale, user *database.User, req orgCreateRequest) (statusCode int, resp any, err error) {
	if !user.CanCreateOrganization() {
		return http.StatusForbidden, nil, errors.New("not allowed to create organizations")
	}
	org := &database.User{
		Name:     req.OrgName,
		IsActive: true,
		Type:     database.UserTypeOrganization,
	}
	if err = database.CreateOrganization(org, user); err != nil {
		if code, resp := orgNameError(l, "orgName", err); resp != nil {
			return code, resp, nil
		}
		log.Error("postOrg: create organization %q: %v", req.OrgName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create organization")
	}
	return http.StatusCreated, &orgNameResponse{Name: org.Name}, nil
}

type orgSettings struct {
	Name        string `json:"name"`
	FullName    string `json:"fullName"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
	AvatarURL   string `json:"avatarURL"`
	// HasCustomAvatar is true when an avatar was uploaded, which can be
	// deleted.
	HasCustomAvatar bool `json:"hasCustomAvatar"`
	// MaxRepoCreation is only sent to site admins, who can change it.
	MaxRepoCreation *int `json:"maxRepoCreation,omitempty"`
}

func getOrgSettings(user *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	org := m.Org
	settings := &orgSettings{
		Name:            org.Name,
		FullName:        org.FullName,
		Description:     org.Description,
		Website:         org.Website,
		Location:        org.Location,
		AvatarURL:       tool.AppendAvatarSize(org.AvatarURLPath(), 200),
		HasCustomAvatar: org.UseCustomAvatar,
	}
	if user.IsAdmin {
		settings.MaxRepoCreation = &org.MaxRepoCreation
	}
	return http.StatusOK, settings, nil
}

type orgSettingsRequest struct {
	OrgName     string `json:"name" validate:"required,alphadashdot,max=35"`
	FullName    string `json:"fullName" validate:"max=100"`
	Description string `json:"description" validate:"max=255"`
	Website     string `json:"website" validate:"omitempty,url,max=100"`
	Location    string `json:"location" validate:"max=50"`
	// MaxRepoCreation is only changed for site admins.
	MaxRepoCreation int `json:"maxRepoCreation"`
}

// patchOrgSettings changes the name and the profile of the organization, and
// for site admins, how many repositories it can create.
func patchOrgSettings(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership, req orgSettingsRequest) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	ctx := c.Request().Context()
	org := m.Org
	if org.Name != req.OrgName {
		if err = database.Handle.Users().ChangeUsername(ctx, org.ID, req.OrgName); err != nil {
			if code, resp := orgNameError(l, "name", err); resp != nil {
				return code, resp, nil
			}
			log.Error("patchOrgSettings: rename organization %d to %q: %v", org.ID, req.OrgName, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "rename organization")
		}
	}

	opts := database.UpdateUserOptions{
		FullName:    &req.FullName,
		Website:     &req.Website,
		Location:    &req.Location,
		Description: &req.Description,
	}
	if user.IsAdmin {
		opts.MaxRepoCreation = &req.MaxRepoCreation
	}
	if err = database.Handle.Users().Update(ctx, org.ID, opts); err != nil {
		log.Error("patchOrgSettings: update organization %d: %v", org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update organization")
	}
	return http.StatusOK, &orgNameResponse{Name: req.OrgName}, nil
}

// avatarMaxSize is the largest avatar image users and organization owners can
// upload. The image is resized once saved.
const avatarMaxSize = 5 << 20 // 5 MiB

// limitAvatarUpload replaces the size limit of other requests, which is too
// small for an image.
func limitAvatarUpload(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, avatarMaxSize+64<<10) // Room for the multipart framing.
}

// readAvatarUpload returns the image uploaded as the "avatar" form field, or
// the status code and the error to answer with.
func readAvatarUpload(r *http.Request, l i18n.Locale) (data []byte, statusCode int, err error) {
	file, _, err := r.FormFile("avatar")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, http.StatusRequestEntityTooLarge, errors.New(l.Tr("settings.uploaded_avatar_too_large", avatarMaxSize>>20))
		}
		return nil, http.StatusBadRequest, errors.New(l.Tr("settings.uploaded_avatar_not_a_image"))
	}
	defer func() { _ = file.Close() }()

	data, err = io.ReadAll(io.LimitReader(file, avatarMaxSize+1))
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrap(err, "read avatar")
	}
	if len(data) > avatarMaxSize {
		return nil, http.StatusRequestEntityTooLarge, errors.New(l.Tr("settings.uploaded_avatar_too_large", avatarMaxSize>>20))
	}
	if !tool.IsImageFile(data) {
		return nil, http.StatusBadRequest, errors.New(l.Tr("settings.uploaded_avatar_not_a_image"))
	}
	return data, http.StatusOK, nil
}

func putOrgAvatar(c flamego.Context, l i18n.Locale, m *orgMembership) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	r := c.Request().Request
	data, code, err := readAvatarUpload(r, l)
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("putOrgAvatar: organization %d: %v", m.Org.ID, err)
		}
		return code, nil, err
	}
	if err = database.Handle.Users().UseCustomAvatar(r.Context(), m.Org.ID, data); err != nil {
		log.Error("putOrgAvatar: save avatar of organization %d: %v", m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save avatar")
	}
	return http.StatusNoContent, nil, nil
}

func deleteOrgAvatar(c flamego.Context, m *orgMembership) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = database.Handle.Users().DeleteCustomAvatar(c.Request().Context(), m.Org.ID); err != nil {
		log.Error("deleteOrgAvatar: delete avatar of organization %d: %v", m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete avatar")
	}
	return http.StatusNoContent, nil, nil
}

type orgDeleteRequest struct {
	Password string `json:"password" validate:"required"`
}

// deleteOrg deletes the organization once the owner confirms their password.
// Organizations that still own repositories cannot be deleted.
func deleteOrg(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership, req orgDeleteRequest) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	_, err = database.Handle.Users().Authenticate(c.Request().Context(), user.Name, req.Password, user.LoginSource)
	if err != nil {
		if auth.IsErrBadCredentials(err) {
			msg := l.Tr("form.enterred_invalid_password")
			return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"password": &msg}}, nil
		}
		log.Error("deleteOrg: authenticate user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
	}
	if err = database.DeleteOrganization(m.Org); err != nil {
		if database.IsErrUserOwnRepos(err) {
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("form.org_still_own_repo")}, nil
		}
		log.Error("deleteOrg: delete organization %d: %v", m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete organization")
	}
	return http.StatusNoContent, nil, nil
}
