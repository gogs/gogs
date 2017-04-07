// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/Unknwon/com"
	"gopkg.in/editorconfig/editorconfig-core-go.v1"
	"gopkg.in/macaron.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/setting"
)

type PullRequest struct {
	BaseRepo *models.Repository
	Allowed  bool
	SameRepo bool
	HeadInfo string // [<user>:]<branch>
}

type Repository struct {
	AccessMode   models.AccessMode
	IsWatching   bool
	IsViewBranch bool
	IsViewTag    bool
	IsViewCommit bool
	Repository   *models.Repository
	Owner        *models.User
	Commit       *git.Commit
	Tag          *git.Tag
	GitRepo      *git.Repository
	BranchName   string
	TagName      string
	TreePath     string
	CommitID     string
	RepoLink     string
	CloneLink    models.CloneLink
	CommitsCount int64
	Mirror       *models.Mirror

	PullRequest *PullRequest
}

// IsOwner returns true if current user is the owner of repository.
func (r *Repository) IsOwner() bool {
	return r.AccessMode >= models.ACCESS_MODE_OWNER
}

// IsAdmin returns true if current user has admin or higher access of repository.
func (r *Repository) IsAdmin() bool {
	return r.AccessMode >= models.ACCESS_MODE_ADMIN
}

// IsWriter returns true if current user has write or higher access of repository.
func (r *Repository) IsWriter() bool {
	return r.AccessMode >= models.ACCESS_MODE_WRITE
}

// HasAccess returns true if the current user has at least read access for this repository
func (r *Repository) HasAccess() bool {
	return r.AccessMode >= models.ACCESS_MODE_READ
}

// CanEnableEditor returns true if repository is editable and user has proper access level.
func (r *Repository) CanEnableEditor() bool {
	return r.Repository.CanEnableEditor() && r.IsViewBranch && r.IsWriter() && !r.Repository.IsBranchRequirePullRequest(r.BranchName)
}

// GetEditorconfig returns the .editorconfig definition if found in the
// HEAD of the default repo branch.
func (r *Repository) GetEditorconfig() (*editorconfig.Editorconfig, error) {
	commit, err := r.GitRepo.GetBranchCommit(r.Repository.DefaultBranch)
	if err != nil {
		return nil, err
	}
	treeEntry, err := commit.GetTreeEntryByPath(".editorconfig")
	if err != nil {
		return nil, err
	}
	reader, err := treeEntry.Blob().Data()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return editorconfig.ParseBytes(data)
}

// PullRequestURL returns URL for composing a pull request.
// This function does not check if the repository can actually compose a pull request.
func (r *Repository) PullRequestURL(baseBranch, headBranch string) string {
	repoLink := r.RepoLink
	if r.PullRequest.BaseRepo != nil {
		repoLink = r.PullRequest.BaseRepo.Link()
	}
	return fmt.Sprintf("%s/compare/%s...%s:%s", repoLink, baseBranch, r.Owner.Name, headBranch)
}

// composeGoGetImport returns go-get-import meta content.
func composeGoGetImport(owner, repo string) string {
	return path.Join(setting.Domain, setting.AppSubURL, owner, repo)
}

// earlyResponseForGoGetMeta responses appropriate go-get meta with status 200
// if user does not have actual access to the requested repository,
// or the owner or repository does not exist at all.
// This is particular a workaround for "go get" command which does not respect
// .netrc file.
func earlyResponseForGoGetMeta(ctx *Context) {
	ctx.PlainText(200, []byte(com.Expand(`<meta name="go-import" content="{GoGetImport} git {CloneLink}">`,
		map[string]string{
			"GoGetImport": composeGoGetImport(ctx.Params(":username"), ctx.Params(":reponame")),
			"CloneLink":   models.ComposeHTTPSCloneURL(ctx.Params(":username"), ctx.Params(":reponame")),
		})))
}

// [0]: issues, [1]: wiki
func RepoAssignment(pages ...bool) macaron.Handler {
	return func(ctx *Context) {
		var (
			owner        *models.User
			err          error
			isIssuesPage bool
			isWikiPage   bool
		)

		if len(pages) > 0 {
			isIssuesPage = pages[0]
		}
		if len(pages) > 1 {
			isWikiPage = pages[1]
		}

		ownerName := ctx.Params(":username")
		repoName := strings.TrimSuffix(ctx.Params(":reponame"), ".git")
		refName := ctx.Params(":branchname")
		if len(refName) == 0 {
			refName = ctx.Params(":path")
		}

		// Check if the user is the same as the repository owner
		if ctx.IsLogged && ctx.User.LowerName == strings.ToLower(ownerName) {
			owner = ctx.User
		} else {
			owner, err = models.GetUserByName(ownerName)
			if err != nil {
				if errors.IsUserNotExist(err) {
					if ctx.Query("go-get") == "1" {
						earlyResponseForGoGetMeta(ctx)
						return
					}
					ctx.NotFound()
				} else {
					ctx.Handle(500, "GetUserByName", err)
				}
				return
			}
		}
		ctx.Repo.Owner = owner
		ctx.Data["Username"] = ctx.Repo.Owner.Name

		// Get repository.
		repo, err := models.GetRepositoryByName(owner.ID, repoName)
		if err != nil {
			if errors.IsRepoNotExist(err) {
				if ctx.Query("go-get") == "1" {
					earlyResponseForGoGetMeta(ctx)
					return
				}
				ctx.NotFound()
			} else {
				ctx.Handle(500, "GetRepositoryByName", err)
			}
			return
		}

		ctx.Repo.Repository = repo
		ctx.Data["RepoName"] = ctx.Repo.Repository.Name
		ctx.Data["IsBareRepo"] = ctx.Repo.Repository.IsBare
		ctx.Repo.RepoLink = repo.Link()
		ctx.Data["RepoLink"] = ctx.Repo.RepoLink
		ctx.Data["RepoRelPath"] = ctx.Repo.Owner.Name + "/" + ctx.Repo.Repository.Name

		// Admin has super access.
		if ctx.IsLogged && ctx.User.IsAdmin {
			ctx.Repo.AccessMode = models.ACCESS_MODE_OWNER
		} else {
			mode, err := models.AccessLevel(ctx.UserID(), repo)
			if err != nil {
				ctx.Handle(500, "AccessLevel", err)
				return
			}
			ctx.Repo.AccessMode = mode
		}

		// Check access
		if ctx.Repo.AccessMode == models.ACCESS_MODE_NONE {
			if ctx.Query("go-get") == "1" {
				earlyResponseForGoGetMeta(ctx)
				return
			}

			// Redirect to any accessible page if not yet on it
			if repo.IsPartialPublic() &&
				(!(isIssuesPage || isWikiPage) ||
					(isIssuesPage && !repo.CanGuestViewIssues()) ||
					(isWikiPage && !repo.CanGuestViewWiki())) {
				switch {
				case repo.CanGuestViewIssues():
					ctx.Redirect(repo.Link() + "/issues")
				case repo.CanGuestViewWiki():
					ctx.Redirect(repo.Link() + "/wiki")
				default:
					ctx.NotFound()
				}
				return
			}

			// Response 404 if user is on completely private repository or possible accessible page but owner doesn't enabled
			if !repo.IsPartialPublic() ||
				(isIssuesPage && !repo.CanGuestViewIssues()) ||
				(isWikiPage && !repo.CanGuestViewWiki()) {
				ctx.NotFound()
				return
			}

			ctx.Repo.Repository.EnableIssues = repo.CanGuestViewIssues()
			ctx.Repo.Repository.EnableWiki = repo.CanGuestViewWiki()
		}

		if repo.IsMirror {
			ctx.Repo.Mirror, err = models.GetMirrorByRepoID(repo.ID)
			if err != nil {
				ctx.Handle(500, "GetMirror", err)
				return
			}
			ctx.Data["MirrorEnablePrune"] = ctx.Repo.Mirror.EnablePrune
			ctx.Data["MirrorInterval"] = ctx.Repo.Mirror.Interval
			ctx.Data["Mirror"] = ctx.Repo.Mirror
		}

		gitRepo, err := git.OpenRepository(models.RepoPath(ownerName, repoName))
		if err != nil {
			ctx.Handle(500, "RepoAssignment Invalid repo "+models.RepoPath(ownerName, repoName), err)
			return
		}
		ctx.Repo.GitRepo = gitRepo

		tags, err := ctx.Repo.GitRepo.GetTags()
		if err != nil {
			ctx.Handle(500, fmt.Sprintf("GetTags '%s'", ctx.Repo.Repository.RepoPath()), err)
			return
		}
		ctx.Data["Tags"] = tags
		ctx.Repo.Repository.NumTags = len(tags)

		ctx.Data["Title"] = owner.Name + "/" + repo.Name
		ctx.Data["Repository"] = repo
		ctx.Data["Owner"] = ctx.Repo.Repository.Owner
		ctx.Data["IsRepositoryOwner"] = ctx.Repo.IsOwner()
		ctx.Data["IsRepositoryAdmin"] = ctx.Repo.IsAdmin()
		ctx.Data["IsRepositoryWriter"] = ctx.Repo.IsWriter()

		ctx.Data["DisableSSH"] = setting.SSH.Disabled
		ctx.Data["DisableHTTP"] = setting.Repository.DisableHTTPGit
		ctx.Data["CloneLink"] = repo.CloneLink()
		ctx.Data["WikiCloneLink"] = repo.WikiCloneLink()

		if ctx.IsLogged {
			ctx.Data["IsWatchingRepo"] = models.IsWatching(ctx.User.ID, repo.ID)
			ctx.Data["IsStaringRepo"] = models.IsStaring(ctx.User.ID, repo.ID)
		}

		// repo is bare and display enable
		if ctx.Repo.Repository.IsBare {
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
			ctx.Data["GoGetImport"] = composeGoGetImport(owner.Name, repo.Name)
			prefix := setting.AppURL + path.Join(owner.Name, repo.Name, "src", ctx.Repo.BranchName)
			ctx.Data["GoDocDirectory"] = prefix + "{/dir}"
			ctx.Data["GoDocFile"] = prefix + "{/dir}/{file}#L{line}"
		}

		ctx.Data["IsGuest"] = !ctx.Repo.HasAccess()
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
			ctx.Repo.GitRepo, err = git.OpenRepository(repoPath)
			if err != nil {
				ctx.Handle(500, "RepoRef Invalid repo "+repoPath, err)
				return
			}
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
						ctx.Repo.TreePath = strings.Join(parts[i+1:], "/")
					}
					hasMatched = true
					break
				}
			}
			if !hasMatched && len(parts[0]) == 40 {
				refName = parts[0]
				ctx.Repo.TreePath = strings.Join(parts[1:], "/")
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
					ctx.NotFound()
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
		ctx.Data["TreePath"] = ctx.Repo.TreePath
		ctx.Data["IsViewBranch"] = ctx.Repo.IsViewBranch
		ctx.Data["IsViewTag"] = ctx.Repo.IsViewTag
		ctx.Data["IsViewCommit"] = ctx.Repo.IsViewCommit

		// People who have push access or have fored repository can propose a new pull request.
		if ctx.Repo.IsWriter() || (ctx.IsLogged && ctx.User.HasForkedRepo(ctx.Repo.Repository.ID)) {
			// Pull request is allowed if this is a fork repository
			// and base repository accepts pull requests.
			if ctx.Repo.Repository.BaseRepo != nil {
				if ctx.Repo.Repository.BaseRepo.AllowsPulls() {
					ctx.Repo.PullRequest.Allowed = true
					// In-repository pull requests has higher priority than cross-repository if user is viewing
					// base repository and 1) has write access to it 2) has forked it.
					if ctx.Repo.IsWriter() {
						ctx.Data["BaseRepo"] = ctx.Repo.Repository.BaseRepo
						ctx.Repo.PullRequest.BaseRepo = ctx.Repo.Repository.BaseRepo
						ctx.Repo.PullRequest.HeadInfo = ctx.Repo.Owner.Name + ":" + ctx.Repo.BranchName
					} else {
						ctx.Data["BaseRepo"] = ctx.Repo.Repository
						ctx.Repo.PullRequest.BaseRepo = ctx.Repo.Repository
						ctx.Repo.PullRequest.HeadInfo = ctx.User.Name + ":" + ctx.Repo.BranchName
					}
				}
			} else {
				// Or, this is repository accepts pull requests between branches.
				if ctx.Repo.Repository.AllowsPulls() {
					ctx.Data["BaseRepo"] = ctx.Repo.Repository
					ctx.Repo.PullRequest.BaseRepo = ctx.Repo.Repository
					ctx.Repo.PullRequest.Allowed = true
					ctx.Repo.PullRequest.SameRepo = true
					ctx.Repo.PullRequest.HeadInfo = ctx.Repo.BranchName
				}
			}
		}
		ctx.Data["PullRequestCtx"] = ctx.Repo.PullRequest
	}
}

func RequireRepoAdmin() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsLogged || (!ctx.Repo.IsAdmin() && !ctx.User.IsAdmin) {
			ctx.NotFound()
			return
		}
	}
}

func RequireRepoWriter() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsLogged || (!ctx.Repo.IsWriter() && !ctx.User.IsAdmin) {
			ctx.NotFound()
			return
		}
	}
}

// GitHookService checks if repository Git hooks service has been enabled.
func GitHookService() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.User.CanEditGitHook() {
			ctx.NotFound()
			return
		}
	}
}
