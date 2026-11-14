package web

import (
	"net/http"
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

type repoBlameCommit struct {
	repoCommitListItem
	// Subject is the first line of the message as plain text.
	Subject string `json:"subject"`
	// PreviousSHA is the commit before this one that has the file, at
	// PreviousPath, to see the lines before this commit changed them. Both are
	// left out for the commit that added the file.
	PreviousSHA  string `json:"previousSHA,omitempty"`
	PreviousPath string `json:"previousPath,omitempty"`
}

// repoBlameHunk is a run of consecutive lines that the same commit last
// changed.
type repoBlameHunk struct {
	SHA string `json:"sha"`
	// StartLine is the number of the first line of the hunk, counting from 1.
	StartLine int `json:"startLine"`
	LineCount int `json:"lineCount"`
}

type repoBlameResponse struct {
	Ref string `json:"ref"`
	// RefType is "branch", "tag" or "commit".
	RefType  string   `json:"refType"`
	CommitID string   `json:"commitID"`
	Path     string   `json:"path"`
	Name     string   `json:"name"`
	Size     int64    `json:"size"`
	Branches []string `json:"branches"`
	Tags     []string `json:"tags"`
	// Kind is "text", or why the file has no blame: "binary" for a file
	// that is not text and "too_large" for a text file too large to show.
	Kind string `json:"kind"`
	// Content is the text of the file, which Hunks split into lines.
	Content string          `json:"content"`
	Hunks   []repoBlameHunk `json:"hunks"`
	// Commits are the commits of the hunks by their SHA.
	Commits map[string]*repoBlameCommit `json:"commits"`
	// TabWidth is the width of a tab that .editorconfig sets for the file.
	TabWidth int `json:"tabWidth,omitempty"`
}

// getRepoBlame tells which commit last changed each line of the file at the
// path after the ref in the route.
func getRepoBlame(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoBlameResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoBlame: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		log.Error("getRepoBlame: resolve ref %q in repo %d: %v", c.Param("path"), repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}
	// 🚨 SECURITY: Prevent path traversal.
	treePath := pathx.Clean(ref.Path)
	if treePath == "" {
		return http.StatusNotFound, nil, errors.New("path is not a file")
	}
	entry, err := ref.Commit.TreeEntry(ctx, treePath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("path does not exist")
		}
		log.Error("getRepoBlame: get entry %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
	} else if entry.IsTree() || entry.IsCommit() {
		return http.StatusNotFound, nil, errors.New("path is not a file")
	}

	resp = &repoBlameResponse{
		Ref:      ref.Name,
		RefType:  ref.Type,
		CommitID: ref.Commit.ID.String(),
		Path:     treePath,
		Name:     entry.Name(),
		Size:     entry.Blob().Size(ctx),
		Hunks:    []repoBlameHunk{},
		Commits:  map[string]*repoBlameCommit{},
	}
	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		log.Error("getRepoBlame: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		log.Error("getRepoBlame: list tags of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}

	p, err := entry.Blob().Bytes(ctx)
	if err != nil {
		log.Error("getRepoBlame: read %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read file")
	}
	switch {
	case !tool.IsTextFile(p):
		resp.Kind = "binary"
		return http.StatusOK, resp, nil
	case resp.Size >= conf.UI.MaxDisplayFileSize:
		resp.Kind = "too_large"
		return http.StatusOK, resp, nil
	}
	resp.Kind = "text"

	hunks, err := gitRepo.Blame(ctx, resp.CommitID, treePath)
	if err != nil {
		log.Error("getRepoBlame: blame %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "blame")
	}
	indent, err := editorconfigIndent(ctx, ref.Commit, treePath)
	if err != nil {
		log.Error("getRepoBlame: read .editorconfig of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read .editorconfig")
	}
	resp.TabWidth = indent.TabWidth

	authors := &commitAuthors{ctx: ctx, users: map[string]*database.User{}}
	metas := repo.ComposeMetas()
	var content strings.Builder
	for _, hunk := range hunks {
		sha := hunk.Commit.ID.String()
		resp.Hunks = append(resp.Hunks, repoBlameHunk{SHA: sha, StartLine: hunk.StartLine, LineCount: len(hunk.Lines)})
		for _, line := range hunk.Lines {
			content.WriteString(line)
			content.WriteByte('\n')
		}
		if _, ok := resp.Commits[sha]; ok {
			continue
		}
		commit := &repoBlameCommit{
			repoCommitListItem: repoCommitListItem{
				SHA:         sha,
				SubjectHTML: template.RenderCommitMessage(false, hunk.Commit.Summary, repo.Link(), metas),
				Author:      authors.signature(hunk.Commit.Author),
			},
			Subject: hunk.Commit.Summary,
		}
		if hunk.Commit.PreviousID != nil {
			commit.PreviousSHA = hunk.Commit.PreviousID.String()
			commit.PreviousPath = hunk.Commit.PreviousPath
		}
		resp.Commits[sha] = commit
	}
	resp.Content = toUTF8([]byte(content.String()))
	return http.StatusOK, resp, nil
}
