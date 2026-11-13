package web

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/tool"
)

var commitIDPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

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
	ctx   context.Context
	users map[string]*database.User
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
		AvatarURL: tool.AvatarLink(s.Email),
	}
	if u != nil {
		sig.AvatarURL = u.AvatarURLPath()
		sig.ProfileURL = conf.Server.Subpath + "/" + u.Name
	}
	return sig
}

type repoCommitListItem struct {
	SHA string `json:"sha"`
	// SubjectHTML is the first line of the message, with links to the issues
	// it mentions.
	SubjectHTML string              `json:"subjectHTML"`
	IsMerge     bool                `json:"isMerge"`
	Author      repoCommitSignature `json:"author"`
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
// the "q" query when the path is "search", like the server-rendered page.
func getRepoCommits(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoCommitsResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoCommits: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		log.Error("getRepoCommits: resolve ref %q in repo %d: %v", c.Param("path"), repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}

	resp = &repoCommitsResponse{
		Ref:      ref.Name,
		RefType:  ref.Type,
		Page:     1,
		PageSize: conf.UI.User.CommitsPagingNum,
		Commits:  []repoCommitListItem{},
	}
	var commits []*git.Commit
	if ref.Path == "search" {
		resp.Search = true
		resp.Keyword = strings.TrimSpace(c.Query("q"))
		if resp.Keyword != "" {
			commits, err = ref.Commit.SearchCommits(ctx, resp.Keyword)
			if err != nil {
				log.Error("getRepoCommits: search commits of %q in repo %d: %v", ref.Name, repo.ID, err)
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
			log.Error("getRepoCommits: list commits of %q in repo %d: %v", ref.Name, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "list commits")
		}
		resp.HasNext = len(commits) == resp.PageSize
	}

	authors := &commitAuthors{ctx: ctx, users: map[string]*database.User{}}
	metas := repo.ComposeMetas()
	for _, commit := range commits {
		resp.Commits = append(resp.Commits, repoCommitListItem{
			SHA:         commit.ID.String(),
			SubjectHTML: template.RenderCommitMessage(false, commit.Message, repo.Link(), metas),
			IsMerge:     commit.ParentsCount() > 1,
			Author:      authors.signature(commit.Author),
		})
	}

	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		log.Error("getRepoCommits: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		log.Error("getRepoCommits: list tags of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}
	return http.StatusOK, resp, nil
}
