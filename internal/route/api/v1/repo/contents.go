// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"

	"github.com/gogs/git-module"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/repoutil"
)

type links struct {
	Git  string `json:"git"`
	Self string `json:"self"`
	HTML string `json:"html"`
}

type repoContent struct {
	Type            string `json:"type"`
	Target          string `json:"target,omitempty"`
	SubmoduleGitURL string `json:"submodule_git_url,omitempty"`
	Encoding        string `json:"encoding,omitempty"`
	Size            int64  `json:"size"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	Content         string `json:"content,omitempty"`
	Sha             string `json:"sha"`
	URL             string `json:"url"`
	GitURL          string `json:"git_url"`
	HTMLURL         string `json:"html_url"`
	DownloadURL     string `json:"download_url"`
	Links           links  `json:"_links"`
}

func toRepoContent(c *context.APIContext, ref, subpath string, commit *git.Commit, entry *git.TreeEntry) (*repoContent, error) {
	repoURL := fmt.Sprintf("%s/repos/%s/%s", c.BaseURL, c.Params(":username"), c.Params(":reponame"))
	selfURL := fmt.Sprintf("%s/contents/%s", repoURL, subpath)
	htmlURL := fmt.Sprintf("%s/src/%s/%s", repoutil.HTMLURL(c.Repo.Owner.Name, c.Repo.Repository.Name), ref, entry.Name())
	downloadURL := fmt.Sprintf("%s/raw/%s/%s", repoutil.HTMLURL(c.Repo.Owner.Name, c.Repo.Repository.Name), ref, entry.Name())

	content := &repoContent{
		Size:        entry.Size(),
		Name:        entry.Name(),
		Path:        subpath,
		Sha:         entry.ID().String(),
		URL:         selfURL,
		HTMLURL:     htmlURL,
		DownloadURL: downloadURL,
		Links: links{
			Self: selfURL,
			HTML: htmlURL,
		},
	}

	switch {
	case entry.IsBlob(), entry.IsExec():
		content.Type = "file"
		p, err := entry.Blob().Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "get blob content")
		}
		content.Encoding = "base64"
		content.Content = base64.StdEncoding.EncodeToString(p)
		content.GitURL = fmt.Sprintf("%s/git/blobs/%s", repoURL, entry.ID().String())

	case entry.IsTree():
		content.Type = "dir"
		content.GitURL = fmt.Sprintf("%s/git/trees/%s", repoURL, entry.ID().String())

	case entry.IsSymlink():
		content.Type = "symlink"
		p, err := entry.Blob().Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "get blob content")
		}
		content.Target = string(p)

	case entry.IsCommit():
		content.Type = "submodule"
		mod, err := commit.Submodule(subpath)
		if err != nil {
			return nil, errors.Wrap(err, "get submodule")
		}
		content.SubmoduleGitURL = mod.URL

	default:
		panic("unreachable")
	}

	content.Links.Git = content.GitURL
	return content, nil
}

func GetContents(c *context.APIContext) {
	repoPath := repoutil.RepositoryPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	ref := c.Query("ref")
	if ref == "" {
		ref = c.Repo.Repository.DefaultBranch
	}

	commit, err := gitRepo.CatFileCommit(ref)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get commit")
		return
	}

	treePath := c.Params("*")
	entry, err := commit.TreeEntry(treePath)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get tree entry")
		return
	}

	if !entry.IsTree() {
		content, err := toRepoContent(c, ref, treePath, commit, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", treePath)
			return
		}

		c.JSONSuccess(content)
		return
	}

	// The entry is a directory
	dir, err := gitRepo.LsTree(entry.ID().String())
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get tree")
		return
	}

	entries, err := dir.Entries()
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "list entries")
		return
	}

	if len(entries) == 0 {
		c.JSONSuccess([]string{})
		return
	}

	contents := make([]*repoContent, 0, len(entries))
	for _, entry := range entries {
		subpath := path.Join(treePath, entry.Name())
		content, err := toRepoContent(c, ref, subpath, commit, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", subpath)
			return
		}

		contents = append(contents, content)
	}
	c.JSONSuccess(contents)
}

// PutContentsRequest is the API message for creating or updating a file.
type PutContentsRequest struct {
	Message string `json:"message" binding:"Required"`
	Content string `json:"content" binding:"Required"`
	Branch  string `json:"branch"`
}

// PUT /repos/:username/:reponame/contents/*
func PutContents(c *context.APIContext, r PutContentsRequest) {
	content, err := base64.StdEncoding.DecodeString(r.Content)
	if err != nil {
		c.Error(err, "decoding base64")
		return
	}

	if r.Branch == "" {
		r.Branch = c.Repo.Repository.DefaultBranch
	}
	treePath := c.Params("*")
	err = c.Repo.Repository.UpdateRepoFile(
		c.User,
		db.UpdateRepoFileOptions{
			OldBranch:   c.Repo.Repository.DefaultBranch,
			NewBranch:   r.Branch,
			OldTreeName: treePath,
			NewTreeName: treePath,
			Message:     r.Message,
			Content:     string(content),
		},
	)
	if err != nil {
		c.Error(err, "updating repository file")
		return
	}

	repoPath := repoutil.RepositoryPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	commit, err := gitRepo.CatFileCommit(r.Branch)
	if err != nil {
		c.Error(err, "get file commit")
		return
	}

	entry, err := commit.TreeEntry(treePath)
	if err != nil {
		c.Error(err, "get tree entry")
		return
	}

	apiContent, err := toRepoContent(c, r.Branch, treePath, commit, entry)
	if err != nil {
		c.Error(err, "convert to *repoContent")
		return
	}

	apiCommit, err := gitCommitToAPICommit(commit, c)
	if err != nil {
		c.Error(err, "convert to *api.Commit")
		return
	}

	c.JSON(
		http.StatusCreated,
		map[string]any{
			"content": apiContent,
			"commit":  apiCommit,
		},
	)
}
