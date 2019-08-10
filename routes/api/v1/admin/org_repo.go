// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
)

func GetRepositoryByParams(c *context.APIContext) *models.Repository {
	repo, err := models.GetRepositoryByName(c.Org.Team.OrgID, c.Params(":reponame"))
	if err != nil {
		c.NotFoundOrServerError("GetRepositoryByName", errors.IsRepoNotExist, err)
		return nil
	}
	return repo
}

func AddTeamRepository(c *context.APIContext) {
	repo := GetRepositoryByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.AddRepository(repo); err != nil {
		c.ServerError("AddRepository", err)
		return
	}

	c.NoContent()
}

func RemoveTeamRepository(c *context.APIContext) {
	repo := GetRepositoryByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.RemoveRepository(repo.ID); err != nil {
		c.ServerError("RemoveRepository", err)
		return
	}

	c.NoContent()
}
