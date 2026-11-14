package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/cae/zip"
	"github.com/urfave/cli/v3"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/osx"
)

var backupCommand = cli.Command{
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

// backupDataDirs are the directories under the app data path that are included in a backup and restored from it.
var backupDataDirs = []string{"ssh", "attachments", "avatars", "repo-avatars"}

func runBackup(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger()
	zip.Verbose = cmd.Bool("verbose")

	err := conf.Init(logger, configFromLineage(cmd))
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}
	if _, err = conf.InitLogging(logger, true); err != nil {
		return errors.Wrap(err, "init logging")
	}

	conn, err := database.SetEngine(logger)
	if err != nil {
		return errors.Wrap(err, "set engine")
	}

	tmpDir := cmd.String("tempdir")
	if !osx.Exist(tmpDir) {
		logger.FatalContext(ctx, "Temporary directory does not exist", "tempDir", tmpDir)
	}
	rootDir, err := os.MkdirTemp(tmpDir, "gogs-backup-")
	if err != nil {
		logger.FatalContext(ctx, "Failed to create backup root directory", "rootDir", rootDir, "error", err)
	}
	logger.Info("Created backup root directory", "rootDir", rootDir)

	// Metadata
	metaFile := path.Join(rootDir, "metadata.ini")
	metadata := ini.Empty()
	metadata.Section("").Key("VERSION").SetValue(strconv.Itoa(currentBackupFormatVersion))
	metadata.Section("").Key("DATE_TIME").SetValue(time.Now().String())
	metadata.Section("").Key("GOGS_VERSION").SetValue(conf.App.Version)
	if err = metadata.SaveTo(metaFile); err != nil {
		logger.FatalContext(ctx, "Failed to save metadata", "metadataPath", metaFile, "error", err)
	}

	archiveName := filepath.Join(cmd.String("target"), cmd.String("archive-name"))
	logger.Info("Packing backup files", "archivePath", archiveName)

	z, err := zip.Create(archiveName)
	if err != nil {
		logger.FatalContext(ctx, "Failed to create backup archive", "archivePath", archiveName, "error", err)
	}
	if err = z.AddFile(archiveRootDir+"/metadata.ini", metaFile); err != nil {
		logger.FatalContext(ctx, "Failed to include 'metadata.ini'", "error", err)
	}

	// Database
	dbDir := filepath.Join(rootDir, "db")
	if err = database.DumpDatabase(ctx, logger, conn, dbDir, cmd.Bool("verbose")); err != nil {
		logger.FatalContext(ctx, "Failed to dump database", "error", err)
	}
	if err = z.AddDir(archiveRootDir+"/db", dbDir); err != nil {
		logger.FatalContext(ctx, "Failed to include 'db'", "error", err)
	}

	if !cmd.Bool("database-only") {
		// Custom files
		err = addCustomDirToBackup(logger, z)
		if err != nil {
			logger.FatalContext(ctx, "Failed to add custom directory to backup", "error", err)
		}

		// Data files
		for _, dir := range backupDataDirs {
			dirPath := filepath.Join(conf.Server.AppDataPath, dir)
			if !osx.IsDir(dirPath) {
				continue
			}

			if err = z.AddDir(path.Join(archiveRootDir+"/data", dir), dirPath); err != nil {
				logger.FatalContext(ctx, "Failed to include 'data'", "error", err)
			}
		}
	}

	// Repositories
	if !cmd.Bool("exclude-repos") && !cmd.Bool("database-only") {
		reposDump := filepath.Join(rootDir, "repositories.zip")
		logger.Info("Dumping repositories", "repoRoot", conf.Repository.Root)
		if cmd.Bool("exclude-mirror-repos") {
			repos, err := database.GetNonMirrorRepositories()
			if err != nil {
				logger.FatalContext(ctx, "Failed to get non-mirror repositories", "error", err)
			}
			reposZip, err := zip.Create(reposDump)
			if err != nil {
				logger.FatalContext(ctx, "Failed to create repositories archive", "reposArchivePath", reposDump, "error", err)
			}
			baseDir := filepath.Base(conf.Repository.Root)
			for _, r := range repos {
				name := r.FullName() + ".git"
				if err := reposZip.AddDir(filepath.Join(baseDir, name), filepath.Join(conf.Repository.Root, name)); err != nil {
					logger.FatalContext(ctx, "Failed to add repository to archive", "repoPath", name, "error", err)
				}
			}
			if err = reposZip.Close(); err != nil {
				logger.FatalContext(ctx, "Failed to save repositories archive", "reposArchivePath", reposDump, "error", err)
			}
		} else {
			if err = zip.PackTo(conf.Repository.Root, reposDump, true); err != nil {
				logger.FatalContext(ctx, "Failed to dump repositories", "error", err)
			}
		}
		logger.Info("Repositories dumped", "reposArchivePath", reposDump)

		if err = z.AddFile(archiveRootDir+"/repositories.zip", reposDump); err != nil {
			logger.FatalContext(ctx, "Failed to include repositories archive", "reposArchivePath", reposDump, "error", err)
		}
	}

	if err = z.Close(); err != nil {
		logger.FatalContext(ctx, "Failed to save backup archive", "archivePath", archiveName, "error", err)
	}

	_ = os.RemoveAll(rootDir)
	logger.Info("Backup succeeded", "archivePath", archiveName)

	return nil
}

func addCustomDirToBackup(logger *logx.Logger, z *zip.ZipArchive) error {
	customDir := conf.CustomDir()
	entries, err := os.ReadDir(customDir)
	if err != nil {
		return errors.Wrap(err, "list custom directory entries")
	}

	for _, e := range entries {
		if e.Name() == "data" {
			// Skip the "data" directory because it lives under the "custom" directory in
			// the Docker setup and will be backed up separately.
			logger.Debug(`Skipping "data" directory in custom directory`)
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
