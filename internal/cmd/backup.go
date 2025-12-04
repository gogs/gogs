// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/unknwon/cae/zip"
	"github.com/urfave/cli"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/osutil"
)

var Backup = cli.Command{
	Name:  "backup",
	Usage: "Backup files and database",
	Description: `Backup dumps and compresses all related files and database into zip file,
which can be used for migrating Gogs to another server. The output format is meant to be
portable among all supported database engines.`,
	Action: runBackup,
	Flags: []cli.Flag{
		stringFlag("config, c", "", "Custom configuration file path"),
		boolFlag("verbose, v", "Show process details"),
		stringFlag("tempdir, t", os.TempDir(), "Temporary directory path"),
		stringFlag("target", "./", "Target directory path to save backup archive"),
		stringFlag("archive-name", fmt.Sprintf("gogs-backup-%s.zip", time.Now().Format("20060102150405")), "Name of backup archive"),
		boolFlag("database-only", "Only dump database"),
		boolFlag("exclude-mirror-repos", "Exclude mirror repositories"),
		boolFlag("exclude-repos", "Exclude repositories"),
	},
}

const (
	currentBackupFormatVersion = 1
	archiveRootDir             = "gogs-backup"
)

func runBackup(c *cli.Context) error {
	zip.Verbose = c.Bool("verbose")

	err := conf.Init(c.String("config"))
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}
	conf.InitLogging(true)

	conn, err := database.SetEngine()
	if err != nil {
		return errors.Wrap(err, "set engine")
	}

	tmpDir := c.String("tempdir")
	if !osutil.IsExist(tmpDir) {
		log.Fatal("'--tempdir' does not exist: %s", tmpDir)
	}
	rootDir, err := os.MkdirTemp(tmpDir, "gogs-backup-")
	if err != nil {
		log.Fatal("Failed to create backup root directory '%s': %v", rootDir, err)
	}
	log.Info("Backup root directory: %s", rootDir)

	// Metadata
	metaFile := path.Join(rootDir, "metadata.ini")
	metadata := ini.Empty()
	metadata.Section("").Key("VERSION").SetValue(strconv.Itoa(currentBackupFormatVersion))
	metadata.Section("").Key("DATE_TIME").SetValue(time.Now().String())
	metadata.Section("").Key("GOGS_VERSION").SetValue(conf.App.Version)
	if err = metadata.SaveTo(metaFile); err != nil {
		log.Fatal("Failed to save metadata '%s': %v", metaFile, err)
	}

	archiveName := filepath.Join(c.String("target"), c.String("archive-name"))
	log.Info("Packing backup files to: %s", archiveName)

	z, err := zip.Create(archiveName)
	if err != nil {
		log.Fatal("Failed to create backup archive '%s': %v", archiveName, err)
	}
	if err = z.AddFile(archiveRootDir+"/metadata.ini", metaFile); err != nil {
		log.Fatal("Failed to include 'metadata.ini': %v", err)
	}

	// Database
	dbDir := filepath.Join(rootDir, "db")
	if err = database.DumpDatabase(context.Background(), conn, dbDir, c.Bool("verbose")); err != nil {
		log.Fatal("Failed to dump database: %v", err)
	}
	if err = z.AddDir(archiveRootDir+"/db", dbDir); err != nil {
		log.Fatal("Failed to include 'db': %v", err)
	}

	if !c.Bool("database-only") {
		// Custom files
		err = addCustomDirToBackup(z)
		if err != nil {
			log.Fatal("Failed to add custom directory to backup: %v", err)
		}

		// Data files
		for _, dir := range []string{"ssh", "attachments", "avatars", "repo-avatars"} {
			dirPath := filepath.Join(conf.Server.AppDataPath, dir)
			if !osutil.IsDir(dirPath) {
				continue
			}

			if err = z.AddDir(path.Join(archiveRootDir+"/data", dir), dirPath); err != nil {
				log.Fatal("Failed to include 'data': %v", err)
			}
		}
	}

	// Repositories
	if !c.Bool("exclude-repos") && !c.Bool("database-only") {
		reposDump := filepath.Join(rootDir, "repositories.zip")
		log.Info("Dumping repositories in %q", conf.Repository.Root)
		if c.Bool("exclude-mirror-repos") {
			repos, err := database.GetNonMirrorRepositories()
			if err != nil {
				log.Fatal("Failed to get non-mirror repositories: %v", err)
			}
			reposZip, err := zip.Create(reposDump)
			if err != nil {
				log.Fatal("Failed to create %q: %v", reposDump, err)
			}
			baseDir := filepath.Base(conf.Repository.Root)
			for _, r := range repos {
				name := r.FullName() + ".git"
				if err := reposZip.AddDir(filepath.Join(baseDir, name), filepath.Join(conf.Repository.Root, name)); err != nil {
					log.Fatal("Failed to add %q: %v", name, err)
				}
			}
			if err = reposZip.Close(); err != nil {
				log.Fatal("Failed to save %q: %v", reposDump, err)
			}
		} else {
			if err = zip.PackTo(conf.Repository.Root, reposDump, true); err != nil {
				log.Fatal("Failed to dump repositories: %v", err)
			}
		}
		log.Info("Repositories dumped to: %s", reposDump)

		if err = z.AddFile(archiveRootDir+"/repositories.zip", reposDump); err != nil {
			log.Fatal("Failed to include %q: %v", reposDump, err)
		}
	}

	if err = z.Close(); err != nil {
		log.Fatal("Failed to save backup archive '%s': %v", archiveName, err)
	}

	_ = os.RemoveAll(rootDir)
	log.Info("Backup succeed! Archive is located at: %s", archiveName)
	log.Stop()
	return nil
}

func addCustomDirToBackup(z *zip.ZipArchive) error {
	customDir := conf.CustomDir()
	entries, err := os.ReadDir(customDir)
	if err != nil {
		return errors.Wrap(err, "list custom directory entries")
	}

	for _, e := range entries {
		if e.Name() == "data" {
			// Skip the "data" directory because it lives under the "custom" directory in
			// the Docker setup and will be backed up separately.
			log.Trace(`Skipping "data" directory in custom directory`)
			continue
		}

		add := z.AddFile
		if e.IsDir() {
			add = z.AddDir
		}
		err = add(
			fmt.Sprintf("%s/custom/%s", archiveRootDir, e.Name()),
			filepath.Join(customDir, e.Name()),
		)
		if err != nil {
			return errors.Wrapf(err, "add %q", e.Name())
		}
	}
	return nil
}
