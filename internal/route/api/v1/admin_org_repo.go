package v1

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func getRepositoryByParams(c *context.APIContext) *database.Repository {
	repo, err := database.GetRepositoryByName(c.Org.Team.OrgID, c.Params(":reponame"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by name")
		return nil
	}
	return repo
}

func adminAddTeamRepository(c *context.APIContext) {
	repo := getRepositoryByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.AddRepository(repo); err != nil {
		c.Error(err, "add repository")
		return
	}

	c.NoContent()
}

func adminRemoveTeamRepository(c *context.APIContext) {
	repo := getRepositoryByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.RemoveRepository(repo.ID); err != nil {
		c.Error(err, "remove repository")
		return
	}

	c.NoContent()
}
