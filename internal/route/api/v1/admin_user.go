package v1

import (
	"net/http"

	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
)

func parseLoginSource(c *apiContext, sourceID int64) {
	if sourceID == 0 {
		return
	}

	_, err := database.Handle.LoginSources().GetByID(c.Request().Context(), sourceID)
	if err != nil {
		if database.IsErrLoginSourceNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get login source by ID")
		}
		return
	}
}

type adminCreateUserRequest struct {
	SourceID   int64  `json:"source_id"`
	LoginName  string `json:"login_name"`
	Username   string `json:"username" validate:"required,alphadashdot,max=35"`
	FullName   string `json:"full_name" validate:"max=100"`
	Email      string `json:"email" validate:"required,email,max=254"`
	Password   string `json:"password" validate:"max=255"`
	SendNotify bool   `json:"send_notify"`
}

func adminCreateUser(c *apiContext, l i18n.Locale, form adminCreateUserRequest) {
	parseLoginSource(c, form.SourceID)
	if c.Written() {
		return
	}

	u, err := database.Handle.Users().Create(
		c.Request().Context(),
		form.Username,
		form.Email,
		database.CreateUserOptions{
			FullName:    form.FullName,
			Password:    form.Password,
			LoginSource: form.SourceID,
			LoginName:   form.LoginName,
			Activated:   true,
		},
	)
	if err != nil {
		if database.IsErrUserAlreadyExist(err) ||
			database.IsErrEmailAlreadyUsed(err) ||
			database.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "create user")
		}
		return
	}
	log.Trace("Account %q created by admin %q", u.Name, c.User.Name)

	// Send email notification.
	if form.SendNotify && conf.Email.Enabled {
		if err := email.SendRegisterNotifyMail(l, database.NewMailerUser(u)); err != nil {
			log.Error("Failed to send register notify mail: %v", err)
		}
	}

	c.JSON(http.StatusCreated, toUser(u))
}

type adminEditUserRequest struct {
	SourceID         int64  `json:"source_id"`
	LoginName        string `json:"login_name"`
	FullName         string `json:"full_name" validate:"max=100"`
	Email            string `json:"email" validate:"required,email,max=254"`
	Password         string `json:"password" validate:"max=255"`
	Website          string `json:"website" validate:"max=50"`
	Location         string `json:"location" validate:"max=50"`
	Active           *bool  `json:"active"`
	Admin            *bool  `json:"admin"`
	AllowGitHook     *bool  `json:"allow_git_hook"`
	AllowImportLocal *bool  `json:"allow_import_local"`
	MaxRepoCreation  *int   `json:"max_repo_creation"`
}

func adminEditUser(c *apiContext, form adminEditUserRequest) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}

	parseLoginSource(c, form.SourceID)
	if c.Written() {
		return
	}

	opts := database.UpdateUserOptions{
		LoginSource:      &form.SourceID,
		LoginName:        &form.LoginName,
		FullName:         &form.FullName,
		Website:          &form.Website,
		Location:         &form.Location,
		MaxRepoCreation:  form.MaxRepoCreation,
		IsActivated:      form.Active,
		IsAdmin:          form.Admin,
		AllowGitHook:     form.AllowGitHook,
		AllowImportLocal: form.AllowImportLocal,
		ProhibitLogin:    nil, // TODO: Add this option to API
	}

	if form.Password != "" {
		opts.Password = &form.Password
	}

	if u.Email != form.Email {
		opts.Email = &form.Email
	}

	err := database.Handle.Users().Update(c.Request().Context(), u.ID, opts)
	if err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "update user")
		}
		return
	}
	log.Trace("Account updated by admin %q: %s", c.User.Name, u.Name)

	u, err = database.Handle.Users().GetByID(c.Request().Context(), u.ID)
	if err != nil {
		c.Error(err, "get user")
		return
	}
	c.JSONSuccess(toUser(u))
}

func adminDeleteUser(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}

	if err := database.Handle.Users().DeleteByID(c.Request().Context(), u.ID, false); err != nil {
		if database.IsErrUserOwnRepos(err) ||
			database.IsErrUserHasOrgs(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "delete user")
		}
		return
	}
	log.Trace("Account deleted by admin(%s): %s", c.User.Name, u.Name)

	c.NoContent()
}

func adminCreatePublicKey(c *apiContext, form createPublicKeyRequest) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	createUserPublicKey(c, form, u.ID)
}
