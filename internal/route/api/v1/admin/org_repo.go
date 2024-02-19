// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func GetRepositoryByParams(c *context.APIContext) *database.Repository {
	repo, err := database.GetRepositoryByName(c.Org.Team.OrgID, c.Params(":reponame"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by name")
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
		c.Error(err, "add repository")
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
		c.Error(err, "remove repository")
		return
	}

	c.NoContent()
}
