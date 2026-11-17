package v1

func listTags(c *apiContext) {
	tags, err := c.Repo.Repository.GetTags(c.Request().Context())
	if err != nil {
		c.Error(err, "get tags")
		return
	}

	apiTags := make([]*tag, len(tags))
	for i := range tags {
		commit, err := tags[i].GetCommit(c.Request().Context())
		if err != nil {
			c.Error(err, "get commit")
			return
		}
		apiTags[i] = toTag(tags[i], commit)
	}

	c.JSONSuccess(&apiTags)
}
