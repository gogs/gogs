// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/macaron.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

func RetrieveBaseRepo(ctx *Context, repo *models.Repository) {
	// Non-fork repository will not return error in this method.
	if err := repo.GetBaseRepo(); err != nil {
		if models.IsErrRepoNotExist(err) {
			repo.IsFork = false
			repo.ForkID = 0
			return
		}
		ctx.Handle(500, "GetBaseRepo", err)
		return
	} else if err = repo.BaseRepo.GetOwner(); err != nil {
		ctx.Handle(500, "BaseRepo.GetOwner", err)
		return
	}

	bsaeRepo := repo.BaseRepo
	baseGitRepo, err := git.OpenRepository(models.RepoPath(bsaeRepo.Owner.Name, bsaeRepo.Name))
	if err != nil {
		ctx.Handle(500, "OpenRepository", err)
		return
	}
	if len(bsaeRepo.DefaultBranch) > 0 && baseGitRepo.IsBranchExist(bsaeRepo.DefaultBranch) {
		ctx.Data["BaseDefaultBranch"] = bsaeRepo.DefaultBranch
	} else {
		baseBranches, err := baseGitRepo.GetBranches()
		if err != nil {
			ctx.Handle(500, "GetBranches", err)
			return
		}
		if len(baseBranches) > 0 {
			ctx.Data["BaseDefaultBranch"] = baseBranches[0]
		}
	}
}

func RepoAssignment(args ...bool) macaron.Handler {
	return func(ctx *Context) {
		var (
			displayBare bool // To display bare page if it is a bare repo.
		)
		if len(args) >= 1 {
			displayBare = args[0]
		}

		var (
			owner *models.User
			err   error
		)

		userName := ctx.Params(":username")
		repoName := ctx.Params(":reponame")
		refName := ctx.Params(":branchname")
		if len(refName) == 0 {
			refName = ctx.Params(":path")
		}

		// Check if the user is the same as the repository owner
		if ctx.IsSigned && ctx.User.LowerName == strings.ToLower(userName) {
			owner = ctx.User
		} else {
			owner, err = models.GetUserByName(userName)
			if err != nil {
				if models.IsErrUserNotExist(err) {
					ctx.Handle(404, "GetUserByName", err)
				} else {
					ctx.Handle(500, "GetUserByName", err)
				}
				return
			}
		}
		ctx.Repo.Owner = owner

		// Get repository.
		repo, err := models.GetRepositoryByName(owner.Id, repoName)
		if err != nil {
			if models.IsErrRepoNotExist(err) {
				ctx.Handle(404, "GetRepositoryByName", err)
			} else {
				ctx.Handle(500, "GetRepositoryByName", err)
			}
			return
		} else if err = repo.GetOwner(); err != nil {
			ctx.Handle(500, "GetOwner", err)
			return
		}

		// Admin has super access.
		if ctx.IsSigned && ctx.User.IsAdmin {
			ctx.Repo.AccessMode = models.ACCESS_MODE_OWNER
		} else {
			mode, err := models.AccessLevel(ctx.User, repo)
			if err != nil {
				ctx.Handle(500, "AccessLevel", err)
				return
			}
			ctx.Repo.AccessMode = mode
		}

		// Check access.
		if ctx.Repo.AccessMode == models.ACCESS_MODE_NONE {
			ctx.Handle(404, "no access right", err)
			return
		}
		ctx.Data["HasAccess"] = true

		if repo.IsMirror {
			ctx.Repo.Mirror, err = models.GetMirror(repo.ID)
			if err != nil {
				ctx.Handle(500, "GetMirror", err)
				return
			}
			ctx.Data["MirrorInterval"] = ctx.Repo.Mirror.Interval
			ctx.Data["Mirror"] = ctx.Repo.Mirror
		}

		ctx.Repo.Repository = repo
		ctx.Data["IsBareRepo"] = ctx.Repo.Repository.IsBare

		gitRepo, err := git.OpenRepository(models.RepoPath(userName, repoName))
		if err != nil {
			ctx.Handle(500, "RepoAssignment Invalid repo "+models.RepoPath(userName, repoName), err)
			return
		}
		ctx.Repo.GitRepo = gitRepo
		ctx.Repo.RepoLink = repo.RepoLink()
		ctx.Data["RepoLink"] = ctx.Repo.RepoLink
		ctx.Data["RepoRelPath"] = ctx.Repo.Owner.Name + "/" + ctx.Repo.Repository.Name

		tags, err := ctx.Repo.GitRepo.GetTags()
		if err != nil {
			ctx.Handle(500, "GetTags", err)
			return
		}
		ctx.Data["Tags"] = tags
		ctx.Repo.Repository.NumTags = len(tags)

		if repo.IsFork {
			RetrieveBaseRepo(ctx, repo)
			if ctx.Written() {
				return
			}
		}

		ctx.Data["Title"] = owner.Name + "/" + repo.Name
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = ctx.Repo.Repository.Owner
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner()
		ctx.Data["IsRepositoryAdmin"] = ctx.Repo.IsAdmin()
		ctx.Data["IsRepositoryPusher"] = ctx.Repo.IsPusher()
		ctx.Data["CanPullRequest"] = ctx.Repo.IsAdmin() && repo.BaseRepo != nil && repo.BaseRepo.AllowsPulls()

		ctx.Data["DisableSSH"] = setting.DisableSSH
		ctx.Data["CloneLink"] = repo.CloneLink()
		ctx.Data["WikiCloneLink"] = repo.WikiCloneLink()

		if ctx.IsSigned {
			ctx.Data["IsWatchingRepo"] = models.IsWatching(ctx.User.Id, repo.ID)
			ctx.Data["IsStaringRepo"] = models.IsStaring(ctx.User.Id, repo.ID)
		}

		// repo is bare and display enable
		if ctx.Repo.Repository.IsBare {
			log.Debug("Bare repository: %s", ctx.Repo.RepoLink)
			// NOTE: to prevent templating error
			ctx.Data["BranchName"] = ""
			if displayBare {
				if !ctx.Repo.IsAdmin() {
					ctx.Flash.Info(ctx.Tr("repo.repo_is_empty"), true)
				}
				ctx.HTML(200, "repo/bare")
			}
			return
		}

		ctx.Data["TagName"] = ctx.Repo.TagName
		brs, err := ctx.Repo.GitRepo.GetBranches()
		if err != nil {
			ctx.Handle(500, "GetBranches", err)
			return
		}
		ctx.Data["Branches"] = brs
		ctx.Data["BrancheCount"] = len(brs)

		// If not branch selected, try default one.
		// If default branch doesn't exists, fall back to some other branch.
		if len(ctx.Repo.BranchName) == 0 {
			if len(ctx.Repo.Repository.DefaultBranch) > 0 && gitRepo.IsBranchExist(ctx.Repo.Repository.DefaultBranch) {
				ctx.Repo.BranchName = ctx.Repo.Repository.DefaultBranch
			} else if len(brs) > 0 {
				ctx.Repo.BranchName = brs[0]
			}
		}

		ctx.Data["BranchName"] = ctx.Repo.BranchName
		ctx.Data["CommitID"] = ctx.Repo.CommitID

		if ctx.Query("go-get") == "1" {
			ctx.Data["GoGetImport"] = path.Join(setting.Domain, setting.AppSubUrl, owner.Name, repo.Name)
			prefix := path.Join(setting.AppUrl, owner.Name, repo.Name, "src", ctx.Repo.BranchName)
			ctx.Data["GoDocDirectory"] = prefix + "{/dir}"
			ctx.Data["GoDocFile"] = prefix + "{/dir}/{file}#L{line}"
		}
	}
}

// RepoRef handles repository reference name including those contain `/`.
func RepoRef() macaron.Handler {
	return func(ctx *Context) {
		// Empty repository does not have reference information.
		if ctx.Repo.Repository.IsBare {
			return
		}

		var (
			refName string
			err     error
		)

		// For API calls.
		if ctx.Repo.GitRepo == nil {
			repoPath := models.RepoPath(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
			gitRepo, err := git.OpenRepository(repoPath)
			if err != nil {
				ctx.Handle(500, "RepoRef Invalid repo "+repoPath, err)
				return
			}
			ctx.Repo.GitRepo = gitRepo
		}

		// Get default branch.
		if len(ctx.Params("*")) == 0 {
			refName = ctx.Repo.Repository.DefaultBranch
			if !ctx.Repo.GitRepo.IsBranchExist(refName) {
				brs, err := ctx.Repo.GitRepo.GetBranches()
				if err != nil {
					ctx.Handle(500, "GetBranches", err)
					return
				}
				refName = brs[0]
			}
			ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetBranchCommit(refName)
			if err != nil {
				ctx.Handle(500, "GetBranchCommit", err)
				return
			}
			ctx.Repo.CommitID = ctx.Repo.Commit.ID.String()
			ctx.Repo.IsViewBranch = true

		} else {
			hasMatched := false
			parts := strings.Split(ctx.Params("*"), "/")
			for i, part := range parts {
				refName = strings.TrimPrefix(refName+"/"+part, "/")

				if ctx.Repo.GitRepo.IsBranchExist(refName) ||
					ctx.Repo.GitRepo.IsTagExist(refName) {
					if i < len(parts)-1 {
						ctx.Repo.TreeName = strings.Join(parts[i+1:], "/")
					}
					hasMatched = true
					break
				}
			}
			if !hasMatched && len(parts[0]) == 40 {
				refName = parts[0]
				ctx.Repo.TreeName = strings.Join(parts[1:], "/")
			}

			if ctx.Repo.GitRepo.IsBranchExist(refName) {
				ctx.Repo.IsViewBranch = true

				ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetBranchCommit(refName)
				if err != nil {
					ctx.Handle(500, "GetBranchCommit", err)
					return
				}
				ctx.Repo.CommitID = ctx.Repo.Commit.ID.String()

			} else if ctx.Repo.GitRepo.IsTagExist(refName) {
				ctx.Repo.IsViewTag = true
				ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetTagCommit(refName)
				if err != nil {
					ctx.Handle(500, "GetTagCommit", err)
					return
				}
				ctx.Repo.CommitID = ctx.Repo.Commit.ID.String()
			} else if len(refName) == 40 {
				ctx.Repo.IsViewCommit = true
				ctx.Repo.CommitID = refName

				ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetCommit(refName)
				if err != nil {
					ctx.Handle(404, "GetCommit", nil)
					return
				}
			} else {
				ctx.Handle(404, "RepoRef invalid repo", fmt.Errorf("branch or tag not exist: %s", refName))
				return
			}
		}

		ctx.Repo.BranchName = refName
		ctx.Data["BranchName"] = ctx.Repo.BranchName
		ctx.Data["CommitID"] = ctx.Repo.CommitID
		ctx.Data["IsViewBranch"] = ctx.Repo.IsViewBranch
		ctx.Data["IsViewTag"] = ctx.Repo.IsViewTag
		ctx.Data["IsViewCommit"] = ctx.Repo.IsViewCommit

		ctx.Repo.CommitsCount, err = ctx.Repo.Commit.CommitsCount()
		if err != nil {
			ctx.Handle(500, "CommitsCount", err)
			return
		}
		ctx.Data["CommitsCount"] = ctx.Repo.CommitsCount
	}
}

func RequireRepoAdmin() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.Repo.IsAdmin() {
			ctx.Handle(404, ctx.Req.RequestURI, nil)
			return
		}
	}
}

func RequireRepoPusher() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.Repo.IsPusher() {
			ctx.Handle(404, ctx.Req.RequestURI, nil)
			return
		}
	}
}

// GitHookService checks if repository Git hooks service has been enabled.
func GitHookService() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.User.CanEditGitHook() {
			ctx.Handle(404, "GitHookService", nil)
			return
		}
	}
}
