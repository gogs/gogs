package repo

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/urlx"
)

func MustBeNotEmpty(c *context.Context) {
	if c.Repo.Repository.IsEmpty {
		c.NotFound()
	}
}

func checkContextUser(c *context.Context, uid int64) *database.User {
	orgs, err := database.GetOwnedOrgsByUserIDDesc(c.User.ID, "updated_unix")
	if err != nil {
		c.Error(err, "get owned organization by user ID")
		return nil
	}
	c.Data["Orgs"] = orgs

	// Not equal means current user is an organization.
	if uid == c.User.ID || uid == 0 {
		return c.User
	}

	org, err := database.Handle.Users().GetByID(c.Req.Context(), uid)
	if database.IsErrUserNotExist(err) {
		return c.User
	}

	if err != nil {
		c.Error(err, "get user by ID")
		return nil
	}

	// Check ownership of organization.
	if !org.IsOrganization() || !(c.User.IsAdmin || org.IsOwnedBy(c.User.ID)) {
		c.Status(http.StatusForbidden)
		return nil
	}
	return org
}

func Action(c *context.Context) {
	var err error
	switch c.Params(":action") {
	case "watch":
		err = database.WatchRepo(c.User.ID, c.Repo.Repository.ID, true)
	case "unwatch":
		if userID := c.QueryInt64("user_id"); userID != 0 {
			if c.User.IsAdmin {
				err = database.WatchRepo(userID, c.Repo.Repository.ID, false)
			}
		} else {
			err = database.WatchRepo(c.User.ID, c.Repo.Repository.ID, false)
		}
	case "star":
		err = database.StarRepo(c.User.ID, c.Repo.Repository.ID, true)
	case "unstar":
		err = database.StarRepo(c.User.ID, c.Repo.Repository.ID, false)
	case "desc": // FIXME: this is not used
		if !c.Repo.IsOwner() {
			c.NotFound()
			return
		}

		c.Repo.Repository.Description = c.Query("desc")
		c.Repo.Repository.Website = c.Query("site")
		err = database.UpdateRepository(c.Repo.Repository, false)
	}

	if err != nil {
		c.Errorf(err, "action %q", c.Params(":action"))
		return
	}

	redirectTo := c.Query("redirect_to")
	if !urlx.IsSameSite(redirectTo) {
		redirectTo = c.Repo.RepoLink
	}
	c.Redirect(redirectTo)
}

func Download(c *context.Context) {
	var (
		uri           = c.Params("*")
		refName       string
		ext           string
		archivePath   string
		archiveFormat git.ArchiveFormat
	)

	switch {
	case strings.HasSuffix(uri, ".zip"):
		ext = ".zip"
		archivePath = filepath.Join(c.Repo.GitRepo.Path(), "archives", "zip")
		archiveFormat = git.ArchiveZip
	case strings.HasSuffix(uri, ".tar.gz"):
		ext = ".tar.gz"
		archivePath = filepath.Join(c.Repo.GitRepo.Path(), "archives", "targz")
		archiveFormat = git.ArchiveTarGz
	default:
		log.Trace("Unknown format: %s", uri)
		c.NotFound()
		return
	}
	refName = strings.TrimSuffix(uri, ext)

	if !osx.IsDir(archivePath) {
		if err := os.MkdirAll(archivePath, os.ModePerm); err != nil {
			c.Error(err, "create archive directory")
			return
		}
	}

	// Get corresponding commit.
	var (
		commit *git.Commit
		err    error
	)
	gitRepo := c.Repo.GitRepo
	if gitRepo.HasBranch(c.Req.Context(), refName) {
		commit, err = gitRepo.BranchCommit(c.Req.Context(), refName)
		if err != nil {
			c.Error(err, "get branch commit")
			return
		}
	} else if gitRepo.HasTag(c.Req.Context(), refName) {
		commit, err = gitRepo.TagCommit(c.Req.Context(), refName)
		if err != nil {
			c.Error(err, "get tag commit")
			return
		}
	} else if len(refName) >= 7 && len(refName) <= 64 {
		commit, err = gitRepo.CatFileCommit(c.Req.Context(), refName)
		if err != nil {
			c.NotFound()
			return
		}
	} else {
		c.NotFound()
		return
	}

	archivePath = path.Join(archivePath, tool.ShortSHA1(commit.ID.String())+ext)
	if !osx.IsFile(archivePath) {
		// Generate into a temporary file and rename it into place, so an interrupted
		// or concurrent generation never leaves a partial archive at the path that
		// gets served for all later requests. Leftovers are removed on startup along
		// with the rest of the temporary data.
		tmpDir := filepath.Join(conf.Server.AppDataPath, "tmp", "archives")
		if err := os.MkdirAll(tmpDir, os.ModePerm); err != nil {
			c.Error(err, "create temporary archive directory")
			return
		}
		tmpFile, err := os.CreateTemp(tmpDir, "archive-*"+ext)
		if err != nil {
			c.Error(err, "create temporary archive file")
			return
		}
		tmpPath := tmpFile.Name()
		_ = tmpFile.Close()
		defer func() { _ = os.Remove(tmpPath) }()

		// os.CreateTemp creates the file with 0600, keep the permissions Git would
		// give a newly created archive.
		if err = os.Chmod(tmpPath, 0o644); err != nil {
			c.Error(err, "set temporary archive file permissions")
			return
		}

		if err = commit.Archive(c.Req.Context(), archiveFormat, tmpPath); err != nil {
			c.Error(err, "creates archive")
			return
		}
		if err = os.Rename(tmpPath, archivePath); err != nil {
			c.Error(err, "move archive into place")
			return
		}
	}

	c.ServeFile(archivePath, c.Repo.Repository.Name+"-"+refName+ext)
}
