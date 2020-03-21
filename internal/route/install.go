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

	"github.com/gogs/git-module"
	"github.com/pkg/errors"
	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
)

const (
	INSTALL = "install"
)

func checkRunMode() {
	if conf.IsProdMode() {
		macaron.Env = macaron.PROD
		macaron.ColorLog = false
		git.SetOutput(nil)
	} else {
		git.SetOutput(os.Stdout)
	}
	log.Info("Run mode: %s", strings.Title(macaron.Env))
}

// GlobalInit is for global configuration reload-able.
func GlobalInit(customConf string) error {
	err := conf.Init(customConf)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}

	conf.InitLogging(false)
	log.Info("%s %s", conf.App.BrandName, conf.App.Version)
	log.Trace("Work directory: %s", conf.WorkDir())
	log.Trace("Custom path: %s", conf.CustomDir())
	log.Trace("Custom config: %s", conf.CustomConf)
	log.Trace("Log path: %s", conf.Log.RootPath)
	log.Trace("Build time: %s", conf.BuildTime)
	log.Trace("Build commit: %s", conf.BuildCommit)

	if conf.Email.Enabled {
		log.Trace("Email service is enabled")
	}

	email.NewContext()

	if conf.Security.InstallLock {
		highlight.NewContext()
		markup.NewSanitizer()
		if err := db.NewEngine(); err != nil {
			log.Fatal("Failed to initialize ORM engine: %v", err)
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
	if conf.HasMinWinSvc {
		log.Info("Builtin Windows Service is supported")
	}
	if conf.Server.LoadAssetsFromDisk {
		log.Trace("Assets are loaded from disk")
	}
	checkRunMode()

	if !conf.Security.InstallLock {
		return nil
	}

	if conf.SSH.StartBuiltinServer {
		ssh.Listen(conf.SSH.ListenHost, conf.SSH.ListenPort, conf.SSH.ServerCiphers)
		log.Info("SSH server started on %s:%v", conf.SSH.ListenHost, conf.SSH.ListenPort)
		log.Trace("SSH server cipher list: %v", conf.SSH.ServerCiphers)
	}

	if conf.SSH.RewriteAuthorizedKeysAtStart {
		if err := db.RewriteAuthorizedKeys(); err != nil {
			log.Warn("Failed to rewrite authorized_keys file: %v", err)
		}
	}

	return nil
}

func InstallInit(c *context.Context) {
	if conf.Security.InstallLock {
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
	f.DbHost = conf.Database.Host
	f.DbUser = conf.Database.User
	f.DbName = conf.Database.Name
	f.DbPath = conf.Database.Path

	c.Data["CurDbOption"] = "PostgreSQL"
	switch conf.Database.Type {
	case "mysql":
		c.Data["CurDbOption"] = "MySQL"
	case "mssql":
		c.Data["CurDbOption"] = "MSSQL"
	case "sqlite3":
		if db.EnableSQLite3 {
			c.Data["CurDbOption"] = "SQLite3"
		}
	}

	// Application general settings
	f.AppName = conf.App.BrandName
	f.RepoRootPath = conf.Repository.Root

	// Note(unknwon): it's hard for Windows users change a running user,
	// 	so just use current one if config says default.
	if conf.IsWindowsRuntime() && conf.App.RunUser == "git" {
		f.RunUser = osutil.CurrentUsername()
	} else {
		f.RunUser = conf.App.RunUser
	}

	f.Domain = conf.Server.Domain
	f.SSHPort = conf.SSH.Port
	f.UseBuiltinSSHServer = conf.SSH.StartBuiltinServer
	f.HTTPPort = conf.Server.HTTPPort
	f.AppUrl = conf.Server.ExternalURL
	f.LogRootPath = conf.Log.RootPath

	// E-mail service settings
	if conf.Email.Enabled {
		f.SMTPHost = conf.Email.Host
		f.SMTPFrom = conf.Email.From
		f.SMTPUser = conf.Email.User
	}
	f.RegisterConfirm = conf.Auth.RequireEmailConfirmation
	f.MailNotify = conf.User.EnableEmailNotification

	// Server and other services settings
	f.OfflineMode = conf.Server.OfflineMode
	f.DisableGravatar = conf.Picture.DisableGravatar
	f.EnableFederatedAvatar = conf.Picture.EnableFederatedAvatar
	f.DisableRegistration = conf.Auth.DisableRegistration
	f.EnableCaptcha = conf.Auth.EnableRegistrationCaptcha
	f.RequireSignInView = conf.Auth.RequireSigninView

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
	dbTypes := map[string]string{
		"PostgreSQL": "postgres",
		"MySQL":      "mysql",
		"MSSQL":      "mssql",
		"SQLite3":    "sqlite3",
	}
	conf.Database.Type = dbTypes[f.DbType]
	conf.Database.Host = f.DbHost
	conf.Database.User = f.DbUser
	conf.Database.Password = f.DbPasswd
	conf.Database.Name = f.DbName
	conf.Database.SSLMode = f.SSLMode
	conf.Database.Path = f.DbPath

	if conf.Database.Type == "sqlite3" && len(conf.Database.Path) == 0 {
		c.FormErr("DbPath")
		c.RenderWithErr(c.Tr("install.err_empty_db_path"), INSTALL, &f)
		return
	}

	// Set test engine.
	if err := db.NewTestEngine(); err != nil {
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

	currentUser, match := conf.CheckRunUser(f.RunUser)
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
	if osutil.IsFile(conf.CustomConf) {
		// Keeps custom settings if there is already something.
		if err := cfg.Append(conf.CustomConf); err != nil {
			log.Error("Failed to load custom conf %q: %v", conf.CustomConf, err)
		}
	}
	cfg.Section("database").Key("TYPE").SetValue(conf.Database.Type)
	cfg.Section("database").Key("HOST").SetValue(conf.Database.Host)
	cfg.Section("database").Key("NAME").SetValue(conf.Database.Name)
	cfg.Section("database").Key("USER").SetValue(conf.Database.User)
	cfg.Section("database").Key("PASSWORD").SetValue(conf.Database.Password)
	cfg.Section("database").Key("SSL_MODE").SetValue(conf.Database.SSLMode)
	cfg.Section("database").Key("PATH").SetValue(conf.Database.Path)

	cfg.Section("").Key("BRAND_NAME").SetValue(f.AppName)
	cfg.Section("repository").Key("ROOT").SetValue(f.RepoRootPath)
	cfg.Section("").Key("RUN_USER").SetValue(f.RunUser)
	cfg.Section("server").Key("DOMAIN").SetValue(f.Domain)
	cfg.Section("server").Key("HTTP_PORT").SetValue(f.HTTPPort)
	cfg.Section("server").Key("EXTERNAL_URL").SetValue(f.AppUrl)

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

	_ = os.MkdirAll(filepath.Dir(conf.CustomConf), os.ModePerm)
	if err := cfg.SaveTo(conf.CustomConf); err != nil {
		c.RenderWithErr(c.Tr("install.save_config_failed", err), INSTALL, &f)
		return
	}

	// NOTE: We reuse the current value because this handler does not have access to CLI flags.
	err = GlobalInit(conf.CustomConf)
	if err != nil {
		c.RenderWithErr(c.Tr("install.init_failed", err), INSTALL, &f)
		return
	}

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
				conf.Security.InstallLock = false
				c.FormErr("AdminName", "AdminEmail")
				c.RenderWithErr(c.Tr("install.invalid_admin_setting", err), INSTALL, &f)
				return
			}
			log.Info("Admin account already exist")
			u, _ = db.GetUserByName(u.Name)
		}

		// Auto-login for admin
		_ = c.Session.Set("uid", u.ID)
		_ = c.Session.Set("uname", u.Name)
	}

	log.Info("First-time run install finished!")
	c.Flash.Success(c.Tr("install.install_success"))
	c.Redirect(f.AppUrl + "user/login")
}
