// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"time"

	"github.com/gogs/git-module"
	"github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/repoutil"
)

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
		content, err := toRepoContent(subpath, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", subpath)
			return
		}

		contents = append(contents, content)
	}
	c.JSONSuccess(contents)
}

type PutContentRequest struct {
	Message  string           `json:"accept"`
	Content  string           `json:"message"`
	Sha      string           `json:"sha"`
	Branch   string           `json:"branch"`
	Commiter *gogs.CommitUser `json:"commiter"`
	Author   *gogs.CommitUser `json:"author"`
}

type commitPayload struct {
	Sha      string           `json:"sha"`
	NodeID   string           `json:"node_id"`
	Url      string           `json:"url"`
	HtmlUrl  string           `json:"html_url"`
	Commiter *gogs.CommitUser `json:"commiter"`
	Author   *gogs.CommitUser `json:"author"`
	Message  string           `json:"message"`
}

type contentPayload struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Sha         string `json:"sha"`
	Size        string `json:"size"`
	Url         string `json:"url"`
	HtmlUrl     string `json:"html_url"`
	GitUrl      string `json:"git_url"`
	Type        string `json:"type"`
	DownloadUrl string `json:"download_url"`
}

type putContentResponse struct {
	Content contentPayload
	Commit  commitPayload
}

func ToJsonPayload(s *git.Signature) *gogs.CommitUser {
	return &gogs.CommitUser{
		Name:  s.Name,
		Email: s.Email,
		Date:  s.When.String(),
	}
}

func PutContents(c *context.APIContext, form *PutContentRequest) {
	repoPath := repoutil.RepositoryPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	content, err := base64.StdEncoding.DecodeString(form.Content)
	if err != nil {
		c.Error(err, "decoding base64")
		return
	}
	treePath := c.Params("*")
	filename := filepath.Join(gitRepo.Path(), treePath)

	// checkout
	var branch string = form.Branch
	if form.Branch == "" {
		branch = c.Repo.Repository.DefaultBranch
	}
	gitRepo.Checkout(branch)

	// add
	localPath := c.Repo.Repository.LocalCopyPath()
	ioutil.WriteFile(path.Join(localPath, filename), content, 0644)

	gitRepo.Add(git.AddOptions{
		Pathsepcs: []string{filename},
	})

	commitDate, err := time.Parse(time.RFC3339, form.Commiter.Date)
	if err != nil {
		commitDate = time.Now()
	}

	var commiter = git.Signature{
		Name:  c.User.FullName,
		Email: c.User.Email,
		When:  commitDate,
	}

	if form.Commiter != nil {
		commiter.Name = form.Commiter.Name
		commiter.Email = form.Commiter.Email
	}

	authorDate, err := time.Parse(time.RFC3339, form.Author.Date)
	if err != nil {
		authorDate = time.Now()
	}

	var author = git.Signature{
		Name:  c.User.FullName,
		Email: c.User.Email,
		When:  authorDate,
	}

	if form.Commiter != nil {
		author.Name = form.Author.Name
		author.Email = form.Author.Email
	}

	// commit
	gitRepo.Commit(&commiter, form.Message, git.CommitOptions{
		Author: &author,
	})

	commit, err := gitRepo.BranchCommit(branch)
	if err != nil {
		c.Error(err, "git branch commit not found")
		return
	}

	responseObj := putContentResponse{
		Content: contentPayload{},
		Commit: commitPayload{
			Sha:      commit.ID.String(),
			Url:      c.Req.RequestURI,
			Commiter: ToJsonPayload(&commiter),
			Author:   ToJsonPayload(&author),
			Message:  commit.Message,
		},
	}

	c.JSONSuccess(responseObj)
}
