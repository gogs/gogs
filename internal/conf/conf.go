package conf

import (
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/conf"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/semverx"
)

// Config is the configuration loaded from conf assets and the custom
// configuration file.
type Config struct {
	// File is the raw configuration that the options are mapped from.
	File *ini.File
	// CustomConf is the absolute path of the custom configuration file used.
	CustomConf string

	App         AppOpts
	Server      ServerOpts
	SSH         SSHOpts
	Repository  RepositoryOpts
	Database    DatabaseOpts
	Security    SecurityOpts
	Email       EmailOpts
	Auth        AuthOpts
	User        UserOpts
	Session     SessionOpts
	Attachment  AttachmentOpts
	Picture     PictureOpts
	Mirror      MirrorOpts
	I18n        I18nOpts
	LFS         LFSOpts
	Cache       CacheOptions
	HTTP        HTTPOpts
	Release     ReleaseOpts
	Webhook     WebhookOpts
	Markdown    MarkdownOpts
	Smartypants SmartypantsOpts
	Admin       AdminOpts
	Cron        CronOpts
	Git         GitOpts
	API         APIOpts
	UI          UIOpts
	Prometheus  PrometheusOpts
	Tracing     TracingOpts
	// Log is set by InitLogging.
	Log *LogOpts

	// HasRobotsTxt is whether the custom directory has a "robots.txt".
	HasRobotsTxt bool
}

// Init returns the configuration loaded from conf assets and given custom
// configuration file. If `customConf` is empty, it falls back to default
// location, i.e., "<CUSTOM DIR>/conf/app.ini". When hookMode is true, i.e., the
// program runs as a Git server-side hook callback, a missing custom
// configuration file is allowed and no warnings are logged.
//
// NOTE: The order of loading configuration sections matters as one may depend on another.
//
// ⚠️ WARNING: Do not print anything in this function other than warnings.
func Init(logger *logx.Logger, customConf string, hookMode bool) (*Config, error) {
	c := new(Config)
	data, err := conf.Files.ReadFile("app.ini")
	if err != nil {
		return nil, errors.Wrap(err, `read default "app.ini"`)
	}

	c.File, err = ini.Load(
		data,
		ini.IgnoreInlineComment(true),
		ini.NameMapping(ini.SnakeCase),
		ini.ValueMapping(os.ExpandEnv),
	)
	if err != nil {
		return nil, errors.Wrap(err, `parse "app.ini"`)
	}

	if customConf == "" {
		customConf = filepath.Join(CustomDir(), "conf", "app.ini")
	} else {
		customConf, err = filepath.Abs(customConf)
		if err != nil {
			return nil, errors.Wrap(err, "get absolute path")
		}
	}
	c.CustomConf = customConf

	if osx.IsFile(customConf) {
		if err = c.File.Append(customConf); err != nil {
			return nil, errors.Wrapf(err, "append %q", customConf)
		}
	} else if !hookMode {
		return nil, errors.Newf("custom config %q not found: see https://gogs.io/getting-started/installation#configuration for first-time setup", customConf)
	}

	if err = c.File.Section(ini.DefaultSection).MapTo(&c.App); err != nil {
		return nil, errors.Wrap(err, "mapping default section")
	}

	// ***************************
	// ----- Server settings -----
	// ***************************

	if err = c.File.Section("server").MapTo(&c.Server); err != nil {
		return nil, errors.Wrap(err, "mapping [server] section")
	}
	c.Server.AppDataPath = ensureAbs(c.Server.AppDataPath)
	if c.Server.CertFile != "" {
		c.Server.CertFile = ensureAbs(c.Server.CertFile)
	}
	if c.Server.KeyFile != "" {
		c.Server.KeyFile = ensureAbs(c.Server.KeyFile)
	}

	if !strings.HasSuffix(c.Server.ExternalURL, "/") {
		c.Server.ExternalURL += "/"
	}
	c.Server.URL, err = url.Parse(c.Server.ExternalURL)
	if err != nil {
		return nil, errors.Wrapf(err, "parse '[server] EXTERNAL_URL' %q", err)
	}

	// Subpath should start with '/' and end without '/', i.e., '/{subpath}'.
	c.Server.Subpath = strings.TrimRight(c.Server.URL.Path, "/")
	c.Server.SubpathDepth = strings.Count(c.Server.Subpath, "/")

	unixSocketMode, err := strconv.ParseUint(c.Server.UnixSocketPermission, 8, 32)
	if err != nil {
		return nil, errors.Wrapf(err, "parse '[server] UNIX_SOCKET_PERMISSION' %q", c.Server.UnixSocketPermission)
	}
	if unixSocketMode > 0o777 {
		unixSocketMode = 0o666
	}
	c.Server.UnixSocketMode = os.FileMode(unixSocketMode)

	// ************************
	// ----- SSH settings -----
	// ************************

	c.SSH.RootPath = filepath.Join(HomeDir(), ".ssh")
	c.SSH.KeyTestPath = os.TempDir()
	if err = c.File.Section("server").MapTo(&c.SSH); err != nil {
		return nil, errors.Wrap(err, "mapping SSH settings from [server] section")
	}
	c.SSH.RootPath = ensureAbs(c.SSH.RootPath)
	c.SSH.KeyTestPath = ensureAbs(c.SSH.KeyTestPath)

	if !c.SSH.Disabled {
		if !c.SSH.StartBuiltinServer {
			if err := os.MkdirAll(c.SSH.RootPath, 0o700); err != nil {
				return nil, errors.Wrap(err, "create SSH root directory")
			} else if err = os.MkdirAll(c.SSH.KeyTestPath, 0o644); err != nil {
				return nil, errors.Wrap(err, "create SSH key test directory")
			}
		} else {
			c.SSH.RewriteAuthorizedKeysAtStart = false
		}

		// Check if server is eligible for minimum key size check when user chooses to enable it.
		// It is forced to be disabled on Windows servers and with OpenSSH versions lower than 5.1
		// because the "ssh-keygen" in Windows does not print key type.
		// See https://github.com/gogs/gogs/issues/4507.
		if c.SSH.MinimumKeySizeCheck {
			sshVersion, err := openSSHVersion()
			if err != nil {
				return nil, errors.Wrap(err, "get OpenSSH version")
			}

			if IsWindowsRuntime() || semverx.Compare(sshVersion, "<", "5.1") {
				if !hookMode {
					logger.Warn("SSH minimum key size check is disabled because it requires a non-Windows server with OpenSSH 5.1 or later", "windowsRuntime", IsWindowsRuntime(), "openSSHVersion", sshVersion)
				}
			} else {
				c.SSH.MinimumKeySizes = map[string]int{}
				for _, key := range c.File.Section("ssh.minimum_key_sizes").Keys() {
					if key.MustInt() != -1 {
						c.SSH.MinimumKeySizes[strings.ToLower(key.Name())] = key.MustInt()
					}
				}
			}
		}
	}

	// *******************************
	// ----- Repository settings -----
	// *******************************

	c.Repository.Root = filepath.Join(HomeDir(), "gogs-repositories")
	if err = c.File.Section("repository").MapTo(&c.Repository); err != nil {
		return nil, errors.Wrap(err, "mapping [repository] section")
	}
	c.Repository.Root = ensureAbs(c.Repository.Root)
	c.Repository.Upload.TempPath = ensureAbs(c.Repository.Upload.TempPath)

	// *****************************
	// ----- Database settings -----
	// *****************************

	if err = c.File.Section("database").MapTo(&c.Database); err != nil {
		return nil, errors.Wrap(err, "mapping [database] section")
	}
	c.Database.Path = ensureAbs(c.Database.Path)

	// *****************************
	// ----- Security settings -----
	// *****************************

	if err = c.File.Section("security").MapTo(&c.Security); err != nil {
		return nil, errors.Wrap(err, "mapping [security] section")
	}

	if c.Security.SecretKey == "" || c.Security.SecretKey == "CHANGE-ME-OR-FAIL-TO-START" {
		return nil, errors.New("[security] SECRET_KEY must be set to a strong, unguessable value")
	}

	currentUser, match := CheckRunUser(c.App.RunUser)
	if !match {
		return nil, errors.Newf("user configured to run Gogs is %q, but the current user is %q", c.App.RunUser, currentUser)
	}

	// **************************
	// ----- Email settings -----
	// **************************

	if err = c.File.Section("email").MapTo(&c.Email); err != nil {
		return nil, errors.Wrap(err, "mapping [email] section")
	}
	if c.Email.CertFile != "" {
		c.Email.CertFile = ensureAbs(c.Email.CertFile)
	}
	if c.Email.KeyFile != "" {
		c.Email.KeyFile = ensureAbs(c.Email.KeyFile)
	}

	if c.Email.Enabled {
		if c.Email.From == "" {
			c.Email.From = c.Email.User
		}

		parsed, err := mail.ParseAddress(c.Email.From)
		if err != nil {
			return nil, errors.Wrapf(err, "parse mail address %q", c.Email.From)
		}
		c.Email.FromEmail = parsed.Address
	}

	// ***********************************
	// ----- Authentication settings -----
	// ***********************************

	if err = c.File.Section("auth").MapTo(&c.Auth); err != nil {
		return nil, errors.Wrap(err, "mapping [auth] section")
	}
	switch c.Auth.AntiBotVerification {
	case "", AntiBotVerificationAltcha, AntiBotVerificationCaptcha:
	default:
		return nil, errors.Newf("[auth] ANTI_BOT_VERIFICATION must be empty, %q or %q, but got %q", AntiBotVerificationAltcha, AntiBotVerificationCaptcha, c.Auth.AntiBotVerification)
	}
	for _, raw := range c.Auth.TrustedProxyIPs {
		// Allow bare IPs as a convenience by promoting them to single-host CIDRs.
		if !strings.Contains(raw, "/") {
			if ip := net.ParseIP(raw); ip != nil {
				if ip.To4() != nil {
					raw += "/32"
				} else {
					raw += "/128"
				}
			}
		}
		_, cidr, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, errors.Wrapf(err, "parse trusted proxy CIDR %q", raw)
		}
		c.Auth.TrustedProxyCIDRs = append(c.Auth.TrustedProxyCIDRs, cidr)
	}

	// *************************
	// ----- User settings -----
	// *************************

	if err = c.File.Section("user").MapTo(&c.User); err != nil {
		return nil, errors.Wrap(err, "mapping [user] section")
	}

	// ****************************
	// ----- Session settings -----
	// ****************************

	if err = c.File.Section("session").MapTo(&c.Session); err != nil {
		return nil, errors.Wrap(err, "mapping [session] section")
	}
	if c.Session.Provider == "file" {
		c.Session.ProviderConfig = ensureAbs(c.Session.ProviderConfig)
	}

	// *******************************
	// ----- Attachment settings -----
	// *******************************

	if err = c.File.Section("attachment").MapTo(&c.Attachment); err != nil {
		return nil, errors.Wrap(err, "mapping [attachment] section")
	}
	c.Attachment.Path = ensureAbs(c.Attachment.Path)

	// ****************************
	// ----- Picture settings -----
	// ****************************

	if err = c.File.Section("picture").MapTo(&c.Picture); err != nil {
		return nil, errors.Wrap(err, "mapping [picture] section")
	}
	c.Picture.AvatarUploadPath = ensureAbs(c.Picture.AvatarUploadPath)
	c.Picture.RepositoryAvatarUploadPath = ensureAbs(c.Picture.RepositoryAvatarUploadPath)

	// ***************************
	// ----- Mirror settings -----
	// ***************************

	if err = c.File.Section("mirror").MapTo(&c.Mirror); err != nil {
		return nil, errors.Wrap(err, "mapping [mirror] section")
	}

	if c.Mirror.DefaultInterval <= 0 {
		c.Mirror.DefaultInterval = 8
	}

	// *************************
	// ----- I18n settings -----
	// *************************

	if err = c.File.Section("i18n").MapTo(&c.I18n); err != nil {
		return nil, errors.Wrap(err, "mapping [i18n] section")
	}

	// *************************
	// ----- LFS settings -----
	// *************************

	if err = c.File.Section("lfs").MapTo(&c.LFS); err != nil {
		return nil, errors.Wrap(err, "mapping [lfs] section")
	}
	c.LFS.ObjectsPath = ensureAbs(c.LFS.ObjectsPath)
	c.LFS.ObjectsTempPath = ensureAbs(c.LFS.ObjectsTempPath)

	handleDeprecated(c.File, &c.Auth)
	if !hookMode {
		for _, warning := range checkInvalidOptions(c.File) {
			logger.Warn("Found invalid configuration", "warning", warning)
		}
	}

	if err = c.File.Section("cache").MapTo(&c.Cache); err != nil {
		return nil, errors.Wrap(err, "mapping [cache] section")
	} else if err = c.File.Section("http").MapTo(&c.HTTP); err != nil {
		return nil, errors.Wrap(err, "mapping [http] section")
	} else if err = c.File.Section("release").MapTo(&c.Release); err != nil {
		return nil, errors.Wrap(err, "mapping [release] section")
	} else if err = c.File.Section("webhook").MapTo(&c.Webhook); err != nil {
		return nil, errors.Wrap(err, "mapping [webhook] section")
	} else if err = c.File.Section("markdown").MapTo(&c.Markdown); err != nil {
		return nil, errors.Wrap(err, "mapping [markdown] section")
	} else if err = c.File.Section("smartypants").MapTo(&c.Smartypants); err != nil {
		return nil, errors.Wrap(err, "mapping [smartypants] section")
	} else if err = c.File.Section("admin").MapTo(&c.Admin); err != nil {
		return nil, errors.Wrap(err, "mapping [admin] section")
	} else if err = c.File.Section("cron").MapTo(&c.Cron); err != nil {
		return nil, errors.Wrap(err, "mapping [cron] section")
	} else if err = c.File.Section("git").MapTo(&c.Git); err != nil {
		return nil, errors.Wrap(err, "mapping [git] section")
	} else if err = c.File.Section("api").MapTo(&c.API); err != nil {
		return nil, errors.Wrap(err, "mapping [api] section")
	} else if err = c.File.Section("ui").MapTo(&c.UI); err != nil {
		return nil, errors.Wrap(err, "mapping [ui] section")
	} else if err = c.File.Section("prometheus").MapTo(&c.Prometheus); err != nil {
		return nil, errors.Wrap(err, "mapping [prometheus] section")
	} else if err = c.File.Section("tracing").MapTo(&c.Tracing); err != nil {
		return nil, errors.Wrap(err, "mapping [tracing] section")
	} else if c.Tracing.EnableAdminCollector && !c.Tracing.Enabled {
		return nil, errors.New("[tracing] ENABLE_ADMIN_COLLECTOR requires ENABLED = true")
	}

	c.HasRobotsTxt = osx.IsFile(filepath.Join(CustomDir(), "robots.txt"))
	return c, nil
}
