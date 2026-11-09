package context

import (
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/repox"
)

type PullRequest struct {
	BaseRepo *database.Repository
	Allowed  bool
	SameRepo bool
	HeadInfo string // [<user>:]<branch>
}

type Repository struct {
	AccessMode   database.AccessMode
	IsWatching   bool
	IsViewBranch bool
	IsViewTag    bool
	IsViewCommit bool
	Repository   *database.Repository
	Owner        *database.User
	Commit       *git.Commit
	Tag          *git.Tag
	GitRepo      *git.Repository
	BranchName   string
	TagName      string
	TreePath     string
	CommitID     string
	RepoLink     string
	CloneLink    repox.CloneLink
	CommitsCount int64
	Mirror       *database.Mirror

	PullRequest *PullRequest
}

// IsOwner returns true if current user is the owner of repository.
func (r *Repository) IsOwner() bool {
	return r.AccessMode >= database.AccessModeOwner
}

// IsAdmin returns true if current user has admin or higher access of repository.
func (r *Repository) IsAdmin() bool {
	return r.AccessMode >= database.AccessModeAdmin
}

// IsWriter returns true if current user has write or higher access of repository.
func (r *Repository) IsWriter() bool {
	return r.AccessMode >= database.AccessModeWrite
}

// HasAccess returns true if the current user has at least read access for this repository
func (r *Repository) HasAccess() bool {
	return r.AccessMode >= database.AccessModeRead
}

// CanEnableEditor returns true if repository is editable and user has proper access level.
func (r *Repository) CanEnableEditor() bool {
	return r.Repository.CanEnableEditor() && r.IsViewBranch && r.IsWriter() && !r.Repository.IsBranchRequirePullRequest(r.BranchName)
}

// MakeURL accepts a string or url.URL as argument and returns escaped URL prepended with repository URL.
func (r *Repository) MakeURL(location any) string {
	switch location := location.(type) {
	case string:
		tempURL := url.URL{
			Path: r.RepoLink + "/" + location,
		}
		return tempURL.String()
	case url.URL:
		location.Path = r.RepoLink + "/" + location.Path
		return location.String()
	default:
		panic("location type must be either string or url.URL")
	}
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
			owner        *database.User
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

		// Check if the user is the same as the repository owner
		if c.IsLogged && c.User.LowerName == strings.ToLower(ownerName) {
			owner = c.User
		} else {
			owner, err = database.Handle.Users().GetByUsername(c.Req.Context(), ownerName)
			if err != nil {
				c.NotFoundOrError(err, "get user by name")
				return
			}
		}
		c.Repo.Owner = owner

		repo, err := database.GetRepositoryByName(owner.ID, repoName)
		if err != nil {
			c.NotFoundOrError(err, "get repository by name")
			return
		}

		c.Repo.Repository = repo
		c.Repo.RepoLink = repo.Link()

		// Admin has super access
		if c.IsLogged && c.User.IsAdmin {
			c.Repo.AccessMode = database.AccessModeOwner
		} else {
			c.Repo.AccessMode = database.Handle.Permissions().AccessMode(c.Req.Context(), c.UserID(), repo.ID,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
		}

		// If the authenticated user has no direct access, see if the repository is a fork
		// and whether the user has access to the base repository.
		if c.Repo.AccessMode == database.AccessModeNone && repo.BaseRepo != nil {
			mode := min(
				// Users shouldn't have indirect access level higher than write.
				database.Handle.Permissions().AccessMode(c.Req.Context(), c.UserID(), repo.BaseRepo.ID,
					database.AccessModeOptions{
						OwnerID: repo.BaseRepo.OwnerID,
						Private: repo.BaseRepo.IsPrivate,
					},
				),
				database.AccessModeWrite,
			)
			c.Repo.AccessMode = mode
		}

		// Check access
		if c.Repo.AccessMode == database.AccessModeNone {
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
			c.Repo.Mirror, err = database.GetMirrorByRepoID(repo.ID)
			if err != nil {
				c.Error(err, "get mirror by repository ID")
				return
			}
		}

		gitRepo, err := git.Open(database.RepoPath(ownerName, repoName))
		if err != nil {
			c.Error(err, "open repository")
			return
		}
		c.Repo.GitRepo = gitRepo

		tags, err := c.Repo.GitRepo.Tags(c.Req.Context())
		if err != nil {
			c.Error(err, "get tags")
			return
		}
		c.Repo.Repository.NumTags = len(tags)

		// An empty repository has no branches to load.
		if c.Repo.Repository.IsEmpty {
			return
		}

		branches, err := c.Repo.GitRepo.Branches(c.Req.Context())
		if err != nil {
			c.Error(err, "get branches")
			return
		}

		// If not branch selected, try default one.
		// If default branch doesn't exists, fall back to some other branch.
		if c.Repo.BranchName == "" {
			if len(c.Repo.Repository.DefaultBranch) > 0 && gitRepo.HasBranch(c.Req.Context(), c.Repo.Repository.DefaultBranch) {
				c.Repo.BranchName = c.Repo.Repository.DefaultBranch
			} else if len(branches) > 0 {
				c.Repo.BranchName = branches[0]
			}
		}

	}
}

// RepoRef handles repository reference name including those contain `/`.
func RepoRef() macaron.Handler {
	return func(c *Context) {
		// Empty repository does not have reference information.
		if c.Repo.Repository.IsEmpty {
			return
		}

		var (
			refName string
			err     error
		)

		// For API calls.
		if c.Repo.GitRepo == nil {
			repoPath := database.RepoPath(c.Repo.Owner.Name, c.Repo.Repository.Name)
			c.Repo.GitRepo, err = git.Open(repoPath)
			if err != nil {
				c.Error(err, "open repository")
				return
			}
		}

		// Get default branch.
		if c.Params("*") == "" {
			refName = c.Repo.Repository.DefaultBranch
			if !c.Repo.GitRepo.HasBranch(c.Req.Context(), refName) {
				branches, err := c.Repo.GitRepo.Branches(c.Req.Context())
				if err != nil {
					c.Error(err, "get branches")
					return
				}
				refName = branches[0]
			}
			c.Repo.Commit, err = c.Repo.GitRepo.BranchCommit(c.Req.Context(), refName)
			if err != nil {
				c.Error(err, "get branch commit")
				return
			}
			c.Repo.CommitID = c.Repo.Commit.ID.String()
			c.Repo.IsViewBranch = true

		} else {
			hasMatched := false
			parts := strings.Split(c.Params("*"), "/")
			for i, part := range parts {
				refName = strings.TrimPrefix(refName+"/"+part, "/")

				if c.Repo.GitRepo.HasBranch(c.Req.Context(), refName) ||
					c.Repo.GitRepo.HasTag(c.Req.Context(), refName) {
					if i < len(parts)-1 {
						c.Repo.TreePath = strings.Join(parts[i+1:], "/")
					}
					hasMatched = true
					break
				}
			}
			if !hasMatched && (len(parts[0]) == 40 || len(parts[0]) == 64) {
				refName = parts[0]
				c.Repo.TreePath = strings.Join(parts[1:], "/")
			}

			if c.Repo.GitRepo.HasBranch(c.Req.Context(), refName) {
				c.Repo.IsViewBranch = true

				c.Repo.Commit, err = c.Repo.GitRepo.BranchCommit(c.Req.Context(), refName)
				if err != nil {
					c.Error(err, "get branch commit")
					return
				}
				c.Repo.CommitID = c.Repo.Commit.ID.String()

			} else if c.Repo.GitRepo.HasTag(c.Req.Context(), refName) {
				c.Repo.IsViewTag = true
				c.Repo.Commit, err = c.Repo.GitRepo.TagCommit(c.Req.Context(), refName)
				if err != nil {
					c.Error(err, "get tag commit")
					return
				}
				c.Repo.CommitID = c.Repo.Commit.ID.String()
			} else if len(refName) == 40 || len(refName) == 64 {
				c.Repo.IsViewCommit = true
				c.Repo.CommitID = refName

				c.Repo.Commit, err = c.Repo.GitRepo.CatFileCommit(c.Req.Context(), refName)
				if err != nil {
					c.NotFound()
					return
				}
			} else {
				c.NotFound()
				return
			}
		}

		c.Repo.BranchName = refName

		// People who have push access or have forked repository can propose a new pull request.
		if c.Repo.IsWriter() || (c.IsLogged && database.Handle.Repositories().HasForkedBy(c.Req.Context(), c.Repo.Repository.ID, c.User.ID)) {
			// Pull request is allowed if this is a fork repository
			// and base repository accepts pull requests.
			if c.Repo.Repository.BaseRepo != nil {
				if c.Repo.Repository.BaseRepo.AllowsPulls() {
					c.Repo.PullRequest.Allowed = true
					// In-repository pull requests has higher priority than cross-repository if user is viewing
					// base repository and 1) has write access to it 2) has forked it.
					if c.Repo.IsWriter() {
						c.Repo.PullRequest.BaseRepo = c.Repo.Repository.BaseRepo
						c.Repo.PullRequest.HeadInfo = c.Repo.Owner.Name + ":" + c.Repo.BranchName
					} else {
						c.Repo.PullRequest.BaseRepo = c.Repo.Repository
						c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
					}
				}
			} else {
				// Or, this is repository accepts pull requests between branches.
				if c.Repo.Repository.AllowsPulls() {
					c.Repo.PullRequest.BaseRepo = c.Repo.Repository
					c.Repo.PullRequest.Allowed = true
					c.Repo.PullRequest.SameRepo = true
					c.Repo.PullRequest.HeadInfo = c.Repo.BranchName
				}
			}
		}
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
