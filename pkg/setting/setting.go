// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package setting

import (
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Unknwon/com"
	_ "github.com/go-macaron/cache/memcache"
	_ "github.com/go-macaron/cache/redis"
	"github.com/go-macaron/session"
	_ "github.com/go-macaron/session/redis"
	"github.com/mcuadros/go-version"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"

	"github.com/gogits/go-libravatar"

	"github.com/gogits/gogs/pkg/bindata"
	"github.com/gogits/gogs/pkg/process"
	"github.com/gogits/gogs/pkg/user"
)

type Scheme string

const (
	SCHEME_HTTP        Scheme = "http"
	SCHEME_HTTPS       Scheme = "https"
	SCHEME_FCGI        Scheme = "fcgi"
	SCHEME_UNIX_SOCKET Scheme = "unix"
)

type LandingPage string

const (
	LANDING_PAGE_HOME    LandingPage = "/"
	LANDING_PAGE_EXPLORE LandingPage = "/explore"
)

var (
	// Build information should only be set by -ldflags.
	BuildTime    string
	BuildGitHash string

	// App settings
	AppVer         string
	AppName        string
	AppURL         string
	AppSubURL      string
	AppSubURLDepth int // Number of slashes
	AppPath        string
	AppDataPath    string

	// Server settings
	Protocol             Scheme
	Domain               string
	HTTPAddr             string
	HTTPPort             string
	LocalURL             string
	OfflineMode          bool
	DisableRouterLog     bool
	CertFile, KeyFile    string
	TLSMinVersion        string
	StaticRootPath       string
	EnableGzip           bool
	LandingPageURL       LandingPage
	UnixSocketPermission uint32

	HTTP struct {
		AccessControlAllowOrigin string
	}

	SSH struct {
		Disabled            bool           `ini:"DISABLE_SSH"`
		StartBuiltinServer  bool           `ini:"START_SSH_SERVER"`
		Domain              string         `ini:"SSH_DOMAIN"`
		Port                int            `ini:"SSH_PORT"`
		ListenHost          string         `ini:"SSH_LISTEN_HOST"`
		ListenPort          int            `ini:"SSH_LISTEN_PORT"`
		RootPath            string         `ini:"SSH_ROOT_PATH"`
		ServerCiphers       []string       `ini:"SSH_SERVER_CIPHERS"`
		KeyTestPath         string         `ini:"SSH_KEY_TEST_PATH"`
		KeygenPath          string         `ini:"SSH_KEYGEN_PATH"`
		MinimumKeySizeCheck bool           `ini:"MINIMUM_KEY_SIZE_CHECK"`
		MinimumKeySizes     map[string]int `ini:"-"`
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

	// Repository settings
	Repository struct {
		AnsiCharset              string
		ForcePrivate             bool
		MaxCreationLimit         int
		MirrorQueueLength        int
		PullRequestQueueLength   int
		PreferredLicenses        []string
		DisableHTTPGit           bool `ini:"DISABLE_HTTP_GIT"`
		EnableLocalPathMigration bool
		CommitsFetchConcurrency  int
		EnableRawFileRenderMode  bool

		// Repository editor settings
		Editor struct {
			LineWrapExtensions   []string
			PreviewableFileModes []string
		} `ini:"-"`

		// Repository upload settings
		Upload struct {
			Enabled      bool
			TempPath     string
			AllowedTypes []string `delim:"|"`
			FileMaxSize  int64
			MaxFiles     int
		} `ini:"-"`
	}
	RepoRootPath string
	ScriptType   string

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
	AvatarUploadPath      string
	GravatarSource        string
	DisableGravatar       bool
	EnableFederatedAvatar bool
	LibravatarService     *libravatar.Libravatar

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
		GCArgs                   []string `delim:" "`
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

	// I18n settings
	Langs     []string
	Names     []string
	dateLangs map[string]string

	// Highlight settings are loaded in modules/template/hightlight.go

	// Other settings
	ShowFooterBranding         bool
	ShowFooterVersion          bool
	ShowFooterTemplateLoadTime bool
	SupportMiniWinService      bool

	// Global setting objects
	Cfg          *ini.File
	CustomPath   string // Custom directory path
	CustomConf   string
	ProdMode     bool
	RunUser      string
	IsWindows    bool
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

// execPath returns the executable path.
func execPath() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	return filepath.Abs(file)
}

func init() {
	IsWindows = runtime.GOOS == "windows"
	log.New(log.CONSOLE, log.ConsoleConfig{})

	var err error
	if AppPath, err = execPath(); err != nil {
		log.Fatal(2, "Fail to get app path: %v\n", err)
	}

	// Note: we don't use path.Dir here because it does not handle case
	//	which path starts with two "/" in Windows: "//psf/Home/..."
	AppPath = strings.Replace(AppPath, "\\", "/", -1)
}

// WorkDir returns absolute path of work directory.
func WorkDir() (string, error) {
	wd := os.Getenv("GOGS_WORK_DIR")
	if len(wd) > 0 {
		return wd, nil
	}

	i := strings.LastIndex(AppPath, "/")
	if i == -1 {
		return AppPath, nil
	}
	return AppPath[:i], nil
}

func forcePathSeparator(path string) {
	if strings.Contains(path, "\\") {
		log.Fatal(2, "Do not use '\\' or '\\\\' in paths, instead, please use '/' in all places")
	}
}

// IsRunUserMatchCurrentUser returns false if configured run user does not match
// actual user that runs the app. The first return value is the actual user name.
// This check is ignored under Windows since SSH remote login is not the main
// method to login on Windows.
func IsRunUserMatchCurrentUser(runUser string) (string, bool) {
	if IsWindows {
		return "", true
	}

	currentUser := user.CurrentUsername()
	return currentUser, runUser == currentUser
}

// getOpenSSHVersion parses and returns string representation of OpenSSH version
// returned by command "ssh -V".
func getOpenSSHVersion() string {
	// Note: somehow version is printed to stderr
	_, stderr, err := process.Exec("getOpenSSHVersion", "ssh", "-V")
	if err != nil {
		log.Fatal(2, "Fail to get OpenSSH version: %v - %s", err, stderr)
	}

	// Trim unused information: https://github.com/gogits/gogs/issues/4507#issuecomment-305150441
	version := strings.TrimRight(strings.Fields(stderr)[0], ",1234567890")
	version = strings.TrimSuffix(strings.TrimPrefix(version, "OpenSSH_"), "p")
	return version
}

// NewContext initializes configuration context.
// NOTE: do not print any log except error.
func NewContext() {
	workDir, err := WorkDir()
	if err != nil {
		log.Fatal(2, "Fail to get work directory: %v", err)
	}

	Cfg, err = ini.Load(bindata.MustAsset("conf/app.ini"))
	if err != nil {
		log.Fatal(2, "Fail to parse 'conf/app.ini': %v", err)
	}

	CustomPath = os.Getenv("GOGS_CUSTOM")
	if len(CustomPath) == 0 {
		CustomPath = workDir + "/custom"
	}

	if len(CustomConf) == 0 {
		CustomConf = CustomPath + "/conf/app.ini"
	}

	if com.IsFile(CustomConf) {
		if err = Cfg.Append(CustomConf); err != nil {
			log.Fatal(2, "Fail to load custom conf '%s': %v", CustomConf, err)
		}
	} else {
		log.Warn("Custom config '%s' not found, ignore this if you're running first time", CustomConf)
	}
	Cfg.NameMapper = ini.AllCapsUnderscore

	homeDir, err := com.HomeDir()
	if err != nil {
		log.Fatal(2, "Fail to get home directory: %v", err)
	}
	homeDir = strings.Replace(homeDir, "\\", "/", -1)

	LogRootPath = Cfg.Section("log").Key("ROOT_PATH").MustString(path.Join(workDir, "log"))
	forcePathSeparator(LogRootPath)

	sec := Cfg.Section("server")
	AppName = Cfg.Section("").Key("APP_NAME").MustString("Gogs")
	AppURL = sec.Key("ROOT_URL").MustString("http://localhost:3000/")
	if AppURL[len(AppURL)-1] != '/' {
		AppURL += "/"
	}

	// Check if has app suburl.
	url, err := url.Parse(AppURL)
	if err != nil {
		log.Fatal(2, "Invalid ROOT_URL '%s': %s", AppURL, err)
	}
	// Suburl should start with '/' and end without '/', such as '/{subpath}'.
	// This value is empty if site does not have sub-url.
	AppSubURL = strings.TrimSuffix(url.Path, "/")
	AppSubURLDepth = strings.Count(AppSubURL, "/")

	Protocol = SCHEME_HTTP
	if sec.Key("PROTOCOL").String() == "https" {
		Protocol = SCHEME_HTTPS
		CertFile = sec.Key("CERT_FILE").String()
		KeyFile = sec.Key("KEY_FILE").String()
		TLSMinVersion = sec.Key("TLS_MIN_VERSION").String()
	} else if sec.Key("PROTOCOL").String() == "fcgi" {
		Protocol = SCHEME_FCGI
	} else if sec.Key("PROTOCOL").String() == "unix" {
		Protocol = SCHEME_UNIX_SOCKET
		UnixSocketPermissionRaw := sec.Key("UNIX_SOCKET_PERMISSION").MustString("666")
		UnixSocketPermissionParsed, err := strconv.ParseUint(UnixSocketPermissionRaw, 8, 32)
		if err != nil || UnixSocketPermissionParsed > 0777 {
			log.Fatal(2, "Fail to parse unixSocketPermission: %s", UnixSocketPermissionRaw)
		}
		UnixSocketPermission = uint32(UnixSocketPermissionParsed)
	}
	Domain = sec.Key("DOMAIN").MustString("localhost")
	HTTPAddr = sec.Key("HTTP_ADDR").MustString("0.0.0.0")
	HTTPPort = sec.Key("HTTP_PORT").MustString("3000")
	LocalURL = sec.Key("LOCAL_ROOT_URL").MustString(string(Protocol) + "://localhost:" + HTTPPort + "/")
	OfflineMode = sec.Key("OFFLINE_MODE").MustBool()
	DisableRouterLog = sec.Key("DISABLE_ROUTER_LOG").MustBool()
	StaticRootPath = sec.Key("STATIC_ROOT_PATH").MustString(workDir)
	AppDataPath = sec.Key("APP_DATA_PATH").MustString("data")
	EnableGzip = sec.Key("ENABLE_GZIP").MustBool()

	switch sec.Key("LANDING_PAGE").MustString("home") {
	case "explore":
		LandingPageURL = LANDING_PAGE_EXPLORE
	default:
		LandingPageURL = LANDING_PAGE_HOME
	}

	SSH.RootPath = path.Join(homeDir, ".ssh")
	SSH.ServerCiphers = sec.Key("SSH_SERVER_CIPHERS").Strings(",")
	SSH.KeyTestPath = os.TempDir()
	if err = Cfg.Section("server").MapTo(&SSH); err != nil {
		log.Fatal(2, "Fail to map SSH settings: %v", err)
	}
	if SSH.Disabled {
		SSH.StartBuiltinServer = false
		SSH.MinimumKeySizeCheck = false
	}

	if !SSH.Disabled && !SSH.StartBuiltinServer {
		if err := os.MkdirAll(SSH.RootPath, 0700); err != nil {
			log.Fatal(2, "Fail to create '%s': %v", SSH.RootPath, err)
		} else if err = os.MkdirAll(SSH.KeyTestPath, 0644); err != nil {
			log.Fatal(2, "Fail to create '%s': %v", SSH.KeyTestPath, err)
		}
	}

	// Check if server is eligible for minimum key size check when user choose to enable.
	// Windows server and OpenSSH version lower than 5.1 (https://github.com/gogits/gogs/issues/4507)
	// are forced to be disabled because the "ssh-keygen" in Windows does not print key type.
	if SSH.MinimumKeySizeCheck &&
		(IsWindows || version.Compare(getOpenSSHVersion(), "5.1", "<")) {
		SSH.MinimumKeySizeCheck = false
		log.Warn(`SSH minimum key size check is forced to be disabled because server is not eligible:
1. Windows server
2. OpenSSH version is lower than 5.1`)
	}

	if SSH.MinimumKeySizeCheck {
		SSH.MinimumKeySizes = map[string]int{}
		for _, key := range Cfg.Section("ssh.minimum_key_sizes").Keys() {
			if key.MustInt() != -1 {
				SSH.MinimumKeySizes[strings.ToLower(key.Name())] = key.MustInt()
			}
		}
	}

	sec = Cfg.Section("security")
	InstallLock = sec.Key("INSTALL_LOCK").MustBool()
	SecretKey = sec.Key("SECRET_KEY").String()
	LoginRememberDays = sec.Key("LOGIN_REMEMBER_DAYS").MustInt()
	CookieUserName = sec.Key("COOKIE_USERNAME").String()
	CookieRememberName = sec.Key("COOKIE_REMEMBER_NAME").String()
	CookieSecure = sec.Key("COOKIE_SECURE").MustBool(false)
	ReverseProxyAuthUser = sec.Key("REVERSE_PROXY_AUTHENTICATION_USER").MustString("X-WEBAUTH-USER")
	EnableLoginStatusCookie = sec.Key("ENABLE_LOGIN_STATUS_COOKIE").MustBool(false)
	LoginStatusCookieName = sec.Key("LOGIN_STATUS_COOKIE_NAME").MustString("login_status")

	sec = Cfg.Section("attachment")
	AttachmentPath = sec.Key("PATH").MustString(path.Join(AppDataPath, "attachments"))
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
	}[Cfg.Section("time").Key("FORMAT").MustString("RFC1123")]

	RunUser = Cfg.Section("").Key("RUN_USER").String()
	// Does not check run user when the install lock is off.
	if InstallLock {
		currentUser, match := IsRunUserMatchCurrentUser(RunUser)
		if !match {
			log.Fatal(2, "Expect user '%s' but current user is: %s", RunUser, currentUser)
		}
	}

	ProdMode = Cfg.Section("").Key("RUN_MODE").String() == "prod"

	// Determine and create root git repository path.
	sec = Cfg.Section("repository")
	RepoRootPath = sec.Key("ROOT").MustString(path.Join(homeDir, "gogs-repositories"))
	forcePathSeparator(RepoRootPath)
	if !filepath.IsAbs(RepoRootPath) {
		RepoRootPath = path.Join(workDir, RepoRootPath)
	} else {
		RepoRootPath = path.Clean(RepoRootPath)
	}
	ScriptType = sec.Key("SCRIPT_TYPE").MustString("bash")
	if err = Cfg.Section("repository").MapTo(&Repository); err != nil {
		log.Fatal(2, "Fail to map Repository settings: %v", err)
	} else if err = Cfg.Section("repository.editor").MapTo(&Repository.Editor); err != nil {
		log.Fatal(2, "Fail to map Repository.Editor settings: %v", err)
	} else if err = Cfg.Section("repository.upload").MapTo(&Repository.Upload); err != nil {
		log.Fatal(2, "Fail to map Repository.Upload settings: %v", err)
	}

	if !filepath.IsAbs(Repository.Upload.TempPath) {
		Repository.Upload.TempPath = path.Join(workDir, Repository.Upload.TempPath)
	}

	sec = Cfg.Section("picture")
	AvatarUploadPath = sec.Key("AVATAR_UPLOAD_PATH").MustString(path.Join(AppDataPath, "avatars"))
	forcePathSeparator(AvatarUploadPath)
	if !filepath.IsAbs(AvatarUploadPath) {
		AvatarUploadPath = path.Join(workDir, AvatarUploadPath)
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
	if OfflineMode {
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

	if err = Cfg.Section("http").MapTo(&HTTP); err != nil {
		log.Fatal(2, "Fail to map HTTP settings: %v", err)
	} else if err = Cfg.Section("webhook").MapTo(&Webhook); err != nil {
		log.Fatal(2, "Fail to map Webhook settings: %v", err)
	} else if err = Cfg.Section("release.attachment").MapTo(&Release.Attachment); err != nil {
		log.Fatal(2, "Fail to map Release.Attachment settings: %v", err)
	} else if err = Cfg.Section("markdown").MapTo(&Markdown); err != nil {
		log.Fatal(2, "Fail to map Markdown settings: %v", err)
	} else if err = Cfg.Section("smartypants").MapTo(&Smartypants); err != nil {
		log.Fatal(2, "Fail to map Smartypants settings: %v", err)
	} else if err = Cfg.Section("admin").MapTo(&Admin); err != nil {
		log.Fatal(2, "Fail to map Admin settings: %v", err)
	} else if err = Cfg.Section("cron").MapTo(&Cron); err != nil {
		log.Fatal(2, "Fail to map Cron settings: %v", err)
	} else if err = Cfg.Section("git").MapTo(&Git); err != nil {
		log.Fatal(2, "Fail to map Git settings: %v", err)
	} else if err = Cfg.Section("mirror").MapTo(&Mirror); err != nil {
		log.Fatal(2, "Fail to map Mirror settings: %v", err)
	} else if err = Cfg.Section("api").MapTo(&API); err != nil {
		log.Fatal(2, "Fail to map API settings: %v", err)
	} else if err = Cfg.Section("ui").MapTo(&UI); err != nil {
		log.Fatal(2, "Fail to map UI settings: %v", err)
	}

	if Mirror.DefaultInterval <= 0 {
		Mirror.DefaultInterval = 24
	}

	Langs = Cfg.Section("i18n").Key("LANGS").Strings(",")
	Names = Cfg.Section("i18n").Key("NAMES").Strings(",")
	dateLangs = Cfg.Section("i18n.datelang").KeysHash()

	ShowFooterBranding = Cfg.Section("other").Key("SHOW_FOOTER_BRANDING").MustBool()
	ShowFooterVersion = Cfg.Section("other").Key("SHOW_FOOTER_VERSION").MustBool()
	ShowFooterTemplateLoadTime = Cfg.Section("other").Key("SHOW_FOOTER_TEMPLATE_LOAD_TIME").MustBool()

	HasRobotsTxt = com.IsFile(path.Join(CustomPath, "robots.txt"))
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
	sec := Cfg.Section("service")
	Service.ActiveCodeLives = sec.Key("ACTIVE_CODE_LIVE_MINUTES").MustInt(180)
	Service.ResetPwdCodeLives = sec.Key("RESET_PASSWD_CODE_LIVE_MINUTES").MustInt(180)
	Service.DisableRegistration = sec.Key("DISABLE_REGISTRATION").MustBool()
	Service.ShowRegistrationButton = sec.Key("SHOW_REGISTRATION_BUTTON").MustBool(!Service.DisableRegistration)
	Service.RequireSignInView = sec.Key("REQUIRE_SIGNIN_VIEW").MustBool()
	Service.EnableReverseProxyAuth = sec.Key("ENABLE_REVERSE_PROXY_AUTHENTICATION").MustBool()
	Service.EnableReverseProxyAutoRegister = sec.Key("ENABLE_REVERSE_PROXY_AUTO_REGISTRATION").MustBool()
	Service.EnableCaptcha = sec.Key("ENABLE_CAPTCHA").MustBool()
}

func newLogService() {
	if len(BuildTime) > 0 {
		log.Trace("Build Time: %s", BuildTime)
		log.Trace("Build Git Hash: %s", BuildGitHash)
	}

	// Because we always create a console logger as primary logger before all settings are loaded,
	// thus if user doesn't set console logger, we should remove it after other loggers are created.
	hasConsole := false

	// Get and check log modes.
	LogModes = strings.Split(Cfg.Section("log").Key("MODE").MustString("console"), ",")
	LogConfigs = make([]interface{}, len(LogModes))
	levelNames := map[string]log.LEVEL{
		"trace": log.TRACE,
		"info":  log.INFO,
		"warn":  log.WARN,
		"error": log.ERROR,
		"fatal": log.FATAL,
	}
	for i, mode := range LogModes {
		mode = strings.ToLower(strings.TrimSpace(mode))
		sec, err := Cfg.GetSection("log." + mode)
		if err != nil {
			log.Fatal(2, "Unknown logger mode: %s", mode)
		}

		validLevels := []string{"trace", "info", "warn", "error", "fatal"}
		name := Cfg.Section("log." + mode).Key("LEVEL").Validate(func(v string) string {
			v = strings.ToLower(v)
			if com.IsSliceContainsStr(validLevels, v) {
				return v
			}
			return "trace"
		})
		level := levelNames[name]

		// Generate log configuration.
		switch log.MODE(mode) {
		case log.CONSOLE:
			hasConsole = true
			LogConfigs[i] = log.ConsoleConfig{
				Level:      level,
				BufferSize: Cfg.Section("log").Key("BUFFER_LEN").MustInt64(100),
			}

		case log.FILE:
			logPath := path.Join(LogRootPath, "gogs.log")
			if err = os.MkdirAll(path.Dir(logPath), os.ModePerm); err != nil {
				log.Fatal(2, "Fail to create log directory '%s': %v", path.Dir(logPath), err)
			}

			LogConfigs[i] = log.FileConfig{
				Level:      level,
				BufferSize: Cfg.Section("log").Key("BUFFER_LEN").MustInt64(100),
				Filename:   logPath,
				FileRotationConfig: log.FileRotationConfig{
					Rotate:   sec.Key("LOG_ROTATE").MustBool(true),
					Daily:    sec.Key("DAILY_ROTATE").MustBool(true),
					MaxSize:  1 << uint(sec.Key("MAX_SIZE_SHIFT").MustInt(28)),
					MaxLines: sec.Key("MAX_LINES").MustInt64(1000000),
					MaxDays:  sec.Key("MAX_DAYS").MustInt64(7),
				},
			}

		case log.SLACK:
			LogConfigs[i] = log.SlackConfig{
				Level:      level,
				BufferSize: Cfg.Section("log").Key("BUFFER_LEN").MustInt64(100),
				URL:        sec.Key("URL").String(),
			}
		}

		log.New(log.MODE(mode), LogConfigs[i])
		log.Trace("Log Mode: %s (%s)", strings.Title(mode), strings.Title(name))
	}

	// Make sure everyone gets version info printed.
	log.Info("%s %s", AppName, AppVer)
	if !hasConsole {
		log.Delete(log.CONSOLE)
	}
}

func newCacheService() {
	CacheAdapter = Cfg.Section("cache").Key("ADAPTER").In("memory", []string{"memory", "redis", "memcache"})
	switch CacheAdapter {
	case "memory":
		CacheInterval = Cfg.Section("cache").Key("INTERVAL").MustInt(60)
	case "redis", "memcache":
		CacheConn = strings.Trim(Cfg.Section("cache").Key("HOST").String(), "\" ")
	default:
		log.Fatal(2, "Unknown cache adapter: %s", CacheAdapter)
	}

	log.Info("Cache Service Enabled")
}

func newSessionService() {
	SessionConfig.Provider = Cfg.Section("session").Key("PROVIDER").In("memory",
		[]string{"memory", "file", "redis", "mysql"})
	SessionConfig.ProviderConfig = strings.Trim(Cfg.Section("session").Key("PROVIDER_CONFIG").String(), "\" ")
	SessionConfig.CookieName = Cfg.Section("session").Key("COOKIE_NAME").MustString("i_like_gogits")
	SessionConfig.CookiePath = AppSubURL
	SessionConfig.Secure = Cfg.Section("session").Key("COOKIE_SECURE").MustBool()
	SessionConfig.Gclifetime = Cfg.Section("session").Key("GC_INTERVAL_TIME").MustInt64(3600)
	SessionConfig.Maxlifetime = Cfg.Section("session").Key("SESSION_LIFE_TIME").MustInt64(86400)
	CSRFCookieName = Cfg.Section("session").Key("CSRF_COOKIE_NAME").MustString("_csrf")

	log.Info("Session Service Enabled")
}

// Mailer represents mail service.
type Mailer struct {
	QueueLength       int
	Subject           string
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
}

var (
	MailService *Mailer
)

// newMailService initializes mail service options from configuration.
// No non-error log will be printed in hook mode.
func newMailService() {
	sec := Cfg.Section("mailer")
	if !sec.Key("ENABLED").MustBool() {
		return
	}

	MailService = &Mailer{
		QueueLength:    sec.Key("SEND_BUFFER_LEN").MustInt(100),
		Subject:        sec.Key("SUBJECT").MustString(AppName),
		Host:           sec.Key("HOST").String(),
		User:           sec.Key("USER").String(),
		Passwd:         sec.Key("PASSWD").String(),
		DisableHelo:    sec.Key("DISABLE_HELO").MustBool(),
		HeloHostname:   sec.Key("HELO_HOSTNAME").String(),
		SkipVerify:     sec.Key("SKIP_VERIFY").MustBool(),
		UseCertificate: sec.Key("USE_CERTIFICATE").MustBool(),
		CertFile:       sec.Key("CERT_FILE").String(),
		KeyFile:        sec.Key("KEY_FILE").String(),
		UsePlainText:   sec.Key("USE_PLAIN_TEXT").MustBool(),
	}
	MailService.From = sec.Key("FROM").MustString(MailService.User)

	if len(MailService.From) > 0 {
		parsed, err := mail.ParseAddress(MailService.From)
		if err != nil {
			log.Fatal(2, "Invalid mailer.FROM (%s): %v", MailService.From, err)
		}
		MailService.FromEmail = parsed.Address
	}

	if HookMode {
		return
	}
	log.Info("Mail Service Enabled")
}

func newRegisterMailService() {
	if !Cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").MustBool() {
		return
	} else if MailService == nil {
		log.Warn("Register Mail Service: Mail Service is not enabled")
		return
	}
	Service.RegisterEmailConfirm = true
	log.Info("Register Mail Service Enabled")
}

// newNotifyMailService initializes notification email service options from configuration.
// No non-error log will be printed in hook mode.
func newNotifyMailService() {
	if !Cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").MustBool() {
		return
	} else if MailService == nil {
		log.Warn("Notify Mail Service: Mail Service is not enabled")
		return
	}
	Service.EnableNotifyMail = true

	if HookMode {
		return
	}
	log.Info("Notify Mail Service Enabled")
}

func NewService() {
	newService()
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
