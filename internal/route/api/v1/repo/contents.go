// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"fmt"
	"path"

	"github.com/gogs/git-module"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
)

func GetContents(c *context.APIContext) {
	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.ServerError("open repository", err)
		return
	}

	ref := c.Query("ref")
	if ref == "" {
		ref = c.Repo.Repository.DefaultBranch
	}

	commit, err := gitRepo.CatFileCommit(ref)
	if err != nil {
		c.NotFoundOrServerError("get commit", gitutil.IsErrRevisionNotExist, err)
		return
	}

	treePath := c.Params("*")
	entry, err := commit.TreeEntry(treePath)
	if err != nil {
		c.NotFoundOrServerError("get tree entry", gitutil.IsErrRevisionNotExist, err)
		return
	}

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

	toRepoContent := func(subpath string, entry *git.TreeEntry) (*repoContent, error) {
		repoURL := fmt.Sprintf("%s/repos/%s/%s", c.BaseURL, c.Params(":username"), c.Params(":reponame"))
		selfURL := fmt.Sprintf("%s/contents/%s", repoURL, subpath)
		htmlURL := fmt.Sprintf("%s/src/%s/%s", c.Repo.Repository.HTMLURL(), ref, entry.Name())
		downloadURL := fmt.Sprintf("%s/raw/%s/%s", c.Repo.Repository.HTMLURL(), ref, entry.Name())

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

	if !entry.IsTree() {
		content, err := toRepoContent(treePath, entry)
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
		c.NotFoundOrServerError("get tree", gitutil.IsErrRevisionNotExist, err)
		return
	}

	entries, err := dir.Entries()
	if err != nil {
		c.NotFoundOrServerError("list entries", gitutil.IsErrRevisionNotExist, err)
		return
	}

	if len(entries) == 0 {
		c.JSONSuccess([]string{})
		return
	}

	contents := make([]*repoContent, 0, len(entries))
	for _, entry := range entries {
		subpath := path.Join(treePath, entry.Name())
		content, err := toRepoContent(subpath, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", subpath)
			return
		}

		contents = append(contents, content)
	}
	c.JSONSuccess(contents)
}
