package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/route/api/v1/types"
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

// https://github.com/gogs/go-gogs-client/wiki/Repositories#list-branches
func listBranches(c *apiContext) {
	branches, err := c.Repo.Repository.GetBranches(c.Request().Context())
	if err != nil {
		c.Error(err, "get branches")
		return
	}

	apiBranches := make([]*types.RepositoryBranch, len(branches))
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
	commit := createReference(c, git.RefsHeads, r)
	if commit == nil {
		return
	}
	branch := &database.Branch{RepoPath: c.Repo.Repository.RepoPath(), Name: r.Name}
	c.JSON(http.StatusCreated, toBranch(branch, commit))
}

// createReference creates the branch or tag, by the prefix of its reference,
// and returns the commit it points at. It returns nil when it has written an
// error response.
func createReference(c *apiContext, prefix string, r createReferenceRequest) *git.Commit {
	repo := c.Repo.Repository
	if repo.IsMirror {
		c.ErrorStatus(http.StatusForbidden, errors.New("Mirror repository is read-only."))
		return nil
	}

	ctx := c.Request().Context()
	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return nil
	}

	kind, isValidName := "branch", gitRepo.IsValidBranchName
	if prefix == git.RefsTags {
		kind, isValidName = "tag", gitRepo.IsValidTagName
	}
	if !isValidName(ctx, r.Name) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid %s name %q.", kind, r.Name))
		return nil
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
		} else {
			c.Error(err, "get target commit")
		}
		return nil
	}

	// A protected branch keeps its protection after it is deleted.
	if prefix == git.RefsHeads && !checkBranchProtection(c, r.Name) {
		return nil
	}

	err = repo.CreateReference(ctx, c.Logger, c.User, prefix+r.Name, commit.ID.String())
	if errors.Is(err, git.ErrReferenceConflict) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The %s name %q is already taken.", kind, r.Name))
		return nil
	} else if err != nil {
		c.Errorf(err, "create %s", kind)
		return nil
	}
	return commit
}

// checkBranchProtection returns whether the protection of the branch lets the
// user push to it, and writes the 403 response when it does not.
func checkBranchProtection(c *apiContext, branch string) bool {
	denial, err := database.Handle.ProtectBranches().CheckPushByName(c.Request().Context(), c.Repo.Repository.ID, branch, c.User.ID, false)
	if err != nil {
		c.Error(err, "check branch protection")
		return false
	}
	switch denial {
	case database.PushDeniedNotInAllowlist:
		c.ErrorStatus(http.StatusForbidden, errors.Newf("Branch %q is protected and you are not in the push allowlist.", branch))
		return false
	case database.PushDeniedRequiresPullRequest:
		c.ErrorStatus(http.StatusForbidden, errors.Newf("Branch %q is protected and commits must be merged through pull request.", branch))
		return false
	}
	return true
}
