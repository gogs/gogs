// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"
	"strings"
	"time"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
)

func GetSingleCommit(c *context.APIContext) {
	if strings.Contains(c.Req.Header.Get("Accept"), api.MediaApplicationSHA) {
		c.SetParams("*", c.Params(":sha"))
		GetReferenceSHA(c)
		return
	}

	gitRepo, err := git.OpenRepository(c.Repo.Repository.RepoPath())
	if err != nil {
		c.ServerError("OpenRepository", err)
		return
	}
	commit, err := gitRepo.GetCommit(c.Params(":sha"))
	if err != nil {
		c.NotFoundOrServerError("GetCommit", git.IsErrNotExist, err)
		return
	}

	// Retrieve author and committer information
	var apiAuthor, apiCommitter *api.User
	author, err := models.GetUserByEmail(commit.Author.Email)
	if err != nil && !errors.IsUserNotExist(err) {
		c.ServerError("Get user by author email", err)
		return
	} else if err == nil {
		apiAuthor = author.APIFormat()
	}
	// Save one query if the author is also the committer
	if commit.Committer.Email == commit.Author.Email {
		apiCommitter = apiAuthor
	} else {
		committer, err := models.GetUserByEmail(commit.Committer.Email)
		if err != nil && !errors.IsUserNotExist(err) {
			c.ServerError("Get user by committer email", err)
			return
		} else if err == nil {
			apiCommitter = committer.APIFormat()
		}
	}

	// Retrieve parent(s) of the commit
	apiParents := make([]*api.CommitMeta, commit.ParentCount())
	for i := 0; i < commit.ParentCount(); i++ {
		sha, _ := commit.ParentID(i)
		apiParents[i] = &api.CommitMeta{
			URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/commits/" + sha.String(),
			SHA: sha.String(),
		}
	}

	c.JSONSuccess(&api.Commit{
		CommitMeta: &api.CommitMeta{
			URL: setting.AppURL + c.Link[1:],
			SHA: commit.ID.String(),
		},
		HTMLURL: c.Repo.Repository.HTMLURL() + "/commits/" + commit.ID.String(),
		RepoCommit: &api.RepoCommit{
			URL: setting.AppURL + c.Link[1:],
			Author: &api.CommitUser{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
				Date:  commit.Author.When.Format(time.RFC3339),
			},
			Committer: &api.CommitUser{
				Name:  commit.Committer.Name,
				Email: commit.Committer.Email,
				Date:  commit.Committer.When.Format(time.RFC3339),
			},
			Message: commit.Summary(),
			Tree: &api.CommitMeta{
				URL: c.BaseURL + "/repos/" + c.Repo.Repository.FullName() + "/tree/" + commit.ID.String(),
				SHA: commit.ID.String(),
			},
		},
		Author:    apiAuthor,
		Committer: apiCommitter,
		Parents:   apiParents,
	})
}

func GetReferenceSHA(c *context.APIContext) {
	gitRepo, err := git.OpenRepository(c.Repo.Repository.RepoPath())
	if err != nil {
		c.ServerError("OpenRepository", err)
		return
	}

	ref := c.Params("*")
	refType := 0 // 0-undetermined, 1-branch, 2-tag
	if strings.HasPrefix(ref, git.BRANCH_PREFIX) {
		ref = strings.TrimPrefix(ref, git.BRANCH_PREFIX)
		refType = 1
	} else if strings.HasPrefix(ref, git.TAG_PREFIX) {
		ref = strings.TrimPrefix(ref, git.TAG_PREFIX)
		refType = 2
	} else {
		if gitRepo.IsBranchExist(ref) {
			refType = 1
		} else if gitRepo.IsTagExist(ref) {
			refType = 2
		} else {
			c.NotFound()
			return
		}
	}

	var sha string
	if refType == 1 {
		sha, err = gitRepo.GetBranchCommitID(ref)
	} else if refType == 2 {
		sha, err = gitRepo.GetTagCommitID(ref)
	}
	if err != nil {
		c.NotFoundOrServerError("get reference commit ID", git.IsErrNotExist, err)
		return
	}
	c.PlainText(http.StatusOK, []byte(sha))
}
