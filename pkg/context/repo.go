// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"fmt"
	"io/ioutil"
	"strings"

	"gopkg.in/editorconfig/editorconfig-core-go.v1"
	"gopkg.in/macaron.v1"

	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/setting"
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

// [0]: issues, [1]: wiki
func RepoAssignment(pages ...bool) macaron.Handler {
	return func(c *Context) {
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

		ownerName := c.Params(":username")
		repoName := strings.TrimSuffix(c.Params(":reponame"), ".git")
		refName := c.Params(":branchname")
		if len(refName) == 0 {
			refName = c.Params(":path")
		}

		// Check if the user is the same as the repository owner
		if c.IsLogged && c.User.LowerName == strings.ToLower(ownerName) {
			owner = c.User
		} else {
			owner, err = models.GetUserByName(ownerName)
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}
		}
		c.Repo.Owner = owner
		c.Data["Username"] = c.Repo.Owner.Name

		repo, err := models.GetRepositoryByName(owner.ID, repoName)
		if err != nil {
			c.NotFoundOrServerError("GetRepositoryByName", errors.IsRepoNotExist, err)
			return
		}

		c.Repo.Repository = repo
		c.Data["RepoName"] = c.Repo.Repository.Name
		c.Data["IsBareRepo"] = c.Repo.Repository.IsBare
		c.Repo.RepoLink = repo.Link()
		c.Data["RepoLink"] = c.Repo.RepoLink
		c.Data["RepoRelPath"] = c.Repo.Owner.Name + "/" + c.Repo.Repository.Name

		// Admin has super access.
		if c.IsLogged && c.User.IsAdmin {
			c.Repo.AccessMode = models.ACCESS_MODE_OWNER
		} else {
			mode, err := models.AccessLevel(c.UserID(), repo)
			if err != nil {
				c.ServerError("AccessLevel", err)
				return
			}
			c.Repo.AccessMode = mode
		}

		// Check access
		if c.Repo.AccessMode == models.ACCESS_MODE_NONE {
			// Redirect to any accessible page if not yet on it
			if repo.IsPartialPublic() &&
				(!(isIssuesPage || isWikiPage) ||
					(isIssuesPage && !repo.CanGuestViewIssues()) ||
					(isWikiPage && !repo.CanGuestViewWiki())) {
				switch {
				case repo.CanGuestViewIssues():
					c.Redirect(repo.Link() + "/issues")
				case repo.CanGuestViewWiki():
					c.Redirect(repo.Link() + "/wiki")
				default:
					c.NotFound()
				}
				return
			}

			// Response 404 if user is on completely private repository or possible accessible page but owner doesn't enabled
			if !repo.IsPartialPublic() ||
				(isIssuesPage && !repo.CanGuestViewIssues()) ||
				(isWikiPage && !repo.CanGuestViewWiki()) {
				c.NotFound()
				return
			}

			c.Repo.Repository.EnableIssues = repo.CanGuestViewIssues()
			c.Repo.Repository.EnableWiki = repo.CanGuestViewWiki()
		}

		if repo.IsMirror {
			c.Repo.Mirror, err = models.GetMirrorByRepoID(repo.ID)
			if err != nil {
				c.ServerError("GetMirror", err)
				return
			}
			c.Data["MirrorEnablePrune"] = c.Repo.Mirror.EnablePrune
			c.Data["MirrorInterval"] = c.Repo.Mirror.Interval
			c.Data["Mirror"] = c.Repo.Mirror
		}

		gitRepo, err := git.OpenRepository(models.RepoPath(ownerName, repoName))
		if err != nil {
			c.ServerError(fmt.Sprintf("RepoAssignment Invalid repo '%s'", c.Repo.Repository.RepoPath()), err)
			return
		}
		c.Repo.GitRepo = gitRepo

		tags, err := c.Repo.GitRepo.GetTags()
		if err != nil {
			c.ServerError(fmt.Sprintf("GetTags '%s'", c.Repo.Repository.RepoPath()), err)
			return
		}
		c.Data["Tags"] = tags
		c.Repo.Repository.NumTags = len(tags)

		c.Data["Title"] = owner.Name + "/" + repo.Name
		c.Data["Repository"] = repo
		c.Data["Owner"] = c.Repo.Repository.Owner
		c.Data["IsRepositoryOwner"] = c.Repo.IsOwner()
		c.Data["IsRepositoryAdmin"] = c.Repo.IsAdmin()
		c.Data["IsRepositoryWriter"] = c.Repo.IsWriter()

		c.Data["DisableSSH"] = setting.SSH.Disabled
		c.Data["DisableHTTP"] = setting.Repository.DisableHTTPGit
		c.Data["CloneLink"] = repo.CloneLink()
		c.Data["WikiCloneLink"] = repo.WikiCloneLink()

		if c.IsLogged {
			c.Data["IsWatchingRepo"] = models.IsWatching(c.User.ID, repo.ID)
			c.Data["IsStaringRepo"] = models.IsStaring(c.User.ID, repo.ID)
		}

		// repo is bare and display enable
		if c.Repo.Repository.IsBare {
			return
		}

		c.Data["TagName"] = c.Repo.TagName
		brs, err := c.Repo.GitRepo.GetBranches()
		if err != nil {
			c.ServerError("GetBranches", err)
			return
		}
		c.Data["Branches"] = brs
		c.Data["BrancheCount"] = len(brs)

		// If not branch selected, try default one.
		// If default branch doesn't exists, fall back to some other branch.
		if len(c.Repo.BranchName) == 0 {
			if len(c.Repo.Repository.DefaultBranch) > 0 && gitRepo.IsBranchExist(c.Repo.Repository.DefaultBranch) {
				c.Repo.BranchName = c.Repo.Repository.DefaultBranch
			} else if len(brs) > 0 {
				c.Repo.BranchName = brs[0]
			}
		}
		c.Data["BranchName"] = c.Repo.BranchName
		c.Data["CommitID"] = c.Repo.CommitID

		c.Data["IsGuest"] = !c.Repo.HasAccess()
	}
}

// RepoRef handles repository reference name including those contain `/`.
func RepoRef() macaron.Handler {
	return func(c *Context) {
		// Empty repository does not have reference information.
		if c.Repo.Repository.IsBare {
			return
		}

		var (
			refName string
			err     error
		)

		// For API calls.
		if c.Repo.GitRepo == nil {
			repoPath := models.RepoPath(c.Repo.Owner.Name, c.Repo.Repository.Name)
			c.Repo.GitRepo, err = git.OpenRepository(repoPath)
			if err != nil {
				c.Handle(500, "RepoRef Invalid repo "+repoPath, err)
				return
			}
		}

		// Get default branch.
		if len(c.Params("*")) == 0 {
			refName = c.Repo.Repository.DefaultBranch
			if !c.Repo.GitRepo.IsBranchExist(refName) {
				brs, err := c.Repo.GitRepo.GetBranches()
				if err != nil {
					c.Handle(500, "GetBranches", err)
					return
				}
				refName = brs[0]
			}
			c.Repo.Commit, err = c.Repo.GitRepo.GetBranchCommit(refName)
			if err != nil {
				c.Handle(500, "GetBranchCommit", err)
				return
			}
			c.Repo.CommitID = c.Repo.Commit.ID.String()
			c.Repo.IsViewBranch = true

		} else {
			hasMatched := false
			parts := strings.Split(c.Params("*"), "/")
			for i, part := range parts {
				refName = strings.TrimPrefix(refName+"/"+part, "/")

				if c.Repo.GitRepo.IsBranchExist(refName) ||
					c.Repo.GitRepo.IsTagExist(refName) {
					if i < len(parts)-1 {
						c.Repo.TreePath = strings.Join(parts[i+1:], "/")
					}
					hasMatched = true
					break
				}
			}
			if !hasMatched && len(parts[0]) == 40 {
				refName = parts[0]
				c.Repo.TreePath = strings.Join(parts[1:], "/")
			}

			if c.Repo.GitRepo.IsBranchExist(refName) {
				c.Repo.IsViewBranch = true

				c.Repo.Commit, err = c.Repo.GitRepo.GetBranchCommit(refName)
				if err != nil {
					c.Handle(500, "GetBranchCommit", err)
					return
				}
				c.Repo.CommitID = c.Repo.Commit.ID.String()

			} else if c.Repo.GitRepo.IsTagExist(refName) {
				c.Repo.IsViewTag = true
				c.Repo.Commit, err = c.Repo.GitRepo.GetTagCommit(refName)
				if err != nil {
					c.Handle(500, "GetTagCommit", err)
					return
				}
				c.Repo.CommitID = c.Repo.Commit.ID.String()
			} else if len(refName) == 40 {
				c.Repo.IsViewCommit = true
				c.Repo.CommitID = refName

				c.Repo.Commit, err = c.Repo.GitRepo.GetCommit(refName)
				if err != nil {
					c.NotFound()
					return
				}
			} else {
				c.Handle(404, "RepoRef invalid repo", fmt.Errorf("branch or tag not exist: %s", refName))
				return
			}
		}

		c.Repo.BranchName = refName
		c.Data["BranchName"] = c.Repo.BranchName
		c.Data["CommitID"] = c.Repo.CommitID
		c.Data["TreePath"] = c.Repo.TreePath
		c.Data["IsViewBranch"] = c.Repo.IsViewBranch
		c.Data["IsViewTag"] = c.Repo.IsViewTag
		c.Data["IsViewCommit"] = c.Repo.IsViewCommit

		// People who have push access or have fored repository can propose a new pull request.
		if c.Repo.IsWriter() || (c.IsLogged && c.User.HasForkedRepo(c.Repo.Repository.ID)) {
			// Pull request is allowed if this is a fork repository
			// and base repository accepts pull requests.
			if c.Repo.Repository.BaseRepo != nil {
				if c.Repo.Repository.BaseRepo.AllowsPulls() {
					c.Repo.PullRequest.Allowed = true
					// In-repository pull requests has higher priority than cross-repository if user is viewing
					// base repository and 1) has write access to it 2) has forked it.
					if c.Repo.IsWriter() {
						c.Data["BaseRepo"] = c.Repo.Repository.BaseRepo
						c.Repo.PullRequest.BaseRepo = c.Repo.Repository.BaseRepo
						c.Repo.PullRequest.HeadInfo = c.Repo.Owner.Name + ":" + c.Repo.BranchName
					} else {
						c.Data["BaseRepo"] = c.Repo.Repository
						c.Repo.PullRequest.BaseRepo = c.Repo.Repository
						c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
					}
				}
			} else {
				// Or, this is repository accepts pull requests between branches.
				if c.Repo.Repository.AllowsPulls() {
					c.Data["BaseRepo"] = c.Repo.Repository
					c.Repo.PullRequest.BaseRepo = c.Repo.Repository
					c.Repo.PullRequest.Allowed = true
					c.Repo.PullRequest.SameRepo = true
					c.Repo.PullRequest.HeadInfo = c.Repo.BranchName
				}
			}
		}
		c.Data["PullRequestCtx"] = c.Repo.PullRequest
	}
}

func RequireRepoAdmin() macaron.Handler {
	return func(c *Context) {
		if !c.IsLogged || (!c.Repo.IsAdmin() && !c.User.IsAdmin) {
			c.NotFound()
			return
		}
	}
}

func RequireRepoWriter() macaron.Handler {
	return func(c *Context) {
		if !c.IsLogged || (!c.Repo.IsWriter() && !c.User.IsAdmin) {
			c.NotFound()
			return
		}
	}
}

// GitHookService checks if repository Git hooks service has been enabled.
func GitHookService() macaron.Handler {
	return func(c *Context) {
		if !c.User.CanEditGitHook() {
			c.NotFound()
			return
		}
	}
}
