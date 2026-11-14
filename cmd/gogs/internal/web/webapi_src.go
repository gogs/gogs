package web

import (
	"bytes"
	"context"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/editorconfig/editorconfig-core-go/v2"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/tool"
)

type repoCloneLinks struct {
	HTTPS string `json:"https,omitempty"`
	SSH   string `json:"ssh,omitempty"`
}

// repoCloneLinksOf returns the clone links of the repository for the
// protocols the site serves.
func repoCloneLinksOf(owner, repo string, isWiki bool) *repoCloneLinks {
	links := repox.NewCloneLink(owner, repo, isWiki)
	resp := &repoCloneLinks{}
	if !conf.Repository.DisableHTTPGit {
		resp.HTTPS = links.HTTPS
	}
	if !conf.SSH.Disabled {
		resp.SSH = links.SSH
	}
	return resp
}

type repoEntryCommit struct {
	SHA         string    `json:"sha"`
	SubjectHTML string    `json:"subjectHTML"`
	When        time.Time `json:"when"`
}

type repoTreeEntry struct {
	Name string `json:"name"`
	// Type is "dir", "file", "symlink" or "submodule".
	Type string `json:"type"`
	// SubmoduleURL is where the repository of a submodule is, when it can be
	// inferred.
	SubmoduleURL    string           `json:"submoduleURL,omitempty"`
	SubmoduleCommit string           `json:"submoduleCommit,omitempty"`
	Commit          *repoEntryCommit `json:"commit,omitempty"`
}

type repoReadme struct {
	Name string `json:"name"`
	// HTML is set for a README in a markup language, and Text otherwise.
	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
}

type repoFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Kind is "text", "markup", "notebook", "image", "video", "pdf",
	// "binary", or "too_large" for a text file too large to show.
	Kind string `json:"kind"`
	// Content is set for a text file, HTML for a file in a markup language,
	// and Notebook for a Jupyter notebook.
	Content  *string       `json:"content,omitempty"`
	HTML     string        `json:"html,omitempty"`
	Notebook *repoNotebook `json:"notebook,omitempty"`
	IsReadme bool          `json:"isReadme"`
	// IsSymlink is set for a symbolic link, whose content is its target.
	IsSymlink bool `json:"isSymlink"`
	// TabWidth is the width of a tab that .editorconfig sets for the file.
	TabWidth int `json:"tabWidth,omitempty"`
}

type repoSrcSummary struct {
	Description string          `json:"description"`
	Website     string          `json:"website"`
	CommitCount int64           `json:"commitCount"`
	CloneLinks  *repoCloneLinks `json:"cloneLinks"`
}

type repoPullRequestTarget struct {
	// RepoLink is the repository to open the pull request in.
	RepoLink   string `json:"repoLink"`
	BaseBranch string `json:"baseBranch"`
	// Head is the branch to merge, after its owner for another repository.
	Head string `json:"head"`
}

type repoSrcResponse struct {
	// Empty is set for a repository without commits. Only the fields of its
	// quick guide are set then.
	Empty          bool            `json:"empty"`
	DefaultBranch  string          `json:"defaultBranch"`
	IsMirror       bool            `json:"isMirror"`
	ViewerCanWrite bool            `json:"viewerCanWrite"`
	CloneLinks     *repoCloneLinks `json:"cloneLinks,omitempty"`

	Ref string `json:"ref"`
	// RefType is "branch", "tag" or "commit".
	RefType  string `json:"refType"`
	CommitID string `json:"commitID"`
	Path     string `json:"path"`
	// Kind is "dir" or "file".
	Kind         string              `json:"kind"`
	LatestCommit *repoCommitListItem `json:"latestCommit,omitempty"`
	Entries      []repoTreeEntry     `json:"entries,omitempty"`
	Readme       *repoReadme         `json:"readme,omitempty"`
	File         *repoFile           `json:"file,omitempty"`
	// Summary is set for the root directory.
	Summary *repoSrcSummary `json:"summary,omitempty"`
	// Editor tells whether files can be changed in the browser: "ok", or why
	// not: "not_branch" off a branch and "no_write" without write access. It
	// is left out where the browser never changes files, e.g., in a mirror or
	// a branch that only takes pull requests.
	Editor        string                 `json:"editor,omitempty"`
	CanUploadFile bool                   `json:"canUploadFile"`
	PullRequest   *repoPullRequestTarget `json:"pullRequest,omitempty"`
	Branches      []string               `json:"branches"`
	Tags          []string               `json:"tags"`
}

// getRepoSrc describes the directory or the file at the path after the ref
// in the route, for the code browser, or the quick guide of an empty
// repository.
func getRepoSrc(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoSrcResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	resp = &repoSrcResponse{
		DefaultBranch:  repo.DefaultBranch,
		IsMirror:       repo.IsMirror,
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}
	if repo.IsEmpty {
		if c.Param("path") != "" {
			return http.StatusNotFound, nil, errors.New("repository is empty")
		}
		resp.Empty = true
		resp.CloneLinks = repoCloneLinksOf(repoCtx.Owner.Name, repo.Name, false)
		return http.StatusOK, resp, nil
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoSrc: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		log.Error("getRepoSrc: resolve ref %q in repo %d: %v", c.Param("path"), repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}
	// 🚨 SECURITY: Prevent path traversal.
	treePath := pathx.Clean(ref.Path)
	entry, err := ref.Commit.TreeEntry(ctx, treePath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("path does not exist")
		}
		log.Error("getRepoSrc: get entry %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
	}

	resp.Ref = ref.Name
	resp.RefType = ref.Type
	resp.CommitID = ref.Commit.ID.String()
	resp.Path = treePath
	if repo.CanEnableEditor() {
		switch {
		case ref.Type != "branch":
			resp.Editor = "not_branch"
		case !repoCtx.Viewer.CanWrite():
			resp.Editor = "no_write"
		case editableBranch(c, repoCtx, gitRepo, ref.Name):
			resp.Editor = "ok"
			resp.CanUploadFile = conf.Repository.Upload.Enabled
		}
	}
	if ref.Type == "branch" {
		resp.PullRequest = pullRequestTarget(ctx, repoCtx, ref.Name)
	}
	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		log.Error("getRepoSrc: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		log.Error("getRepoSrc: list tags of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}

	latestCommit := ref.Commit
	if treePath != "" {
		latestCommit, err = ref.Commit.CommitByPath(ctx, git.CommitByRevisionOptions{Path: treePath})
		if err != nil {
			log.Error("getRepoSrc: get last commit of %q in repo %d: %v", treePath, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get last commit")
		}
	}
	resp.LatestCommit = &toCommitListItems(ctx, repo, []*git.Commit{latestCommit})[0]

	// Relative links in rendered files resolve against the directory they
	// are in.
	treeLink := repo.Link() + "/src/" + ref.Name
	if treePath != "" {
		treeLink += "/" + treePath
	}
	if !entry.IsTree() {
		resp.Kind = "file"
		resp.File, err = describeRepoFile(ctx, repo, ref.Commit, entry, treePath, path.Dir(treeLink))
		if err != nil {
			log.Error("getRepoSrc: read %q of repo %d: %v", treePath, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "read file")
		}
		return http.StatusOK, resp, nil
	}

	resp.Kind = "dir"
	if treePath == "" {
		resp.Summary = &repoSrcSummary{
			Description: repo.Description,
			Website:     repo.Website,
			CloneLinks:  repoCloneLinksOf(repoCtx.Owner.Name, repo.Name, false),
		}
		if resp.Summary.CommitCount, err = ref.Commit.CommitsCount(ctx); err != nil {
			log.Error("getRepoSrc: count commits of repo %d: %v", repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
		}
	}
	tree, err := ref.Commit.Subtree(ctx, treePath)
	if err != nil {
		log.Error("getRepoSrc: get tree %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get subtree")
	}
	entries, err := tree.Entries(ctx)
	if err != nil {
		log.Error("getRepoSrc: list entries of %q in repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list entries")
	}
	entries.Sort()
	infoCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	infos, err := entries.CommitsInfo(infoCtx, ref.Commit, git.CommitsInfoOptions{
		Path:           treePath,
		MaxConcurrency: conf.Repository.CommitsFetchConcurrency,
	})
	if err != nil {
		log.Error("getRepoSrc: get last commits of entries of %q in repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get commits info")
	}
	metas := repo.ComposeMetas()
	resp.Entries = make([]repoTreeEntry, 0, len(infos))
	for _, info := range infos {
		item := repoTreeEntry{Name: info.Entry.Name(), Type: "file"}
		switch {
		case info.Submodule != nil:
			item.Type = "submodule"
			item.SubmoduleURL = git.InferSubmoduleURL(repo.Link(), info.Submodule)
			item.SubmoduleCommit = info.Submodule.Commit
		case info.Entry.IsTree():
			item.Type = "dir"
		case info.Entry.IsSymlink():
			item.Type = "symlink"
		}
		if info.Commit != nil {
			item.Commit = &repoEntryCommit{
				SHA:         info.Commit.ID.String(),
				SubjectHTML: template.RenderCommitMessage(false, info.Commit.Summary(), repo.Link(), metas),
				When:        info.Commit.Committer.When.UTC(),
			}
		}
		resp.Entries = append(resp.Entries, item)
	}

	for _, e := range entries {
		if e.IsTree() || !markup.IsReadmeFile(e.Name()) {
			continue
		}
		p, err := e.Blob().Bytes(ctx)
		if err != nil {
			log.Error("getRepoSrc: read README %q of repo %d: %v", e.Name(), repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "read README")
		}
		if !tool.IsTextFile(p) {
			break
		}
		resp.Readme = &repoReadme{Name: e.Name()}
		switch markup.Detect(e.Name()) {
		case markup.TypeMarkdown:
			resp.Readme.HTML = string(markup.Markdown(p, treeLink, metas))
		case markup.TypeOrgMode:
			resp.Readme.HTML = string(markup.OrgMode(p, treeLink, metas))
		default:
			resp.Readme.Text = toUTF8(p)
		}
		break
	}
	return http.StatusOK, resp, nil
}

// describeRepoFile reads the file of the entry to show it. Relative links in
// a file in a markup language resolve against dirLink.
func describeRepoFile(ctx context.Context, repo *database.Repository, commit *git.Commit, entry *git.TreeEntry, treePath, dirLink string) (*repoFile, error) {
	blob := entry.Blob()
	p, err := blob.Bytes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read blob")
	}
	file := &repoFile{
		Name:      entry.Name(),
		Size:      blob.Size(ctx),
		IsReadme:  markup.IsReadmeFile(entry.Name()),
		IsSymlink: entry.IsSymlink(),
	}
	switch {
	case tool.IsTextFile(p):
		if file.Size >= conf.UI.MaxDisplayFileSize {
			file.Kind = "too_large"
			break
		}
		switch markup.Detect(file.Name) {
		case markup.TypeMarkdown:
			file.Kind = "markup"
			file.HTML = string(markup.Markdown(p, dirLink, repo.ComposeMetas()))
		case markup.TypeOrgMode:
			file.Kind = "markup"
			file.HTML = string(markup.OrgMode(p, dirLink, repo.ComposeMetas()))
		case markup.TypeIPythonNotebook:
			file.Notebook, err = parseNotebook(p, dirLink, repo.ComposeMetas())
			if err == nil {
				file.Kind = "notebook"
				break
			}
			// A file that is not a valid notebook shows as text.
			fallthrough
		default:
			file.Kind = "text"
			content := toUTF8(p)
			file.Content = &content
			indent, err := editorconfigIndent(ctx, commit, treePath)
			if err != nil {
				return nil, errors.Wrap(err, "read .editorconfig")
			}
			file.TabWidth = indent.TabWidth
		}
	case tool.IsPDFFile(p):
		file.Kind = "pdf"
	case tool.IsVideoFile(p):
		file.Kind = "video"
	case tool.IsImageFile(p):
		file.Kind = "image"
	default:
		file.Kind = "binary"
	}
	return file, nil
}

// toUTF8 converts text in another encoding to UTF-8, keeping it as is when
// the encoding is not known.
func toUTF8(p []byte) string {
	content, err := template.ToUTF8WithErr(p)
	if err != nil {
		return string(p)
	}
	return content
}

// editorconfigIndent returns how the ".editorconfig" at the root of the
// commit indents the file at the path, which need not exist.
func editorconfigIndent(ctx context.Context, commit *git.Commit, filePath string) (*repoEditorIndent, error) {
	indent := &repoEditorIndent{}
	entry, err := commit.TreeEntry(ctx, ".editorconfig")
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return indent, nil
		}
		return nil, errors.Wrap(err, "get .editorconfig")
	}
	p, err := entry.Blob().Bytes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read .editorconfig")
	}
	// A broken .editorconfig is the repository's content, so it only means no
	// rules.
	ec, err := editorconfig.Parse(bytes.NewReader(p))
	if err != nil {
		return indent, nil
	}
	def, err := ec.GetDefinitionForFilename(filePath)
	if err != nil {
		return indent, nil
	}

	switch def.IndentStyle {
	case editorconfig.IndentStyleTab, editorconfig.IndentStyleSpaces:
		indent.Style = def.IndentStyle
	}
	indent.TabWidth = def.TabWidth
	if def.TabWidth <= 0 {
		indent.TabWidth, _ = strconv.Atoi(def.Raw["tab_width"])
	}
	if def.IndentSize == editorconfig.IndentStyleTab {
		indent.Size = indent.TabWidth
	} else if size, err := strconv.Atoi(def.IndentSize); err == nil {
		indent.Size = size
		if indent.TabWidth <= 0 {
			indent.TabWidth = size
		}
	}
	return indent, nil
}

// pullRequestTarget returns where the viewer can propose the changes of the
// branch, like the server-rendered pages: writers and those who forked the
// repository can, when the repository, or its base for a fork, takes pull
// requests.
func pullRequestTarget(ctx context.Context, repoCtx *repoContext, branch string) *repoPullRequestTarget {
	repo := repoCtx.Repo
	viewer := repoCtx.Viewer
	if !viewer.CanWrite() && (!viewer.Authenticated() || !database.Handle.Repositories().HasForkedBy(ctx, repo.ID, viewer.ID())) {
		return nil
	}
	if !repo.IsFork {
		if !repo.AllowsPulls() {
			return nil
		}
		return &repoPullRequestTarget{RepoLink: repo.Link(), BaseBranch: repo.DefaultBranch, Head: branch}
	}
	base, err := database.GetRepositoryByID(repo.ForkID)
	if err != nil {
		if !database.IsErrRepoNotExist(err) {
			log.Error("pullRequestTarget: get base of repo %d: %v", repo.ID, err)
		}
		return nil
	} else if !base.AllowsPulls() {
		return nil
	}
	if viewer.CanWrite() {
		return &repoPullRequestTarget{RepoLink: base.Link(), BaseBranch: base.DefaultBranch, Head: repoCtx.Owner.Name + ":" + branch}
	}
	return &repoPullRequestTarget{RepoLink: repo.Link(), BaseBranch: repo.DefaultBranch, Head: viewer.Name + ":" + branch}
}

// repoPathsMax bounds how many paths a file list has, which is for jumping to
// a file in the browser.
const repoPathsMax = 50000

type repoPathsResponse struct {
	Ref      string   `json:"ref"`
	CommitID string   `json:"commitID"`
	Paths    []string `json:"paths"`
	// Truncated is set when the tree has more files than Paths lists.
	Truncated bool `json:"truncated"`
}

// getRepoPaths lists the paths of every file of the ref in the route, or of
// the default branch without one.
func getRepoPaths(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoPathsResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoPaths: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	ref, err := resolveRepoRef(ctx, repo, gitRepo, c.Param("path"))
	if err != nil {
		log.Error("getRepoPaths: resolve ref %q in repo %d: %v", c.Param("path"), repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "resolve ref")
	} else if ref == nil || ref.Path != "" {
		return http.StatusNotFound, nil, errors.New("ref does not exist")
	}

	paths, err := gitRepo.TreePaths(ctx, ref.Commit.ID.String())
	if err != nil {
		log.Error("getRepoPaths: list paths of %q in repo %d: %v", ref.Name, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list paths")
	}
	resp = &repoPathsResponse{
		Ref:      ref.Name,
		CommitID: ref.Commit.ID.String(),
		Paths:    paths,
	}
	if resp.Paths == nil {
		resp.Paths = []string{}
	}
	if len(resp.Paths) > repoPathsMax {
		resp.Paths = resp.Paths[:repoPathsMax]
		resp.Truncated = true
	}
	return http.StatusOK, resp, nil
}
