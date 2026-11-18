package web

import (
	"bytes"
	"context"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/editorconfig/editorconfig-core-go/v2"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

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

// repoOverviewFile is a file shown in a tab below the file list, e.g., the
// README or the license.
type repoOverviewFile struct {
	// Kind is "readme", "code_of_conduct", "contributing", "license" or
	// "security".
	Kind string `json:"kind"`
	// Path is relative to the directory being viewed, e.g., ".github/CONTRIBUTING.md".
	Path string `json:"path"`
	// HTML is set for a file in a markup language, and Text otherwise.
	HTML string `json:"html,omitempty"`
	Text string `json:"text,omitempty"`
}

type repoFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Kind is "text", "markup", "notebook", "image", "video", "pdf",
	// "binary", or "too_large" for a text file too large to show.
	Kind string `json:"kind"`
	// Content is set for a text file, including a notebook or a file in a
	// markup language for their code view, HTML for a file in a markup
	// language, and Notebook for a Jupyter notebook.
	Content  *string       `json:"content,omitempty"`
	HTML     string        `json:"html,omitempty"`
	Notebook *repoNotebook `json:"notebook,omitempty"`
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
	// LatestRelease is shown under the release count, unless there are no
	// tags.
	LatestRelease *repoLatestRelease `json:"latestRelease,omitempty"`
}

// repoLatestRelease is the newest published release that is not a
// pre-release, like GitHub's "Latest", or the newest tag when there is none.
type repoLatestRelease struct {
	TagName string `json:"tagName"`
	// Title and CreatedAt are only set for a release.
	Title     string     `json:"title,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// latestRelease returns the newest published release that is not a
// pre-release, or the newest of tags, which are sorted newest first.
func latestRelease(ctx context.Context, repoID int64, tags []string) (*repoLatestRelease, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	releases, err := database.Handle.Releases().ListPublishedByRepoID(ctx, repoID)
	if err != nil {
		return nil, errors.Wrap(err, "list published releases")
	}
	for _, r := range releases {
		if !r.IsPrerelease {
			return &repoLatestRelease{TagName: r.TagName, Title: r.Title, CreatedAt: &r.Created}, nil
		}
	}
	return &repoLatestRelease{TagName: tags[0]}, nil
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
	// Overview lists the files shown in tabs below the file list, in the
	// order of the tabs.
	Overview []repoOverviewFile `json:"overview,omitempty"`
	File     *repoFile          `json:"file,omitempty"`
	// Summary is set for the root directory.
	Summary *repoSrcSummary `json:"summary,omitempty"`
	// Editor tells whether files can be changed in the browser: "ok", or why
	// not: "not_branch" off a branch and "no_write" without write access. It
	// is left out where the browser never changes files, e.g., in a mirror or
	// a branch that takes the viewer's changes only through pull requests.
	Editor        string                 `json:"editor,omitempty"`
	CanUploadFile bool                   `json:"canUploadFile"`
	PullRequest   *repoPullRequestTarget `json:"pullRequest,omitempty"`
	Branches      []string               `json:"branches"`
	Tags          []string               `json:"tags"`
}

// getRepoSrc describes the directory or the file at the path after the ref
// in the route, for the code browser, or the quick guide of an empty
// repository.
func getRepoSrc(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoSrcResponse, err error) {
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
	// 🚨 SECURITY: Prevent path traversal.
	treePath := pathx.Clean(ref.Path)
	entry, err := ref.Commit.TreeEntry(ctx, treePath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("path does not exist")
		}
		logger.Error("Failed to get tree entry", "treePath", treePath, "repoID", repo.ID, "error", err)
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
		case editableBranch(c, logger, repoCtx, gitRepo, ref.Name):
			resp.Editor = "ok"
			resp.CanUploadFile = conf.Repository.Upload.Enabled
		}
	}
	if ref.Type == "branch" {
		resp.PullRequest = pullRequestTarget(ctx, logger, repoCtx, ref.Name)
	}
	if resp.Branches, err = gitRepo.Branches(ctx); err != nil {
		logger.Error("Failed to list branches", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	if resp.Tags, err = gitRepo.Tags(ctx); err != nil {
		logger.Error("Failed to list tags", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list tags")
	}

	latestCommit := ref.Commit
	if treePath != "" {
		latestCommit, err = ref.Commit.CommitByPath(ctx, git.CommitByRevisionOptions{Path: treePath})
		if err != nil {
			logger.Error("Failed to get last commit", "treePath", treePath, "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get last commit")
		}
	}
	resp.LatestCommit = &toCommitListItems(ctx, logger, repo, []*git.Commit{latestCommit})[0]

	// Relative links in rendered files resolve against the directory they
	// are in.
	treeLink := repo.Link() + "/src/" + ref.Name
	if treePath != "" {
		treeLink += "/" + treePath
	}
	if !entry.IsTree() {
		resp.Kind = "file"
		resp.File, err = describeRepoFile(ctx, logger, repo, ref.Commit, entry, treePath, path.Dir(treeLink))
		if err != nil {
			logger.Error("Failed to read file", "treePath", treePath, "repoID", repo.ID, "error", err)
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
			logger.Error("Failed to count commits", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "count commits")
		}
		if resp.Summary.LatestRelease, err = latestRelease(ctx, repo.ID, resp.Tags); err != nil {
			logger.Error("Failed to get latest release", "repoID", repo.ID, "error", err)
			return http.StatusInternalServerError, nil, err
		}
	}
	tree, err := ref.Commit.Subtree(ctx, treePath)
	if err != nil {
		logger.Error("Failed to get tree", "treePath", treePath, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get subtree")
	}
	entries, err := tree.Entries(ctx)
	if err != nil {
		logger.Error("Failed to list tree entries", "treePath", treePath, "repoID", repo.ID, "error", err)
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
		logger.Error("Failed to get last commits of tree entries", "treePath", treePath, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get commits info")
	}
	metas := repo.ComposeMetas()
	dirCount := 0
	for _, e := range entries {
		if e.IsTree() {
			dirCount++
		}
	}
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
			if dirCount <= flattenDirsMax {
				item.Name, err = flattenDir(ctx, tree, item.Name)
				if err != nil {
					logger.Error("Failed to flatten directory", "dirName", item.Name, "treePath", treePath, "repoID", repo.ID, "error", err)
					return http.StatusInternalServerError, nil, errors.Wrap(err, "flatten directory")
				}
			}
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

	if resp.Overview, err = readOverviewFiles(ctx, logger, ref.Commit, entries, treePath == "", treeLink, metas); err != nil {
		logger.Error("Failed to read overview files", "treePath", treePath, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read overview files")
	}
	return http.StatusOK, resp, nil
}

// flattenDirsMax bounds how many directories of a directory are flattened,
// since each level needs a tree read.
const flattenDirsMax = 100

// flattenDir follows the directory at the name in the tree while it holds
// only one directory, and returns the path to the last one, e.g., "cmd/gogs",
// so the code browser shows such a chain as one row like GitHub.
func flattenDir(ctx context.Context, tree *git.Tree, name string) (string, error) {
	sub, err := tree.Subtree(ctx, name)
	if err != nil {
		return "", errors.Wrap(err, "get subtree")
	}
	for {
		entries, err := sub.Entries(ctx)
		if err != nil {
			return "", errors.Wrap(err, "list entries")
		}
		if len(entries) != 1 || !entries[0].IsTree() {
			return name, nil
		}
		name += "/" + entries[0].Name()
		if sub, err = sub.Subtree(ctx, entries[0].Name()); err != nil {
			return "", errors.Wrap(err, "get subtree")
		}
	}
}

// overviewKinds are the kinds of overview files, in the order of their tabs.
var overviewKinds = []string{"readme", "code_of_conduct", "contributing", "license", "security"}

// overviewKindOf returns the kind of overview file that a file named name is,
// or an empty string if it is none.
func overviewKindOf(name string) string {
	base := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
	switch {
	case markup.IsReadmeFile(name):
		return "readme"
	case base == "code_of_conduct" || base == "code-of-conduct":
		return "code_of_conduct"
	case base == "contributing":
		return "contributing"
	case base == "license" || base == "licence" || base == "copying" || base == "unlicense" ||
		strings.HasPrefix(base, "license-") || strings.HasPrefix(base, "licence-"):
		return "license"
	case base == "security":
		return "security"
	}
	return ""
}

// readOverviewFiles reads the text files shown in tabs below the file list of
// a directory with the given entries, in the order of their tabs. A
// subdirectory shows only its README. The root directory also shows its
// license, and its code of conduct, contributing guide and security policy,
// which are looked up in ".github", then in the root directory, then in
// "docs", as on GitHub. Relative links in a file resolve against its
// directory under treeLink.
func readOverviewFiles(ctx context.Context, logger *logx.Logger, commit *git.Commit, entries git.Entries, isRoot bool, treeLink string, metas map[string]string) ([]repoOverviewFile, error) {
	type found struct {
		dir   string
		entry *git.TreeEntry
	}
	files := make(map[string]found, len(overviewKinds))
	add := func(dir string, entries git.Entries) {
		for _, e := range entries {
			if e.IsTree() || e.IsCommit() {
				continue
			}
			kind := overviewKindOf(e.Name())
			if kind == "" || (kind != "readme" && !isRoot) || (dir != "" && (kind == "readme" || kind == "license")) {
				continue
			}
			if _, ok := files[kind]; !ok {
				files[kind] = found{dir: dir, entry: e}
			}
		}
	}
	subdirEntries := func(dir string) (git.Entries, error) {
		for _, e := range entries {
			if e.IsTree() && e.Name() == dir {
				tree, err := commit.Subtree(ctx, dir)
				if err != nil {
					return nil, errors.Wrapf(err, "get subtree %q", dir)
				}
				return tree.Entries(ctx)
			}
		}
		return nil, nil
	}

	if isRoot {
		githubEntries, err := subdirEntries(".github")
		if err != nil {
			return nil, err
		}
		add(".github", githubEntries)
	}
	add("", entries)
	if isRoot {
		docsEntries, err := subdirEntries("docs")
		if err != nil {
			return nil, err
		}
		add("docs", docsEntries)
	}

	var overview []repoOverviewFile
	for _, kind := range overviewKinds {
		f, ok := files[kind]
		if !ok {
			continue
		}
		p, err := f.entry.Blob().Bytes(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "read %q", path.Join(f.dir, f.entry.Name()))
		}
		if !tool.IsTextFile(p) {
			continue
		}
		file := repoOverviewFile{Kind: kind, Path: path.Join(f.dir, f.entry.Name())}
		dirLink := strings.TrimSuffix(treeLink+"/"+f.dir, "/")
		switch markup.Detect(f.entry.Name()) {
		case markup.TypeMarkdown:
			file.HTML = string(markup.MarkdownFile(p, dirLink, metas))
		case markup.TypeOrgMode:
			file.HTML = string(markup.OrgMode(p, dirLink, metas))
		default:
			file.Text = toUTF8(logger, p)
		}
		overview = append(overview, file)
	}
	return overview, nil
}

// describeRepoFile reads the file of the entry to show it. Relative links in
// a file in a markup language resolve against dirLink.
func describeRepoFile(ctx context.Context, logger *logx.Logger, repo *database.Repository, commit *git.Commit, entry *git.TreeEntry, treePath, dirLink string) (*repoFile, error) {
	blob := entry.Blob()
	p, err := blob.Bytes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read blob")
	}
	file := &repoFile{
		Name:      entry.Name(),
		Size:      blob.Size(ctx),
		IsSymlink: entry.IsSymlink(),
	}
	switch {
	case tool.IsTextFile(p):
		if file.Size >= conf.UI.MaxDisplayFileSize {
			file.Kind = "too_large"
			break
		}
		content := toUTF8(logger, p)
		file.Content = &content
		indent, err := editorconfigIndent(ctx, commit, treePath)
		if err != nil {
			return nil, errors.Wrap(err, "read .editorconfig")
		}
		file.TabWidth = indent.TabWidth
		switch markup.Detect(file.Name) {
		case markup.TypeMarkdown:
			file.Kind = "markup"
			file.HTML = string(markup.MarkdownFile(p, dirLink, repo.ComposeMetas()))
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
func toUTF8(logger *logx.Logger, p []byte) string {
	content, err := template.ToUTF8WithErr(logger, p)
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
// branch: writers and those who forked the repository can, when the repository, or its base for a fork, takes pull
// requests.
func pullRequestTarget(ctx context.Context, logger *logx.Logger, repoCtx *repoContext, branch string) *repoPullRequestTarget {
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
	base, err := database.Handle.Repositories().GetByID(ctx, repo.ForkID)
	if err != nil {
		if !database.IsErrRepoNotExist(err) {
			logger.Error("Failed to get base repository", "repoID", repo.ID, "error", err)
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
func getRepoPaths(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoPathsResponse, err error) {
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

	paths, err := gitRepo.TreePaths(ctx, ref.Commit.ID.String())
	if err != nil {
		logger.Error("Failed to list paths", "ref", ref.Name, "repoID", repo.ID, "error", err)
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
