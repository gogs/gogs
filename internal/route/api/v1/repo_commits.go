package v1

import (
	"net/http"
	"strings"
	"time"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/route/api/v1/types"
)

const mediaApplicationSHA = "application/vnd.gogs.sha"

// getAllCommits returns a page of the commits reachable from the "sha" query
// parameter, a branch, a tag or a commit, which defaults to HEAD.
func getAllCommits(c *apiContext) {
	pageSize := c.QueryInt("pageSize")
	if pageSize <= 0 {
		pageSize = 30
	}
	pageSize = toAllowedPageSize(pageSize)
	page := max(c.QueryInt("page"), 1)

	result := make([]*types.Commit, 0, pageSize)
	sha := c.Query("sha")
	if sha == "" {
		if c.Repo.Repository.IsEmpty {
			c.JSONSuccess(result)
			return
		}
		sha = "HEAD"
	}

	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}
	start, err := gitRepo.CatFileCommit(c.Request().Context(), sha)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get commit")
		return
	}
	total, err := start.CommitsCount(c.Request().Context())
	if err != nil {
		c.Error(err, "count commits")
		return
	}
	commits, err := gitRepo.Log(c.Request().Context(), start.ID.String(), git.LogOptions{
		MaxCount: pageSize,
		Skip:     (page - 1) * pageSize,
	})
	if err != nil {
		c.Error(err, "git log")
		return
	}

	for _, commit := range commits {
		apiCommit, err := gitCommitToAPICommit(commit, c)
		if err != nil {
			c.Error(err, "convert git commit to api commit")
			return
		}
		result = append(result, apiCommit)
	}

	c.SetLinkHeader(int(total), pageSize)
	c.JSONSuccess(result)
}

// getSingleCommit will return a single Commit object based on the specified SHA.
func getSingleCommit(c *apiContext) {
	if strings.Contains(c.Request().Header.Get("Accept"), mediaApplicationSHA) {
		serveReferenceSHA(c, c.Param("sha"))
		return
	}

	commit, err := git.RepoCatFileCommit(c.Request().Context(), c.Repo.Repository.RepoPath(), c.Param("sha"))
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get commit")
		return
	}

	apiCommit, err := gitCommitToAPICommit(commit, c)
	if err != nil {
		c.Error(err, "convert git commit to api commit")
	}
	c.JSONSuccess(apiCommit)
}

func getReferenceSHA(c *apiContext) {
	serveReferenceSHA(c, c.Param("**"))
}

func serveReferenceSHA(c *apiContext, ref string) {
	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	refType := 0 // 0-unknown, 1-branch, 2-tag
	if after, ok := strings.CutPrefix(ref, git.RefsHeads); ok {
		ref = after
		refType = 1
	} else if after, ok := strings.CutPrefix(ref, git.RefsTags); ok {
		ref = after
		refType = 2
	} else {
		if gitRepo.HasBranch(c.Request().Context(), ref) {
			refType = 1
		} else if gitRepo.HasTag(c.Request().Context(), ref) {
			refType = 2
		} else {
			c.NotFound()
			return
		}
	}

	var sha string
	switch refType {
	case 1:
		sha, err = gitRepo.BranchCommitID(c.Request().Context(), ref)
	case 2:
		sha, err = gitRepo.TagCommitID(c.Request().Context(), ref)
	}
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get reference commit ID")
		return
	}
	c.PlainText(http.StatusOK, sha)
}

// gitCommitToAPICommit converts a Git commit to an API commit.
func gitCommitToAPICommit(commit *git.Commit, c *apiContext) (*types.Commit, error) {
	// Retrieve author and committer information
	var apiAuthor, apiCommitter *types.User
	author, err := database.Handle.Users().GetByEmail(c.Request().Context(), commit.Author.Email)
	if err != nil && !database.IsErrUserNotExist(err) {
		return nil, err
	} else if err == nil {
		apiAuthor = toUser(author)
	}

	// Save one query if the author is also the committer
	if commit.Committer.Email == commit.Author.Email {
		apiCommitter = apiAuthor
	} else {
		committer, err := database.Handle.Users().GetByEmail(c.Request().Context(), commit.Committer.Email)
		if err != nil && !database.IsErrUserNotExist(err) {
			return nil, err
		} else if err == nil {
			apiCommitter = toUser(committer)
		}
	}

	// Retrieve parent(s) of the commit
	apiParents := make([]*types.CommitMeta, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, _ := commit.ParentID(i)
		apiParents[i] = &types.CommitMeta{
			URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/commits/" + sha.String(),
			SHA: sha.String(),
		}
	}

	return &types.Commit{
		CommitMeta: &types.CommitMeta{
			URL: conf.Server.ExternalURL + c.Link[1:],
			SHA: commit.ID.String(),
		},
		HTMLURL: c.Repo.Repository.HTMLURL() + "/commits/" + commit.ID.String(),
		RepoCommit: &types.RepoCommit{
			URL: conf.Server.ExternalURL + c.Link[1:],
			Author: &types.CommitUser{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
				Date:  commit.Author.When.Format(time.RFC3339),
			},
			Committer: &types.CommitUser{
				Name:  commit.Committer.Name,
				Email: commit.Committer.Email,
				Date:  commit.Committer.When.Format(time.RFC3339),
			},
			Message: commit.Summary(),
			Tree: &types.CommitMeta{
				URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/tree/" + commit.ID.String(),
				SHA: commit.ID.String(),
			},
		},
		Author:    apiAuthor,
		Committer: apiCommitter,
		Parents:   apiParents,
	}, nil
}
