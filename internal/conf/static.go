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
var HasMinWinSvc bool

// Build information should only be set by -ldflags.
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
		// ⚠️ WARNING: Should only be set by gogs.go.
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
)

// transferDeprecated transfers deprecated values to the new ones when set.
func transferDeprecated() {
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
}
