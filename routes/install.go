// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routes

import (
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

	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/cron"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/mailer"
	"github.com/gogs/gogs/pkg/markup"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/ssh"
	"github.com/gogs/gogs/pkg/template/highlight"
	"github.com/gogs/gogs/pkg/tool"
	"github.com/gogs/gogs/pkg/user"
)

const (
	INSTALL = "install"
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
		markup.NewSanitizer()
		if err := models.NewEngine(); err != nil {
			log.Fatal(2, "Fail to initialize ORM engine: %v", err)
		}
		models.HasEngine = true

		models.LoadAuthSources()
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

	if !setting.InstallLock {
		return
	}

	if setting.SSH.StartBuiltinServer {
		ssh.Listen(setting.SSH.ListenHost, setting.SSH.ListenPort, setting.SSH.ServerCiphers)
		log.Info("SSH server started on %s:%v", setting.SSH.ListenHost, setting.SSH.ListenPort)
		log.Trace("SSH server cipher list: %v", setting.SSH.ServerCiphers)
	}

	if setting.SSH.RewriteAuthorizedKeysAtStart {
		if err := models.RewriteAuthorizedKeys(); err != nil {
			log.Warn("Failed to rewrite authorized_keys file: %v", err)
		}
	}
}

func InstallInit(c *context.Context) {
	if setting.InstallLock {
		c.NotFound()
		return
	}

	c.Title("install.install")
	c.PageIs("Install")

	dbOpts := []string{"MySQL", "PostgreSQL", "MSSQL"}
	if models.EnableSQLite3 {
		dbOpts = append(dbOpts, "SQLite3")
	}
	c.Data["DbOptions"] = dbOpts
}

func Install(c *context.Context) {
	f := form.Install{}

	// Database settings
	f.DbHost = models.DbCfg.Host
	f.DbUser = models.DbCfg.User
	f.DbName = models.DbCfg.Name
	f.DbPath = models.DbCfg.Path

	c.Data["CurDbOption"] = "MySQL"
	switch models.DbCfg.Type {
	case "postgres":
		c.Data["CurDbOption"] = "PostgreSQL"
	case "mssql":
		c.Data["CurDbOption"] = "MSSQL"
	case "sqlite3":
		if models.EnableSQLite3 {
			c.Data["CurDbOption"] = "SQLite3"
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
	f.AppUrl = setting.AppURL
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

	form.Assign(f, c.Data)
	c.Success(INSTALL)
}

func InstallPost(c *context.Context, f form.Install) {
	c.Data["CurDbOption"] = f.DbType

	if c.HasError() {
		if c.HasValue("Err_SMTPEmail") {
			c.FormErr("SMTP")
		}
		if c.HasValue("Err_AdminName") ||
			c.HasValue("Err_AdminPasswd") ||
			c.HasValue("Err_AdminEmail") {
			c.FormErr("Admin")
		}

		c.Success(INSTALL)
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		c.RenderWithErr(c.Tr("install.test_git_failed", err), INSTALL, &f)
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
		c.FormErr("DbPath")
		c.RenderWithErr(c.Tr("install.err_empty_db_path"), INSTALL, &f)
		return
	}

	// Set test engine.
	var x *xorm.Engine
	if err := models.NewTestEngine(x); err != nil {
		if strings.Contains(err.Error(), `Unknown database type: sqlite3`) {
			c.FormErr("DbType")
			c.RenderWithErr(c.Tr("install.sqlite3_not_available", "https://gogs.io/docs/installation/install_from_binary.html"), INSTALL, &f)
		} else {
			c.FormErr("DbSetting")
			c.RenderWithErr(c.Tr("install.invalid_db_setting", err), INSTALL, &f)
		}
		return
	}

	// Test repository root path.
	f.RepoRootPath = strings.Replace(f.RepoRootPath, "\\", "/", -1)
	if err := os.MkdirAll(f.RepoRootPath, os.ModePerm); err != nil {
		c.FormErr("RepoRootPath")
		c.RenderWithErr(c.Tr("install.invalid_repo_path", err), INSTALL, &f)
		return
	}

	// Test log root path.
	f.LogRootPath = strings.Replace(f.LogRootPath, "\\", "/", -1)
	if err := os.MkdirAll(f.LogRootPath, os.ModePerm); err != nil {
		c.FormErr("LogRootPath")
		c.RenderWithErr(c.Tr("install.invalid_log_root_path", err), INSTALL, &f)
		return
	}

	currentUser, match := setting.IsRunUserMatchCurrentUser(f.RunUser)
	if !match {
		c.FormErr("RunUser")
		c.RenderWithErr(c.Tr("install.run_user_not_match", f.RunUser, currentUser), INSTALL, &f)
		return
	}

	// Check host address and port
	if len(f.SMTPHost) > 0 && !strings.Contains(f.SMTPHost, ":") {
		c.FormErr("SMTP", "SMTPHost")
		c.RenderWithErr(c.Tr("install.smtp_host_missing_port"), INSTALL, &f)
		return
	}

	// Make sure FROM field is valid
	if len(f.SMTPFrom) > 0 {
		_, err := mail.ParseAddress(f.SMTPFrom)
		if err != nil {
			c.FormErr("SMTP", "SMTPFrom")
			c.RenderWithErr(c.Tr("install.invalid_smtp_from", err), INSTALL, &f)
			return
		}
	}

	// Check logic loophole between disable self-registration and no admin account.
	if f.DisableRegistration && len(f.AdminName) == 0 {
		c.FormErr("Services", "Admin")
		c.RenderWithErr(c.Tr("install.no_admin_and_disable_registration"), INSTALL, f)
		return
	}

	// Check admin password.
	if len(f.AdminName) > 0 && len(f.AdminPasswd) == 0 {
		c.FormErr("Admin", "AdminPasswd")
		c.RenderWithErr(c.Tr("install.err_empty_admin_password"), INSTALL, f)
		return
	}
	if f.AdminPasswd != f.AdminConfirmPasswd {
		c.FormErr("Admin", "AdminPasswd")
		c.RenderWithErr(c.Tr("form.password_not_match"), INSTALL, f)
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
			log.Error(2, "Fail to load custom conf '%s': %v", setting.CustomConf, err)
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

	mode := "file"
	if f.EnableConsoleMode {
		mode = "console, file"
	}
	cfg.Section("log").Key("MODE").SetValue(mode)
	cfg.Section("log").Key("LEVEL").SetValue("Info")
	cfg.Section("log").Key("ROOT_PATH").SetValue(f.LogRootPath)

	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	secretKey, err := tool.RandomString(15)
	if err != nil {
		c.RenderWithErr(c.Tr("install.secret_key_failed", err), INSTALL, &f)
		return
	}
	cfg.Section("security").Key("SECRET_KEY").SetValue(secretKey)

	os.MkdirAll(filepath.Dir(setting.CustomConf), os.ModePerm)
	if err := cfg.SaveTo(setting.CustomConf); err != nil {
		c.RenderWithErr(c.Tr("install.save_config_failed", err), INSTALL, &f)
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
				c.FormErr("AdminName", "AdminEmail")
				c.RenderWithErr(c.Tr("install.invalid_admin_setting", err), INSTALL, &f)
				return
			}
			log.Info("Admin account already exist")
			u, _ = models.GetUserByName(u.Name)
		}

		// Auto-login for admin
		c.Session.Set("uid", u.ID)
		c.Session.Set("uname", u.Name)
	}

	log.Info("First-time run install finished!")
	c.Flash.Success(c.Tr("install.install_success"))
	c.Redirect(f.AppUrl + "user/login")
}
