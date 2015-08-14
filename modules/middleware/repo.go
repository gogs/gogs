// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Unknwon/macaron"
	"github.com/mcuadros/go-version"
	"github.com/mssola/user_agent"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const (
	FIREFOX_COPY_SUPPORT = "41.0"
	CHROME_COPY_SUPPORT  = "43.0.2356"
)

func ApiRepoAssignment() macaron.Handler {
	return func(ctx *Context) {
		userName := ctx.Params(":username")
		repoName := ctx.Params(":reponame")

		var (
			u   *models.User
			err error
		)

		// Check if the user is the same as the repository owner.
		if ctx.IsSigned && ctx.User.LowerName == strings.ToLower(userName) {
			u = ctx.User
		} else {
			u, err = models.GetUserByName(userName)
			if err != nil {
				if models.IsErrUserNotExist(err) {
					ctx.Error(404)
				} else {
					ctx.JSON(500, &base.ApiJsonErr{"GetUserByName: " + err.Error(), base.DOC_URL})
				}
				return
			}
		}
		ctx.Repo.Owner = u

		// Get repository.
		repo, err := models.GetRepositoryByName(u.Id, repoName)
		if err != nil {
			if models.IsErrRepoNotExist(err) {
				ctx.Error(404)
			} else {
				ctx.JSON(500, &base.ApiJsonErr{"GetRepositoryByName: " + err.Error(), base.DOC_URL})
			}
			return
		} else if err = repo.GetOwner(); err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"GetOwner: " + err.Error(), base.DOC_URL})
			return
		}

		mode, err := models.AccessLevel(ctx.User, repo)
		if err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"AccessLevel: " + err.Error(), base.DOC_URL})
			return
		}

		ctx.Repo.AccessMode = mode

		// Check access.
		if ctx.Repo.AccessMode == models.ACCESS_MODE_NONE {
			ctx.Error(404)
			return
		}

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

		// Check if the user is the same as the repository owner
		if ctx.IsSigned && ctx.User.LowerName == strings.ToLower(userName) {
			u = ctx.User
		} else {
			u, err = models.GetUserByName(userName)
			if err != nil {
				if models.IsErrUserNotExist(err) {
					ctx.Handle(404, "GetUserByName", err)
				} else {
					ctx.Handle(500, "GetUserByName", err)
				}
				return
			}
		}
		ctx.Repo.Owner = u

		// Get repository.
		repo, err := models.GetRepositoryByName(u.Id, repoName)
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

		mode, err := models.AccessLevel(ctx.User, repo)
		if err != nil {
			ctx.Handle(500, "AccessLevel", err)
			return
		}
		ctx.Repo.AccessMode = mode

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

		if repo.IsFork {
			// Non-fork repository will not return error in this method.
			if err = repo.GetBaseRepo(); err != nil {
				ctx.Handle(500, "GetBaseRepo", err)
				return
			} else if repo.BaseRepo.GetOwner(); err != nil {
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

		ctx.Data["Title"] = u.Name + "/" + repo.Name
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = ctx.Repo.Repository.Owner
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner()
		ctx.Data["IsRepositoryAdmin"] = ctx.Repo.IsAdmin()

		ctx.Data["DisableSSH"] = setting.DisableSSH
		ctx.Repo.CloneLink, err = repo.CloneLink()
		if err != nil {
			ctx.Handle(500, "CloneLink", err)
			return
		}
		ctx.Data["CloneLink"] = ctx.Repo.CloneLink

		if ctx.Query("go-get") == "1" {
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
			ctx.Data["IsWatchingRepo"] = models.IsWatching(ctx.User.Id, repo.ID)
			ctx.Data["IsStaringRepo"] = models.IsStaring(ctx.User.Id, repo.ID)
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
		ctx.Data["CommitId"] = ctx.Repo.CommitId

		userAgent := ctx.Req.Header.Get("User-Agent")
		ua := user_agent.New(userAgent)
		browserName, browserVer := ua.Browser()

		ctx.Data["BrowserSupportsCopy"] = (browserName == "Chrome" && version.Compare(browserVer, CHROME_COPY_SUPPORT, ">=")) ||
			(browserName == "Firefox" && version.Compare(browserVer, FIREFOX_COPY_SUPPORT, ">="))
	}
}

func RequireRepoAdmin() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.Repo.IsAdmin() {
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
