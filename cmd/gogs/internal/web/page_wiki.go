package web

import (
	"mime"
	"net/http"
	"path"
	"strings"

	log "unknwon.dev/clog/v2"

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
		log.Error("serveWikiPage: open wiki repository of repository %d: %v", repo.ID, err)
		c.ServerError(err)
		return
	}
	commit, err := wikiRepo.BranchCommit(ctx, database.WikiBranch(ctx, wikiPath))
	if err != nil {
		log.Error("serveWikiPage: get wiki branch commit of repository %d: %v", repo.ID, err)
		c.ServerError(err)
		return
	}
	blob, err := commit.Blob(ctx, name)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			c.ServeWeb()
			return
		}
		log.Error("serveWikiPage: get wiki file %q of repository %d: %v", name, repo.ID, err)
		c.ServerError(err)
		return
	}
	data, err := blob.Bytes(ctx)
	if err != nil {
		log.Error("serveWikiPage: read wiki file %q of repository %d: %v", name, repo.ID, err)
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
