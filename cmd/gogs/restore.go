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

// lastSupportedVersionOfFormat maps each retired backup archive format version
// to the last Gogs version that is able to import it.
var lastSupportedVersionOfFormat = map[int]string{}

func runRestore(ctx context.Context, cmd *cli.Command) error {
	logger := conf.NewConsoleLogger(os.Stdout)
	zip.Verbose = cmd.Bool("verbose")

	tmpDir := cmd.String("tempdir")
	if !osx.IsDir(tmpDir) {
		logger.FatalContext(ctx, "Temporary directory does not exist", "tempDir", tmpDir)
	}
	// The archive is extracted into a new directory that only the current user can open, because it holds the
	// configuration and the database dump, and its custom files are moved into the custom directory. Another user
	// could otherwise read them, or replace them by creating the directory first.
	extractDir, err := os.MkdirTemp(tmpDir, "gogs-restore-")
	if err != nil {
		logger.FatalContext(ctx, "Failed to create extract directory", "tempDir", tmpDir, "error", err)
	}
	defer func() { _ = os.RemoveAll(extractDir) }()
	// fatal exits without running deferred calls, so it removes the extract directory itself.
	fatal := func(msg string, args ...any) {
		_ = os.RemoveAll(extractDir)
		logger.FatalContext(ctx, msg, args...)
	}
	archivePath := filepath.Join(extractDir, archiveRootDir)

	logger.Info("Restoring backup", "archivePath", cmd.String("from"))
	err = zip.ExtractTo(cmd.String("from"), extractDir)
	if err != nil {
		fatal("Failed to extract backup archive", "error", err)
	}

	// Check backup version
	metaFile := filepath.Join(archivePath, "metadata.ini")
	if !osx.IsFile(metaFile) {
		fatal("File 'metadata.ini' is missing")
	}
	metadata, err := ini.Load(metaFile)
	if err != nil {
		fatal("Failed to load metadata", "metadataPath", metaFile, "error", err)
	}
	backupVersion := metadata.Section("").Key("GOGS_VERSION").MustString("999.0")
	if semverx.Compare(conf.App.Version, "<", backupVersion) {
		fatal("Current Gogs version is lower than backup version", "currentVersion", conf.App.Version, "backupVersion", backupVersion)
	}
	formatVersion := metadata.Section("").Key("VERSION").MustInt()
	if formatVersion == 0 {
		fatal("Backup format version is missing from metadata", "metadataPath", metaFile, "missingKey", "VERSION")
	}
	if formatVersion != currentBackupFormatVersion {
		fatal("Unsupported backup format version", "backupFormatVersion", formatVersion, "supportedFormatVersion", currentBackupFormatVersion, "lastSupportedGogsVersion", lastSupportedVersionOfFormat[formatVersion])
	}

	// If config file is not present in backup, user must set this file via flag.
	// Otherwise, it's optional to set config file flag.
	configFile := filepath.Join(archivePath, "custom", "conf", "app.ini")
	var customConf string
	if lineageConf := configFromLineage(cmd); lineageConf != "" {
		customConf = lineageConf
	} else if !osx.IsFile(configFile) {
		fatal("Custom config file is not found in backup and '--config' is not specified")
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

	conn, err := database.SetEngine(logger.Scoped("db"), false)
	if err != nil {
		return errors.Wrap(err, "set engine")
	}

	// Files are restored before the database because the current ones are kept as "<path>.bak" and can be moved
	// back by hand, whereas a failed database import cannot be undone.
	if !cmd.Bool("database-only") {
		if err = checkLeftoverBackups(archivePath); err != nil {
			fatal("Failed to restore files", "error", err)
		}

		// Custom files
		if srcDir := filepath.Join(archivePath, "custom"); osx.IsDir(srcDir) {
			if err = restoreCustomDir(srcDir, conf.CustomDir()); err != nil {
				fatal("Failed to import 'custom'", "error", err)
			}
		}

		// Data files
		_ = os.MkdirAll(conf.Server.AppDataPath, os.ModePerm)
		for _, dir := range backupDataDirs {
			// Skip if the backup archive does not have the corresponding data.
			srcPath := filepath.Join(archivePath, "data", dir)
			if !osx.IsDir(srcPath) {
				continue
			}

			if err = replacePath(srcPath, filepath.Join(conf.Server.AppDataPath, dir)); err != nil {
				fatal("Failed to import 'data'", "error", err)
			}
		}
	}

	// Database
	dbDir := path.Join(archivePath, "db")
	if err = database.ImportDatabase(ctx, logger.Scoped("db"), conn, dbDir, cmd.Bool("verbose")); err != nil {
		fatal("Failed to import database", "error", err)
	}

	// Repositories
	reposPath := filepath.Join(archivePath, "repositories.zip")
	if !cmd.Bool("exclude-repos") && !cmd.Bool("database-only") && osx.IsFile(reposPath) {
		if err := zip.ExtractTo(reposPath, filepath.Dir(conf.Repository.Root)); err != nil {
			fatal("Failed to extract 'repositories.zip'", "error", err)
		}
	}

	logger.Info("Restore succeeded")

	return nil
}

// replacePath moves srcPath to dstPath, keeping the current dstPath as "<dstPath>.bak". When the move fails, e.g.,
// because the temporary directory is on another file system, the current dstPath is moved back so it is not lost.
func replacePath(srcPath, dstPath string) error {
	if !osx.Exist(dstPath) {
		return os.Rename(srcPath, dstPath)
	}
	if err := moveAside(dstPath); err != nil {
		return errors.Wrap(err, "back up current")
	}
	if err := os.Rename(srcPath, dstPath); err != nil {
		if rollbackErr := os.Rename(dstPath+".bak", dstPath); rollbackErr != nil {
			return errors.Wrapf(err, "move, and the current one is left at '%s' because moving it back failed: %v", dstPath+".bak", rollbackErr)
		}
		return errors.Wrap(err, "move")
	}
	return nil
}

// moveAside renames the path to "<path>.bak" to make room for the restored copy.
func moveAside(path string) error {
	// Renaming onto a leftover from an earlier restore fails with an unhelpful error, e.g., "Access is denied" on
	// Windows.
	if err := leftoverBackupError(path); err != nil {
		return err
	}
	return os.Rename(path, path+".bak")
}

// checkLeftoverBackups returns an error when a path that the restore moves aside already has a "<path>.bak", so
// the restore stops before it replaces any file rather than partway through.
func checkLeftoverBackups(archivePath string) error {
	var paths []string
	// The archive may have no custom files, in which case none are replaced.
	entries, _ := os.ReadDir(filepath.Join(archivePath, "custom"))
	for _, e := range entries {
		if !skipCustomEntry(conf.CustomDir(), e.Name()) {
			paths = append(paths, filepath.Join(conf.CustomDir(), e.Name()))
		}
	}
	for _, dir := range backupDataDirs {
		if osx.IsDir(filepath.Join(archivePath, "data", dir)) {
			paths = append(paths, filepath.Join(conf.Server.AppDataPath, dir))
		}
	}

	for _, path := range paths {
		if osx.Exist(path) {
			if err := leftoverBackupError(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// leftoverBackupError returns an error when "<path>.bak" already exists.
func leftoverBackupError(path string) error {
	if !osx.Exist(path + ".bak") {
		return nil
	}
	// The path is quoted with '%s' rather than %q because %q escapes every backslash in a Windows path, which makes
	// the path harder to read and copy from the message.
	return errors.Newf("'%s' already exists, probably from an earlier restore. Move or remove it, then run the restore again", path+".bak")
}

// skipCustomEntry reports whether the restore leaves the entry of the custom directory as it is:
//   - "data" is never touched, the same as in backups, because it is the app data directory in the Docker setup,
//     which holds the open SQLite database.
//   - An app data directory, which is also in the custom directory when APP_DATA_PATH points there, is restored
//     from the archive's "data".
func skipCustomEntry(customDir, name string) bool {
	if name == "data" {
		return true
	}
	path := filepath.Join(customDir, name)
	absPath, _ := filepath.Abs(path)
	info, statErr := os.Stat(path)
	for _, dir := range backupDataDirs {
		dataPath := filepath.Join(conf.Server.AppDataPath, dir)
		if absDataPath, _ := filepath.Abs(dataPath); absPath == absDataPath {
			return true
		}
		// The paths may differ but lead to the same directory, e.g., through a symlink or in another letter case.
		if dataInfo, err := os.Stat(dataPath); statErr == nil && err == nil && os.SameFile(info, dataInfo) {
			return true
		}
	}
	return false
}

// restoreCustomDir moves each entry of srcDir into customDir, keeping the current one as "<path>.bak". Entries
// that are not in srcDir stay in place, and so do the ones skipCustomEntry skips.
func restoreCustomDir(srcDir, customDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return errors.Wrap(err, "list entries")
	}
	if err = os.MkdirAll(customDir, os.ModePerm); err != nil {
		return errors.Wrap(err, "create custom directory")
	}

	for _, e := range entries {
		if skipCustomEntry(customDir, e.Name()) {
			continue
		}

		if err = replacePath(filepath.Join(srcDir, e.Name()), filepath.Join(customDir, e.Name())); err != nil {
			return errors.Wrapf(err, "replace '%s'", e.Name())
		}
	}
	return nil
}
