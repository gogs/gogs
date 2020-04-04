// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-macaron/cache/memcache"
	_ "github.com/go-macaron/cache/redis"
	_ "github.com/go-macaron/session/redis"
	"github.com/gogs/go-libravatar"
	"github.com/mcuadros/go-version"
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/assets/conf"
	"gogs.io/gogs/internal/osutil"
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
// NOTE: The order of loading configuration sections matters as one may depend on another.
//
// ⚠️ WARNING: Do not print anything in this function other than warnings.
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

	// *****************************
	// ----- Database settings -----
	// *****************************

	if err = File.Section("database").MapTo(&Database); err != nil {
		return errors.Wrap(err, "mapping [database] section")
	}
	Database.Path = ensureAbs(Database.Path)

	// *****************************
	// ----- Security settings -----
	// *****************************

	if err = File.Section("security").MapTo(&Security); err != nil {
		return errors.Wrap(err, "mapping [security] section")
	}

	// Check run user when the install is locked.
	if Security.InstallLock {
		currentUser, match := CheckRunUser(App.RunUser)
		if !match {
			return fmt.Errorf("user configured to run Gogs is %q, but the current user is %q", App.RunUser, currentUser)
		}
	}

	// **************************
	// ----- Email settings -----
	// **************************

	if err = File.Section("email").MapTo(&Email); err != nil {
		return errors.Wrap(err, "mapping [email] section")
	}
	// LEGACY [0.13]: In case there are values with old section name.
	if err = File.Section("mailer").MapTo(&Email); err != nil {
		return errors.Wrap(err, "mapping [mailer] section")
	}

	if Email.Enabled {
		if Email.From == "" {
			Email.From = Email.User
		}

		parsed, err := mail.ParseAddress(Email.From)
		if err != nil {
			return errors.Wrapf(err, "parse mail address %q", Email.From)
		}
		Email.FromEmail = parsed.Address
	}

	// ***********************************
	// ----- Authentication settings -----
	// ***********************************

	if err = File.Section("auth").MapTo(&Auth); err != nil {
		return errors.Wrap(err, "mapping [auth] section")
	}
	// LEGACY [0.13]: In case there are values with old section name.
	if err = File.Section("service").MapTo(&Auth); err != nil {
		return errors.Wrap(err, "mapping [service] section")
	}

	// *************************
	// ----- User settings -----
	// *************************

	if err = File.Section("user").MapTo(&User); err != nil {
		return errors.Wrap(err, "mapping [user] section")
	}

	// ****************************
	// ----- Session settings -----
	// ****************************

	if err = File.Section("session").MapTo(&Session); err != nil {
		return errors.Wrap(err, "mapping [session] section")
	}

	// *******************************
	// ----- Attachment settings -----
	// *******************************

	if err = File.Section("attachment").MapTo(&Attachment); err != nil {
		return errors.Wrap(err, "mapping [attachment] section")
	}
	Attachment.Path = ensureAbs(Attachment.Path)

	// *************************
	// ----- Time settings -----
	// *************************

	if err = File.Section("time").MapTo(&Time); err != nil {
		return errors.Wrap(err, "mapping [time] section")
	}

	Time.FormatLayout = map[string]string{
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
	}[Time.Format]
	if Time.FormatLayout == "" {
		return fmt.Errorf("unrecognized '[time] FORMAT': %s", Time.Format)
	}

	// ****************************
	// ----- Picture settings -----
	// ****************************

	if err = File.Section("picture").MapTo(&Picture); err != nil {
		return errors.Wrap(err, "mapping [picture] section")
	}
	Picture.AvatarUploadPath = ensureAbs(Picture.AvatarUploadPath)
	Picture.RepositoryAvatarUploadPath = ensureAbs(Picture.RepositoryAvatarUploadPath)

	switch Picture.GravatarSource {
	case "gravatar":
		Picture.GravatarSource = "https://secure.gravatar.com/avatar/"
	case "libravatar":
		Picture.GravatarSource = "https://seccdn.libravatar.org/avatar/"
	}

	if Server.OfflineMode {
		Picture.DisableGravatar = true
		Picture.EnableFederatedAvatar = false
	}
	if Picture.DisableGravatar {
		Picture.EnableFederatedAvatar = false
	}
	if Picture.EnableFederatedAvatar {
		gravatarURL, err := url.Parse(Picture.GravatarSource)
		if err != nil {
			return errors.Wrapf(err, "parse Gravatar source %q", Picture.GravatarSource)
		}

		Picture.LibravatarService = libravatar.New()
		if gravatarURL.Scheme == "https" {
			Picture.LibravatarService.SetUseHTTPS(true)
			Picture.LibravatarService.SetSecureFallbackHost(gravatarURL.Host)
		} else {
			Picture.LibravatarService.SetUseHTTPS(false)
			Picture.LibravatarService.SetFallbackHost(gravatarURL.Host)
		}
	}

	// ***************************
	// ----- Mirror settings -----
	// ***************************

	if err = File.Section("mirror").MapTo(&Mirror); err != nil {
		return errors.Wrap(err, "mapping [mirror] section")
	}

	if Mirror.DefaultInterval <= 0 {
		Mirror.DefaultInterval = 8
	}

	// *************************
	// ----- I18n settings -----
	// *************************

	I18n = new(i18nConf)
	if err = File.Section("i18n").MapTo(I18n); err != nil {
		return errors.Wrap(err, "mapping [i18n] section")
	}
	I18n.dateLangs = File.Section("i18n.datelang").KeysHash()

	handleDeprecated()

	if err = File.Section("cache").MapTo(&Cache); err != nil {
		return errors.Wrap(err, "mapping [cache] section")
	} else if err = File.Section("http").MapTo(&HTTP); err != nil {
		return errors.Wrap(err, "mapping [http] section")
	} else if err = File.Section("lfs").MapTo(&LFS); err != nil {
		return errors.Wrap(err, "mapping [lfs] section")
	} else if err = File.Section("release").MapTo(&Release); err != nil {
		return errors.Wrap(err, "mapping [release] section")
	} else if err = File.Section("webhook").MapTo(&Webhook); err != nil {
		return errors.Wrap(err, "mapping [webhook] section")
	} else if err = File.Section("markdown").MapTo(&Markdown); err != nil {
		return errors.Wrap(err, "mapping [markdown] section")
	} else if err = File.Section("smartypants").MapTo(&Smartypants); err != nil {
		return errors.Wrap(err, "mapping [smartypants] section")
	} else if err = File.Section("admin").MapTo(&Admin); err != nil {
		return errors.Wrap(err, "mapping [admin] section")
	} else if err = File.Section("cron").MapTo(&Cron); err != nil {
		return errors.Wrap(err, "mapping [cron] section")
	} else if err = File.Section("git").MapTo(&Git); err != nil {
		return errors.Wrap(err, "mapping [git] section")
	} else if err = File.Section("api").MapTo(&API); err != nil {
		return errors.Wrap(err, "mapping [api] section")
	} else if err = File.Section("ui").MapTo(&UI); err != nil {
		return errors.Wrap(err, "mapping [ui] section")
	} else if err = File.Section("prometheus").MapTo(&Prometheus); err != nil {
		return errors.Wrap(err, "mapping [prometheus] section")
	} else if err = File.Section("other").MapTo(&Other); err != nil {
		return errors.Wrap(err, "mapping [other] section")
	}

	HasRobotsTxt = osutil.IsFile(filepath.Join(CustomDir(), "robots.txt"))
	return nil
}

// MustInit panics if configuration initialization failed.
func MustInit(customConf string) {
	err := Init(customConf)
	if err != nil {
		panic(err)
	}
}
