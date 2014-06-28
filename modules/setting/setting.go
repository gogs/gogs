// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package setting

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

	"github.com/gogits/gogs/modules/bin"
	"github.com/gogits/gogs/modules/log"
)

type Scheme string

const (
	HTTP  Scheme = "http"
	HTTPS Scheme = "https"
)

var (
	// App settings.
	AppVer  string
	AppName string
	AppLogo string
	AppUrl  string

	// Server settings.
	Protocol           Scheme
	Domain             string
	HttpAddr, HttpPort string
	SshPort            int
	OfflineMode        bool
	DisableRouterLog   bool
	CertFile, KeyFile  string
	StaticRootPath     string

	// Security settings.
	InstallLock          bool
	SecretKey            string
	LogInRememberDays    int
	CookieUserName       string
	CookieRememberName   string
	ReverseProxyAuthUser string

	// Webhook settings.
	WebhookTaskInterval   int
	WebhookDeliverTimeout int

	// Repository settings.
	RepoRootPath string
	ScriptType   string

	// Picture settings.
	PictureService  string
	DisableGravatar bool

	// Log settings.
	LogRootPath string
	LogModes    []string
	LogConfigs  []string

	// Cache settings.
	Cache        cache.Cache
	CacheAdapter string
	CacheConfig  string

	EnableRedis    bool
	EnableMemcache bool

	// Session settings.
	SessionProvider string
	SessionConfig   *session.Config
	SessionManager  *session.Manager

	// Global setting objects.
	Cfg        *goconfig.ConfigFile
	CustomPath string // Custom directory path.
	ProdMode   bool
	RunUser    string
)

func ExecPath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	p, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	return p, nil
}

// WorkDir returns absolute path of work directory.
func WorkDir() (string, error) {
	execPath, err := ExecPath()
	return path.Dir(strings.Replace(execPath, "\\", "/", -1)), err
}

// NewConfigContext initializes configuration context.
// NOTE: do not print any log except error.
func NewConfigContext() {
	workDir, err := WorkDir()
	if err != nil {
		log.Fatal("Fail to get work directory: %v", err)
	}

	data, err := bin.Asset("conf/app.ini")
	if err != nil {
		log.Fatal("Fail to read 'conf/app.ini': %v", err)
	}
	Cfg, err = goconfig.LoadFromData(data)
	if err != nil {
		log.Fatal("Fail to parse 'conf/app.ini': %v", err)
	}

	CustomPath = os.Getenv("GOGS_CUSTOM")
	if len(CustomPath) == 0 {
		CustomPath = path.Join(workDir, "custom")
	}

	cfgPath := path.Join(CustomPath, "conf/app.ini")
	if com.IsFile(cfgPath) {
		if err = Cfg.AppendFiles(cfgPath); err != nil {
			log.Fatal("Fail to load custom 'conf/app.ini': %v", err)
		}
	} else {
		log.Warn("No custom 'conf/app.ini' found")
	}

	AppName = Cfg.MustValue("", "APP_NAME", "Gogs: Go Git Service")
	AppLogo = Cfg.MustValue("", "APP_LOGO", "img/favicon.png")
	AppUrl = Cfg.MustValue("server", "ROOT_URL", "http://localhost:3000")

	Protocol = HTTP
	if Cfg.MustValue("server", "PROTOCOL") == "https" {
		Protocol = HTTPS
		CertFile = Cfg.MustValue("server", "CERT_FILE")
		KeyFile = Cfg.MustValue("server", "KEY_FILE")
	}
	Domain = Cfg.MustValue("server", "DOMAIN", "localhost")
	HttpAddr = Cfg.MustValue("server", "HTTP_ADDR", "0.0.0.0")
	HttpPort = Cfg.MustValue("server", "HTTP_PORT", "3000")
	SshPort = Cfg.MustInt("server", "SSH_PORT", 22)
	OfflineMode = Cfg.MustBool("server", "OFFLINE_MODE")
	DisableRouterLog = Cfg.MustBool("server", "DISABLE_ROUTER_LOG")
	StaticRootPath = Cfg.MustValue("server", "STATIC_ROOT_PATH", workDir)
	LogRootPath = Cfg.MustValue("log", "ROOT_PATH", path.Join(workDir, "log"))

	InstallLock = Cfg.MustBool("security", "INSTALL_LOCK")
	SecretKey = Cfg.MustValue("security", "SECRET_KEY")
	LogInRememberDays = Cfg.MustInt("security", "LOGIN_REMEMBER_DAYS")
	CookieUserName = Cfg.MustValue("security", "COOKIE_USERNAME")
	CookieRememberName = Cfg.MustValue("security", "COOKIE_REMEMBER_NAME")
	ReverseProxyAuthUser = Cfg.MustValue("security", "REVERSE_PROXY_AUTHENTICATION_USER", "X-WEBAUTH-USER")

	RunUser = Cfg.MustValue("", "RUN_USER")
	curUser := os.Getenv("USER")
	if len(curUser) == 0 {
		curUser = os.Getenv("USERNAME")
	}
	// Does not check run user when the install lock is off.
	if InstallLock && RunUser != curUser {
		log.Fatal("Expect user(%s) but current user is: %s", RunUser, curUser)
	}

	// Determine and create root git reposiroty path.
	homeDir, err := com.HomeDir()
	if err != nil {
		log.Fatal("Fail to get home directory: %v", err)
	}
	RepoRootPath = Cfg.MustValue("repository", "ROOT", filepath.Join(homeDir, "gogs-repositories"))
	if !filepath.IsAbs(RepoRootPath) {
		RepoRootPath = filepath.Join(workDir, RepoRootPath)
	} else {
		RepoRootPath = filepath.Clean(RepoRootPath)
	}

	if err = os.MkdirAll(RepoRootPath, os.ModePerm); err != nil {
		log.Fatal("Fail to create repository root path(%s): %v", RepoRootPath, err)
	}
	ScriptType = Cfg.MustValue("repository", "SCRIPT_TYPE", "bash")

	PictureService = Cfg.MustValueRange("picture", "SERVICE", "server",
		[]string{"server"})
	DisableGravatar = Cfg.MustBool("picture", "DISABLE_GRAVATAR")
}

var Service struct {
	RegisterEmailConfirm   bool
	DisableRegistration    bool
	RequireSignInView      bool
	EnableCacheAvatar      bool
	EnableNotifyMail       bool
	EnableReverseProxyAuth bool
	LdapAuth               bool
	ActiveCodeLives        int
	ResetPwdCodeLives      int
}

func newService() {
	Service.ActiveCodeLives = Cfg.MustInt("service", "ACTIVE_CODE_LIVE_MINUTES", 180)
	Service.ResetPwdCodeLives = Cfg.MustInt("service", "RESET_PASSWD_CODE_LIVE_MINUTES", 180)
	Service.DisableRegistration = Cfg.MustBool("service", "DISABLE_REGISTRATION")
	Service.RequireSignInView = Cfg.MustBool("service", "REQUIRE_SIGNIN_VIEW")
	Service.EnableCacheAvatar = Cfg.MustBool("service", "ENABLE_CACHE_AVATAR")
	Service.EnableReverseProxyAuth = Cfg.MustBool("service", "ENABLE_REVERSE_PROXY_AUTHENTICATION")
}

var logLevels = map[string]string{
	"Trace":    "0",
	"Debug":    "1",
	"Info":     "2",
	"Warn":     "3",
	"Error":    "4",
	"Critical": "5",
}

func newLogService() {
	log.Info("%s %s", AppName, AppVer)

	// Get and check log mode.
	LogModes = strings.Split(Cfg.MustValue("log", "MODE", "console"), ",")
	LogConfigs = make([]string, len(LogModes))
	for i, mode := range LogModes {
		mode = strings.TrimSpace(mode)
		modeSec := "log." + mode
		if _, err := Cfg.GetSection(modeSec); err != nil {
			log.Fatal("Unknown log mode: %s", mode)
		}

		// Log level.
		levelName := Cfg.MustValueRange("log."+mode, "LEVEL", "Trace",
			[]string{"Trace", "Debug", "Info", "Warn", "Error", "Critical"})
		level, ok := logLevels[levelName]
		if !ok {
			log.Fatal("Unknown log level: %s", levelName)
		}

		// Generate log configuration.
		switch mode {
		case "console":
			LogConfigs[i] = fmt.Sprintf(`{"level":%s}`, level)
		case "file":
			logPath := Cfg.MustValue(modeSec, "FILE_NAME", path.Join(LogRootPath, "gogs.log"))
			os.MkdirAll(path.Dir(logPath), os.ModePerm)
			LogConfigs[i] = fmt.Sprintf(
				`{"level":%s,"filename":"%s","rotate":%v,"maxlines":%d,"maxsize":%d,"daily":%v,"maxdays":%d}`, level,
				logPath,
				Cfg.MustBool(modeSec, "LOG_ROTATE", true),
				Cfg.MustInt(modeSec, "MAX_LINES", 1000000),
				1<<uint(Cfg.MustInt(modeSec, "MAX_SIZE_SHIFT", 28)),
				Cfg.MustBool(modeSec, "DAILY_ROTATE", true),
				Cfg.MustInt(modeSec, "MAX_DAYS", 7))
		case "conn":
			LogConfigs[i] = fmt.Sprintf(`{"level":%s,"reconnectOnMsg":%v,"reconnect":%v,"net":"%s","addr":"%s"}`, level,
				Cfg.MustBool(modeSec, "RECONNECT_ON_MSG"),
				Cfg.MustBool(modeSec, "RECONNECT"),
				Cfg.MustValueRange(modeSec, "PROTOCOL", "tcp", []string{"tcp", "unix", "udp"}),
				Cfg.MustValue(modeSec, "ADDR", ":7020"))
		case "smtp":
			LogConfigs[i] = fmt.Sprintf(`{"level":%s,"username":"%s","password":"%s","host":"%s","sendTos":"%s","subject":"%s"}`, level,
				Cfg.MustValue(modeSec, "USER", "example@example.com"),
				Cfg.MustValue(modeSec, "PASSWD", "******"),
				Cfg.MustValue(modeSec, "HOST", "127.0.0.1:25"),
				Cfg.MustValue(modeSec, "RECEIVERS", "[]"),
				Cfg.MustValue(modeSec, "SUBJECT", "Diagnostic message from serve"))
		case "database":
			LogConfigs[i] = fmt.Sprintf(`{"level":%s,"driver":"%s","conn":"%s"}`, level,
				Cfg.MustValue(modeSec, "DRIVER"),
				Cfg.MustValue(modeSec, "CONN"))
		}

		log.NewLogger(Cfg.MustInt64("log", "BUFFER_LEN", 10000), mode, LogConfigs[i])
		log.Info("Log Mode: %s(%s)", strings.Title(mode), levelName)
	}
}

func newCacheService() {
	CacheAdapter = Cfg.MustValueRange("cache", "ADAPTER", "memory", []string{"memory", "redis", "memcache"})
	if EnableRedis {
		log.Info("Redis Enabled")
	}
	if EnableMemcache {
		log.Info("Memcache Enabled")
	}

	switch CacheAdapter {
	case "memory":
		CacheConfig = fmt.Sprintf(`{"interval":%d}`, Cfg.MustInt("cache", "INTERVAL", 60))
	case "redis", "memcache":
		CacheConfig = fmt.Sprintf(`{"conn":"%s"}`, Cfg.MustValue("cache", "HOST"))
	default:
		log.Fatal("Unknown cache adapter: %s", CacheAdapter)
	}

	var err error
	Cache, err = cache.NewCache(CacheAdapter, CacheConfig)
	if err != nil {
		log.Fatal("Init cache system failed, adapter: %s, config: %s, %v\n",
			CacheAdapter, CacheConfig, err)
	}

	log.Info("Cache Service Enabled")
}

func newSessionService() {
	SessionProvider = Cfg.MustValueRange("session", "PROVIDER", "memory",
		[]string{"memory", "file", "redis", "mysql"})

	SessionConfig = new(session.Config)
	SessionConfig.ProviderConfig = Cfg.MustValue("session", "PROVIDER_CONFIG")
	SessionConfig.CookieName = Cfg.MustValue("session", "COOKIE_NAME", "i_like_gogits")
	SessionConfig.CookieSecure = Cfg.MustBool("session", "COOKIE_SECURE")
	SessionConfig.EnableSetCookie = Cfg.MustBool("session", "ENABLE_SET_COOKIE", true)
	SessionConfig.GcIntervalTime = Cfg.MustInt64("session", "GC_INTERVAL_TIME", 86400)
	SessionConfig.SessionLifeTime = Cfg.MustInt64("session", "SESSION_LIFE_TIME", 86400)
	SessionConfig.SessionIDHashFunc = Cfg.MustValueRange("session", "SESSION_ID_HASHFUNC",
		"sha1", []string{"sha1", "sha256", "md5"})
	SessionConfig.SessionIDHashKey = Cfg.MustValue("session", "SESSION_ID_HASHKEY")

	if SessionProvider == "file" {
		os.MkdirAll(path.Dir(SessionConfig.ProviderConfig), os.ModePerm)
	}

	var err error
	SessionManager, err = session.NewManager(SessionProvider, *SessionConfig)
	if err != nil {
		log.Fatal("Init session system failed, provider: %s, %v",
			SessionProvider, err)
	}

	log.Info("Session Service Enabled")
}

// Mailer represents mail service.
type Mailer struct {
	Name         string
	Host         string
	From         string
	User, Passwd string
}

type OauthInfo struct {
	ClientId, ClientSecret string
	Scopes                 string
	AuthUrl, TokenUrl      string
}

// Oauther represents oauth service.
type Oauther struct {
	GitHub, Google, Tencent,
	Twitter, Weibo bool
	OauthInfos map[string]*OauthInfo
}

var (
	MailService  *Mailer
	OauthService *Oauther
)

func newMailService() {
	// Check mailer setting.
	if !Cfg.MustBool("mailer", "ENABLED") {
		return
	}

	MailService = &Mailer{
		Name:   Cfg.MustValue("mailer", "NAME", AppName),
		Host:   Cfg.MustValue("mailer", "HOST"),
		User:   Cfg.MustValue("mailer", "USER"),
		Passwd: Cfg.MustValue("mailer", "PASSWD"),
	}
	MailService.From = Cfg.MustValue("mailer", "FROM", MailService.User)
	log.Info("Mail Service Enabled")
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
	Service.EnableNotifyMail = true
	log.Info("Notify Mail Service Enabled")
}

func newWebhookService() {
	WebhookTaskInterval = Cfg.MustInt("webhook", "TASK_INTERVAL", 1)
	WebhookDeliverTimeout = Cfg.MustInt("webhook", "DELIVER_TIMEOUT", 5)
}

func NewServices() {
	newService()
	newLogService()
	newCacheService()
	newSessionService()
	newMailService()
	newRegisterMailService()
	newNotifyMailService()
	newWebhookService()
}
