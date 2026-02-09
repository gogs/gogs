package repo

import (
	"net/http"
	"strings"
	"time"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/route/api/v1/apitype"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

const mediaApplicationSHA = "application/vnd.gogs.sha"

// GetAllCommits returns a slice of commits starting from HEAD.
func GetAllCommits(c *context.APIContext) {
	// Get pagesize, set default if it is not specified.
	pageSize := c.QueryInt("pageSize")
	if pageSize == 0 {
		pageSize = 30
	}

	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	// The response object returned as JSON
	result := make([]*apitype.Commit, 0, pageSize)
	commits, err := gitRepo.Log("HEAD", git.LogOptions{MaxCount: pageSize})
	if err != nil {
		c.Error(err, "git log")
	}

	for _, commit := range commits {
		apiCommit, err := gitCommitToAPICommit(commit, c)
		if err != nil {
			c.Error(err, "convert git commit to api commit")
			return
		}
		result = append(result, apiCommit)
	}

	c.JSONSuccess(result)
}

// GetSingleCommit will return a single Commit object based on the specified SHA.
func GetSingleCommit(c *context.APIContext) {
	if strings.Contains(c.Req.Header.Get("Accept"), mediaApplicationSHA) {
		c.SetParams("*", c.Params(":sha"))
		GetReferenceSHA(c)
		return
	}

	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}
	commit, err := gitRepo.CatFileCommit(c.Params(":sha"))
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get commit")
		return
	}

	apiCommit, err := gitCommitToAPICommit(commit, c)
	if err != nil {
		c.Error(err, "convert git commit to api commit")
	}
	c.JSONSuccess(apiCommit)
}

func GetReferenceSHA(c *context.APIContext) {
	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	ref := c.Params("*")
	refType := 0 // 0-unknown, 1-branch, 2-tag
	if after, ok := strings.CutPrefix(ref, git.RefsHeads); ok {
		ref = after
		refType = 1
	} else if after, ok := strings.CutPrefix(ref, git.RefsTags); ok {
		ref = after
		refType = 2
	} else {
		if gitRepo.HasBranch(ref) {
			refType = 1
		} else if gitRepo.HasTag(ref) {
			refType = 2
		} else {
			c.NotFound()
			return
		}
	}

	var sha string
	switch refType {
	case 1:
		sha, err = gitRepo.BranchCommitID(ref)
	case 2:
		sha, err = gitRepo.TagCommitID(ref)
	}
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get reference commit ID")
		return
	}
	c.PlainText(http.StatusOK, sha)
}

// gitCommitToApiCommit is a helper function to convert git commit object to API commit.
func gitCommitToAPICommit(commit *git.Commit, c *context.APIContext) (*apitype.Commit, error) {
	// Retrieve author and committer information
	var apiAuthor, apiCommitter *apitype.User
	author, err := database.Handle.Users().GetByEmail(c.Req.Context(), commit.Author.Email)
	if err != nil && !database.IsErrUserNotExist(err) {
		return nil, err
	} else if err == nil {
		apiAuthor = convert.ToUser(author)
	}

	// Save one query if the author is also the committer
	if commit.Committer.Email == commit.Author.Email {
		apiCommitter = apiAuthor
	} else {
		committer, err := database.Handle.Users().GetByEmail(c.Req.Context(), commit.Committer.Email)
		if err != nil && !database.IsErrUserNotExist(err) {
			return nil, err
		} else if err == nil {
			apiCommitter = convert.ToUser(committer)
		}
	}

	// Retrieve parent(s) of the commit
	apiParents := make([]*apitype.CommitMeta, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, _ := commit.ParentID(i)
		apiParents[i] = &apitype.CommitMeta{
			URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/commits/" + sha.String(),
			SHA: sha.String(),
		}
	}

	return &apitype.Commit{
		CommitMeta: &apitype.CommitMeta{
			URL: conf.Server.ExternalURL + c.Link[1:],
			SHA: commit.ID.String(),
		},
		HTMLURL: c.Repo.Repository.HTMLURL() + "/commits/" + commit.ID.String(),
		RepoCommit: &apitype.RepoCommit{
			URL: conf.Server.ExternalURL + c.Link[1:],
			Author: &apitype.CommitUser{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
				Date:  commit.Author.When.Format(time.RFC3339),
			},
			Committer: &apitype.CommitUser{
				Name:  commit.Committer.Name,
				Email: commit.Committer.Email,
				Date:  commit.Committer.When.Format(time.RFC3339),
			},
			Message: commit.Summary(),
			Tree: &apitype.CommitMeta{
				URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/tree/" + commit.ID.String(),
				SHA: commit.ID.String(),
			},
		},
		Author:    apiAuthor,
		Committer: apiCommitter,
		Parents:   apiParents,
	}, nil
}
