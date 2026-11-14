package main

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/cae/zip"
	"github.com/urfave/cli/v3"
	"unknwon.dev/ini/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/semverx"
)

var restoreCommand = cli.Command{
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

func runRestore(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger()
	zip.Verbose = cmd.Bool("verbose")

	tmpDir := cmd.String("tempdir")
	if !osx.IsDir(tmpDir) {
		logger.FatalContext(ctx, "Temporary directory does not exist", "tempDir", tmpDir)
	}
	archivePath := path.Join(tmpDir, archiveRootDir)

	// Make sure there was no leftover and also clean up afterwards
	err := os.RemoveAll(archivePath)
	if err != nil {
		logger.FatalContext(ctx, "Failed to clean up previous leftover", "archiveDir", archivePath, "error", err)
	}
	defer func() { _ = os.RemoveAll(archivePath) }()

	logger.Info("Restoring backup", "archivePath", cmd.String("from"))
	err = zip.ExtractTo(cmd.String("from"), tmpDir)
	if err != nil {
		logger.FatalContext(ctx, "Failed to extract backup archive", "error", err)
	}

	// Check backup version
	metaFile := filepath.Join(archivePath, "metadata.ini")
	if !osx.IsFile(metaFile) {
		logger.FatalContext(ctx, "File 'metadata.ini' is missing")
	}
	metadata, err := ini.Load(metaFile)
	if err != nil {
		logger.FatalContext(ctx, "Failed to load metadata", "metadataPath", metaFile, "error", err)
	}
	backupVersion := metadata.Section("").Key("GOGS_VERSION").MustString("999.0")
	if semverx.Compare(conf.App.Version, "<", backupVersion) {
		logger.FatalContext(ctx, "Current Gogs version is lower than backup version", "currentVersion", conf.App.Version, "backupVersion", backupVersion)
	}
	formatVersion := metadata.Section("").Key("VERSION").MustInt()
	if formatVersion == 0 {
		logger.FatalContext(ctx, "Backup format version is missing from metadata", "metadataPath", metaFile, "missingKey", "VERSION")
	}
	if formatVersion != currentBackupFormatVersion {
		logger.FatalContext(ctx, "Unsupported backup format version", "backupFormatVersion", formatVersion, "supportedFormatVersion", currentBackupFormatVersion, "lastSupportedGogsVersion", lastSupportedVersionOfFormat[formatVersion])
	}

	// If config file is not present in backup, user must set this file via flag.
	// Otherwise, it's optional to set config file flag.
	configFile := filepath.Join(archivePath, "custom", "conf", "app.ini")
	var customConf string
	if lineageConf := configFromLineage(cmd); lineageConf != "" {
		customConf = lineageConf
	} else if !osx.IsFile(configFile) {
		logger.FatalContext(ctx, "Custom config file is not found in backup and '--config' is not specified")
	} else {
		customConf = configFile
	}

	err = conf.Init(logger, customConf)
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

	// Files are restored before the database because the current ones are kept as "<path>.bak" and can be moved
	// back by hand, whereas a failed database import cannot be undone.
	if !cmd.Bool("database-only") {
		// Custom files
		if osx.IsDir(conf.CustomDir()) {
			if err = moveAside(conf.CustomDir()); err != nil {
				logger.FatalContext(ctx, "Failed to backup current 'custom'", "error", err)
			}
		}
		if err = os.Rename(filepath.Join(archivePath, "custom"), conf.CustomDir()); err != nil {
			logger.FatalContext(ctx, "Failed to import 'custom'", "error", err)
		}

		// Data files
		_ = os.MkdirAll(conf.Server.AppDataPath, os.ModePerm)
		for _, dir := range backupDataDirs {
			// Skip if backup archive does not have corresponding data
			srcPath := filepath.Join(archivePath, "data", dir)
			if !osx.IsDir(srcPath) {
				continue
			}

			dirPath := filepath.Join(conf.Server.AppDataPath, dir)
			if osx.IsDir(dirPath) {
				if err = moveAside(dirPath); err != nil {
					logger.FatalContext(ctx, "Failed to backup current 'data'", "error", err)
				}
			}
			if err = os.Rename(srcPath, dirPath); err != nil {
				logger.FatalContext(ctx, "Failed to import 'data'", "error", err)
			}
		}
	}

	// Database
	dbDir := path.Join(archivePath, "db")
	if err = database.ImportDatabase(ctx, logger, conn, dbDir, cmd.Bool("verbose")); err != nil {
		logger.FatalContext(ctx, "Failed to import database", "error", err)
	}

	// Repositories
	reposPath := filepath.Join(archivePath, "repositories.zip")
	if !cmd.Bool("exclude-repos") && !cmd.Bool("database-only") && osx.IsFile(reposPath) {
		if err := zip.ExtractTo(reposPath, filepath.Dir(conf.Repository.Root)); err != nil {
			logger.FatalContext(ctx, "Failed to extract 'repositories.zip'", "error", err)
		}
	}

	logger.Info("Restore succeeded")

	return nil
}

// moveAside renames the path to "<path>.bak" to make room for the restored copy.
func moveAside(path string) error {
	// Renaming onto a leftover from an earlier restore fails with an unhelpful error, e.g., "Access is denied" on
	// Windows. The path is quoted with '%s' rather than %q because %q escapes every backslash in a Windows path,
	// which makes the path harder to read and copy from the message.
	if osx.Exist(path + ".bak") {
		return errors.Newf("'%s' already exists, probably from an earlier restore. Move or remove it, then run the restore again", path+".bak")
	}
	return os.Rename(path, path+".bak")
}
