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

	Cfg         *goconfig.ConfigFile
	MailService *Mailer

	Cache        cache.Cache
	CacheAdapter string
	CacheConfig  string
)

var Service struct {
	RegisterEmailConfirm   bool
	DisenableRegisteration bool
	RequireSignInView      bool
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
}

func newLogService() {
	// Get and check log mode.
	mode := Cfg.MustValue("log", "MODE", "console")
	modeSec := "log." + mode
	if _, err := Cfg.GetSection(modeSec); err != nil {
		fmt.Printf("Unknown log mode: %s\n", mode)
		os.Exit(2)
	}

	// Log level.
	levelName := Cfg.MustValue("log."+mode, "LEVEL", "Trace")
	level, ok := logLevels[levelName]
	if !ok {
		fmt.Printf("Unknown log level: %s\n", levelName)
		os.Exit(2)
	}

	// Generate log configuration.
	var config string
	switch mode {
	case "console":
		config = fmt.Sprintf(`{"level":%s}`, level)
	case "file":
		logPath := Cfg.MustValue(modeSec, "FILE_NAME", "log/gogs.log")
		os.MkdirAll(path.Dir(logPath), os.ModePerm)
		config = fmt.Sprintf(
			`{"level":%s,"filename":%s,"rotate":%v,"maxlines":%d,"maxsize",%d,"daily":%v,"maxdays":%d}`, level,
			logPath,
			Cfg.MustBool(modeSec, "LOG_ROTATE", true),
			Cfg.MustInt(modeSec, "MAX_LINES", 1000000),
			1<<uint(Cfg.MustInt(modeSec, "MAX_SIZE_SHIFT", 28)),
			Cfg.MustBool(modeSec, "DAILY_ROTATE", true),
			Cfg.MustInt(modeSec, "MAX_DAYS", 7))
	case "conn":
		config = fmt.Sprintf(`{"level":%s,"reconnectOnMsg":%v,"reconnect":%v,"net":%s,"addr":%s}`, level,
			Cfg.MustBool(modeSec, "RECONNECT_ON_MSG", false),
			Cfg.MustBool(modeSec, "RECONNECT", false),
			Cfg.MustValue(modeSec, "PROTOCOL", "tcp"),
			Cfg.MustValue(modeSec, "ADDR", ":7020"))
	case "smtp":
		config = fmt.Sprintf(`{"level":%s,"username":%s,"password":%s,"host":%s,"sendTos":%s,"subject":%s}`, level,
			Cfg.MustValue(modeSec, "USER", "example@example.com"),
			Cfg.MustValue(modeSec, "PASSWD", "******"),
			Cfg.MustValue(modeSec, "HOST", "127.0.0.1:25"),
			Cfg.MustValue(modeSec, "RECEIVERS", "[]"),
			Cfg.MustValue(modeSec, "SUBJECT", "Diagnostic message from serve"))
	}

	log.NewLogger(Cfg.MustInt64("log", "BUFFER_LEN", 10000), mode, config)
	log.Info("Log Mode: %s(%s)", strings.Title(mode), levelName)
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

func NewConfigContext() {
	var err error
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

	CacheAdapter = Cfg.MustValue("cache", "ADAPTER")
	CacheConfig = Cfg.MustValue("cache", "CONFIG")

	Cache, err = cache.NewCache(CacheAdapter, CacheConfig)
	if err != nil {
		fmt.Printf("Init cache system failed, adapter: %s, config: %s, %v\n",
			CacheAdapter, CacheConfig, err)
		os.Exit(2)
	}

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
	newMailService()
	newRegisterMailService()
}
