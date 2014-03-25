// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"

	"github.com/gogits/cache"
	"github.com/gogits/session"

	"github.com/gogits/gogs/modules/log"
)

// Mailer represents a mail service.
type Mailer struct {
	Name         string
	Host         string
	User, Passwd string
}

var (
	AppVer       string
	AppName      string
	AppLogo      string
	AppUrl       string
	Domain       string
	SecretKey    string
	RunUser      string
	RepoRootPath string

	EnableHttpsClone bool

	LogInRememberDays  int
	CookieUserName     string
	CookieRememberName string

	Cfg         *goconfig.ConfigFile
	MailService *Mailer

	LogMode   string
	LogConfig string

	Cache        cache.Cache
	CacheAdapter string
	CacheConfig  string

	SessionProvider string
	SessionConfig   *session.Config
	SessionManager  *session.Manager

	PictureService string
)

var Service struct {
	RegisterEmailConfirm   bool
	DisenableRegisteration bool
	RequireSignInView      bool
	EnableCacheAvatar      bool
	NotifyMail             bool
	ActiveCodeLives        int
	ResetPwdCodeLives      int
}

func exeDir() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	p, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	return path.Dir(p), nil
}

var logLevels = map[string]string{
	"Trace":    "0",
	"Debug":    "1",
	"Info":     "2",
	"Warn":     "3",
	"Error":    "4",
	"Critical": "5",
}

func newService() {
	Service.ActiveCodeLives = Cfg.MustInt("service", "ACTIVE_CODE_LIVE_MINUTES", 180)
	Service.ResetPwdCodeLives = Cfg.MustInt("service", "RESET_PASSWD_CODE_LIVE_MINUTES", 180)
	Service.DisenableRegisteration = Cfg.MustBool("service", "DISENABLE_REGISTERATION", false)
	Service.RequireSignInView = Cfg.MustBool("service", "REQUIRE_SIGNIN_VIEW", false)
	Service.EnableCacheAvatar = Cfg.MustBool("service", "ENABLE_CACHE_AVATAR", false)
}

func newLogService() {
	// Get and check log mode.
	LogMode = Cfg.MustValue("log", "MODE", "console")
	modeSec := "log." + LogMode
	if _, err := Cfg.GetSection(modeSec); err != nil {
		fmt.Printf("Unknown log mode: %s\n", LogMode)
		os.Exit(2)
	}

	// Log level.
	levelName := Cfg.MustValue("log."+LogMode, "LEVEL", "Trace")
	level, ok := logLevels[levelName]
	if !ok {
		fmt.Printf("Unknown log level: %s\n", levelName)
		os.Exit(2)
	}

	// Generate log configuration.
	switch LogMode {
	case "console":
		LogConfig = fmt.Sprintf(`{"level":%s}`, level)
	case "file":
		logPath := Cfg.MustValue(modeSec, "FILE_NAME", "log/gogs.log")
		os.MkdirAll(path.Dir(logPath), os.ModePerm)
		LogConfig = fmt.Sprintf(
			`{"level":%s,"filename":"%s","rotate":%v,"maxlines":%d,"maxsize":%d,"daily":%v,"maxdays":%d}`, level,
			logPath,
			Cfg.MustBool(modeSec, "LOG_ROTATE", true),
			Cfg.MustInt(modeSec, "MAX_LINES", 1000000),
			1<<uint(Cfg.MustInt(modeSec, "MAX_SIZE_SHIFT", 28)),
			Cfg.MustBool(modeSec, "DAILY_ROTATE", true),
			Cfg.MustInt(modeSec, "MAX_DAYS", 7))
	case "conn":
		LogConfig = fmt.Sprintf(`{"level":"%s","reconnectOnMsg":%v,"reconnect":%v,"net":"%s","addr":"%s"}`, level,
			Cfg.MustBool(modeSec, "RECONNECT_ON_MSG", false),
			Cfg.MustBool(modeSec, "RECONNECT", false),
			Cfg.MustValue(modeSec, "PROTOCOL", "tcp"),
			Cfg.MustValue(modeSec, "ADDR", ":7020"))
	case "smtp":
		LogConfig = fmt.Sprintf(`{"level":"%s","username":"%s","password":"%s","host":"%s","sendTos":"%s","subject":"%s"}`, level,
			Cfg.MustValue(modeSec, "USER", "example@example.com"),
			Cfg.MustValue(modeSec, "PASSWD", "******"),
			Cfg.MustValue(modeSec, "HOST", "127.0.0.1:25"),
			Cfg.MustValue(modeSec, "RECEIVERS", "[]"),
			Cfg.MustValue(modeSec, "SUBJECT", "Diagnostic message from serve"))
	case "database":
		LogConfig = fmt.Sprintf(`{"level":"%s","driver":"%s","conn":"%s"}`, level,
			Cfg.MustValue(modeSec, "Driver"),
			Cfg.MustValue(modeSec, "CONN"))
	}

	log.NewLogger(Cfg.MustInt64("log", "BUFFER_LEN", 10000), LogMode, LogConfig)
	log.Info("Log Mode: %s(%s)", strings.Title(LogMode), levelName)
}

func newCacheService() {
	CacheAdapter = Cfg.MustValue("cache", "ADAPTER", "memory")

	switch CacheAdapter {
	case "memory":
		CacheConfig = fmt.Sprintf(`{"interval":%d}`, Cfg.MustInt("cache", "INTERVAL", 60))
	case "redis", "memcache":
		CacheConfig = fmt.Sprintf(`{"conn":"%s"}`, Cfg.MustValue("cache", "HOST"))
	default:
		fmt.Printf("Unknown cache adapter: %s\n", CacheAdapter)
		os.Exit(2)
	}

	var err error
	Cache, err = cache.NewCache(CacheAdapter, CacheConfig)
	if err != nil {
		fmt.Printf("Init cache system failed, adapter: %s, config: %s, %v\n",
			CacheAdapter, CacheConfig, err)
		os.Exit(2)
	}

	log.Info("Cache Service Enabled")
}

func newSessionService() {
	SessionProvider = Cfg.MustValue("session", "PROVIDER", "memory")

	SessionConfig = new(session.Config)
	SessionConfig.ProviderConfig = Cfg.MustValue("session", "PROVIDER_CONFIG")
	SessionConfig.CookieName = Cfg.MustValue("session", "COOKIE_NAME", "i_like_gogits")
	SessionConfig.CookieSecure = Cfg.MustBool("session", "COOKIE_SECURE")
	SessionConfig.EnableSetCookie = Cfg.MustBool("session", "ENABLE_SET_COOKIE", true)
	SessionConfig.GcIntervalTime = Cfg.MustInt64("session", "GC_INTERVAL_TIME", 86400)
	SessionConfig.SessionLifeTime = Cfg.MustInt64("session", "SESSION_LIFE_TIME", 86400)
	SessionConfig.SessionIDHashFunc = Cfg.MustValue("session", "SESSION_ID_HASHFUNC", "sha1")
	SessionConfig.SessionIDHashKey = Cfg.MustValue("session", "SESSION_ID_HASHKEY")

	if SessionProvider == "file" {
		os.MkdirAll(path.Dir(SessionConfig.ProviderConfig), os.ModePerm)
	}

	var err error
	SessionManager, err = session.NewManager(SessionProvider, *SessionConfig)
	if err != nil {
		fmt.Printf("Init session system failed, provider: %s, %v\n",
			SessionProvider, err)
		os.Exit(2)
	}

	log.Info("Session Service Enabled")
}

func newMailService() {
	// Check mailer setting.
	if Cfg.MustBool("mailer", "ENABLED") {
		MailService = &Mailer{
			Name:   Cfg.MustValue("mailer", "NAME", AppName),
			Host:   Cfg.MustValue("mailer", "HOST", "127.0.0.1:25"),
			User:   Cfg.MustValue("mailer", "USER", "example@example.com"),
			Passwd: Cfg.MustValue("mailer", "PASSWD", "******"),
		}
		log.Info("Mail Service Enabled")
	}
}

func newRegisterMailService() {
	if !Cfg.MustBool("service", "REGISTER_EMAIL_CONFIRM") {
		return
	} else if MailService == nil {
		log.Warn("Register Mail Service: Mail Service is not enabled")
		return
	}
	Service.RegisterEmailConfirm = true
	log.Info("Register Mail Service Enabled")
}

func newNotifyMailService() {
	if !Cfg.MustBool("service", "ENABLE_NOTIFY_MAIL") {
		return
	} else if MailService == nil {
		log.Warn("Notify Mail Service: Mail Service is not enabled")
		return
	}
	Service.NotifyMail = true
	log.Info("Notify Mail Service Enabled")
}

func NewConfigContext() {
	//var err error
	workDir, err := exeDir()
	if err != nil {
		fmt.Printf("Fail to get work directory: %s\n", err)
		os.Exit(2)
	}

	cfgPath := filepath.Join(workDir, "conf/app.ini")
	Cfg, err = goconfig.LoadConfigFile(cfgPath)
	if err != nil {
		fmt.Printf("Cannot load config file '%s'\n", cfgPath)
		os.Exit(2)
	}
	Cfg.BlockMode = false

	cfgPath = filepath.Join(workDir, "custom/conf/app.ini")
	if com.IsFile(cfgPath) {
		if err = Cfg.AppendFiles(cfgPath); err != nil {
			fmt.Printf("Cannot load config file '%s'\n", cfgPath)
			os.Exit(2)
		}
	}

	AppName = Cfg.MustValue("", "APP_NAME", "Gogs: Go Git Service")
	AppLogo = Cfg.MustValue("", "APP_LOGO", "img/favicon.png")
	AppUrl = Cfg.MustValue("server", "ROOT_URL")
	Domain = Cfg.MustValue("server", "DOMAIN")
	SecretKey = Cfg.MustValue("security", "SECRET_KEY")

	RunUser = Cfg.MustValue("", "RUN_USER")
	curUser := os.Getenv("USERNAME")
	if len(curUser) == 0 {
		curUser = os.Getenv("whoami")
	}
	if RunUser != curUser {
		fmt.Printf("Expect user(%s) but current user is: %s\n", RunUser, curUser)
		os.Exit(2)
	}

	EnableHttpsClone = Cfg.MustBool("security", "ENABLE_HTTPS_CLONE", false)

	LogInRememberDays = Cfg.MustInt("security", "LOGIN_REMEMBER_DAYS")
	CookieUserName = Cfg.MustValue("security", "COOKIE_USERNAME")
	CookieRememberName = Cfg.MustValue("security", "COOKIE_REMEMBER_NAME")

	PictureService = Cfg.MustValue("picture", "SERVICE")

	// Determine and create root git reposiroty path.
	RepoRootPath = Cfg.MustValue("repository", "ROOT")
	if err = os.MkdirAll(RepoRootPath, os.ModePerm); err != nil {
		fmt.Printf("models.init(fail to create RepoRootPath(%s)): %v\n", RepoRootPath, err)
		os.Exit(2)
	}
}

func NewServices() {
	newService()
	newLogService()
	newCacheService()
	newSessionService()
	newMailService()
	newRegisterMailService()
	newNotifyMailService()
}
