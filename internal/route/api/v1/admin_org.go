package v1

func adminCreateOrg(c *apiContext, form createOrgRequest) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	if u.IsOrganization() {
		c.NotFound()
		return
	}
	createOrgForUser(c, form, u)
}
