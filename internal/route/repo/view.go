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

func renderDirectory(c *context.Context, treeLink string, entry *git.TreeEntry, sourceEntry *git.TreeEntry, sourceTreePath string) {
	treePath := c.Repo.TreePath
	if sourceEntry != nil {
		treePath = sourceTreePath
	}
	c.Data["TreePath"] = treePath

	tree, err := c.Repo.Commit.Subtree(treePath)
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
		Path:           treePath,
		MaxConcurrency: conf.Repository.CommitsFetchConcurrency,
		Timeout:        5 * time.Minute,
	})
	if err != nil {
		c.Error(err, "get commits info")
		return
	}

	var readmeFile *git.Blob
	var readmeFileName string
	for _, entry := range entries {
		if entry.IsTree() || !markup.IsReadmeFile(entry.Name()) {
			continue
		}

		// TODO(unknwon): collect all possible README files and show with priority.
		readmeFile = entry.Blob()
		readmeFileName = readmeFile.Name()
		if ok, sourceEntry, sourcePath, err := isSymlink(entry, c); ok {
			c.Data["IsSymlinkSourceExists"] = true
			if err != nil {
				c.Data["SourceTreePath"] = err.Error()
				c.Data["IsSymlinkSourceExists"] = false
			} else if sourceEntry != nil {
				c.Data["SourceTreePath"] = sourcePath
				readmeFile = sourceEntry.Blob()
			}
			c.Data["IsSymlink"] = true
		}
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
		c.Data["FileName"] = readmeFileName
		if isTextFile {
			switch markup.Detect(readmeFile.Name()) {
			case markup.MARKDOWN:
				c.Data["IsMarkdown"] = true
				p = markup.Markdown(p, treeLink, c.Repo.Repository.ComposeMetas())
			case markup.ORG_MODE:
				c.Data["IsMarkdown"] = true
				p = markup.OrgMode(p, treeLink, c.Repo.Repository.ComposeMetas())
			case markup.IPYTHON_NOTEBOOK:
				c.Data["IsIPythonNotebook"] = true
				c.Data["RawFileLink"] = c.Repo.RepoLink + "/raw/" + path.Join(c.Repo.BranchName, treePath, readmeFile.Name())
			default:
				p = bytes.Replace(p, []byte("\n"), []byte(`<br>`), -1)
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

func renderFile(c *context.Context, entry *git.TreeEntry, sourceEntry *git.TreeEntry, sourceTreePath string, treeLink, rawLink string) {
	c.Data["IsViewFile"] = true
	c.Data["IsShowRawLink"] = true
	c.Data["RawFileLink"] = rawLink + "/" + c.Repo.TreePath
	c.Data["EditableFileTreePath"] = c.Repo.TreePath

	var blob *git.Blob
	if entry.IsSymlink() {
		c.Data["SourceTreePath"] = sourceTreePath
		if sourceEntry != nil {
			blob = sourceEntry.Blob()
			c.Data["SourceFileSize"] = sourceEntry.Blob().Size()
			c.Data["RawFileLink"] = rawLink + "/" + sourceTreePath
			c.Data["EditableFileTreePath"] = sourceTreePath
		} else {
			c.Data["SourceFileSize"] = int64(0)
			c.Data["IsShowRawLink"] = false
		}
	}

	if blob == nil {
		blob = entry.Blob()
	}

	p, err := blob.Bytes()
	if err != nil {
		c.Error(err, "read blob")
		return
	}

	c.Data["FileSize"] = entry.Blob().Size()
	c.Data["FileName"] = entry.Blob().Name()
	c.Data["HighlightClass"] = highlight.FileNameToHighlightClass(blob.Name())

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
		case markup.MARKDOWN:
			c.Data["IsMarkdown"] = true
			c.Data["FileContent"] = string(markup.Markdown(p, path.Dir(treeLink), c.Repo.Repository.ComposeMetas()))
		case markup.ORG_MODE:
			c.Data["IsMarkdown"] = true
			c.Data["FileContent"] = string(markup.OrgMode(p, path.Dir(treeLink), c.Repo.Repository.ComposeMetas()))
		case markup.IPYTHON_NOTEBOOK:
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
			if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
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
			if !entry.IsSymlink() || sourceEntry != nil {
				c.Data["CanEditFile"] = true
				c.Data["EditFileTooltip"] = c.Tr("repo.editor.edit_this_file")
			}
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

	if ok, sourceEntry, sourceTreePath, err := isSymlink(entry, c); ok {
		c.Data["IsSymlink"] = true
		c.Data["IsSymlinkSourceExists"] = true
		if err != nil {
			renderFile(c, entry, nil, err.Error(), treeLink, rawLink)
			c.Data["IsSymlinkSourceExists"] = false
		} else if sourceEntry != nil {
			treeLink = branchLink + "/" + sourceTreePath
			if sourceEntry.IsTree() {
				renderDirectory(c, treeLink, entry, sourceEntry, sourceTreePath)
			} else {
				renderFile(c, entry, sourceEntry, sourceTreePath, treeLink, rawLink)
			}
		}
	} else {
		if entry.IsTree() {
			renderDirectory(c, treeLink, entry, nil, "")
		} else {
			renderFile(c, entry, nil, "", treeLink, rawLink)
		}
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

// isSymlink returns destination of given entry if it's a symlink.
func isSymlink(entry *git.TreeEntry, c *context.Context) (bool, *git.TreeEntry, string, error) {
	if !entry.IsSymlink() {
		return false, nil, "", nil
	}
	p, err := entry.Blob().Bytes()
	if err != nil {
		return true, nil, "", err
	}
	sourceTreePath := string(p)
	sourceEntry, err := c.Repo.Commit.TreeEntry("/" + sourceTreePath)
	if err != nil {
		return true, sourceEntry, sourceTreePath, err
	}
	if sourceEntry.IsSymlink() {
		return isSymlink(sourceEntry, c)
	}
	return true, sourceEntry, sourceTreePath, nil
}
