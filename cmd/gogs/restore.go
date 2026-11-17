package main

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/cae/zip"
	"github.com/urfave/cli/v3"
	"unknwon.dev/ini/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/iox"
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
	logger := conf.NewConsoleLogger(os.Stdout)
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

	conn, err := database.SetEngine(logger, false)
	if err != nil {
		return errors.Wrap(err, "set engine")
	}

	// Files are restored before the database because the current ones are kept as "<path>.bak" and can be moved
	// back by hand, whereas a failed database import cannot be undone.
	if !cmd.Bool("database-only") {
		// Custom files. Hold <custom>/data first: the archive omits it, and the
		// database file already opened by SetEngine has to stay the same inode.
		var held string
		if osx.IsDir(conf.CustomDir()) {
			held, err = holdCustomData(conf.CustomDir())
			if err != nil {
				logger.FatalContext(ctx, "Failed to preserve current 'custom/data'", "error", err)
			}
			if err = moveAside(conf.CustomDir()); err != nil {
				if held != "" {
					if rbErr := os.Rename(held, filepath.Join(conf.CustomDir(), "data")); rbErr != nil {
						logger.FatalContext(ctx, "Failed to backup current 'custom'", "error", err, "restoreHeldError", rbErr)
					}
				}
				logger.FatalContext(ctx, "Failed to backup current 'custom'", "error", err)
			}
		}
		if err = moveTree(filepath.Join(archivePath, "custom"), conf.CustomDir()); err != nil {
			// FatalContext exits the process, so this rollback cannot be deferred.
			if rbErr := rollbackCustomInstall(conf.CustomDir(), held); rbErr != nil {
				logger.FatalContext(ctx, "Failed to import 'custom'", "error", err, "rollbackError", rbErr)
			}
			logger.FatalContext(ctx, "Failed to import 'custom'", "error", err)
		}
		if err = restoreHeldCustomData(conf.CustomDir(), held); err != nil {
			logger.FatalContext(ctx, "Failed to restore 'custom/data'", "error", err)
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
			if err = moveTree(srcPath, dirPath); err != nil {
				// FatalContext exits the process, so this rollback cannot be deferred.
				if rbErr := rollbackDataInstall(dirPath); rbErr != nil {
					logger.FatalContext(ctx, "Failed to import 'data'", "error", err, "rollbackError", rbErr)
				}
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

// holdCustomData renames <custom>/data to the sibling <custom>.restore-data when
// that directory exists. The rename stays on the custom directory's filesystem,
// so the database file keeps the inode SetEngine already opened. The returned
// path is empty when there is nothing to hold.
func holdCustomData(customDir string) (string, error) {
	dataDir := filepath.Join(customDir, "data")
	if !osx.IsDir(dataDir) {
		return "", nil
	}
	held := customDir + ".restore-data"
	if osx.Exist(held) {
		return "", errors.Newf("'%s' already exists, probably from an earlier restore. Move or remove it, then run the restore again", held)
	}
	if err := os.Rename(dataDir, held); err != nil {
		return "", errors.Wrap(err, "hold custom data")
	}
	return held, nil
}

// restoreHeldCustomData moves a directory held by holdCustomData back to <custom>/data.
func restoreHeldCustomData(customDir, held string) error {
	if held == "" {
		return nil
	}
	dst := filepath.Join(customDir, "data")
	if osx.Exist(dst) {
		return errors.Newf("Restored custom directory already contains 'data', so held data remains at '%s'.", held)
	}
	return errors.Wrap(os.Rename(held, dst), "restore held custom data")
}

// rollbackCustomInstall puts the previous custom directory back after a failed
// install. A partial destination is removed only when <custom>.bak exists.
func rollbackCustomInstall(customDir, held string) error {
	bak := customDir + ".bak"
	if osx.Exist(bak) {
		if err := os.RemoveAll(customDir); err != nil {
			return errors.Wrap(err, "remove partial custom directory")
		}
		if err := os.Rename(bak, customDir); err != nil {
			return errors.Wrap(err, "restore previous custom directory")
		}
	}
	if held == "" {
		return nil
	}
	return errors.Wrap(os.Rename(held, filepath.Join(customDir, "data")), "restore held custom data")
}

// rollbackDataInstall puts a data directory moved by moveAside back after a
// failed install. A partial destination is removed only when the .bak tree exists.
func rollbackDataInstall(dirPath string) error {
	bak := dirPath + ".bak"
	if !osx.Exist(bak) {
		return nil
	}
	if err := os.RemoveAll(dirPath); err != nil {
		return errors.Wrapf(err, "remove partial data directory '%s', previous copy remains at '%s'", dirPath, bak)
	}
	return errors.Wrapf(os.Rename(bak, dirPath), "restore previous data directory '%s'", bak)
}

// moveTree moves src to dst. A cross-device rename is copied, then the source is removed.
// A failed copy removes the partial destination.
func moveTree(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if !isCrossDevice(err) {
		return errors.Wrap(err, "rename")
	}
	if err := copyTree(src, dst); err != nil {
		return removePartialDestination(dst, err)
	}
	return errors.Wrap(os.RemoveAll(src), "remove source after copy")
}

// removePartialDestination deletes a tree left behind by a failed copy.
// When the delete fails, the error names dst and keeps both failures.
func removePartialDestination(dst string, copyErr error) error {
	if rmErr := os.RemoveAll(dst); rmErr != nil {
		return errors.Wrapf(copyErr, "partial destination '%s' remains: %v", dst, rmErr)
	}
	return copyErr
}

// isCrossDevice reports whether err is a cross-device link error, including one
// wrapped by *os.LinkError.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// copyTree reproduces the directory src at dst. Regular files are copied with iox.CopyFile.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return errors.Wrap(err, "stat source")
	}
	if !info.IsDir() {
		return errors.Newf("cannot copy %s: not a directory", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return errors.Wrap(err, "create destination")
	}

	err = filepath.WalkDir(src, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, srcPath)
		if err != nil {
			return errors.Wrap(err, "relative path")
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			dirInfo, err := d.Info()
			if err != nil {
				return errors.Wrap(err, "stat directory")
			}
			return errors.Wrap(os.MkdirAll(target, dirInfo.Mode().Perm()), "create directory")
		}
		if !d.Type().IsRegular() {
			return errors.Newf("cannot copy %s: not a regular file", srcPath)
		}
		return errors.Wrapf(iox.CopyFile(srcPath, target), "copy %s", srcPath)
	})
	return errors.Wrap(err, "copy tree")
}
