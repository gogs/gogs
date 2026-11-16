package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/mimex"
	"gogs.io/gogs/internal/repox"
)

// whitespaceFlag maps the `whitespace` query value to its `git diff` flag.
// `ignore-change` (`-b`) still surfaces added/removed blank lines, unlike
// `ignore-all` (`-w`). Empty or unknown values disable whitespace handling.
func whitespaceFlag(v string) string {
	switch v {
	case "ignore-all":
		return "-w"
	case "ignore-change":
		return "-b"
	default:
		return ""
	}
}

func writeErrorResponse(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	message := err.Error()
	// Match the JSON-API ReturnHandler: in prod, never leak 5xx detail.
	if code >= http.StatusInternalServerError && conf.IsProdMode() {
		message = "Internal server error"
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func getRepoCommitRaw(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.Viewer.CanRead() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}

	owner := repoCtx.Owner
	repo := repoCtx.Repo
	sha := c.Param("sha")
	format := c.Param("format")

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		logger.Error("Failed to open repository", "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}

	if _, err = gitRepo.CatFileCommit(c.Request().Context(), sha); err != nil {
		if git.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("commit does not exist"))
			return
		}
		logger.Error("Failed to get commit", "commitID", sha, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "cat-file commit"))
		return
	}

	var rawOpts []git.RawDiffOptions
	if flag := whitespaceFlag(c.Request().URL.Query().Get("whitespace")); flag != "" {
		rawOpts = append(rawOpts, git.RawDiffOptions{
			Args: []string{flag},
		})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err = gitRepo.RawDiff(c.Request().Context(), sha, git.RawDiffFormat(format), w, rawOpts...); err != nil {
		logger.Error("Failed to get raw diff", "commitID", sha, "error", err)
	}
}

// getRepoCompareRaw writes the diff from one commit to another as text.
func getRepoCompareRaw(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.Viewer.CanRead() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}

	owner := repoCtx.Owner
	repo := repoCtx.Repo
	before, after := c.Param("before"), c.Param("after")
	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		logger.Error("Failed to open repository", "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}
	r, err := resolveCompareRange(c.Request().Context(), gitRepo, repo.DefaultBranch, before, after)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("commit does not exist"))
			return
		}
		logger.Error("Failed to compare commits", "before", before, "after", after, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "compare commits"))
		return
	}

	opt := git.RawDiffOptions{Base: r.base}
	if flag := whitespaceFlag(c.Request().URL.Query().Get("whitespace")); flag != "" {
		opt.Args = []string{flag}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err = gitRepo.RawDiff(c.Request().Context(), after, git.RawDiffNormal, w, opt); err != nil {
		logger.Error("Failed to get raw diff", "before", before, "after", after, "error", err)
	}
}

// resolveRef resolves ref as a commit SHA, then a branch name, then a tag
// name, in that order. A branch and tag of the same name resolve to the branch.
func resolveRef(ctx context.Context, gitRepo *git.Repository, ref string) (*git.Commit, error) {
	commit, err := gitRepo.CatFileCommit(ctx, ref)
	if err == nil {
		return commit, nil
	}
	if !git.IsErrRevisionNotExist(err) {
		return nil, errors.Wrap(err, "cat-file commit")
	}
	commit, err = gitRepo.BranchCommit(ctx, ref)
	if err == nil {
		return commit, nil
	}
	if !git.IsErrRevisionNotExist(err) {
		return nil, errors.Wrap(err, "get branch commit")
	}
	commit, err = gitRepo.TagCommit(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "get tag commit")
	}
	return commit, nil
}

func getRepoRawFile(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.Viewer.CanRead() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}

	owner := repoCtx.Owner
	repo := repoCtx.Repo
	rawRef := c.Param("ref")
	filepath := c.Param("filepath")

	ref, err := url.PathUnescape(rawRef)
	if err != nil {
		writeErrorResponse(w, http.StatusNotFound, errors.New("ref does not exist"))
		return
	}

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		logger.Error("Failed to open repository", "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}

	commit, err := resolveRef(c.Request().Context(), gitRepo, ref)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("ref does not exist"))
			return
		}
		logger.Error("Failed to resolve ref", "ref", ref, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "resolve ref"))
		return
	}

	entry, err := commit.TreeEntry(c.Request().Context(), filepath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("file does not exist"))
			return
		}
		logger.Error("Failed to get tree entry", "commitID", commit.ID, "path", filepath, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get tree entry"))
		return
	}
	// The content of a symlink is its target path, which is served like GitHub
	// does instead of following the link.
	if !entry.IsBlob() && !entry.IsExec() && !entry.IsSymlink() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("file does not exist"))
		return
	}

	data, err := entry.Blob().Bytes(c.Request().Context())
	if err != nil {
		logger.Error("Failed to read blob", "commitID", commit.ID, "path", filepath, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "read blob"))
		return
	}

	if pathCommit, err := commit.CommitByPath(c.Request().Context(), git.CommitByRevisionOptions{Path: filepath}); err == nil && pathCommit != nil {
		w.Header().Set("Last-Modified", pathCommit.Committer.When.Format(http.TimeFormat))
	}

	render, _ := strconv.ParseBool(c.Request().URL.Query().Get("render"))
	switch {
	case strings.EqualFold(path.Ext(filepath), ".svg"):
		// Serve SVG as an image so READMEs can show it, and sandbox it so its
		// scripts never run when it is opened directly.
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	case !mimex.IsText(data) && !mimex.IsImage(data):
		w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(filepath)+`"`)
		w.Header().Set("Content-Transfer-Encoding", "binary")
	case mimex.IsText(data) && (!conf.Repository.EnableRawFileRenderMode || !render):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	_, _ = w.Write(data)
}

// readableRepo returns the repository in the path when the viewer can read
// it, and nil otherwise. It writes nothing, so the web app renders the page and
// tells the viewer what is wrong.
func readableRepo(c flamego.Context, logger *logx.Logger, viewer *database.User) *database.Repository {
	if viewer == nil && conf.Auth.RequireSigninView || viewer != nil && (viewer.ProhibitLogin || viewer.IsDisabled()) {
		return nil
	}

	ctx := c.Request().Context()
	ownerName := c.Param("owner")
	repoName := c.Param("repo")
	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if !database.IsErrUserNotExist(err) {
			logger.Error("Failed to get repository owner by username", "ownerName", ownerName, "error", err)
		}
		return nil
	}
	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if !database.IsErrRepoNotExist(err) {
			logger.Error("Failed to get repository by name", "ownerName", ownerName, "repoName", repoName, "error", err)
		}
		return nil
	}
	if !newViewer(ctx, logger, viewer, owner, repo).CanRead() {
		return nil
	}
	return repo
}

// maybeRedirectToExternalTracker sends the pages of issues to the external
// issue tracker of the repository when it has one. Pull requests share their
// numbers with issues and keep their pages here.
func maybeRedirectToExternalTracker(c flamego.Context, logger *logx.Logger, viewer *database.User) {
	repo := readableRepo(c, logger, viewer)
	if repo == nil {
		return
	}
	link := repo.ExternalTrackerLink()
	if link == "" {
		return
	}
	if index := c.ParamInt64("index"); index > 0 {
		issue, err := database.Handle.Issues().GetByIndex(c.Request().Context(), repo.ID, index)
		switch {
		case database.IsErrIssueNotExist(err):
		case err != nil:
			logger.Error("Failed to get issue by index", "repoID", repo.ID, "index", index, "error", err)
			return
		case issue.IsPull:
			return
		}
	}
	// The URL is used as is, so that its query and fragment reach the external site.
	c.Redirect(link)
}

// maybeRedirectToExternalWiki sends the pages of the wiki to the external wiki
// of the repository when it has one.
func maybeRedirectToExternalWiki(c flamego.Context, logger *logx.Logger, viewer *database.User) {
	repo := readableRepo(c, logger, viewer)
	if repo == nil {
		return
	}
	if link := repo.ExternalWikiLink(); link != "" {
		// The URL is used as is, so that its query and fragment reach the external site.
		c.Redirect(link)
	}
}
