package v1

import (
	"fmt"

	"gogs.io/gogs/internal/git"
)

func getRepoGitTree(c *apiContext) {
	sha := c.Param("sha")
	tree, err := git.RepoLsTree(c.Request().Context(), c.Repo.Repository.RepoPath(), sha)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get tree")
		return
	}

	entries, err := tree.Entries(c.Request().Context())
	if err != nil {
		c.Error(err, "list entries")
		return
	}

	type repoGitTreeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		Size int64  `json:"size"`
		Sha  string `json:"sha"`
		URL  string `json:"url"`
	}
	type repoGitTree struct {
		Sha  string              `json:"sha"`
		URL  string              `json:"url"`
		Tree []*repoGitTreeEntry `json:"tree"`
	}

	treesURL := fmt.Sprintf("%s/repos/%s/%s/git/trees", c.BaseURL, c.Param("username"), c.Param("reponame"))

	if len(entries) == 0 {
		c.JSONSuccess(&repoGitTree{
			Sha: sha,
			URL: fmt.Sprintf(treesURL+"/%s", sha),
		})
		return
	}

	children := make([]*repoGitTreeEntry, 0, len(entries))
	for _, entry := range entries {
		children = append(children, &repoGitTreeEntry{
			Path: entry.Name(),
			Mode: fmt.Sprintf("%06o", entry.Mode()),
			Type: string(entry.Type()),
			Size: entry.Size(c.Request().Context()),
			Sha:  entry.ID().String(),
			URL:  fmt.Sprintf(treesURL+"/%s", entry.ID().String()),
		})
	}
	c.JSONSuccess(&repoGitTree{
		Sha:  c.Param("sha"),
		URL:  fmt.Sprintf(treesURL+"/%s", sha),
		Tree: children,
	})
}
