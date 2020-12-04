// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"net/url"
	"os"
	"time"

	"github.com/gogs/go-libravatar"
)

// ℹ️ README: This file contains static values that should only be set at initialization time.

// HasMinWinSvc is whether the application is built with Windows Service support.
//
// ⚠️ WARNING: should only be set by "internal/conf/static_minwinsvc.go".
var HasMinWinSvc bool

// Build time and commit information.
//
// ⚠️ WARNING: should only be set by "-ldflags".
var (
	BuildTime   string
	BuildCommit string
)

// CustomConf returns the absolute path of custom configuration file that is used.
var CustomConf string

// ⚠️ WARNING: After changing the following section, do not forget to update template of
// "/admin/config" page as well.
var (
	// Application settings
	App struct {
		// ⚠️ WARNING: Should only be set by the main package (i.e. "gogs.go").
		Version string `ini:"-"`

		BrandName string
		RunUser   string
		RunMode   string
	}

	// SSH settings
	SSH struct {
		Disabled                     bool   `ini:"DISABLE_SSH"`
		Domain                       string `ini:"SSH_DOMAIN"`
		Port                         int    `ini:"SSH_PORT"`
		RootPath                     string `ini:"SSH_ROOT_PATH"`
		KeygenPath                   string `ini:"SSH_KEYGEN_PATH"`
		KeyTestPath                  string `ini:"SSH_KEY_TEST_PATH"`
		MinimumKeySizeCheck          bool
		MinimumKeySizes              map[string]int `ini:"-"` // Load from [ssh.minimum_key_sizes]
		RewriteAuthorizedKeysAtStart bool

		StartBuiltinServer bool     `ini:"START_SSH_SERVER"`
		ListenHost         string   `ini:"SSH_LISTEN_HOST"`
		ListenPort         int      `ini:"SSH_LISTEN_PORT"`
		ServerCiphers      []string `ini:"SSH_SERVER_CIPHERS"`
		ServerMACs         []string `ini:"SSH_SERVER_MACS"`
	}

	// Repository settings
	Repository struct {
		Root                     string
		ScriptType               string
		ANSICharset              string `ini:"ANSI_CHARSET"`
		ForcePrivate             bool
		MaxCreationLimit         int
		PreferredLicenses        []string
		DisableHTTPGit           bool `ini:"DISABLE_HTTP_GIT"`
		EnableLocalPathMigration bool
		EnableRawFileRenderMode  bool
		CommitsFetchConcurrency  int

		// Repository editor settings
		Editor struct {
			LineWrapExtensions   []string
			PreviewableFileModes []string
		} `ini:"repository.editor"`

		// Repository upload settings
		Upload struct {
			Enabled      bool
			TempPath     string
			AllowedTypes []string `delim:"|"`
			FileMaxSize  int64
			MaxFiles     int
		} `ini:"repository.upload"`
	}

	// Security settings
	Security struct {
		InstallLock             bool
		SecretKey               string
		LoginRememberDays       int
		CookieRememberName      string
		CookieUsername          string
		CookieSecure            bool
		EnableLoginStatusCookie bool
		LoginStatusCookieName   string
	}

	// Email settings
	Email struct {
		Enabled       bool
		SubjectPrefix string
		Host          string
		From          string
		User          string
		Password      string

		DisableHELO  bool   `ini:"DISABLE_HELO"`
		HELOHostname string `ini:"HELO_HOSTNAME"`

		SkipVerify     bool
		UseCertificate bool
		CertFile       string
		KeyFile        string

		UsePlainText    bool
		AddPlainTextAlt bool

		// Derived from other static values
		FromEmail string `ini:"-"` // Parsed email address of From without person's name.
	}

	// Authentication settings
	Auth struct {
		ActivateCodeLives         int
		ResetPasswordCodeLives    int
		RequireEmailConfirmation  bool
		RequireSigninView         bool
		DisableRegistration       bool
		EnableRegistrationCaptcha bool

		EnableReverseProxyAuthentication   bool
		EnableReverseProxyAutoRegistration bool
		ReverseProxyAuthenticationHeader   string
	}

	// User settings
	User struct {
		EnableEmailNotification bool
	}

	// Session settings
	Session struct {
		Provider       string
		ProviderConfig string
		CookieName     string
		CookieSecure   bool
		GCInterval     int64 `ini:"GC_INTERVAL"`
		MaxLifeTime    int64
		CSRFCookieName string `ini:"CSRF_COOKIE_NAME"`
	}

	// Cache settings
	Cache struct {
		Adapter  string
		Interval int
		Host     string
	}

	// HTTP settings
	HTTP struct {
		AccessControlAllowOrigin string
	}

	// Attachment settings
	Attachment struct {
		Enabled      bool
		Path         string
		AllowedTypes []string `delim:"|"`
		MaxSize      int64
		MaxFiles     int
	}

	// Release settings
	Release struct {
		Attachment struct {
			Enabled      bool
			AllowedTypes []string `delim:"|"`
			MaxSize      int64
			MaxFiles     int
		} `ini:"release.attachment"`
	}

	// Time settings
	Time struct {
		Format string

		// Derived from other static values
		FormatLayout string `ini:"-"` // Actual layout of the Format.
	}

	// Picture settings
	Picture struct {
		AvatarUploadPath           string
		RepositoryAvatarUploadPath string
		GravatarSource             string
		DisableGravatar            bool
		EnableFederatedAvatar      bool

		// Derived from other static values
		LibravatarService *libravatar.Libravatar `ini:"-"` // Initialized client for federated avatar.
	}

	// Mirror settings
	Mirror struct {
		DefaultInterval int
	}

	// Webhook settings
	Webhook struct {
		Types          []string
		DeliverTimeout int
		SkipTLSVerify  bool `ini:"SKIP_TLS_VERIFY"`
		PagingNum      int
	}

	// Markdown settings
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
		// ⚠️ WARNING: Should only be set by "internal/db/repo.go".
		Version string `ini:"-"`

		DisableDiffHighlight bool
		MaxDiffFiles         int      `ini:"MAX_GIT_DIFF_FILES"`
		MaxDiffLines         int      `ini:"MAX_GIT_DIFF_LINES"`
		MaxDiffLineChars     int      `ini:"MAX_GIT_DIFF_LINE_CHARACTERS"`
		GCArgs               []string `ini:"GC_ARGS" delim:" "`
		Timeout              struct {
			Migrate int
			Mirror  int
			Clone   int
			Pull    int
			Diff    int
			GC      int `ini:"GC"`
		} `ini:"git.timeout"`
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

	// Other settings
	Other struct {
		ShowFooterBranding         bool
		ShowFooterTemplateLoadTime bool
	}

	// Global setting
	HasRobotsTxt bool
)

type ServerOpts struct {
	ExternalURL          string `ini:"EXTERNAL_URL"`
	Domain               string
	Protocol             string
	HTTPAddr             string `ini:"HTTP_ADDR"`
	HTTPPort             string `ini:"HTTP_PORT"`
	CertFile             string
	KeyFile              string
	TLSMinVersion        string `ini:"TLS_MIN_VERSION"`
	UnixSocketPermission string
	LocalRootURL         string `ini:"LOCAL_ROOT_URL"`

	OfflineMode      bool
	DisableRouterLog bool
	EnableGzip       bool

	AppDataPath        string
	LoadAssetsFromDisk bool

	LandingURL string `ini:"LANDING_URL"`

	// Derived from other static values
	URL            *url.URL    `ini:"-"` // Parsed URL object of ExternalURL.
	Subpath        string      `ini:"-"` // Subpath found the ExternalURL. Should be empty when not found.
	SubpathDepth   int         `ini:"-"` // The number of slashes found in the Subpath.
	UnixSocketMode os.FileMode `ini:"-"` // Parsed file mode of UnixSocketPermission.
}

// Server settings
var Server ServerOpts

type DatabaseOpts struct {
	Type         string
	Host         string
	Name         string
	User         string
	Password     string
	SSLMode      string `ini:"SSL_MODE"`
	Path         string
	MaxOpenConns int
	MaxIdleConns int
}

// Database settings
var Database DatabaseOpts

type LFSOpts struct {
	Storage     string
	ObjectsPath string
}

// LFS settings
var LFS LFSOpts

type i18nConf struct {
	Langs     []string          `delim:","`
	Names     []string          `delim:","`
	dateLangs map[string]string `ini:"-"`
}

// DateLang transforms standard language locale name to corresponding value in datetime plugin.
func (c *i18nConf) DateLang(lang string) string {
	name, ok := c.dateLangs[lang]
	if ok {
		return name
	}
	return "en"
}

// I18n settings
var I18n *i18nConf

// handleDeprecated transfers deprecated values to the new ones when set.
func handleDeprecated() {
	// Add fallback logic here, example:
	// if App.AppName != "" {
	// 	App.BrandName = App.AppName
	// 	App.AppName = ""
	// }
}

// HookMode indicates whether program starts as Git server-side hook callback.
// All operations should be done synchronously to prevent program exits before finishing.
//
// ⚠️ WARNING: Should only be set by "internal/cmd/serv.go".
var HookMode bool

// Indicates which database backend is currently being used.
var (
	UseSQLite3    bool
	UseMySQL      bool
	UsePostgreSQL bool
	UseMSSQL      bool
)
