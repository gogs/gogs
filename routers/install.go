// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"
	"gopkg.in/macaron.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/cron"
	"github.com/gogits/gogs/modules/form"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/markdown"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/ssh"
	"github.com/gogits/gogs/modules/template/highlight"
	"github.com/gogits/gogs/modules/user"
)

const (
	INSTALL base.TplName = "install"
)

func checkRunMode() {
	if setting.ProdMode {
		macaron.Env = macaron.PROD
		macaron.ColorLog = false
	} else {
		git.Debug = true
	}
	log.Info("Run Mode: %s", strings.Title(macaron.Env))
}

func NewServices() {
	setting.NewServices()
	mailer.NewContext()
}

// GlobalInit is for global configuration reload-able.
func GlobalInit() {
	setting.NewContext()
	log.Trace("Custom path: %s", setting.CustomPath)
	log.Trace("Log path: %s", setting.LogRootPath)
	models.LoadConfigs()
	NewServices()

	if setting.InstallLock {
		highlight.NewContext()
		markdown.BuildSanitizer()
		if err := models.NewEngine(); err != nil {
			log.Fatal(2, "Fail to initialize ORM engine: %v", err)
		}
		models.HasEngine = true

		models.LoadRepoConfig()
		models.NewRepoContext()

		// Booting long running goroutines.
		cron.NewContext()
		models.InitSyncMirrors()
		models.InitDeliverHooks()
		models.InitTestPullRequests()
	}
	if models.EnableSQLite3 {
		log.Info("SQLite3 Supported")
	}
	if setting.SupportMiniWinService {
		log.Info("Builtin Windows Service Supported")
	}
	checkRunMode()

	if setting.InstallLock && setting.SSH.StartBuiltinServer {
		ssh.Listen(setting.SSH.ListenHost, setting.SSH.ListenPort, setting.SSH.ServerCiphers)
		log.Info("SSH server started on %s:%v", setting.SSH.ListenHost, setting.SSH.ListenPort)
		log.Trace("SSH server cipher list: %v", setting.SSH.ServerCiphers)
	}
}

func InstallInit(ctx *context.Context) {
	if setting.InstallLock {
		ctx.Handle(404, "Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = ctx.Tr("install.install")
	ctx.Data["PageIsInstall"] = true

	dbOpts := []string{"MySQL", "PostgreSQL", "MSSQL"}
	if models.EnableSQLite3 {
		dbOpts = append(dbOpts, "SQLite3")
	}
	ctx.Data["DbOptions"] = dbOpts
}

func Install(ctx *context.Context) {
	f := form.Install{}

	// Database settings
	f.DbHost = models.DbCfg.Host
	f.DbUser = models.DbCfg.User
	f.DbName = models.DbCfg.Name
	f.DbPath = models.DbCfg.Path

	ctx.Data["CurDbOption"] = "MySQL"
	switch models.DbCfg.Type {
	case "postgres":
		ctx.Data["CurDbOption"] = "PostgreSQL"
	case "mssql":
		ctx.Data["CurDbOption"] = "MSSQL"
	case "sqlite3":
		if models.EnableSQLite3 {
			ctx.Data["CurDbOption"] = "SQLite3"
		}
	}

	// Application general settings
	f.AppName = setting.AppName
	f.RepoRootPath = setting.RepoRootPath

	// Note(unknwon): it's hard for Windows users change a running user,
	// 	so just use current one if config says default.
	if setting.IsWindows && setting.RunUser == "git" {
		f.RunUser = user.CurrentUsername()
	} else {
		f.RunUser = setting.RunUser
	}

	f.Domain = setting.Domain
	f.SSHPort = setting.SSH.Port
	f.UseBuiltinSSHServer = setting.SSH.StartBuiltinServer
	f.HTTPPort = setting.HTTPPort
	f.AppUrl = setting.AppUrl
	f.LogRootPath = setting.LogRootPath

	// E-mail service settings
	if setting.MailService != nil {
		f.SMTPHost = setting.MailService.Host
		f.SMTPFrom = setting.MailService.From
		f.SMTPUser = setting.MailService.User
	}
	f.RegisterConfirm = setting.Service.RegisterEmailConfirm
	f.MailNotify = setting.Service.EnableNotifyMail

	// Server and other services settings
	f.OfflineMode = setting.OfflineMode
	f.DisableGravatar = setting.DisableGravatar
	f.EnableFederatedAvatar = setting.EnableFederatedAvatar
	f.DisableRegistration = setting.Service.DisableRegistration
	f.EnableCaptcha = setting.Service.EnableCaptcha
	f.RequireSignInView = setting.Service.RequireSignInView

	form.Assign(f, ctx.Data)
	ctx.HTML(200, INSTALL)
}

func InstallPost(ctx *context.Context, f form.Install) {
	ctx.Data["CurDbOption"] = f.DbType

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
		ctx.RenderWithErr(ctx.Tr("install.test_git_failed", err), INSTALL, &f)
		return
	}

	// Pass basic check, now test configuration.
	// Test database setting.
	dbTypes := map[string]string{"MySQL": "mysql", "PostgreSQL": "postgres", "MSSQL": "mssql", "SQLite3": "sqlite3", "TiDB": "tidb"}
	models.DbCfg.Type = dbTypes[f.DbType]
	models.DbCfg.Host = f.DbHost
	models.DbCfg.User = f.DbUser
	models.DbCfg.Passwd = f.DbPasswd
	models.DbCfg.Name = f.DbName
	models.DbCfg.SSLMode = f.SSLMode
	models.DbCfg.Path = f.DbPath

	if models.DbCfg.Type == "sqlite3" && len(models.DbCfg.Path) == 0 {
		ctx.Data["Err_DbPath"] = true
		ctx.RenderWithErr(ctx.Tr("install.err_empty_db_path"), INSTALL, &f)
		return
	}

	// Set test engine.
	var x *xorm.Engine
	if err := models.NewTestEngine(x); err != nil {
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			ctx.Data["Err_DbType"] = true
			ctx.RenderWithErr(ctx.Tr("install.sqlite3_not_available", "https://gogs.io/docs/installation/install_from_binary.html"), INSTALL, &f)
		} else {
			ctx.Data["Err_DbSetting"] = true
			ctx.RenderWithErr(ctx.Tr("install.invalid_db_setting", err), INSTALL, &f)
		}
		return
	}

	// Test repository root path.
	f.RepoRootPath = strings.Replace(f.RepoRootPath, "\\", "/", -1)
	if err := os.MkdirAll(f.RepoRootPath, os.ModePerm); err != nil {
		ctx.Data["Err_RepoRootPath"] = true
		ctx.RenderWithErr(ctx.Tr("install.invalid_repo_path", err), INSTALL, &f)
		return
	}

	// Test log root path.
	f.LogRootPath = strings.Replace(f.LogRootPath, "\\", "/", -1)
	if err := os.MkdirAll(f.LogRootPath, os.ModePerm); err != nil {
		ctx.Data["Err_LogRootPath"] = true
		ctx.RenderWithErr(ctx.Tr("install.invalid_log_root_path", err), INSTALL, &f)
		return
	}

	currentUser, match := setting.IsRunUserMatchCurrentUser(f.RunUser)
	if !match {
		ctx.Data["Err_RunUser"] = true
		ctx.RenderWithErr(ctx.Tr("install.run_user_not_match", f.RunUser, currentUser), INSTALL, &f)
		return
	}

	// Make sure FROM field is valid
	if len(f.SMTPFrom) > 0 {
		_, err := mail.ParseAddress(f.SMTPFrom)
		if err != nil {
			ctx.Data["Err_SMTP"] = true
			ctx.Data["Err_SMTPFrom"] = true
			ctx.RenderWithErr(ctx.Tr("install.invalid_smtp_from", err), INSTALL, &f)
			return
		}
	}

	// Check logic loophole between disable self-registration and no admin account.
	if f.DisableRegistration && len(f.AdminName) == 0 {
		ctx.Data["Err_Services"] = true
		ctx.Data["Err_Admin"] = true
		ctx.RenderWithErr(ctx.Tr("install.no_admin_and_disable_registration"), INSTALL, f)
		return
	}

	// Check admin password.
	if len(f.AdminName) > 0 && len(f.AdminPasswd) == 0 {
		ctx.Data["Err_Admin"] = true
		ctx.Data["Err_AdminPasswd"] = true
		ctx.RenderWithErr(ctx.Tr("install.err_empty_admin_password"), INSTALL, f)
		return
	}
	if f.AdminPasswd != f.AdminConfirmPasswd {
		ctx.Data["Err_Admin"] = true
		ctx.Data["Err_AdminPasswd"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), INSTALL, f)
		return
	}

	if f.AppUrl[len(f.AppUrl)-1] != '/' {
		f.AppUrl += "/"
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

	cfg.Section("").Key("APP_NAME").SetValue(f.AppName)
	cfg.Section("repository").Key("ROOT").SetValue(f.RepoRootPath)
	cfg.Section("").Key("RUN_USER").SetValue(f.RunUser)
	cfg.Section("server").Key("DOMAIN").SetValue(f.Domain)
	cfg.Section("server").Key("HTTP_PORT").SetValue(f.HTTPPort)
	cfg.Section("server").Key("ROOT_URL").SetValue(f.AppUrl)

	if f.SSHPort == 0 {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("true")
	} else {
		cfg.Section("server").Key("DISABLE_SSH").SetValue("false")
		cfg.Section("server").Key("SSH_PORT").SetValue(com.ToStr(f.SSHPort))
		cfg.Section("server").Key("START_SSH_SERVER").SetValue(com.ToStr(f.UseBuiltinSSHServer))
	}

	if len(strings.TrimSpace(f.SMTPHost)) > 0 {
		cfg.Section("mailer").Key("ENABLED").SetValue("true")
		cfg.Section("mailer").Key("HOST").SetValue(f.SMTPHost)
		cfg.Section("mailer").Key("FROM").SetValue(f.SMTPFrom)
		cfg.Section("mailer").Key("USER").SetValue(f.SMTPUser)
		cfg.Section("mailer").Key("PASSWD").SetValue(f.SMTPPasswd)
	} else {
		cfg.Section("mailer").Key("ENABLED").SetValue("false")
	}
	cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").SetValue(com.ToStr(f.RegisterConfirm))
	cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").SetValue(com.ToStr(f.MailNotify))

	cfg.Section("server").Key("OFFLINE_MODE").SetValue(com.ToStr(f.OfflineMode))
	cfg.Section("picture").Key("DISABLE_GRAVATAR").SetValue(com.ToStr(f.DisableGravatar))
	cfg.Section("picture").Key("ENABLE_FEDERATED_AVATAR").SetValue(com.ToStr(f.EnableFederatedAvatar))
	cfg.Section("service").Key("DISABLE_REGISTRATION").SetValue(com.ToStr(f.DisableRegistration))
	cfg.Section("service").Key("ENABLE_CAPTCHA").SetValue(com.ToStr(f.EnableCaptcha))
	cfg.Section("service").Key("REQUIRE_SIGNIN_VIEW").SetValue(com.ToStr(f.RequireSignInView))

	cfg.Section("").Key("RUN_MODE").SetValue("prod")

	cfg.Section("session").Key("PROVIDER").SetValue("file")

	cfg.Section("log").Key("MODE").SetValue("file")
	cfg.Section("log").Key("LEVEL").SetValue("Info")
	cfg.Section("log").Key("ROOT_PATH").SetValue(f.LogRootPath)

	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	secretKey, err := base.GetRandomString(15)
	if err != nil {
		ctx.RenderWithErr(ctx.Tr("install.secret_key_failed", err), INSTALL, &f)
		return
	}
	cfg.Section("security").Key("SECRET_KEY").SetValue(secretKey)

	os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm)
	if err := cfg.SaveTo(setting.CustomConf); err != nil {
		ctx.RenderWithErr(ctx.Tr("install.save_config_failed", err), INSTALL, &f)
		return
	}

	GlobalInit()

	// Create admin account
	if len(f.AdminName) > 0 {
		u := &models.User{
			Name:     f.AdminName,
			Email:    f.AdminEmail,
			Passwd:   f.AdminPasswd,
			IsAdmin:  true,
			IsActive: true,
		}
		if err := models.CreateUser(u); err != nil {
			if !models.IsErrUserAlreadyExist(err) {
				setting.InstallLock = false
				ctx.Data["Err_AdminName"] = true
				ctx.Data["Err_AdminEmail"] = true
				ctx.RenderWithErr(ctx.Tr("install.invalid_admin_setting", err), INSTALL, &f)
				return
			}
			log.Info("Admin account already exist")
			u, _ = models.GetUserByName(u.Name)
		}

		// Auto-login for admin
		ctx.Session.Set("uid", u.ID)
		ctx.Session.Set("uname", u.Name)
	}

	log.Info("First-time run install finished!")
	ctx.Flash.Success(ctx.Tr("install.install_success"))
	ctx.Redirect(f.AppUrl + "user/login")
}
