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

	"github.com/gogits/gogs/modules/log"
)

// Mailer represents a mail service.
type Mailer struct {
	Name         string
	Host         string
	User, Passwd string
}

var (
	AppVer      string
	AppName     string
	Domain      string
	Cfg         *goconfig.ConfigFile
	MailService *Mailer
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
	Cfg.BlockMode = false

	cfgPath = filepath.Join(workDir, "custom/conf/app.ini")
	if com.IsFile(cfgPath) {
		if err = Cfg.AppendFiles(cfgPath); err != nil {
			fmt.Printf("Cannot load config file '%s'\n", cfgPath)
			os.Exit(2)
		}
	}
	Cfg.BlockMode = false

	AppName = Cfg.MustValue("", "APP_NAME", "Gogs: Go Git Service")
	Domain = Cfg.MustValue("server", "DOMAIN")

	// Check mailer setting.
	if Cfg.MustBool("mailer", "ENABLED") {
		MailService = &Mailer{
			Name:   Cfg.MustValue("mailer", "NAME", AppName),
			Host:   Cfg.MustValue("mailer", "HOST", "127.0.0.1:25"),
			User:   Cfg.MustValue("mailer", "USER", "example@example.com"),
			Passwd: Cfg.MustValue("mailer", "PASSWD", "******"),
		}
		log.Info("Mail Service Enabled")
	}
}
