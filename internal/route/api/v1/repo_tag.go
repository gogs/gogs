package v1

import (
	"net/http"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

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

// POST /repos/:username/:reponame/tags
func createTag(c *apiContext, r createReferenceRequest) {
	commit := createReference(c, git.RefsTags, r)
	if commit == nil {
		return
	}
	tag := &database.Tag{RepoPath: c.Repo.Repository.RepoPath(), Name: r.Name}
	c.JSON(http.StatusCreated, toTag(tag, commit))
}
