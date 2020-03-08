// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"strings"
	"time"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/markup"
)

const (
	WIKI_START = "repo/wiki/start"
	WIKI_VIEW  = "repo/wiki/view"
	WIKI_NEW   = "repo/wiki/new"
	WIKI_PAGES = "repo/wiki/pages"
)

func MustEnableWiki(c *context.Context) {
	if !c.Repo.Repository.EnableWiki {
		c.Handle(404, "MustEnableWiki", nil)
		return
	}

	if c.Repo.Repository.EnableExternalWiki {
		c.Redirect(c.Repo.Repository.ExternalWikiURL)
		return
	}
}

type PageMeta struct {
	Name    string
	URL     string
	Updated time.Time
}

func renderWikiPage(c *context.Context, isViewPage bool) (*git.Repository, string) {
	wikiRepo, err := git.Open(c.Repo.Repository.WikiPath())
	if err != nil {
		c.ServerError("open repository", err)
		return nil, ""
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.ServerError("get branch commit", err)
		return nil, ""
	}

	// Get page list.
	if isViewPage {
		entries, err := commit.Entries()
		if err != nil {
			c.ServerError("list entries", err)
			return nil, ""
		}
		pages := make([]PageMeta, 0, len(entries))
		for i := range entries {
			if entries[i].Type() == git.ObjectBlob && strings.HasSuffix(entries[i].Name(), ".md") {
				name := strings.TrimSuffix(entries[i].Name(), ".md")
				pages = append(pages, PageMeta{
					Name: name,
					URL:  db.ToWikiPageURL(name),
				})
			}
		}
		c.Data["Pages"] = pages
	}

	pageURL := c.Params(":page")
	if len(pageURL) == 0 {
		pageURL = "Home"
	}
	c.Data["PageURL"] = pageURL

	pageName := db.ToWikiPageName(pageURL)
	c.Data["old_title"] = pageName
	c.Data["Title"] = pageName
	c.Data["title"] = pageName
	c.Data["RequireHighlightJS"] = true

	blob, err := commit.Blob(pageName + ".md")
	if err != nil {
		if gitutil.IsErrRevisionNotExist(err) {
			c.Redirect(c.Repo.RepoLink + "/wiki/_pages")
		} else {
			c.ServerError("GetBlobByPath", err)
		}
		return nil, ""
	}
	p, err := blob.Bytes()
	if err != nil {
		c.ServerError("Data", err)
		return nil, ""
	}
	if isViewPage {
		c.Data["content"] = string(markup.Markdown(p, c.Repo.RepoLink, c.Repo.Repository.ComposeMetas()))
	} else {
		c.Data["content"] = string(p)
	}

	return wikiRepo, pageName
}

func Wiki(c *context.Context) {
	c.Data["PageIsWiki"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Data["Title"] = c.Tr("repo.wiki")
		c.HTML(200, WIKI_START)
		return
	}

	wikiRepo, pageName := renderWikiPage(c, true)
	if c.Written() {
		return
	}

	// Get last change information.
	commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: pageName + ".md"})
	if err != nil {
		c.ServerError("get commits by path", err)
		return
	}
	c.Data["Author"] = commits[0].Author

	c.HTML(200, WIKI_VIEW)
}

func WikiPages(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.wiki.pages")
	c.Data["PageIsWiki"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Redirect(c.Repo.RepoLink + "/wiki")
		return
	}

	wikiRepo, err := git.Open(c.Repo.Repository.WikiPath())
	if err != nil {
		c.ServerError("open repository", err)
		return
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.ServerError("get branch commit", err)
		return
	}

	entries, err := commit.Entries()
	if err != nil {
		c.ServerError("list entries", err)
		return
	}
	pages := make([]PageMeta, 0, len(entries))
	for i := range entries {
		if entries[i].Type() == git.ObjectBlob && strings.HasSuffix(entries[i].Name(), ".md") {
			commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: entries[i].Name()})
			if err != nil {
				c.ServerError("get commits by path", err)
				return
			}
			name := strings.TrimSuffix(entries[i].Name(), ".md")
			pages = append(pages, PageMeta{
				Name:    name,
				URL:     db.ToWikiPageURL(name),
				Updated: commits[0].Author.When,
			})
		}
	}
	c.Data["Pages"] = pages

	c.HTML(200, WIKI_PAGES)
}

func NewWiki(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Data["title"] = "Home"
	}

	c.HTML(200, WIKI_NEW)
}

func NewWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.HTML(200, WIKI_NEW)
		return
	}

	if err := c.Repo.Repository.AddWikiPage(c.User, f.Title, f.Content, f.Message); err != nil {
		if db.IsErrWikiAlreadyExist(err) {
			c.Data["Err_Title"] = true
			c.RenderWithErr(c.Tr("repo.wiki.page_already_exists"), WIKI_NEW, &f)
		} else {
			c.ServerError("AddWikiPage", err)
		}
		return
	}

	c.Redirect(c.Repo.RepoLink + "/wiki/" + db.ToWikiPageURL(db.ToWikiPageName(f.Title)))
}

func EditWiki(c *context.Context) {
	c.Data["PageIsWiki"] = true
	c.Data["PageIsWikiEdit"] = true
	c.Data["RequireSimpleMDE"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Redirect(c.Repo.RepoLink + "/wiki")
		return
	}

	renderWikiPage(c, false)
	if c.Written() {
		return
	}

	c.HTML(200, WIKI_NEW)
}

func EditWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.HTML(200, WIKI_NEW)
		return
	}

	if err := c.Repo.Repository.EditWikiPage(c.User, f.OldTitle, f.Title, f.Content, f.Message); err != nil {
		c.ServerError("EditWikiPage", err)
		return
	}

	c.Redirect(c.Repo.RepoLink + "/wiki/" + db.ToWikiPageURL(db.ToWikiPageName(f.Title)))
}

func DeleteWikiPagePost(c *context.Context) {
	pageURL := c.Params(":page")
	if len(pageURL) == 0 {
		pageURL = "Home"
	}

	pageName := db.ToWikiPageName(pageURL)
	if err := c.Repo.Repository.DeleteWikiPage(c.User, pageName); err != nil {
		c.ServerError("DeleteWikiPage", err)
		return
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/wiki/",
	})
}
