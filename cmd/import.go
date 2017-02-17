// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Unknwon/com"
	"github.com/urfave/cli"

	"github.com/gogits/gogs/modules/setting"
)

var (
	CmdImport = cli.Command{
		Name:  "import",
		Usage: "Import portable data as local Gogs data",
		Description: `Allow user import data from other Gogs installations to local instance
without manually hacking the data files`,
		Subcommands: []cli.Command{
			subcmdImportLocale,
		},
	}

	subcmdImportLocale = cli.Command{
		Name:   "locale",
		Usage:  "Import locale files to local repository",
		Action: runImportLocale,
		Flags: []cli.Flag{
			stringFlag("source", "", "Source directory that stores new locale files"),
			stringFlag("target", "", "Target directory that stores old locale files"),
			stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		},
	}
)

func runImportLocale(c *cli.Context) error {
	if !c.IsSet("source") {
		return fmt.Errorf("Source directory is not specified")
	} else if !c.IsSet("target") {
		return fmt.Errorf("Target directory is not specified")
	}
	if !com.IsDir(c.String("source")) {
		return fmt.Errorf("Source directory does not exist or is not a directory")
	} else if !com.IsDir(c.String("target")) {
		return fmt.Errorf("Target directory does not exist or is not a directory")
	}

	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}

	setting.NewContext()

	now := time.Now()

	// Cut out en-US.
	for _, lang := range setting.Langs[1:] {
		name := fmt.Sprintf("locale_%s.ini", lang)
		source := filepath.Join(c.String("source"), name)
		target := filepath.Join(c.String("target"), name)
		if !com.IsFile(source) {
			continue
		}

		if err := com.Copy(source, target); err != nil {
			return fmt.Errorf("Copy file: %v", err)
		}

		// Modification time of files from Crowdin often ahead of current,
		// so we need to set back to current.
		os.Chtimes(target, now, now)
	}

	fmt.Println("Locale files has been successfully imported!")
	return nil
}
