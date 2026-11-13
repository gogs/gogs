package web

import (
	"mime"
	"net/http"
	"path"
	"strings"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/tool"
)

// serveWikiPage serves a file of the wiki that is not a page, e.g., an image
// that a page links to relatively, and hands everything else to the web app.
// Mount it after withPageRepo and requireWiki.
func serveWikiPage(c *pageContext) {
	repo := c.Repo.Repository
	name := c.Param("page")
	if !repo.HasWiki() || path.Ext(name) == "" || strings.EqualFold(path.Ext(name), ".md") {
		c.ServeWeb()
		return
	}

	ctx := c.Request().Context()
	wikiPath := repo.WikiPath()
	wikiRepo, err := git.Open(wikiPath)
	if err != nil {
		c.Logger.Error("Failed to open wiki repository", "repoID", repo.ID, "error", err)
		c.ServerError(err)
		return
	}
	commit, err := wikiRepo.BranchCommit(ctx, database.WikiBranch(ctx, wikiPath))
	if err != nil {
		c.Logger.Error("Failed to get wiki branch commit", "repoID", repo.ID, "error", err)
		c.ServerError(err)
		return
	}
	blob, err := commit.Blob(ctx, name)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			c.ServeWeb()
			return
		}
		c.Logger.Error("Failed to get wiki file", "repoID", repo.ID, "fileName", name, "error", err)
		c.ServerError(err)
		return
	}
	data, err := blob.Bytes(ctx)
	if err != nil {
		c.Logger.Error("Failed to read wiki file", "repoID", repo.ID, "fileName", name, "error", err)
		c.ServerError(err)
		return
	}

	// Like raw repository files, only images are shown inline, and text is
	// served as plain text so it never runs as a page of this site.
	w := c.ResponseWriter()
	if !tool.IsTextFile(data) {
		if !tool.IsImageFile(data) {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			w.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !conf.Repository.EnableRawFileRenderMode || !c.QueryBool("render") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
