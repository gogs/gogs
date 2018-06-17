// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"
	"path"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"
	"github.com/mcuadros/go-version"
	"github.com/urfave/cli"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/setting"
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
		stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
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
	if !com.IsExist(tmpDir) {
		log.Fatal(0, "'--tempdir' does not exist: %s", tmpDir)
	}

	log.Info("Restore backup from: %s", c.String("from"))
	if err := zip.ExtractTo(c.String("from"), tmpDir); err != nil {
		log.Fatal(0, "Failed to extract backup archive: %v", err)
	}
	archivePath := path.Join(tmpDir, _ARCHIVE_ROOT_DIR)
	defer os.RemoveAll(archivePath)

	// Check backup version
	metaFile := path.Join(archivePath, "metadata.ini")
	if !com.IsExist(metaFile) {
		log.Fatal(0, "File 'metadata.ini' is missing")
	}
	metadata, err := ini.Load(metaFile)
	if err != nil {
		log.Fatal(0, "Failed to load metadata '%s': %v", metaFile, err)
	}
	backupVersion := metadata.Section("").Key("GOGS_VERSION").MustString("999.0")
	if version.Compare(setting.AppVer, backupVersion, "<") {
		log.Fatal(0, "Current Gogs version is lower than backup version: %s < %s", setting.AppVer, backupVersion)
	}
	formatVersion := metadata.Section("").Key("VERSION").MustInt()
	if formatVersion == 0 {
		log.Fatal(0, "Failed to determine the backup format version from metadata '%s': %s", metaFile, "VERSION is not presented")
	}
	if formatVersion != _CURRENT_BACKUP_FORMAT_VERSION {
		log.Fatal(0, "Backup format version found is %d but this binary only supports %d\nThe last known version that is able to import your backup is %s",
			formatVersion, _CURRENT_BACKUP_FORMAT_VERSION, lastSupportedVersionOfFormat[formatVersion])
	}

	// If config file is not present in backup, user must set this file via flag.
	// Otherwise, it's optional to set config file flag.
	configFile := path.Join(archivePath, "custom/conf/app.ini")
	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	} else if !com.IsExist(configFile) {
		log.Fatal(0, "'--config' is not specified and custom config file is not found in backup")
	} else {
		setting.CustomConf = configFile
	}
	setting.NewContext()
	models.LoadConfigs()
	models.SetEngine()

	// Database
	dbDir := path.Join(archivePath, "db")
	if err = models.ImportDatabase(dbDir, c.Bool("verbose")); err != nil {
		log.Fatal(0, "Failed to import database: %v", err)
	}

	// Custom files
	if !c.Bool("database-only") {
		if com.IsExist(setting.CustomPath) {
			if err = os.Rename(setting.CustomPath, setting.CustomPath+".bak"); err != nil {
				log.Fatal(0, "Failed to backup current 'custom': %v", err)
			}
		}
		if err = os.Rename(path.Join(archivePath, "custom"), setting.CustomPath); err != nil {
			log.Fatal(0, "Failed to import 'custom': %v", err)
		}
	}

	// Data files
	if !c.Bool("database-only") {
		os.MkdirAll(setting.AppDataPath, os.ModePerm)
		for _, dir := range []string{"attachments", "avatars", "repo-avatars"} {
			// Skip if backup archive does not have corresponding data
			srcPath := path.Join(archivePath, "data", dir)
			if !com.IsDir(srcPath) {
				continue
			}

			dirPath := path.Join(setting.AppDataPath, dir)
			if com.IsExist(dirPath) {
				if err = os.Rename(dirPath, dirPath+".bak"); err != nil {
					log.Fatal(0, "Failed to backup current 'data': %v", err)
				}
			}
			if err = os.Rename(srcPath, dirPath); err != nil {
				log.Fatal(0, "Failed to import 'data': %v", err)
			}
		}
	}

	// Repositories
	reposPath := path.Join(archivePath, "repositories.zip")
	if !c.Bool("exclude-repos") && !c.Bool("database-only") && com.IsExist(reposPath) {
		if err := zip.ExtractTo(reposPath, path.Dir(setting.RepoRootPath)); err != nil {
			log.Fatal(0, "Failed to extract 'repositories.zip': %v", err)
		}
	}

	log.Info("Restore succeed!")
	log.Shutdown()
	return nil
}
