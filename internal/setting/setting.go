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

	_ "github.com/go-macaron/cache/memcache"
	_ "github.com/go-macaron/cache/redis"
	"github.com/go-macaron/session"
	_ "github.com/go-macaron/session/redis"
	"github.com/mcuadros/go-version"
	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"

	"github.com/gogs/go-libravatar"

	"gogs.io/gogs/internal/assets/conf"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/user"
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
	BuildTime   string
	BuildCommit string

	// App settings
	AppVersion     string
	AppName        string
	AppURL         string
	AppSubURL      string
	AppSubURLDepth int // Number of slashes
	AppPath        string
	AppDataPath    string
	HostAddress    string // AppURL without protocol and slashes

	// Server settings
	Protocol             Scheme
	Domain               string
	HTTPAddr             string
	HTTPPort             string
	LocalURL             string
	OfflineMode          bool
	DisableRouterLog     bool
	CertFile             string
	KeyFile              string
	TLSMinVersion        string
	LoadAssetsFromDisk   bool
	StaticRootPath       string
	EnableGzip           bool
	LandingPageURL       LandingPage
	UnixSocketPermission uint32

	HTTP struct {
		AccessControlAllowOrigin string
	}

	SSH struct {
		Disabled                     bool           `ini:"DISABLE_SSH"`
		StartBuiltinServer           bool           `ini:"START_SSH_SERVER"`
		Domain                       string         `ini:"SSH_DOMAIN"`
		Port                         int            `ini:"SSH_PORT"`
		ListenHost                   string         `ini:"SSH_LISTEN_HOST"`
		ListenPort                   int            `ini:"SSH_LISTEN_PORT"`
		RootPath                     string         `ini:"SSH_ROOT_PATH"`
		RewriteAuthorizedKeysAtStart bool           `ini:"REWRITE_AUTHORIZED_KEYS_AT_START"`
		ServerCiphers                []string       `ini:"SSH_SERVER_CIPHERS"`
		KeyTestPath                  string         `ini:"SSH_KEY_TEST_PATH"`
		KeygenPath                   string         `ini:"SSH_KEYGEN_PATH"`
		MinimumKeySizeCheck          bool           `ini:"MINIMUM_KEY_SIZE_CHECK"`
		MinimumKeySizes              map[string]int `ini:"-"`
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

	err := log.NewConsole()
	if err != nil {
		panic("init console logger: " + err.Error())
	}

	AppPath, err = execPath()
	if err != nil {
		log.Fatal("Failed to get executable path: %v", err)
	}

	// NOTE: we don't use path.Dir here because it does not handle case
	// which path starts with two "/" in Windows: "//psf/Home/..."
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
		log.Fatal("Do not use '\\' or '\\\\' in paths, please use '/' in all places")
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
	// NOTE: Somehow the version is printed to stderr.
	_, stderr, err := process.Exec("setting.getOpenSSHVersion", "ssh", "-V")
	if err != nil {
		log.Fatal("Failed to get OpenSSH version: %v - %s", err, stderr)
	}

	// Trim unused information: https://github.com/gogs/gogs/issues/4507#issuecomment-305150441
	version := strings.TrimRight(strings.Fields(stderr)[0], ",1234567890")
	version = strings.TrimSuffix(strings.TrimPrefix(version, "OpenSSH_"), "p")
	return version
}

// Init initializes configuration by loading from sources.
// ⚠️ WARNING: Do not print anything in this function other than wanrings or errors.
func Init() {
	workDir, err := WorkDir()
	if err != nil {
		log.Fatal("Failed to get work directory: %v", err)
		return
	}

	Cfg, err = ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, conf.MustAsset("conf/app.ini"))
	if err != nil {
		log.Fatal("Failed to parse 'conf/app.ini': %v", err)
		return
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
			log.Fatal("Failed to load custom conf %q: %v", CustomConf, err)
			return
		}
	} else {
		log.Warn("Custom config '%s' not found, ignore this warning if you're running the first time", CustomConf)
	}
	Cfg.NameMapper = ini.SnackCase

	homeDir, err := com.HomeDir()
	if err != nil {
		log.Fatal("Failed to get home directory: %v", err)
		return
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
		log.Fatal("Failed to parse ROOT_URL %q: %s", AppURL, err)
		return
	}
	// Suburl should start with '/' and end without '/', such as '/{subpath}'.
	// This value is empty if site does not have sub-url.
	AppSubURL = strings.TrimSuffix(url.Path, "/")
	AppSubURLDepth = strings.Count(AppSubURL, "/")
	HostAddress = url.Host

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
			log.Fatal("Failed to parse unixSocketPermission %q: %v", UnixSocketPermissionRaw, err)
			return
		}
		UnixSocketPermission = uint32(UnixSocketPermissionParsed)
	}
	Domain = sec.Key("DOMAIN").MustString("localhost")
	HTTPAddr = sec.Key("HTTP_ADDR").MustString("0.0.0.0")
	HTTPPort = sec.Key("HTTP_PORT").MustString("3000")
	LocalURL = sec.Key("LOCAL_ROOT_URL").MustString(string(Protocol) + "://localhost:" + HTTPPort + "/")
	OfflineMode = sec.Key("OFFLINE_MODE").MustBool()
	DisableRouterLog = sec.Key("DISABLE_ROUTER_LOG").MustBool()
	LoadAssetsFromDisk = sec.Key("LOAD_ASSETS_FROM_DISK").MustBool()
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
	SSH.RewriteAuthorizedKeysAtStart = sec.Key("REWRITE_AUTHORIZED_KEYS_AT_START").MustBool()
	SSH.ServerCiphers = sec.Key("SSH_SERVER_CIPHERS").Strings(",")
	SSH.KeyTestPath = os.TempDir()
	if err = Cfg.Section("server").MapTo(&SSH); err != nil {
		log.Fatal("Failed to map SSH settings: %v", err)
		return
	}
	if SSH.Disabled {
		SSH.StartBuiltinServer = false
		SSH.MinimumKeySizeCheck = false
	}

	if !SSH.Disabled && !SSH.StartBuiltinServer {
		if err := os.MkdirAll(SSH.RootPath, 0700); err != nil {
			log.Fatal("Failed to create '%s': %v", SSH.RootPath, err)
			return
		} else if err = os.MkdirAll(SSH.KeyTestPath, 0644); err != nil {
			log.Fatal("Failed to create '%s': %v", SSH.KeyTestPath, err)
			return
		}
	}

	if SSH.StartBuiltinServer {
		SSH.RewriteAuthorizedKeysAtStart = false
	}

	// Check if server is eligible for minimum key size check when user choose to enable.
	// Windows server and OpenSSH version lower than 5.1 (https://gogs.io/gogs/issues/4507)
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
			log.Fatal("The user configured to run Gogs is %q, but the current user is %q", RunUser, currentUser)
			return
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
		log.Fatal("Failed to map Repository settings: %v", err)
		return
	} else if err = Cfg.Section("repository.editor").MapTo(&Repository.Editor); err != nil {
		log.Fatal("Failed to map Repository.Editor settings: %v", err)
		return
	} else if err = Cfg.Section("repository.upload").MapTo(&Repository.Upload); err != nil {
		log.Fatal("Failed to map Repository.Upload settings: %v", err)
		return
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
	RepositoryAvatarUploadPath = sec.Key("REPOSITORY_AVATAR_UPLOAD_PATH").MustString(path.Join(AppDataPath, "repo-avatars"))
	forcePathSeparator(RepositoryAvatarUploadPath)
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
		log.Fatal("Failed to map HTTP settings: %v", err)
		return
	} else if err = Cfg.Section("webhook").MapTo(&Webhook); err != nil {
		log.Fatal("Failed to map Webhook settings: %v", err)
		return
	} else if err = Cfg.Section("release.attachment").MapTo(&Release.Attachment); err != nil {
		log.Fatal("Failed to map Release.Attachment settings: %v", err)
		return
	} else if err = Cfg.Section("markdown").MapTo(&Markdown); err != nil {
		log.Fatal("Failed to map Markdown settings: %v", err)
		return
	} else if err = Cfg.Section("smartypants").MapTo(&Smartypants); err != nil {
		log.Fatal("Failed to map Smartypants settings: %v", err)
		return
	} else if err = Cfg.Section("admin").MapTo(&Admin); err != nil {
		log.Fatal("Failed to map Admin settings: %v", err)
		return
	} else if err = Cfg.Section("cron").MapTo(&Cron); err != nil {
		log.Fatal("Failed to map Cron settings: %v", err)
		return
	} else if err = Cfg.Section("git").MapTo(&Git); err != nil {
		log.Fatal("Failed to map Git settings: %v", err)
		return
	} else if err = Cfg.Section("mirror").MapTo(&Mirror); err != nil {
		log.Fatal("Failed to map Mirror settings: %v", err)
		return
	} else if err = Cfg.Section("api").MapTo(&API); err != nil {
		log.Fatal("Failed to map API settings: %v", err)
		return
	} else if err = Cfg.Section("ui").MapTo(&UI); err != nil {
		log.Fatal("Failed to map UI settings: %v", err)
		return
	} else if err = Cfg.Section("prometheus").MapTo(&Prometheus); err != nil {
		log.Fatal("Failed to map Prometheus settings: %v", err)
		return
	}

	if Mirror.DefaultInterval <= 0 {
		Mirror.DefaultInterval = 24
	}

	Langs = Cfg.Section("i18n").Key("LANGS").Strings(",")
	Names = Cfg.Section("i18n").Key("NAMES").Strings(",")
	dateLangs = Cfg.Section("i18n.datelang").KeysHash()

	ShowFooterBranding = Cfg.Section("other").Key("SHOW_FOOTER_BRANDING").MustBool()
	ShowFooterTemplateLoadTime = Cfg.Section("other").Key("SHOW_FOOTER_TEMPLATE_LOAD_TIME").MustBool()

	HasRobotsTxt = com.IsFile(path.Join(CustomPath, "robots.txt"))
}

// InitLogging initializes the logging infrastructure of the application.
func InitLogging() {
	// Because we always create a console logger as the primary logger at init time,
	// we need to remove it in case the user doesn't configure to use it after the
	// logging infrastructure is initalized.
	hasConsole := false

	// Iterate over [log.*] sections to initialize individual logger.
	LogModes = strings.Split(Cfg.Section("log").Key("MODE").MustString("console"), ",")
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
		sec, err := Cfg.GetSection(secName)
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

func newCacheService() {
	CacheAdapter = Cfg.Section("cache").Key("ADAPTER").In("memory", []string{"memory", "redis", "memcache"})
	switch CacheAdapter {
	case "memory":
		CacheInterval = Cfg.Section("cache").Key("INTERVAL").MustInt(60)
	case "redis", "memcache":
		CacheConn = strings.Trim(Cfg.Section("cache").Key("HOST").String(), "\" ")
	default:
		log.Fatal("Unrecognized cache adapter %q", CacheAdapter)
		return
	}

	log.Trace("Cache service is enabled")
}

func newSessionService() {
	SessionConfig.Provider = Cfg.Section("session").Key("PROVIDER").In("memory",
		[]string{"memory", "file", "redis", "mysql"})
	SessionConfig.ProviderConfig = strings.Trim(Cfg.Section("session").Key("PROVIDER_CONFIG").String(), "\" ")
	SessionConfig.CookieName = Cfg.Section("session").Key("COOKIE_NAME").MustString("i_like_gogs")
	SessionConfig.CookiePath = AppSubURL
	SessionConfig.Secure = Cfg.Section("session").Key("COOKIE_SECURE").MustBool()
	SessionConfig.Gclifetime = Cfg.Section("session").Key("GC_INTERVAL_TIME").MustInt64(3600)
	SessionConfig.Maxlifetime = Cfg.Section("session").Key("SESSION_LIFE_TIME").MustInt64(86400)
	CSRFCookieName = Cfg.Section("session").Key("CSRF_COOKIE_NAME").MustString("_csrf")

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
	sec := Cfg.Section("mailer")
	if !sec.Key("ENABLED").MustBool() {
		return
	}

	MailService = &Mailer{
		QueueLength:     sec.Key("SEND_BUFFER_LEN").MustInt(100),
		SubjectPrefix:   sec.Key("SUBJECT_PREFIX").MustString("[" + AppName + "] "),
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
	if !Cfg.Section("service").Key("REGISTER_EMAIL_CONFIRM").MustBool() {
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
	if !Cfg.Section("service").Key("ENABLE_NOTIFY_MAIL").MustBool() {
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
