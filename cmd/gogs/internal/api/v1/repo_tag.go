package v1

import (
	"net/http"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

func listTags(c *apiContext, repository conf.RepositoryOpts) {
	tags, err := c.Repo.Repository.GetTags(c.Request().Context(), repository.Root)
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
func createTag(c *apiContext, repository conf.RepositoryOpts, push database.PushOptions, r createReferenceRequest) {
	commit, err := createReference(c, repository.Root, push, git.RefsTags, r)
	if err != nil {
		c.Error(err, "create tag")
		return
	} else if commit == nil {
		return
	}
	tag := &database.Tag{RepoPath: c.Repo.Repository.RepoPath(repository.Root), Name: r.Name}
	c.JSON(http.StatusCreated, toTag(tag, commit))
}
