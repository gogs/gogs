// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func ToApiSignature(signature *git.Signature) *api.Signature {
	return &api.Signature{
		Email:			signature.Email,
		Name: 			signature.Name,
		When: 			signature.When,
	}
}

func ToApiCommit(commit *git.Commit) *api.Commit {
	return &api.Commit{
		ID:            	commit.Id.String(),
		Author:        	*ToApiSignature(commit.Author),
		Committer:    	*ToApiSignature(commit.Committer),
		CommitMessage:  commit.CommitMessage,
	}
}

func convertToGitRepo(repository *models.Repository) (*git.Repository, error) {
	repoPath, err := repository.RepoPath()
	if err != nil {
		return nil, err
	}

	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		return nil, err
	}

	return gitRepo, nil
}

func CommitById(ctx *middleware.Context) {
	gitRepo, err := convertToGitRepo(ctx.Repo.Repository)
	if err != nil {
		log.Error(4, "convertToGitRepo: %v", err)
		ctx.Error(500)
		return
	}

	commit, err := gitRepo.GetCommit(ctx.Params(":commitId"))
	if err != nil {
		log.Error(4, "GetCommit: %v", err)
		ctx.Error(500, err)
		return
	}

	ctx.JSON(200, ToApiCommit(commit))
}