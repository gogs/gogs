package v1

import (
	"bytes"
	"net/http"
	"path"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/editorconfig/editorconfig-core-go/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/mimex"
	"gogs.io/gogs/internal/repox"
)

func getRawFile(c *apiContext) {
	if !c.Repo.HasAccess() {
		c.NotFound()
		return
	}

	if c.Repo.Repository.IsEmpty {
		c.NotFound()
		return
	}

	ctx := c.Request().Context()
	gitRepo, err := git.Open(database.RepoPath(c.Repo.Owner.Name, c.Repo.Repository.Name))
	if err != nil {
		c.webError(err, "open repository")
		return
	}

	// Failing to resolve the reference responds with the web pages, as it did
	// before migrating to Flamego.
	//
	// The shortest prefix of the path that names a branch or a tag is the
	// reference, the rest is the tree path. Otherwise, the first segment must be
	// a full commit SHA.
	var refName, treePath string
	hasMatched := false
	parts := strings.Split(c.Param("**"), "/")
	for i, part := range parts {
		refName = strings.TrimPrefix(refName+"/"+part, "/")
		if gitRepo.HasBranch(ctx, refName) || gitRepo.HasTag(ctx, refName) {
			treePath = strings.Join(parts[i+1:], "/")
			hasMatched = true
			break
		}
	}
	if !hasMatched && (len(parts[0]) == 40 || len(parts[0]) == 64) {
		refName = parts[0]
		treePath = strings.Join(parts[1:], "/")
	}

	var commit *git.Commit
	switch {
	case gitRepo.HasBranch(ctx, refName):
		commit, err = gitRepo.BranchCommit(ctx, refName)
		if err != nil {
			c.webError(err, "get branch commit")
			return
		}
	case gitRepo.HasTag(ctx, refName):
		commit, err = gitRepo.TagCommit(ctx, refName)
		if err != nil {
			c.webError(err, "get tag commit")
			return
		}
	case len(refName) == 40 || len(refName) == 64:
		commit, err = gitRepo.CatFileCommit(ctx, refName)
		if err != nil {
			c.webNotFound()
			return
		}
	default:
		c.webNotFound()
		return
	}

	entry, err := commit.TreeEntry(ctx, treePath)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get tree entry")
		return
	}
	// The content of a symlink is its target path, which is served instead of
	// following the link.
	if !entry.IsBlob() && !entry.IsExec() && !entry.IsSymlink() {
		c.NotFound()
		return
	}
	if err = serveBlob(c, commit, treePath, entry.Blob()); err != nil {
		c.Error(err, "serve blob")
	}
}

func serveBlob(c *apiContext, commit *git.Commit, treePath string, blob *git.Blob) error {
	ctx := c.Request().Context()
	data, err := blob.Bytes(ctx)
	if err != nil {
		return err
	}

	pathCommit, err := commit.CommitByPath(ctx, git.CommitByRevisionOptions{Path: treePath})
	if err != nil {
		return errors.Newf("get commit by path %q: %v", treePath, err)
	}

	w := c.ResponseWriter()
	w.Header().Set("Last-Modified", pathCommit.Committer.When.Format(http.TimeFormat))
	if !mimex.IsText(data) {
		if !mimex.IsImage(data) {
			w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(treePath)+"\"")
			w.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !conf.Repository.EnableRawFileRenderMode || !c.QueryBool("render") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	if _, err := w.Write(data); err != nil {
		return errors.Newf("write buffer to response: %v", err)
	}
	return nil
}

func getArchive(c *apiContext) {
	repoPath := database.RepoPath(c.Param("username"), c.Param("reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	archivePath, name, err := repox.PrepareArchive(c.Request().Context(), gitRepo, c.Repo.Repository.Name, c.Param("**"))
	if err != nil {
		// Responds with the web pages, as it did before migrating to Flamego.
		c.webNotFoundOrError(err, "prepare archive")
		return
	}

	repox.ServeArchive(c.ResponseWriter(), c.Request().Request, archivePath, name)
}

func getEditorconfig(c *apiContext) {
	gitRepo, err := git.Open(database.RepoPath(c.Repo.Owner.Name, c.Repo.Repository.Name))
	if err != nil {
		c.webError(err, "open repository")
		return
	}

	ctx := c.Request().Context()
	commit, err := gitRepo.BranchCommit(ctx, c.Repo.Repository.DefaultBranch)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get commit of default branch")
		return
	}
	entry, err := commit.TreeEntry(ctx, ".editorconfig")
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get .editorconfig")
		return
	}
	p, err := entry.Blob().Bytes(ctx)
	if err != nil {
		c.Error(err, "read .editorconfig")
		return
	}
	ec, err := editorconfig.Parse(bytes.NewReader(p))
	if err != nil {
		c.Error(err, "parse .editorconfig")
		return
	}

	fileName := c.Param("filename")
	def, err := ec.GetDefinitionForFilename(fileName)
	if err != nil {
		c.Error(err, "get definition for filename")
		return
	}
	if def == nil {
		c.NotFound()
		return
	}
	c.JSONSuccess(def)
}
