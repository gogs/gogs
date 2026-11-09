package v1

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/repox"
)

type links struct {
	Git  string `json:"git"`
	Self string `json:"self"`
	HTML string `json:"html"`
}

type repoContent struct {
	Type            string `json:"type"`
	Target          string `json:"target,omitempty"`
	SubmoduleGitURL string `json:"submodule_git_url,omitempty"`
	Encoding        string `json:"encoding,omitempty"`
	Size            int64  `json:"size"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	Content         string `json:"content,omitempty"`
	Sha             string `json:"sha"`
	URL             string `json:"url"`
	GitURL          string `json:"git_url"`
	HTMLURL         string `json:"html_url"`
	DownloadURL     string `json:"download_url"`
	Links           links  `json:"_links"`
}

func toRepoContent(c *apiContext, ref, subpath string, commit *git.Commit, entry *git.TreeEntry) (*repoContent, error) {
	repoURL := fmt.Sprintf("%s/repos/%s/%s", c.BaseURL, c.Param("username"), c.Param("reponame"))
	selfURL := fmt.Sprintf("%s/contents/%s", repoURL, subpath)
	htmlURL := fmt.Sprintf("%s/src/%s/%s", repox.HTMLURL(c.Repo.Owner.Name, c.Repo.Repository.Name), ref, entry.Name())
	downloadURL := fmt.Sprintf("%s/raw/%s/%s", repox.HTMLURL(c.Repo.Owner.Name, c.Repo.Repository.Name), ref, entry.Name())

	content := &repoContent{
		Size:        entry.Size(c.Request().Context()),
		Name:        entry.Name(),
		Path:        subpath,
		Sha:         entry.ID().String(),
		URL:         selfURL,
		HTMLURL:     htmlURL,
		DownloadURL: downloadURL,
		Links: links{
			Self: selfURL,
			HTML: htmlURL,
		},
	}

	switch {
	case entry.IsBlob(), entry.IsExec():
		content.Type = "file"
		p, err := entry.Blob().Bytes(c.Request().Context())
		if err != nil {
			return nil, errors.Wrap(err, "get blob content")
		}
		content.Encoding = "base64"
		content.Content = base64.StdEncoding.EncodeToString(p)
		content.GitURL = fmt.Sprintf("%s/git/blobs/%s", repoURL, entry.ID().String())

	case entry.IsTree():
		content.Type = "dir"
		content.GitURL = fmt.Sprintf("%s/git/trees/%s", repoURL, entry.ID().String())

	case entry.IsSymlink():
		content.Type = "symlink"
		p, err := entry.Blob().Bytes(c.Request().Context())
		if err != nil {
			return nil, errors.Wrap(err, "get blob content")
		}
		content.Target = string(p)

	case entry.IsCommit():
		content.Type = "submodule"
		mod, err := commit.Submodule(c.Request().Context(), subpath)
		if err != nil {
			return nil, errors.Wrap(err, "get submodule")
		}
		content.SubmoduleGitURL = mod.URL

	default:
		panic("unreachable")
	}

	content.Links.Git = content.GitURL
	return content, nil
}

func getContents(c *apiContext) {
	repoPath := repox.RepositoryPath(c.Param("username"), c.Param("reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	ref := c.Query("ref")
	if ref == "" {
		ref = c.Repo.Repository.DefaultBranch
	}

	commit, err := gitRepo.CatFileCommit(c.Request().Context(), ref)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get commit")
		return
	}

	// 🚨 SECURITY: Prevent path traversal.
	treePath := pathx.Clean(c.Param("**"))
	entry, err := commit.TreeEntry(c.Request().Context(), treePath)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get tree entry")
		return
	}

	if !entry.IsTree() {
		content, err := toRepoContent(c, ref, treePath, commit, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", treePath)
			return
		}

		c.JSONSuccess(content)
		return
	}

	// The entry is a directory
	dir, err := gitRepo.LsTree(c.Request().Context(), entry.ID().String())
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get tree")
		return
	}

	entries, err := dir.Entries(c.Request().Context())
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "list entries")
		return
	}

	if len(entries) == 0 {
		c.JSONSuccess([]string{})
		return
	}

	contents := make([]*repoContent, 0, len(entries))
	for _, entry := range entries {
		subpath := path.Join(treePath, entry.Name())
		content, err := toRepoContent(c, ref, subpath, commit, entry)
		if err != nil {
			c.Errorf(err, "convert %q to repoContent", subpath)
			return
		}

		contents = append(contents, content)
	}
	c.JSONSuccess(contents)
}

// putContentsRequest is the API message for creating or updating a file.
type putContentsRequest struct {
	Message string `json:"message" validate:"required"`
	Content string `json:"content" validate:"required"`
	Branch  string `json:"branch"`
}

// PUT /repos/:username/:reponame/contents/*
func putContents(c *apiContext, r putContentsRequest) {
	content, err := base64.StdEncoding.DecodeString(r.Content)
	if err != nil {
		c.Error(err, "decoding base64")
		return
	}

	if r.Branch == "" {
		r.Branch = c.Repo.Repository.DefaultBranch
	}

	// 🚨 SECURITY: Prevent path traversal.
	treePath := pathx.Clean(c.Param("**"))

	err = c.Repo.Repository.UpdateRepoFile(
		c.Request().Context(),
		c.User,
		database.UpdateRepoFileOptions{
			OldBranch:   c.Repo.Repository.DefaultBranch,
			NewBranch:   r.Branch,
			OldTreeName: treePath,
			NewTreeName: treePath,
			Message:     r.Message,
			Content:     string(content),
		},
	)
	if err != nil {
		c.Error(err, "updating repository file")
		return
	}

	repoPath := repox.RepositoryPath(c.Param("username"), c.Param("reponame"))
	commit, err := git.RepoCatFileCommit(c.Request().Context(), repoPath, r.Branch)
	if err != nil {
		c.Error(err, "get file commit")
		return
	}

	entry, err := commit.TreeEntry(c.Request().Context(), treePath)
	if err != nil {
		c.Error(err, "get tree entry")
		return
	}

	apiContent, err := toRepoContent(c, r.Branch, treePath, commit, entry)
	if err != nil {
		c.Error(err, "convert to *repoContent")
		return
	}

	apiCommit, err := gitCommitToAPICommit(commit, c)
	if err != nil {
		c.Error(err, "convert to *api.Commit")
		return
	}

	c.JSON(
		http.StatusCreated,
		map[string]any{
			"content": apiContent,
			"commit":  apiCommit,
		},
	)
}
