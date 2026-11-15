package v1

import (
	"gogs.io/gogs/internal/route/api/v1/types"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories#get-branch
func getBranch(c *apiContext) {
	branch, err := c.Repo.Repository.GetBranch(c.Request().Context(), c.Param("**"))
	if err != nil {
		c.NotFoundOrError(err, "get branch")
		return
	}

	commit, err := branch.GetCommit(c.Request().Context())
	if err != nil {
		c.Error(err, "get commit")
		return
	}

	c.JSONSuccess(toBranch(branch, commit))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#list-branches
// listBranches lists all branches, or a page of them when the "page" or
// "limit" query parameter is given.
func listBranches(c *apiContext) {
	branches, err := c.Repo.Repository.GetBranches(c.Request().Context())
	if err != nil {
		c.Error(err, "get branches")
		return
	}

	if c.Query("page") != "" || c.Query("limit") != "" {
		pageSize := toAllowedPageSize(c.QueryInt("limit"))
		page := max(c.QueryInt("page"), 1)
		c.SetLinkHeader(len(branches), pageSize)
		start := min((page-1)*pageSize, len(branches))
		branches = branches[start:min(start+pageSize, len(branches))]
	}

	apiBranches := make([]*types.RepositoryBranch, len(branches))
	for i := range branches {
		commit, err := branches[i].GetCommit(c.Request().Context())
		if err != nil {
			c.Error(err, "get commit")
			return
		}
		apiBranches[i] = toBranch(branches[i], commit)
	}

	c.JSONSuccess(&apiBranches)
}
