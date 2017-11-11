// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"strings"

	"github.com/Unknwon/com"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/mailer"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/routes"
)

const (
	USERS     = "admin/user/list"
	USER_NEW  = "admin/user/new"
	USER_EDIT = "admin/user/edit"
)

func Users(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.users")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminUsers"] = true

	routes.RenderUserSearch(c, &routes.UserSearchOptions{
		Type:     models.USER_TYPE_INDIVIDUAL,
		Counter:  models.CountUsers,
		Ranger:   models.Users,
		PageSize: setting.UI.Admin.UserPagingNum,
		OrderBy:  "id ASC",
		TplName:  USERS,
	})
}

func NewUser(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.users.new_account")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminUsers"] = true

	c.Data["login_type"] = "0-0"

	sources, err := models.LoginSources()
	if err != nil {
		c.Handle(500, "LoginSources", err)
		return
	}
	c.Data["Sources"] = sources

	c.Data["CanSendEmail"] = setting.MailService != nil
	c.HTML(200, USER_NEW)
}

func NewUserPost(c *context.Context, f form.AdminCrateUser) {
	c.Data["Title"] = c.Tr("admin.users.new_account")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminUsers"] = true

	sources, err := models.LoginSources()
	if err != nil {
		c.Handle(500, "LoginSources", err)
		return
	}
	c.Data["Sources"] = sources

	c.Data["CanSendEmail"] = setting.MailService != nil

	if c.HasError() {
		c.HTML(200, USER_NEW)
		return
	}

	u := &models.User{
		Name:      f.UserName,
		Email:     f.Email,
		Passwd:    f.Password,
		IsActive:  true,
		LoginType: models.LOGIN_PLAIN,
	}

	if len(f.LoginType) > 0 {
		fields := strings.Split(f.LoginType, "-")
		if len(fields) == 2 {
			u.LoginType = models.LoginType(com.StrTo(fields[0]).MustInt())
			u.LoginSource = com.StrTo(fields[1]).MustInt64()
			u.LoginName = f.LoginName
		}
	}

	if err := models.CreateUser(u); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			c.Data["Err_UserName"] = true
			c.RenderWithErr(c.Tr("form.username_been_taken"), USER_NEW, &f)
		case models.IsErrEmailAlreadyUsed(err):
			c.Data["Err_Email"] = true
			c.RenderWithErr(c.Tr("form.email_been_used"), USER_NEW, &f)
		case models.IsErrNameReserved(err):
			c.Data["Err_UserName"] = true
			c.RenderWithErr(c.Tr("user.form.name_reserved", err.(models.ErrNameReserved).Name), USER_NEW, &f)
		case models.IsErrNamePatternNotAllowed(err):
			c.Data["Err_UserName"] = true
			c.RenderWithErr(c.Tr("user.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), USER_NEW, &f)
		default:
			c.Handle(500, "CreateUser", err)
		}
		return
	}
	log.Trace("Account created by admin (%s): %s", c.User.Name, u.Name)

	// Send email notification.
	if f.SendNotify && setting.MailService != nil {
		mailer.SendRegisterNotifyMail(c.Context, models.NewMailerUser(u))
	}

	c.Flash.Success(c.Tr("admin.users.new_success", u.Name))
	c.Redirect(setting.AppSubURL + "/admin/users/" + com.ToStr(u.ID))
}

func prepareUserInfo(c *context.Context) *models.User {
	u, err := models.GetUserByID(c.ParamsInt64(":userid"))
	if err != nil {
		c.Handle(500, "GetUserByID", err)
		return nil
	}
	c.Data["User"] = u

	if u.LoginSource > 0 {
		c.Data["LoginSource"], err = models.GetLoginSourceByID(u.LoginSource)
		if err != nil {
			c.Handle(500, "GetLoginSourceByID", err)
			return nil
		}
	} else {
		c.Data["LoginSource"] = &models.LoginSource{}
	}

	sources, err := models.LoginSources()
	if err != nil {
		c.Handle(500, "LoginSources", err)
		return nil
	}
	c.Data["Sources"] = sources

	return u
}

func EditUser(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.users.edit_account")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminUsers"] = true
	c.Data["EnableLocalPathMigration"] = setting.Repository.EnableLocalPathMigration

	prepareUserInfo(c)
	if c.Written() {
		return
	}

	c.HTML(200, USER_EDIT)
}

func EditUserPost(c *context.Context, f form.AdminEditUser) {
	c.Data["Title"] = c.Tr("admin.users.edit_account")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminUsers"] = true
	c.Data["EnableLocalPathMigration"] = setting.Repository.EnableLocalPathMigration

	u := prepareUserInfo(c)
	if c.Written() {
		return
	}

	if c.HasError() {
		c.HTML(200, USER_EDIT)
		return
	}

	fields := strings.Split(f.LoginType, "-")
	if len(fields) == 2 {
		loginType := models.LoginType(com.StrTo(fields[0]).MustInt())
		loginSource := com.StrTo(fields[1]).MustInt64()

		if u.LoginSource != loginSource {
			u.LoginSource = loginSource
			u.LoginType = loginType
		}
	}

	if len(f.Password) > 0 {
		u.Passwd = f.Password
		var err error
		if u.Salt, err = models.GetUserSalt(); err != nil {
			c.Handle(500, "UpdateUser", err)
			return
		}
		u.EncodePasswd()
	}

	u.LoginName = f.LoginName
	u.FullName = f.FullName
	u.Email = f.Email
	u.Website = f.Website
	u.Location = f.Location
	u.MaxRepoCreation = f.MaxRepoCreation
	u.IsActive = f.Active
	u.IsAdmin = f.Admin
	u.AllowGitHook = f.AllowGitHook
	u.AllowImportLocal = f.AllowImportLocal
	u.ProhibitLogin = f.ProhibitLogin

	if err := models.UpdateUser(u); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			c.Data["Err_Email"] = true
			c.RenderWithErr(c.Tr("form.email_been_used"), USER_EDIT, &f)
		} else {
			c.Handle(500, "UpdateUser", err)
		}
		return
	}
	log.Trace("Account profile updated by admin (%s): %s", c.User.Name, u.Name)

	c.Flash.Success(c.Tr("admin.users.update_profile_success"))
	c.Redirect(setting.AppSubURL + "/admin/users/" + c.Params(":userid"))
}

func DeleteUser(c *context.Context) {
	u, err := models.GetUserByID(c.ParamsInt64(":userid"))
	if err != nil {
		c.Handle(500, "GetUserByID", err)
		return
	}

	if err = models.DeleteUser(u); err != nil {
		switch {
		case models.IsErrUserOwnRepos(err):
			c.Flash.Error(c.Tr("admin.users.still_own_repo"))
			c.JSON(200, map[string]interface{}{
				"redirect": setting.AppSubURL + "/admin/users/" + c.Params(":userid"),
			})
		case models.IsErrUserHasOrgs(err):
			c.Flash.Error(c.Tr("admin.users.still_has_org"))
			c.JSON(200, map[string]interface{}{
				"redirect": setting.AppSubURL + "/admin/users/" + c.Params(":userid"),
			})
		default:
			c.Handle(500, "DeleteUser", err)
		}
		return
	}
	log.Trace("Account deleted by admin (%s): %s", c.User.Name, u.Name)

	c.Flash.Success(c.Tr("admin.users.deletion_success"))
	c.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubURL + "/admin/users",
	})
}
