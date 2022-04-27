// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/unknwon/cae/zip"
	"github.com/urfave/cli"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/semverutil"
)

var Restore = cli.Command{
	Name:  "restore",
	Usage: "Restore files and database from backup",
	Description: `Restore imports all related files and database from a backup archive.
The backup version must lower or equal to current Gogs version. You can also import
backup from other database engines, which is useful for database migrating.

If corresponding files or database tables are not presented in the archive, they will
be skipped and remain unchanged.`,
	Action: runRestore,
	Flags: []cli.Flag{
		stringFlag("config, c", "", "Custom configuration file path"),
		boolFlag("verbose, v", "Show process details"),
		stringFlag("tempdir, t", os.TempDir(), "Temporary directory path"),
		stringFlag("from", "", "Path to backup archive"),
		boolFlag("database-only", "Only import database"),
		boolFlag("exclude-repos", "Exclude repositories"),
	},
}

// lastSupportedVersionOfFormat returns the last supported version of the backup archive
// format that is able to import.
var lastSupportedVersionOfFormat = map[int]string{}

func runRestore(c *cli.Context) error {
	zip.Verbose = c.Bool("verbose")

	tmpDir := c.String("tempdir")
	if !osutil.IsDir(tmpDir) {
		log.Fatal("'--tempdir' does not exist: %s", tmpDir)
	}
	archivePath := path.Join(tmpDir, archiveRootDir)

	// Make sure there was no leftover and also clean up afterwards
	err := os.RemoveAll(archivePath)
	if err != nil {
		log.Fatal("Failed to clean up previous leftover in %q: %v", archivePath, err)
	}
	defer func() { _ = os.RemoveAll(archivePath) }()

	log.Info("Restoring backup from: %s", c.String("from"))
	err = zip.ExtractTo(c.String("from"), tmpDir)
	if err != nil {
		log.Fatal("Failed to extract backup archive: %v", err)
	}

	// Check backup version
	metaFile := filepath.Join(archivePath, "metadata.ini")
	if !osutil.IsFile(metaFile) {
		log.Fatal("File 'metadata.ini' is missing")
	}
	metadata, err := ini.Load(metaFile)
	if err != nil {
		log.Fatal("Failed to load metadata '%s': %v", metaFile, err)
	}
	backupVersion := metadata.Section("").Key("GOGS_VERSION").MustString("999.0")
	if semverutil.Compare(conf.App.Version, "<", backupVersion) {
		log.Fatal("Current Gogs version is lower than backup version: %s < %s", conf.App.Version, backupVersion)
	}
	formatVersion := metadata.Section("").Key("VERSION").MustInt()
	if formatVersion == 0 {
		log.Fatal("Failed to determine the backup format version from metadata '%s': %s", metaFile, "VERSION is not presented")
	}
	if formatVersion != currentBackupFormatVersion {
		log.Fatal("Backup format version found is %d but this binary only supports %d\nThe last known version that is able to import your backup is %s",
			formatVersion, currentBackupFormatVersion, lastSupportedVersionOfFormat[formatVersion])
	}

	// If config file is not present in backup, user must set this file via flag.
	// Otherwise, it's optional to set config file flag.
	configFile := filepath.Join(archivePath, "custom", "conf", "app.ini")
	var customConf string
	if c.IsSet("config") {
		customConf = c.String("config")
	} else if !osutil.IsFile(configFile) {
		log.Fatal("'--config' is not specified and custom config file is not found in backup")
	} else {
		customConf = configFile
	}

	err = conf.Init(customConf)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}
	conf.InitLogging(true)

	conn, err := db.SetEngine()
	if err != nil {
		return errors.Wrap(err, "set engine")
	}

	// Database
	dbDir := path.Join(archivePath, "db")
	if err = db.ImportDatabase(conn, dbDir, c.Bool("verbose")); err != nil {
		log.Fatal("Failed to import database: %v", err)
	}

	// Custom files
	if !c.Bool("database-only") {
		if osutil.IsDir(conf.CustomDir()) {
			if err = os.Rename(conf.CustomDir(), conf.CustomDir()+".bak"); err != nil {
				log.Fatal("Failed to backup current 'custom': %v", err)
			}
		}
		if err = os.Rename(filepath.Join(archivePath, "custom"), conf.CustomDir()); err != nil {
			log.Fatal("Failed to import 'custom': %v", err)
		}
	}

	// Data files
	if !c.Bool("database-only") {
		_ = os.MkdirAll(conf.Server.AppDataPath, os.ModePerm)
		for _, dir := range []string{"attachments", "avatars", "repo-avatars"} {
			// Skip if backup archive does not have corresponding data
			srcPath := filepath.Join(archivePath, "data", dir)
			if !osutil.IsDir(srcPath) {
				continue
			}

			dirPath := filepath.Join(conf.Server.AppDataPath, dir)
			if osutil.IsDir(dirPath) {
				if err = os.Rename(dirPath, dirPath+".bak"); err != nil {
					log.Fatal("Failed to backup current 'data': %v", err)
				}
			}
			if err = os.Rename(srcPath, dirPath); err != nil {
				log.Fatal("Failed to import 'data': %v", err)
			}
		}
	}

	// Repositories
	reposPath := filepath.Join(archivePath, "repositories.zip")
	if !c.Bool("exclude-repos") && !c.Bool("database-only") && osutil.IsFile(reposPath) {
		if err := zip.ExtractTo(reposPath, filepath.Dir(conf.Repository.Root)); err != nil {
			log.Fatal("Failed to extract 'repositories.zip': %v", err)
		}
	}

	log.Info("Restore succeed!")
	log.Stop()
	return nil
}
