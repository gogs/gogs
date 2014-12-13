// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Unknwon/macaron"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

func ApiRepoAssignment() macaron.Handler {
	return func(ctx *Context) {
		userName := ctx.Params(":username")
		repoName := ctx.Params(":reponame")

		var (
			u   *models.User
			err error
		)

		// Collaborators who have write access can be seen as owners.
		if ctx.IsSigned {
			ctx.Repo.IsOwner, err = models.HasAccess(ctx.User.Name, userName+"/"+repoName, models.WRITABLE)
			if err != nil {
				ctx.JSON(500, &base.ApiJsonErr{"HasAccess: " + err.Error(), base.DOC_URL})
				return
			}
			ctx.Repo.IsTrueOwner = ctx.User.LowerName == strings.ToLower(userName)
		}

		if !ctx.Repo.IsTrueOwner {
			u, err = models.GetUserByName(userName)
			if err != nil {
				if err == models.ErrUserNotExist {
					ctx.Error(404)
				} else {
					ctx.JSON(500, &base.ApiJsonErr{"GetUserByName: " + err.Error(), base.DOC_URL})
				}
				return
			}
		} else {
			u = ctx.User
		}
		ctx.Repo.Owner = u

		// Organization owner team members are true owners as well.
		if ctx.IsSigned && ctx.Repo.Owner.IsOrganization() && ctx.Repo.Owner.IsOwnedBy(ctx.User.Id) {
			ctx.Repo.IsTrueOwner = true
		}

		// Get repository.
		repo, err := models.GetRepositoryByName(u.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				ctx.Error(404)
				return
			}
			ctx.JSON(500, &base.ApiJsonErr{"GetRepositoryByName: " + err.Error(), base.DOC_URL})
			return
		} else if err = repo.GetOwner(); err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"GetOwner: " + err.Error(), base.DOC_URL})
			return
		}

		// Check if the mirror repository owner(mirror repository doesn't have access).
		if ctx.IsSigned && !ctx.Repo.IsOwner {
			if repo.OwnerId == ctx.User.Id {
				ctx.Repo.IsOwner = true
			}
			// Check if current user has admin permission to repository.
			if u.IsOrganization() {
				auth, err := models.GetHighestAuthorize(u.Id, ctx.User.Id, repo.Id, 0)
				if err != nil {
					ctx.JSON(500, &base.ApiJsonErr{"GetHighestAuthorize: " + err.Error(), base.DOC_URL})
					return
				}
				if auth == models.ORG_ADMIN {
					ctx.Repo.IsOwner = true
					ctx.Repo.IsAdmin = true
				}
			}
		}

		// Check access.
		if repo.IsPrivate && !ctx.Repo.IsOwner {
			if ctx.User == nil {
				ctx.Error(404)
				return
			}

			hasAccess, err := models.HasAccess(ctx.User.Name, ctx.Repo.Owner.Name+"/"+repo.Name, models.READABLE)
			if err != nil {
				ctx.JSON(500, &base.ApiJsonErr{"HasAccess: " + err.Error(), base.DOC_URL})
				return
			} else if !hasAccess {
				ctx.Error(404)
				return
			}
		}
		ctx.Repo.HasAccess = true

		ctx.Repo.Repository = repo
	}
}

// RepoRef handles repository reference name including those contain `/`.
func RepoRef() macaron.Handler {
	return func(ctx *Context) {
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
			ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetCommitOfBranch(refName)
			if err != nil {
				ctx.Handle(500, "GetCommitOfBranch", err)
				return
			}
			ctx.Repo.CommitId = ctx.Repo.Commit.Id.String()
			ctx.Repo.IsBranch = true

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
				ctx.Repo.IsBranch = true

				ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetCommitOfBranch(refName)
				if err != nil {
					ctx.Handle(500, "GetCommitOfBranch", err)
					return
				}
				ctx.Repo.CommitId = ctx.Repo.Commit.Id.String()

			} else if ctx.Repo.GitRepo.IsTagExist(refName) {
				ctx.Repo.IsTag = true
				ctx.Repo.Commit, err = ctx.Repo.GitRepo.GetCommitOfTag(refName)
				if err != nil {
					ctx.Handle(500, "GetCommitOfTag", err)
					return
				}
				ctx.Repo.CommitId = ctx.Repo.Commit.Id.String()
			} else if len(refName) == 40 {
				ctx.Repo.IsCommit = true
				ctx.Repo.CommitId = refName

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
		ctx.Data["CommitId"] = ctx.Repo.CommitId
		ctx.Data["IsBranch"] = ctx.Repo.IsBranch
		ctx.Data["IsTag"] = ctx.Repo.IsTag
		ctx.Data["IsCommit"] = ctx.Repo.IsCommit

		ctx.Repo.CommitsCount, err = ctx.Repo.Commit.CommitsCount()
		if err != nil {
			ctx.Handle(500, "CommitsCount", err)
			return
		}
		ctx.Data["CommitsCount"] = ctx.Repo.CommitsCount
	}
}

func RepoAssignment(redirect bool, args ...bool) macaron.Handler {
	return func(ctx *Context) {
		var (
			displayBare bool // To display bare page if it is a bare repo.
		)
		if len(args) >= 1 {
			displayBare = args[0]
		}

		var (
			u   *models.User
			err error
		)

		userName := ctx.Params(":username")
		repoName := ctx.Params(":reponame")
		refName := ctx.Params(":branchname")
		if len(refName) == 0 {
			refName = ctx.Params(":path")
		}

		// Collaborators who have write access can be seen as owners.
		if ctx.IsSigned {
			ctx.Repo.IsOwner, err = models.HasAccess(ctx.User.Name, userName+"/"+repoName, models.WRITABLE)
			if err != nil {
				ctx.Handle(500, "HasAccess", err)
				return
			}
			ctx.Repo.IsTrueOwner = ctx.User.LowerName == strings.ToLower(userName)
		}

		if !ctx.Repo.IsTrueOwner {
			u, err = models.GetUserByName(userName)
			if err != nil {
				if err == models.ErrUserNotExist {
					ctx.Handle(404, "GetUserByName", err)
				} else if redirect {
					log.Error(4, "GetUserByName", err)
					ctx.Redirect(setting.AppSubUrl + "/")
				} else {
					ctx.Handle(500, "GetUserByName", err)
				}
				return
			}
		} else {
			u = ctx.User
		}

		if u == nil {
			if redirect {
				ctx.Redirect(setting.AppSubUrl + "/")
				return
			}
			ctx.Handle(404, "RepoAssignment", errors.New("invliad user account for single repository"))
			return
		}
		ctx.Repo.Owner = u

		// Organization owner team members are true owners as well.
		if ctx.IsSigned && ctx.Repo.Owner.IsOrganization() && ctx.Repo.Owner.IsOwnedBy(ctx.User.Id) {
			ctx.Repo.IsTrueOwner = true
		}

		// Get repository.
		repo, err := models.GetRepositoryByName(u.Id, repoName)
		if err != nil {
			if err == models.ErrRepoNotExist {
				ctx.Handle(404, "GetRepositoryByName", err)
				return
			} else if redirect {
				ctx.Redirect(setting.AppSubUrl + "/")
				return
			}
			ctx.Handle(500, "GetRepositoryByName", err)
			return
		} else if err = repo.GetOwner(); err != nil {
			ctx.Handle(500, "GetOwner", err)
			return
		}

		// Check if the mirror repository owner(mirror repository doesn't have access).
		if ctx.IsSigned && !ctx.Repo.IsOwner {
			if repo.OwnerId == ctx.User.Id {
				ctx.Repo.IsOwner = true
			}
			// Check if current user has admin permission to repository.
			if u.IsOrganization() {
				auth, err := models.GetHighestAuthorize(u.Id, ctx.User.Id, repo.Id, 0)
				if err != nil {
					ctx.Handle(500, "GetHighestAuthorize", err)
					return
				}
				if auth == models.ORG_ADMIN {
					ctx.Repo.IsOwner = true
					ctx.Repo.IsAdmin = true
				}
			}
		}

		// Check access.
		if repo.IsPrivate && !ctx.Repo.IsOwner {
			if ctx.User == nil {
				ctx.Handle(404, "HasAccess", nil)
				return
			}

			hasAccess, err := models.HasAccess(ctx.User.Name, ctx.Repo.Owner.Name+"/"+repo.Name, models.READABLE)
			if err != nil {
				ctx.Handle(500, "HasAccess", err)
				return
			} else if !hasAccess {
				ctx.Handle(404, "HasAccess", nil)
				return
			}
		}
		ctx.Repo.HasAccess = true
		ctx.Data["HasAccess"] = true

		if repo.IsMirror {
			ctx.Repo.Mirror, err = models.GetMirror(repo.Id)
			if err != nil {
				ctx.Handle(500, "GetMirror", err)
				return
			}
			ctx.Data["MirrorInterval"] = ctx.Repo.Mirror.Interval
		}

		repo.NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
		repo.NumOpenMilestones = repo.NumMilestones - repo.NumClosedMilestones
		ctx.Repo.Repository = repo
		ctx.Data["IsBareRepo"] = ctx.Repo.Repository.IsBare

		gitRepo, err := git.OpenRepository(models.RepoPath(userName, repoName))
		if err != nil {
			ctx.Handle(500, "RepoAssignment Invalid repo "+models.RepoPath(userName, repoName), err)
			return
		}
		ctx.Repo.GitRepo = gitRepo
		ctx.Repo.RepoLink, err = repo.RepoLink()
		if err != nil {
			ctx.Handle(500, "RepoLink", err)
			return
		}
		ctx.Data["RepoLink"] = ctx.Repo.RepoLink

		tags, err := ctx.Repo.GitRepo.GetTags()
		if err != nil {
			ctx.Handle(500, "GetTags", err)
			return
		}
		ctx.Data["Tags"] = tags
		ctx.Repo.Repository.NumTags = len(tags)

		// Non-fork repository will not return error in this method.
		if err = repo.GetForkRepo(); err != nil {
			ctx.Handle(500, "GetForkRepo", err)
			return
		}

		ctx.Data["Title"] = u.Name + "/" + repo.Name
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = ctx.Repo.Repository.Owner
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner
		ctx.Data["IsRepositoryTrueOwner"] = ctx.Repo.IsTrueOwner

		ctx.Repo.CloneLink, err = repo.CloneLink()
		if err != nil {
			ctx.Handle(500, "CloneLink", err)
			return
		}
		ctx.Data["CloneLink"] = ctx.Repo.CloneLink

		if ctx.Repo.Repository.IsGoget {
			ctx.Data["GoGetLink"] = fmt.Sprintf("%s%s/%s", setting.AppUrl, u.LowerName, repo.LowerName)
			ctx.Data["GoGetImport"] = fmt.Sprintf("%s/%s/%s", setting.Domain, u.LowerName, repo.LowerName)
		}

		// repo is bare and display enable
		if ctx.Repo.Repository.IsBare {
			log.Debug("Bare repository: %s", ctx.Repo.RepoLink)
			// NOTE: to prevent templating error
			ctx.Data["BranchName"] = ""
			if displayBare {
				ctx.HTML(200, "repo/bare")
			}
			return
		}

		if ctx.IsSigned {
			ctx.Data["IsWatchingRepo"] = models.IsWatching(ctx.User.Id, repo.Id)
			ctx.Data["IsStaringRepo"] = models.IsStaring(ctx.User.Id, repo.Id)
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
		if ctx.Repo.BranchName == "" {
			if ctx.Repo.Repository.DefaultBranch != "" && gitRepo.IsBranchExist(ctx.Repo.Repository.DefaultBranch) {
				ctx.Repo.BranchName = ctx.Repo.Repository.DefaultBranch
			} else if len(brs) > 0 {
				ctx.Repo.BranchName = brs[0]
			}
		}

		ctx.Data["BranchName"] = ctx.Repo.BranchName
		ctx.Data["CommitId"] = ctx.Repo.CommitId
	}
}

func RequireTrueOwner() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.Repo.IsTrueOwner && !ctx.Repo.IsAdmin {
			if !ctx.IsSigned {
				ctx.SetCookie("redirect_to", "/"+url.QueryEscape(setting.AppSubUrl+ctx.Req.RequestURI), 0, setting.AppSubUrl)
				ctx.Redirect(setting.AppSubUrl + "/user/login")
				return
			}
			ctx.Handle(404, ctx.Req.RequestURI, nil)
			return
		}
	}
}

// GitHookService checks if repository Git hooks service has been enabled.
func GitHookService() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.User.AllowGitHook && !ctx.User.IsAdmin {
			ctx.Handle(404, "GitHookService", nil)
			return
		}
	}
}
