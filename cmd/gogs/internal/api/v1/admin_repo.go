package v1

import "gogs.io/gogs/internal/database"

func adminCreateRepo(c *apiContext, repoCreation database.RepoCreationOptions, repoDeletion database.DeleteRepoOptions, form createRepoRequest) {
	owner := getUserByParams(c)
	if c.Written() {
		return
	}

	createUserRepo(c, repoCreation, repoDeletion, owner, form)
}
