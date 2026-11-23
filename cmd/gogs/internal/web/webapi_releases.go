package web

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/repox"
)

// releasesPageSize is how many tags a page of releases lists.
const releasesPageSize = 10

func releaseAttachmentLimits(release conf.ReleaseOpts) attachmentLimits {
	return attachmentLimits(release.Attachment)
}

// limitReleaseAttachmentUpload caps the size of an attachment uploaded for a
// release.
func limitReleaseAttachmentUpload(c flamego.Context, release conf.ReleaseOpts) {
	limitAttachmentUpload(c, releaseAttachmentLimits(release))
}

// withRepoCode lets through the viewers who can read the code of a repository
// that has commits, which is where releases live.
func withRepoCode(c flamego.Context, repoCtx *repoContext) {
	if !repoCtx.Viewer.CanRead() || repoCtx.Repo.IsEmpty {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoRelease injects the release of the tag in the route.
func withRepoRelease(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) {
	w := c.ResponseWriter()
	tag := c.Param("tag")
	ctx := c.Request().Context()
	rel, err := database.Handle.Releases().GetByTagName(ctx, repoCtx.Repo.ID, tag)
	if err != nil {
		if database.IsErrReleaseNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("release does not exist"))
			return
		}
		logger.Error("Failed to get release", "tagName", tag, "repoID", repoCtx.Repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get release"))
		return
	}
	if err = database.Handle.Releases().LoadAttributes(ctx, rel); err != nil {
		logger.Error("Failed to load release attributes", "releaseID", rel.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "load release"))
		return
	}
	c.Map(rel)
}

// openRepoGit opens the Git repository of the repository context, which is
// stored under root.
func openRepoGit(root string, repoCtx *repoContext) (*git.Repository, error) {
	return git.Open(repox.RepositoryPath(root, repoCtx.Owner.Name, repoCtx.Repo.Name))
}

type repoReleaseAttachment struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// toReleaseAttachments returns the files of the release its note does not
// link to, i.e., its downloads. Their URLs are under subpath.
func toReleaseAttachments(subpath string, attachments []*database.Attachment, note string) []repoReleaseAttachment {
	list := make([]repoReleaseAttachment, 0, len(attachments))
	for _, a := range attachments {
		if linksAttachment(note, a.UUID) {
			continue
		}
		list = append(list, repoReleaseAttachment{UUID: a.UUID, Name: a.Name, URL: subpath + "/attachments/" + a.UUID})
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

func getRepoReleases(c flamego.Context, links database.LinkOptions, markupOpts markup.Options, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoReleasesResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoOpts.Root, repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	page, err := gitRepo.TagsAfter(ctx, c.Query("after"), releasesPageSize)
	if err != nil {
		logger.Error("Failed to list tags", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}
	published, err := database.Handle.Releases().ListPublishedByRepoID(ctx, repo.ID, page.Tags...)
	if err != nil {
		logger.Error("Failed to list published releases", "repoID", repo.ID, "error", err)
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
			logger.Error("Failed to get tag commit", "tagName", tag, "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get tag commit")
		}
		numCommits, err := commit.CommitsCount(ctx)
		if err != nil {
			logger.Error("Failed to count commits", "tagName", tag, "repoID", repo.ID, "error", err)
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
		drafts, err := database.Handle.Releases().ListDraftsByRepoID(ctx, repo.ID)
		if err != nil {
			logger.Error("Failed to list draft releases", "repoID", repo.ID, "error", err)
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
	metas := repo.ComposeMetas(markupOpts.Subpath, repoOpts.Root)
	notes := make([]string, 0, len(releases))
	for _, r := range releases {
		notes = append(notes, r.Note)
	}
	commits := markup.ResolveCommits(markupOpts, metas, notes...)
	for _, r := range releases {
		item := repoRelease{TagName: r.TagName, Sha: r.Sha1}
		if r.ID > 0 {
			if err = database.Handle.Releases().LoadAttributes(ctx, r); err != nil {
				logger.Error("Failed to load release attributes", "releaseID", r.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "load release")
			}
			publisher := toIssueUser(links, r.Publisher)
			item.Title = r.Title
			item.Target = r.Target
			item.IsDraft = r.IsDraft
			item.IsPrerelease = r.IsPrerelease
			item.Publisher = &publisher
			item.NoteHTML = markAppLinks(markupOpts, markup.MarkdownWithCommits(markupOpts, r.Note, repo.Link(markupOpts.Subpath), metas, commits), repo)
			item.Attachments = toReleaseAttachments(markupOpts.Subpath, r.Attachments, r.Note)
			if r.CreatedUnix > 0 {
				item.CreatedAt = &r.Created
			}
			count, err := counter.count(ctx, r)
			if err != nil {
				logger.Error("Failed to count commits", "branch", r.Target, "repoID", repo.ID, "error", err)
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

func getRepoNewRelease(c flamego.Context, release conf.ReleaseOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoReleaseFormOptions, err error) {
	gitRepo, err := openRepoGit(repoOpts.Root, repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	branches, err := gitRepo.Branches(c.Request().Context())
	if err != nil {
		logger.Error("Failed to list branches", "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	return http.StatusOK, &repoReleaseFormOptions{
		Branches:      branches,
		DefaultBranch: repoCtx.Repo.DefaultBranch,
		Attachment:    attachmentSettings(releaseAttachmentLimits(release)),
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
	// RemovedAttachments are the UUIDs of the downloads the editor removed. The
	// others it does not send were attached after it loaded, e.g., by someone
	// else, and stay.
	RemovedAttachments []string `json:"removedAttachments"`
}

func postRepoRelease(c flamego.Context, links database.LinkOptions, release conf.ReleaseOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, req repoReleaseRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	if strings.TrimSpace(req.TagName) == "" {
		msg := l.Tr("repo::release.tag_name") + l.Tr("form::require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"tagName": &msg}}, nil
	}
	if resp := textFieldError(l, "title", "repo::release.title", req.Title, 255); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if statusCode, err = checkNewAttachments(ctx, releaseAttachmentLimits(release), req.Attachments); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check attachments", "error", err)
		}
		return statusCode, nil, err
	}

	gitRepo, err := openRepoGit(repoOpts.Root, repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if !gitRepo.HasBranch(ctx, req.Target) {
		msg := l.Tr("form::target_branch_not_exist")
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
		logger.Error("Failed to get branch commit", "branch", req.Target, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}
	numCommits, err := commit.CommitsCount(ctx)
	if err != nil {
		logger.Error("Failed to count commits", "branch", req.Target, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
	}

	rel := &database.Release{
		RepoID:       repo.ID,
		PublisherID:  viewer.ID,
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
	if err = database.Handle.Releases().Create(ctx, logger, links, gitRepo, rel, req.Attachments); err != nil {
		var msg string
		switch {
		case database.IsErrReleaseAlreadyExist(err):
			statusCode, msg = http.StatusUnprocessableEntity, l.Tr("repo::release.tag_name_already_exist")
		case database.IsErrInvalidTagName(err):
			statusCode, msg = http.StatusBadRequest, l.Tr("repo::release.tag_name_invalid")
		default:
			logger.Error("Failed to create release", "tagName", req.TagName, "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "create release")
		}
		return statusCode, &bindingErrorResponse{Fields: fieldErrors{"tagName": &msg}}, nil
	}
	logger.Debug("Release created", "userName", viewer.LowerName, "repoName", repo.Name, "tagName", rel.TagName)
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

func getRepoRelease(server conf.ServerOpts, release conf.ReleaseOpts, rel *database.Release) (statusCode int, resp *repoReleaseEdit, err error) {
	return http.StatusOK, &repoReleaseEdit{
		TagName:      rel.TagName,
		Target:       rel.Target,
		Title:        rel.Title,
		Content:      rel.Note,
		IsDraft:      rel.IsDraft,
		IsPrerelease: rel.IsPrerelease,
		Attachments:  toReleaseAttachments(server.Subpath, rel.Attachments, rel.Note),
		Attachment:   attachmentSettings(releaseAttachmentLimits(release)),
	}, nil
}

func patchRepoRelease(c flamego.Context, att conf.AttachmentOpts, links database.LinkOptions, release conf.ReleaseOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, l i18n.Locale, viewer *database.User, repoCtx *repoContext, rel *database.Release, req repoReleaseRequest) (statusCode int, resp any, err error) {
	if resp := textFieldError(l, "title", "repo::release.title", req.Title, 255); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	// The editor sends the downloads it keeps and the new files. The files the
	// note no longer links to and the downloads the editor removed are deleted
	// rather than left without a release, and the others stay. Files the note
	// links to do not count toward the limit.
	var unlinked []*database.Attachment
	var noteLinked []string
	for _, a := range rel.Attachments {
		switch {
		case slices.Contains(req.Attachments, a.UUID):
		case linksAttachment(req.Content, a.UUID):
			noteLinked = append(noteLinked, a.UUID)
		case linksAttachment(rel.Note, a.UUID), slices.Contains(req.RemovedAttachments, a.UUID):
			unlinked = append(unlinked, a)
		default:
			req.Attachments = append(req.Attachments, a.UUID)
		}
	}

	var added []string
	for _, uuid := range req.Attachments {
		if !slices.ContainsFunc(rel.Attachments, func(a *database.Attachment) bool { return a.UUID == uuid }) {
			added = append(added, uuid)
		}
	}
	lim := releaseAttachmentLimits(release)
	if len(req.Attachments) > lim.MaxFiles && len(added) > 0 {
		return http.StatusBadRequest, nil, errors.Newf("at most %d files can be attached", lim.MaxFiles)
	}
	if statusCode, err = checkNewAttachments(c.Request().Context(), lim, added); err != nil {
		if statusCode == http.StatusInternalServerError {
			logger.Error("Failed to check attachments", "error", err)
		}
		return statusCode, nil, err
	}

	req.Attachments = append(req.Attachments, noteLinked...)

	gitRepo, err := openRepoGit(repoOpts.Root, repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	// A published release has its tag, so it cannot go back to a draft.
	isPublish := rel.IsDraft && !req.IsDraft
	rel.Title = req.Title
	rel.Note = req.Content
	rel.IsDraft = rel.IsDraft && req.IsDraft
	rel.IsPrerelease = req.IsPrerelease
	if err = database.Handle.Releases().Update(c.Request().Context(), logger, links, viewer, gitRepo, rel, isPublish, req.Attachments); err != nil {
		switch {
		case database.IsErrInvalidTagName(err):
			// The tag name cannot be edited, so it is not a field error.
			return http.StatusBadRequest, nil, errors.New(l.Tr("repo::release.tag_name_invalid"))
		case database.IsErrReleaseNotExist(err):
			return http.StatusNotFound, nil, errors.New("release does not exist")
		}
		logger.Error("Failed to update release", "releaseID", rel.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update release")
	}
	// The release is already saved without the files, so they have to go even
	// when the client goes away.
	if err = database.Handle.Attachments().Delete(context.WithoutCancel(c.Request().Context()), att.Path, true, unlinked...); err != nil {
		logger.Error("Failed to delete unlinked files", "releaseID", rel.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete unlinked files")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoRelease(c flamego.Context, att conf.AttachmentOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext, rel *database.Release) (statusCode int, resp any, err error) {
	if err = database.Handle.Releases().DeleteByID(c.Request().Context(), logger, repoOpts.Root, att.Path, repoCtx.Repo.ID, rel.ID); err != nil {
		logger.Error("Failed to delete release", "releaseID", rel.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete release")
	}
	return http.StatusNoContent, nil, nil
}

// postRepoReleaseAttachment stores a file to attach to a release.
func postRepoReleaseAttachment(c flamego.Context, server conf.ServerOpts, att conf.AttachmentOpts, release conf.ReleaseOpts, logger *logx.Logger, l i18n.Locale, viewer *database.User) (statusCode int, resp any, err error) {
	statusCode, resp, err = storeAttachment(c, l, att.Path, server.ExternalURL, viewer, releaseAttachmentLimits(release))
	if statusCode == http.StatusInternalServerError {
		logger.Error("Failed to store attachment", "error", err)
	}
	return statusCode, resp, err
}
