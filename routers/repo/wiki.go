// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/markdown"
)

const (
	WIKI_START base.TplName = "repo/wiki/start"
	WIKI_VIEW  base.TplName = "repo/wiki/view"
	WIKI_NEW   base.TplName = "repo/wiki/new"
	WIKI_PAGES base.TplName = "repo/wiki/pages"
)

func MustEnableWiki(ctx *context.Context) {
	if !ctx.Repo.Repository.EnableWiki {
		ctx.Handle(404, "MustEnableWiki", nil)
		return
	}

	if ctx.Repo.Repository.EnableExternalWiki {
		ctx.Redirect(ctx.Repo.Repository.ExternalWikiURL)
		return
	}
}

type PageMeta struct {
	Name    string
	URL     string
	Updated time.Time
}

func UrlEncoded(str string) string {
	u, err := url.Parse(str)
	if err != nil {
		return str
	}
	return u.String()
}
func UrlDecoded(str string) string {
	res, err := url.QueryUnescape(str)
	if err != nil {
		return str
	}
	return res
}
func commitTreeBlobEntry(entry *git.TreeEntry, target string, wikiOnly bool) *git.TreeEntry {
	name := entry.Name()
	ext := filepath.Ext(name)
	if !wikiOnly || markdown.IsMarkdownFile(name) || ext == ".textile" {
		if matchName(name, target) || matchName(UrlEncoded(name), target) || matchName(UrlDecoded(name), target) {
			return entry
		}
		nameOnly := strings.TrimSuffix(name, ext)
		if matchName(nameOnly, target) || matchName(UrlEncoded(nameOnly), target) || matchName(UrlDecoded(nameOnly), target) {
			return entry
		}
	}
	return nil
}
func commitTreeDirEntry(repo *git.Repository, commit *git.Commit, entries []*git.TreeEntry, prevPath, target string, wikiOnly bool) (*git.TreeEntry, error) {
	for i := range entries {
		entry := entries[i]
		if entry.Type == git.OBJECT_BLOB {
			res := commitTreeBlobEntry(entry, target, wikiOnly)
			if res != nil {
				return res, nil
			}
		} else if entry.IsDir() {
			hasSlash := strings.Contains(target, "/")
			if !hasSlash {
				continue
			}
			var nPath string
			if len(prevPath) == 0 {
				nPath = entry.Name()
			} else {
				nPath = prevPath + "/" + entry.Name()
			}
			if !strings.HasPrefix(target, nPath + "/") {
				continue
			}
			var err error
			var te *git.TreeEntry
			if te, err = commit.GetTreeEntryByPath(nPath); te == nil || err != nil {
				if err == nil {
					err = errors.New(fmt.Sprintf("commit.GetTreeEntryByPath(%s) => nil", nPath))
				}
				return nil, err
			}
			var tree *git.Tree
			if tree, err = repo.GetTree(te.ID.String()); tree == nil || err != nil {
				if err == nil {
					err = errors.New(fmt.Sprintf("repo.GetTree(%s) => nil", te.ID.String()))
				}
				return nil, err
			}
			var ls git.Entries
			if ls, err = tree.ListEntries(); err != nil {
				return nil, err
			}
			nTarget := target[strings.Index(target, "/")+1:]
			if te, err = commitTreeDirEntry(repo, commit, ls, nPath, nTarget, wikiOnly); te == nil || err != nil {
				if err == nil {
					err = errors.New(fmt.Sprintf("commitTreeDirEntry(repo, commit, ls, %s, %s, %t) => nil", nPath, nTarget, wikiOnly))
				}
				return nil, err
			}
			return te, nil
		}
	}
	return nil, git.ErrNotExist{"", target}
}
func _commitTreeEntry(repo *git.Repository, commit *git.Commit, target string, wikiOnly bool) (*git.TreeEntry, error) {
	entries, err := commit.ListEntries()
	if err != nil {
		return nil, err
	}
	return commitTreeDirEntry(repo, commit, entries, "", target, wikiOnly)
}
func _findBlob(repo *git.Repository, commit *git.Commit, target string, wikiOnly bool) (*git.Blob, error) {
	entry, err := _commitTreeEntry(repo, commit, target, wikiOnly)
	if err != nil {
		if git.IsErrNotExist(err) {
			entry, err = _commitTreeEntry(repo, commit, UrlEncoded(target), wikiOnly)
			if err != nil {
				if git.IsErrNotExist(err) {
					entry, err = _commitTreeEntry(repo, commit, UrlDecoded(target), wikiOnly)
					if err != nil {
						return nil, err
					}
					return entry.Blob(), nil
				}
				return nil, err
			}
			return entry.Blob(), nil
		}
		return nil, err
	}
	return entry.Blob(), nil
}
func commitTreeEntry(repo *git.Repository, commit *git.Commit, target string) (*git.TreeEntry, error) {
	return _commitTreeEntry(repo, commit, target, true)
}
func findBlob(repo *git.Repository, commit *git.Commit, target string) (*git.Blob, error) {
	return _findBlob(repo, commit, target, true)
}
func matchName(target, name string) bool {
	if len(target) != len(name) {
		return false
	}
	name = strings.ToLower(name)
	target = strings.ToLower(target)
	if name == target {
		return true
	}
	target = strings.Replace(target, " ", "?", -1)
	target = strings.Replace(target, "-", "?", -1)
	for i := range name {
		ch := name[i]
		reqCh := target[i]
		if ch != reqCh {
			if string(reqCh) != "?" {
				return false
			}
		}
	}
	return true
}

func findWikiRepoCommit(ctx *context.Context) (*git.Repository, *git.Commit, error) {
	wikiRepo, err := git.OpenRepository(ctx.Repo.Repository.WikiPath())
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return nil, nil, err
	}
	commit, err := wikiRepo.GetBranchCommit("master")
	if err != nil {
		ctx.Handle(500, "GetBranchCommit", err)
		return wikiRepo, nil, err
	}
	return wikiRepo, commit, nil
}

func renderWikiPage(ctx *context.Context, isViewPage bool) (*git.Repository, *git.TreeEntry) {
	wikiRepo, commit, err := findWikiRepoCommit(ctx)
	if err != nil {
		return nil, nil
	}

	// Get page list.
	if isViewPage {
		entries, err := commit.ListEntries()
		if err != nil {
			ctx.Handle(500, "ListEntries", err)
			return nil, nil
		}
		pages := make([]PageMeta, 0, len(entries))
		for i := range entries {
			if entries[i].Type == git.OBJECT_BLOB {
				name := entries[i].Name()
				ext := filepath.Ext(name)
				if markdown.IsMarkdownFile(name) || ext == ".textile" {
					name = strings.TrimSuffix(name, ext)
					if name == "_Sidebar" || name == "_Footer" || name == "_Header" {
						continue
					}
					pages = append(pages, PageMeta{
						Name: name,
						URL:  models.ToWikiPageURL(name),
					})
				}
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

	entry, err := commitTreeEntry(wikiRepo, commit, pageName)
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.Redirect(ctx.Repo.RepoLink + "/wiki/_pages")
		} else {
			ctx.Handle(500, "GetBlobByPath", err)
		}
		return nil, nil
	}
	blob := entry.Blob()
	r, err := blob.Data()
	if err != nil {
		ctx.Handle(500, "Data", err)
		return nil, nil
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		ctx.Handle(500, "ReadAll", err)
		return nil, nil
	}
	sidebarPresent := false
	sidebarContent := []byte{}
	blob, err = findBlob(wikiRepo, commit, "_Sidebar")
	if err == nil {
		r, err = blob.Data()
		if err == nil {
			dataSB, err := ioutil.ReadAll(r)
			if err == nil {
				sidebarPresent = true
				sidebarContent = dataSB
			}
		}
	}
	footerPresent := false
	footerContent := []byte{}
	blob, err = findBlob(wikiRepo, commit, "_Footer")
	if err == nil {
		r, err = blob.Data()
		if err == nil {
			dataSB, err := ioutil.ReadAll(r)
			if err == nil {
				footerPresent = true
				footerContent = dataSB
			}
		}
	}
	if isViewPage {
		metas := ctx.Repo.Repository.ComposeMetas()
		ctx.Data["content"] = markdown.RenderWiki(data, ctx.Repo.RepoLink, metas)
		ctx.Data["sidebarPresent"] = sidebarPresent
		ctx.Data["sidebarContent"] = markdown.RenderWiki(sidebarContent, ctx.Repo.RepoLink, metas)
		ctx.Data["footerPresent"] = footerPresent
		ctx.Data["footerContent"] = markdown.RenderWiki(footerContent, ctx.Repo.RepoLink, metas)
	} else {
		ctx.Data["content"] = string(data)
		ctx.Data["sidebarPresent"] = false
		ctx.Data["sidebarContent"] = ""
		ctx.Data["footerPresent"] = false
		ctx.Data["footerContent"] = ""
	}

	return wikiRepo, entry
}

func Wiki(ctx *context.Context) {
	ctx.Data["PageIsWiki"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Data["Title"] = ctx.Tr("repo.wiki")
		ctx.HTML(200, WIKI_START)
		return
	}

	wikiRepo, entry := renderWikiPage(ctx, true)
	if ctx.Written() {
		return
	}

	ename := entry.Name()
	if !markdown.IsMarkdownFile(ename) {
		ext := strings.ToUpper(filepath.Ext(ename))
		ctx.Data["FormatWarning"] = fmt.Sprintf("%s rendering is not supported at the moment. Rendered as Markdown.", ext)
	}
	// Get last change information.
	lastCommit, err := wikiRepo.GetCommitByPath(ename)
	if err != nil {
		ctx.Handle(500, "GetCommitByPath", err)
		return
	}
	ctx.Data["Author"] = lastCommit.Author

	ctx.HTML(200, WIKI_VIEW)
}

func WikiPages(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.wiki.pages")
	ctx.Data["PageIsWiki"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Redirect(ctx.Repo.RepoLink + "/wiki")
		return
	}

	wikiRepo, commit, err := findWikiRepoCommit(ctx)
	if err != nil {
		return
	}

	entries, err := commit.ListEntries()
	if err != nil {
		ctx.Handle(500, "ListEntries", err)
		return
	}
	pages := make([]PageMeta, 0, len(entries))
	for i := range entries {
		if entries[i].Type == git.OBJECT_BLOB {
			c, err := wikiRepo.GetCommitByPath(entries[i].Name())
			if err != nil {
				ctx.Handle(500, "GetCommit", err)
				return
			}
			name := entries[i].Name()
			ext := filepath.Ext(name)
			if markdown.IsMarkdownFile(name) || ext == ".textile" {
				name = strings.TrimSuffix(name, ext)
				pages = append(pages, PageMeta{
					Name:    name,
					URL:     models.ToWikiPageURL(name),
					Updated: c.Author.When,
				})
			}
		}
	}
	ctx.Data["Pages"] = pages

	ctx.HTML(200, WIKI_PAGES)
}

func WikiRaw(ctx *context.Context) {
	wikiRepo, commit, err := findWikiRepoCommit(ctx)
	if err != nil {
		return
	}
	uri := ctx.Params("*")
	blob, err := _findBlob(wikiRepo, commit, uri, false)
	if err != nil {
		if git.IsErrNotExist(err) {
			defBranch := ctx.Repo.Repository.DefaultBranch
			var commit *git.Commit
			if commit, err = ctx.Repo.GitRepo.GetBranchCommit(defBranch); commit == nil || err != nil {
				ctx.Handle(500, "GetBranchCommit", err)
				return
			}
			if blob, err = commit.GetBlobByPath(ctx.Repo.TreePath); blob == nil || err != nil {
				ctx.Handle(404, "GetBlobByPath", nil)
				return
			}
			ctx.Redirect(ctx.Repo.RepoLink + "/raw/"+defBranch+"/"+uri)
		} else {
			ctx.Handle(500, "GetBlobByPath", err)
		}
		return
	}
	if err = ServeBlob(ctx, blob); err != nil {
		ctx.Handle(500, "ServeBlob", err)
	}
}

func NewWiki(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.wiki.new_page")
	ctx.Data["PageIsWiki"] = true
	ctx.Data["RequireSimpleMDE"] = true

	if !ctx.Repo.Repository.HasWiki() {
		ctx.Data["title"] = "Home"
	}

	ctx.HTML(200, WIKI_NEW)
}

func NewWikiPost(ctx *context.Context, form auth.NewWikiForm) {
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

func EditWiki(ctx *context.Context) {
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

func EditWikiPost(ctx *context.Context, form auth.NewWikiForm) {
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

func DeleteWikiPagePost(ctx *context.Context) {
	pageURL := ctx.Params(":page")
	if len(pageURL) == 0 {
		pageURL = "Home"
	}

	pageName := models.ToWikiPageName(pageURL)
	if err := ctx.Repo.Repository.DeleteWikiPage(ctx.User, pageName); err != nil {
		ctx.Handle(500, "DeleteWikiPage", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/wiki/",
	})
}
