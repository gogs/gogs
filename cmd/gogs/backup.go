package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/urfave/cli/v3"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
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
	logger := conf.NewConsoleLogger(os.Stdout)

	err := conf.Init(logger, configFromLineage(cmd))
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

	if err = os.MkdirAll(cmd.String("target"), os.ModePerm); err != nil {
		logger.FatalContext(ctx, "Failed to create target directory", "targetDir", cmd.String("target"), "error", err)
	}
	f, err := os.Create(archiveName)
	if err != nil {
		logger.FatalContext(ctx, "Failed to create backup archive", "archivePath", archiveName, "error", err)
	}
	z := &archiveWriter{Writer: zip.NewWriter(f), logger: logger, verbose: cmd.Bool("verbose")}
	if err = z.add(archiveRootDir+"/metadata.ini", metaFile); err != nil {
		logger.FatalContext(ctx, "Failed to include 'metadata.ini'", "error", err)
	}

	// Database
	dbDir := filepath.Join(rootDir, "db")
	if err = database.DumpDatabase(ctx, logger.Scoped("db"), conn, dbDir, cmd.Bool("verbose")); err != nil {
		logger.FatalContext(ctx, "Failed to dump database", "error", err)
	}
	if err = z.add(archiveRootDir+"/db", dbDir); err != nil {
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

			if err = z.add(path.Join(archiveRootDir+"/data", dir), dirPath); err != nil {
				logger.FatalContext(ctx, "Failed to include 'data'", "error", err)
			}
		}
	}

	// Repositories
	if !cmd.Bool("exclude-repos") && !cmd.Bool("database-only") {
		logger.Info("Dumping repositories", "repoRoot", conf.Repository.Root)
		if err = addRepositoriesToBackup(ctx, z, cmd.Bool("exclude-mirror-repos")); err != nil {
			logger.FatalContext(ctx, "Failed to dump repositories", "error", err)
		}
	}

	if err = z.Close(); err != nil {
		logger.FatalContext(ctx, "Failed to save backup archive", "archivePath", archiveName, "error", err)
	}
	if err = f.Close(); err != nil {
		logger.FatalContext(ctx, "Failed to save backup archive", "archivePath", archiveName, "error", err)
	}

	_ = os.RemoveAll(rootDir)
	logger.Info("Backup succeeded", "archivePath", archiveName)

	return nil
}

func addCustomDirToBackup(logger *logx.Logger, z *archiveWriter) error {
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

		err = z.add(
			fmt.Sprintf("%s/custom/%s", archiveRootDir, e.Name()),
			filepath.Join(customDir, e.Name()),
		)
		if err != nil {
			return errors.Wrapf(err, "add %q", e.Name())
		}
	}
	return nil
}

// addRepositoriesToBackup writes "repositories.zip" into the backup archive as
// it packs the repositories, so it takes no temporary space. Like the
// repository root, its entries start with the name of the root directory.
func addRepositoriesToBackup(ctx context.Context, z *archiveWriter, excludeMirrors bool) error {
	fh := &zip.FileHeader{
		Name:     archiveRootDir + "/repositories.zip",
		Method:   zip.Store, // The entries of the inner archive are already compressed.
		Modified: time.Now(),
	}
	fh.SetMode(0o644)
	w, err := z.CreateHeader(fh)
	if err != nil {
		return errors.Wrap(err, "create entry")
	}

	repos := &archiveWriter{Writer: zip.NewWriter(w), logger: z.logger, verbose: z.verbose}
	baseDir := filepath.Base(conf.Repository.Root)
	if excludeMirrors {
		nonMirrors, err := database.Handle.Repositories().ListNonMirrors(ctx)
		if err != nil {
			return errors.Wrap(err, "get non-mirror repositories")
		}
		for _, r := range nonMirrors {
			// Directories are named in lower case, unlike repositories and their
			// owners.
			owner := r.MustOwner().Name
			repoPaths := []string{repox.RepositoryPath(owner, r.Name)}
			if wikiPath := database.WikiPath(owner, r.Name); osx.IsDir(wikiPath) {
				repoPaths = append(repoPaths, wikiPath)
			}
			for _, repoPath := range repoPaths {
				rel, err := filepath.Rel(conf.Repository.Root, repoPath)
				if err != nil {
					return errors.Wrap(err, "get relative path")
				}
				if err = repos.add(path.Join(baseDir, filepath.ToSlash(rel)), repoPath); err != nil {
					return errors.Wrapf(err, "add %q", rel)
				}
			}
		}
	} else if err = repos.add(baseDir, conf.Repository.Root); err != nil {
		return errors.Wrap(err, "add repositories")
	}
	return repos.Close()
}

// archiveWriter writes files to a zip archive as they are added. Symbolic links
// are stored as links, with their target as the content.
type archiveWriter struct {
	*zip.Writer
	logger  *logx.Logger
	verbose bool
}

// add adds the file or the directory tree at absPath to the archive under
// name. A link at absPath itself, including a Windows junction, is followed.
func (z *archiveWriter) add(name, absPath string) error {
	fi, err := os.Stat(absPath)
	if err != nil {
		return errors.Wrap(err, "get file info")
	}
	if !fi.IsDir() {
		return z.addEntry(name, absPath, fi)
	}
	return fs.WalkDir(os.DirFS(absPath), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return errors.Wrap(err, "get file info")
		}
		return z.addEntry(path.Join(name, rel), filepath.Join(absPath, filepath.FromSlash(rel)), fi)
	})
}

// addEntry adds the file, directory or link at p with its info to the archive
// under name, without the entries of a directory.
func (z *archiveWriter) addEntry(name, p string, fi fs.FileInfo) error {
	fh, err := zip.FileInfoHeader(fi)
	if err != nil {
		return errors.Wrap(err, "create header")
	}
	fh.Name = name
	if z.verbose {
		z.logger.Debug("Adding to archive", "name", fh.Name)
	}
	if fi.IsDir() {
		fh.Name += "/"
		_, err = z.CreateHeader(fh)
		return err
	}

	fh.Method = zip.Deflate
	w, err := z.CreateHeader(fh)
	if err != nil {
		return errors.Wrap(err, "create entry")
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(p)
		if err != nil {
			return errors.Wrap(err, "read link")
		}
		_, err = io.WriteString(w, target)
		return err
	}

	src, err := os.Open(p)
	if err != nil {
		return errors.Wrap(err, "open")
	}
	defer func() { _ = src.Close() }()
	_, err = io.Copy(w, src)
	return err
}
