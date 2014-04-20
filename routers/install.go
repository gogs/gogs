// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/Unknwon/goconfig"
	"github.com/go-martini/martini"
	"github.com/go-xorm/xorm"
	qlog "github.com/qiniu/log"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/cron"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/social"
)

// Check run mode(Default of martini is Dev).
func checkRunMode() {
	switch base.Cfg.MustValue("", "RUN_MODE") {
	case "prod":
		martini.Env = martini.Prod
		base.IsProdMode = true
	case "test":
		martini.Env = martini.Test
	}
	log.Info("Run Mode: %s", strings.Title(martini.Env))
}

func NewServices() {
	base.NewBaseServices()
	social.NewOauthService()
}

// GlobalInit is for global configuration reload-able.
func GlobalInit() {
	base.NewConfigContext()
	mailer.NewMailerContext()
	models.LoadModelsConfig()
	models.LoadRepoConfig()
	models.NewRepoContext()
	NewServices()

	if base.InstallLock {
		if err := models.NewEngine(); err != nil {
			qlog.Fatal(err)
		}

		models.HasEngine = true
		if models.EnableSQLite3 {
			log.Info("SQLite3 Enabled")
		}
		cron.NewCronContext()
	}
	checkRunMode()
}

func Install(ctx *middleware.Context, form auth.InstallForm) {
	if base.InstallLock {
		ctx.Handle(404, "install.Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = "Install"
	ctx.Data["PageIsInstall"] = true

	// Get and assign value to install form.
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
		form.RepoRootPath = base.RepoRootPath
	}
	if len(form.RunUser) == 0 {
		form.RunUser = base.RunUser
	}
	if len(form.Domain) == 0 {
		form.Domain = base.Domain
	}
	if len(form.AppUrl) == 0 {
		form.AppUrl = base.AppUrl
	}

	auth.AssignForm(form, ctx.Data)
	ctx.HTML(200, "install")
}

func InstallPost(ctx *middleware.Context, form auth.InstallForm) {
	if base.InstallLock {
		ctx.Handle(404, "install.Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = "Install"
	ctx.Data["PageIsInstall"] = true

	if ctx.HasError() {
		ctx.HTML(200, "install")
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		ctx.RenderWithErr("Fail to test 'git' command: "+err.Error(), "install", &form)
		return
	}

	// Pass basic check, now test configuration.
	// Test database setting.
	dbTypes := map[string]string{"mysql": "mysql", "pgsql": "postgres", "sqlite": "sqlite3"}
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
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.RenderWithErr("Your release version does not support SQLite3, please download the official binary version "+
				"from https://github.com/gogits/gogs/wiki/Install-from-binary, NOT the gobuild version.", "install", &form)
		} else {
			ctx.RenderWithErr("Database setting is not correct: "+err.Error(), "install", &form)
		}
		return
	}

	// Test repository root path.
	if err := os.MkdirAll(form.RepoRootPath, os.ModePerm); err != nil {
		ctx.RenderWithErr("Repository root path is invalid: "+err.Error(), "install", &form)
		return
	}

	// Check run user.
	curUser := os.Getenv("USER")
	if len(curUser) == 0 {
		curUser = os.Getenv("USERNAME")
	}
	// Does not check run user when the install lock is off.
	if form.RunUser != curUser {
		ctx.RenderWithErr("Run user isn't the current user: "+form.RunUser+" -> "+curUser, "install", &form)
		return
	}

	// Save settings.
	base.Cfg.SetValue("database", "DB_TYPE", models.DbCfg.Type)
	base.Cfg.SetValue("database", "HOST", models.DbCfg.Host)
	base.Cfg.SetValue("database", "NAME", models.DbCfg.Name)
	base.Cfg.SetValue("database", "USER", models.DbCfg.User)
	base.Cfg.SetValue("database", "PASSWD", models.DbCfg.Pwd)
	base.Cfg.SetValue("database", "SSL_MODE", models.DbCfg.SslMode)
	base.Cfg.SetValue("database", "PATH", models.DbCfg.Path)

	base.Cfg.SetValue("repository", "ROOT", form.RepoRootPath)
	base.Cfg.SetValue("", "RUN_USER", form.RunUser)
	base.Cfg.SetValue("server", "DOMAIN", form.Domain)
	base.Cfg.SetValue("server", "ROOT_URL", form.AppUrl)

	if len(strings.TrimSpace(form.SmtpHost)) > 0 {
		base.Cfg.SetValue("mailer", "ENABLED", "true")
		base.Cfg.SetValue("mailer", "HOST", form.SmtpHost)
		base.Cfg.SetValue("mailer", "USER", form.SmtpEmail)
		base.Cfg.SetValue("mailer", "PASSWD", form.SmtpPasswd)

		base.Cfg.SetValue("service", "REGISTER_EMAIL_CONFIRM", base.ToStr(form.RegisterConfirm == "on"))
		base.Cfg.SetValue("service", "ENABLE_NOTIFY_MAIL", base.ToStr(form.MailNotify == "on"))
	}

	base.Cfg.SetValue("", "RUN_MODE", "prod")

	base.Cfg.SetValue("security", "INSTALL_LOCK", "true")

	os.MkdirAll("custom/conf", os.ModePerm)
	if err := goconfig.SaveConfigFile(base.Cfg, "custom/conf/app.ini"); err != nil {
		ctx.RenderWithErr("Fail to save configuration: "+err.Error(), "install", &form)
		return
	}

	GlobalInit()

	// Create admin account.
	if _, err := models.RegisterUser(&models.User{Name: form.AdminName, Email: form.AdminEmail, Passwd: form.AdminPasswd,
		IsAdmin: true, IsActive: true}); err != nil {
		if err != models.ErrUserAlreadyExist {
			base.InstallLock = false
			ctx.RenderWithErr("Admin account setting is invalid: "+err.Error(), "install", &form)
			return
		}
		log.Info("Admin account already exist")
	}

	log.Info("First-time run install finished!")
	ctx.Flash.Success("Welcome! We're glad that you choose Gogs, have fun and take care.")
	ctx.Redirect("/user/login")
}
