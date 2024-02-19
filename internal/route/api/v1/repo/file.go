// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/route/repo"
)

func GetRawFile(c *context.APIContext) {
	if !c.Repo.HasAccess() {
		c.NotFound()
		return
	}

	if c.Repo.Repository.IsBare {
		c.NotFound()
		return
	}

	blob, err := c.Repo.Commit.Blob(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get blob")
		return
	}
	if err = repo.ServeBlob(c.Context, blob); err != nil {
		c.Error(err, "serve blob")
	}
}

func GetArchive(c *context.APIContext) {
	repoPath := database.RepoPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}
	c.Repo.GitRepo = gitRepo

	repo.Download(c.Context)
}

func GetEditorconfig(c *context.APIContext) {
	ec, err := c.Repo.Editorconfig()
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get .editorconfig")
		return
	}

	fileName := c.Params("filename")
	def, err := ec.GetDefinitionForFilename(fileName)
	if err != nil {
		c.Error(err, "get definition for filename")
		return
	}
	if def == nil {
		c.NotFound()
		return
	}
	c.JSONSuccess(def)
}
