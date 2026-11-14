package v1

func adminCreateRepo(c *apiContext, form createRepoRequest) {
	owner := getUserByParams(c)
	if c.Written() {
		return
	}

	createUserRepo(c, owner, form)
}
