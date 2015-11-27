// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"io/ioutil"
	"strings"

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

type PageMeta struct {
	Name string
	URL  string
}

func renderWikiPage(ctx *middleware.Context, isViewPage bool) (*git.Repository, string) {
	wikiRepo, err := git.OpenRepository(ctx.Repo.Repository.WikiPath())
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return nil, ""
	}
	commit, err := wikiRepo.GetCommitOfBranch("master")
	if err != nil {
		ctx.Handle(500, "GetCommitOfBranch", err)
		return nil, ""
	}

	// Get page list.
	if isViewPage {
		entries, err := commit.ListEntries()
		if err != nil {
			ctx.Handle(500, "ListEntries", err)
			return nil, ""
		}
		pages := make([]PageMeta, len(entries))
		for i := range entries {
			name := strings.TrimSuffix(entries[i].Name(), ".md")
			pages[i] = PageMeta{
				Name: name,
				URL:  models.ToWikiPageURL(name),
			}
		}
		ctx.Data["Pages"] = pages
	}

	pageURL := ctx.Params(":page")
	if len(pageURL) == 0 {
		pageURL = "Home"
	}
	ctx.Data["PageURL"] = pageURL

	pageName := models.ToWikiPageName(pageURL)
	ctx.Data["old_title"] = pageName
	ctx.Data["Title"] = pageName
	ctx.Data["title"] = pageName
	ctx.Data["RequireHighlightJS"] = true

	blob, err := commit.GetBlobByPath(pageName + ".md")
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.Redirect(ctx.Repo.RepoLink + "/wiki/_pages")
		} else {
			ctx.Handle(500, "GetBlobByPath", err)
		}
		return nil, ""
	}
	r, err := blob.Data()
	if err != nil {
		ctx.Handle(500, "Data", err)
		return nil, ""
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		ctx.Handle(500, "ReadAll", err)
		return nil, ""
	}
	if isViewPage {
		ctx.Data["content"] = string(base.RenderMarkdown(data, ctx.Repo.RepoLink))
	} else {
		ctx.Data["content"] = string(data)
	}

	return wikiRepo, pageName
}

func Wiki(ctx *middleware.Context) {
	ctx.Data["PageIsWiki"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Data["Title"] = ctx.Tr("repo.wiki")
		ctx.HTML(200, WIKI_START)
		return
	}

	wikiRepo, pageName := renderWikiPage(ctx, true)
	if ctx.Written() {
		return
	}

	// Get last change information.
	lastCommit, err := wikiRepo.GetCommitByPath(pageName + ".md")
	if err != nil {
		ctx.Handle(500, "GetCommitByPath", err)
		return
	}
	ctx.Data["Author"] = lastCommit.Author

	ctx.HTML(200, WIKI_VIEW)
}

func WikiPages(ctx *middleware.Context) {

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
		if models.IsErrWikiAlreadyExist(err) {
			ctx.Data["Err_Title"] = true
			ctx.RenderWithErr(ctx.Tr("repo.wiki.page_already_exists"), WIKI_NEW, &form)
		} else {
			ctx.Handle(500, "AddWikiPage", err)
		}
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/wiki/" + models.ToWikiPageURL(form.Title))
}

func EditWiki(ctx *middleware.Context) {
	ctx.Data["PageIsWiki"] = true
	ctx.Data["PageIsWikiEdit"] = true
	ctx.Data["RequireSimpleMDE"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Redirect(ctx.Repo.RepoLink + "/wiki")
		return
	}

	renderWikiPage(ctx, false)
	if ctx.Written() {
		return
	}

	ctx.HTML(200, WIKI_NEW)
}

func EditWikiPost(ctx *middleware.Context, form auth.NewWikiForm) {
	ctx.Data["Title"] = ctx.Tr("repo.wiki.new_page")
	ctx.Data["PageIsWiki"] = true
	ctx.Data["RequireSimpleMDE"] = true

	if ctx.HasError() {
		ctx.HTML(200, WIKI_NEW)
		return
	}

	if err := ctx.Repo.Repository.EditWikiPage(ctx.User, form.OldTitle, form.Title, form.Content, form.Message); err != nil {
		ctx.Handle(500, "EditWikiPage", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/wiki/" + models.ToWikiPageURL(form.Title))
}
