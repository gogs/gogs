package web

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/repox"
)

// releasesPageSize is how many tags a page of releases lists.
const releasesPageSize = 10

func releaseAttachmentLimits() attachmentLimits {
	return attachmentLimits(conf.Release.Attachment)
}

// withRepoCode lets through the viewers who can read the code of a repository
// that has commits, which is where releases live.
func withRepoCode(c flamego.Context, repoCtx *repoContext) {
	if !repoCtx.Viewer.CanRead() || repoCtx.Repo.IsEmpty {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoRelease injects the release of the tag in the route.
func withRepoRelease(c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	tag := c.Param("tag")
	rel, err := database.GetRelease(repoCtx.Repo.ID, tag)
	if err != nil {
		if database.IsErrReleaseNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("release does not exist"))
			return
		}
		log.Error("withRepoRelease: get release %q of repo %d: %v", tag, repoCtx.Repo.ID, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get release"))
		return
	}
	if err = rel.LoadAttributes(); err != nil {
		log.Error("withRepoRelease: load attributes of release %d: %v", rel.ID, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "load release"))
		return
	}
	c.Map(rel)
}

func openRepoGit(repoCtx *repoContext) (*git.Repository, error) {
	return git.Open(repox.RepositoryPath(repoCtx.Owner.Name, repoCtx.Repo.Name))
}

type repoReleaseAttachment struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func toReleaseAttachments(attachments []*database.Attachment) []repoReleaseAttachment {
	list := make([]repoReleaseAttachment, 0, len(attachments))
	for _, a := range attachments {
		list = append(list, repoReleaseAttachment{UUID: a.UUID, Name: a.Name, URL: conf.Server.Subpath + "/attachments/" + a.UUID})
	}
	return list
}

type repoRelease struct {
	TagName string `json:"tagName"`
	Sha     string `json:"sha"`
	// The fields below are only set for a release, not for a tag that has no
	// release.
	Title        string                  `json:"title,omitempty"`
	Target       string                  `json:"target,omitempty"`
	IsDraft      bool                    `json:"isDraft,omitempty"`
	IsPrerelease bool                    `json:"isPrerelease,omitempty"`
	Publisher    *issueUser              `json:"publisher,omitempty"`
	CreatedAt    *time.Time              `json:"createdAt,omitempty"`
	NoteHTML     string                  `json:"noteHTML,omitempty"`
	Attachments  []repoReleaseAttachment `json:"attachments,omitempty"`
	// NumCommitsBehind is how many commits the target branch is ahead of the
	// release.
	NumCommitsBehind int64 `json:"numCommitsBehind,omitempty"`
}

type repoReleasesResponse struct {
	Releases []repoRelease `json:"releases"`
	// HasPrevious is true when the page does not start with the latest tag, and
	// PreviousAfter is then the "after" of the previous page.
	HasPrevious   bool   `json:"hasPrevious"`
	PreviousAfter string `json:"previousAfter"`
	// NextAfter is the "after" of the next page, or empty on the last page.
	NextAfter      string `json:"nextAfter,omitempty"`
	ViewerCanWrite bool   `json:"viewerCanWrite"`
	IsMirror       bool   `json:"isMirror"`
}

func getRepoReleases(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoReleasesResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoReleases: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	page, err := gitRepo.TagsAfter(ctx, c.Query("after"), releasesPageSize)
	if err != nil {
		log.Error("getRepoReleases: list tags of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}
	published, err := database.GetPublishedReleasesByRepoID(repo.ID, page.Tags...)
	if err != nil {
		log.Error("getRepoReleases: list published releases of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list releases")
	}
	byTag := make(map[string]*database.Release, len(published))
	for _, r := range published {
		byTag[r.TagName] = r
	}

	counter := &commitsCounter{gitRepo: gitRepo, counts: map[string]int64{}}
	releases := make([]*database.Release, 0, len(page.Tags))
	for _, tag := range page.Tags {
		if r := byTag[tag]; r != nil {
			releases = append(releases, r)
			continue
		}

		commit, err := gitRepo.TagCommit(ctx, tag)
		if err != nil {
			log.Error("getRepoReleases: get commit of tag %q in repo %d: %v", tag, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get tag commit")
		}
		numCommits, err := commit.CommitsCount(ctx)
		if err != nil {
			log.Error("getRepoReleases: count commits of tag %q in repo %d: %v", tag, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
		}
		releases = append(releases, &database.Release{
			TagName:    tag,
			Sha1:       commit.ID.String(),
			NumCommits: numCommits,
		})
	}
	database.SortReleases(releases)

	// Drafts are listed first, on the latest page, and only to writers who can
	// publish them.
	viewerCanWrite := repoCtx.Viewer.CanWrite()
	if viewerCanWrite && page.HasLatest {
		drafts, err := database.GetDraftReleasesByRepoID(repo.ID)
		if err != nil {
			log.Error("getRepoReleases: list draft releases of repo %d: %v", repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "list drafts")
		}
		releases = append(drafts, releases...)
	}

	resp = &repoReleasesResponse{
		Releases:       make([]repoRelease, 0, len(releases)),
		HasPrevious:    !page.HasLatest,
		PreviousAfter:  page.PreviousAfter,
		ViewerCanWrite: viewerCanWrite,
		IsMirror:       repo.IsMirror,
	}
	for _, r := range releases {
		item := repoRelease{TagName: r.TagName, Sha: r.Sha1}
		if r.ID > 0 {
			if err = r.LoadAttributes(); err != nil {
				log.Error("getRepoReleases: load attributes of release %d: %v", r.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "load release")
			}
			publisher := toIssueUser(r.Publisher)
			item.Title = r.Title
			item.Target = r.Target
			item.IsDraft = r.IsDraft
			item.IsPrerelease = r.IsPrerelease
			item.Publisher = &publisher
			item.NoteHTML = string(markup.Markdown(r.Note, repo.Link(), repo.ComposeMetas()))
			item.Attachments = toReleaseAttachments(r.Attachments)
			if r.CreatedUnix > 0 {
				item.CreatedAt = &r.Created
			}
			count, err := counter.count(ctx, r)
			if err != nil {
				log.Error("getRepoReleases: count commits of %q in repo %d: %v", r.Target, repo.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
			}
			item.NumCommitsBehind = count - r.NumCommits
		}
		resp.Releases = append(resp.Releases, item)
	}
	// Pages follow the order of the tags in Git, not the sorted releases.
	if page.HasNext && len(page.Tags) > 0 {
		resp.NextAfter = page.Tags[len(page.Tags)-1]
	}
	return http.StatusOK, resp, nil
}

// commitsCounter counts the commits of the targets of releases, once per
// target.
type commitsCounter struct {
	gitRepo *git.Repository
	counts  map[string]int64
}

func (c *commitsCounter) count(ctx context.Context, r *database.Release) (int64, error) {
	if n, ok := c.counts[r.Target]; ok {
		return n, nil
	}
	// A deleted target counts as the commits of the first release seen on it.
	n := r.NumCommits
	if c.gitRepo.HasBranch(ctx, r.Target) {
		commit, err := c.gitRepo.BranchCommit(ctx, r.Target)
		if err != nil {
			return 0, errors.Wrap(err, "get branch commit")
		}
		if n, err = commit.CommitsCount(ctx); err != nil {
			return 0, errors.Wrap(err, "count commits")
		}
	}
	c.counts[r.Target] = n
	return n, nil
}

type repoReleaseFormOptions struct {
	Branches      []string                 `json:"branches"`
	DefaultBranch string                   `json:"defaultBranch"`
	Attachment    *issueAttachmentSettings `json:"attachment,omitempty"`
}

func getRepoNewRelease(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoReleaseFormOptions, err error) {
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoNewRelease: open repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	branches, err := gitRepo.Branches(c.Request().Context())
	if err != nil {
		log.Error("getRepoNewRelease: list branches of repo %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	return http.StatusOK, &repoReleaseFormOptions{
		Branches:      branches,
		DefaultBranch: repoCtx.Repo.DefaultBranch,
		Attachment:    attachmentSettings(releaseAttachmentLimits()),
	}, nil
}

type repoReleaseRequest struct {
	// TagName and Target are only read when a release is created.
	TagName      string   `json:"tagName"`
	Target       string   `json:"target"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	IsDraft      bool     `json:"isDraft"`
	IsPrerelease bool     `json:"isPrerelease"`
	Attachments  []string `json:"attachments"`
}

func postRepoRelease(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoReleaseRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	if strings.TrimSpace(req.TagName) == "" {
		msg := l.Tr("repo.release.tag_name") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"tagName": &msg}}, nil
	}
	if resp := textFieldError(l, "title", "repo.release.title", req.Title, 255); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if statusCode, err = checkNewAttachments(releaseAttachmentLimits(), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("postRepoRelease: %v", err)
		}
		return statusCode, nil, err
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("postRepoRelease: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if !gitRepo.HasBranch(ctx, req.Target) {
		msg := l.Tr("form.target_branch_not_exist")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"target": &msg}}, nil
	}

	// An existing tag keeps the time it was made, a new one is made now.
	var tagCreatedUnix int64
	if tag, err := gitRepo.Tag(ctx, git.RefsTags+req.TagName); err == nil {
		if commit, err := tag.Commit(ctx); err == nil {
			tagCreatedUnix = commit.Author.When.Unix()
		}
	}
	commit, err := gitRepo.BranchCommit(ctx, req.Target)
	if err != nil {
		log.Error("postRepoRelease: get commit of branch %q in repo %d: %v", req.Target, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}
	numCommits, err := commit.CommitsCount(ctx)
	if err != nil {
		log.Error("postRepoRelease: count commits of branch %q in repo %d: %v", req.Target, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
	}

	rel := &database.Release{
		RepoID:       repo.ID,
		PublisherID:  user.ID,
		Title:        req.Title,
		TagName:      req.TagName,
		Target:       req.Target,
		Sha1:         commit.ID.String(),
		NumCommits:   numCommits,
		Note:         req.Content,
		IsDraft:      req.IsDraft,
		IsPrerelease: req.IsPrerelease,
		CreatedUnix:  tagCreatedUnix,
	}
	if err = database.NewRelease(ctx, gitRepo, rel, req.Attachments); err != nil {
		var msg string
		switch {
		case database.IsErrReleaseAlreadyExist(err):
			statusCode, msg = http.StatusUnprocessableEntity, l.Tr("repo.release.tag_name_already_exist")
		case database.IsErrInvalidTagName(err):
			statusCode, msg = http.StatusBadRequest, l.Tr("repo.release.tag_name_invalid")
		default:
			log.Error("postRepoRelease: create release %q in repo %d: %v", req.TagName, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "create release")
		}
		return statusCode, &bindingErrorResponse{Fields: fieldErrors{"tagName": &msg}}, nil
	}
	log.Trace("Release created: %s/%s:%s", user.LowerName, repo.Name, rel.TagName)
	return http.StatusCreated, nil, nil
}

type repoReleaseEdit struct {
	TagName      string                   `json:"tagName"`
	Target       string                   `json:"target"`
	Title        string                   `json:"title"`
	Content      string                   `json:"content"`
	IsDraft      bool                     `json:"isDraft"`
	IsPrerelease bool                     `json:"isPrerelease"`
	Attachments  []repoReleaseAttachment  `json:"attachments"`
	Attachment   *issueAttachmentSettings `json:"attachment,omitempty"`
}

func getRepoRelease(rel *database.Release) (statusCode int, resp *repoReleaseEdit, err error) {
	return http.StatusOK, &repoReleaseEdit{
		TagName:      rel.TagName,
		Target:       rel.Target,
		Title:        rel.Title,
		Content:      rel.Note,
		IsDraft:      rel.IsDraft,
		IsPrerelease: rel.IsPrerelease,
		Attachments:  toReleaseAttachments(rel.Attachments),
		Attachment:   attachmentSettings(releaseAttachmentLimits()),
	}, nil
}

func patchRepoRelease(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, rel *database.Release, req repoReleaseRequest) (statusCode int, resp any, err error) {
	if resp := textFieldError(l, "title", "repo.release.title", req.Title, 255); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	// The files already attached to the release stay unless left out, the
	// others must be new.
	var added []string
	for _, uuid := range req.Attachments {
		if !slices.ContainsFunc(rel.Attachments, func(a *database.Attachment) bool { return a.UUID == uuid }) {
			added = append(added, uuid)
		}
	}
	lim := releaseAttachmentLimits()
	if len(req.Attachments) > lim.MaxFiles && len(added) > 0 {
		return http.StatusBadRequest, nil, errors.Newf("at most %d files can be attached", lim.MaxFiles)
	}
	if statusCode, err = checkNewAttachments(lim, added); err != nil {
		if statusCode == http.StatusInternalServerError {
			log.Error("patchRepoRelease: %v", err)
		}
		return statusCode, nil, err
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("patchRepoRelease: open repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	// A published release has its tag, so it cannot go back to a draft.
	isPublish := rel.IsDraft && !req.IsDraft
	rel.Title = req.Title
	rel.Note = req.Content
	rel.IsDraft = rel.IsDraft && req.IsDraft
	rel.IsPrerelease = req.IsPrerelease
	if err = database.UpdateRelease(c.Request().Context(), user, gitRepo, rel, isPublish, req.Attachments); err != nil {
		if database.IsErrInvalidTagName(err) {
			msg := l.Tr("repo.release.tag_name_invalid")
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"tagName": &msg}}, nil
		}
		log.Error("patchRepoRelease: update release %d: %v", rel.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update release")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoRelease(c flamego.Context, repoCtx *repoContext, rel *database.Release) (statusCode int, resp any, err error) {
	if err = database.DeleteReleaseOfRepoByID(c.Request().Context(), repoCtx.Repo.ID, rel.ID); err != nil {
		log.Error("deleteRepoRelease: delete release %d: %v", rel.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete release")
	}
	return http.StatusNoContent, nil, nil
}

// postRepoReleaseAttachment stores a file to attach to a release.
func postRepoReleaseAttachment(c flamego.Context, l i18n.Locale) (statusCode int, resp any, err error) {
	statusCode, resp, err = storeAttachment(c, l, releaseAttachmentLimits())
	if statusCode == http.StatusInternalServerError {
		log.Error("postRepoReleaseAttachment: %v", err)
	}
	return statusCode, resp, err
}
