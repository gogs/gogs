// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/routes/repo"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Contents#download-raw-content
func GetRawFile(c *context.APIContext) {
	if !c.Repo.HasAccess() {
		c.Status(404)
		return
	}

	if c.Repo.Repository.IsBare {
		c.Status(404)
		return
	}

	blob, err := c.Repo.Commit.GetBlobByPath(c.Repo.TreePath)
	if err != nil {
		if git.IsErrNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetBlobByPath", err)
		}
		return
	}
	if err = repo.ServeBlob(c.Context, blob); err != nil {
		c.Error(500, "ServeBlob", err)
	}
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories-Contents#download-archive
func GetArchive(c *context.APIContext) {
	repoPath := models.RepoPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		c.Error(500, "OpenRepository", err)
		return
	}
	c.Repo.GitRepo = gitRepo

	repo.Download(c.Context)
}

func GetEditorconfig(c *context.APIContext) {
	ec, err := c.Repo.GetEditorconfig()
	if err != nil {
		if git.IsErrNotExist(err) {
			c.Error(404, "GetEditorconfig", err)
		} else {
			c.Error(500, "GetEditorconfig", err)
		}
		return
	}

	fileName := c.Params("filename")
	def := ec.GetDefinitionForFilename(fileName)
	if def == nil {
		c.Error(404, "GetDefinitionForFilename", err)
		return
	}
	c.JSON(200, def)
}
