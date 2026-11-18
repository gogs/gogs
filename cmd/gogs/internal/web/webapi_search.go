package web

import (
	"context"
	"net/http"
	"path"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

const (
	// The limits keep one search from taking over a small server: matches of a
	// common keyword in a large repository can run into the millions.
	codeSearchMaxResults        = 200
	codeSearchMaxResultsPerFile = 10
	codeSearchContextLines      = 1
	codeSearchMaxLineLength     = 500
	codeSearchTimeout           = 10 * time.Second
	// An exhaustive search of a repository lists every matching line of a
	// file, and is allowed more lines and more time.
	codeSearchExhaustiveMaxResults = 5000
	codeSearchExhaustiveTimeout    = time.Minute
	// A search of all repositories looks at the most recently updated ones
	// first, and at most this many of them.
	codeSearchMaxRepos = 1000
	// A search that waits longer than this for a slot gives up, so a few
	// visitors that keep the slots busy cannot queue every other search.
	codeSearchSlotWait = 10 * time.Second
)

// codeSearchSlots bounds how many searches run at once, since each one reads
// every file of a tree.
var codeSearchSlots = make(chan struct{}, runtime.NumCPU())

// codeQuery is a search query: a keyword and the qualifiers that narrow down
// where to look, like the code search of GitHub.
type codeQuery struct {
	Keyword string
	// Owner is the user or the organization of "user:" and "org:".
	Owner string
	// Repo is the "owner/name" of "repo:".
	Repo string
	// Paths are the values of "path:", which a file must match one of.
	Paths []string
}

// parseCodeQuery splits the qualifiers from the keyword, which is the rest of
// the query. Quotes around the keyword are left out, so it can be a phrase
// that starts with a qualifier, e.g., `"path: x"`.
func parseCodeQuery(q string) codeQuery {
	var query codeQuery
	var words []string
	for _, field := range strings.Fields(q) {
		key, value, ok := strings.Cut(field, ":")
		switch {
		case ok && value != "" && (strings.EqualFold(key, "org") || strings.EqualFold(key, "user")):
			query.Owner = value
		case ok && value != "" && strings.EqualFold(key, "repo"):
			query.Repo = value
		case ok && value != "" && strings.EqualFold(key, "path"):
			query.Paths = append(query.Paths, value)
		default:
			words = append(words, field)
		}
	}
	keyword := strings.Join(words, " ")
	if len(keyword) >= 2 && strings.HasPrefix(keyword, `"`) && strings.HasSuffix(keyword, `"`) {
		keyword = keyword[1 : len(keyword)-1]
	}
	query.Keyword = keyword
	return query
}

// pathspecs turns the paths into pathspecs of Git. A path without wildcards
// matches any file whose path contains it, ignoring case.
func (q codeQuery) pathspecs() []string {
	var specs []string
	for _, p := range q.Paths {
		if strings.ContainsAny(p, "*?[") {
			p = strings.TrimPrefix(p, "/")
			specs = append(specs, ":(glob,icase)"+p, ":(glob,icase)**/"+p)
			continue
		}
		// Glob characters were ruled out above, so the path is taken literally.
		specs = append(specs, ":(glob,icase)**/*"+p+"*", ":(glob,icase)**/*"+p+"*/**")
	}
	return specs
}

type repoSearchLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
	// IsMatch is set for lines that contain the keyword, and not for the lines
	// around them.
	IsMatch bool `json:"isMatch"`
}

type repoSearchFile struct {
	// Repo is the "owner/name" of the repository, set by the search of all
	// repositories.
	Repo string `json:"repo,omitempty"`
	// Ref is the ref searched, set by the search of all repositories.
	Ref   string           `json:"ref,omitempty"`
	Path  string           `json:"path"`
	Lines []repoSearchLine `json:"lines"`
}

// appendSearchFiles groups the results by file.
func appendSearchFiles(files []repoSearchFile, repo, ref, keyword string, matchCase bool, results []*git.GrepResult) []repoSearchFile {
	start := len(files)
	for _, result := range results {
		if n := len(files); n == start || files[n-1].Path != result.Path {
			files = append(files, repoSearchFile{Repo: repo, Ref: ref, Path: result.Path})
		}
		file := &files[len(files)-1]
		file.Lines = append(file.Lines, repoSearchLine{
			Line:    result.Line,
			Text:    truncateLine(result.Text, keyword, matchCase),
			IsMatch: result.IsMatch,
		})
	}
	return files
}

// acquireCodeSearchSlot waits for a search slot, at most codeSearchSlotWait,
// and returns the function that releases it.
func acquireCodeSearchSlot(ctx context.Context) (func(), error) {
	timer := time.NewTimer(codeSearchSlotWait)
	defer timer.Stop()
	select {
	case codeSearchSlots <- struct{}{}:
		return func() { <-codeSearchSlots }, nil
	case <-timer.C:
		return nil, errors.New("no slot became free in time")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type repoSearchResponse struct {
	Ref string `json:"ref"`
	// RefType is "branch", "tag" or "commit".
	RefType string `json:"refType"`
	// Query is what was searched for, and Keyword is its part without the
	// qualifiers.
	Query   string `json:"query"`
	Keyword string `json:"keyword"`
	// Exhaustive is set for an exhaustive search, see getRepoSearch.
	Exhaustive bool `json:"exhaustive"`
	// MatchCase is set when letter case must match, rather than being ignored.
	MatchCase bool             `json:"matchCase"`
	Files     []repoSearchFile `json:"files"`
	// Truncated is set when more lines matched than the response lists.
	Truncated bool `json:"truncated"`
	// TimedOut is set when the search took too long and listed nothing.
	TimedOut bool `json:"timedOut"`
	// Busy is set when other searches took every slot for too long, so this
	// one did not run.
	Busy     bool     `json:"busy"`
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
}

// getRepoSearch searches the files of the ref in the route, or of the default
// branch when there is none, for lines that contain the "q" query. With
// "matchCase=true", letter case must match. With "exhaustive=true", it lists
// every matching line of a file rather than the first few, and gets higher
// limits. Only authenticated viewers can search exhaustively, since such a
// search holds a slot for long.
func getRepoSearch(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoSearchResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		logger.Error("Failed to resolve ref", "path", c.Param("path"), "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil || ref.Path != "" {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}

	query := parseCodeQuery(c.Query("q"))
	// A smaller limit, e.g., for showing a few matches as the viewer types,
	// makes the search stop sooner.
	limit := max(1, min(c.QueryInt("limit", codeSearchMaxResults), codeSearchMaxResults))
	resp = &repoSearchResponse{
		Ref:        ref.Name,
		RefType:    ref.Type,
		Query:      strings.TrimSpace(c.Query("q")),
		Keyword:    query.Keyword,
		Exhaustive: c.QueryBool("exhaustive") && repoCtx.Viewer.User != nil,
		MatchCase:  c.QueryBool("matchCase"),
		Files:      []repoSearchFile{},
	}
	var release func()
	if query.Keyword != "" {
		release, err = acquireCodeSearchSlot(ctx)
		if err != nil {
			// The page still lists what it can and says why there are no results.
			logger.Warn("Failed to wait for a code search slot", "repoID", repo.ID, "error", err)
			resp.Busy = true
		}
	}
	if release != nil {
		timeout := codeSearchTimeout
		opts := git.GrepOptions{
			MaxResults:        limit,
			MaxResultsPerFile: min(limit, codeSearchMaxResultsPerFile),
			ContextLines:      codeSearchContextLines,
			Pathspecs:         query.pathspecs(),
			MatchCase:         resp.MatchCase,
		}
		if resp.Exhaustive {
			timeout = codeSearchExhaustiveTimeout
			opts.MaxResults = codeSearchExhaustiveMaxResults
			opts.MaxResultsPerFile = 0
		}
		searchCtx, cancel := context.WithTimeout(ctx, timeout)
		results, truncated, err := gitRepo.Grep(searchCtx, ref.Commit.ID.String(), query.Keyword, opts)
		cancel()
		release()
		switch {
		case errors.Is(err, git.ErrExecTimeout):
			resp.TimedOut = true
		case err != nil:
			logger.Error("Failed to search code", "ref", ref.Name, "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "search")
		}
		resp.Truncated = truncated
		resp.Files = appendSearchFiles(resp.Files, "", "", query.Keyword, resp.MatchCase, results)
	}

	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		logger.Error("Failed to list branches", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		logger.Error("Failed to list tags", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}
	return http.StatusOK, resp, nil
}

type codeSearchResponse struct {
	Query   string `json:"query"`
	Keyword string `json:"keyword"`
	// MatchCase is set when letter case must match, rather than being ignored.
	MatchCase bool             `json:"matchCase"`
	Files     []repoSearchFile `json:"files"`
	// ReposSearched is how many repositories were searched.
	ReposSearched int `json:"reposSearched"`
	// Truncated is set when more lines matched than the response lists.
	Truncated bool `json:"truncated"`
	// TimedOut is set when the search took too long, and the repositories
	// left were not searched.
	TimedOut bool `json:"timedOut"`
	// Busy is set when other searches took every slot for too long, so this
	// one did not run.
	Busy bool `json:"busy"`
}

// getCodeSearch searches the default branches of the repositories the viewer
// can read for lines that contain the "q" query, narrowed down by its
// qualifiers. With "matchCase=true", letter case must match.
func getCodeSearch(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *codeSearchResponse, err error) {
	ctx := c.Request().Context()
	query := parseCodeQuery(c.Query("q"))
	resp = &codeSearchResponse{
		Query:     strings.TrimSpace(c.Query("q")),
		Keyword:   query.Keyword,
		MatchCase: c.QueryBool("matchCase"),
		Files:     []repoSearchFile{},
	}
	if query.Keyword == "" {
		return http.StatusOK, resp, nil
	}

	repos, err := codeSearchRepos(ctx, logger, viewer, query)
	if err != nil {
		logger.Error("Failed to list repositories to search", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}

	release, err := acquireCodeSearchSlot(ctx)
	if err != nil {
		logger.Warn("Failed to wait for a code search slot", "error", err)
		resp.Busy = true
		return http.StatusOK, resp, nil
	}
	defer release()
	searchCtx, cancel := context.WithTimeout(ctx, codeSearchTimeout)
	defer cancel()

	matches := 0
	for _, repo := range repos {
		if searchCtx.Err() != nil {
			resp.TimedOut = true
			break
		}
		if matches >= codeSearchMaxResults {
			resp.Truncated = true
			break
		}
		gitRepo, err := git.Open(repo.RepoPath())
		if err != nil {
			logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
			continue
		}
		ref, err := resolveRepoRef(searchCtx, repo, gitRepo, "")
		if err != nil || ref == nil {
			// The repository has no branch to search, or it went away since
			// it was listed.
			continue
		}
		results, truncated, err := gitRepo.Grep(searchCtx, ref.Commit.ID.String(), query.Keyword, git.GrepOptions{
			MaxResults:        codeSearchMaxResults - matches,
			MaxResultsPerFile: codeSearchMaxResultsPerFile,
			ContextLines:      codeSearchContextLines,
			Pathspecs:         query.pathspecs(),
			MatchCase:         resp.MatchCase,
		})
		if errors.Is(err, git.ErrExecTimeout) {
			resp.TimedOut = true
			break
		} else if err != nil {
			logger.Error("Failed to search code", "repoID", repo.ID, "error", err)
			continue
		}
		resp.ReposSearched++
		resp.Truncated = resp.Truncated || truncated
		for _, result := range results {
			if result.IsMatch {
				matches++
			}
		}
		resp.Files = appendSearchFiles(resp.Files, repo.Owner.Name+"/"+repo.Name, ref.Name, query.Keyword, resp.MatchCase, results)
	}
	return http.StatusOK, resp, nil
}

// codeSearchRepos returns the repositories with code that the user can read
// and that match the qualifiers of the query, most recently updated first.
func codeSearchRepos(ctx context.Context, logger *logx.Logger, viewer *database.User, query codeQuery) ([]*database.Repository, error) {
	var candidates []*database.Repository
	if query.Repo != "" {
		repo, err := repoByFullName(ctx, query.Repo)
		if err != nil {
			return nil, errors.Wrap(err, "get repository")
		} else if repo == nil {
			return nil, nil
		}
		candidates = []*database.Repository{repo}
	} else {
		var ownerID int64
		if query.Owner != "" {
			owner, err := database.Handle.Users().GetByUsername(ctx, query.Owner)
			if database.IsErrUserNotExist(err) {
				return nil, nil
			} else if err != nil {
				return nil, errors.Wrap(err, "get owner")
			}
			ownerID = owner.ID
		}
		var err error
		if candidates, err = viewerRepoCandidates(ctx, viewer, ownerID); err != nil {
			return nil, errors.Wrap(err, "list repositories")
		}
	}

	// 🚨 SECURITY: The list of repositories also has private ones whose wiki
	// or issues are public, so check that the code can be read.
	repos := make([]*database.Repository, 0, len(candidates))
	for _, repo := range candidates {
		if repo.IsEmpty || !newViewer(ctx, logger, viewer, repo.Owner, repo).CanRead() {
			continue
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// repoByFullName returns the repository of the "owner/name", with its owner
// loaded, or nil when there is none.
func repoByFullName(ctx context.Context, fullName string) (*database.Repository, error) {
	ownerName, name := path.Split(fullName)
	owner, err := database.Handle.Users().GetByUsername(ctx, strings.TrimSuffix(ownerName, "/"))
	if database.IsErrUserNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, name)
	if database.IsErrRepoNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "get repository")
	}
	repo.Owner = owner
	return repo, nil
}

// issueSearchLimit is how many issues and pull requests a search lists, which
// is for picking one rather than browsing.
const issueSearchLimit = 10

type issueSearchItem struct {
	// Repo is the "owner/name" of the repository.
	Repo     string `json:"repo"`
	Index    int64  `json:"index"`
	Title    string `json:"title"`
	IsPull   bool   `json:"isPull"`
	IsClosed bool   `json:"isClosed"`
}

type issueSearchResponse struct {
	Items []issueSearchItem `json:"items"`
}

// getIssueSearch finds the issues and the pull requests whose titles contain
// the "q" query, or whose number it is, in the repositories the viewer can
// see them in, or only in the "owner/name" of the "repo" query.
func getIssueSearch(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *issueSearchResponse, err error) {
	ctx := c.Request().Context()
	resp = &issueSearchResponse{Items: []issueSearchItem{}}
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword == "" {
		return http.StatusOK, resp, nil
	}

	var candidates []*database.Repository
	if fullName := c.Query("repo"); fullName != "" {
		repo, err := repoByFullName(ctx, fullName)
		if err != nil {
			logger.Error("Failed to get repository", "repo", fullName, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
		} else if repo == nil {
			return http.StatusOK, resp, nil
		}
		candidates = []*database.Repository{repo}
	} else if candidates, err = viewerRepoCandidates(ctx, viewer, 0); err != nil {
		logger.Error("Failed to list repositories", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}

	// 🚨 SECURITY: Check what the viewer can see of each repository, like the
	// issue and the pull request lists do.
	opts := database.SearchIssuesOptions{Keyword: keyword, Limit: issueSearchLimit}
	repos := make(map[int64]*database.Repository, len(candidates))
	for _, repo := range candidates {
		access := newViewer(ctx, logger, viewer, repo.Owner, repo)
		if repo.EnableIssues && !repo.EnableExternalTracker && (access.CanRead() || repo.CanGuestViewIssues()) {
			opts.IssueRepoIDs = append(opts.IssueRepoIDs, repo.ID)
			repos[repo.ID] = repo
		}
		if access.CanRead() && repo.AllowsPulls() {
			opts.PullRepoIDs = append(opts.PullRepoIDs, repo.ID)
			repos[repo.ID] = repo
		}
	}
	issues, err := database.SearchIssues(opts)
	if err != nil {
		logger.Error("Failed to search issues", "keyword", keyword, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search issues")
	}
	for _, issue := range issues {
		repo := repos[issue.RepoID]
		resp.Items = append(resp.Items, issueSearchItem{
			Repo:     repo.Owner.Name + "/" + repo.Name,
			Index:    issue.Index,
			Title:    issue.Title,
			IsPull:   issue.IsPull,
			IsClosed: issue.IsClosed,
		})
	}
	return http.StatusOK, resp, nil
}

// viewerRepoCandidates lists the repositories the user can see in lists, the
// most recently updated first, at most codeSearchMaxRepos of them, with their
// owners loaded. The list also has private repositories whose wiki or issues
// are public, so callers check what the user can read of each.
func viewerRepoCandidates(ctx context.Context, viewer *database.User, ownerID int64) ([]*database.Repository, error) {
	opts := &database.SearchRepoOptions{OrderBy: "updated_unix DESC", PageSize: 100, OwnerID: ownerID}
	if viewer != nil {
		opts.UserID = viewer.ID
	}
	var candidates []*database.Repository
	for opts.Page = 1; len(candidates) < codeSearchMaxRepos; opts.Page++ {
		repos, _, err := database.SearchRepositoryByName(opts)
		if err != nil {
			return nil, errors.Wrap(err, "search repositories")
		}
		if err = database.Handle.Repositories().LoadAttributes(ctx, repos...); err != nil {
			return nil, errors.Wrap(err, "load repository attributes")
		}
		candidates = append(candidates, repos...)
		if len(repos) < opts.PageSize {
			break
		}
	}
	return candidates, nil
}

// truncateLine cuts a long line, e.g., of minified code, to the length shown,
// keeping the first match of the keyword in view.
func truncateLine(line, keyword string, matchCase bool) string {
	runes := []rune(line)
	if len(runes) <= codeSearchMaxLineLength {
		return line
	}

	start := 0
	searched, keywordSearched := line, keyword
	if !matchCase {
		searched, keywordSearched = strings.ToLower(line), strings.ToLower(keyword)
	}
	// Lowercasing changes the length of a few characters, and then the offsets
	// no longer line up, so the line is cut from its start.
	if utf8.RuneCountInString(searched) == len(runes) {
		if i := strings.Index(searched, keywordSearched); i >= 0 {
			// Show some context before the match.
			start = max(0, utf8.RuneCountInString(searched[:i])-codeSearchMaxLineLength/5)
		}
	}
	end := min(len(runes), start+codeSearchMaxLineLength)
	text := string(runes[start:end])
	if start > 0 {
		text = "…" + text
	}
	if end < len(runes) {
		text += "…"
	}
	return text
}
