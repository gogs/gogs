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

	"github.com/Unknwon/goconfig"
	"github.com/go-martini/martini"
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
	switch setting.Cfg.MustValue("", "RUN_MODE") {
	case "prod":
		martini.Env = martini.Prod
		setting.ProdMode = true
	case "test":
		martini.Env = martini.Test
	}
	log.Info("Run Mode: %s", strings.Title(martini.Env))
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
	models.LoadRepoConfig()
	models.NewRepoContext()
	NewServices()

	if setting.InstallLock {
		if err := models.NewEngine(); err != nil {
			log.Fatal("Fail to initialize ORM engine: %v", err)
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
		ctx.Handle(404, "install.Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = "Install"
	ctx.Data["PageIsInstall"] = true

	// Get and assign values to install form.
	if len(form.Host) == 0 {
		form.Host = models.DbCfg.Host
	}
	if len(form.User) == 0 {
		form.User = models.DbCfg.User
	}
	if len(form.Passwd) == 0 {
		form.Passwd = models.DbCfg.Pwd
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
		form.RunUser = setting.RunUser
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
		ctx.Handle(404, "install.InstallPost", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = "Install"
	ctx.Data["PageIsInstall"] = true

	renderDbOption(ctx)
	ctx.Data["CurDbOption"] = form.Database

	if ctx.HasError() {
		ctx.HTML(200, INSTALL)
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		ctx.RenderWithErr("Fail to test 'git' command: "+err.Error(), INSTALL, &form)
		return
	}

	// Pass basic check, now test configuration.
	// Test database setting.
	dbTypes := map[string]string{"MySQL": "mysql", "PostgreSQL": "postgres", "SQLite3": "sqlite3"}
	models.DbCfg.Type = dbTypes[form.Database]
	models.DbCfg.Host = form.Host
	models.DbCfg.User = form.User
	models.DbCfg.Pwd = form.Passwd
	models.DbCfg.Name = form.DatabaseName
	models.DbCfg.SslMode = form.SslMode
	models.DbCfg.Path = form.DatabasePath

	// Set test engine.
	var x *xorm.Engine
	if err := models.NewTestEngine(x); err != nil {
		// NOTE: should use core.QueryDriver (github.com/go-xorm/core)
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.RenderWithErr("Your release version does not support SQLite3, please download the official binary version "+
				"from http://gogs.io/docs/installation/install_from_binary.md, NOT the gobuild version.", INSTALL, &form)
		} else {
			ctx.RenderWithErr("Database setting is not correct: "+err.Error(), INSTALL, &form)
		}
		return
	}

	// Test repository root path.
	if err := os.MkdirAll(form.RepoRootPath, os.ModePerm); err != nil {
		ctx.RenderWithErr("Repository root path is invalid: "+err.Error(), INSTALL, &form)
		return
	}

	// Check run user.
	curUser := os.Getenv("USER")
	if len(curUser) == 0 {
		curUser = os.Getenv("USERNAME")
	}
	// Does not check run user when the install lock is off.
	if form.RunUser != curUser {
		ctx.RenderWithErr("Run user isn't the current user: "+form.RunUser+" -> "+curUser, INSTALL, &form)
		return
	}

	// Save settings.
	setting.Cfg.SetValue("database", "DB_TYPE", models.DbCfg.Type)
	setting.Cfg.SetValue("database", "HOST", models.DbCfg.Host)
	setting.Cfg.SetValue("database", "NAME", models.DbCfg.Name)
	setting.Cfg.SetValue("database", "USER", models.DbCfg.User)
	setting.Cfg.SetValue("database", "PASSWD", models.DbCfg.Pwd)
	setting.Cfg.SetValue("database", "SSL_MODE", models.DbCfg.SslMode)
	setting.Cfg.SetValue("database", "PATH", models.DbCfg.Path)

	setting.Cfg.SetValue("repository", "ROOT", form.RepoRootPath)
	setting.Cfg.SetValue("", "RUN_USER", form.RunUser)
	setting.Cfg.SetValue("server", "DOMAIN", form.Domain)
	setting.Cfg.SetValue("server", "ROOT_URL", form.AppUrl)

	if len(strings.TrimSpace(form.SmtpHost)) > 0 {
		setting.Cfg.SetValue("mailer", "ENABLED", "true")
		setting.Cfg.SetValue("mailer", "HOST", form.SmtpHost)
		setting.Cfg.SetValue("mailer", "USER", form.SmtpEmail)
		setting.Cfg.SetValue("mailer", "PASSWD", form.SmtpPasswd)

		setting.Cfg.SetValue("service", "REGISTER_EMAIL_CONFIRM", base.ToStr(form.RegisterConfirm == "on"))
		setting.Cfg.SetValue("service", "ENABLE_NOTIFY_MAIL", base.ToStr(form.MailNotify == "on"))
	}

	setting.Cfg.SetValue("", "RUN_MODE", "prod")

	setting.Cfg.SetValue("security", "INSTALL_LOCK", "true")

	os.MkdirAll("custom/conf", os.ModePerm)
	if err := goconfig.SaveConfigFile(setting.Cfg, path.Join(setting.CustomPath, "conf/app.ini")); err != nil {
		ctx.RenderWithErr("Fail to save configuration: "+err.Error(), INSTALL, &form)
		return
	}

	GlobalInit()

	// Create admin account.
	if _, err := models.CreateUser(&models.User{Name: form.AdminName, Email: form.AdminEmail, Passwd: form.AdminPasswd,
		IsAdmin: true, IsActive: true}); err != nil {
		if err != models.ErrUserAlreadyExist {
			setting.InstallLock = false
			ctx.RenderWithErr("Admin account setting is invalid: "+err.Error(), INSTALL, &form)
			return
		}
		log.Info("Admin account already exist")
	}

	log.Info("First-time run install finished!")
	ctx.Flash.Success("Welcome! We're glad that you choose Gogs, have fun and take care.")
	ctx.Redirect("/user/login")
}
