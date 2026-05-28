package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/gogs/git-module"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/gitx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/tool"
	log "unknwon.dev/clog/v2"
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

func getRepoCommitRaw(c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.ViewerCanRead() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("repository does not exist"))
		return
	}

	owner := repoCtx.Owner
	repo := repoCtx.Repo
	sha := c.Param("sha")
	format := c.Param("format")

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		log.Error("getRepoCommitRaw: open repository %q/%q: %v", owner.Name, repo.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}

	if _, err = gitRepo.CatFileCommit(sha); err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("commit does not exist"))
			return
		}
		log.Error("getRepoCommitRaw: cat-file commit %q in %q/%q: %v", sha, owner.Name, repo.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "cat-file commit"))
		return
	}

	var rawOpts []git.RawDiffOptions
	if flag := whitespaceFlag(c.Request().URL.Query().Get("whitespace")); flag != "" {
		rawOpts = append(rawOpts, git.RawDiffOptions{
			CommandOptions: git.CommandOptions{Args: []string{flag}},
		})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err = gitRepo.RawDiff(sha, git.RawDiffFormat(format), w, rawOpts...); err != nil {
		log.Error("getRepoCommitRaw: get raw diff %s: %v", sha, err)
	}
}

// resolveRef resolves ref as a commit SHA, then a branch name, then a tag
// name, in that order. A branch and tag of the same name resolve to the branch.
func resolveRef(gitRepo *git.Repository, ref string) (*git.Commit, error) {
	commit, err := gitRepo.CatFileCommit(ref)
	if err == nil {
		return commit, nil
	}
	if !gitx.IsErrRevisionNotExist(err) {
		return nil, errors.Wrap(err, "cat-file commit")
	}
	commit, err = gitRepo.BranchCommit(ref)
	if err == nil {
		return commit, nil
	}
	if !gitx.IsErrRevisionNotExist(err) {
		return nil, errors.Wrap(err, "get branch commit")
	}
	commit, err = gitRepo.TagCommit(ref)
	if err != nil {
		return nil, errors.Wrap(err, "get tag commit")
	}
	return commit, nil
}

func getRepoRawFile(c flamego.Context, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !repoCtx.ViewerCanRead() {
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
		log.Error("getRepoRawFile: open repository %q/%q: %v", owner.Name, repo.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}

	commit, err := resolveRef(gitRepo, ref)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("ref does not exist"))
			return
		}
		log.Error("getRepoRawFile: resolve ref %q in %q/%q: %v", ref, owner.Name, repo.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "resolve ref"))
		return
	}

	blob, err := commit.Blob(filepath)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) || errors.Is(err, git.ErrNotBlob) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("file does not exist"))
			return
		}
		log.Error("getRepoRawFile: blob %s:%s in %q/%q: %v", commit.ID, filepath, owner.Name, repo.Name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get blob"))
		return
	}

	data, err := blob.Bytes()
	if err != nil {
		log.Error("getRepoRawFile: read blob %s:%s: %v", commit.ID, filepath, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "read blob"))
		return
	}

	if pathCommit, err := commit.CommitByPath(git.CommitByRevisionOptions{Path: filepath}); err == nil && pathCommit != nil {
		w.Header().Set("Last-Modified", pathCommit.Committer.When.Format(http.TimeFormat))
	}

	render, _ := strconv.ParseBool(c.Request().URL.Query().Get("render"))
	switch {
	case !tool.IsTextFile(data) && !tool.IsImageFile(data):
		w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(filepath)+`"`)
		w.Header().Set("Content-Transfer-Encoding", "binary")
	case tool.IsTextFile(data) && (!conf.Repository.EnableRawFileRenderMode || !render):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	_, _ = w.Write(data)
}
