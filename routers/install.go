// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/macaron"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/cron"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/social"
)

const (
	INSTALL base.TplName = "install"
)

func checkRunMode() {
	switch setting.Cfg.Section("").Key("RUN_MODE").String() {
	case "prod":
		macaron.Env = macaron.PROD
		setting.ProdMode = true
	case "test":
		macaron.Env = macaron.TEST
	}
	log.Info("Run Mode: %s", strings.Title(macaron.Env))
}

func NewServices() {
	setting.NewServices()
	social.NewOauthService()
}

// GlobalInit is for global configuration reload-able.
func GlobalInit() {
	setting.NewConfigContext()
	log.Trace("Custom path: %s", setting.CustomPath)
	log.Trace("Log path: %s", setting.LogRootPath)
	mailer.NewMailerContext()
	models.LoadModelsConfig()
	NewServices()

	if setting.InstallLock {
		models.LoadRepoConfig()
		models.NewRepoContext()

		if err := models.NewEngine(); err != nil {
			log.Fatal(4, "Fail to initialize ORM engine: %v", err)
		}

		models.HasEngine = true
		cron.NewCronContext()
		log.NewGitLogger(path.Join(setting.LogRootPath, "http.log"))
	}
	if models.EnableSQLite3 {
		log.Info("SQLite3 Enabled")
	}
	checkRunMode()
}

func renderDbOption(ctx *middleware.Context) {
	ctx.Data["DbOptions"] = []string{"MySQL", "PostgreSQL", "SQLite3"}
}

// @router /install [get]
func Install(ctx *middleware.Context, form auth.InstallForm) {
	if setting.InstallLock {
		ctx.Handle(404, "Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = ctx.Tr("install.install")
	ctx.Data["PageIsInstall"] = true

	// FIXME: when i'm ckeching length here? should they all be 0 no matter when?
	// Get and assign values to install form.
	if len(form.DbHost) == 0 {
		form.DbHost = models.DbCfg.Host
	}
	if len(form.DbUser) == 0 {
		form.DbUser = models.DbCfg.User
	}
	if len(form.DbPasswd) == 0 {
		form.DbPasswd = models.DbCfg.Pwd
	}
	if len(form.DatabaseName) == 0 {
		form.DatabaseName = models.DbCfg.Name
	}
	if len(form.DatabasePath) == 0 {
		form.DatabasePath = models.DbCfg.Path
	}

	if len(form.RepoRootPath) == 0 {
		form.RepoRootPath = setting.RepoRootPath
	}
	if len(form.RunUser) == 0 {
		// Note: it's not normall to use SSH in windows so current user can be first option(not git).
		if setting.IsWindows && setting.RunUser == "git" {
			form.RunUser = os.Getenv("USER")
			if len(form.RunUser) == 0 {
				form.RunUser = os.Getenv("USERNAME")
			}
		} else {
			form.RunUser = setting.RunUser
		}
	}
	if len(form.Domain) == 0 {
		form.Domain = setting.Domain
	}
	if len(form.AppUrl) == 0 {
		form.AppUrl = setting.AppUrl
	}

	renderDbOption(ctx)
	curDbOp := ""
	if models.EnableSQLite3 {
		curDbOp = "SQLite3" // Default when enabled.
	}
	ctx.Data["CurDbOption"] = curDbOp

	auth.AssignForm(form, ctx.Data)
	ctx.HTML(200, INSTALL)
}

func InstallPost(ctx *middleware.Context, form auth.InstallForm) {
	if setting.InstallLock {
		ctx.Handle(404, "InstallPost", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = ctx.Tr("install.install")
	ctx.Data["PageIsInstall"] = true

	renderDbOption(ctx)
	ctx.Data["CurDbOption"] = form.Database

	if ctx.HasError() {
		ctx.HTML(200, INSTALL)
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		ctx.RenderWithErr(ctx.Tr("install.test_git_failed", err), INSTALL, &form)
		return
	}

	// Pass basic check, now test configuration.
	// Test database setting.
	dbTypes := map[string]string{"MySQL": "mysql", "PostgreSQL": "postgres", "SQLite3": "sqlite3"}
	models.DbCfg.Type = dbTypes[form.Database]
	models.DbCfg.Host = form.DbHost
	models.DbCfg.User = form.DbUser
	models.DbCfg.Pwd = form.DbPasswd
	models.DbCfg.Name = form.DatabaseName
	models.DbCfg.SslMode = form.SslMode
	models.DbCfg.Path = form.DatabasePath

	// Set test engine.
	var x *xorm.Engine
	if err := models.NewTestEngine(x); err != nil {
		// FIXME: should use core.QueryDriver (github.com/go-xorm/core)
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.RenderWithErr(ctx.Tr("install.sqlite3_not_available", "http://gogs.io/docs/installation/install_from_binary.html"), INSTALL, &form)
		} else {
			ctx.RenderWithErr(ctx.Tr("install.invalid_db_setting", err), INSTALL, &form)
		}
		return
	}

	// Test repository root path.
	if err := os.MkdirAll(form.RepoRootPath, os.ModePerm); err != nil {
		ctx.Data["Err_RepoRootPath"] = true
		ctx.RenderWithErr(ctx.Tr("install.invalid_repo_path", err), INSTALL, &form)
		return
	}

	// Check run user.
	curUser := os.Getenv("USER")
	if len(curUser) == 0 {
		curUser = os.Getenv("USERNAME")
	}
	// Does not check run user when the install lock is off.
	if form.RunUser != curUser {
		ctx.Data["Err_RunUser"] = true
		ctx.RenderWithErr(ctx.Tr("install.run_user_not_match", form.RunUser, curUser), INSTALL, &form)
		return
	}

	// Check admin password.
	if form.AdminPasswd != form.ConfirmPasswd {
		ctx.Data["Err_AdminPasswd"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), INSTALL, form)
		return
	}

	// Save settings.
	setting.Cfg.Section("database").Key("DB_TYPE").SetValue(models.DbCfg.Type)
	setting.Cfg.Section("database").Key("HOST").SetValue(models.DbCfg.Host)
	setting.Cfg.Section("database").Key("NAME").SetValue(models.DbCfg.Name)
	setting.Cfg.Section("database").Key("USER").SetValue(models.DbCfg.User)
	setting.Cfg.Section("database").Key("PASSWD").SetValue(models.DbCfg.Pwd)
	setting.Cfg.Section("database").Key("SSL_MODE").SetValue(models.DbCfg.SslMode)
	setting.Cfg.Section("database").Key("PATH").SetValue(models.DbCfg.Path)

	setting.Cfg.Section("repository").Key("ROOT").SetValue(form.RepoRootPath)
	setting.Cfg.Section("").Key("RUN_USER").SetValue(form.RunUser)
	setting.Cfg.Section("server").Key("DOMAIN").SetValue(form.Domain)
	setting.Cfg.Section("server").Key("ROOT_URL").SetValue(form.AppUrl)

	if len(strings.TrimSpace(form.SmtpHost)) > 0 {
		setting.Cfg.Section("mailer").Key("ENABLED").SetValue("true")
		setting.Cfg.Section("mailer").Key("HOST").SetValue(form.SmtpHost)
		setting.Cfg.Section("mailer").Key("USER").SetValue(form.SmtpEmail)
		setting.Cfg.Section("mailer").Key("PASSWD").SetValue(form.SmtpPasswd)

		setting.Cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").SetValue(com.ToStr(form.RegisterConfirm == "on"))
		setting.Cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").SetValue(com.ToStr(form.MailNotify == "on"))
	}

	setting.Cfg.Section("").Key("RUN_MODE").SetValue("prod")

	setting.Cfg.Section("session").Key("PROVIDER").SetValue("file")

	setting.Cfg.Section("log").Key("MODE").SetValue("file")

	setting.Cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	setting.Cfg.Section("security").Key("SECRET_KEY").SetValue(base.GetRandomString(15))

	os.MkdirAll("custom/conf", os.ModePerm)
	if err := setting.Cfg.SaveTo(path.Join(setting.CustomPath, "conf/app.ini")); err != nil {
		ctx.RenderWithErr(ctx.Tr("install.save_config_failed", err), INSTALL, &form)
		return
	}

	GlobalInit()

	// Create admin account.
	if err := models.CreateUser(&models.User{Name: form.AdminName, Email: form.AdminEmail, Passwd: form.AdminPasswd,
		IsAdmin: true, IsActive: true}); err != nil {
		if err != models.ErrUserAlreadyExist {
			setting.InstallLock = false
			ctx.Data["Err_AdminName"] = true
			ctx.Data["Err_AdminEmail"] = true
			ctx.RenderWithErr(ctx.Tr("install.invalid_admin_setting", err), INSTALL, &form)
			return
		}
		log.Info("Admin account already exist")
	}

	log.Info("First-time run install finished!")
	ctx.Flash.Success(ctx.Tr("install.install_success"))
	ctx.Redirect(setting.AppSubUrl + "/user/login")
}
