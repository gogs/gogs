// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Unknwon/com"
	"github.com/urfave/cli"

	"github.com/gogits/gogs/pkg/setting"
)

var (
	Import = cli.Command{
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

	line := make([]byte, 0, 100)
	badChars := []byte(`="`)
	escapedQuotes := []byte(`\"`)
	regularQuotes := []byte(`"`)
	// Cut out en-US.
	for _, lang := range setting.Langs[1:] {
		name := fmt.Sprintf("locale_%s.ini", lang)
		source := filepath.Join(c.String("source"), name)
		target := filepath.Join(c.String("target"), name)
		if !com.IsFile(source) {
			continue
		}

		// Crowdin surrounds double quotes for strings contain quotes inside,
		// this breaks INI parser, we need to fix that.
		sr, err := os.Open(source)
		if err != nil {
			return fmt.Errorf("Open: %v", err)
		}

		tw, err := os.Create(target)
		if err != nil {
			if err != nil {
				return fmt.Errorf("Open: %v", err)
			}
		}

		scanner := bufio.NewScanner(sr)
		for scanner.Scan() {
			line = scanner.Bytes()
			idx := bytes.Index(line, badChars)
			if idx > -1 && line[len(line)-1] == '"' {
				// We still want the "=" sign
				line = append(line[:idx+1], line[idx+2:len(line)-1]...)
				line = bytes.Replace(line, escapedQuotes, regularQuotes, -1)
			}
			tw.Write(line)
			tw.WriteString("\n")
		}
		sr.Close()
		tw.Close()

		// Modification time of files from Crowdin often ahead of current,
		// so we need to set back to current.
		os.Chtimes(target, now, now)
	}

	fmt.Println("Locale files has been successfully imported!")
	return nil
}
