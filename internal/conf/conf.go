// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"net/mail"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-macaron/cache/memcache"
	_ "github.com/go-macaron/cache/redis"
	"github.com/go-macaron/session"
	_ "github.com/go-macaron/session/redis"
	"github.com/mcuadros/go-version"
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"

	"github.com/gogs/go-libravatar"

	"gogs.io/gogs/internal/assets/conf"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/user"
)

func init() {
	// Initialize the primary logger until logging service is up.
	err := log.NewConsole()
	if err != nil {
		panic("init console logger: " + err.Error())
	}
}

// Asset is a wrapper for getting conf assets.
func Asset(name string) ([]byte, error) {
	return conf.Asset(name)
}

// AssetDir is a wrapper for getting conf assets.
func AssetDir(name string) ([]string, error) {
	return conf.AssetDir(name)
}

// MustAsset is a wrapper for getting conf assets.
func MustAsset(name string) []byte {
	return conf.MustAsset(name)
}

// File is the configuration object.
var File *ini.File

// Init initializes configuration from conf assets and given custom configuration file.
// If `customConf` is empty, it falls back to default location, i.e. "<WORK DIR>/custom".
// It is safe to call this function multiple times with desired `customConf`, but it is
// not concurrent safe.
//
// ⚠️ WARNING: Do not print anything in this function other than wanrings.
func Init(customConf string) error {
	var err error
	File, err = ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, conf.MustAsset("conf/app.ini"))
	if err != nil {
		return errors.Wrap(err, "parse 'conf/app.ini'")
	}
	File.NameMapper = ini.SnackCase

	if customConf == "" {
		customConf = filepath.Join(CustomDir(), "conf", "app.ini")
	} else {
		customConf, err = filepath.Abs(customConf)
		if err != nil {
			return errors.Wrap(err, "get absolute path")
		}
	}
	CustomConf = customConf

	if osutil.IsFile(customConf) {
		if err = File.Append(customConf); err != nil {
			return errors.Wrapf(err, "append %q", customConf)
		}
	} else {
		log.Warn("Custom config %q not found. Ignore this warning if you're running for the first time", customConf)
	}

	if err = File.Section(ini.DefaultSection).MapTo(&App); err != nil {
		return errors.Wrap(err, "mapping default section")
	}

	// ***************************
	// ----- Server settings -----
	// ***************************

	if err = File.Section("server").MapTo(&Server); err != nil {
		return errors.Wrap(err, "mapping [server] section")
	}
	Server.AppDataPath = ensureAbs(Server.AppDataPath)

	if !strings.HasSuffix(Server.ExternalURL, "/") {
		Server.ExternalURL += "/"
	}
	Server.URL, err = url.Parse(Server.ExternalURL)
	if err != nil {
		return errors.Wrapf(err, "parse '[server] EXTERNAL_URL' %q", err)
	}

	// Subpath should start with '/' and end without '/', i.e. '/{subpath}'.
	Server.Subpath = strings.TrimRight(Server.URL.Path, "/")
	Server.SubpathDepth = strings.Count(Server.Subpath, "/")

	unixSocketMode, err := strconv.ParseUint(Server.UnixSocketPermission, 8, 32)
	if err != nil {
		return errors.Wrapf(err, "parse '[server] UNIX_SOCKET_PERMISSION' %q", Server.UnixSocketPermission)
	}
	if unixSocketMode > 0777 {
		unixSocketMode = 0666
	}
	Server.UnixSocketMode = os.FileMode(unixSocketMode)

	// ************************
	// ----- SSH settings -----
	// ************************

	SSH.RootPath = filepath.Join(HomeDir(), ".ssh")
	SSH.KeyTestPath = os.TempDir()
	if err = File.Section("server").MapTo(&SSH); err != nil {
		return errors.Wrap(err, "mapping SSH settings from [server] section")
	}
	SSH.RootPath = ensureAbs(SSH.RootPath)
	SSH.KeyTestPath = ensureAbs(SSH.KeyTestPath)

	if !SSH.Disabled {
		if !SSH.StartBuiltinServer {
			if err := os.MkdirAll(SSH.RootPath, 0700); err != nil {
				return errors.Wrap(err, "create SSH root directory")
			} else if err = os.MkdirAll(SSH.KeyTestPath, 0644); err != nil {
				return errors.Wrap(err, "create SSH key test directory")
			}
		} else {
			SSH.RewriteAuthorizedKeysAtStart = false
		}

		// Check if server is eligible for minimum key size check when user choose to enable.
		// Windows server and OpenSSH version lower than 5.1 are forced to be disabled because
		// the "ssh-keygen" in Windows does not print key type.
		// See https://github.com/gogs/gogs/issues/4507.
		if SSH.MinimumKeySizeCheck {
			sshVersion, err := openSSHVersion()
			if err != nil {
				return errors.Wrap(err, "get OpenSSH version")
			}

			if IsWindowsRuntime() || version.Compare(sshVersion, "5.1", "<") {
				log.Warn(`SSH minimum key size check is forced to be disabled because server is not eligible:
	1. Windows server
	2. OpenSSH version is lower than 5.1`)
			} else {
				SSH.MinimumKeySizes = map[string]int{}
				for _, key := range File.Section("ssh.minimum_key_sizes").Keys() {
					if key.MustInt() != -1 {
						SSH.MinimumKeySizes[strings.ToLower(key.Name())] = key.MustInt()
					}
				}
			}
		}
	}

	// *******************************
	// ----- Repository settings -----
	// *******************************

	Repository.Root = filepath.Join(HomeDir(), "gogs-repositories")
	if err = File.Section("repository").MapTo(&Repository); err != nil {
		return errors.Wrap(err, "mapping [repository] section")
	}
	Repository.Root = ensureAbs(Repository.Root)
	Repository.Upload.TempPath = ensureAbs(Repository.Upload.TempPath)

	// *******************************
	// ----- Database settings -----
	// *******************************

	if err = File.Section("database").MapTo(&Database); err != nil {
		return errors.Wrap(err, "mapping [database] section")
	}
	Database.Path = ensureAbs(Database.Path)

	handleDeprecated()

	// TODO

	sec := File.Section("security")
	InstallLock = sec.Key("INSTALL_LOCK").MustBool()
	SecretKey = sec.Key("SECRET_KEY").String()
	LoginRememberDays = sec.Key("LOGIN_REMEMBER_DAYS").MustInt()
	CookieUserName = sec.Key("COOKIE_USERNAME").String()
	CookieRememberName = sec.Key("COOKIE_REMEMBER_NAME").String()
	CookieSecure = sec.Key("COOKIE_SECURE").MustBool(false)
	ReverseProxyAuthUser = sec.Key("REVERSE_PROXY_AUTHENTICATION_USER").MustString("X-WEBAUTH-USER")
	EnableLoginStatusCookie = sec.Key("ENABLE_LOGIN_STATUS_COOKIE").MustBool(false)
	LoginStatusCookieName = sec.Key("LOGIN_STATUS_COOKIE_NAME").MustString("login_status")

	// Does not check run user when the install lock is off.
	if InstallLock {
		currentUser, match := IsRunUserMatchCurrentUser(App.RunUser)
		if !match {
			log.Fatal("The user configured to run Gogs is %q, but the current user is %q", App.RunUser, currentUser)
		}
	}

	sec = File.Section("attachment")
	AttachmentPath = sec.Key("PATH").MustString(filepath.Join(Server.AppDataPath, "attachments"))
	if !filepath.IsAbs(AttachmentPath) {
		AttachmentPath = path.Join(workDir, AttachmentPath)
	}
	AttachmentAllowedTypes = strings.Replace(sec.Key("ALLOWED_TYPES").MustString("image/jpeg,image/png"), "|", ",", -1)
	AttachmentMaxSize = sec.Key("MAX_SIZE").MustInt64(4)
	AttachmentMaxFiles = sec.Key("MAX_FILES").MustInt(5)
	AttachmentEnabled = sec.Key("ENABLED").MustBool(true)

	TimeFormat = map[string]string{
		"ANSIC":       time.ANSIC,
		"UnixDate":    time.UnixDate,
		"RubyDate":    time.RubyDate,
		"RFC822":      time.RFC822,
		"RFC822Z":     time.RFC822Z,
		"RFC850":      time.RFC850,
		"RFC1123":     time.RFC1123,
		"RFC1123Z":    time.RFC1123Z,
		"RFC3339":     time.RFC3339,
		"RFC3339Nano": time.RFC3339Nano,
		"Kitchen":     time.Kitchen,
		"Stamp":       time.Stamp,
		"StampMilli":  time.StampMilli,
		"StampMicro":  time.StampMicro,
		"StampNano":   time.StampNano,
	}[File.Section("time").Key("FORMAT").MustString("RFC1123")]

	sec = File.Section("picture")
	AvatarUploadPath = sec.Key("AVATAR_UPLOAD_PATH").MustString(filepath.Join(Server.AppDataPath, "avatars"))
	if !filepath.IsAbs(AvatarUploadPath) {
		AvatarUploadPath = path.Join(workDir, AvatarUploadPath)
	}
	RepositoryAvatarUploadPath = sec.Key("REPOSITORY_AVATAR_UPLOAD_PATH").MustString(filepath.Join(Server.AppDataPath, "repo-avatars"))
	if !filepath.IsAbs(RepositoryAvatarUploadPath) {
		RepositoryAvatarUploadPath = path.Join(workDir, RepositoryAvatarUploadPath)
	}
	switch source := sec.Key("GRAVATAR_SOURCE").MustString("gravatar"); source {
	case "duoshuo":
		GravatarSource = "http://gravatar.duoshuo.com/avatar/"
	case "gravatar":
		GravatarSource = "https://secure.gravatar.com/avatar/"
	case "libravatar":
		GravatarSource = "https://seccdn.libravatar.org/avatar/"
	default:
		GravatarSource = source
	}
	DisableGravatar = sec.Key("DISABLE_GRAVATAR").MustBool()
	EnableFederatedAvatar = sec.Key("ENABLE_FEDERATED_AVATAR").MustBool(true)
	if Server.OfflineMode {
		DisableGravatar = true
		EnableFederatedAvatar = false
	}
	if DisableGravatar {
		EnableFederatedAvatar = false
	}

	if EnableFederatedAvatar {
		LibravatarService = libravatar.New()
		parts := strings.Split(GravatarSource, "/")
		if len(parts) >= 3 {
			if parts[0] == "https:" {
				LibravatarService.SetUseHTTPS(true)
				LibravatarService.SetSecureFallbackHost(parts[2])
			} else {
				LibravatarService.SetUseHTTPS(false)
				LibravatarService.SetFallbackHost(parts[2])
			}
		}
	}

	if err = File.Section("http").MapTo(&HTTP); err != nil {
		log.Fatal("Failed to map HTTP settings: %v", err)
	} else if err = File.Section("webhook").MapTo(&Webhook); err != nil {
		log.Fatal("Failed to map Webhook settings: %v", err)
	} else if err = File.Section("release.attachment").MapTo(&Release.Attachment); err != nil {
		log.Fatal("Failed to map Release.Attachment settings: %v", err)
	} else if err = File.Section("markdown").MapTo(&Markdown); err != nil {
		log.Fatal("Failed to map Markdown settings: %v", err)
	} else if err = File.Section("smartypants").MapTo(&Smartypants); err != nil {
		log.Fatal("Failed to map Smartypants settings: %v", err)
	} else if err = File.Section("admin").MapTo(&Admin); err != nil {
		log.Fatal("Failed to map Admin settings: %v", err)
	} else if err = File.Section("cron").MapTo(&Cron); err != nil {
		log.Fatal("Failed to map Cron settings: %v", err)
	} else if err = File.Section("git").MapTo(&Git); err != nil {
		log.Fatal("Failed to map Git settings: %v", err)
	} else if err = File.Section("mirror").MapTo(&Mirror); err != nil {
		log.Fatal("Failed to map Mirror settings: %v", err)
	} else if err = File.Section("api").MapTo(&API); err != nil {
		log.Fatal("Failed to map API settings: %v", err)
	} else if err = File.Section("ui").MapTo(&UI); err != nil {
		log.Fatal("Failed to map UI settings: %v", err)
	} else if err = File.Section("prometheus").MapTo(&Prometheus); err != nil {
		log.Fatal("Failed to map Prometheus settings: %v", err)
	}

	if Mirror.DefaultInterval <= 0 {
		Mirror.DefaultInterval = 24
	}

	Langs = File.Section("i18n").Key("LANGS").Strings(",")
	Names = File.Section("i18n").Key("NAMES").Strings(",")
	dateLangs = File.Section("i18n.datelang").KeysHash()

	ShowFooterBranding = File.Section("other").Key("SHOW_FOOTER_BRANDING").MustBool()
	ShowFooterTemplateLoadTime = File.Section("other").Key("SHOW_FOOTER_TEMPLATE_LOAD_TIME").MustBool()

	HasRobotsTxt = osutil.IsFile(path.Join(CustomDir(), "robots.txt"))
	return nil
}

// MustInit panics if configuration initialization failed.
func MustInit(customConf string) {
	err := Init(customConf)
	if err != nil {
		panic(err)
	}
}

// TODO

var (
	HTTP struct {
		AccessControlAllowOrigin string
	}

	// Security settings
	InstallLock             bool
	SecretKey               string
	LoginRememberDays       int
	CookieUserName          string
	CookieRememberName      string
	CookieSecure            bool
	ReverseProxyAuthUser    string
	EnableLoginStatusCookie bool
	LoginStatusCookieName   string

	// Database settings
	UseSQLite3    bool
	UseMySQL      bool
	UsePostgreSQL bool
	UseMSSQL      bool

	// Webhook settings
	Webhook struct {
		Types          []string
		QueueLength    int
		DeliverTimeout int
		SkipTLSVerify  bool `ini:"SKIP_TLS_VERIFY"`
		PagingNum      int
	}

	// Release settigns
	Release struct {
		Attachment struct {
			Enabled      bool
			TempPath     string
			AllowedTypes []string `delim:"|"`
			MaxSize      int64
			MaxFiles     int
		} `ini:"-"`
	}

	// Markdown sttings
	Markdown struct {
		EnableHardLineBreak bool
		CustomURLSchemes    []string `ini:"CUSTOM_URL_SCHEMES"`
		FileExtensions      []string
	}

	// Smartypants settings
	Smartypants struct {
		Enabled      bool
		Fractions    bool
		Dashes       bool
		LatexDashes  bool
		AngledQuotes bool
	}

	// Admin settings
	Admin struct {
		DisableRegularOrgCreation bool
	}

	// Picture settings
	AvatarUploadPath           string
	RepositoryAvatarUploadPath string
	GravatarSource             string
	DisableGravatar            bool
	EnableFederatedAvatar      bool
	LibravatarService          *libravatar.Libravatar

	// Log settings
	LogRootPath string
	LogModes    []string
	LogConfigs  []interface{}

	// Attachment settings
	AttachmentPath         string
	AttachmentAllowedTypes string
	AttachmentMaxSize      int64
	AttachmentMaxFiles     int
	AttachmentEnabled      bool

	// Time settings
	TimeFormat string

	// Cache settings
	CacheAdapter  string
	CacheInterval int
	CacheConn     string

	// Session settings
	SessionConfig  session.Options
	CSRFCookieName string

	// Cron tasks
	Cron struct {
		UpdateMirror struct {
			Enabled    bool
			RunAtStart bool
			Schedule   string
		} `ini:"cron.update_mirrors"`
		RepoHealthCheck struct {
			Enabled    bool
			RunAtStart bool
			Schedule   string
			Timeout    time.Duration
			Args       []string `delim:" "`
		} `ini:"cron.repo_health_check"`
		CheckRepoStats struct {
			Enabled    bool
			RunAtStart bool
			Schedule   string
		} `ini:"cron.check_repo_stats"`
		RepoArchiveCleanup struct {
			Enabled    bool
			RunAtStart bool
			Schedule   string
			OlderThan  time.Duration
		} `ini:"cron.repo_archive_cleanup"`
	}

	// Git settings
	Git struct {
		Version                  string `ini:"-"`
		DisableDiffHighlight     bool
		MaxGitDiffLines          int
		MaxGitDiffLineCharacters int
		MaxGitDiffFiles          int
		GCArgs                   []string `ini:"GC_ARGS" delim:" "`
		Timeout                  struct {
			Migrate int
			Mirror  int
			Clone   int
			Pull    int
			GC      int `ini:"GC"`
		} `ini:"git.timeout"`
	}

	// Mirror settings
	Mirror struct {
		DefaultInterval int
	}

	// API settings
	API struct {
		MaxResponseItems int
	}

	// UI settings
	UI struct {
		ExplorePagingNum   int
		IssuePagingNum     int
		FeedMaxCommitNum   int
		ThemeColorMetaTag  string
		MaxDisplayFileSize int64

		Admin struct {
			UserPagingNum   int
			RepoPagingNum   int
			NoticePagingNum int
			OrgPagingNum    int
		} `ini:"ui.admin"`
		User struct {
			RepoPagingNum     int
			NewsFeedPagingNum int
			CommitsPagingNum  int
		} `ini:"ui.user"`
	}

	// Prometheus settings
	Prometheus struct {
		Enabled           bool
		EnableBasicAuth   bool
		BasicAuthUsername string
		BasicAuthPassword string
	}

	// I18n settings
	Langs     []string
	Names     []string
	dateLangs map[string]string

	// Highlight settings are loaded in modules/template/hightlight.go

	// Other settings
	ShowFooterBranding         bool
	ShowFooterTemplateLoadTime bool

	// Global setting objects
	HasRobotsTxt bool
)

// DateLang transforms standard language locale name to corresponding value in datetime plugin.
func DateLang(lang string) string {
	name, ok := dateLangs[lang]
	if ok {
		return name
	}
	return "en"
}

// IsRunUserMatchCurrentUser returns false if configured run user does not match
// actual user that runs the app. The first return value is the actual user name.
// This check is ignored under Windows since SSH remote login is not the main
// method to login on Windows.
func IsRunUserMatchCurrentUser(runUser string) (string, bool) {
	if IsWindowsRuntime() {
		return "", true
	}

	currentUser := user.CurrentUsername()
	return currentUser, runUser == currentUser
}

// InitLogging initializes the logging service of the application.
func InitLogging() {
	LogRootPath = File.Section("log").Key("ROOT_PATH").MustString(filepath.Join(WorkDir(), "log"))

	// Because we always create a console logger as the primary logger at init time,
	// we need to remove it in case the user doesn't configure to use it after the
	// logging service is initalized.
	hasConsole := false

	// Iterate over [log.*] sections to initialize individual logger.
	LogModes = strings.Split(File.Section("log").Key("MODE").MustString("console"), ",")
	LogConfigs = make([]interface{}, len(LogModes))
	levelMappings := map[string]log.Level{
		"trace": log.LevelTrace,
		"info":  log.LevelInfo,
		"warn":  log.LevelWarn,
		"error": log.LevelError,
		"fatal": log.LevelFatal,
	}

	type config struct {
		Buffer int64
		Config interface{}
	}
	for i, mode := range LogModes {
		mode = strings.ToLower(strings.TrimSpace(mode))
		secName := "log." + mode
		sec, err := File.GetSection(secName)
		if err != nil {
			log.Fatal("Missing configuration section [%s] for %q logger", secName, mode)
			return
		}

		level := levelMappings[sec.Key("LEVEL").MustString("trace")]
		buffer := sec.Key("BUFFER_LEN").MustInt64(100)
		c := new(config)
		switch mode {
		case log.DefaultConsoleName:
			hasConsole = true
			c = &config{
				Buffer: buffer,
				Config: log.ConsoleConfig{
					Level: level,
				},
			}
			err = log.NewConsole(c.Buffer, c.Config)

		case log.DefaultFileName:
			logPath := filepath.Join(LogRootPath, "gogs.log")
			logDir := filepath.Dir(logPath)
			err = os.MkdirAll(logDir, os.ModePerm)
			if err != nil {
				log.Fatal("Failed to create log directory %q: %v", logDir, err)
				return
			}

			c = &config{
				Buffer: buffer,
				Config: log.FileConfig{
					Level:    level,
					Filename: logPath,
					FileRotationConfig: log.FileRotationConfig{
						Rotate:   sec.Key("LOG_ROTATE").MustBool(true),
						Daily:    sec.Key("DAILY_ROTATE").MustBool(true),
						MaxSize:  1 << uint(sec.Key("MAX_SIZE_SHIFT").MustInt(28)),
						MaxLines: sec.Key("MAX_LINES").MustInt64(1000000),
						MaxDays:  sec.Key("MAX_DAYS").MustInt64(7),
					},
				},
			}
			err = log.NewFile(c.Buffer, c.Config)

		case log.DefaultSlackName:
			c = &config{
				Buffer: buffer,
				Config: log.SlackConfig{
					Level: level,
					URL:   sec.Key("URL").String(),
				},
			}
			err = log.NewSlack(c.Buffer, c.Config)

		case log.DefaultDiscordName:
			c = &config{
				Buffer: buffer,
				Config: log.DiscordConfig{
					Level:    level,
					URL:      sec.Key("URL").String(),
					Username: sec.Key("USERNAME").String(),
				},
			}

		default:
			continue
		}

		if err != nil {
			log.Fatal("Failed to init %s logger: %v", mode, err)
			return
		}
		LogConfigs[i] = c

		log.Trace("Log mode: %s (%s)", strings.Title(mode), strings.Title(strings.ToLower(level.String())))
	}

	if !hasConsole {
		log.Remove(log.DefaultConsoleName)
	}
}

var Service struct {
	ActiveCodeLives                int
	ResetPwdCodeLives              int
	RegisterEmailConfirm           bool
	DisableRegistration            bool
	ShowRegistrationButton         bool
	RequireSignInView              bool
	EnableNotifyMail               bool
	EnableReverseProxyAuth         bool
	EnableReverseProxyAutoRegister bool
	EnableCaptcha                  bool
}

func newService() {
	sec := File.Section("service")
	Service.ActiveCodeLives = sec.Key("ACTIVE_CODE_LIVE_MINUTES").MustInt(180)
	Service.ResetPwdCodeLives = sec.Key("RESET_PASSWD_CODE_LIVE_MINUTES").MustInt(180)
	Service.DisableRegistration = sec.Key("DISABLE_REGISTRATION").MustBool()
	Service.ShowRegistrationButton = sec.Key("SHOW_REGISTRATION_BUTTON").MustBool(!Service.DisableRegistration)
	Service.RequireSignInView = sec.Key("REQUIRE_SIGNIN_VIEW").MustBool()
	Service.EnableReverseProxyAuth = sec.Key("ENABLE_REVERSE_PROXY_AUTHENTICATION").MustBool()
	Service.EnableReverseProxyAutoRegister = sec.Key("ENABLE_REVERSE_PROXY_AUTO_REGISTRATION").MustBool()
	Service.EnableCaptcha = sec.Key("ENABLE_CAPTCHA").MustBool()
}

func newCacheService() {
	CacheAdapter = File.Section("cache").Key("ADAPTER").In("memory", []string{"memory", "redis", "memcache"})
	switch CacheAdapter {
	case "memory":
		CacheInterval = File.Section("cache").Key("INTERVAL").MustInt(60)
	case "redis", "memcache":
		CacheConn = strings.Trim(File.Section("cache").Key("HOST").String(), "\" ")
	default:
		log.Fatal("Unrecognized cache adapter %q", CacheAdapter)
		return
	}

	log.Trace("Cache service is enabled")
}

func newSessionService() {
	SessionConfig.Provider = File.Section("session").Key("PROVIDER").In("memory",
		[]string{"memory", "file", "redis", "mysql"})
	SessionConfig.ProviderConfig = strings.Trim(File.Section("session").Key("PROVIDER_CONFIG").String(), "\" ")
	SessionConfig.CookieName = File.Section("session").Key("COOKIE_NAME").MustString("i_like_gogs")
	SessionConfig.CookiePath = Server.Subpath
	SessionConfig.Secure = File.Section("session").Key("COOKIE_SECURE").MustBool()
	SessionConfig.Gclifetime = File.Section("session").Key("GC_INTERVAL_TIME").MustInt64(3600)
	SessionConfig.Maxlifetime = File.Section("session").Key("SESSION_LIFE_TIME").MustInt64(86400)
	CSRFCookieName = File.Section("session").Key("CSRF_COOKIE_NAME").MustString("_csrf")

	log.Trace("Session service is enabled")
}

// Mailer represents mail service.
type Mailer struct {
	QueueLength       int
	SubjectPrefix     string
	Host              string
	From              string
	FromEmail         string
	User, Passwd      string
	DisableHelo       bool
	HeloHostname      string
	SkipVerify        bool
	UseCertificate    bool
	CertFile, KeyFile string
	UsePlainText      bool
	AddPlainTextAlt   bool
}

var (
	MailService *Mailer
)

// newMailService initializes mail service options from configuration.
// No non-error log will be printed in hook mode.
func newMailService() {
	sec := File.Section("mailer")
	if !sec.Key("ENABLED").MustBool() {
		return
	}

	MailService = &Mailer{
		QueueLength:     sec.Key("SEND_BUFFER_LEN").MustInt(100),
		SubjectPrefix:   sec.Key("SUBJECT_PREFIX").MustString("[" + App.BrandName + "] "),
		Host:            sec.Key("HOST").String(),
		User:            sec.Key("USER").String(),
		Passwd:          sec.Key("PASSWD").String(),
		DisableHelo:     sec.Key("DISABLE_HELO").MustBool(),
		HeloHostname:    sec.Key("HELO_HOSTNAME").String(),
		SkipVerify:      sec.Key("SKIP_VERIFY").MustBool(),
		UseCertificate:  sec.Key("USE_CERTIFICATE").MustBool(),
		CertFile:        sec.Key("CERT_FILE").String(),
		KeyFile:         sec.Key("KEY_FILE").String(),
		UsePlainText:    sec.Key("USE_PLAIN_TEXT").MustBool(),
		AddPlainTextAlt: sec.Key("ADD_PLAIN_TEXT_ALT").MustBool(),
	}
	MailService.From = sec.Key("FROM").MustString(MailService.User)

	if len(MailService.From) > 0 {
		parsed, err := mail.ParseAddress(MailService.From)
		if err != nil {
			log.Fatal("Failed to parse value %q for '[mailer] FROM': %v", MailService.From, err)
			return
		}
		MailService.FromEmail = parsed.Address
	}

	if HookMode {
		return
	}
	log.Trace("Mail service is enabled")
}

func newRegisterMailService() {
	if !File.Section("service").Key("REGISTER_EMAIL_CONFIRM").MustBool() {
		return
	} else if MailService == nil {
		log.Warn("Email confirmation is not enabled due to the mail service is not available")
		return
	}
	Service.RegisterEmailConfirm = true
	log.Trace("Email confirmation is enabled")
}

// newNotifyMailService initializes notification email service options from configuration.
// No non-error log will be printed in hook mode.
func newNotifyMailService() {
	if !File.Section("service").Key("ENABLE_NOTIFY_MAIL").MustBool() {
		return
	} else if MailService == nil {
		log.Warn("Email notification is not enabled due to the mail service is not available")
		return
	}
	Service.EnableNotifyMail = true

	if HookMode {
		return
	}
	log.Trace("Email notification is enabled")
}

func NewService() {
	newService()
}

func NewServices() {
	newService()
	newCacheService()
	newSessionService()
	newMailService()
	newRegisterMailService()
	newNotifyMailService()
}

// HookMode indicates whether program starts as Git server-side hook callback.
var HookMode bool

// NewPostReceiveHookServices initializes all services that are needed by
// Git server-side post-receive hook callback.
func NewPostReceiveHookServices() {
	HookMode = true
	newService()
	newMailService()
	newNotifyMailService()
}
