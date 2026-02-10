package v1

import (
	"gogs.io/gogs/internal/context"
)

func listTags(c *context.APIContext) {
	tags, err := c.Repo.Repository.GetTags()
	if err != nil {
		c.Error(err, "get tags")
		return
	}

	apiTags := make([]*tag, len(tags))
	for i := range tags {
		commit, err := tags[i].GetCommit()
		if err != nil {
			c.Error(err, "get commit")
			return
		}
		apiTags[i] = toTag(tags[i], commit)
	}

	c.JSONSuccess(&apiTags)
}
