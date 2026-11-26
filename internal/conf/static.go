package conf

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"unknwon.dev/ini/v2"

	"gogs.io/gogs/conf"
)

// ℹ️ README: This file contains static values that should only be set at initialization time.
//
// ⚠️ WARNING: After changing any options, do not forget to update the
// "/admin/config" page as well.

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

var (
	// Security settings
	Security struct {
		SecretKey               string
		LoginRememberDays       int
		EnableLoginStatusCookie bool
		LoginStatusCookieName   string
		LocalNetworkAllowlist   []string `delim:","`
	}

	// Email settings
	Email struct {
		Enabled       bool
		SubjectPrefix string
		Host          string
		From          string
		User          string
		Password      string

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
		// ⚠️ WARNING: Should only be set by "internal/database/repositories.go".
		Version string `ini:"-"`

		GCArgs  []string `ini:"GC_ARGS" delim:" "`
		Timeout struct {
			Migrate int
			Mirror  int
			GC      int `ini:"GC"`
		} `ini:"git.timeout"`
	}

	// API settings
	API struct {
		MaxResponseItems int
	}

	// Prometheus settings
	Prometheus struct {
		Enabled           bool
		EnableBasicAuth   bool
		BasicAuthUsername string
		BasicAuthPassword string
	}

	// Tracing settings
	Tracing struct {
		Enabled                  bool
		EnableAdminCollector     bool
		MaxMemoryMiB             int64 `ini:"MAX_MEMORY_MIB"`
		MaxAge                   time.Duration
		SlowThreshold            time.Duration
		SamplesPerRoutePerMinute int
	}

	// Global setting
	HasRobotsTxt bool
)

type CacheOptions struct {
	Adapter  string
	Interval int
	Host     string
}

var Cache CacheOptions

type AppOpts struct {
	// ⚠️ WARNING: Should only be set by the main package (i.e., "cmd/gogs/main.go").
	Version string `ini:"-"`

	BrandName string
	RunUser   string
	RunMode   string
}

// Application settings
var App AppOpts

type AuthOpts struct {
	ActivateCodeLives           int
	ResetPasswordCodeLives      int
	RequireEmailConfirmation    bool
	RequireSigninView           bool
	DisableRegistration         bool
	AntiBotVerificationStrategy string
	// Whether sign up and log in on the web ask for the anti-bot
	// verification, which is off for both when AntiBotVerificationStrategy is empty.
	AntiBotVerificationOnSignUp bool `ini:"ANTI_BOT_VERIFICATION_ON_SIGN_UP"`
	AntiBotVerificationOnLogIn  bool `ini:"ANTI_BOT_VERIFICATION_ON_LOG_IN"`

	EnableReverseProxyAuthentication   bool
	EnableReverseProxyAutoRegistration bool
	ReverseProxyAuthenticationHeader   string
	TrustedProxyIPs                    []string `ini:"TRUSTED_PROXY_IPS"`
	CustomLogoutURL                    string   `ini:"CUSTOM_LOGOUT_URL"`

	// Derived from other static values
	TrustedProxyCIDRs []*net.IPNet `ini:"-"` // Parsed CIDR form of TrustedProxyIPs.
}

// Authentication settings
var Auth AuthOpts

// Values of [auth] ANTI_BOT_VERIFICATION_STRATEGY.
const (
	// AntiBotVerificationStrategyAltcha asks the browser to solve a proof-of-work challenge.
	AntiBotVerificationStrategyAltcha = "altcha"
	// AntiBotVerificationStrategyCaptcha asks the user to type the characters shown in
	// an image.
	AntiBotVerificationStrategyCaptcha = "captcha"
)

type ServerOpts struct {
	ExternalURL          string `ini:"EXTERNAL_URL"`
	Domain               string
	Protocol             string
	HTTPAddr             string `ini:"HTTP_ADDR"`
	HTTPPort             int    `ini:"HTTP_PORT"`
	CertFile             string
	KeyFile              string
	TLSMinVersion        string `ini:"TLS_MIN_VERSION"`
	UnixSocketPermission string
	LocalRootURL         string `ini:"LOCAL_ROOT_URL"`

	DisableRouterLog bool
	EnableGzip       bool

	AppDataPath        string
	LoadAssetsFromDisk bool

	LandingURL string `ini:"LANDING_URL"`

	// Derived from other static values
	URL            *url.URL    `ini:"-"` // Parsed URL object of ExternalURL.
	Subpath        string      `ini:"-"` // Subpath found in the ExternalURL. Should be empty when not found.
	SubpathDepth   int         `ini:"-"` // The number of slashes found in the Subpath.
	UnixSocketMode os.FileMode `ini:"-"` // Parsed file mode of UnixSocketPermission.
}

// Server settings
var Server ServerOpts

type SSHOpts struct {
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
	ServerAlgorithms   []string `ini:"SSH_SERVER_ALGORITHMS"`
}

// SSH settings
var SSH SSHOpts

type RepositoryOpts struct {
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
	DefaultBranch            string

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

// Repository settings
var Repository RepositoryOpts

type DatabaseOpts struct {
	Type         string
	Host         string
	Name         string
	Schema       string
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
	Storage         string
	ObjectsPath     string
	ObjectsTempPath string
}

// LFS settings
var LFS LFSOpts

type UIUserOpts struct {
	RepoPagingNum     int
	NewsFeedPagingNum int
	CommitsPagingNum  int
}

type UIOpts struct {
	ExplorePagingNum   int
	IssuePagingNum     int
	FeedMaxCommitNum   int
	MaxDisplayFileSize int64

	Admin struct {
		UserPagingNum   int
		RepoPagingNum   int
		NoticePagingNum int
		OrgPagingNum    int
	} `ini:"ui.admin"`
	User UIUserOpts `ini:"ui.user"`
}

// UI settings
var UI UIOpts

type PictureOpts struct {
	AvatarUploadPath           string
	RepositoryAvatarUploadPath string
}

// Picture settings
var Picture PictureOpts

type i18nConf struct {
	Langs []string `delim:","`
	Names []string `delim:","`
}

// I18n settings
var I18n *i18nConf

// handleDeprecated transfers deprecated values to the new ones when set, and
// returns a warning for each deprecated option set.
func handleDeprecated() (warnings []string) {
	// [auth] ENABLE_REGISTRATION_CAPTCHA turns the verification of sign up
	// on or off, with the default altcha when no strategy is set.
	if key, err := File.Section("auth").GetKey("ENABLE_REGISTRATION_CAPTCHA"); err == nil {
		Auth.AntiBotVerificationOnSignUp = key.MustBool()
		if Auth.AntiBotVerificationOnSignUp && Auth.AntiBotVerificationStrategy == "" {
			Auth.AntiBotVerificationStrategy = AntiBotVerificationStrategyAltcha
		}
	}

	for oldPath, newPath := range deprecatedOptionPaths {
		if File.Section(oldPath.section).HasKey(oldPath.option) {
			warnings = append(
				warnings,
				fmt.Sprintf("option [%s] %s is deprecated, use [%s] %s instead",
					oldPath.section, oldPath.option,
					newPath.section, newPath.option,
				),
			)
		}
	}
	return warnings
}

type optionPath struct {
	section string
	option  string
}

// deprecatedOptionPaths maps deprecated options, which still work through
// handleDeprecated, to the options replacing them.
var deprecatedOptionPaths = map[optionPath]optionPath{
	{"auth", "ENABLE_REGISTRATION_CAPTCHA"}: {"auth", "ANTI_BOT_VERIFICATION_ON_SIGN_UP"},
}

// checkInvalidOptions checks invalid (renamed/deleted) configuration sections
// and options and returns a list of warnings.
func checkInvalidOptions(config *ini.File) (warnings []string) {
	renamedSections := map[string]string{
		"mailer":  "email",
		"service": "auth",
	}
	for oldSection, newSection := range renamedSections {
		if len(config.Section(oldSection).KeyNames()) > 0 {
			warnings = append(warnings, fmt.Sprintf("section [%s] is invalid, use [%s] instead", oldSection, newSection))
		}
	}

	renamedOptionPaths := map[optionPath]optionPath{
		// Example:
		// {"security", "REVERSE_PROXY_AUTHENTICATION_USER"}: {"auth", "REVERSE_PROXY_AUTHENTICATION_HEADER"},
	}
	for oldPath, newPath := range renamedOptionPaths {
		if config.Section(oldPath.section).HasKey(oldPath.option) {
			warnings = append(
				warnings,
				fmt.Sprintf("option [%s] %s is invalid, use [%s] %s instead",
					oldPath.section, oldPath.option,
					newPath.section, newPath.option,
				),
			)
		}
	}

	// Check options that don't exist anymore.
	defaultConfigData, err := conf.Files.ReadFile("app.ini")
	if err != nil {
		// Warning is best-effort, OK to skip on error.
		return warnings
	}
	defaultConfig, err := ini.Load(defaultConfigData, ini.IgnoreInlineComment(true))
	if err != nil {
		// Warning is best-effort, OK to skip on error.
		return warnings
	}
	for _, section := range config.Sections() {
		// Skip sections already warned about.
		if _, ok := renamedSections[section.Name()]; ok {
			continue
		}
		for _, option := range section.Keys() {
			if _, ok := renamedOptionPaths[optionPath{section.Name(), option.Name()}]; ok {
				continue
			}
			if _, ok := deprecatedOptionPaths[optionPath{section.Name(), option.Name()}]; ok {
				continue
			}
			if !defaultConfig.Section(section.Name()).HasKey(option.Name()) {
				warnings = append(warnings, fmt.Sprintf("option [%s] %s is invalid", section.Name(), option.Name()))
			}
		}
	}

	return warnings
}

// HookMode indicates whether program starts as Git server-side hook callback.
// All operations should be done synchronously to prevent the program from exiting before finishing.
//
// ⚠️ WARNING: Should only be set by "cmd/gogs/serv.go".
var HookMode bool

// Indicates which database backend is currently being used.
var (
	UseSQLite3    bool
	UseMySQL      bool
	UsePostgreSQL bool
)

// UsersAvatarPathPrefix is the path prefix to user avatars. It is under "-",
// a reserved username, so it never shadows the pages of a user.
const UsersAvatarPathPrefix = "-/avatars"

// GeneratedAvatarsPathPrefix is the path prefix to avatars generated from a
// seed, for people who have no user, e.g., commit authors.
const GeneratedAvatarsPathPrefix = UsersAvatarPathPrefix + "/seed"
