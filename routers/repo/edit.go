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
	EDIT             base.TplName = "repo/edit"
	DIFF_PREVIEW     base.TplName = "repo/diff_preview"
	DIFF_PREVIEW_NEW base.TplName = "repo/diff_preview_new"
)

func EditFile(ctx *context.Context) {
	editFile(ctx, false)
}

func NewFile(ctx *context.Context) {
	editFile(ctx, true)
}

func editFile(ctx *context.Context, isNewFile bool) {
	ctx.Data["PageIsEdit"] = true
	ctx.Data["IsNewFile"] = isNewFile
	ctx.Data["RequireHighlightJS"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	branchName := ctx.Repo.BranchName
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	treeName := ctx.Repo.TreeName

	var treeNames []string
	if len(treeName) > 0 {
		treeNames = strings.Split(treeName, "/")
	}

	if !isNewFile {
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)

		if err != nil && git.IsErrNotExist(err) {
			ctx.Handle(404, "GetTreeEntryByPath", err)
			return
		}

		if (ctx.Repo.IsViewCommit) || entry == nil || entry.IsDir() {
			ctx.Handle(404, "repo.Home", nil)
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

		_, isTextFile := base.IsTextFile(buf)

		if !isTextFile {
			ctx.Handle(404, "repo.Home", nil)
			return
		}

		d, _ := ioutil.ReadAll(dataRc)
		buf = append(buf, d...)

		if err, content := template.ToUtf8WithErr(buf); err != nil {
			if err != nil {
				log.Error(4, "Convert content encoding: %s", err)
			}
			ctx.Data["FileContent"] = string(buf)
		} else {
			ctx.Data["FileContent"] = content
		}
	} else {
		treeNames = append(treeNames, "")
	}

	ctx.Data["RequireSimpleMDE"] = true

	ctx.Data["UserName"] = userName
	ctx.Data["RepoName"] = repoName
	ctx.Data["BranchName"] = branchName
	ctx.Data["TreeName"] = treeName
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["CommitSummary"] = ""
	ctx.Data["CommitMessage"] = ""
	ctx.Data["CommitChoice"] = "direct"
	ctx.Data["NewBranchName"] = ""
	ctx.Data["CommitDirectlyToThisBranch"] = ctx.Tr("repo.commit_directly_to_this_branch", "<strong class=\"branch-name\">"+branchName+"</strong>")
	ctx.Data["CreateNewBranch"] = ctx.Tr("repo.create_new_branch", "<strong>"+ctx.Tr("repo.new_branch")+"</strong>")
	ctx.Data["LastCommit"] = ctx.Repo.Commit.ID
	ctx.Data["MdFileExtensions"] = strings.Join(setting.Markdown.MdFileExtensions, ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Editor.LineWrapExtensions, ",")
	ctx.Data["PreviewTabApis"] = strings.Join(setting.Editor.PreviewTabApis, ",")
	ctx.Data["PreviewDiffUrl"] = ctx.Repo.RepoLink + "/preview/" + branchName + "/" + treeName

	ctx.HTML(200, EDIT)
}

func EditFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, false)
}

func NewFilePost(ctx *context.Context, form auth.EditRepoFileForm) {
	editFilePost(ctx, form, true)
}

func editFilePost(ctx *context.Context, form auth.EditRepoFileForm, isNewFile bool) {
	ctx.Data["PageIsEdit"] = true
	ctx.Data["IsNewFile"] = isNewFile
	ctx.Data["RequireHighlightJS"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	oldBranchName := ctx.Repo.BranchName
	branchName := oldBranchName
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	oldTreeName := ctx.Repo.TreeName
	content := form.Content
	commitChoice := form.CommitChoice
	lastCommit := form.LastCommit

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

	ctx.Data["RequireSimpleMDE"] = true

	ctx.Data["UserName"] = userName
	ctx.Data["RepoName"] = repoName
	ctx.Data["BranchName"] = branchName
	ctx.Data["TreeName"] = treeName
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["FileContent"] = content
	ctx.Data["CommitSummary"] = form.CommitSummary
	ctx.Data["CommitMessage"] = form.CommitMessage
	ctx.Data["CommitChoice"] = commitChoice
	ctx.Data["NewBranchName"] = branchName
	ctx.Data["CommitDirectlyToThisBranch"] = ctx.Tr("repo.commit_directly_to_this_branch", "<strong class=\"branch-name\">"+oldBranchName+"</strong>")
	ctx.Data["CreateNewBranch"] = ctx.Tr("repo.create_new_branch", "<strong>"+ctx.Tr("repo.new_branch")+"</strong>")
	ctx.Data["LastCommit"] = ctx.Repo.Commit.ID
	ctx.Data["MdFileExtensions"] = strings.Join(setting.Markdown.MdFileExtensions, ",")
	ctx.Data["LineWrapExtensions"] = strings.Join(setting.Editor.LineWrapExtensions, ",")
	ctx.Data["PreviewTabApis"] = strings.Join(setting.Editor.PreviewTabApis, ",")
	ctx.Data["PreviewDiffUrl"] = ctx.Repo.RepoLink + "/preview/" + branchName + "/" + treeName

	if ctx.HasError() {
		ctx.HTML(200, EDIT)
		return
	}

	if len(treeName) == 0 {
		ctx.Data["Err_Filename"] = true
		ctx.RenderWithErr(ctx.Tr("repo.filename_cannot_be_empty"), EDIT, &form)
		log.Error(4, "%s: %s", "EditFile", "Filename can't be empty")
		return
	}

	if oldBranchName != branchName {
		if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
			ctx.Data["Err_Branchname"] = true
			ctx.RenderWithErr(ctx.Tr("repo.branch_already_exists"), EDIT, &form)
			log.Error(4, "%s: %s - %s", "BranchName", branchName, "Branch already exists")
			return
		}

	}

	treepath := ""
	for index, part := range treeNames {
		treepath = path.Join(treepath, part)
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treepath)
		if err != nil {
			// Means there is no item with that name, so we're good
			break
		}
		if index != len(treeNames)-1 {
			if !entry.IsDir() {
				ctx.Data["Err_Filename"] = true
				ctx.RenderWithErr(ctx.Tr("repo.directory_is_a_file"), EDIT, &form)
				log.Error(4, "%s: %s - %s", "EditFile", treeName, "Directory given is a file")
				return
			}
		} else {
			if entry.IsDir() {
				ctx.Data["Err_Filename"] = true
				ctx.RenderWithErr(ctx.Tr("repo.filename_is_a_directory"), EDIT, &form)
				log.Error(4, "%s: %s - %s", "EditFile", treeName, "Filename given is a dirctory")
				return
			}
		}
	}

	if !isNewFile {
		_, err := ctx.Repo.Commit.GetTreeEntryByPath(oldTreeName)
		if err != nil && git.IsErrNotExist(err) {
			ctx.Data["Err_Filename"] = true
			ctx.RenderWithErr(ctx.Tr("repo.file_editing_no_longer_exists"), EDIT, &form)
			log.Error(4, "%s: %s / %s - %s", "EditFile", branchName, oldTreeName, "File doesn't exist for editing")
			return
		}
		if lastCommit != ctx.Repo.CommitID {
			if files, err := ctx.Repo.Commit.GetFilesChangedSinceCommit(lastCommit); err == nil {
				for _, file := range files {
					if file == treeName {
						name := ctx.Repo.Commit.Author.Name
						if u, err := models.GetUserByEmail(ctx.Repo.Commit.Author.Email); err == nil {
							name = `<a href="` + setting.AppSubUrl + "/" + u.Name + `" target="_blank">` + u.Name + `</a>`
						}
						message := ctx.Tr("repo.user_has_committed_since_you_started_editing", name) +
							` <a href="` + ctx.Repo.RepoLink + "/commit/" + ctx.Repo.CommitID + `" target="_blank">` + ctx.Tr("repo.see_what_changed") + `</a>` +
							" " + ctx.Tr("repo.pressing_commit_again_will_overwrite_those_changes", "<em>"+ctx.Tr("repo.commit_changes")+"</em>")
						log.Error(4, "%s: %s / %s - %s", "EditFile", branchName, oldTreeName, "File updated by another user")
						ctx.RenderWithErr(message, EDIT, &form)
						return
					}
				}
			}
		}
	}
	if oldTreeName != treeName {
		// We have a new filename (rename or completely new file) so we need to make sure it doesn't already exist, can't clobber
		_, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)
		if err == nil {
			ctx.Data["Err_Filename"] = true
			ctx.RenderWithErr(ctx.Tr("repo.file_already_exists"), EDIT, &form)
			log.Error(4, "%s: %s - %s", "NewFile", treeName, "File already exists, can't create new")
			return
		}
	}

	message := ""
	if form.CommitSummary != "" {
		message = strings.Trim(form.CommitSummary, " ")
	} else {
		if isNewFile {
			message = ctx.Tr("repo.add") + " '" + treeName + "'"
		} else {
			message = ctx.Tr("repo.update") + " '" + treeName + "'"
		}
	}
	if strings.Trim(form.CommitMessage, " ") != "" {
		message += "\n\n" + strings.Trim(form.CommitMessage, " ")
	}

	if err := ctx.Repo.Repository.UpdateRepoFile(ctx.User, oldBranchName, branchName, oldTreeName, treeName, content, message, isNewFile); err != nil {
		ctx.Data["Err_Filename"] = true
		ctx.RenderWithErr(ctx.Tr("repo.unable_to_update_file"), EDIT, &form)
		log.Error(4, "%s: %v", "EditFile", err)
		return
	}

	if branch, err := ctx.Repo.Repository.GetBranch(branchName); err != nil {
		log.Error(4, "repo.Repository.GetBranch(%s): %v", branchName, err)
	} else if commit, err := branch.GetCommit(); err != nil {
		log.Error(4, "branch.GetCommit(): %v", err)
	} else {
		pc := &models.PushCommits{
			Len: 1,
			Commits: []*models.PushCommit{&models.PushCommit{
				commit.ID.String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name,
			}},
		}
		oldCommitID := ctx.Repo.CommitID
		newCommitID := commit.ID.String()
		if branchName != oldBranchName {
			oldCommitID = "0000000000000000000000000000000000000000" // New Branch so we use all 0s
		}
		if err := models.CommitRepoAction(ctx.User.ID, ctx.Repo.Owner.ID, ctx.User.LowerName, ctx.Repo.Owner.Email,
			ctx.Repo.Repository.ID, ctx.Repo.Owner.LowerName, ctx.Repo.Repository.Name, "refs/heads/"+branchName, pc,
			oldCommitID, newCommitID); err != nil {
			log.Error(4, "models.CommitRepoAction(branch = %s): %v", branchName, err)
		}
		models.HookQueue.Add(ctx.Repo.Repository.ID)
	}

	// Leaving this off until forked repos that get a branch can compare with forks master and not upstream
	//if oldBranchName != branchName {
	//	ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/compare/" + oldBranchName + "..." + branchName))
	//} else {
	ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName))
	//}
}

func DiffPreviewPost(ctx *context.Context, form auth.EditPreviewDiffForm) {
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	branchName := ctx.Repo.BranchName
	treeName := ctx.Repo.TreeName
	content := form.Content

	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treeName)
	if (err != nil && git.IsErrNotExist(err)) || entry.IsDir() {
		ctx.Data["FileContent"] = content
		ctx.HTML(200, DIFF_PREVIEW_NEW)
		return
	}

	diff, err := ctx.Repo.Repository.GetPreviewDiff(models.RepoPath(userName, repoName), branchName, treeName, content, setting.Git.MaxGitDiffLines, setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		ctx.Error(404, err.Error())
		log.Error(4, "%s: %v", "GetPreviewDiff", err)
		return
	}

	if diff.NumFiles() == 0 {
		ctx.Error(200, ctx.Tr("repo.no_changes_to_show"))
		return
	}

	ctx.Data["IsSplitStyle"] = ctx.Query("style") == "split"
	ctx.Data["File"] = diff.Files[0]

	ctx.HTML(200, DIFF_PREVIEW)
}

func EscapeUrl(str string) string {
	return strings.NewReplacer("?", "%3F", "%", "%25", "#", "%23", " ", "%20", "^", "%5E", "\\", "%5C", "{", "%7B", "}", "%7D", "|", "%7C").Replace(str)
}
