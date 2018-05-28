// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"
	"github.com/urfave/cli"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/setting"
)

var Backup = cli.Command{
	Name:  "backup",
	Usage: "Backup files and database",
	Description: `Backup dumps and compresses all related files and database into zip file,
which can be used for migrating Gogs to another server. The output format is meant to be
portable among all supported database engines.`,
	Action: runBackup,
	Flags: []cli.Flag{
		stringFlag("config, c", "custom/conf/app.ini", "Custom configuration file path"),
		boolFlag("verbose, v", "Show process details"),
		stringFlag("tempdir, t", os.TempDir(), "Temporary directory path"),
		stringFlag("target", "./", "Target directory path to save backup archive"),
		stringFlag("archive-name", fmt.Sprintf("gogs-backup-%s.zip", time.Now().Format("20060102150405")), "Name of backup archive"),
		boolFlag("database-only", "Only dump database"),
		boolFlag("exclude-repos", "Exclude repositories"),
	},
}

const _ARCHIVE_ROOT_DIR = "gogs-backup"

func runBackup(c *cli.Context) error {
	zip.Verbose = c.Bool("verbose")
	if c.IsSet("config") {
		setting.CustomConf = c.String("config")
	}
	setting.NewContext()
	models.LoadConfigs()
	models.SetEngine()

	tmpDir := c.String("tempdir")
	if !com.IsExist(tmpDir) {
		log.Fatal(0, "'--tempdir' does not exist: %s", tmpDir)
	}
	rootDir, err := ioutil.TempDir(tmpDir, "gogs-backup-")
	if err != nil {
		log.Fatal(0, "Fail to create backup root directory '%s': %v", rootDir, err)
	}
	log.Info("Backup root directory: %s", rootDir)

	// Metadata
	metaFile := path.Join(rootDir, "metadata.ini")
	metadata := ini.Empty()
	metadata.Section("").Key("VERSION").SetValue("1")
	metadata.Section("").Key("DATE_TIME").SetValue(time.Now().String())
	metadata.Section("").Key("GOGS_VERSION").SetValue(setting.AppVer)
	if err = metadata.SaveTo(metaFile); err != nil {
		log.Fatal(0, "Fail to save metadata '%s': %v", metaFile, err)
	}

	archiveName := path.Join(c.String("target"), c.String("archive-name"))
	log.Info("Packing backup files to: %s", archiveName)

	z, err := zip.Create(archiveName)
	if err != nil {
		log.Fatal(0, "Fail to create backup archive '%s': %v", archiveName, err)
	}
	if err = z.AddFile(_ARCHIVE_ROOT_DIR+"/metadata.ini", metaFile); err != nil {
		log.Fatal(0, "Fail to include 'metadata.ini': %v", err)
	}

	// Database
	dbDir := path.Join(rootDir, "db")
	if err = models.DumpDatabase(dbDir); err != nil {
		log.Fatal(0, "Fail to dump database: %v", err)
	}
	if err = z.AddDir(_ARCHIVE_ROOT_DIR+"/db", dbDir); err != nil {
		log.Fatal(0, "Fail to include 'db': %v", err)
	}

	// Custom files
	if !c.Bool("database-only") {
		if err = z.AddDir(_ARCHIVE_ROOT_DIR+"/custom", setting.CustomPath); err != nil {
			log.Fatal(0, "Fail to include 'custom': %v", err)
		}
	}

	// Data files
	if !c.Bool("database-only") {
		for _, dir := range []string{"attachments", "avatars"} {
			dirPath := path.Join(setting.AppDataPath, dir)
			if !com.IsDir(dirPath) {
				continue
			}

			if err = z.AddDir(path.Join(_ARCHIVE_ROOT_DIR+"/data", dir), dirPath); err != nil {
				log.Fatal(0, "Fail to include 'data': %v", err)
			}
		}
	}

	// Repositories
	if !c.Bool("exclude-repos") && !c.Bool("database-only") {
		reposDump := path.Join(rootDir, "repositories.zip")
		log.Info("Dumping repositories in '%s'", setting.RepoRootPath)
		if err = zip.PackTo(setting.RepoRootPath, reposDump, true); err != nil {
			log.Fatal(0, "Fail to dump repositories: %v", err)
		}
		log.Info("Repositories dumped to: %s", reposDump)

		if err = z.AddFile(_ARCHIVE_ROOT_DIR+"/repositories.zip", reposDump); err != nil {
			log.Fatal(0, "Fail to include 'repositories.zip': %v", err)
		}
	}

	if err = z.Close(); err != nil {
		log.Fatal(0, "Fail to save backup archive '%s': %v", archiveName, err)
	}

	os.RemoveAll(rootDir)
	log.Info("Backup succeed! Archive is located at: %s", archiveName)
	log.Shutdown()
	return nil
}
