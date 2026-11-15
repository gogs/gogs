package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

type createReleaseRequest struct {
	TagName string `json:"tag_name" validate:"required"`
	// The branch to tag when the tag does not exist yet, defaults to the default
	// branch.
	TargetCommitish string `json:"target_commitish"`
	Name            string `json:"name" validate:"max=255"`
	Body            string `json:"body"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

// POST /repos/:username/:reponame/releases
func createRelease(c *apiContext, r createReleaseRequest) {
	ctx := c.Request().Context()
	repo := c.Repo.Repository
	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	if !gitRepo.IsValidTagName(ctx, r.TagName) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid tag name %q.", r.TagName))
		return
	}
	if r.TargetCommitish == "" {
		r.TargetCommitish = repo.DefaultBranch
	}
	if !gitRepo.HasBranch(ctx, r.TargetCommitish) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The target branch %q does not exist.", r.TargetCommitish))
		return
	}

	// An existing tag keeps the time it was made, a new one is made now.
	var tagCreatedUnix int64
	if tag, err := gitRepo.Tag(ctx, git.RefsTags+r.TagName); err == nil {
		if commit, err := tag.Commit(ctx); err == nil {
			tagCreatedUnix = commit.Author.When.Unix()
		}
	}
	commit, err := gitRepo.BranchCommit(ctx, r.TargetCommitish)
	if err != nil {
		c.Error(err, "get branch commit")
		return
	}
	numCommits, err := commit.CommitsCount(ctx)
	if err != nil {
		c.Error(err, "count commits")
		return
	}

	rel := &database.Release{
		RepoID:       repo.ID,
		PublisherID:  c.User.ID,
		Title:        r.Name,
		TagName:      r.TagName,
		Target:       r.TargetCommitish,
		Sha1:         commit.ID.String(),
		NumCommits:   numCommits,
		Note:         r.Body,
		IsDraft:      r.Draft,
		IsPrerelease: r.Prerelease,
		CreatedUnix:  tagCreatedUnix,
	}
	if err = database.NewRelease(ctx, gitRepo, rel, nil); err != nil {
		switch {
		case database.IsErrReleaseAlreadyExist(err):
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("A release of the tag %q already exists.", r.TagName))
		case database.IsErrInvalidTagName(err):
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid tag name %q.", r.TagName))
		default:
			c.Error(err, "create release")
		}
		return
	}
	writeRelease(c, http.StatusCreated, rel.ID)
}

// getReleaseOfRepo returns the release in the route, or nil when it has written
// an error response.
func getReleaseOfRepo(c *apiContext) *database.Release {
	rel, err := database.GetReleaseByID(c.ParamInt64("id"))
	if err == nil && rel.RepoID != c.Repo.Repository.ID {
		err = database.ErrReleaseNotExist{}
	}
	if err != nil {
		c.NotFoundOrError(err, "get release")
		return nil
	}
	return rel
}

// writeRelease responds with the release of the ID, read again so that it has
// its creation time.
func writeRelease(c *apiContext, status int, id int64) {
	rel, err := database.GetReleaseByID(id)
	if err != nil {
		c.Error(err, "get release")
		return
	}
	c.JSON(status, toRelease(rel))
}

// GET /repos/:username/:reponame/releases/:id
func getRelease(c *apiContext) {
	rel := getReleaseOfRepo(c)
	if rel == nil {
		return
	}
	c.JSONSuccess(toRelease(rel))
}

type editReleaseRequest struct {
	Name       *string `json:"name" validate:"omitempty,max=255"`
	Body       *string `json:"body"`
	Draft      *bool   `json:"draft"`
	Prerelease *bool   `json:"prerelease"`
}

// PATCH /repos/:username/:reponame/releases/:id
func editRelease(c *apiContext, r editReleaseRequest) {
	rel := getReleaseOfRepo(c)
	if rel == nil {
		return
	}
	// A published release has its tag, so it cannot go back to a draft.
	if r.Draft != nil && *r.Draft && !rel.IsDraft {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("A published release cannot become a draft."))
		return
	}

	gitRepo, err := git.Open(c.Repo.Repository.RepoPath())
	if err != nil {
		c.Error(err, "open repository")
		return
	}

	isPublish := r.Draft != nil && !*r.Draft && rel.IsDraft
	if r.Name != nil {
		rel.Title = *r.Name
	}
	if r.Body != nil {
		rel.Note = *r.Body
	}
	if r.Prerelease != nil {
		rel.IsPrerelease = *r.Prerelease
	}
	if isPublish {
		rel.IsDraft = false
	}

	// Keep the attachments, which this endpoint does not manage.
	uuids := make([]string, len(rel.Attachments))
	for i, a := range rel.Attachments {
		uuids[i] = a.UUID
	}
	if err = database.UpdateRelease(c.Request().Context(), c.User, gitRepo, rel, isPublish, uuids); err != nil {
		if database.IsErrInvalidTagName(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid tag name %q.", rel.TagName))
			return
		}
		c.Error(err, "update release")
		return
	}
	writeRelease(c, http.StatusOK, rel.ID)
}

// DELETE /repos/:username/:reponame/releases/:id
func deleteRelease(c *apiContext) {
	rel := getReleaseOfRepo(c)
	if rel == nil {
		return
	}
	if err := database.DeleteReleaseOfRepoByID(c.Request().Context(), c.Repo.Repository.ID, rel.ID); err != nil {
		c.Error(err, "delete release")
		return
	}
	c.NoContent()
}
