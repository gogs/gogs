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
		c.NotFound()
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
		c.Error(err, "open repository")
		return nil, ""
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.Error(err, "get branch commit")
		return nil, ""
	}

	// Get page list.
	if isViewPage {
		entries, err := commit.Entries()
		if err != nil {
			c.Error(err, "list entries")
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
	if pageURL == "" {
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
			c.Error(err, "get blob")
		}
		return nil, ""
	}
	p, err := blob.Bytes()
	if err != nil {
		c.Error(err, "read blob")
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
		c.Success(WIKI_START)
		return
	}

	wikiRepo, pageName := renderWikiPage(c, true)
	if c.Written() {
		return
	}

	// Get last change information.
	commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: pageName + ".md"})
	if err != nil {
		c.Error(err, "get commits by path")
		return
	}
	c.Data["Author"] = commits[0].Author

	c.Success(WIKI_VIEW)
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
		c.Error(err, "open repository")
		return
	}
	commit, err := wikiRepo.BranchCommit("master")
	if err != nil {
		c.Error(err, "get branch commit")
		return
	}

	entries, err := commit.Entries()
	if err != nil {
		c.Error(err, "list entries")
		return
	}
	pages := make([]PageMeta, 0, len(entries))
	for i := range entries {
		if entries[i].Type() == git.ObjectBlob && strings.HasSuffix(entries[i].Name(), ".md") {
			commits, err := wikiRepo.Log(git.RefsHeads+"master", git.LogOptions{Path: entries[i].Name()})
			if err != nil {
				c.Error(err, "get commits by path")
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

	c.Success(WIKI_PAGES)
}

func NewWiki(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if !c.Repo.Repository.HasWiki() {
		c.Data["title"] = "Home"
	}

	c.Success(WIKI_NEW)
}

func NewWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.Success(WIKI_NEW)
		return
	}

	if err := c.Repo.Repository.AddWikiPage(c.User, f.Title, f.Content, f.Message); err != nil {
		if db.IsErrWikiAlreadyExist(err) {
			c.Data["Err_Title"] = true
			c.RenderWithErr(c.Tr("repo.wiki.page_already_exists"), WIKI_NEW, &f)
		} else {
			c.Error(err, "add wiki page")
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

	c.Success(WIKI_NEW)
}

func EditWikiPost(c *context.Context, f form.NewWiki) {
	c.Data["Title"] = c.Tr("repo.wiki.new_page")
	c.Data["PageIsWiki"] = true
	c.Data["RequireSimpleMDE"] = true

	if c.HasError() {
		c.Success(WIKI_NEW)
		return
	}

	if err := c.Repo.Repository.EditWikiPage(c.User, f.OldTitle, f.Title, f.Content, f.Message); err != nil {
		c.Error(err, "edit wiki page")
		return
	}

	c.Redirect(c.Repo.RepoLink + "/wiki/" + db.ToWikiPageURL(db.ToWikiPageName(f.Title)))
}

func DeleteWikiPagePost(c *context.Context) {
	pageURL := c.Params(":page")
	if pageURL == "" {
		pageURL = "Home"
	}

	pageName := db.ToWikiPageName(pageURL)
	if err := c.Repo.Repository.DeleteWikiPage(c.User, pageName); err != nil {
		c.Error(err, "delete wiki page")
		return
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/wiki/",
	})
}
