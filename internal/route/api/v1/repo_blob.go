package v1

import (
	"encoding/base64"
	"fmt"

	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/repox"
)

func repoGitBlob(c *apiContext) {
	repoPath := repox.RepositoryPath(c.Param("username"), c.Param("reponame"))
	sha := c.Param("sha")
	blob, err := git.RepoCatFileBlob(c.Request().Context(), repoPath, sha)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get blob")
		return
	}

	type repoGitBlob struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		URL      string `json:"url"`
		SHA      string `json:"sha"`
		Size     int64  `json:"size"`
	}

	content, err := blob.Blob().Bytes(c.Request().Context())
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get blob content")
		return
	}

	c.JSONSuccess(&repoGitBlob{
		Content:  base64.StdEncoding.EncodeToString(content),
		Encoding: "base64",
		URL:      fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", c.BaseURL, c.Param("username"), c.Param("reponame"), sha),
		SHA:      sha,
		Size:     blob.Size(c.Request().Context()),
	})
}
