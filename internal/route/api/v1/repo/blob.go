package repo

import (
	"encoding/base64"
	"fmt"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/gitutil"
)

func GetRepoGitBlob(c *context.APIContext) {
	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	sha := c.Params(":sha")
	blob, err := gitRepo.CatFileBlob(sha)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get blob")
	}

	type repoGitBlob struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		URL      string `json:"url"`
		Sha      string `json:"sha"`
		Size     int64  `json:"size"`
	}

	content, err := blob.Blob().Bytes()
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get blob content")
	}

	c.JSONSuccess(&repoGitBlob{
		Content:  base64.StdEncoding.EncodeToString(content),
		Encoding: "base64",
		URL:      fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", c.BaseURL, c.Params(":username"), c.Params(":reponame"), sha),
		Sha:      sha,
		Size:     blob.Size(),
	})
}
