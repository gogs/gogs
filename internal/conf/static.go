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

// SecurityOpts are the Security settings.
type SecurityOpts struct {
	SecretKey               string
	LoginRememberDays       int
	EnableLoginStatusCookie bool
	LoginStatusCookieName   string
	LocalNetworkAllowlist   []string `delim:","`
}

// EmailOpts are the Email settings.
type EmailOpts struct {
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

// UserOpts are the User settings.
type UserOpts struct {
	EnableEmailNotification bool
}

// SessionOpts are the Session settings.
type SessionOpts struct {
	Provider       string
	ProviderConfig string
	CookieName     string
	CookieSecure   bool
	GCInterval     int64 `ini:"GC_INTERVAL"`
	MaxLifeTime    int64
}

// HTTPOpts are the HTTP settings.
type HTTPOpts struct {
	AccessControlAllowOrigin string
}

// AttachmentOpts are the Attachment settings.
type AttachmentOpts struct {
	Enabled      bool
	Path         string
	AllowedTypes []string `delim:"|"`
	MaxSize      int64
	MaxFiles     int
}

// ReleaseOpts are the Release settings.
type ReleaseOpts struct {
	Attachment struct {
		Enabled      bool
		AllowedTypes []string `delim:"|"`
		MaxSize      int64
		MaxFiles     int
	} `ini:"release.attachment"`
}

// MirrorOpts are the Mirror settings.
type MirrorOpts struct {
	DefaultInterval int
}

// WebhookOpts are the Webhook settings.
type WebhookOpts struct {
	Types          []string
	DeliverTimeout int
	SkipTLSVerify  bool `ini:"SKIP_TLS_VERIFY"`
	PagingNum      int
}

// MarkdownOpts are the Markdown settings.
type MarkdownOpts struct {
	EnableHardLineBreak bool
	CustomURLSchemes    []string `ini:"CUSTOM_URL_SCHEMES"`
	FileExtensions      []string
}

// SmartypantsOpts are the Smartypants settings.
type SmartypantsOpts struct {
	Enabled      bool
	Fractions    bool
	Dashes       bool
	LatexDashes  bool
	AngledQuotes bool
}

// AdminOpts are the Admin settings.
type AdminOpts struct {
	DisableRegularOrgCreation bool
}

// CronOpts are the Cron tasks.
type CronOpts struct {
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

// GitOpts are the Git settings.
type GitOpts struct {
	GCArgs  []string `ini:"GC_ARGS" delim:" "`
	Timeout struct {
		Migrate int
		Mirror  int
		GC      int `ini:"GC"`
	} `ini:"git.timeout"`
}

// APIOpts are the API settings.
type APIOpts struct {
	MaxResponseItems int
}

// PrometheusOpts are the Prometheus settings.
type PrometheusOpts struct {
	Enabled           bool
	EnableBasicAuth   bool
	BasicAuthUsername string
	BasicAuthPassword string
}

// TracingOpts are the Tracing settings.
type TracingOpts struct {
	Enabled                  bool
	EnableAdminCollector     bool
	MaxMemoryMiB             int64 `ini:"MAX_MEMORY_MIB"`
	MaxAge                   time.Duration
	SlowThreshold            time.Duration
	SamplesPerRoutePerMinute int
}

type CacheOptions struct {
	Adapter  string
	Interval int
	Host     string
}

type AppOpts struct {
	// ⚠️ WARNING: Should only be set by the main package (i.e., "cmd/gogs/main.go").
	Version string `ini:"-"`

	BrandName string
	RunUser   string
	RunMode   string
}

type AuthOpts struct {
	ActivateCodeLives        int
	ResetPasswordCodeLives   int
	RequireEmailConfirmation bool
	RequireSigninView        bool
	DisableRegistration      bool
	AntiBotVerification      string

	EnableReverseProxyAuthentication   bool
	EnableReverseProxyAutoRegistration bool
	ReverseProxyAuthenticationHeader   string
	TrustedProxyIPs                    []string `ini:"TRUSTED_PROXY_IPS"`
	CustomLogoutURL                    string   `ini:"CUSTOM_LOGOUT_URL"`

	// Derived from other static values
	TrustedProxyCIDRs []*net.IPNet `ini:"-"` // Parsed CIDR form of TrustedProxyIPs.
}

// Values of [auth] ANTI_BOT_VERIFICATION.
const (
	// AntiBotVerificationAltcha asks the browser to solve a proof-of-work challenge.
	AntiBotVerificationAltcha = "altcha"
	// AntiBotVerificationCaptcha asks the user to type the characters shown in
	// an image.
	AntiBotVerificationCaptcha = "captcha"
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

type LFSOpts struct {
	Storage         string
	ObjectsPath     string
	ObjectsTempPath string
}

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

type PictureOpts struct {
	AvatarUploadPath           string
	RepositoryAvatarUploadPath string
}

type I18nOpts struct {
	Langs []string `delim:","`
	Names []string `delim:","`
}

// handleDeprecated transfers deprecated values in the file to the new options
// of auth when set.
func handleDeprecated(file *ini.File, auth *AuthOpts) {
	// [auth] ENABLE_REGISTRATION_CAPTCHA only switches between the default
	// altcha and no verification, so an explicit "captcha" is kept.
	if key, err := file.Section("auth").GetKey("ENABLE_REGISTRATION_CAPTCHA"); err == nil {
		if key.MustBool() {
			if auth.AntiBotVerification == "" {
				auth.AntiBotVerification = AntiBotVerificationAltcha
			}
		} else if auth.AntiBotVerification == AntiBotVerificationAltcha {
			auth.AntiBotVerification = ""
		}
	}
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

	type optionPath struct {
		section string
		option  string
	}
	renamedOptionPaths := map[optionPath]optionPath{
		// Example:
		// {"security", "REVERSE_PROXY_AUTHENTICATION_USER"}: {"auth", "REVERSE_PROXY_AUTHENTICATION_HEADER"},
	}
	// Deprecated options still work, see handleDeprecated.
	deprecatedOptionPaths := map[optionPath]optionPath{
		{"auth", "ENABLE_REGISTRATION_CAPTCHA"}: {"auth", "ANTI_BOT_VERIFICATION"},
	}
	for oldPath, newPath := range deprecatedOptionPaths {
		if config.Section(oldPath.section).HasKey(oldPath.option) {
			warnings = append(
				warnings,
				fmt.Sprintf("option [%s] %s is deprecated, use [%s] %s instead",
					oldPath.section, oldPath.option,
					newPath.section, newPath.option,
				),
			)
		}
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

// UsersAvatarPathPrefix is the path prefix to user avatars. It is under "-",
// a reserved username, so it never shadows the pages of a user.
const UsersAvatarPathPrefix = "-/avatars"

// GeneratedAvatarsPathPrefix is the path prefix to avatars generated from a
// seed, for people who have no user, e.g., commit authors.
const GeneratedAvatarsPathPrefix = UsersAvatarPathPrefix + "/seed"
