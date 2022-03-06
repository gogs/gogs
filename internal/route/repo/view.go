// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"bytes"
	"fmt"
	gotemplate "html/template"
	"path"
	"strings"
	"time"

	"github.com/gogs/git-module"
	"github.com/pkg/errors"
	"github.com/unknwon/paginater"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/template"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/tool"
)

const (
	BARE     = "repo/bare"
	HOME     = "repo/home"
	WATCHERS = "repo/watchers"
	FORKS    = "repo/forks"
)

func renderDirectory(c *context.Context, treeLink string) {
	tree, err := c.Repo.Commit.Subtree(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get subtree")
		return
	}

	entries, err := tree.Entries()
	if err != nil {
		c.Error(err, "list entries")
		return
	}
	entries.Sort()

	c.Data["Files"], err = entries.CommitsInfo(c.Repo.Commit, git.CommitsInfoOptions{
		Path:           c.Repo.TreePath,
		MaxConcurrency: conf.Repository.CommitsFetchConcurrency,
		Timeout:        5 * time.Minute,
	})
	if err != nil {
		c.Error(err, "get commits info")
		return
	}

	var readmeFile *git.Blob
	for _, entry := range entries {
		if entry.IsTree() || !markup.IsReadmeFile(entry.Name()) {
			continue
		}

		// TODO(unknwon): collect all possible README files and show with priority.
		readmeFile = entry.Blob()
		break
	}

	if readmeFile != nil {
		c.Data["RawFileLink"] = ""
		c.Data["ReadmeInList"] = true
		c.Data["ReadmeExist"] = true

		p, err := readmeFile.Bytes()
		if err != nil {
			c.Error(err, "read file")
			return
		}

		isTextFile := tool.IsTextFile(p)
		c.Data["IsTextFile"] = isTextFile
		c.Data["FileName"] = readmeFile.Name()
		if isTextFile {
			switch markup.Detect(readmeFile.Name()) {
			case markup.TypeMarkdown:
				c.Data["IsMarkdown"] = true
				p = markup.Markdown(p, treeLink, c.Repo.Repository.ComposeMetas())
			case markup.TypeOrgMode:
				c.Data["IsMarkdown"] = true
				p = markup.OrgMode(p, treeLink, c.Repo.Repository.ComposeMetas())
			case markup.TypeIPythonNotebook:
				c.Data["IsIPythonNotebook"] = true
				c.Data["RawFileLink"] = c.Repo.RepoLink + "/raw/" + path.Join(c.Repo.BranchName, c.Repo.TreePath, readmeFile.Name())
			default:
				p = bytes.ReplaceAll(p, []byte("\n"), []byte(`<br>`))
			}
			c.Data["FileContent"] = string(p)
		}
	}

	// Show latest commit info of repository in table header,
	// or of directory if not in root directory.
	latestCommit := c.Repo.Commit
	if len(c.Repo.TreePath) > 0 {
		latestCommit, err = c.Repo.Commit.CommitByPath(git.CommitByRevisionOptions{Path: c.Repo.TreePath})
		if err != nil {
			c.Error(err, "get commit by path")
			return
		}
	}
	c.Data["LatestCommit"] = latestCommit
	c.Data["LatestCommitUser"] = db.ValidateCommitWithEmail(latestCommit)

	if c.Repo.CanEnableEditor() {
		c.Data["CanAddFile"] = true
		c.Data["CanUploadFile"] = conf.Repository.Upload.Enabled
	}
}

func renderFile(c *context.Context, entry *git.TreeEntry, treeLink, rawLink string) {
	c.Data["IsViewFile"] = true

	blob := entry.Blob()
	p, err := blob.Bytes()
	if err != nil {
		c.Error(err, "read blob")
		return
	}

	c.Data["FileSize"] = blob.Size()
	c.Data["FileName"] = blob.Name()
	c.Data["HighlightClass"] = highlight.FileNameToHighlightClass(blob.Name())
	c.Data["RawFileLink"] = rawLink + "/" + c.Repo.TreePath

	isTextFile := tool.IsTextFile(p)
	c.Data["IsTextFile"] = isTextFile

	// Assume file is not editable first.
	if !isTextFile {
		c.Data["EditFileTooltip"] = c.Tr("repo.editor.cannot_edit_non_text_files")
	}

	canEnableEditor := c.Repo.CanEnableEditor()
	switch {
	case isTextFile:
		if blob.Size() >= conf.UI.MaxDisplayFileSize {
			c.Data["IsFileTooLarge"] = true
			break
		}

		c.Data["ReadmeExist"] = markup.IsReadmeFile(blob.Name())

		switch markup.Detect(blob.Name()) {
		case markup.TypeMarkdown:
			c.Data["IsMarkdown"] = true
			c.Data["FileContent"] = string(markup.Markdown(p, path.Dir(treeLink), c.Repo.Repository.ComposeMetas()))
		case markup.TypeOrgMode:
			c.Data["IsMarkdown"] = true
			c.Data["FileContent"] = string(markup.OrgMode(p, path.Dir(treeLink), c.Repo.Repository.ComposeMetas()))
		case markup.TypeIPythonNotebook:
			c.Data["IsIPythonNotebook"] = true
		default:
			// Building code view blocks with line number on server side.
			var fileContent string
			if err, content := template.ToUTF8WithErr(p); err != nil {
				if err != nil {
					log.Error("ToUTF8WithErr: %s", err)
				}
				fileContent = string(p)
			} else {
				fileContent = content
			}

			var output bytes.Buffer
			lines := strings.Split(fileContent, "\n")
			// Remove blank line at the end of file
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			for index, line := range lines {
				output.WriteString(fmt.Sprintf(`<li class="L%d" rel="L%d">%s</li>`, index+1, index+1, gotemplate.HTMLEscapeString(strings.TrimRight(line, "\r"))) + "\n")
			}
			c.Data["FileContent"] = gotemplate.HTML(output.String())

			output.Reset()
			for i := 0; i < len(lines); i++ {
				output.WriteString(fmt.Sprintf(`<span id="L%d">%d</span>`, i+1, i+1))
			}
			c.Data["LineNums"] = gotemplate.HTML(output.String())
		}

		if canEnableEditor {
			c.Data["CanEditFile"] = true
			c.Data["EditFileTooltip"] = c.Tr("repo.editor.edit_this_file")
		} else if !c.Repo.IsViewBranch {
			c.Data["EditFileTooltip"] = c.Tr("repo.editor.must_be_on_a_branch")
		} else if !c.Repo.IsWriter() {
			c.Data["EditFileTooltip"] = c.Tr("repo.editor.fork_before_edit")
		}

	case tool.IsPDFFile(p):
		c.Data["IsPDFFile"] = true
	case tool.IsVideoFile(p):
		c.Data["IsVideoFile"] = true
	case tool.IsImageFile(p):
		c.Data["IsImageFile"] = true
	}

	if canEnableEditor {
		c.Data["CanDeleteFile"] = true
		c.Data["DeleteFileTooltip"] = c.Tr("repo.editor.delete_this_file")
	} else if !c.Repo.IsViewBranch {
		c.Data["DeleteFileTooltip"] = c.Tr("repo.editor.must_be_on_a_branch")
	} else if !c.Repo.IsWriter() {
		c.Data["DeleteFileTooltip"] = c.Tr("repo.editor.must_have_write_access")
	}
}

func setEditorconfigIfExists(c *context.Context) {
	ec, err := c.Repo.Editorconfig()
	if err != nil && !gitutil.IsErrRevisionNotExist(errors.Cause(err)) {
		log.Warn("setEditorconfigIfExists.Editorconfig [repo_id: %d]: %v", c.Repo.Repository.ID, err)
		return
	}
	c.Data["Editorconfig"] = ec
}

func Home(c *context.Context) {
	c.Data["PageIsViewFiles"] = true

	if c.Repo.Repository.IsBare {
		c.Success(BARE)
		return
	}

	title := c.Repo.Repository.Owner.Name + "/" + c.Repo.Repository.Name
	if len(c.Repo.Repository.Description) > 0 {
		title += ": " + c.Repo.Repository.Description
	}
	c.Data["Title"] = title
	if c.Repo.BranchName != c.Repo.Repository.DefaultBranch {
		c.Data["Title"] = title + " @ " + c.Repo.BranchName
	}
	c.Data["RequireHighlightJS"] = true

	branchLink := c.Repo.RepoLink + "/src/" + c.Repo.BranchName
	treeLink := branchLink
	rawLink := c.Repo.RepoLink + "/raw/" + c.Repo.BranchName

	isRootDir := false
	if len(c.Repo.TreePath) > 0 {
		treeLink += "/" + c.Repo.TreePath
	} else {
		isRootDir = true

		// Only show Git stats panel when view root directory
		var err error
		c.Repo.CommitsCount, err = c.Repo.Commit.CommitsCount()
		if err != nil {
			c.Error(err, "count commits")
			return
		}
		c.Data["CommitsCount"] = c.Repo.CommitsCount
	}
	c.Data["PageIsRepoHome"] = isRootDir

	// Get current entry user currently looking at.
	entry, err := c.Repo.Commit.TreeEntry(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrError(gitutil.NewError(err), "get tree entry")
		return
	}

	if entry.IsTree() {
		renderDirectory(c, treeLink)
	} else {
		renderFile(c, entry, treeLink, rawLink)
	}
	if c.Written() {
		return
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	var treeNames []string
	paths := make([]string, 0, 5)
	if len(c.Repo.TreePath) > 0 {
		treeNames = strings.Split(c.Repo.TreePath, "/")
		for i := range treeNames {
			paths = append(paths, strings.Join(treeNames[:i+1], "/"))
		}

		c.Data["HasParentPath"] = true
		if len(paths)-2 >= 0 {
			c.Data["ParentPath"] = "/" + paths[len(paths)-2]
		}
	}

	c.Data["Paths"] = paths
	c.Data["TreeLink"] = treeLink
	c.Data["TreeNames"] = treeNames
	c.Data["BranchLink"] = branchLink
	c.Success(HOME)
}

func RenderUserCards(c *context.Context, total int, getter func(page int) ([]*db.User, error), tpl string) {
	page := c.QueryInt("page")
	if page <= 0 {
		page = 1
	}
	pager := paginater.New(total, db.ItemsPerPage, page, 5)
	c.Data["Page"] = pager

	items, err := getter(pager.Current())
	if err != nil {
		c.Error(err, "getter")
		return
	}
	c.Data["Cards"] = items

	c.Success(tpl)
}

func Watchers(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.watchers")
	c.Data["CardsTitle"] = c.Tr("repo.watchers")
	c.Data["PageIsWatchers"] = true
	RenderUserCards(c, c.Repo.Repository.NumWatches, c.Repo.Repository.GetWatchers, WATCHERS)
}

func Stars(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.stargazers")
	c.Data["CardsTitle"] = c.Tr("repo.stargazers")
	c.Data["PageIsStargazers"] = true
	RenderUserCards(c, c.Repo.Repository.NumStars, c.Repo.Repository.GetStargazers, WATCHERS)
}

func Forks(c *context.Context) {
	c.Data["Title"] = c.Tr("repos.forks")

	forks, err := c.Repo.Repository.GetForks()
	if err != nil {
		c.Error(err, "get forks")
		return
	}

	for _, fork := range forks {
		if err = fork.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}
	}
	c.Data["Forks"] = forks

	c.Success(FORKS)
}
