// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package route

import (
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unknwon/com"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"
	"gopkg.in/macaron.v1"
	"xorm.io/xorm"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/mailer"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/setting"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/user"
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
	log.Info("Run mode: %s", strings.Title(macaron.Env))
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
	db.LoadConfigs()
	NewServices()

	if setting.InstallLock {
		highlight.NewContext()
		markup.NewSanitizer()
		if err := db.NewEngine(); err != nil {
			log.Fatal(2, "Fail to initialize ORM engine: %v", err)
		}
		db.HasEngine = true

		db.LoadAuthSources()
		db.LoadRepoConfig()
		db.NewRepoContext()

		// Booting long running goroutines.
		cron.NewContext()
		db.InitSyncMirrors()
		db.InitDeliverHooks()
		db.InitTestPullRequests()
	}
	if db.EnableSQLite3 {
		log.Info("SQLite3 is supported")
	}
	if setting.SupportMiniWinService {
		log.Info("Builtin Windows Service is supported")
	}
	if setting.LoadAssetsFromDisk {
		log.Info("Assets are loaded from disk")
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
		if err := db.RewriteAuthorizedKeys(); err != nil {
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
	if db.EnableSQLite3 {
		dbOpts = append(dbOpts, "SQLite3")
	}
	c.Data["DbOptions"] = dbOpts
}

func Install(c *context.Context) {
	f := form.Install{}

	// Database settings
	f.DbHost = db.DbCfg.Host
	f.DbUser = db.DbCfg.User
	f.DbName = db.DbCfg.Name
	f.DbPath = db.DbCfg.Path

	c.Data["CurDbOption"] = "MySQL"
	switch db.DbCfg.Type {
	case "postgres":
		c.Data["CurDbOption"] = "PostgreSQL"
	case "mssql":
		c.Data["CurDbOption"] = "MSSQL"
	case "sqlite3":
		if db.EnableSQLite3 {
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
	db.DbCfg.Type = dbTypes[f.DbType]
	db.DbCfg.Host = f.DbHost
	db.DbCfg.User = f.DbUser
	db.DbCfg.Passwd = f.DbPasswd
	db.DbCfg.Name = f.DbName
	db.DbCfg.SSLMode = f.SSLMode
	db.DbCfg.Path = f.DbPath

	if db.DbCfg.Type == "sqlite3" && len(db.DbCfg.Path) == 0 {
		c.FormErr("DbPath")
		c.RenderWithErr(c.Tr("install.err_empty_db_path"), INSTALL, &f)
		return
	}

	// Set test engine.
	var x *xorm.Engine
	if err := db.NewTestEngine(x); err != nil {
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
	cfg.Section("database").Key("DB_TYPE").SetValue(db.DbCfg.Type)
	cfg.Section("database").Key("HOST").SetValue(db.DbCfg.Host)
	cfg.Section("database").Key("NAME").SetValue(db.DbCfg.Name)
	cfg.Section("database").Key("USER").SetValue(db.DbCfg.User)
	cfg.Section("database").Key("PASSWD").SetValue(db.DbCfg.Passwd)
	cfg.Section("database").Key("SSL_MODE").SetValue(db.DbCfg.SSLMode)
	cfg.Section("database").Key("PATH").SetValue(db.DbCfg.Path)

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
		u := &db.User{
			Name:     f.AdminName,
			Email:    f.AdminEmail,
			Passwd:   f.AdminPasswd,
			IsAdmin:  true,
			IsActive: true,
		}
		if err := db.CreateUser(u); err != nil {
			if !db.IsErrUserAlreadyExist(err) {
				setting.InstallLock = false
				c.FormErr("AdminName", "AdminEmail")
				c.RenderWithErr(c.Tr("install.invalid_admin_setting", err), INSTALL, &f)
				return
			}
			log.Info("Admin account already exist")
			u, _ = db.GetUserByName(u.Name)
		}

		// Auto-login for admin
		c.Session.Set("uid", u.ID)
		c.Session.Set("uname", u.Name)
	}

	log.Info("First-time run install finished!")
	c.Flash.Success(c.Tr("install.install_success"))
	c.Redirect(f.AppUrl + "user/login")
}
