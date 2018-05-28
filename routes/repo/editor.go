// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	log "gopkg.in/clog.v1"

	"github.com/gogs/git-module"
	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/template"
	"github.com/gogs/gogs/pkg/tool"
)

const (
	EDIT_FILE         = "repo/editor/edit"
	EDIT_DIFF_PREVIEW = "repo/editor/diff_preview"
	DELETE_FILE       = "repo/editor/delete"
	UPLOAD_FILE       = "repo/editor/upload"
)

// getParentTreeFields returns list of parent tree names and corresponding tree paths
// based on given tree path.
func getParentTreeFields(treePath string) (treeNames []string, treePaths []string) {
	if len(treePath) == 0 {
		return treeNames, treePaths
	}

	treeNames = strings.Split(treePath, "/")
	treePaths = make([]string, len(treeNames))
	for i := range treeNames {
		treePaths[i] = strings.Join(treeNames[:i+1], "/")
	}
	return treeNames, treePaths
}

func editFile(c *context.Context, isNewFile bool) {
	c.PageIs("Edit")
	c.RequireHighlightJS()
	c.RequireSimpleMDE()
	c.Data["IsNewFile"] = isNewFile

	treeNames, treePaths := getParentTreeFields(c.Repo.TreePath)

	if !isNewFile {
		entry, err := c.Repo.Commit.GetTreeEntryByPath(c.Repo.TreePath)
		if err != nil {
			c.NotFoundOrServerError("GetTreeEntryByPath", git.IsErrNotExist, err)
			return
		}

		// No way to edit a directory online.
		if entry.IsDir() {
			c.NotFound()
			return
		}

		blob := entry.Blob()
		dataRc, err := blob.Data()
		if err != nil {
			c.ServerError("blob.Data", err)
			return
		}

		c.Data["FileSize"] = blob.Size()
		c.Data["FileName"] = blob.Name()

		buf := make([]byte, 1024)
		n, _ := dataRc.Read(buf)
		buf = buf[:n]

		// Only text file are editable online.
		if !tool.IsTextFile(buf) {
			c.NotFound()
			return
		}

		d, _ := ioutil.ReadAll(dataRc)
		buf = append(buf, d...)
		if err, content := template.ToUTF8WithErr(buf); err != nil {
			if err != nil {
				log.Error(2, "ToUTF8WithErr: %v", err)
			}
			c.Data["FileContent"] = string(buf)
		} else {
			c.Data["FileContent"] = content
		}
	} else {
		treeNames = append(treeNames, "") // Append empty string to allow user name the new file.
	}

	c.Data["ParentTreePath"] = path.Dir(c.Repo.TreePath)
	c.Data["TreeNames"] = treeNames
	c.Data["TreePaths"] = treePaths
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + c.Repo.BranchName
	c.Data["commit_summary"] = ""
	c.Data["commit_message"] = ""
	c.Data["commit_choice"] = "direct"
	c.Data["new_branch_name"] = ""
	c.Data["last_commit"] = c.Repo.Commit.ID
	c.Data["MarkdownFileExts"] = strings.Join(setting.Markdown.FileExtensions, ",")
	c.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	c.Data["PreviewableFileModes"] = strings.Join(setting.Repository.Editor.PreviewableFileModes, ",")
	c.Data["EditorconfigURLPrefix"] = fmt.Sprintf("%s/api/v1/repos/%s/editorconfig/", setting.AppSubURL, c.Repo.Repository.FullName())

	c.Success(EDIT_FILE)
}

func EditFile(c *context.Context) {
	editFile(c, false)
}

func NewFile(c *context.Context) {
	editFile(c, true)
}

func editFilePost(c *context.Context, f form.EditRepoFile, isNewFile bool) {
	c.PageIs("Edit")
	c.RequireHighlightJS()
	c.RequireSimpleMDE()
	c.Data["IsNewFile"] = isNewFile

	oldBranchName := c.Repo.BranchName
	branchName := oldBranchName
	oldTreePath := c.Repo.TreePath
	lastCommit := f.LastCommit
	f.LastCommit = c.Repo.Commit.ID.String()

	if f.IsNewBrnach() {
		branchName = f.NewBranchName
	}

	f.TreePath = strings.Trim(path.Clean("/"+f.TreePath), " /")
	treeNames, treePaths := getParentTreeFields(f.TreePath)

	c.Data["ParentTreePath"] = path.Dir(c.Repo.TreePath)
	c.Data["TreePath"] = f.TreePath
	c.Data["TreeNames"] = treeNames
	c.Data["TreePaths"] = treePaths
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + branchName
	c.Data["FileContent"] = f.Content
	c.Data["commit_summary"] = f.CommitSummary
	c.Data["commit_message"] = f.CommitMessage
	c.Data["commit_choice"] = f.CommitChoice
	c.Data["new_branch_name"] = branchName
	c.Data["last_commit"] = f.LastCommit
	c.Data["MarkdownFileExts"] = strings.Join(setting.Markdown.FileExtensions, ",")
	c.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	c.Data["PreviewableFileModes"] = strings.Join(setting.Repository.Editor.PreviewableFileModes, ",")

	if c.HasError() {
		c.Success(EDIT_FILE)
		return
	}

	if len(f.TreePath) == 0 {
		c.FormErr("TreePath")
		c.RenderWithErr(c.Tr("repo.editor.filename_cannot_be_empty"), EDIT_FILE, &f)
		return
	}

	if oldBranchName != branchName {
		if _, err := c.Repo.Repository.GetBranch(branchName); err == nil {
			c.FormErr("NewBranchName")
			c.RenderWithErr(c.Tr("repo.editor.branch_already_exists", branchName), EDIT_FILE, &f)
			return
		}
	}

	var newTreePath string
	for index, part := range treeNames {
		newTreePath = path.Join(newTreePath, part)
		entry, err := c.Repo.Commit.GetTreeEntryByPath(newTreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				// Means there is no item with that name, so we're good
				break
			}

			c.ServerError("Repo.Commit.GetTreeEntryByPath", err)
			return
		}
		if index != len(treeNames)-1 {
			if !entry.IsDir() {
				c.FormErr("TreePath")
				c.RenderWithErr(c.Tr("repo.editor.directory_is_a_file", part), EDIT_FILE, &f)
				return
			}
		} else {
			if entry.IsLink() {
				c.FormErr("TreePath")
				c.RenderWithErr(c.Tr("repo.editor.file_is_a_symlink", part), EDIT_FILE, &f)
				return
			} else if entry.IsDir() {
				c.FormErr("TreePath")
				c.RenderWithErr(c.Tr("repo.editor.filename_is_a_directory", part), EDIT_FILE, &f)
				return
			}
		}
	}

	if !isNewFile {
		_, err := c.Repo.Commit.GetTreeEntryByPath(oldTreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				c.FormErr("TreePath")
				c.RenderWithErr(c.Tr("repo.editor.file_editing_no_longer_exists", oldTreePath), EDIT_FILE, &f)
			} else {
				c.ServerError("GetTreeEntryByPath", err)
			}
			return
		}
		if lastCommit != c.Repo.CommitID {
			files, err := c.Repo.Commit.GetFilesChangedSinceCommit(lastCommit)
			if err != nil {
				c.ServerError("GetFilesChangedSinceCommit", err)
				return
			}

			for _, file := range files {
				if file == f.TreePath {
					c.RenderWithErr(c.Tr("repo.editor.file_changed_while_editing", c.Repo.RepoLink+"/compare/"+lastCommit+"..."+c.Repo.CommitID), EDIT_FILE, &f)
					return
				}
			}
		}
	}

	if oldTreePath != f.TreePath {
		// We have a new filename (rename or completely new file) so we need to make sure it doesn't already exist, can't clobber.
		entry, err := c.Repo.Commit.GetTreeEntryByPath(f.TreePath)
		if err != nil {
			if !git.IsErrNotExist(err) {
				c.ServerError("GetTreeEntryByPath", err)
				return
			}
		}
		if entry != nil {
			c.FormErr("TreePath")
			c.RenderWithErr(c.Tr("repo.editor.file_already_exists", f.TreePath), EDIT_FILE, &f)
			return
		}
	}

	message := strings.TrimSpace(f.CommitSummary)
	if len(message) == 0 {
		if isNewFile {
			message = c.Tr("repo.editor.add", f.TreePath)
		} else {
			message = c.Tr("repo.editor.update", f.TreePath)
		}
	}

	f.CommitMessage = strings.TrimSpace(f.CommitMessage)
	if len(f.CommitMessage) > 0 {
		message += "\n\n" + f.CommitMessage
	}

	if err := c.Repo.Repository.UpdateRepoFile(c.User, models.UpdateRepoFileOptions{
		LastCommitID: lastCommit,
		OldBranch:    oldBranchName,
		NewBranch:    branchName,
		OldTreeName:  oldTreePath,
		NewTreeName:  f.TreePath,
		Message:      message,
		Content:      strings.Replace(f.Content, "\r", "", -1),
		IsNewFile:    isNewFile,
	}); err != nil {
		c.FormErr("TreePath")
		c.RenderWithErr(c.Tr("repo.editor.fail_to_update_file", f.TreePath, err), EDIT_FILE, &f)
		return
	}

	if f.IsNewBrnach() && c.Repo.PullRequest.Allowed {
		c.Redirect(c.Repo.PullRequestURL(oldBranchName, f.NewBranchName))
	} else {
		c.Redirect(c.Repo.RepoLink + "/src/" + branchName + "/" + template.EscapePound(f.TreePath))
	}
}

func EditFilePost(c *context.Context, f form.EditRepoFile) {
	editFilePost(c, f, false)
}

func NewFilePost(c *context.Context, f form.EditRepoFile) {
	editFilePost(c, f, true)
}

func DiffPreviewPost(c *context.Context, f form.EditPreviewDiff) {
	treePath := c.Repo.TreePath

	entry, err := c.Repo.Commit.GetTreeEntryByPath(treePath)
	if err != nil {
		c.Error(500, "GetTreeEntryByPath: "+err.Error())
		return
	} else if entry.IsDir() {
		c.Error(422)
		return
	}

	diff, err := c.Repo.Repository.GetDiffPreview(c.Repo.BranchName, treePath, f.Content)
	if err != nil {
		c.Error(500, "GetDiffPreview: "+err.Error())
		return
	}

	if diff.NumFiles() == 0 {
		c.PlainText(200, []byte(c.Tr("repo.editor.no_changes_to_show")))
		return
	}
	c.Data["File"] = diff.Files[0]

	c.HTML(200, EDIT_DIFF_PREVIEW)
}

func DeleteFile(c *context.Context) {
	c.Data["PageIsDelete"] = true
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + c.Repo.BranchName
	c.Data["TreePath"] = c.Repo.TreePath
	c.Data["commit_summary"] = ""
	c.Data["commit_message"] = ""
	c.Data["commit_choice"] = "direct"
	c.Data["new_branch_name"] = ""
	c.HTML(200, DELETE_FILE)
}

func DeleteFilePost(c *context.Context, f form.DeleteRepoFile) {
	c.Data["PageIsDelete"] = true
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + c.Repo.BranchName
	c.Data["TreePath"] = c.Repo.TreePath

	oldBranchName := c.Repo.BranchName
	branchName := oldBranchName

	if f.IsNewBrnach() {
		branchName = f.NewBranchName
	}
	c.Data["commit_summary"] = f.CommitSummary
	c.Data["commit_message"] = f.CommitMessage
	c.Data["commit_choice"] = f.CommitChoice
	c.Data["new_branch_name"] = branchName

	if c.HasError() {
		c.HTML(200, DELETE_FILE)
		return
	}

	if oldBranchName != branchName {
		if _, err := c.Repo.Repository.GetBranch(branchName); err == nil {
			c.Data["Err_NewBranchName"] = true
			c.RenderWithErr(c.Tr("repo.editor.branch_already_exists", branchName), DELETE_FILE, &f)
			return
		}
	}

	message := strings.TrimSpace(f.CommitSummary)
	if len(message) == 0 {
		message = c.Tr("repo.editor.delete", c.Repo.TreePath)
	}

	f.CommitMessage = strings.TrimSpace(f.CommitMessage)
	if len(f.CommitMessage) > 0 {
		message += "\n\n" + f.CommitMessage
	}

	if err := c.Repo.Repository.DeleteRepoFile(c.User, models.DeleteRepoFileOptions{
		LastCommitID: c.Repo.CommitID,
		OldBranch:    oldBranchName,
		NewBranch:    branchName,
		TreePath:     c.Repo.TreePath,
		Message:      message,
	}); err != nil {
		c.Handle(500, "DeleteRepoFile", err)
		return
	}

	if f.IsNewBrnach() && c.Repo.PullRequest.Allowed {
		c.Redirect(c.Repo.PullRequestURL(oldBranchName, f.NewBranchName))
	} else {
		c.Flash.Success(c.Tr("repo.editor.file_delete_success", c.Repo.TreePath))
		c.Redirect(c.Repo.RepoLink + "/src/" + branchName)
	}
}

func renderUploadSettings(c *context.Context) {
	c.Data["RequireDropzone"] = true
	c.Data["UploadAllowedTypes"] = strings.Join(setting.Repository.Upload.AllowedTypes, ",")
	c.Data["UploadMaxSize"] = setting.Repository.Upload.FileMaxSize
	c.Data["UploadMaxFiles"] = setting.Repository.Upload.MaxFiles
}

func UploadFile(c *context.Context) {
	c.Data["PageIsUpload"] = true
	renderUploadSettings(c)

	treeNames, treePaths := getParentTreeFields(c.Repo.TreePath)
	if len(treeNames) == 0 {
		// We must at least have one element for user to input.
		treeNames = []string{""}
	}

	c.Data["TreeNames"] = treeNames
	c.Data["TreePaths"] = treePaths
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + c.Repo.BranchName
	c.Data["commit_summary"] = ""
	c.Data["commit_message"] = ""
	c.Data["commit_choice"] = "direct"
	c.Data["new_branch_name"] = ""

	c.HTML(200, UPLOAD_FILE)
}

func UploadFilePost(c *context.Context, f form.UploadRepoFile) {
	c.Data["PageIsUpload"] = true
	renderUploadSettings(c)

	oldBranchName := c.Repo.BranchName
	branchName := oldBranchName

	if f.IsNewBrnach() {
		branchName = f.NewBranchName
	}

	f.TreePath = strings.Trim(path.Clean("/"+f.TreePath), " /")
	treeNames, treePaths := getParentTreeFields(f.TreePath)
	if len(treeNames) == 0 {
		// We must at least have one element for user to input.
		treeNames = []string{""}
	}

	c.Data["TreePath"] = f.TreePath
	c.Data["TreeNames"] = treeNames
	c.Data["TreePaths"] = treePaths
	c.Data["BranchLink"] = c.Repo.RepoLink + "/src/" + branchName
	c.Data["commit_summary"] = f.CommitSummary
	c.Data["commit_message"] = f.CommitMessage
	c.Data["commit_choice"] = f.CommitChoice
	c.Data["new_branch_name"] = branchName

	if c.HasError() {
		c.HTML(200, UPLOAD_FILE)
		return
	}

	if oldBranchName != branchName {
		if _, err := c.Repo.Repository.GetBranch(branchName); err == nil {
			c.Data["Err_NewBranchName"] = true
			c.RenderWithErr(c.Tr("repo.editor.branch_already_exists", branchName), UPLOAD_FILE, &f)
			return
		}
	}

	var newTreePath string
	for _, part := range treeNames {
		newTreePath = path.Join(newTreePath, part)
		entry, err := c.Repo.Commit.GetTreeEntryByPath(newTreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				// Means there is no item with that name, so we're good
				break
			}

			c.Handle(500, "Repo.Commit.GetTreeEntryByPath", err)
			return
		}

		// User can only upload files to a directory.
		if !entry.IsDir() {
			c.Data["Err_TreePath"] = true
			c.RenderWithErr(c.Tr("repo.editor.directory_is_a_file", part), UPLOAD_FILE, &f)
			return
		}
	}

	message := strings.TrimSpace(f.CommitSummary)
	if len(message) == 0 {
		message = c.Tr("repo.editor.upload_files_to_dir", f.TreePath)
	}

	f.CommitMessage = strings.TrimSpace(f.CommitMessage)
	if len(f.CommitMessage) > 0 {
		message += "\n\n" + f.CommitMessage
	}

	if err := c.Repo.Repository.UploadRepoFiles(c.User, models.UploadRepoFileOptions{
		LastCommitID: c.Repo.CommitID,
		OldBranch:    oldBranchName,
		NewBranch:    branchName,
		TreePath:     f.TreePath,
		Message:      message,
		Files:        f.Files,
	}); err != nil {
		c.Data["Err_TreePath"] = true
		c.RenderWithErr(c.Tr("repo.editor.unable_to_upload_files", f.TreePath, err), UPLOAD_FILE, &f)
		return
	}

	if f.IsNewBrnach() && c.Repo.PullRequest.Allowed {
		c.Redirect(c.Repo.PullRequestURL(oldBranchName, f.NewBranchName))
	} else {
		c.Redirect(c.Repo.RepoLink + "/src/" + branchName + "/" + f.TreePath)
	}
}

func UploadFileToServer(c *context.Context) {
	file, header, err := c.Req.FormFile("file")
	if err != nil {
		c.Error(500, fmt.Sprintf("FormFile: %v", err))
		return
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, _ := file.Read(buf)
	if n > 0 {
		buf = buf[:n]
	}
	fileType := http.DetectContentType(buf)

	if len(setting.Repository.Upload.AllowedTypes) > 0 {
		allowed := false
		for _, t := range setting.Repository.Upload.AllowedTypes {
			t := strings.Trim(t, " ")
			if t == "*/*" || t == fileType {
				allowed = true
				break
			}
		}

		if !allowed {
			c.Error(400, ErrFileTypeForbidden.Error())
			return
		}
	}

	upload, err := models.NewUpload(header.Filename, buf, file)
	if err != nil {
		c.Error(500, fmt.Sprintf("NewUpload: %v", err))
		return
	}

	log.Trace("New file uploaded: %s", upload.UUID)
	c.JSON(200, map[string]string{
		"uuid": upload.UUID,
	})
}

func RemoveUploadFileFromServer(c *context.Context, f form.RemoveUploadFile) {
	if len(f.File) == 0 {
		c.Status(204)
		return
	}

	if err := models.DeleteUploadByUUID(f.File); err != nil {
		c.Error(500, fmt.Sprintf("DeleteUploadByUUID: %v", err))
		return
	}

	log.Trace("Upload file removed: %s", f.File)
	c.Status(204)
}
