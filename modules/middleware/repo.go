// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/git"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func RepoAssignment(redirect bool, args ...bool) martini.Handler {
	return func(ctx *Context, params martini.Params) {
		// valid brachname
		var validBranch bool
		// display bare quick start if it is a bare repo
		var displayBare bool

		if len(args) >= 1 {
			validBranch = args[0]
		}

		if len(args) >= 2 {
			displayBare = args[1]
		}

		var (
			user *models.User
			err  error
		)

		userName := params["username"]
		repoName := params["reponame"]
		refName := params["branchname"]

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
		ctx.Repo.Owner = user

		// get repository
		repo, err := models.GetRepositoryByName(user.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				ctx.Handle(404, "RepoAssignment", err)
				return
			} else if redirect {
				ctx.Redirect("/")
				return
			}
			ctx.Handle(500, "RepoAssignment", err)
			return
		}

		// Check access.
		if repo.IsPrivate {
			if ctx.User == nil {
				ctx.Handle(404, "RepoAssignment(HasAccess)", nil)
				return
			}

			hasAccess, err := models.HasAccess(ctx.User.Name, ctx.Repo.Owner.Name+"/"+repo.Name, models.AU_READABLE)
			if err != nil {
				ctx.Handle(500, "RepoAssignment(HasAccess)", err)
				return
			} else if !hasAccess {
				ctx.Handle(404, "RepoAssignment(HasAccess)", nil)
				return
			}
		}
		ctx.Repo.HasAccess = true
		ctx.Data["HasAccess"] = true

		if repo.IsMirror {
			ctx.Repo.Mirror, err = models.GetMirror(repo.Id)
			if err != nil {
				ctx.Handle(500, "RepoAssignment(GetMirror)", err)
				return
			}
			ctx.Data["MirrorInterval"] = ctx.Repo.Mirror.Interval
		}

		repo.NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
		ctx.Repo.Repository = repo
		ctx.Data["IsBareRepo"] = ctx.Repo.Repository.IsBare

		gitRepo, err := git.OpenRepository(models.RepoPath(userName, repoName))
		if err != nil {
			ctx.Handle(500, "RepoAssignment Invalid repo "+models.RepoPath(userName, repoName), err)
			return
		}
		ctx.Repo.GitRepo = gitRepo
		ctx.Repo.RepoLink = "/" + user.Name + "/" + repo.Name

		tags, err := ctx.Repo.GitRepo.GetTags()
		if err != nil {
			ctx.Handle(500, "RepoAssignment(GetTags))", err)
			return
		}
		ctx.Repo.Repository.NumTags = len(tags)

		ctx.Data["Title"] = user.Name + "/" + repo.Name
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = user
		ctx.Data["RepoLink"] = ctx.Repo.RepoLink
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner
		ctx.Data["BranchName"] = ""

		ctx.Repo.CloneLink.SSH = fmt.Sprintf("%s@%s:%s/%s.git", base.RunUser, base.Domain, user.LowerName, repo.LowerName)
		ctx.Repo.CloneLink.HTTPS = fmt.Sprintf("%s%s/%s.git", base.AppUrl, user.LowerName, repo.LowerName)
		ctx.Data["CloneLink"] = ctx.Repo.CloneLink

		if ctx.Repo.Repository.IsGoget {
			ctx.Data["GoGetLink"] = fmt.Sprintf("%s%s/%s", base.AppUrl, user.LowerName, repo.LowerName)
			ctx.Data["GoGetImport"] = fmt.Sprintf("%s/%s/%s", base.Domain, user.LowerName, repo.LowerName)
		}

		// when repo is bare, not valid branch
		if !ctx.Repo.Repository.IsBare && validBranch {
		detect:
			if len(refName) > 0 {
				if gitRepo.IsBranchExist(refName) {
					ctx.Repo.IsBranch = true
					ctx.Repo.BranchName = refName

					ctx.Repo.Commit, err = gitRepo.GetCommitOfBranch(refName)
					if err != nil {
						ctx.Handle(404, "RepoAssignment invalid branch", nil)
						return
					}
					ctx.Repo.CommitId = ctx.Repo.Commit.Id.String()

				} else if gitRepo.IsTagExist(refName) {
					ctx.Repo.IsBranch = true
					ctx.Repo.BranchName = refName

					ctx.Repo.Commit, err = gitRepo.GetCommitOfTag(refName)
					if err != nil {
						ctx.Handle(404, "RepoAssignment invalid tag", nil)
						return
					}
					ctx.Repo.CommitId = ctx.Repo.Commit.Id.String()

				} else if len(refName) == 40 {
					ctx.Repo.IsCommit = true
					ctx.Repo.CommitId = refName
					ctx.Repo.BranchName = refName

					ctx.Repo.Commit, err = gitRepo.GetCommit(refName)
					if err != nil {
						ctx.Handle(404, "RepoAssignment invalid commit", nil)
						return
					}
				} else {
					ctx.Handle(404, "RepoAssignment invalid repo", nil)
					return
				}

			} else {
				refName = ctx.Repo.Repository.DefaultBranch
				if len(refName) == 0 {
					refName = "master"
				}
				goto detect
			}

			ctx.Data["IsBranch"] = ctx.Repo.IsBranch
			ctx.Data["IsCommit"] = ctx.Repo.IsCommit
			log.Debug("Repo.Commit: %v", ctx.Repo.Commit)
		}

		log.Debug("displayBare: %v; IsBare: %v", displayBare, ctx.Repo.Repository.IsBare)

		// repo is bare and display enable
		if displayBare && ctx.Repo.Repository.IsBare {
			log.Debug("Bare repository: %s", ctx.Repo.RepoLink)
			ctx.HTML(200, "repo/single_bare")
			return
		}

		if ctx.IsSigned {
			ctx.Repo.IsWatching = models.IsWatching(ctx.User.Id, repo.Id)
		}

		ctx.Data["BranchName"] = ctx.Repo.BranchName
		brs, err := ctx.Repo.GitRepo.GetBranches()
		if err != nil {
			log.Error("RepoAssignment(GetBranches): %v", err)
		}
		ctx.Data["Branches"] = brs
		ctx.Data["CommitId"] = ctx.Repo.CommitId
		ctx.Data["IsRepositoryWatching"] = ctx.Repo.IsWatching
	}
}
