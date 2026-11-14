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
	log "unknwon.dev/clog/v2"

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
	// A search of all repositories looks at the most recently updated ones
	// first, and at most this many of them.
	codeSearchMaxRepos = 1000
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
func appendSearchFiles(files []repoSearchFile, repo, ref, keyword string, results []*git.GrepResult) []repoSearchFile {
	start := len(files)
	for _, result := range results {
		if n := len(files); n == start || files[n-1].Path != result.Path {
			files = append(files, repoSearchFile{Repo: repo, Ref: ref, Path: result.Path})
		}
		file := &files[len(files)-1]
		file.Lines = append(file.Lines, repoSearchLine{
			Line:    result.Line,
			Text:    truncateLine(result.Text, keyword),
			IsMatch: result.IsMatch,
		})
	}
	return files
}

// acquireCodeSearchSlot waits for a search slot, and returns the function that
// releases it.
func acquireCodeSearchSlot(ctx context.Context) (func(), error) {
	select {
	case codeSearchSlots <- struct{}{}:
		return func() { <-codeSearchSlots }, nil
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
	Query   string           `json:"query"`
	Keyword string           `json:"keyword"`
	Files   []repoSearchFile `json:"files"`
	// Truncated is set when more lines matched than the response lists.
	Truncated bool `json:"truncated"`
	// TimedOut is set when the search took too long and listed nothing.
	TimedOut bool     `json:"timedOut"`
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
}

// getRepoSearch searches the files of the ref in the route, or of the default
// branch when there is none, for lines that contain the "q" query.
func getRepoSearch(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoSearchResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoSearch: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		log.Error("getRepoSearch: resolve ref %q in repo %d: %v", c.Param("path"), repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil || ref.Path != "" {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}

	query := parseCodeQuery(c.Query("q"))
	resp = &repoSearchResponse{
		Ref:     ref.Name,
		RefType: ref.Type,
		Query:   strings.TrimSpace(c.Query("q")),
		Keyword: query.Keyword,
		Files:   []repoSearchFile{},
	}
	if query.Keyword != "" {
		release, err := acquireCodeSearchSlot(ctx)
		if err != nil {
			log.Error("getRepoSearch: wait to search repo %d: %v", repo.ID, err)
			return http.StatusServiceUnavailable, nil, err
		}
		searchCtx, cancel := context.WithTimeout(ctx, codeSearchTimeout)
		results, truncated, err := gitRepo.Grep(searchCtx, ref.Commit.ID.String(), query.Keyword, git.GrepOptions{
			MaxResults:        codeSearchMaxResults,
			MaxResultsPerFile: codeSearchMaxResultsPerFile,
			ContextLines:      codeSearchContextLines,
			Pathspecs:         query.pathspecs(),
		})
		cancel()
		release()
		switch {
		case errors.Is(err, git.ErrExecTimeout):
			resp.TimedOut = true
		case err != nil:
			log.Error("getRepoSearch: search %q of repo %d: %v", ref.Name, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "search")
		}
		resp.Truncated = truncated
		resp.Files = appendSearchFiles(resp.Files, "", "", query.Keyword, results)
	}

	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		log.Error("getRepoSearch: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		log.Error("getRepoSearch: list tags of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}
	return http.StatusOK, resp, nil
}

type codeSearchResponse struct {
	Query   string           `json:"query"`
	Keyword string           `json:"keyword"`
	Files   []repoSearchFile `json:"files"`
	// ReposSearched is how many repositories were searched.
	ReposSearched int `json:"reposSearched"`
	// Truncated is set when more lines matched than the response lists.
	Truncated bool `json:"truncated"`
	// TimedOut is set when the search took too long, and the repositories
	// left were not searched.
	TimedOut bool `json:"timedOut"`
}

// getCodeSearch searches the default branches of the repositories the viewer
// can read for lines that contain the "q" query, narrowed down by its
// qualifiers.
func getCodeSearch(c flamego.Context, user *database.User) (statusCode int, resp *codeSearchResponse, err error) {
	ctx := c.Request().Context()
	query := parseCodeQuery(c.Query("q"))
	resp = &codeSearchResponse{
		Query:   strings.TrimSpace(c.Query("q")),
		Keyword: query.Keyword,
		Files:   []repoSearchFile{},
	}
	if query.Keyword == "" {
		return http.StatusOK, resp, nil
	}

	repos, err := codeSearchRepos(ctx, user, query)
	if err != nil {
		log.Error("getCodeSearch: list repositories: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}

	release, err := acquireCodeSearchSlot(ctx)
	if err != nil {
		log.Error("getCodeSearch: wait to search: %v", err)
		return http.StatusServiceUnavailable, nil, err
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
			log.Error("getCodeSearch: open repository %d: %v", repo.ID, err)
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
		})
		if errors.Is(err, git.ErrExecTimeout) {
			resp.TimedOut = true
			break
		} else if err != nil {
			log.Error("getCodeSearch: search repo %d: %v", repo.ID, err)
			continue
		}
		resp.ReposSearched++
		resp.Truncated = resp.Truncated || truncated
		for _, result := range results {
			if result.IsMatch {
				matches++
			}
		}
		resp.Files = appendSearchFiles(resp.Files, repo.Owner.Name+"/"+repo.Name, ref.Name, query.Keyword, results)
	}
	return http.StatusOK, resp, nil
}

// codeSearchRepos returns the repositories with code that the user can read
// and that match the qualifiers of the query, most recently updated first.
func codeSearchRepos(ctx context.Context, user *database.User, query codeQuery) ([]*database.Repository, error) {
	if user != nil && user.IsDisabled() {
		user = nil
	}

	var candidates []*database.Repository
	if query.Repo != "" {
		ownerName, name := path.Split(query.Repo)
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
		candidates = []*database.Repository{repo}
	} else {
		opts := &database.SearchRepoOptions{OrderBy: "updated_unix DESC", PageSize: 100}
		if user != nil {
			opts.UserID = user.ID
		}
		if query.Owner != "" {
			owner, err := database.Handle.Users().GetByUsername(ctx, query.Owner)
			if database.IsErrUserNotExist(err) {
				return nil, nil
			} else if err != nil {
				return nil, errors.Wrap(err, "get owner")
			}
			opts.OwnerID = owner.ID
		}
		for opts.Page = 1; len(candidates) < codeSearchMaxRepos; opts.Page++ {
			repos, _, err := database.SearchRepositoryByName(opts)
			if err != nil {
				return nil, errors.Wrap(err, "search repositories")
			}
			if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
				return nil, errors.Wrap(err, "load repository attributes")
			}
			candidates = append(candidates, repos...)
			if len(repos) < opts.PageSize {
				break
			}
		}
	}

	// 🚨 SECURITY: The list of repositories also has private ones whose wiki
	// or issues are public, so check that the code can be read.
	repos := make([]*database.Repository, 0, len(candidates))
	for _, repo := range candidates {
		if repo.IsEmpty || !newViewer(ctx, user, repo.Owner, repo).CanRead() {
			continue
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

// truncateLine cuts a long line, e.g., of minified code, to the length shown,
// keeping the first match of the keyword in view.
func truncateLine(line, keyword string) string {
	runes := []rune(line)
	if len(runes) <= codeSearchMaxLineLength {
		return line
	}

	start := 0
	lower := []rune(strings.ToLower(line))
	// Lowercasing changes the length of a few characters, and then the offsets
	// no longer line up, so the line is cut from its start.
	if len(lower) == len(runes) {
		if i := strings.Index(string(lower), strings.ToLower(keyword)); i >= 0 {
			// Show some context before the match.
			start = max(0, utf8.RuneCountInString(string(lower)[:i])-codeSearchMaxLineLength/5)
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
