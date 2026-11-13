package web

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/git"
)

const (
	// The limits keep one search from taking over a small server: matches of a
	// common keyword in a large repository can run into the millions.
	codeSearchMaxResults        = 200
	codeSearchMaxResultsPerFile = 10
	codeSearchMaxLineLength     = 500
	codeSearchTimeout           = 10 * time.Second
)

// codeSearchSlots bounds how many searches run at once, since each one reads
// every file of a tree.
var codeSearchSlots = make(chan struct{}, runtime.NumCPU())

type repoSearchMatch struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

type repoSearchFile struct {
	Path    string            `json:"path"`
	Matches []repoSearchMatch `json:"matches"`
}

type repoSearchResponse struct {
	Ref string `json:"ref"`
	// RefType is "branch", "tag" or "commit".
	RefType string           `json:"refType"`
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

	resp = &repoSearchResponse{
		Ref:     ref.Name,
		RefType: ref.Type,
		Keyword: strings.TrimSpace(c.Query("q")),
		Files:   []repoSearchFile{},
	}
	if resp.Keyword != "" {
		select {
		case codeSearchSlots <- struct{}{}:
		case <-ctx.Done():
			log.Error("getRepoSearch: wait to search repo %d: %v", repo.ID, ctx.Err())
			return http.StatusServiceUnavailable, nil, ctx.Err()
		}
		searchCtx, cancel := context.WithTimeout(ctx, codeSearchTimeout)
		results, truncated, err := gitRepo.Grep(searchCtx, ref.Commit.ID.String(), resp.Keyword, git.GrepOptions{
			MaxResults:        codeSearchMaxResults,
			MaxResultsPerFile: codeSearchMaxResultsPerFile,
		})
		cancel()
		<-codeSearchSlots
		switch {
		case errors.Is(err, git.ErrExecTimeout):
			resp.TimedOut = true
		case err != nil:
			log.Error("getRepoSearch: search %q of repo %d: %v", ref.Name, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "search")
		}
		resp.Truncated = truncated
		for _, result := range results {
			if n := len(resp.Files); n == 0 || resp.Files[n-1].Path != result.Path {
				resp.Files = append(resp.Files, repoSearchFile{Path: result.Path})
			}
			file := &resp.Files[len(resp.Files)-1]
			file.Matches = append(file.Matches, repoSearchMatch{Line: result.Line, Text: truncateLine(result.Text, resp.Keyword)})
		}
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
