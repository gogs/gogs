package web

import (
	stdctx "context"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/gogs/git-module"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/tool"
)

type repoInfoCounts struct {
	Watchers   int `json:"watchers"`
	Stars      int `json:"stars"`
	Forks      int `json:"forks"`
	OpenIssues int `json:"openIssues"`
	OpenPulls  int `json:"openPulls"`
}

type repoInfo struct {
	Owner          string         `json:"owner"`
	Name           string         `json:"name"`
	AvatarURL      string         `json:"avatarURL"`
	Visibility     string         `json:"visibility"`
	IsAdmin        bool           `json:"isAdmin"`
	EnableIssues   bool           `json:"enableIssues"`
	AllowsPulls    bool           `json:"allowsPulls"`
	EnableWiki     bool           `json:"enableWiki"`
	Counts         repoInfoCounts `json:"counts"`
	ViewerWatching bool           `json:"viewerWatching"`
	ViewerStarred  bool           `json:"viewerStarred"`
	MirrorOf       string         `json:"mirrorOf,omitempty"`
}

func getRepoInfo(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoInfo, err error) {
	ctx := r.Context()
	params := c.Params()
	ownerName := params["owner"]
	repoName := params["name"]

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by username")
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repo by name")
	}

	var viewerID int64
	if user != nil {
		viewerID = user.ID
	}

	// Site admins get owner-level access on every repo, mirroring
	// `RepoAssignment` in internal/context/repo.go.
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
	// flags to what the guest is allowed to see, matching `RepoAssignment`.
	enableIssues := repo.EnableIssues
	enableWiki := repo.EnableWiki
	if mode < database.AccessModeRead {
		if !repo.IsPartialPublic() {
			return http.StatusNotFound, nil, nil
		}
		enableIssues = repo.CanGuestViewIssues()
		enableWiki = repo.CanGuestViewWiki()
	}

	repo.Owner = owner
	// `repo.AvatarLink()` panics on an empty `RelAvatarLink()` (which is the
	// common case: most repos don't set a custom avatar), so check that first
	// and fall back to the owner's avatar.
	avatarURL := owner.AvatarURL()
	if repo.RelAvatarLink() != "" {
		avatarURL = repo.AvatarLink()
	}

	visibility := "public"
	if repo.IsPrivate {
		visibility = "private"
	}

	out := &repoInfo{
		Owner:        owner.Name,
		Name:         repo.Name,
		AvatarURL:    avatarURL,
		Visibility:   visibility,
		IsAdmin:      mode >= database.AccessModeAdmin,
		EnableIssues: enableIssues,
		AllowsPulls:  repo.AllowsPulls(),
		EnableWiki:   enableWiki,
		Counts: repoInfoCounts{
			Watchers:   repo.NumWatches,
			Stars:      repo.NumStars,
			Forks:      repo.NumForks,
			OpenIssues: repo.NumOpenIssues,
			OpenPulls:  repo.NumOpenPulls,
		},
		ViewerWatching: viewerID > 0 && database.IsWatching(viewerID, repo.ID),
		ViewerStarred:  viewerID > 0 && database.IsStaring(viewerID, repo.ID),
	}

	if repo.IsMirror {
		mirror, err := database.GetMirrorByRepoID(repo.ID)
		if err != nil {
			log.Error("getRepoInfo: get mirror by repo ID %d: %v", repo.ID, err)
		} else if mirror != nil {
			out.MirrorOf = mirror.Address()
		}
	}

	return http.StatusOK, out, nil
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

// whitespaceFlag maps the `whitespace` query value used on the diff page to
// the matching git diff flag. `ignore-all` → `-w` ignores all whitespace
// changes. `ignore-change` → `-b` is the milder variant that still surfaces
// added/removed blank lines. An empty or unknown value means no whitespace
// handling.
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
	repoName := params["name"]
	commitID := params["sha"]

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by username")
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
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
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	// Treat any `CatFileCommit` failure as "commit not found". The git CLI
	// returns several different errors for unknown revisions (parse failure,
	// "bad file" from cat-file, exit code 128) and only `ErrRevisionNotExist`
	// is recognized by `gitx.IsErrRevisionNotExist`. Surfacing those as 500
	// would render a Server error page for a routine "wrong SHA" navigation.
	commit, err := gitRepo.CatFileCommit(commitID)
	if err != nil {
		return http.StatusNotFound, nil, nil
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			return http.StatusNotFound, nil, nil
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

// getRepoCommitRawDiff streams the unified diff for a single commit as
// `text/plain`. Replaces the legacy `repo.RawDiff` handler. The `{ext}` path
// param controls the output format (`diff` or `patch`, matching `git diff`
// vs `git format-patch` output). Supports the same `?whitespace=` flag as
// `getRepoCommit` so the React diff page's whitespace toggle works.
func getRepoCommitRawDiff(c flamego.Context, r *http.Request, user *database.User) {
	w := c.ResponseWriter()
	ctx := r.Context()
	params := c.Params()
	ownerName := params["owner"]
	repoName := params["name"]
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
		writeStatus(http.StatusNotFound)
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

// resolveRepoForViewer loads the repo identified by `{owner}/{name}` path
// params and asserts the viewer has at least read access. Returns the repo or
// a (statusCode, error) tuple suitable for short-circuiting a handler.
func resolveRepoForViewer(c flamego.Context, ctx stdctx.Context, user *database.User) (*database.Repository, int, error) {
	params := c.Params()
	owner, err := database.Handle.Users().GetByUsername(ctx, params["owner"])
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, nil
		}
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user by username")
	}
	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, params["name"])
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return nil, http.StatusNotFound, nil
		}
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

// repoActionResponse echoes the new viewer/count state so the client can
// update without a follow-up GET. Used by watch/star endpoints.
type repoActionResponse struct {
	ViewerWatching bool `json:"viewerWatching,omitempty"`
	ViewerStarred  bool `json:"viewerStarred,omitempty"`
	Watchers       int  `json:"watchers,omitempty"`
	Stars          int  `json:"stars,omitempty"`
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
		return http.StatusInternalServerError, nil, errors.Wrap(err, "watch repo")
	}
	// Reload to get the updated NumWatches count from the after-trigger.
	updated, err := database.Handle.Repositories().GetByName(r.Context(), repo.OwnerID, repo.Name)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoActionResponse{
		ViewerWatching: watching,
		Watchers:       updated.NumWatches,
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
		return http.StatusInternalServerError, nil, errors.Wrap(err, "star repo")
	}
	updated, err := database.Handle.Repositories().GetByName(r.Context(), repo.OwnerID, repo.Name)
	if err != nil {
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoActionResponse{
		ViewerStarred: starred,
		Stars:         updated.NumStars,
	}, nil
}

func postRepoStar(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoStarAction(c, r, user, true)
}

func deleteRepoStar(c flamego.Context, r *http.Request, user *database.User) (statusCode int, resp *repoActionResponse, err error) {
	return repoStarAction(c, r, user, false)
}

// getRepoRaw streams the contents of a single file at the given ref (branch,
// tag, or commit SHA). Replaces the legacy `repo.SingleDownload` handler.
// Matches its behavior: `Last-Modified` from the commit that last touched
// the file, `Content-Disposition: attachment` for binary non-image blobs,
// `text/plain; charset=utf-8` for text blobs (unless `?render=true` and the
// site has `EnableRawFileRenderMode` set).
func getRepoRaw(c flamego.Context, r *http.Request, user *database.User) {
	w := c.ResponseWriter()
	ctx := r.Context()

	writeStatus := func(code int) {
		w.WriteHeader(code)
	}

	ownerName := c.Param("owner")
	repoName := c.Param("name")
	rest := c.Param("**")
	// The `{ref}/{path...}` segment is collapsed into a single `**` capture
	// so a ref like `feat/foo` resolves correctly: we walk it left-to-right
	// against the git repo's known refs (branches and tags) and treat the
	// first prefix that resolves to a commit as the ref, leaving the rest as
	// the in-tree path. Bare commit SHAs are matched first since they're the
	// common case for the React diff page's "Expand all lines" fetch.
	if rest == "" {
		writeStatus(http.StatusNotFound)
		return
	}

	owner, err := database.Handle.Users().GetByUsername(ctx, ownerName)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRaw: get user by username %q: %v", ownerName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, repoName)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeStatus(http.StatusNotFound)
			return
		}
		log.Error("getRepoRaw: get repo by name %q/%q: %v", ownerName, repoName, err)
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
		log.Error("getRepoRaw: open repository %q/%q: %v", ownerName, repoName, err)
		writeStatus(http.StatusInternalServerError)
		return
	}

	ref, filePath, commit, err := resolveRefPath(gitRepo, rest)
	if err != nil || commit == nil {
		writeStatus(http.StatusNotFound)
		return
	}
	_ = ref // ref is implied by the path; kept for future logging if needed.

	blob, err := commit.Blob(filePath)
	if err != nil {
		writeStatus(http.StatusNotFound)
		return
	}

	data, err := blob.Bytes()
	if err != nil {
		log.Error("getRepoRaw: read blob %s:%s: %v", commit.ID, filePath, err)
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

// resolveRefPath walks the `{ref}/{filepath...}` segment left-to-right,
// returning the first prefix that resolves to a commit. Commit SHAs match
// in a single step; multi-segment branch and tag names (e.g. `feat/foo`)
// require walking until the prefix matches a known ref.
func resolveRefPath(gitRepo *git.Repository, rest string) (string, string, *git.Commit, error) {
	// Fast path: a full or short commit SHA as the first segment.
	first, after, _ := strings.Cut(rest, "/")
	if isHexSHA(first) {
		if commit, err := gitRepo.CatFileCommit(first); err == nil {
			return first, after, commit, nil
		}
	}

	branches, _ := gitRepo.Branches()
	tags, _ := gitRepo.Tags()
	knownRefs := append([]string{}, branches...)
	knownRefs = append(knownRefs, tags...)
	// Match the longest ref prefix first so `release/v1` wins over `release`.
	sortDescByLength(knownRefs)
	for _, ref := range knownRefs {
		if rest == ref {
			return ref, "", nil, nil
		}
		if strings.HasPrefix(rest, ref+"/") {
			commit, err := gitRepo.BranchCommit(ref)
			if err != nil {
				commit, err = gitRepo.TagCommit(ref)
				if err != nil {
					continue
				}
			}
			return ref, rest[len(ref)+1:], commit, nil
		}
	}
	return "", "", nil, errors.New("ref not found")
}

func isHexSHA(s string) bool {
	if len(s) < 4 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func sortDescByLength(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && len(s[j]) > len(s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
