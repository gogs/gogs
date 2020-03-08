// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"fmt"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
)

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
	Links           Links  `json:"_links"`
}

type Links struct {
	Git  string `json:"git"`
	Self string `json:"self"`
	HTML string `json:"html"`
}

func GetContents(c *context.APIContext) {
	treeEntry, err := c.Repo.Commit.TreeEntry(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrServerError("get tree entry", gitutil.IsErrRevisionNotExist, err)
		return
	}
	username := c.Params(":username")
	reponame := c.Params(":reponame")

	// TODO: figure out the best way to do this
	// :base-url/:username/:project/raw/:refs/:path
	templateDownloadURL := "%s/%s/%s/raw/%s"
	// :base-url/repos/:username/:project/contents/:path
	templateSelfLink := "%s/repos/%s/%s/contents/%s"
	// :baseurl/repos/:username/:project/git/trees/:sha
	templateGitURLLink := "%s/repos/%s/%s/trees/%s"
	// :baseurl/repos/:username/:project/tree/:sha
	templateHTMLLLink := "%s/repos/%s/%s/tree/%s"

	gitURL := fmt.Sprintf(templateGitURLLink, c.BaseURL, username, reponame, treeEntry.ID().String())
	htmlURL := fmt.Sprintf(templateHTMLLLink, c.BaseURL, username, reponame, treeEntry.ID().String())
	selfURL := fmt.Sprintf(templateSelfLink, c.BaseURL, username, reponame, c.Repo.TreePath)

	// TODO(unknwon): Make a treeEntryToRepoContent helper.
	contents := &repoContent{
		Size:        treeEntry.Size(),
		Name:        treeEntry.Name(),
		Path:        c.Repo.TreePath,
		Sha:         treeEntry.ID().String(),
		URL:         selfURL,
		GitURL:      gitURL,
		HTMLURL:     htmlURL,
		DownloadURL: fmt.Sprintf(templateDownloadURL, c.BaseURL, username, reponame, c.Repo.TreePath),
		Links: Links{
			Git:  gitURL,
			Self: selfURL,
			HTML: htmlURL,
		},
	}

	// A tree entry can only be one of the following types:
	//   1. Tree (directory)
	//   2. SubModule
	//   3. SymLink
	//   4. Blob (file)
	if treeEntry.IsCommit() {
		// TODO(unknwon): submoduleURL is not set as current git-module doesn't handle it properly
		contents.Type = "submodule"
		c.JSONSuccess(contents)
		return

	} else if treeEntry.IsSymlink() {
		contents.Type = "symlink"
		blob, err := c.Repo.Commit.Blob(c.Repo.TreePath)
		if err != nil {
			c.ServerError("GetBlobByPath", err)
			return
		}
		p, err := blob.Bytes()
		if err != nil {
			c.ServerError("Data", err)
			return
		}
		contents.Target = string(p)
		c.JSONSuccess(contents)
		return

	} else if treeEntry.IsBlob() {
		blob, err := c.Repo.Commit.Blob(c.Repo.TreePath)
		if err != nil {
			c.ServerError("GetBlobByPath", err)
			return
		}
		p, err := blob.Bytes()
		if err != nil {
			c.ServerError("Data", err)
			return
		}
		contents.Content = base64.StdEncoding.EncodeToString(p)
		contents.Type = "file"
		c.JSONSuccess(contents)
		return
	}

	// TODO: treeEntry.IsExec()

	// treeEntry is a directory
	dirTree, err := c.Repo.GitRepo.LsTree(treeEntry.ID().String())
	if err != nil {
		c.NotFoundOrServerError("get tree", gitutil.IsErrRevisionNotExist, err)
		return
	}

	entries, err := dirTree.Entries()
	if err != nil {
		c.NotFoundOrServerError("list entries", gitutil.IsErrRevisionNotExist, err)
		return
	}

	if len(entries) == 0 {
		c.JSONSuccess([]string{})
		return
	}

	var results = make([]*repoContent, 0, len(entries))
	for _, entry := range entries {
		gitURL := fmt.Sprintf(templateGitURLLink, c.BaseURL, username, reponame, entry.ID().String())
		htmlURL := fmt.Sprintf(templateHTMLLLink, c.BaseURL, username, reponame, entry.ID().String())
		selfURL := fmt.Sprintf(templateSelfLink, c.BaseURL, username, reponame, c.Repo.TreePath)
		var contentType string
		if entry.IsTree() {
			contentType = "dir"
		} else if entry.IsCommit() {
			// TODO(unknwon): submoduleURL is not set as current git-module doesn't handle it properly
			contentType = "submodule"
		} else if entry.IsSymlink() {
			contentType = "symlink"
			blob, err := c.Repo.Commit.Blob(c.Repo.TreePath)
			if err != nil {
				c.ServerError("GetBlobByPath", err)
				return
			}
			p, err := blob.Bytes()
			if err != nil {
				c.ServerError("Data", err)
				return
			}
			contents.Target = string(p)
		} else {
			contentType = "file"
		}

		results = append(results, &repoContent{
			Type:        contentType,
			Size:        entry.Size(),
			Name:        entry.Name(),
			Path:        c.Repo.TreePath,
			Sha:         entry.ID().String(),
			URL:         selfURL,
			GitURL:      gitURL,
			HTMLURL:     htmlURL,
			DownloadURL: fmt.Sprintf(templateDownloadURL, c.BaseURL, username, reponame, c.Repo.TreePath),
			Links: Links{
				Git:  gitURL,
				Self: selfURL,
				HTML: htmlURL,
			},
		})
	}
	c.JSONSuccess(results)
}
