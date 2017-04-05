// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"bytes"
	"fmt"
	gotemplate "html/template"
	"io/ioutil"
	"path"
	"strings"

	"github.com/Unknwon/paginater"
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/markup"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/template"
	"github.com/gogits/gogs/pkg/template/highlight"
	"github.com/gogits/gogs/pkg/tool"
)

const (
	BARE     = "repo/bare"
	HOME     = "repo/home"
	WATCHERS = "repo/watchers"
	FORKS    = "repo/forks"
)

func renderDirectory(ctx *context.Context, treeLink string) {
	tree, err := ctx.Repo.Commit.SubTree(ctx.Repo.TreePath)
	if err != nil {
		ctx.NotFoundOrServerError("Repo.Commit.SubTree", git.IsErrNotExist, err)
		return
	}

	entries, err := tree.ListEntries()
	if err != nil {
		ctx.Handle(500, "ListEntries", err)
		return
	}
	entries.Sort()

	ctx.Data["Files"], err = entries.GetCommitsInfoWithCustomConcurrency(ctx.Repo.Commit, ctx.Repo.TreePath, setting.Repository.CommitsFetchConcurrency)
	if err != nil {
		ctx.Handle(500, "GetCommitsInfo", err)
		return
	}

	var readmeFile *git.Blob
	for _, entry := range entries {
		if entry.IsDir() || !markup.IsReadmeFile(entry.Name()) {
			continue
		}

		// TODO: collect all possible README files and show with priority.
		readmeFile = entry.Blob()
		break
	}

	if readmeFile != nil {
		ctx.Data["RawFileLink"] = ""
		ctx.Data["ReadmeInList"] = true
		ctx.Data["ReadmeExist"] = true

		dataRc, err := readmeFile.Data()
		if err != nil {
			ctx.Handle(500, "Data", err)
			return
		}

		buf := make([]byte, 1024)
		n, _ := dataRc.Read(buf)
		buf = buf[:n]

		isTextFile := tool.IsTextFile(buf)
		ctx.Data["IsTextFile"] = isTextFile
		ctx.Data["FileName"] = readmeFile.Name()
		if isTextFile {
			d, _ := ioutil.ReadAll(dataRc)
			buf = append(buf, d...)
			switch {
			case markup.IsMarkdownFile(readmeFile.Name()):
				ctx.Data["IsMarkdown"] = true
				buf = markup.Markdown(buf, treeLink, ctx.Repo.Repository.ComposeMetas())
			default:
				buf = bytes.Replace(buf, []byte("\n"), []byte(`<br>`), -1)
			}
			ctx.Data["FileContent"] = string(buf)
		}
	}

	// Show latest commit info of repository in table header,
	// or of directory if not in root directory.
	latestCommit := ctx.Repo.Commit
	if len(ctx.Repo.TreePath) > 0 {
		latestCommit, err = ctx.Repo.Commit.GetCommitByPath(ctx.Repo.TreePath)
		if err != nil {
			ctx.Handle(500, "GetCommitByPath", err)
			return
		}
	}
	ctx.Data["LatestCommit"] = latestCommit
	ctx.Data["LatestCommitUser"] = models.ValidateCommitWithEmail(latestCommit)

	if ctx.Repo.CanEnableEditor() {
		ctx.Data["CanAddFile"] = true
		ctx.Data["CanUploadFile"] = setting.Repository.Upload.Enabled
	}
}

func renderFile(ctx *context.Context, entry *git.TreeEntry, treeLink, rawLink string) {
	ctx.Data["IsViewFile"] = true

	blob := entry.Blob()
	dataRc, err := blob.Data()
	if err != nil {
		ctx.Handle(500, "Data", err)
		return
	}

	ctx.Data["FileSize"] = blob.Size()
	ctx.Data["FileName"] = blob.Name()
	ctx.Data["HighlightClass"] = highlight.FileNameToHighlightClass(blob.Name())
	ctx.Data["RawFileLink"] = rawLink + "/" + ctx.Repo.TreePath

	buf := make([]byte, 1024)
	n, _ := dataRc.Read(buf)
	buf = buf[:n]

	isTextFile := tool.IsTextFile(buf)
	ctx.Data["IsTextFile"] = isTextFile

	// Assume file is not editable first.
	if !isTextFile {
		ctx.Data["EditFileTooltip"] = ctx.Tr("repo.editor.cannot_edit_non_text_files")
	}

	canEnableEditor := ctx.Repo.CanEnableEditor()
	switch {
	case isTextFile:
		if blob.Size() >= setting.UI.MaxDisplayFileSize {
			ctx.Data["IsFileTooLarge"] = true
			break
		}

		d, _ := ioutil.ReadAll(dataRc)
		buf = append(buf, d...)

		isMarkdown := markup.IsMarkdownFile(blob.Name())
		ctx.Data["IsMarkdown"] = isMarkdown
		ctx.Data["ReadmeExist"] = isMarkdown && markup.IsReadmeFile(blob.Name())

		ctx.Data["IsIPythonNotebook"] = strings.HasSuffix(blob.Name(), ".ipynb")

		if isMarkdown {
			ctx.Data["FileContent"] = string(markup.Markdown(buf, path.Dir(treeLink), ctx.Repo.Repository.ComposeMetas()))
		} else {
			// Building code view blocks with line number on server side.
			var fileContent string
			if err, content := template.ToUTF8WithErr(buf); err != nil {
				if err != nil {
					log.Error(4, "ToUTF8WithErr: %s", err)
				}
				fileContent = string(buf)
			} else {
				fileContent = content
			}

			var output bytes.Buffer
			lines := strings.Split(fileContent, "\n")
			for index, line := range lines {
				output.WriteString(fmt.Sprintf(`<li class="L%d" rel="L%d">%s</li>`, index+1, index+1, gotemplate.HTMLEscapeString(line)) + "\n")
			}
			ctx.Data["FileContent"] = gotemplate.HTML(output.String())

			output.Reset()
			for i := 0; i < len(lines); i++ {
				output.WriteString(fmt.Sprintf(`<span id="L%d">%d</span>`, i+1, i+1))
			}
			ctx.Data["LineNums"] = gotemplate.HTML(output.String())
		}

		if canEnableEditor {
			ctx.Data["CanEditFile"] = true
			ctx.Data["EditFileTooltip"] = ctx.Tr("repo.editor.edit_this_file")
		} else if !ctx.Repo.IsViewBranch {
			ctx.Data["EditFileTooltip"] = ctx.Tr("repo.editor.must_be_on_a_branch")
		} else if !ctx.Repo.IsWriter() {
			ctx.Data["EditFileTooltip"] = ctx.Tr("repo.editor.fork_before_edit")
		}

	case tool.IsPDFFile(buf):
		ctx.Data["IsPDFFile"] = true
	case tool.IsVideoFile(buf):
		ctx.Data["IsVideoFile"] = true
	case tool.IsImageFile(buf):
		ctx.Data["IsImageFile"] = true
	}

	if canEnableEditor {
		ctx.Data["CanDeleteFile"] = true
		ctx.Data["DeleteFileTooltip"] = ctx.Tr("repo.editor.delete_this_file")
	} else if !ctx.Repo.IsViewBranch {
		ctx.Data["DeleteFileTooltip"] = ctx.Tr("repo.editor.must_be_on_a_branch")
	} else if !ctx.Repo.IsWriter() {
		ctx.Data["DeleteFileTooltip"] = ctx.Tr("repo.editor.must_have_write_access")
	}
}

func setEditorconfigIfExists(ctx *context.Context) {
	ec, err := ctx.Repo.GetEditorconfig()
	if err != nil && !git.IsErrNotExist(err) {
		log.Trace("setEditorconfigIfExists.GetEditorconfig [%d]: %v", ctx.Repo.Repository.ID, err)
		return
	}
	ctx.Data["Editorconfig"] = ec
}

func Home(ctx *context.Context) {
	ctx.Data["PageIsViewFiles"] = true

	if ctx.Repo.Repository.IsBare {
		ctx.HTML(200, BARE)
		return
	}

	title := ctx.Repo.Repository.Owner.Name + "/" + ctx.Repo.Repository.Name
	if len(ctx.Repo.Repository.Description) > 0 {
		title += ": " + ctx.Repo.Repository.Description
	}
	ctx.Data["Title"] = title
	if ctx.Repo.BranchName != ctx.Repo.Repository.DefaultBranch {
		ctx.Data["Title"] = title + " @ " + ctx.Repo.BranchName
	}
	ctx.Data["RequireHighlightJS"] = true

	branchLink := ctx.Repo.RepoLink + "/src/" + ctx.Repo.BranchName
	treeLink := branchLink
	rawLink := ctx.Repo.RepoLink + "/raw/" + ctx.Repo.BranchName

	isRootDir := false
	if len(ctx.Repo.TreePath) > 0 {
		treeLink += "/" + ctx.Repo.TreePath
	} else {
		isRootDir = true

		// Only show Git stats panel when view root directory
		var err error
		ctx.Repo.CommitsCount, err = ctx.Repo.Commit.CommitsCount()
		if err != nil {
			ctx.Handle(500, "CommitsCount", err)
			return
		}
		ctx.Data["CommitsCount"] = ctx.Repo.CommitsCount
	}
	ctx.Data["PageIsRepoHome"] = isRootDir

	// Get current entry user currently looking at.
	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(ctx.Repo.TreePath)
	if err != nil {
		ctx.NotFoundOrServerError("Repo.Commit.GetTreeEntryByPath", git.IsErrNotExist, err)
		return
	}

	if entry.IsDir() {
		renderDirectory(ctx, treeLink)
	} else {
		renderFile(ctx, entry, treeLink, rawLink)
	}
	if ctx.Written() {
		return
	}

	setEditorconfigIfExists(ctx)
	if ctx.Written() {
		return
	}

	var treeNames []string
	paths := make([]string, 0, 5)
	if len(ctx.Repo.TreePath) > 0 {
		treeNames = strings.Split(ctx.Repo.TreePath, "/")
		for i := range treeNames {
			paths = append(paths, strings.Join(treeNames[:i+1], "/"))
		}

		ctx.Data["HasParentPath"] = true
		if len(paths)-2 >= 0 {
			ctx.Data["ParentPath"] = "/" + paths[len(paths)-2]
		}
	}

	ctx.Data["Paths"] = paths
	ctx.Data["TreeLink"] = treeLink
	ctx.Data["TreeNames"] = treeNames
	ctx.Data["BranchLink"] = branchLink
	ctx.HTML(200, HOME)
}

func RenderUserCards(ctx *context.Context, total int, getter func(page int) ([]*models.User, error), tpl string) {
	page := ctx.QueryInt("page")
	if page <= 0 {
		page = 1
	}
	pager := paginater.New(total, models.ItemsPerPage, page, 5)
	ctx.Data["Page"] = pager

	items, err := getter(pager.Current())
	if err != nil {
		ctx.Handle(500, "getter", err)
		return
	}
	ctx.Data["Cards"] = items

	ctx.HTML(200, tpl)
}

func Watchers(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.watchers")
	ctx.Data["CardsTitle"] = ctx.Tr("repo.watchers")
	ctx.Data["PageIsWatchers"] = true
	RenderUserCards(ctx, ctx.Repo.Repository.NumWatches, ctx.Repo.Repository.GetWatchers, WATCHERS)
}

func Stars(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.stargazers")
	ctx.Data["CardsTitle"] = ctx.Tr("repo.stargazers")
	ctx.Data["PageIsStargazers"] = true
	RenderUserCards(ctx, ctx.Repo.Repository.NumStars, ctx.Repo.Repository.GetStargazers, WATCHERS)
}

func Forks(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repos.forks")

	forks, err := ctx.Repo.Repository.GetForks()
	if err != nil {
		ctx.Handle(500, "GetForks", err)
		return
	}

	for _, fork := range forks {
		if err = fork.GetOwner(); err != nil {
			ctx.Handle(500, "GetOwner", err)
			return
		}
	}
	ctx.Data["Forks"] = forks

	ctx.HTML(200, FORKS)
}
