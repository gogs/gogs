// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"io/ioutil"
	"path"
	"strings"

	"github.com/gogits/git-module"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/template"
)

const (
	EDIT_FILE         base.TplName = "repo/editor/edit"
	EDIT_DIFF_PREVIEW base.TplName = "repo/editor/diff_preview"
	DELETE_FILE       base.TplName = "repo/editor/delete"
)

func editFile(ctx *context.Context, isNewFile bool) {
	ctx.Data["PageIsEdit"] = true
	ctx.Data["IsNewFile"] = isNewFile
	ctx.Data["RequireHighlightJS"] = true
	ctx.Data["RequireSimpleMDE"] = true

	branchLink := ctx.Repo.RepoLink + "/src/" + ctx.Repo.BranchName

	var treeNames []string
	if len(ctx.Repo.TreePath) > 0 {
		treeNames = strings.Split(ctx.Repo.TreePath, "/")
	}

	if !isNewFile {
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(ctx.Repo.TreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				ctx.Handle(404, "GetTreeEntryByPath", err)
			} else {
				ctx.Handle(500, "GetTreeEntryByPath", err)
			}
			return
		}

		// No way to edit a directory online.
		if entry.IsDir() {
			ctx.Handle(404, "", nil)
			return
		}

		blob := entry.Blob()
		dataRc, err := blob.Data()
		if err != nil {
			ctx.Handle(404, "blob.Data", err)
			return
		}

		ctx.Data["FileSize"] = blob.Size()
		ctx.Data["FileName"] = blob.Name()

		buf := make([]byte, 1024)
		n, _ := dataRc.Read(buf)
		if n > 0 {
			buf = buf[:n]
		}

		// Only text file are editable online.
		_, isTextFile := base.IsTextFile(buf)
		if !isTextFile {
			ctx.Handle(404, "", nil)
			return
		}

		d, _ := ioutil.ReadAll(dataRc)
		buf = append(buf, d...)
		if err, content := template.ToUTF8WithErr(buf); err != nil {
			if err != nil {
				log.Error(4, "ToUTF8WithErr: %v", err)
			}
			ctx.Data["FileContent"] = string(buf)
		} else {
			ctx.Data["FileContent"] = content
		}
	} else {
		treeNames = append(treeNames, "") // Append empty string to allow user name the new file.
	}

	ctx.Data["TreePath"] = ctx.Repo.TreePath
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["commit_summary"] = ""
	ctx.Data["commit_message"] = ""
	ctx.Data["commit_choice"] = "direct"
	ctx.Data["new_branch_name"] = ""
	ctx.Data["last_commit"] = ctx.Repo.Commit.ID
	ctx.Data["MarkdownFileExts"] = strings.Join(setting.Markdown.FileExtensions, ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	ctx.Data["PreviewableFileModes"] = strings.Join(setting.Repository.Editor.PreviewableFileModes, ",")

	ctx.HTML(200, EDIT_FILE)
}

func EditFile(ctx *context.Context) {
	editFile(ctx, false)
}

func NewFile(ctx *context.Context) {
	editFile(ctx, true)
}

func editFilePost(ctx *context.Context, form auth.EditRepoFileForm, isNewFile bool) {
	ctx.Data["PageIsEdit"] = true
	ctx.Data["IsNewFile"] = isNewFile
	ctx.Data["RequireHighlightJS"] = true
	ctx.Data["RequireSimpleMDE"] = true

	oldBranchName := ctx.Repo.BranchName
	branchName := oldBranchName
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	oldTreePath := ctx.Repo.TreePath
	lastCommit := form.LastCommit
	form.LastCommit = ctx.Repo.Commit.ID.String()

	if form.CommitChoice == "commit-to-new-branch" {
		branchName = form.NewBranchName
	}

	form.TreePath = strings.Trim(form.TreePath, " /")

	var treeNames []string
	if len(form.TreePath) > 0 {
		treeNames = strings.Split(form.TreePath, "/")
	}

	ctx.Data["TreePath"] = form.TreePath
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["FileContent"] = form.Content
	ctx.Data["commit_summary"] = form.CommitSummary
	ctx.Data["commit_message"] = form.CommitMessage
	ctx.Data["commit_choice"] = form.CommitChoice
	ctx.Data["new_branch_name"] = branchName
	ctx.Data["last_commit"] = form.LastCommit
	ctx.Data["MarkdownFileExts"] = strings.Join(setting.Markdown.FileExtensions, ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	ctx.Data["PreviewableFileModes"] = strings.Join(setting.Repository.Editor.PreviewableFileModes, ",")

	if ctx.HasError() {
		ctx.HTML(200, EDIT_FILE)
		return
	}

	if len(form.TreePath) == 0 {
		ctx.Data["Err_TreePath"] = true
		ctx.RenderWithErr(ctx.Tr("repo.editor.filename_cannot_be_empty"), EDIT_FILE, &form)
		return
	}

	if oldBranchName != branchName {
		if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
			ctx.Data["Err_NewBranchName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.editor.branch_already_exists", branchName), EDIT_FILE, &form)
			return
		}
	}

	var newTreePath string
	for index, part := range treeNames {
		newTreePath = path.Join(newTreePath, part)
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(newTreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				// Means there is no item with that name, so we're good
				break
			}

			ctx.Handle(500, "GetTreeEntryByPath", err)
			return
		}
		if index != len(treeNames)-1 {
			if !entry.IsDir() {
				ctx.Data["Err_TreePath"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.directory_is_a_file", part), EDIT_FILE, &form)
				return
			}
		} else {
			if entry.IsDir() {
				ctx.Data["Err_TreePath"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.filename_is_a_directory", part), EDIT_FILE, &form)
				return
			}
		}
	}

	if !isNewFile {
		_, err := ctx.Repo.Commit.GetTreeEntryByPath(oldTreePath)
		if err != nil {
			if git.IsErrNotExist(err) {
				ctx.Data["Err_TreePath"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.file_editing_no_longer_exists", oldTreePath), EDIT_FILE, &form)
			} else {
				ctx.Handle(500, "GetTreeEntryByPath", err)
			}
			return
		}
		if lastCommit != ctx.Repo.CommitID {
			files, err := ctx.Repo.Commit.GetFilesChangedSinceCommit(lastCommit)
			if err != nil {
				ctx.Handle(500, "GetFilesChangedSinceCommit", err)
				return
			}

			for _, file := range files {
				if file == form.TreePath {
					ctx.RenderWithErr(ctx.Tr("repo.editor.file_changed_while_editing", ctx.Repo.RepoLink+"/compare/"+lastCommit+"..."+ctx.Repo.CommitID), EDIT_FILE, &form)
					return
				}
			}
		}
	}

	if oldTreePath != form.TreePath {
		// We have a new filename (rename or completely new file) so we need to make sure it doesn't already exist, can't clobber.
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(form.TreePath)
		if err != nil {
			if !git.IsErrNotExist(err) {
				ctx.Handle(500, "GetTreeEntryByPath", err)
				return
			}
		}
		if entry != nil {
			ctx.Data["Err_TreePath"] = true
			ctx.RenderWithErr(ctx.Tr("repo.editor.file_already_exists", form.TreePath), EDIT_FILE, &form)
			return
		}
	}

	message := strings.TrimSpace(form.CommitSummary)
	if len(message) == 0 {
		if isNewFile {
			message = ctx.Tr("repo.editor.add", form.TreePath)
		} else {
			message = ctx.Tr("repo.editor.update", form.TreePath)
		}
	}

	form.CommitMessage = strings.TrimSpace(form.CommitMessage)
	if len(form.CommitMessage) > 0 {
		message += "\n\n" + form.CommitMessage
	}

	if err := ctx.Repo.Repository.UpdateRepoFile(ctx.User, models.UpdateRepoFileOptions{
		LastCommitID: lastCommit,
		OldBranch:    oldBranchName,
		NewBranch:    branchName,
		OldTreeName:  oldTreePath,
		NewTreeName:  form.TreePath,
		Message:      message,
		Content:      strings.Replace(form.Content, "\r", "", -1),
		IsNewFile:    isNewFile,
	}); err != nil {
		ctx.Data["Err_TreePath"] = true
		ctx.RenderWithErr(ctx.Tr("repo.editor.fail_to_update_file", form.TreePath, err), EDIT_FILE, &form)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + form.TreePath)
}

func EditFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, false)
}

func NewFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, true)
}

func DiffPreviewPost(ctx *context.Context, form auth.EditPreviewDiffForm) {
	treePath := ctx.Repo.TreePath

	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treePath)
	if err != nil {
		ctx.Error(500, "GetTreeEntryByPath: "+err.Error())
		return
	} else if entry.IsDir() {
		ctx.Error(422)
		return
	}

	diff, err := ctx.Repo.Repository.GetDiffPreview(ctx.Repo.BranchName, treePath, form.Content)
	if err != nil {
		ctx.Error(500, "GetDiffPreview: "+err.Error())
		return
	}

	if diff.NumFiles() == 0 {
		ctx.PlainText(200, []byte(ctx.Tr("repo.editor.no_changes_to_show")))
		return
	}
	ctx.Data["File"] = diff.Files[0]

	ctx.HTML(200, EDIT_DIFF_PREVIEW)
}

func DeleteFile(ctx *context.Context) {
	ctx.Data["PageIsDelete"] = true
	ctx.Data["BranchLink"] = ctx.Repo.RepoLink + "/src/" + ctx.Repo.BranchName
	ctx.Data["TreePath"] = ctx.Repo.TreePath
	ctx.Data["commit_summary"] = ""
	ctx.Data["commit_message"] = ""
	ctx.Data["commit_choice"] = "direct"
	ctx.Data["new_branch_name"] = ""
	ctx.HTML(200, DELETE_FILE)
}

func DeleteFilePost(ctx *context.Context, form auth.DeleteRepoFileForm) {
	ctx.Data["PageIsDelete"] = true
	ctx.Data["BranchLink"] = ctx.Repo.RepoLink + "/src/" + ctx.Repo.BranchName
	ctx.Data["TreePath"] = ctx.Repo.TreePath

	oldBranchName := ctx.Repo.BranchName
	branchName := oldBranchName
	treePath := ctx.Repo.TreePath

	if form.CommitChoice == "commit-to-new-branch" {
		branchName = form.NewBranchName
	}
	ctx.Data["commit_summary"] = form.CommitSummary
	ctx.Data["commit_message"] = form.CommitMessage
	ctx.Data["commit_choice"] = form.CommitChoice
	ctx.Data["new_branch_name"] = branchName

	if ctx.HasError() {
		ctx.HTML(200, DELETE_FILE)
		return
	}

	if oldBranchName != branchName {
		if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
			ctx.Data["Err_NewBranchName"] = true
			ctx.RenderWithErr(ctx.Tr("repo.editor.branch_already_exists", branchName), DELETE_FILE, &form)
			return
		}
	}

	message := strings.TrimSpace(form.CommitSummary)
	if len(message) == 0 {
		message = ctx.Tr("repo.editor.delete", treePath)
	}

	form.CommitMessage = strings.TrimSpace(form.CommitMessage)
	if len(form.CommitMessage) > 0 {
		message += "\n\n" + form.CommitMessage
	}

	if err := ctx.Repo.Repository.DeleteRepoFile(ctx.User, models.DeleteRepoFileOptions{
		LastCommitID: ctx.Repo.CommitID,
		OldBranch:    oldBranchName,
		NewBranch:    branchName,
		TreePath:     treePath,
		Message:      message,
	}); err != nil {
		ctx.Handle(500, "DeleteRepoFile", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.editor.file_delete_success", treePath))
	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName)
}
