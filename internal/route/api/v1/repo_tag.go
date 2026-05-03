package v1

import (
	"net/http"
	"strings"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitx"
	"gogs.io/gogs/internal/repox"
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

type createTagRequest struct {
	Name   string `json:"name" binding:"Required"`
	Commit string `json:"commit" binding:"Required"`
}

func createTag(c *context.APIContext, r createTagRequest) {
	repoPath := repox.RepositoryPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	// Validate that the commit exists.
	commit, err := gitRepo.CatFileCommit(r.Commit)
	if err != nil {
		// git-module returns ErrRevisionNotExist for unknown refs, but for
		// well-formed SHAs that are simply absent the underlying git cat-file
		// command exits with status 128 without a typed error. Both cases
		// indicate that the commit does not exist.
		if gitx.IsErrRevisionNotExist(err) || strings.Contains(err.Error(), "exit status 128") {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get commit")
		}
		return
	}

	// 🚨 SECURITY: Trim any leading '-' to prevent command line argument injection.
	// This must happen before the duplicate check so that both use the same name.
	tagName := strings.TrimLeft(r.Name, "-")

	// Validate that the tag name is not already taken.
	if gitRepo.HasTag(tagName) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errTagAlreadyExists(tagName))
		return
	}

	if err = gitRepo.CreateTag(tagName, r.Commit); err != nil {
		c.Error(err, "create tag")
		return
	}

	dbTag := &database.Tag{
		RepoPath: repoPath,
		Name:     tagName,
	}

	c.JSON(http.StatusCreated, toTag(dbTag, commit))
}
