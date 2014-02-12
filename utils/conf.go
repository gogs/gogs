// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"os"

	"github.com/Unknwon/goconfig"
)

var Cfg *goconfig.ConfigFile

func init() {
	var err error
	Cfg, err = goconfig.LoadConfigFile("conf/app.ini")
	if err != nil {
		fmt.Println("Cannot load config file 'app.ini'")
		os.Exit(2)
	}
	Cfg.BlockMode = false
}
