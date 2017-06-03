// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
)

func GetRepositoryByParams(c *context.APIContext) *models.Repository {
	repo, err := models.GetRepositoryByName(c.Org.Team.OrgID, c.Params(":reponame"))
	if err != nil {
		if errors.IsRepoNotExist(err) {
			c.Status(404)
		} else {
			c.Error(500, "GetRepositoryByName", err)
		}
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
		c.Error(500, "AddRepository", err)
		return
	}

	c.Status(204)
}

func RemoveTeamRepository(c *context.APIContext) {
	repo := GetRepositoryByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.RemoveRepository(repo.ID); err != nil {
		c.Error(500, "RemoveRepository", err)
		return
	}

	c.Status(204)
}
