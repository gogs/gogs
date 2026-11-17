package v1

import (
	"gogs.io/gogs/internal/database"
)

func getRepositoryByParams(c *apiContext) *database.Repository {
	repos := database.Handle.Repositories()
	repo, err := repos.GetByName(c.Request().Context(), c.Org.Team.OrgID, c.Param("reponame"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by name")
		return nil
	}
	if err = repos.LoadAttributes(c.Request().Context(), repo); err != nil {
		c.Error(err, "load repository attributes")
		return nil
	}
	return repo
}

func adminAddTeamRepository(c *apiContext) {
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

func adminRemoveTeamRepository(c *apiContext) {
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
