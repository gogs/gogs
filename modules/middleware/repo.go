// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"errors"
	"fmt"
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/git"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
)

func RepoAssignment(redirect bool) martini.Handler {
	return func(ctx *Context, params martini.Params) {
		// assign false first
		ctx.Data["IsRepositoryValid"] = false

		var (
			user *models.User
			err  error
		)

		userName := params["username"]
		repoName := params["reponame"]
		branchName := params["branchname"]

		// get repository owner
		ctx.Repo.IsOwner = ctx.IsSigned && ctx.User.LowerName == strings.ToLower(userName)

		if !ctx.Repo.IsOwner {
			user, err = models.GetUserByName(params["username"])
			if err != nil {
				if redirect {
					ctx.Redirect("/")
					return
				}
				ctx.Handle(200, "RepoAssignment", err)
				return
			}
		} else {
			user = ctx.User
		}

		if user == nil {
			if redirect {
				ctx.Redirect("/")
				return
			}
			ctx.Handle(200, "RepoAssignment", errors.New("invliad user account for single repository"))
			return
		}

		// get repository
		repo, err := models.GetRepositoryByName(user.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				ctx.Handle(404, "RepoAssignment", err)
			} else if redirect {
				ctx.Redirect("/")
				return
			}
			ctx.Handle(404, "RepoAssignment", err)
			return
		}
		ctx.Repo.Repository = repo

		gitRepo, err := git.OpenRepository(models.RepoPath(userName, repoName))
		if err != nil {
			ctx.Handle(404, "RepoAssignment Invalid repo", err)
			return
		}
		ctx.Repo.GitRepo = gitRepo

	detect:
		if len(branchName) > 0 {
			// TODO check tag
			if models.IsBranchExist(user.Name, repoName, branchName) {
				ctx.Repo.IsBranch = true
				ctx.Repo.BranchName = branchName

				ctx.Repo.Commit, err = gitRepo.GetCommitOfBranch(branchName)
				if err != nil {
					ctx.Handle(404, "RepoAssignment invalid branch", nil)
					return
				}

				ctx.Repo.CommitId = ctx.Repo.Commit.Oid.String()

			} else if len(branchName) == 40 {
				ctx.Repo.IsCommit = true
				ctx.Repo.CommitId = branchName
				ctx.Repo.BranchName = branchName

				ctx.Repo.Commit, err = gitRepo.GetCommit(branchName)
				if err != nil {
					ctx.Handle(404, "RepoAssignment invalid commit", nil)
					return
				}
			} else {
				ctx.Handle(404, "RepoAssignment invalid repo", nil)
				return
			}

		} else {
			branchName = "master"
			goto detect
		}

		if ctx.IsSigned {
			ctx.Repo.IsWatching = models.IsWatching(ctx.User.Id, repo.Id)
		}

		ctx.Repo.Owner = user
		ctx.Repo.CloneLink.SSH = fmt.Sprintf("%s@%s:%s/%s.git", base.RunUser, base.Domain, user.LowerName, repo.LowerName)
		ctx.Repo.CloneLink.HTTPS = fmt.Sprintf("%s%s/%s.git", base.AppUrl, user.LowerName, repo.LowerName)
		ctx.Repo.RepoLink = "/" + user.Name + "/" + repo.Name

		ctx.Data["BranchName"] = ctx.Repo.BranchName
		ctx.Data["CommitId"] = ctx.Repo.CommitId
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = user
		ctx.Data["Title"] = user.Name + "/" + repo.Name
		ctx.Data["CloneLink"] = ctx.Repo.CloneLink
		ctx.Data["RepoLink"] = ctx.Repo.RepoLink
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner
		ctx.Data["IsRepositoryWatching"] = ctx.Repo.IsWatching
	}
}
