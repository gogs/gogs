package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osx"
)

var (
	importCommand = cli.Command{
		Name:  "import",
		Usage: "Import portable data as local Gogs data",
		Description: `Allow user import data from other Gogs installations to local instance
without manually hacking the data files`,
		Commands: []*cli.Command{
			&subcmdImportLocale,
		},
	}

	subcmdImportLocale = cli.Command{
		Name:   "locale",
		Usage:  "Import locale files to local repository",
		Action: runImportLocale,
		Flags: []cli.Flag{
			stringFlag("source", "", "Source directory that stores new locale files"),
			stringFlag("target", "", "Target directory that stores old locale files"),
			stringFlag("config, c", "", "Custom configuration file path"),
		},
	}
)

func runImportLocale(_ context.Context, cmd *cli.Command) error {
	if !cmd.IsSet("source") {
		return errors.New("source directory is not specified")
	} else if !cmd.IsSet("target") {
		return errors.New("target directory is not specified")
	}
	if !osx.IsDir(cmd.String("source")) {
		return errors.Newf("source directory %q does not exist or is not a directory", cmd.String("source"))
	} else if !osx.IsDir(cmd.String("target")) {
		return errors.Newf("target directory %q does not exist or is not a directory", cmd.String("target"))
	}

	err := conf.Init(configFromLineage(cmd))
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}

	now := time.Now()

	var line []byte
	badChars := []byte(`="`)
	escapedQuotes := []byte(`\"`)
	regularQuotes := []byte(`"`)
	// Cut out en-US.
	for _, lang := range conf.I18n.Langs[1:] {
		name := fmt.Sprintf("locale_%s.ini", lang)
		source := filepath.Join(cmd.String("source"), name)
		target := filepath.Join(cmd.String("target"), name)
		if !osx.IsFile(source) {
			continue
		}

		// Crowdin surrounds double quotes for strings contain quotes inside,
		// this breaks INI parser, we need to fix that.
		sr, err := os.Open(source)
		if err != nil {
			return errors.Newf("open: %v", err)
		}

		tw, err := os.Create(target)
		if err != nil {
			return errors.Newf("create: %v", err)
		}

		scanner := bufio.NewScanner(sr)
		for scanner.Scan() {
			line = scanner.Bytes()
			idx := bytes.Index(line, badChars)
			if idx > -1 && line[len(line)-1] == '"' {
				// We still want the "=" sign
				line = append(line[:idx+1], line[idx+2:len(line)-1]...)
				line = bytes.ReplaceAll(line, escapedQuotes, regularQuotes)
			}
			_, _ = tw.Write(line)
			_, _ = tw.WriteString("\n")
		}
		_ = sr.Close()
		_ = tw.Close()

		// Modification time of files from Crowdin often ahead of current,
		// so we need to set back to current.
		_ = os.Chtimes(target, now, now)
	}

	fmt.Println("Locale files has been successfully imported!")
	return nil
}
