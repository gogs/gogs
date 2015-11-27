// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"io/ioutil"

	"github.com/gogits/git-shell"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	WIKI_START base.TplName = "repo/wiki/start"
	WIKI_VIEW  base.TplName = "repo/wiki/view"
	WIKI_NEW   base.TplName = "repo/wiki/new"
)

func Wiki(ctx *middleware.Context) {
	ctx.Data["PageIsWiki"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Data["Title"] = ctx.Tr("repo.wiki")
		ctx.HTML(200, WIKI_START)
		return
	}

	wikiRepo, err := git.OpenRepository(ctx.Repo.Repository.WikiPath())
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return
	}
	commit, err := wikiRepo.GetCommitOfBranch("master")
	if err != nil {
		ctx.Handle(500, "GetCommitOfBranch", err)
		return
	}

	page := models.ToWikiPageName(ctx.Params(":page"))
	if len(page) == 0 {
		page = "Home"
	}
	ctx.Data["Title"] = page
	ctx.Data["RequireHighlightJS"] = true

	blob, err := commit.GetBlobByPath(page + ".md")
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.Redirect(ctx.Repo.RepoLink + "/wiki/_list")
		} else {
			ctx.Handle(500, "GetBlobByPath", err)
		}
		return
	}
	r, err := blob.Data()
	if err != nil {
		ctx.Handle(500, "Data", err)
		return
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		ctx.Handle(500, "ReadAll", err)
		return
	}
	ctx.Data["Content"] = string(base.RenderMarkdown(data, ctx.Repo.RepoLink))

	// Get last change information.
	lastCommit, err := wikiRepo.GetCommitByPath(page + ".md")
	if err != nil {
		ctx.Handle(500, "GetCommitByPath", err)
		return
	}
	ctx.Data["Author"] = lastCommit.Author

	ctx.HTML(200, WIKI_VIEW)
}

func WikiList(ctx *middleware.Context) {

}

func NewWiki(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.wiki.new_page")
	ctx.Data["PageIsWiki"] = true
	ctx.Data["RequireSimpleMDE"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Data["title"] = "Home"
	}

	ctx.HTML(200, WIKI_NEW)
}

func NewWikiPost(ctx *middleware.Context, form auth.NewWikiForm) {
	ctx.Data["Title"] = ctx.Tr("repo.wiki.new_page")
	ctx.Data["PageIsWiki"] = true
	ctx.Data["RequireSimpleMDE"] = true

	if ctx.HasError() {
		ctx.HTML(200, WIKI_NEW)
		return
	}

	if err := ctx.Repo.Repository.AddWikiPage(ctx.User, form.Title, form.Content, form.Message); err != nil {
		ctx.Handle(500, "AddWikiPage", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/wiki/" + models.ToWikiPageURL(form.Title))
}

func EditWiki(ctx *middleware.Context) {
	ctx.PlainText(200, []byte(ctx.Params(":page")))
}
