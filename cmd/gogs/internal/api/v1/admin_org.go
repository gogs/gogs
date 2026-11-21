package v1

func adminCreateOrg(c *apiContext, form createOrgRequest) {
	createOrgForUser(c, form, getUserByParams(c))
}
