// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/Unknwon/com"
	"github.com/Unknwon/goconfig"
)

var (
	AppVer  string
	AppName string
	Cfg     *goconfig.ConfigFile
)

func exeDir() (string, error) {
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	p, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	return path.Dir(p), nil
}

func init() {
	var err error
	workDir, err := exeDir()
	if err != nil {
		fmt.Printf("Fail to get work directory: %s\n", err)
		os.Exit(2)
	}

	cfgPath := filepath.Join(workDir, "conf/app.ini")
	Cfg, err = goconfig.LoadConfigFile(cfgPath)
	if err != nil {
		fmt.Printf("Cannot load config file '%s'\n", cfgPath)
		os.Exit(2)
	}

	cfgPath = filepath.Join(workDir, "custom/conf/app.ini")
	if com.IsFile(cfgPath) {
		if err = Cfg.AppendFiles(cfgPath); err != nil {
			fmt.Printf("Cannot load config file '%s'\n", cfgPath)
			os.Exit(2)
		}
	}
	Cfg.BlockMode = false

	AppName = Cfg.MustValue("", "APP_NAME")
}
