package repo

import (
	"mime"
	"net/http"
	"path"
	"strings"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/tool"
)

func MustEnableWiki(c *context.Context) {
	if !c.Repo.Repository.EnableWiki {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalWiki {
		c.Redirect(c.Repo.Repository.ExternalWikiURL)
		return
	}
}

// ServeWikiPage serves a file of the wiki that is not a page, e.g., an image
// that a page links to relatively, and hands everything else to the web
// frontend.
func ServeWikiPage(c *context.Context) {
	name := c.Params(":page")
	if !c.Repo.Repository.HasWiki() || path.Ext(name) == "" || strings.EqualFold(path.Ext(name), ".md") {
		c.ServeWeb()
		return
	}

	ctx := c.Req.Context()
	wikiPath := c.Repo.Repository.WikiPath()
	wikiRepo, err := git.Open(wikiPath)
	if err != nil {
		c.Error(err, "open wiki repository")
		return
	}
	commit, err := wikiRepo.BranchCommit(ctx, database.WikiBranch(ctx, wikiPath))
	if err != nil {
		c.Error(err, "get wiki branch commit")
		return
	}
	blob, err := commit.Blob(ctx, name)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			c.ServeWeb()
			return
		}
		c.Error(err, "get wiki file")
		return
	}
	data, err := blob.Bytes(ctx)
	if err != nil {
		c.Error(err, "read wiki file")
		return
	}

	// Like raw repository files, only images are shown inline, and text is
	// served as plain text so it never runs as a page of this site.
	if !tool.IsTextFile(data) {
		if !tool.IsImageFile(data) {
			c.Resp.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			c.Resp.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !conf.Repository.EnableRawFileRenderMode || !c.QueryBool("render") {
		c.Resp.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	c.Resp.WriteHeader(http.StatusOK)
	_, _ = c.Resp.Write(data)
}
