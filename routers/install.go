// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/macaron"
	"github.com/go-xorm/xorm"
	"gopkg.in/ini.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/cron"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/social"
	"github.com/gogits/gogs/modules/user"
)

const (
	INSTALL base.TplName = "install"
)

func checkRunMode() {
	switch setting.Cfg.Section("").Key("RUN_MODE").String() {
	case "prod":
		macaron.Env = macaron.PROD
		macaron.ColorLog = false
		setting.ProdMode = true
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
		models.InitDeliverHooks()
		log.NewGitLogger(path.Join(setting.LogRootPath, "http.log"))
	}
	if models.EnableSQLite3 {
		log.Info("SQLite3 Supported")
	}
	checkRunMode()
}

func InstallInit(ctx *middleware.Context) {
	if setting.InstallLock {
		ctx.Handle(404, "Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = ctx.Tr("install.install")
	ctx.Data["PageIsInstall"] = true

	ctx.Data["DbOptions"] = []string{"MySQL", "PostgreSQL", "SQLite3"}
}

func Install(ctx *middleware.Context) {
	form := auth.InstallForm{}

	// Database settings
	form.DbHost = models.DbCfg.Host
	form.DbUser = models.DbCfg.User
	form.DbName = models.DbCfg.Name
	form.DbPath = models.DbCfg.Path

	if models.EnableSQLite3 {
		ctx.Data["CurDbOption"] = "SQLite3" // Default when enabled.
	} else {
		ctx.Data["CurDbOption"] = "MySQL"
	}

	// Application general settings
	form.AppName = setting.AppName
	form.RepoRootPath = setting.RepoRootPath

	// Note(unknwon): it's hard for Windows users change a running user,
	// 	so just use current one if config says default.
	if setting.IsWindows && setting.RunUser == "git" {
		form.RunUser = user.CurrentUsername()
	} else {
		form.RunUser = setting.RunUser
	}

	form.Domain = setting.Domain
	form.SSHPort = setting.SSHPort
	form.HTTPPort = setting.HttpPort
	form.AppUrl = setting.AppUrl

	// E-mail service settings
	if setting.MailService != nil {
		form.SMTPHost = setting.MailService.Host
		form.SMTPFrom = setting.MailService.From
		form.SMTPEmail = setting.MailService.User
	}
	form.RegisterConfirm = setting.Service.RegisterEmailConfirm
	form.MailNotify = setting.Service.EnableNotifyMail

	// Server and other services settings
	form.OfflineMode = setting.OfflineMode
	form.DisableRegistration = setting.Service.DisableRegistration
	form.RequireSignInView = setting.Service.RequireSignInView

	auth.AssignForm(form, ctx.Data)
	ctx.HTML(200, INSTALL)
}

func InstallPost(ctx *middleware.Context, form auth.InstallForm) {
	ctx.Data["CurDbOption"] = form.DbType

	if ctx.HasError() {
		if ctx.HasValue("Err_SMTPEmail") {
			ctx.Data["Err_SMTP"] = true
		}
		if ctx.HasValue("Err_AdminName") ||
			ctx.HasValue("Err_AdminPasswd") ||
			ctx.HasValue("Err_AdminEmail") {
			ctx.Data["Err_Admin"] = true
		}

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
	models.DbCfg.Type = dbTypes[form.DbType]
	models.DbCfg.Host = form.DbHost
	models.DbCfg.User = form.DbUser
	models.DbCfg.Passwd = form.DbPasswd
	models.DbCfg.Name = form.DbName
	models.DbCfg.SSLMode = form.SSLMode
	models.DbCfg.Path = form.DbPath

	if models.DbCfg.Type == "sqlite3" && len(models.DbCfg.Path) == 0 {
		ctx.Data["Err_DbPath"] = true
		ctx.RenderWithErr(ctx.Tr("install.err_empty_sqlite_path"), INSTALL, &form)
		return
	}

	// Set test engine.
	var x *xorm.Engine
	if err := models.NewTestEngine(x); err != nil {
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.Data["Err_DbType"] = true
			ctx.RenderWithErr(ctx.Tr("install.sqlite3_not_available", "http://gogs.io/docs/installation/install_from_binary.html"), INSTALL, &form)
		} else {
			ctx.Data["Err_DbSetting"] = true
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
	curUser := user.CurrentUsername()
	if form.RunUser != curUser {
		ctx.Data["Err_RunUser"] = true
		ctx.RenderWithErr(ctx.Tr("install.run_user_not_match", form.RunUser, curUser), INSTALL, &form)
		return
	}

	// Check admin password.
	if form.AdminPasswd != form.AdminConfirmPasswd {
		ctx.Data["Err_Admin"] = true
		ctx.Data["Err_AdminPasswd"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), INSTALL, form)
		return
	}

	if form.AppUrl[len(form.AppUrl)-1] != '/' {
		form.AppUrl += "/"
	}

	// Save settings.
	cfg := ini.Empty()
	if com.IsFile(setting.CustomConf) {
		// Keeps custom settings if there is already something.
		if err := cfg.Append(setting.CustomConf); err != nil {
			log.Error(4, "Fail to load custom conf '%s': %v", setting.CustomConf, err)
		}
	}
	cfg.Section("database").Key("DB_TYPE").SetValue(models.DbCfg.Type)
	cfg.Section("database").Key("HOST").SetValue(models.DbCfg.Host)
	cfg.Section("database").Key("NAME").SetValue(models.DbCfg.Name)
	cfg.Section("database").Key("USER").SetValue(models.DbCfg.User)
	cfg.Section("database").Key("PASSWD").SetValue(models.DbCfg.Passwd)
	cfg.Section("database").Key("SSL_MODE").SetValue(models.DbCfg.SSLMode)
	cfg.Section("database").Key("PATH").SetValue(models.DbCfg.Path)

	cfg.Section("").Key("APP_NAME").SetValue(form.AppName)
	cfg.Section("repository").Key("ROOT").SetValue(form.RepoRootPath)
	cfg.Section("").Key("RUN_USER").SetValue(form.RunUser)
	cfg.Section("server").Key("DOMAIN").SetValue(form.Domain)
	cfg.Section("server").Key("HTTP_PORT").SetValue(form.HTTPPort)
	cfg.Section("server").Key("ROOT_URL").SetValue(form.AppUrl)

	if form.SSHPort == 0 {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("true")
	} else {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("false")
		cfg.Section("server").Key("SSH_PORT").SetValue(com.ToStr(form.SSHPort))
	}

	if len(strings.TrimSpace(form.SMTPHost)) > 0 {
		cfg.Section("mailer").Key("ENABLED").SetValue("true")
		cfg.Section("mailer").Key("HOST").SetValue(form.SMTPHost)
		cfg.Section("mailer").Key("FROM").SetValue(form.SMTPFrom)
		cfg.Section("mailer").Key("USER").SetValue(form.SMTPEmail)
		cfg.Section("mailer").Key("PASSWD").SetValue(form.SMTPPasswd)
	} else {
		cfg.Section("mailer").Key("ENABLED").SetValue("false")
	}
	cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").SetValue(com.ToStr(form.RegisterConfirm))
	cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").SetValue(com.ToStr(form.MailNotify))

	cfg.Section("server").Key("OFFLINE_MODE").SetValue(com.ToStr(form.OfflineMode))
	cfg.Section("service").Key("DISABLE_REGISTRATION").SetValue(com.ToStr(form.DisableRegistration))
	cfg.Section("service").Key("REQUIRE_SIGNIN_VIEW").SetValue(com.ToStr(form.RequireSignInView))

	cfg.Section("").Key("RUN_MODE").SetValue("prod")

	cfg.Section("session").Key("PROVIDER").SetValue("file")

	cfg.Section("log").Key("MODE").SetValue("file")
	cfg.Section("log").Key("LEVEL").SetValue("Info")

	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	cfg.Section("security").Key("SECRET_KEY").SetValue(base.GetRandomString(15))

	os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm)
	if err := cfg.SaveTo(setting.CustomConf); err != nil {
		ctx.RenderWithErr(ctx.Tr("install.save_config_failed", err), INSTALL, &form)
		return
	}

	GlobalInit()

	// Create admin account.
	if len(form.AdminName) > 0 {
		if err := models.CreateUser(&models.User{
			Name:     form.AdminName,
			Email:    form.AdminEmail,
			Passwd:   form.AdminPasswd,
			IsAdmin:  true,
			IsActive: true,
		}); err != nil {
			if !models.IsErrUserAlreadyExist(err) {
				setting.InstallLock = false
				ctx.Data["Err_AdminName"] = true
				ctx.Data["Err_AdminEmail"] = true
				ctx.RenderWithErr(ctx.Tr("install.invalid_admin_setting", err), INSTALL, &form)
				return
			}
			log.Info("Admin account already exist")
		}
	}

	log.Info("First-time run install finished!")
	ctx.Flash.Success(ctx.Tr("install.install_success"))
	ctx.Redirect(form.AppUrl + "user/login")
}
