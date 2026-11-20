package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/userx"
)

var commitIDPattern = lazyregexp.New(`^[0-9a-f]{7,64}$`)

// compareRangeRoute matches the "{before}...{after}" segment of compare routes
// for SHA-1 and SHA-256 commit IDs. Flamego binds parameters by submatch
// position, so the patterns must not contain capture groups.
const compareRangeRoute = "{before: /[0-9a-f]{40}|[0-9a-f]{64}/}...{after: /[0-9a-f]{40}|[0-9a-f]{64}/}"

// repoRef is a branch, a tag or a commit of a repository, and the path after
// it in a route.
type repoRef struct {
	Name string
	// Type is "branch", "tag" or "commit".
	Type   string
	Commit *git.Commit
	Path   string
}

// resolveRepoRef reads the ref at the start of the route path, which is the
// shortest leading part that names a branch or a tag, or else a commit ID. An
// empty path is the default branch. It returns nil when nothing matches.
func resolveRepoRef(ctx context.Context, repo *database.Repository, gitRepo *git.Repository, routePath string) (*repoRef, error) {
	routePath = strings.Trim(routePath, "/")
	if routePath == "" {
		name := repo.DefaultBranch
		if !gitRepo.HasBranch(ctx, name) {
			branches, err := gitRepo.Branches(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "list branches")
			} else if len(branches) == 0 {
				return nil, nil
			}
			name = branches[0]
		}
		commit, err := gitRepo.BranchCommit(ctx, name)
		if err != nil {
			return nil, errors.Wrap(err, "get branch commit")
		}
		return &repoRef{Name: name, Type: "branch", Commit: commit}, nil
	}

	parts := strings.Split(routePath, "/")
	for i := range parts {
		name := strings.Join(parts[:i+1], "/")
		rest := strings.Join(parts[i+1:], "/")
		var commit *git.Commit
		var err error
		var typ string
		switch {
		case gitRepo.HasBranch(ctx, name):
			typ = "branch"
			commit, err = gitRepo.BranchCommit(ctx, name)
		case gitRepo.HasTag(ctx, name):
			typ = "tag"
			commit, err = gitRepo.TagCommit(ctx, name)
		default:
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "get commit of %s %q", typ, name)
		}
		return &repoRef{Name: name, Type: typ, Commit: commit, Path: rest}, nil
	}

	if !commitIDPattern.MatchString(parts[0]) {
		return nil, nil
	}
	commit, err := gitRepo.CatFileCommit(ctx, parts[0])
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get commit")
	}
	return &repoRef{Name: commit.ID.String(), Type: "commit", Commit: commit, Path: strings.Join(parts[1:], "/")}, nil
}

// commitAuthors looks up the users of commit authors by their emails, once
// per email.
type commitAuthors struct {
	ctx    context.Context
	logger *logx.Logger
	users  map[string]*database.User
}

func (a *commitAuthors) signature(s *git.Signature) repoCommitSignature {
	u, ok := a.users[s.Email]
	if !ok {
		u, _ = database.Handle.Users().GetByEmail(a.ctx, s.Email)
		a.users[s.Email] = u
	}
	sig := repoCommitSignature{
		Name:      s.Name,
		Email:     s.Email,
		When:      s.When.UTC(),
		AvatarURL: userx.GeneratedAvatarURLPath(s.Email),
	}
	if u != nil {
		sig.AvatarURL = u.AvatarURLPath()
		sig.ProfileURL = conf.Server.Subpath + "/" + u.Name
	}
	return sig
}

type repoCommitListItem struct {
	SHA string `json:"sha"`
	// SubjectHTML is the first line of the message, with links to the users
	// and issues it mentions.
	SubjectHTML string              `json:"subjectHTML"`
	IsMerge     bool                `json:"isMerge"`
	Author      repoCommitSignature `json:"author"`
	// CoAuthors are the people the "Co-authored-by" trailers of the message
	// name, other than the author.
	CoAuthors []repoCommitSignature `json:"coAuthors,omitempty"`
}

var coAuthorTrailer = lazyregexp.New(`(?im)^co-authored-by:[ \t]*([^<\r\n]*?)[ \t]*<([^>\r\n]+)>[ \t\r]*$`)

// coAuthorsOf returns the people the "Co-authored-by" trailers of the commit
// message name, once per email and without the author.
func coAuthorsOf(message string, author *git.Signature) []*git.Signature {
	seen := map[string]bool{strings.ToLower(author.Email): true}
	var coAuthors []*git.Signature
	for _, m := range coAuthorTrailer.FindAllStringSubmatch(message, -1) {
		email := strings.TrimSpace(m[2])
		if seen[strings.ToLower(email)] {
			continue
		}
		seen[strings.ToLower(email)] = true
		name := m[1]
		if name == "" {
			name = email
		}
		coAuthors = append(coAuthors, &git.Signature{Name: name, Email: email, When: author.When})
	}
	return coAuthors
}

func toCommitListItems(ctx context.Context, logger *logx.Logger, repo *database.Repository, commits []*git.Commit) []repoCommitListItem {
	authors := &commitAuthors{ctx: ctx, logger: logger, users: map[string]*database.User{}}
	metas := repo.ComposeMetas()
	items := make([]repoCommitListItem, 0, len(commits))
	for _, commit := range commits {
		item := repoCommitListItem{
			SHA:         commit.ID.String(),
			SubjectHTML: markup.RenderCommitMessage(commit.Summary(), repo.Link(), metas),
			IsMerge:     commit.ParentsCount() > 1,
			Author:      authors.signature(commit.Author),
		}
		for _, s := range coAuthorsOf(commit.Message, commit.Author) {
			item.CoAuthors = append(item.CoAuthors, authors.signature(s))
		}
		items = append(items, item)
	}
	return items
}

type repoCommitsResponse struct {
	Ref string `json:"ref"`
	// RefType is "branch", "tag" or "commit".
	RefType string `json:"refType"`
	// Path is the file or the directory whose history is listed, or empty for
	// the whole repository.
	Path string `json:"path"`
	// Search is set for a search of the commit messages, which lists every
	// match instead of a page.
	Search   bool                 `json:"search"`
	Keyword  string               `json:"keyword"`
	Commits  []repoCommitListItem `json:"commits"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	HasNext  bool                 `json:"hasNext"`
	Branches []string             `json:"branches"`
	Tags     []string             `json:"tags"`
}

// getRepoCommits lists the commits of the ref in the route, newest first:
// those that changed the path after the ref, or those whose messages match
// the "q" query when the path is "search".
func getRepoCommits(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoCommitsResponse, err error) {
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
	} else if ref == nil {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}

	resp = &repoCommitsResponse{
		Ref:      ref.Name,
		RefType:  ref.Type,
		Page:     1,
		PageSize: conf.UI.User.CommitsPagingNum,
	}
	var commits []*git.Commit
	if ref.Path == "search" {
		resp.Search = true
		resp.Keyword = strings.TrimSpace(c.Query("q"))
		if resp.Keyword != "" {
			commits, err = ref.Commit.SearchCommits(ctx, resp.Keyword)
			if err != nil {
				logger.Error("Failed to search commits", "ref", ref.Name, "repoID", repo.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "search commits")
			}
		}
	} else {
		// 🚨 SECURITY: Prevent path traversal.
		resp.Path = pathx.Clean(ref.Path)
		if page, _ := strconv.Atoi(c.Query("page")); page > 1 {
			resp.Page = page
		}
		if size, _ := strconv.Atoi(c.Query("pageSize")); size > 0 {
			resp.PageSize = min(size, 100)
		}
		commits, err = ref.Commit.CommitsByPage(ctx, resp.Page, resp.PageSize, git.CommitsByPageOptions{Path: resp.Path})
		if err != nil {
			logger.Error("Failed to list commits", "ref", ref.Name, "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "list commits")
		}
		resp.HasNext = len(commits) == resp.PageSize
	}

	resp.Commits = toCommitListItems(ctx, logger, repo, commits)

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

type repoCompareResponse struct {
	Before string `json:"before"`
	After  string `json:"after"`
	// Base is the commit the changes are compared against: Before, or where
	// After forked from the default branch when Before is the empty ID.
	Base string `json:"base"`
	// BaseIsEmptyTree is true when Base is the empty tree, so the comparison
	// shows everything After has.
	BaseIsEmptyTree bool `json:"baseIsEmptyTree"`
	// Commits are the newest commits after Base up to After, at most
	// compareMaxCommits of them.
	Commits []repoCommitListItem `json:"commits"`
	// TotalCommits is the number of commits after Base up to After, which is
	// more than len(Commits) when the list is cut off.
	TotalCommits int64 `json:"totalCommits"`
	// MarkdownExtensions are the extensions of files that show a rich diff.
	MarkdownExtensions []string `json:"markdownExtensions"`
}

// compareMaxCommits is the most commits a comparison lists, so that comparing
// to a long history, e.g., the push of an imported branch, stays fast.
const compareMaxCommits = 50

// compareRange is the range of commits that a comparison of two commits
// shows.
type compareRange struct {
	base            string
	baseIsEmptyTree bool
	after           *git.Commit
}

// listCommits returns the newest commits after base up to after, at most
// compareMaxCommits of them, and the number of all of them.
func (r *compareRange) listCommits(ctx context.Context, gitRepo *git.Repository) (_ []*git.Commit, total int64, _ error) {
	rev := r.base + "..." + r.after.ID.String()
	if r.baseIsEmptyTree {
		rev = r.after.ID.String()
	}
	commits, err := gitRepo.Log(ctx, rev, git.LogOptions{MaxCount: compareMaxCommits})
	if err != nil {
		return nil, 0, errors.Wrap(err, "list commits")
	}
	if total, err = gitRepo.RevListCount(ctx, []string{rev}); err != nil {
		return nil, 0, errors.Wrap(err, "count commits")
	}
	return commits, total, nil
}

// resolveCompareRange resolves the comparison from before to after. The empty
// ID is the base of a created branch, e.g., in the news feed, so the branch is
// compared against where it forked from the default branch, or against the
// empty tree when it adds nothing to the default branch, e.g., it is the
// default branch, or they share no history, e.g., an orphan branch or a
// default branch that does not exist. It returns git.ErrRevisionNotExist when
// before or after is not a commit of the repository.
func resolveCompareRange(ctx context.Context, gitRepo *git.Repository, defaultBranch, before, after string) (*compareRange, error) {
	afterCommit, err := gitRepo.CatFileCommit(ctx, after)
	if err != nil {
		return nil, errors.Wrap(err, "get head commit")
	}
	r := &compareRange{base: before, after: afterCommit}

	if git.IsEmptyID(before) {
		r.base, err = gitRepo.MergeBase(ctx, git.RefsHeads+defaultBranch, after)
		if err != nil && !git.IsErrNoMergeBase(err) {
			return nil, errors.Wrap(err, "get merge base")
		} else if err != nil || r.base == after {
			objectFormat, err := gitRepo.ObjectFormat(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "get object format")
			}
			r.base = objectFormat.EmptyTreeID()
			r.baseIsEmptyTree = true
		}
	} else if _, err = gitRepo.CatFileCommit(ctx, before); err != nil {
		return nil, errors.Wrap(err, "get base commit")
	}
	return r, nil
}

// getRepoCompare describes the changes from one commit to another, e.g., of a
// push. The diff itself is served as text by getRepoCompareRaw.
func getRepoCompare(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoCompareResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	before, after := c.Param("before"), c.Param("after")
	r, err := resolveCompareRange(ctx, gitRepo, repo.DefaultBranch, before, after)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("commit does not exist")
		}
		logger.Error("Failed to compare commits", "before", before, "after", after, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "compare commits")
	}
	commits, total, err := r.listCommits(ctx, gitRepo)
	if err != nil {
		logger.Error("Failed to list commits", "base", r.base, "after", after, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list commits")
	}
	return http.StatusOK, &repoCompareResponse{
		Before:             before,
		After:              after,
		Base:               r.base,
		BaseIsEmptyTree:    r.baseIsEmptyTree,
		Commits:            toCommitListItems(ctx, logger, repo, commits),
		TotalCommits:       total,
		MarkdownExtensions: conf.Markdown.FileExtensions,
	}, nil
}
