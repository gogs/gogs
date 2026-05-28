package web

import (
	stdctx "context"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/gogs/git-module"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/strx"
	"gogs.io/gogs/internal/tool"
)

type repoHeader struct {
	ID         int64  `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatarURL"`
	Visibility string `json:"visibility"`
	MirrorOf   string `json:"mirrorOf,omitempty"`

	WatchCount           int  `json:"watchCount"`
	StarCount            int  `json:"starCount"`
	ForkCount            int  `json:"forkCount"`
	IssuesEnabled        bool `json:"issuesEnabled"`
	OpenIssueCount       int  `json:"openIssueCount"`
	PullRequestsEnabled  bool `json:"pullRequestsEnabled"`
	OpenPullRequestCount int  `json:"openPullRequestCount"`
	WikiEnabled          bool `json:"wikiEnabled"`

	IsViewerAdmin    bool `json:"isViewerAdmin"`
	IsViewerWatching bool `json:"isViewerWatching"`
	IsViewerStarring bool `json:"isViewerStarring"`
}

func getRepoHeader(c flamego.Context, user *database.User) (statusCode int, resp *repoHeader, err error) {
	ctx := c.Request().Context()
	ownerName := c.Param("owner")
	repoName := c.Param("repo")

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoHeader: get user by username %q: %v", ownerName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by username")
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoHeader: get repo by name %q/%q: %v", ownerName, repoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repo by name")
	}

	var viewerID int64
	if user != nil {
		viewerID = user.ID
	}

	// Site admins get owner-level access on every repo.
	var mode database.AccessMode
	if user != nil && user.IsAdmin {
		mode = database.AccessModeOwner
	} else {
		mode = database.Handle.Permissions().AccessMode(ctx, viewerID, repo.ID, database.AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		})
	}

	// Viewer can see the header if they have read access OR the repo exposes
	// issues/wiki publicly. In the partial-public case we mask the feature
	// flags to what the guest is allowed to see.
	issuesEnabled := repo.EnableIssues
	wikiEnabled := repo.EnableWiki
	if mode < database.AccessModeRead {
		if !repo.IsPartialPublic() {
			return http.StatusNotFound, nil, nil
		}
		issuesEnabled = repo.CanGuestViewIssues()
		wikiEnabled = repo.CanGuestViewWiki()
	}

	visibility := "public"
	if repo.IsPrivate {
		visibility = "private"
	}

	resp = &repoHeader{
		ID:         repo.ID,
		Owner:      owner.Name,
		Name:       repo.Name,
		AvatarURL:  strx.Coalesce(repo.AvatarLink(), owner.AvatarURL()),
		Visibility: visibility,

		WatchCount:           repo.NumWatches,
		StarCount:            repo.NumStars,
		ForkCount:            repo.NumForks,
		IssuesEnabled:        issuesEnabled,
		OpenIssueCount:       repo.NumIssues - repo.NumClosedIssues,
		PullRequestsEnabled:  repo.AllowsPulls(),
		OpenPullRequestCount: repo.NumPulls - repo.NumClosedPulls,
		WikiEnabled:          wikiEnabled,

		IsViewerAdmin:    mode >= database.AccessModeAdmin,
		IsViewerWatching: viewerID > 0 && database.IsWatching(viewerID, repo.ID),
		IsViewerStarring: viewerID > 0 && database.IsStaring(viewerID, repo.ID),
	}

	if repo.IsMirror {
		mirror, err := database.GetMirrorByRepoID(repo.ID)
		if err != nil {
			log.Error("getRepoHeader: get mirror by repo ID %d: %v", repo.ID, err)
		} else if mirror != nil {
			resp.MirrorOf = mirror.Address()
		}
	}

	return http.StatusOK, resp, nil
}

type repoCommitSignature struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarURL"`
	UserPath  string `json:"userPath,omitempty"`
	When      string `json:"when"`
}

type repoCommitMeta struct {
	SHA       string              `json:"sha"`
	ShortSHA  string              `json:"shortSha"`
	Summary   string              `json:"summary"`
	Message   string              `json:"message"`
	Author    repoCommitSignature `json:"author"`
	Committer repoCommitSignature `json:"committer"`
	Parents   []string            `json:"parents"`
}

type repoCommit struct {
	Commit     repoCommitMeta `json:"commit"`
	SourcePath string         `json:"sourcePath"`
	RawDiffURL string         `json:"rawDiffURL"`
}

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

func getRepoCommit(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoCommit, err error) {
	ctx := r.Context()
	params := c.Params()
	ownerName := params["owner"]
	repoName := params["repo"]
	commitID := params["sha"]

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoCommit: get user by username %q: %v", ownerName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by username")
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoCommit: get repo by name %q/%q: %v", ownerName, repoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repo by name")
	}
	if repo.IsBare {
		return http.StatusNotFound, nil, nil
	}

	var mode database.AccessMode
	if user != nil && user.IsAdmin {
		mode = database.AccessModeOwner
	} else {
		var viewerID int64
		if user != nil {
			viewerID = user.ID
		}
		mode = database.Handle.Permissions().AccessMode(ctx, viewerID, repo.ID, database.AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		})
	}
	if mode < database.AccessModeRead {
		return http.StatusNotFound, nil, nil
	}

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		log.Error("getRepoCommit: open repository %q/%q: %v", ownerName, repoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	commit, err := gitRepo.CatFileCommit(commitID)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoCommit: cat-file commit %q in %q/%q: %v", commitID, ownerName, repoName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "cat-file commit")
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			log.Error("getRepoCommit: parent ID %d for %q in %q/%q: %v", i, commitID, ownerName, repoName, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parent ID")
		}
		parents[i] = sha.String()
	}

	sig := func(s *git.Signature) repoCommitSignature {
		out := repoCommitSignature{
			Name:      s.Name,
			Email:     s.Email,
			AvatarURL: tool.AvatarLink(s.Email),
			When:      s.When.UTC().Format(time.RFC3339),
		}
		if u, err := database.Handle.Users().GetByEmail(ctx, s.Email); err == nil && u != nil {
			out.UserPath = conf.Server.Subpath + "/" + u.Name
		}
		return out
	}

	body := ""
	if msg := commit.Message; len(msg) > len(commit.Summary()) {
		body = msg[len(commit.Summary()):]
	}

	return http.StatusOK, &repoCommit{
		Commit: repoCommitMeta{
			SHA:       commitID,
			ShortSHA:  tool.ShortSHA1(commitID),
			Summary:   commit.Summary(),
			Message:   body,
			Author:    sig(commit.Author),
			Committer: sig(commit.Committer),
			Parents:   parents,
		},
		SourcePath: conf.Server.Subpath + "/" + path.Join(owner.Name, repo.Name, "src", commitID),
		RawDiffURL: conf.Server.Subpath + "/" + path.Join(owner.Name, repo.Name, "commit", commitID+".diff"),
	}, nil
}

// `{ext}` selects `git diff` (`diff`) vs `git format-patch` (`patch`) output.
func getRepoCommitRaw(c flamego.Context, r *http.Request, user *database.User) {
	w := c.ResponseWriter()
	ctx := r.Context()
	params := c.Params()
	ownerName := params["owner"]
	repoName := params["repo"]
	commitID := params["sha"]
	ext := params["ext"]

	writeStatus := func(code int) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
	}

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoCommitRawDiff: get user by username %q: %v", ownerName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoCommitRawDiff: get repo by name %q/%q: %v", ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}
	if repo.IsBare {
		writeStatus(http.StatusNotFound)
		return
	}

	var mode database.AccessMode
	if user != nil && user.IsAdmin {
		mode = database.AccessModeOwner
	} else {
		var viewerID int64
		if user != nil {
			viewerID = user.ID
		}
		mode = database.Handle.Permissions().AccessMode(ctx, viewerID, repo.ID, database.AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		})
	}
	if mode < database.AccessModeRead {
		writeStatus(http.StatusNotFound)
		return
	}

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		log.Error("getRepoCommitRawDiff: open repository %q/%q: %v", ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	if _, err := gitRepo.CatFileCommit(commitID); err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoCommitRawDiff: cat-file commit %q in %q/%q: %v", commitID, ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	var rawOpts []git.RawDiffOptions
	if flag := whitespaceFlag(r.URL.Query().Get("whitespace")); flag != "" {
		rawOpts = append(rawOpts, git.RawDiffOptions{
			CommandOptions: git.CommandOptions{Args: []string{flag}},
		})
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := gitRepo.RawDiff(commitID, git.RawDiffFormat(ext), w, rawOpts...); err != nil {
		log.Error("getRepoCommitRawDiff: get raw diff %s: %v", commitID, err)
	}
}

func resolveRepoForViewer(c flamego.Context, ctx stdctx.Context, user *database.User) (*database.Repository, int, error) {
	params := c.Params()
	ownerName := params["owner"]
	repoName := params["repo"]
	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, nil
		}
		log.Error("resolveRepoForViewer: get user by username %q: %v", ownerName, err)
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user by username")
	}
	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return nil, http.StatusNotFound, nil
		}
		log.Error("resolveRepoForViewer: get repo by name %q/%q: %v", ownerName, repoName, err)
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get repo by name")
	}
	repo.Owner = owner

	var mode database.AccessMode
	if user.IsAdmin {
		mode = database.AccessModeOwner
	} else {
		mode = database.Handle.Permissions().AccessMode(ctx, user.ID, repo.ID, database.AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		})
	}
	if mode < database.AccessModeRead {
		return nil, http.StatusNotFound, nil
	}
	return repo, 0, nil
}

// No `omitempty`: `false` and `0` are meaningful states the client must see
// (e.g. an unwatch transitions `isViewerWatching` to `false`).
type repoActionResponse struct {
	IsViewerWatching bool `json:"isViewerWatching"`
	IsViewerStarring bool `json:"isViewerStarring"`
	WatchCount       int  `json:"watchCount"`
	StarCount        int  `json:"starCount"`
}

func repoWatchAction(c flamego.Context, r *http.Request, user *database.User, watching bool) (statusCode int, resp *repoActionResponse, err error) {
	if user == nil {
		return http.StatusUnauthorized, nil, nil
	}
	repo, status, err := resolveRepoForViewer(c, r.Context(), user)
	if err != nil || repo == nil {
		return status, nil, err
	}
	// The store layer only exposes `Watch` (not Unwatch) so far, hence the
	// deprecated package-level helper for the unwatch branch.
	if watching {
		err = database.Handle.Repositories().Watch(r.Context(), user.ID, repo.ID)
	} else {
		err = database.WatchRepo(user.ID, repo.ID, false)
	}
	if err != nil {
		log.Error("repoWatchAction: set watching=%t for user %d on repo %d: %v", watching, user.ID, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "watch repo")
	}
	// Reload to get the updated NumWatches count from the after-trigger.
	updated, err := database.Handle.Repositories().GetByName(r.Context(), repo.OwnerID, repo.Name)
	if err != nil {
		log.Error("repoWatchAction: reload repo %d (%q): %v", repo.ID, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoActionResponse{
		IsViewerWatching: watching,
		IsViewerStarring: database.IsStaring(user.ID, repo.ID),
		WatchCount:       updated.NumWatches,
		StarCount:        updated.NumStars,
	}, nil
}

func postRepoWatch(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoWatchAction(c, r, user, true)
}

func deleteRepoWatch(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoWatchAction(c, r, user, false)
}

func repoStarAction(c flamego.Context, r *http.Request, user *database.User, starred bool) (statusCode int, resp *repoActionResponse, err error) {
	if user == nil {
		return http.StatusUnauthorized, nil, nil
	}
	repo, status, err := resolveRepoForViewer(c, r.Context(), user)
	if err != nil || repo == nil {
		return status, nil, err
	}
	// The store layer only exposes `Star` (not Unstar) so far, hence the
	// deprecated package-level helper for the unstar branch.
	if starred {
		err = database.Handle.Repositories().Star(r.Context(), user.ID, repo.ID)
	} else {
		err = database.StarRepo(user.ID, repo.ID, false)
	}
	if err != nil {
		log.Error("repoStarAction: set starred=%t for user %d on repo %d: %v", starred, user.ID, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "star repo")
	}
	updated, err := database.Handle.Repositories().GetByName(r.Context(), repo.OwnerID, repo.Name)
	if err != nil {
		log.Error("repoStarAction: reload repo %d (%q): %v", repo.ID, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoActionResponse{
		IsViewerWatching: database.IsWatching(user.ID, repo.ID),
		IsViewerStarring: starred,
		WatchCount:       updated.NumWatches,
		StarCount:        updated.NumStars,
	}, nil
}

func postRepoStar(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoStarAction(c, r, user, true)
}

func deleteRepoStar(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoStarAction(c, r, user, false)
}

// Slashes inside a ref name (e.g. `feat/foo`) must be percent-encoded as
// `%2F` in the `{ref}` segment so the router can split ref from filepath.
// Bare commit SHAs need no encoding.
func getRepoRawFile(c flamego.Context, r *http.Request, user *database.User) {
	w := c.ResponseWriter()
	ctx := r.Context()

	writeStatus := func(code int) {
		w.WriteHeader(code)
	}

	ownerName := c.Param("owner")
	repoName := c.Param("name")
	rawRef := c.Param("ref")
	filePath := c.Param("file")
	if rawRef == "" || filePath == "" {
		writeStatus(http.StatusNotFound)
		return
	}
	ref, err := url.PathUnescape(rawRef)
	if err != nil {
		writeStatus(http.StatusNotFound)
		return
	}

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRawFile: get user by username %q: %v", ownerName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRawFile: get repo by name %q/%q: %v", ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}
	if repo.IsBare {
		writeStatus(http.StatusNotFound)
		return
	}

	var mode database.AccessMode
	if user != nil && user.IsAdmin {
		mode = database.AccessModeOwner
	} else {
		var viewerID int64
		if user != nil {
			viewerID = user.ID
		}
		mode = database.Handle.Permissions().AccessMode(ctx, viewerID, repo.ID, database.AccessModeOptions{
			OwnerID: repo.OwnerID,
			Private: repo.IsPrivate,
		})
	}
	if mode < database.AccessModeRead {
		writeStatus(http.StatusNotFound)
		return
	}

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		log.Error("getRepoRawFile: open repository %q/%q: %v", ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	commit, err := resolveRef(gitRepo, ref)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRawFile: resolve ref %q in %q/%q: %v", ref, ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	blob, err := commit.Blob(filePath)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) || errors.Is(err, git.ErrNotBlob) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRawFile: blob %s:%s in %q/%q: %v", commit.ID, filePath, ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	data, err := blob.Bytes()
	if err != nil {
		log.Error("getRepoRawFile: read blob %s:%s: %v", commit.ID, filePath, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	if pathCommit, err := commit.CommitByPath(git.CommitByRevisionOptions{Path: filePath}); err == nil && pathCommit != nil {
		w.Header().Set("Last-Modified", pathCommit.Committer.When.Format(http.TimeFormat))
	}

	switch {
	case !tool.IsTextFile(data) && !tool.IsImageFile(data):
		w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(filePath)+`"`)
		w.Header().Set("Content-Transfer-Encoding", "binary")
	case tool.IsTextFile(data) && (!conf.Repository.EnableRawFileRenderMode || r.URL.Query().Get("render") != "true"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	_, _ = w.Write(data)
}

// ref is tried as commit SHA, then branch name, then tag name, so a branch
// and tag of the same name resolve to the branch.
func resolveRef(gitRepo *git.Repository, ref string) (*git.Commit, error) {
	commit, err := gitRepo.CatFileCommit(ref)
	if err == nil {
		return commit, nil
	}
	if !gitx.IsErrRevisionNotExist(err) {
		return nil, err
	}
	commit, err = gitRepo.BranchCommit(ref)
	if err == nil {
		return commit, nil
	}
	if !gitx.IsErrRevisionNotExist(err) {
		return nil, err
	}
	return gitRepo.TagCommit(ref)
}
