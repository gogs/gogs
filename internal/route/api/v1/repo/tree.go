package repo

import (
	"fmt"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

type repoGitTreeResponse struct {
	Sha  string         `json:"sha"`
	URL  string         `json:"url"`
	Tree []*repoGitTree `json:"tree"`
}

type repoGitTree struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
	Sha  string `json:"sha"`
	URL  string `json:"url"`
}

func GetRepoGitTree(c *context.APIContext) {
	repoPath := db.RepoPath(c.Params(":username"), c.Params(":reponame"))
	gitTree, err := c.Repo.GitRepo.GetTree(c.Params(":sha"))
	if err != nil {
		c.NotFoundOrServerError("GetRepoTree", git.IsErrNotExist, err)
		return
	}
	entries, err := gitTree.ListEntries()
	if err != nil {
		c.ServerError("GetRepoTree", err)
		return
	}
	var children []*repoGitTree
	var mode string
	templateURL := `%s/repos/%s/%s/git/trees`
	templateURL = fmt.Sprintf(templateURL, c.BaseURL, c.Params(":username"), c.Params(":reponame"))
	if entries == nil {
		res := &repoGitTreeResponse{
			Sha: c.Params(":sha"),
			URL: fmt.Sprintf(templateURL+"/%s", c.Params(":sha")),
		}
		c.JSONSuccess(res)
	}
	for _, entry := range entries {
		switch entry.Type {
		case git.ObjectCommit:
			mode = "160000"
		case git.ObjectTree:
			mode = "040000"
		case git.ObjectBlob:
			mode = "120000"
		case git.ObjectTag:
			mode = "100644"
		default:
			mode = ""
		}
		children = append(children, &repoGitTree{
			Path: repoPath,
			Mode: mode,
			Type: string(entry.Type),
			Size: entry.Size(),
			Sha:  entry.ID.String(),
			URL:  fmt.Sprintf(templateURL+"/%s", entry.ID.String()),
		})
	}
	results := &repoGitTreeResponse{
		Sha:  c.Params(":sha"),
		URL:  fmt.Sprintf(templateURL+"/%s", c.Params(":sha")),
		Tree: children,
	}
	c.JSONSuccess(results)
}
