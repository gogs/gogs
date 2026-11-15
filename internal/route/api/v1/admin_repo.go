package v1

import (
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func adminCreateRepo(c *apiContext, form createRepoRequest) {
	owner := getUserByParams(c)
	if c.Written() {
		return
	}

	createUserRepo(c, owner, form)
}

// adminListRepos lists a page of all repositories, including private ones, or
// of the repositories whose name or description matches the "q" query
// parameter.
func adminListRepos(c *apiContext) {
	pageSize := toAllowedPageSize(c.QueryInt("limit"))
	page := max(c.QueryInt("page"), 1)

	var (
		repos []*database.Repository
		total int64
		err   error
	)
	if keyword := c.Query("q"); keyword != "" {
		repos, total, err = database.SearchRepositoryByName(&database.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  "id ASC",
			Private:  true,
			Page:     page,
			PageSize: pageSize,
		})
	} else {
		repos, err = database.Repositories(page, pageSize)
		total = database.CountRepositories(true)
	}
	if err != nil {
		c.Error(err, "list repositories")
		return
	}

	results := make([]*types.Repository, len(repos))
	for i := range repos {
		// Loads the base repository of a fork, which the result includes.
		if err = repos[i].LoadAttributes(); err != nil {
			c.Error(err, "load repository attributes")
			return
		}
		results[i] = toRepository(repos[i], &types.RepositoryPermission{Admin: true, Push: true, Pull: true})
	}
	c.SetLinkHeader(int(total), pageSize)
	c.JSONSuccess(&results)
}
