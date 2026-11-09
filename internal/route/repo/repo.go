package repo

import (
	stdctx "context"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/tool"
)

func MustBeNotEmpty(c *context.Context) {
	if c.Repo.Repository.IsEmpty {
		c.NotFound()
	}
}

func Download(c *context.Context) {
	archivePath, name, err := PrepareArchive(c.Req.Context(), c.Repo.GitRepo, c.Repo.Repository.Name, c.Params("*"))
	if err != nil {
		c.NotFoundOrError(err, "prepare archive")
		return
	}
	c.ServeFile(archivePath, name)
}

type archiveNotExistError struct {
	error
}

func (archiveNotExistError) NotFound() bool {
	return true
}

// PrepareArchive returns the path of the archive described by the uri, e.g.,
// "master.zip" or "v1.0.tar.gz", alongside the file name to serve it as. The
// archive is created when it does not exist yet. It returns an error that
// satisfies errx.IsNotFound when the archive format or the reference is unknown.
func PrepareArchive(ctx stdctx.Context, gitRepo *git.Repository, repoName, uri string) (archivePath, name string, _ error) {
	var (
		ext           string
		archiveFormat git.ArchiveFormat
	)
	switch {
	case strings.HasSuffix(uri, ".zip"):
		ext = ".zip"
		archivePath = filepath.Join(gitRepo.Path(), "archives", "zip")
		archiveFormat = git.ArchiveZip
	case strings.HasSuffix(uri, ".tar.gz"):
		ext = ".tar.gz"
		archivePath = filepath.Join(gitRepo.Path(), "archives", "targz")
		archiveFormat = git.ArchiveTarGz
	default:
		return "", "", archiveNotExistError{errors.Newf("unknown archive format %q", uri)}
	}
	refName := strings.TrimSuffix(uri, ext)

	if !osx.IsDir(archivePath) {
		if err := os.MkdirAll(archivePath, os.ModePerm); err != nil {
			return "", "", errors.Wrap(err, "create archive directory")
		}
	}

	// Get corresponding commit.
	var (
		commit *git.Commit
		err    error
	)
	if gitRepo.HasBranch(ctx, refName) {
		commit, err = gitRepo.BranchCommit(ctx, refName)
		if err != nil {
			return "", "", errors.Wrap(err, "get branch commit")
		}
	} else if gitRepo.HasTag(ctx, refName) {
		commit, err = gitRepo.TagCommit(ctx, refName)
		if err != nil {
			return "", "", errors.Wrap(err, "get tag commit")
		}
	} else if len(refName) >= 7 && len(refName) <= 64 {
		commit, err = gitRepo.CatFileCommit(ctx, refName)
		if err != nil {
			return "", "", archiveNotExistError{errors.Wrapf(err, "get commit %q", refName)}
		}
	} else {
		return "", "", archiveNotExistError{errors.Newf("unknown reference %q", refName)}
	}

	archivePath = path.Join(archivePath, tool.ShortSHA1(commit.ID.String())+ext)
	if !osx.IsFile(archivePath) {
		// Generate into a temporary file and rename it into place, so an interrupted
		// or concurrent generation never leaves a partial archive at the path that
		// gets served for all later requests. Leftovers are removed on startup along
		// with the rest of the temporary data.
		tmpDir := filepath.Join(conf.Server.AppDataPath, "tmp", "archives")
		if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
			return "", "", errors.Wrap(err, "create temporary archive directory")
		}
		tmpFile, err := os.CreateTemp(tmpDir, "archive-*"+ext)
		if err != nil {
			return "", "", errors.Wrap(err, "create temporary archive file")
		}
		tmpPath := tmpFile.Name()
		_ = tmpFile.Close()
		defer func() { _ = os.Remove(tmpPath) }()

		// os.CreateTemp creates the file with 0600, keep the permissions Git would
		// give a newly created archive.
		if err = os.Chmod(tmpPath, 0o644); err != nil {
			return "", "", errors.Wrap(err, "set temporary archive file permissions")
		}

		if err = commit.Archive(ctx, archiveFormat, tmpPath); err != nil {
			return "", "", errors.Wrap(err, "create archive")
		}
		if err = os.Rename(tmpPath, archivePath); err != nil {
			return "", "", errors.Wrap(err, "move archive into place")
		}
	}
	return archivePath, repoName + "-" + refName + ext, nil
}
