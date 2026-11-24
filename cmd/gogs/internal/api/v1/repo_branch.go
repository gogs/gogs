package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories#get-branch
func getBranch(c *apiContext) {
	branch, err := c.Repo.Repository.GetBranch(c.Request().Context(), c.Param("**"))
	if err != nil {
		c.NotFoundOrError(err, "get branch")
		return
	}

	commit, err := branch.GetCommit(c.Request().Context())
	if err != nil {
		c.Error(err, "get commit")
		return
	}

	c.JSONSuccess(toBranch(branch, commit))
}

// listBranches lists all branches, or a page of them when the "page" or
// "pageSize" query parameter is given.
//
// https://github.com/gogs/go-gogs-client/wiki/Repositories#list-branches
func listBranches(c *apiContext) {
	branches, err := c.Repo.Repository.GetBranches(c.Request().Context())
	if err != nil {
		c.Error(err, "get branches")
		return
	}

	if c.Query("page") != "" || c.Query("pageSize") != "" {
		pageSize := toAllowedPageSize(c.QueryInt("pageSize"))
		page := c.queryPage(pageSize)
		c.SetLinkHeader(len(branches), pageSize)
		start := min((page-1)*pageSize, len(branches))
		branches = branches[start:min(start+pageSize, len(branches))]
	}

	apiBranches := make([]*v1types.RepositoryBranch, len(branches))
	for i := range branches {
		commit, err := branches[i].GetCommit(c.Request().Context())
		if err != nil {
			c.Error(err, "get commit")
			return
		}
		apiBranches[i] = toBranch(branches[i], commit)
	}

	c.JSONSuccess(&apiBranches)
}

type createReferenceRequest struct {
	Name string `json:"name" validate:"required"`
	// The branch, tag or commit to point at, defaults to the default branch.
	Target string `json:"target"`
}

// POST /repos/:username/:reponame/branches
func createBranch(c *apiContext, r createReferenceRequest) {
	commit, err := createReference(c, git.RefsHeads, r)
	if err != nil {
		c.Error(err, "create branch")
		return
	} else if commit == nil {
		return
	}
	branch := &database.Branch{RepoPath: c.Repo.Repository.RepoPath(), Name: r.Name}
	c.JSON(http.StatusCreated, toBranch(branch, commit))
}

// createReference creates the branch or tag, by the prefix of its reference,
// and returns the commit it points at. It returns a nil commit when it has
// written a client error response, and an error for the caller to report.
func createReference(c *apiContext, prefix string, r createReferenceRequest) (*git.Commit, error) {
	repo := c.Repo.Repository
	if repo.IsMirror {
		c.ErrorStatus(http.StatusForbidden, errors.New("Mirror repository is read-only."))
		return nil, nil
	}

	ctx := c.Request().Context()
	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		return nil, errors.Wrap(err, "open repository")
	}

	kind, isValidName := "branch", gitRepo.IsValidBranchName
	if prefix == git.RefsTags {
		kind, isValidName = "tag", gitRepo.IsValidTagName
	}
	if !isValidName(ctx, r.Name) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid %s name %q.", kind, r.Name))
		return nil, nil
	}

	if r.Target == "" {
		r.Target = repo.DefaultBranch
	}
	// A branch wins over a tag of the same name, like everywhere else in the API.
	rev := r.Target
	if gitRepo.HasBranch(ctx, r.Target) {
		rev = git.RefsHeads + r.Target
	} else if gitRepo.HasTag(ctx, r.Target) {
		rev = git.RefsTags + r.Target
	}
	// Peel annotated tags to the commit they point at.
	commit, err := gitRepo.CatFileCommit(ctx, rev+"^{commit}")
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The target %q does not exist.", r.Target))
			return nil, nil
		}
		return nil, errors.Wrap(err, "get target commit")
	}

	if prefix == git.RefsHeads {
		if gitRepo.HasBranch(ctx, r.Name) {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The %s name %q is already taken.", kind, r.Name))
			return nil, nil
		}
		// A protected branch keeps its protection after it is deleted.
		if ok, err := checkBranchProtection(c, r.Name); err != nil {
			return nil, errors.Wrapf(err, "check protection of branch %q", r.Name)
		} else if !ok {
			return nil, nil
		}
	}

	err = repo.CreateReference(ctx, c.Logger, c.User, prefix+r.Name, commit.ID.String())
	if errors.Is(err, git.ErrReferenceConflict) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The %s name %q is already taken.", kind, r.Name))
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrapf(err, "create %s", kind)
	}
	return commit, nil
}

// checkBranchProtection returns whether the protection of the branch lets the
// user push to it, and writes the 403 response when it does not. The caller
// reports the error.
func checkBranchProtection(c *apiContext, branch string) (bool, error) {
	denial, err := database.Handle.ProtectBranches().CheckPushByName(c.Request().Context(), c.Repo.Repository.ID, branch, c.User.ID, false)
	if err != nil {
		return false, err
	}
	switch denial {
	case database.PushDeniedNotInAllowlist:
		c.ErrorStatus(http.StatusForbidden, errors.Newf("Branch %q is protected and you are not in the push allowlist.", branch))
		return false, nil
	case database.PushDeniedRequiresPullRequest:
		c.ErrorStatus(http.StatusForbidden, errors.Newf("Branch %q is protected and commits must be merged through a pull request.", branch))
		return false, nil
	}
	return true, nil
}
