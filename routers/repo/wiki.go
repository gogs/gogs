// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
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
	ctx.Data["Title"] = ctx.Tr("repo.wiki")
	ctx.Data["PageIsWiki"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.HTML(200, WIKI_START)
		return
	}

	ctx.HTML(200, WIKI_VIEW)
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

	if err := ctx.Repo.Repository.AddWikiPage(form.Title, form.Content, form.Message); err != nil {
		ctx.Handle(500, "AddWikiPage", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/wiki/" + models.ToWikiPageName(form.Title))
}

func EditWiki(ctx *middleware.Context) {
	ctx.PlainText(200, []byte(ctx.Params(":page")))
}
