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

// CustomConf returns the absolute path of custom configuration file that is used.
var CustomConf string

// Build information should only be set by -ldflags.
var (
	BuildTime   string
	BuildCommit string
)

var (
	// Application settings
	App struct {
		BrandName string
		RunUser   string
		RunMode   string

		// Deprecated: Use BrandName instead, will be removed in 0.13.
		AppName string
	}

	// Server settings: [server]
	Server struct {
		ExternalURL          string `ini:"EXTERNAL_URL"`
		Domain               string
		Protocol             string
		HTTPAddr             string `ini:"HTTP_ADDR"`
		HTTPPort             string `ini:"HTTP_PORT"`
		UnixSocketPermission string

		LocalRootURL string `ini:"LOCAL_ROOT_URL"`

		// Deprecated: Use ExternalURL instead, will be removed in 0.13.
		RootURL string `ini:"ROOT_URL"`

		// Derived from other static values
		URL            *url.URL    `ini:"-"` // Parsed URL object of ExternalURL.
		Subpath        string      `ini:"-"` // Subpath found the ExternalURL. Should be empty when not found.
		SubpathDepth   int         `ini:"-"` // The number of slashes found in the Subpath.
		UnixSocketMode os.FileMode `ini:"-"` // Parsed file mode of UnixSocketPermission.
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
}
