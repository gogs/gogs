package v1

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

type createReleaseRequest struct {
	TagName string `json:"tag_name" binding:"Required;MaxSize(255)"`
	// The branch to tag when the tag does not exist yet, defaults to the default
	// branch.
	TargetCommitish string `json:"target_commitish"`
	Name            string `json:"name" binding:"MaxSize(255)"`
	Body            string `json:"body"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

// POST /repos/:username/:reponame/releases
func createRelease(c *apiContext, r createReleaseRequest) {
	ctx := c.Request().Context()
	repo := c.Repo.Repository
	// Mirror syncs would remove or move the tag.
	if repo.IsMirror {
		c.ErrorStatus(http.StatusForbidden, errors.New("Mirror repository is read-only."))
		return
	}
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
	if tag, err := gitRepo.Tag(ctx, r.TagName); err == nil {
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
	if err = database.Handle.Releases().Create(ctx, c.Logger, gitRepo, rel, nil); err != nil {
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
	return getReleaseOfRepoByID(c, c.ParamInt64("id"))
}

// getReleaseOfRepoByID returns the release with given ID of the repository in
// the route, or nil when it has written an error response.
func getReleaseOfRepoByID(c *apiContext, id int64) *database.Release {
	ctx := c.Request().Context()
	rel, err := database.Handle.Releases().GetByID(ctx, id)
	// Only writers see drafts, as in the web interface.
	if err == nil && (rel.RepoID != c.Repo.Repository.ID || rel.IsDraft && !c.Repo.IsWriter()) {
		err = database.ErrReleaseNotExist{}
	}
	if err != nil {
		c.NotFoundOrError(err, "get release")
		return nil
	}
	if err = database.Handle.Releases().LoadAttributes(ctx, rel); err != nil {
		c.Error(err, "load release attributes")
		return nil
	}
	return rel
}

// writeRelease responds with the release of the ID, read again so that it has
// its creation time.
func writeRelease(c *apiContext, status int, id int64) {
	ctx := c.Request().Context()
	rel, err := database.Handle.Releases().GetByID(ctx, id)
	if err != nil {
		c.Error(err, "get release")
		return
	}
	if err = database.Handle.Releases().LoadAttributes(ctx, rel); err != nil {
		c.Error(err, "load release attributes")
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
	apiRelease := toRelease(rel)
	// Hide the email of the publisher from unauthenticated callers, as
	// elsewhere for users.
	if !c.IsLogged {
		apiRelease.Author.Email = ""
	}
	c.JSONSuccess(apiRelease)
}

type editReleaseRequest struct {
	Name       *string `json:"name"`
	Body       *string `json:"body"`
	Draft      *bool   `json:"draft"`
	Prerelease *bool   `json:"prerelease"`
}

// PATCH /repos/:username/:reponame/releases/:id
func editRelease(c *apiContext, r editReleaseRequest) {
	// Binding rules do not measure pointers.
	if r.Name != nil && utf8.RuneCountInString(*r.Name) > 255 {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("The name is longer than 255 characters."))
		return
	}
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

	ctx := c.Request().Context()
	isPublish := r.Draft != nil && !*r.Draft && rel.IsDraft
	if isPublish {
		// Mirror syncs would remove or move the tag.
		if c.Repo.Repository.IsMirror {
			c.ErrorStatus(http.StatusForbidden, errors.New("Mirror repository is read-only."))
			return
		}
		// Drafts made before tag names were checked may have an invalid one.
		if !gitRepo.IsValidTagName(ctx, rel.TagName) {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid tag name %q.", rel.TagName))
			return
		}
		// Publishing tags the target branch when the tag does not exist yet.
		if !gitRepo.HasTag(ctx, rel.TagName) && !gitRepo.HasBranch(ctx, rel.Target) {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The target branch %q does not exist.", rel.Target))
			return
		}
	}
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
	if err = database.Handle.Releases().Update(ctx, c.Logger, c.User, gitRepo, rel, isPublish, uuids); err != nil {
		switch {
		case database.IsErrInvalidTagName(err):
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid tag name %q.", rel.TagName))
		case database.IsErrReleaseNotExist(err):
			c.NotFound()
		default:
			c.Error(err, "update release")
		}
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
	if err := database.Handle.Releases().DeleteByID(c.Request().Context(), c.Logger, c.Repo.Repository.ID, rel.ID); err != nil {
		c.NotFoundOrError(err, "delete release")
		return
	}
	c.NoContent()
}

// GET /repos/:username/:reponame/releases/:id/assets
func listReleaseAssets(c *apiContext) {
	rel := getReleaseOfRepo(c)
	if rel == nil {
		return
	}
	c.JSONSuccess(toReleaseAssets(rel.Attachments))
}

// checkReleaseAssetName writes an error response and returns false when the
// name is not a valid file name or the release has another file of it.
func checkReleaseAssetName(c *apiContext, rel *database.Release, name string, assetID int64) bool {
	switch {
	case name == "", name == ".", name == "..":
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("The name of the file is required."))
		return false
	case utf8.RuneCountInString(name) > 255:
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("The name of the file is longer than 255 characters."))
		return false
	case strings.ContainsAny(name, `/\`) || strings.ContainsFunc(name, unicode.IsControl):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Invalid file name %q.", name))
		return false
	case slices.ContainsFunc(rel.Attachments, func(a *database.Attachment) bool { return a.ID != assetID && a.Name == name }):
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("The release already has a file named %q.", name))
		return false
	}
	return true
}

// POST /repos/:username/:reponame/releases/:id/assets?name=
func uploadReleaseAsset(c *apiContext) {
	lim := conf.Release.Attachment
	if !lim.Enabled {
		c.ErrorStatus(http.StatusForbidden, errors.New("Release attachments are disabled."))
		return
	}
	rel := getReleaseOfRepo(c)
	if rel == nil {
		return
	}
	name := c.Query("name")
	if !checkReleaseAssetName(c, rel, name, 0) {
		return
	}
	if len(rel.Attachments) >= lim.MaxFiles {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("A release has at most %d files.", lim.MaxFiles))
		return
	}

	body := http.MaxBytesReader(c.ResponseWriter(), c.Request().Request.Body, lim.MaxSize<<20)
	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		writeAssetBodyError(c, err, lim.MaxSize)
		return
	}
	head = head[:n]
	fileType := http.DetectContentType(head)
	if !slices.ContainsFunc(lim.AllowedTypes, func(t string) bool {
		t = strings.TrimSpace(t)
		return t == "*/*" || t == fileType
	}) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("Files of type %q are not allowed.", fileType))
		return
	}

	// Stopping midway, e.g., when the client disconnects after the file is
	// stored, would leave the file without its release.
	ctx := context.WithoutCancel(c.Request().Context())
	a, err := database.Handle.Attachments().Create(ctx, c.User.ID, name, io.MultiReader(bytes.NewReader(head), body))
	if err != nil {
		writeAssetBodyError(c, err, lim.MaxSize)
		return
	}
	if err = database.Handle.Releases().Attach(ctx, rel.ID, a.ID); err != nil {
		if delErr := database.Handle.Attachments().Delete(ctx, true, a); delErr != nil {
			c.Logger.Error("Failed to delete attachment of a release that failed to attach", "attachmentID", a.ID, "error", delErr)
		}
		c.NotFoundOrError(err, "attach file to release")
		return
	}
	writeReleaseAsset(c, http.StatusCreated, a.ID)
}

// writeAssetBodyError responds to an error of reading the uploaded file, which
// is the client's when the file is too large.
func writeAssetBodyError(c *apiContext, err error, maxSize int64) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		c.ErrorStatus(http.StatusRequestEntityTooLarge, errors.Newf("The file is larger than %d MB.", maxSize))
		return
	}
	c.Error(err, "store file")
}

// getReleaseAssetOfRepo returns the asset in the route and its release, or nil
// when it has written an error response.
func getReleaseAssetOfRepo(c *apiContext) (*database.Attachment, *database.Release) {
	a, err := database.Handle.Attachments().GetByID(c.Request().Context(), c.ParamInt64("asset_id"))
	if err == nil && a.ReleaseID == 0 {
		err = database.ErrAttachmentNotExist{}
	}
	if err != nil {
		c.NotFoundOrError(err, "get attachment")
		return nil, nil
	}
	// The release of another repository or a draft the viewer cannot see is
	// not found like the asset itself.
	rel := getReleaseOfRepoByID(c, a.ReleaseID)
	if rel == nil {
		return nil, nil
	}
	return a, rel
}

// writeReleaseAsset responds with the asset of the ID, read again so that it
// has its creation time.
func writeReleaseAsset(c *apiContext, status int, id int64) {
	a, err := database.Handle.Attachments().GetByID(c.Request().Context(), id)
	if err != nil {
		c.Error(err, "get attachment")
		return
	}
	c.JSON(status, a.APIFormat())
}

// GET /repos/:username/:reponame/releases/assets/:asset_id
func getReleaseAsset(c *apiContext) {
	a, _ := getReleaseAssetOfRepo(c)
	if a == nil {
		return
	}
	c.JSONSuccess(a.APIFormat())
}

type editReleaseAssetRequest struct {
	Name string `json:"name" binding:"Required"`
}

// PATCH /repos/:username/:reponame/releases/assets/:asset_id
func editReleaseAsset(c *apiContext, r editReleaseAssetRequest) {
	a, rel := getReleaseAssetOfRepo(c)
	if a == nil || !checkReleaseAssetName(c, rel, r.Name, a.ID) {
		return
	}
	if err := database.Handle.Attachments().Rename(c.Request().Context(), a.ID, r.Name); err != nil {
		c.Error(err, "rename attachment")
		return
	}
	writeReleaseAsset(c, http.StatusOK, a.ID)
}

// DELETE /repos/:username/:reponame/releases/assets/:asset_id
func deleteReleaseAsset(c *apiContext) {
	a, _ := getReleaseAssetOfRepo(c)
	if a == nil {
		return
	}
	// Stopping midway would leave the row of a removed file.
	if err := database.Handle.Attachments().Delete(context.WithoutCancel(c.Request().Context()), true, a); err != nil {
		c.Error(err, "delete attachment")
		return
	}
	c.NoContent()
}
