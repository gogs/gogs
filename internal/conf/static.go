// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

// ℹ️ README: This file contains static values that should only be set at initialization time.

// HasMinWinSvc is whether the application is built with Windows Service support.
var HasMinWinSvc bool

// CustomConf returns the absolute path of custom configuration file that is used.
var CustomConf string

var (
	// Application settings
	App struct {
		BrandName string `ini:"APP_NAME"`
		RunUser   string
		RunMode   string
	}

	// Server settings: [server]
	Server struct {
	}
)
