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
	EDIT             base.TplName = "repo/editor/edit"
	DIFF_PREVIEW     base.TplName = "repo/editor/diff_preview"
	DIFF_PREVIEW_NEW base.TplName = "repo/editor/diff_preview_new"
)

func editFile(ctx *context.Context, isNewFile bool) {
	ctx.Data["PageIsEdit"] = true
	ctx.Data["IsNewFile"] = isNewFile
	ctx.Data["RequireHighlightJS"] = true
	ctx.Data["RequireSimpleMDE"] = true

	branchLink := ctx.Repo.RepoLink + "/src/" + ctx.Repo.BranchName
	treeName := ctx.Repo.TreeName

	var treeNames []string
	if len(treeName) > 0 {
		treeNames = strings.Split(treeName, "/")
	}

	if !isNewFile {
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)
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
				log.Error(4, "Convert content encoding: %s", err)
			}
			ctx.Data["FileContent"] = string(buf)
		} else {
			ctx.Data["FileContent"] = content
		}
	} else {
		treeNames = append(treeNames, "") // Append empty string to allow user name the new file.
	}

	ctx.Data["TreeName"] = treeName
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

	ctx.HTML(200, EDIT)
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
	oldTreeName := ctx.Repo.TreeName
	content := form.Content
	commitChoice := form.CommitChoice
	lastCommit := form.LastCommit
	form.LastCommit = ctx.Repo.Commit.ID.String()

	if commitChoice == "commit-to-new-branch" {
		branchName = form.NewBranchName
	}

	treeName := form.TreeName
	treeName = strings.Trim(treeName, " ")
	treeName = strings.Trim(treeName, "/")

	var treeNames []string
	if len(treeName) > 0 {
		treeNames = strings.Split(treeName, "/")
	}

	ctx.Data["TreeName"] = treeName
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["FileContent"] = content
	ctx.Data["commit_summary"] = form.CommitSummary
	ctx.Data["commit_message"] = form.CommitMessage
	ctx.Data["commit_choice"] = commitChoice
	ctx.Data["new_branch_name"] = branchName
	ctx.Data["last_commit"] = form.LastCommit
	ctx.Data["MarkdownFileExts"] = strings.Join(setting.Markdown.FileExtensions, ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Repository.Editor.LineWrapExtensions, ",")
	ctx.Data["PreviewableFileModes"] = strings.Join(setting.Repository.Editor.PreviewableFileModes, ",")

	if ctx.HasError() {
		ctx.HTML(200, EDIT)
		return
	}

	if len(treeName) == 0 {
		ctx.Data["Err_Filename"] = true
		ctx.RenderWithErr(ctx.Tr("repo.editor.filename_cannot_be_empty"), EDIT, &form)
		return
	}

	if oldBranchName != branchName {
		if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
			ctx.Data["Err_Branchname"] = true
			ctx.RenderWithErr(ctx.Tr("repo.editor.branch_already_exists", branchName), EDIT, &form)
			return
		}
	}

	var treepath string
	for index, part := range treeNames {
		treepath = path.Join(treepath, part)
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treepath)
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
				ctx.Data["Err_Filename"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.directory_is_a_file", part), EDIT, &form)
				return
			}
		} else {
			if entry.IsDir() {
				ctx.Data["Err_Filename"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.filename_is_a_directory", part), EDIT, &form)
				return
			}
		}
	}

	if !isNewFile {
		_, err := ctx.Repo.Commit.GetTreeEntryByPath(oldTreeName)
		if err != nil {
			if git.IsErrNotExist(err) {
				ctx.Data["Err_Filename"] = true
				ctx.RenderWithErr(ctx.Tr("repo.editor.file_editing_no_longer_exists", oldTreeName), EDIT, &form)
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
				if file == treeName {
					ctx.RenderWithErr(ctx.Tr("repo.editor.file_changed_while_editing", ctx.Repo.RepoLink+"/compare/"+lastCommit+"..."+ctx.Repo.CommitID), EDIT, &form)
					return
				}
			}
		}
	}

	if oldTreeName != treeName {
		// We have a new filename (rename or completely new file) so we need to make sure it doesn't already exist, can't clobber.
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)
		if err != nil {
			if !git.IsErrNotExist(err) {
				ctx.Handle(500, "GetTreeEntryByPath", err)
				return
			}
		}
		if entry != nil {
			ctx.Data["Err_Filename"] = true
			ctx.RenderWithErr(ctx.Tr("repo.editor.file_already_exists", treeName), EDIT, &form)
			return
		}
	}

	var message string
	if len(form.CommitSummary) > 0 {
		message = strings.TrimSpace(form.CommitSummary)
	} else {
		if isNewFile {
			message = ctx.Tr("repo.editor.add", treeName)
		} else {
			message = ctx.Tr("repo.editor.update", treeName)
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
		OldTreeName:  oldTreeName,
		NewTreeName:  treeName,
		Message:      message,
		Content:      content,
		IsNewFile:    isNewFile,
	}); err != nil {
		ctx.Data["Err_Filename"] = true
		ctx.RenderWithErr(ctx.Tr("repo.editor.failed_to_update_file", err), EDIT, &form)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName)
}

func EditFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, false)
}

func NewFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, true)
}

func DiffPreviewPost(ctx *context.Context, form auth.EditPreviewDiffForm) {
	treeName := ctx.Repo.TreeName
	content := form.Content

	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.Data["FileContent"] = content
			ctx.HTML(200, DIFF_PREVIEW_NEW)
		} else {
			ctx.Error(500, "GetTreeEntryByPath: "+err.Error())
		}
		return
	}
	if entry.IsDir() {
		ctx.Error(422)
		return
	}

	diff, err := ctx.Repo.Repository.GetDiffPreview(ctx.Repo.BranchName, treeName, content)
	if err != nil {
		ctx.Error(500, "GetDiffPreview: "+err.Error())
		return
	}

	if diff.NumFiles() == 0 {
		ctx.PlainText(200, []byte(ctx.Tr("repo.editor.no_changes_to_show")))
		return
	}
	ctx.Data["File"] = diff.Files[0]

	ctx.HTML(200, DIFF_PREVIEW)
}

func DeleteFilePost(ctx *context.Context, form auth.DeleteRepoFileForm) {
	branchName := ctx.Repo.BranchName
	treeName := ctx.Repo.TreeName

	if ctx.HasError() {
		ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName)
		return
	}

	if err := ctx.Repo.Repository.DeleteRepoFile(ctx.User, models.DeleteRepoFileOptions{
		LastCommitID: ctx.Repo.CommitID,
		Branch:       branchName,
		TreePath:     treeName,
		Message:      form.CommitSummary,
	}); err != nil {
		ctx.Handle(500, "DeleteRepoFile", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName)
}
