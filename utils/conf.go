// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/Unknwon/goconfig"
)

var Cfg *goconfig.ConfigFile

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

	cfgPath := filepath.Join(workDir, "conf", "app.ini")
	Cfg, err = goconfig.LoadConfigFile(cfgPath)
	if err != nil {
		fmt.Printf("Cannot load config file '%s'\n", cfgPath)
		os.Exit(2)
	}
	Cfg.BlockMode = false
}
