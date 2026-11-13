package web

import (
	"cmp"
	"html"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/tool"
)

// limitEditorContent caps the size of a file written in the browser.
func limitEditorContent(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 10<<20) // 10 MiB
}

func repoUploadLimits() attachmentLimits {
	allowedTypes := conf.Repository.Upload.AllowedTypes
	// Unlike attachments, uploads of any type are allowed when none are listed.
	if len(allowedTypes) == 0 {
		allowedTypes = []string{"*/*"}
	}
	return attachmentLimits{
		Enabled:      conf.Repository.Upload.Enabled,
		AllowedTypes: allowedTypes,
		MaxSize:      conf.Repository.Upload.FileMaxSize,
		MaxFiles:     conf.Repository.Upload.MaxFiles,
	}
}

// withRepoEditor lets through the writers of a repository that has commits
// and is not a mirror, who can change its files in the browser. Like the
// server-rendered pages, the repository is not found for others.
func withRepoEditor(c flamego.Context, repoCtx *repoContext) {
	repo := repoCtx.Repo
	if !repoCtx.Viewer.CanWrite() || repo.IsEmpty || !repo.CanEnableEditor() {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// editableBranch reports whether files of the branch can be changed in the
// browser: it exists and does not take changes only through pull requests.
func editableBranch(c flamego.Context, repoCtx *repoContext, gitRepo *git.Repository, branch string) bool {
	return branch != "" && gitRepo.HasBranch(c.Request().Context(), branch) && !repoCtx.Repo.IsBranchRequirePullRequest(branch)
}

type repoEditorTarget struct {
	Branch string `json:"branch"`
	// Path is the path in the branch, without the branch.
	Path       string `json:"path"`
	LastCommit string `json:"lastCommit"`
	// Kind is "file", "dir", "symlink", or "none" when nothing is at the path.
	Kind string `json:"kind"`
	// Content is only set for a text file.
	Content            *string  `json:"content,omitempty"`
	MarkdownExtensions []string `json:"markdownExtensions"`
	// PreviewMarkdown tells whether Markdown files can be previewed.
	PreviewMarkdown    bool     `json:"previewMarkdown"`
	LineWrapExtensions []string `json:"lineWrapExtensions"`
	// PullRequestsAllowed tells whether committing to a new branch starts a
	// pull request.
	PullRequestsAllowed bool                     `json:"pullRequestsAllowed"`
	Upload              *issueAttachmentSettings `json:"upload,omitempty"`
}

// getRepoEditorTarget describes the branch and the path in the route, which
// is the branch name followed by the path, for the pages that change files.
// Branch names can have slashes, so the shortest leading part of the route
// that names a branch is the branch, like the server-rendered pages.
func getRepoEditorTarget(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoEditorTarget, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoEditorTarget: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	parts := strings.Split(strings.Trim(c.Param("path"), "/"), "/")
	branch, treePath := "", ""
	for i := range parts {
		if name := strings.Join(parts[:i+1], "/"); gitRepo.HasBranch(ctx, name) {
			branch, treePath = name, strings.Join(parts[i+1:], "/")
			break
		}
	}
	if !editableBranch(c, repoCtx, gitRepo, branch) {
		return http.StatusNotFound, nil, errors.New("branch does not exist")
	}
	// 🚨 SECURITY: Prevent path traversal.
	treePath = pathx.Clean(treePath)

	commit, err := gitRepo.BranchCommit(ctx, branch)
	if err != nil {
		log.Error("getRepoEditorTarget: get commit of branch %q in repo %d: %v", branch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}

	resp = &repoEditorTarget{
		Branch:              branch,
		Path:                treePath,
		LastCommit:          commit.ID.String(),
		Kind:                "dir",
		MarkdownExtensions:  conf.Markdown.FileExtensions,
		PreviewMarkdown:     slices.Contains(conf.Repository.Editor.PreviewableFileModes, "markdown"),
		LineWrapExtensions:  conf.Repository.Editor.LineWrapExtensions,
		PullRequestsAllowed: pullRequestBase(repoCtx) != nil,
		Upload:              attachmentSettings(repoUploadLimits()),
	}
	if treePath == "" {
		return http.StatusOK, resp, nil
	}
	entry, err := commit.TreeEntry(ctx, treePath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			resp.Kind = "none"
			return http.StatusOK, resp, nil
		}
		log.Error("getRepoEditorTarget: get entry %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
	}
	switch {
	case entry.IsTree():
		return http.StatusOK, resp, nil
	case entry.IsSymlink():
		resp.Kind = "symlink"
		return http.StatusOK, resp, nil
	case !entry.IsBlob() && !entry.IsExec():
		// A submodule cannot be changed here.
		resp.Kind = "none"
		return http.StatusOK, resp, nil
	}

	resp.Kind = "file"
	p, err := entry.Blob().Bytes(ctx)
	if err != nil {
		log.Error("getRepoEditorTarget: read %q of repo %d: %v", treePath, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read blob")
	}
	if tool.IsTextFile(p) {
		content, err := template.ToUTF8WithErr(p)
		if err != nil {
			content = string(p)
		}
		resp.Content = &content
	}
	return http.StatusOK, resp, nil
}

type repoEditorIndent struct {
	// Style is "tab" or "space", or left out when .editorconfig does not say.
	Style    string `json:"style,omitempty"`
	Size     int    `json:"size,omitempty"`
	TabWidth int    `json:"tabWidth,omitempty"`
}

// getRepoEditorIndent returns how the ".editorconfig" of the branch indents
// the file at the path, which need not exist yet.
func getRepoEditorIndent(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoEditorIndent, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoEditorIndent: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	branch := c.Query("branch")
	if !editableBranch(c, repoCtx, gitRepo, branch) {
		return http.StatusNotFound, nil, errors.New("branch does not exist")
	}
	commit, err := gitRepo.BranchCommit(ctx, branch)
	if err != nil {
		log.Error("getRepoEditorIndent: get commit of branch %q in repo %d: %v", branch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}

	// 🚨 SECURITY: Prevent path traversal.
	resp, err = editorconfigIndent(ctx, commit, pathx.Clean(c.Query("path")))
	if err != nil {
		log.Error("getRepoEditorIndent: get indent from .editorconfig of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, resp, nil
}

// pullRequestBase returns the repository that takes the pull requests of
// branches of the repository: its base when it is a fork, or itself. It is nil
// when that repository does not take pull requests.
func pullRequestBase(repoCtx *repoContext) *database.Repository {
	repo := repoCtx.Repo
	if repo.IsFork {
		base, err := database.GetRepositoryByID(repo.ForkID)
		if err != nil || !base.AllowsPulls() {
			return nil
		}
		return base
	}
	if repo.AllowsPulls() {
		return repo
	}
	return nil
}

type repoEditorCommitRequest struct {
	Branch string `json:"branch"`
	// NewBranch is the branch to commit to instead, which is created from
	// Branch. It is left out to commit to Branch.
	NewBranch string `json:"newBranch"`
	Summary   string `json:"summary"`
	Message   string `json:"message"`
}

type repoEditorCommitResult struct {
	// Location is where to go after the commit: the file or the directory, or
	// the form to open a pull request from the new branch.
	Location string `json:"location"`
}

// checkEditorCommit checks the branches of a commit request, and returns the
// response for a request that cannot be committed, or nil.
func checkEditorCommit(c flamego.Context, l i18n.Locale, repoCtx *repoContext, gitRepo *git.Repository, req *repoEditorCommitRequest) (statusCode int, resp any, err error) {
	if !editableBranch(c, repoCtx, gitRepo, req.Branch) {
		return http.StatusNotFound, nil, errors.New("branch does not exist")
	}
	if utf8.RuneCountInString(req.Summary) > 100 {
		msg := l.Tr("repo.editor.commit_summary") + l.Tr("form.max_size_error", "100")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"summary": &msg}}, nil
	}
	req.NewBranch = strings.TrimSpace(req.NewBranch)
	if req.NewBranch == "" || req.NewBranch == req.Branch {
		req.NewBranch = ""
		return 0, nil, nil
	}
	var msg string
	switch {
	case utf8.RuneCountInString(req.NewBranch) > 100:
		msg = l.Tr("repo.editor.new_branch_name") + l.Tr("form.max_size_error", "100")
	case gitRepo.HasBranch(c.Request().Context(), req.NewBranch):
		msg = l.Tr("repo.editor.branch_already_exists", req.NewBranch)
	case !gitRepo.IsValidBranchName(c.Request().Context(), req.NewBranch):
		msg = l.Tr("repo.editor.invalid_branch_name", req.NewBranch)
	default:
		return 0, nil, nil
	}
	return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"newBranch": &msg}}, nil
}

// commitMessage returns the summary, or the fallback when it is empty,
// followed by the extended description.
func commitMessage(req repoEditorCommitRequest, fallback string) string {
	message := strings.TrimSpace(req.Summary)
	if message == "" {
		message = fallback
	}
	if desc := strings.TrimSpace(req.Message); desc != "" {
		message += "\n\n" + desc
	}
	return message
}

// commitLocation returns where to go after committing to the request, with
// target the path that changed in the branch.
func commitLocation(repoCtx *repoContext, req repoEditorCommitRequest, target string) string {
	if req.NewBranch != "" {
		if base := pullRequestBase(repoCtx); base != nil {
			return base.Link() + "/compare/" + escapePath(req.Branch) + "..." + repoCtx.Owner.Name + ":" + escapePath(req.NewBranch)
		}
		return repoCtx.Repo.Link() + "/src/" + escapePath(req.NewBranch) + "/" + escapePath(target)
	}
	return repoCtx.Repo.Link() + "/src/" + escapePath(req.Branch) + "/" + escapePath(target)
}

// escapePath escapes each name of a slash-separated path for a URL.
func escapePath(p string) string {
	names := strings.Split(p, "/")
	for i := range names {
		names[i] = url.PathEscape(names[i])
	}
	return strings.Join(names, "/")
}

type repoEditorFileRequest struct {
	repoEditorCommitRequest
	// OldPath is the file being changed, and is left out for a new file.
	OldPath string `json:"oldPath"`
	Path    string `json:"path"`
	Content string `json:"content"`
	// LastCommit is the commit of the branch the change was made on.
	LastCommit string `json:"lastCommit"`
}

type repoEditorConflict struct {
	// Error is HTML that links to the changes made since.
	Error string `json:"error"`
	// LastCommit is the commit of the branch now, which overwrites the
	// changes made since when sent again.
	LastCommit string `json:"lastCommit"`
}

func pathError(msg string) *bindingErrorResponse {
	return &bindingErrorResponse{Fields: fieldErrors{"path": &msg}}
}

// postRepoEditorFile creates a file, or changes and possibly renames one.
func postRepoEditorFile(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoEditorFileRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	isNewFile := req.OldPath == ""
	// 🚨 SECURITY: Prevent path traversal.
	req.Path = pathx.Clean(req.Path)
	req.OldPath = pathx.Clean(req.OldPath)
	switch {
	case req.Path == "":
		return http.StatusBadRequest, pathError(l.Tr("repo.editor.filename_cannot_be_empty")), nil
	case utf8.RuneCountInString(req.Path) > 500:
		return http.StatusBadRequest, pathError(l.Tr("repo.editor.file_path") + l.Tr("form.max_size_error", "500")), nil
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("postRepoEditorFile: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if statusCode, resp, err = checkEditorCommit(c, l, repoCtx, gitRepo, &req.repoEditorCommitRequest); statusCode != 0 {
		return statusCode, resp, err
	}
	commit, err := gitRepo.BranchCommit(ctx, req.Branch)
	if err != nil {
		log.Error("postRepoEditorFile: get commit of branch %q in repo %d: %v", req.Branch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}

	// Every directory in the path must be a directory, and the path must not be
	// a directory or a symlink.
	parts := strings.Split(req.Path, "/")
	for i, part := range parts {
		entry, err := commit.TreeEntry(ctx, strings.Join(parts[:i+1], "/"))
		if err != nil {
			if git.IsErrRevisionNotExist(err) {
				break
			}
			log.Error("postRepoEditorFile: get entry %q of repo %d: %v", req.Path, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
		}
		switch {
		case i < len(parts)-1 && !entry.IsTree():
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.directory_is_a_file", part)), nil
		case i == len(parts)-1 && entry.IsSymlink():
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.file_is_a_symlink", part)), nil
		case i == len(parts)-1 && entry.IsTree():
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.filename_is_a_directory", part)), nil
		}
	}

	if !isNewFile {
		entry, err := commit.TreeEntry(ctx, req.OldPath)
		if err != nil {
			if git.IsErrRevisionNotExist(err) {
				return http.StatusNotFound, nil, errors.New(l.Tr("repo.editor.file_editing_no_longer_exists", req.OldPath))
			}
			log.Error("postRepoEditorFile: get entry %q of repo %d: %v", req.OldPath, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
		}
		// 🚨 SECURITY: Do not allow editing if the old file is a symlink.
		if entry.IsSymlink() {
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.file_is_a_symlink", req.OldPath)), nil
		}

		if req.LastCommit != commit.ID.String() {
			files, err := commit.FilesChangedAfter(ctx, req.LastCommit)
			if err != nil {
				log.Error("postRepoEditorFile: list files changed after %q in repo %d: %v", req.LastCommit, repo.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "list changed files")
			}
			if slices.Contains(files, req.OldPath) || slices.Contains(files, req.Path) {
				compareURL := repo.Link() + "/compare/" + req.LastCommit + "..." + commit.ID.String()
				return http.StatusConflict, &repoEditorConflict{
					Error:      l.Tr("repo.editor.file_changed_while_editing", html.EscapeString(compareURL)),
					LastCommit: commit.ID.String(),
				}, nil
			}
		}
	}

	if req.OldPath != req.Path {
		// A new name must not take the place of another file.
		if _, err := commit.TreeEntry(ctx, req.Path); err == nil {
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.file_already_exists", req.Path)), nil
		} else if !git.IsErrRevisionNotExist(err) {
			log.Error("postRepoEditorFile: get entry %q of repo %d: %v", req.Path, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
		}
	}

	fallback := l.Tr("repo.editor.update", req.Path)
	if isNewFile {
		fallback = l.Tr("repo.editor.add", req.Path)
	}
	err = repo.UpdateRepoFile(ctx, user, database.UpdateRepoFileOptions{
		OldBranch:   req.Branch,
		NewBranch:   cmp.Or(req.NewBranch, req.Branch),
		OldTreeName: req.OldPath,
		NewTreeName: req.Path,
		Message:     commitMessage(req.repoEditorCommitRequest, fallback),
		Content:     strings.ReplaceAll(req.Content, "\r", ""),
		IsNewFile:   isNewFile,
	})
	if err != nil {
		if database.IsErrRepoFileAlreadyExist(err) {
			return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.file_already_exists", req.Path)), nil
		}
		log.Error("postRepoEditorFile: update %q in repo %d: %v", req.Path, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update file")
	}
	return http.StatusOK, &repoEditorCommitResult{Location: commitLocation(repoCtx, req.repoEditorCommitRequest, req.Path)}, nil
}

type repoEditorDeleteRequest struct {
	repoEditorCommitRequest
	Path string `json:"path"`
}

func postRepoEditorDelete(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoEditorDeleteRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	// 🚨 SECURITY: Prevent path traversal.
	req.Path = pathx.Clean(req.Path)

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("postRepoEditorDelete: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if statusCode, resp, err = checkEditorCommit(c, l, repoCtx, gitRepo, &req.repoEditorCommitRequest); statusCode != 0 {
		return statusCode, resp, err
	}
	commit, err := gitRepo.BranchCommit(ctx, req.Branch)
	if err != nil {
		log.Error("postRepoEditorDelete: get commit of branch %q in repo %d: %v", req.Branch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}
	if req.Path == "" {
		return http.StatusNotFound, nil, errors.New("file does not exist")
	}
	entry, err := commit.TreeEntry(ctx, req.Path)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("file does not exist")
		}
		log.Error("postRepoEditorDelete: get entry %q of repo %d: %v", req.Path, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
	}
	if entry.IsTree() || entry.IsSymlink() || (!entry.IsBlob() && !entry.IsExec()) {
		return http.StatusNotFound, nil, errors.New("file does not exist")
	}

	err = repo.DeleteRepoFile(ctx, user, database.DeleteRepoFileOptions{
		LastCommitID: commit.ID.String(),
		OldBranch:    req.Branch,
		NewBranch:    cmp.Or(req.NewBranch, req.Branch),
		TreePath:     req.Path,
		Message:      commitMessage(req.repoEditorCommitRequest, l.Tr("repo.editor.delete", req.Path)),
	})
	if err != nil {
		log.Error("postRepoEditorDelete: delete %q in repo %d: %v", req.Path, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete file")
	}
	return http.StatusOK, &repoEditorCommitResult{Location: commitLocation(repoCtx, req.repoEditorCommitRequest, path.Dir("/" + req.Path)[1:])}, nil
}

type repoEditorUploadRequest struct {
	repoEditorCommitRequest
	// Path is the directory the files go to.
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

func postRepoEditorUpload(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoEditorUploadRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	// 🚨 SECURITY: Prevent path traversal.
	req.Path = pathx.Clean(req.Path)
	lim := repoUploadLimits()
	switch {
	case !lim.Enabled:
		return http.StatusNotFound, nil, errors.New("uploads are disabled")
	case len(req.Files) == 0:
		msg := l.Tr("repo.editor.no_files_to_upload")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"files": &msg}}, nil
	case len(req.Files) > lim.MaxFiles:
		return http.StatusBadRequest, nil, errors.Newf("at most %d files can be uploaded at once", lim.MaxFiles)
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("postRepoEditorUpload: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if statusCode, resp, err = checkEditorCommit(c, l, repoCtx, gitRepo, &req.repoEditorCommitRequest); statusCode != 0 {
		return statusCode, resp, err
	}
	commit, err := gitRepo.BranchCommit(ctx, req.Branch)
	if err != nil {
		log.Error("postRepoEditorUpload: get commit of branch %q in repo %d: %v", req.Branch, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
	}

	// Files can only go into directories.
	if req.Path != "" {
		parts := strings.Split(req.Path, "/")
		for i, part := range parts {
			entry, err := commit.TreeEntry(ctx, strings.Join(parts[:i+1], "/"))
			if err != nil {
				if git.IsErrRevisionNotExist(err) {
					break
				}
				log.Error("postRepoEditorUpload: get entry %q of repo %d: %v", req.Path, repo.ID, err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "get tree entry")
			}
			if !entry.IsTree() {
				return http.StatusUnprocessableEntity, pathError(l.Tr("repo.editor.directory_is_a_file", part)), nil
			}
		}
	}

	uploads, err := database.GetUploadsByUUIDs(req.Files)
	if err != nil {
		log.Error("postRepoEditorUpload: get uploads: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get uploads")
	} else if len(uploads) != len(req.Files) {
		return http.StatusBadRequest, nil, errors.New("some files were not uploaded")
	}

	err = repo.UploadRepoFiles(ctx, user, database.UploadRepoFileOptions{
		LastCommitID: commit.ID.String(),
		OldBranch:    req.Branch,
		NewBranch:    cmp.Or(req.NewBranch, req.Branch),
		TreePath:     req.Path,
		Message:      commitMessage(req.repoEditorCommitRequest, l.Tr("repo.editor.upload_files_to_dir", req.Path)),
		Files:        req.Files,
	})
	if err != nil {
		log.Error("postRepoEditorUpload: upload files to %q in repo %d: %v", req.Path, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "upload files")
	}
	return http.StatusOK, &repoEditorCommitResult{Location: commitLocation(repoCtx, req.repoEditorCommitRequest, req.Path)}, nil
}

// postRepoEditorUploadFile stores a file to commit with the next upload.
func postRepoEditorUploadFile(c flamego.Context, l i18n.Locale) (statusCode int, resp any, err error) {
	f, statusCode, err := readUploadedFile(c, l, repoUploadLimits())
	if err != nil {
		return statusCode, nil, err
	}
	defer func() { _ = f.File.Close() }()

	upload, err := database.NewUpload(f.Header.Filename, f.Head, f.File)
	if err != nil {
		log.Error("postRepoEditorUploadFile: store upload: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "store upload")
	}
	return http.StatusCreated, map[string]string{"uuid": upload.UUID}, nil
}

// deleteRepoEditorUploadFile removes a stored file that will not be committed.
func deleteRepoEditorUploadFile(c flamego.Context) (statusCode int, resp any, err error) {
	if err = database.DeleteUploadByUUID(c.Param("uuid")); err != nil {
		log.Error("deleteRepoEditorUploadFile: delete upload %q: %v", c.Param("uuid"), err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete upload")
	}
	return http.StatusNoContent, nil, nil
}
