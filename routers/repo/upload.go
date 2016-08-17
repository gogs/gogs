// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	git "github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const (
	UPLOAD base.TplName = "repo/upload"
)

func renderUploadSettings(ctx *context.Context) {
	ctx.Data["RequireDropzone"] = true
	ctx.Data["IsUploadEnabled"] = setting.Repository.Upload.Enabled
	ctx.Data["UploadAllowedTypes"] = strings.Join(setting.Repository.Upload.AllowedTypes, ",")
	ctx.Data["UploadMaxSize"] = setting.Repository.Upload.FileMaxSize
	ctx.Data["UploadMaxFiles"] = setting.Repository.Upload.MaxFiles
}

func UploadFile(ctx *context.Context) {
	ctx.Data["PageIsUpload"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	branchName := ctx.Repo.BranchName
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	treeName := ctx.Repo.TreeName

	treeNames := []string{""}
	if len(treeName) > 0 {
		treeNames = strings.Split(treeName, "/")
	}

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
	renderUploadSettings(ctx)

	ctx.HTML(200, UPLOAD)
}

func UploadFilePost(ctx *context.Context, form auth.UploadRepoFileForm) {
	ctx.Data["PageIsUpload"] = true
	renderUploadSettings(ctx)

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	oldBranchName := ctx.Repo.BranchName
	branchName := oldBranchName
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	commitChoice := form.CommitChoice
	files := form.Files

	if commitChoice == "commit-to-new-branch" {
		branchName = form.NewBranchName
	}

	treeName := form.TreeName
	treeName = strings.Trim(treeName, " ")
	treeName = strings.Trim(treeName, "/")

	treeNames := []string{""}
	if len(treeName) > 0 {
		treeNames = strings.Split(treeName, "/")
	}

	ctx.Data["UserName"] = userName
	ctx.Data["RepoName"] = repoName
	ctx.Data["BranchName"] = branchName
	ctx.Data["TreeName"] = treeName
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.Data["CommitSummary"] = form.CommitSummary
	ctx.Data["CommitMessage"] = form.CommitMessage
	ctx.Data["CommitChoice"] = commitChoice
	ctx.Data["NewBranchName"] = branchName
	ctx.Data["CommitDirectlyToThisBranch"] = ctx.Tr("repo.commit_directly_to_this_branch", "<strong class=\"branch-name\">"+oldBranchName+"</strong>")
	ctx.Data["CreateNewBranch"] = ctx.Tr("repo.create_new_branch", "<strong>"+ctx.Tr("repo.new_branch")+"</strong>")

	if ctx.HasError() {
		ctx.HTML(200, UPLOAD)
		return
	}

	if oldBranchName != branchName {
		if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
			ctx.Data["Err_Branchname"] = true
			ctx.RenderWithErr(ctx.Tr("repo.branch_already_exists"), UPLOAD, &form)
			log.Error(4, "%s: %s - %s", "BranchName", branchName, "Branch already exists")
			return
		}

	}

	treepath := ""
	for _, part := range treeNames {
		treepath = path.Join(treepath, part)
		entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treepath)
		if err != nil {
			// Means there is no item with that name, so we're good
			break
		}
		if !entry.IsDir() {
			ctx.Data["Err_Filename"] = true
			ctx.RenderWithErr(ctx.Tr("repo.directory_is_a_file"), UPLOAD, &form)
			log.Error(4, "%s: %s - %s", "UploadFile", treeName, "Directory given is a file")
			return
		}
	}

	message := ""
	if form.CommitSummary != "" {
		message = strings.Trim(form.CommitSummary, " ")
	} else {
		message = ctx.Tr("repo.add_files_to_dir", "'"+treeName+"'")
	}
	if strings.Trim(form.CommitMessage, " ") != "" {
		message += "\n\n" + strings.Trim(form.CommitMessage, " ")
	}

	if err := ctx.Repo.Repository.UploadRepoFiles(ctx.User, oldBranchName, branchName, treeName, message, files); err != nil {
		ctx.Data["Err_Directory"] = true
		ctx.RenderWithErr(ctx.Tr("repo.unable_to_upload_files"), UPLOAD, &form)
		log.Error(4, "%s: %v", "UploadFile", err)
		return
	}

	// Was successful, so now need to call models.CommitRepoAction() with the new commitID for webhooks and watchers
	if branch, err := ctx.Repo.Repository.GetBranch(branchName); err != nil {
		log.Error(4, "repo.Repository.GetBranch(%s): %v", branchName, err)
	} else if commit, err := branch.GetCommit(); err != nil {
		log.Error(4, "branch.GetCommit(): %v", err)
	} else {
		pc := &models.PushCommits{
			Len:     1,
			Commits: []*models.PushCommit{models.CommitToPushCommit(commit)},
		}
		oldCommitID := ctx.Repo.CommitID
		newCommitID := commit.ID.String()
		if branchName != oldBranchName {
			oldCommitID = "0000000000000000000000000000000000000000" // New Branch so we use all 0s
		}
		if err := models.CommitRepoAction(models.CommitRepoActionOptions{
			PusherName:  ctx.User.Name,
			RepoOwnerID: ctx.Repo.Owner.ID,
			RepoName:    ctx.Repo.Owner.Name,
			RefFullName: git.BRANCH_PREFIX + branchName,
			OldCommitID: oldCommitID,
			NewCommitID: newCommitID,
			Commits:     pc,
		}); err != nil {
			log.Error(4, "models.CommitRepoAction(branch = %s): %v", branchName, err)
		}
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName)
}

func UploadFileToServer(ctx *context.Context) {
	if !setting.Repository.Upload.Enabled {
		ctx.Error(404, "upload is not enabled")
		return
	}

	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		ctx.Error(500, fmt.Sprintf("FormFile: %v", err))
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
			ctx.Error(400, ErrFileTypeForbidden.Error())
			return
		}
	}

	up, err := models.NewUpload(header.Filename, buf, file, ctx.User.ID, ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Error(500, fmt.Sprintf("NewUpload: %v", err))
		return
	}

	log.Trace("New file uploaded: %s", up.UUID)
	ctx.JSON(200, map[string]string{
		"uuid": up.UUID,
	})
}

func RemoveUploadFileFromServer(ctx *context.Context, form auth.RemoveUploadFileForm) {
	if !setting.Repository.Upload.Enabled {
		ctx.Error(404, "upload is not enabled")
		return
	}

	if len(form.File) == 0 {
		ctx.Error(404, "invalid params")
		return
	}

	uuid := form.File

	if err := models.RemoveUpload(uuid, ctx.User.ID, ctx.Repo.Repository.ID); err != nil {
		ctx.Error(500, fmt.Sprintf("RemoveUpload: %v", err))
		return
	}

	log.Trace("Upload file removed: %s", uuid)
	ctx.JSON(200, map[string]string{
		"uuid": uuid,
	})
}
