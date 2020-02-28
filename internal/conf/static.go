// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"net/url"
	"os"
)

// ℹ️ README: This file contains static values that should only be set at initialization time.

// HasMinWinSvc is whether the application is built with Windows Service support.
//
// ⚠️ WARNING: should only be set by "static_minwinsvc.go".
var HasMinWinSvc bool

// Build time and commit information.
//
// ⚠️ WARNING: should only be set by -ldflags.
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
		// ⚠️ WARNING: Should only be set by main package (i.e. "gogs.go").
		Version string `ini:"-"`

		BrandName string
		RunUser   string
		RunMode   string

		// Deprecated: Use BrandName instead, will be removed in 0.13.
		AppName string
	}

	// Server settings
	Server struct {
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

		// Deprecated: Use ExternalURL instead, will be removed in 0.13.
		RootURL string `ini:"ROOT_URL"`
		// Deprecated: Use LandingURL instead, will be removed in 0.13.
		LangdingPage string `ini:"LANDING_PAGE"`
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

	// Database settings
	Database struct {
		Type     string
		Host     string
		Name     string
		User     string
		Password string
		SSLMode  string `ini:"SSL_MODE"`
		Path     string

		// Deprecated: Use Type instead, will be removed in 0.13.
		DbType string
		// Deprecated: Use Password instead, will be removed in 0.13.
		Passwd string
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

		// Deprecated: Use Auth.ReverseProxyAuthenticationHeader instead, will be removed in 0.13.
		ReverseProxyAuthenticationUser string
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

		// Deprecated: Use Password instead, will be removed in 0.13.
		Passwd string
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

		// Deprecated: Use ActivateCodeLives instead, will be removed in 0.13.
		ActiveCodeLiveMinutes int
		// Deprecated: Use ResetPasswordCodeLives instead, will be removed in 0.13.
		ResetPasswdCodeLiveMinutes int
		// Deprecated: Use RequireEmailConfirmation instead, will be removed in 0.13.
		RegisterEmailConfirm bool
		// Deprecated: Use EnableRegistrationCaptcha instead, will be removed in 0.13.
		EnableCaptcha bool
		// Deprecated: Use User.EnableEmailNotification instead, will be removed in 0.13.
		EnableNotifyMail bool
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

		// Deprecated: Use GCInterval instead, will be removed in 0.13.
		GCIntervalTime int64 `ini:"GC_INTERVAL_TIME"`
		// Deprecated: Use MaxLifeTime instead, will be removed in 0.13.
		SessionLifeTime int64
	}
)

// handleDeprecated transfers deprecated values to the new ones when set.
func handleDeprecated() {
	if App.AppName != "" {
		App.BrandName = App.AppName
		App.AppName = ""
	}

	if Server.RootURL != "" {
		Server.ExternalURL = Server.RootURL
		Server.RootURL = ""
	}
	if Server.LangdingPage == "explore" {
		Server.LandingURL = "/explore"
		Server.LangdingPage = ""
	}

	if Database.DbType != "" {
		Database.Type = Database.DbType
		Database.DbType = ""
	}
	if Database.Passwd != "" {
		Database.Password = Database.Passwd
		Database.Passwd = ""
	}

	if Email.Passwd != "" {
		Email.Password = Email.Passwd
		Email.Passwd = ""
	}

	if Auth.ActiveCodeLiveMinutes > 0 {
		Auth.ActivateCodeLives = Auth.ActiveCodeLiveMinutes
		Auth.ActiveCodeLiveMinutes = 0
	}
	if Auth.ResetPasswdCodeLiveMinutes > 0 {
		Auth.ResetPasswordCodeLives = Auth.ResetPasswdCodeLiveMinutes
		Auth.ResetPasswdCodeLiveMinutes = 0
	}
	if Auth.RegisterEmailConfirm {
		Auth.RequireEmailConfirmation = true
		Auth.RegisterEmailConfirm = false
	}
	if Auth.EnableCaptcha {
		Auth.EnableRegistrationCaptcha = true
		Auth.EnableCaptcha = false
	}
	if Security.ReverseProxyAuthenticationUser != "" {
		Auth.ReverseProxyAuthenticationHeader = Security.ReverseProxyAuthenticationUser
		Security.ReverseProxyAuthenticationUser = ""
	}

	if Auth.EnableNotifyMail {
		User.EnableEmailNotification = true
		Auth.EnableNotifyMail = false
	}

	if Session.GCIntervalTime > 0 {
		Session.GCInterval = Session.GCIntervalTime
		Session.GCIntervalTime = 0
	}
	if Session.SessionLifeTime > 0 {
		Session.MaxLifeTime = Session.SessionLifeTime
		Session.SessionLifeTime = 0
	}
}
