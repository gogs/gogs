// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogs/git-module"

	"gogs.io/gogs/models"
	"gogs.io/gogs/pkg/context"
	"gogs.io/gogs/routes/repo"
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

	blob, err := c.Repo.Commit.GetBlobByPath(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrServerError("GetBlobByPath", git.IsErrNotExist, err)
		return
	}
	if err = repo.ServeBlob(c.Context, blob); err != nil {
		c.ServerError("ServeBlob", err)
	}
}

func GetArchive(c *context.APIContext) {
	repoPath := models.RepoPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		c.ServerError("OpenRepository", err)
		return
	}
	c.Repo.GitRepo = gitRepo

	repo.Download(c.Context)
}

func GetEditorconfig(c *context.APIContext) {
	ec, err := c.Repo.GetEditorconfig()
	if err != nil {
		c.NotFoundOrServerError("GetEditorconfig", git.IsErrNotExist, err)
		return
	}

	fileName := c.Params("filename")
	def := ec.GetDefinitionForFilename(fileName)
	if def == nil {
		c.NotFound()
		return
	}
	c.JSONSuccess(def)
}
