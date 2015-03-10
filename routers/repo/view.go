// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"bytes"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	HOME base.TplName = "repo/home"
)

func Home(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Repo.Repository.Name

	branchName := ctx.Repo.BranchName
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	repoLink := ctx.Repo.RepoLink
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	rawLink := ctx.Repo.RepoLink + "/raw/" + branchName

	// Get tree path
	treename := ctx.Repo.TreeName

	if len(treename) > 0 && treename[len(treename)-1] == '/' {
		ctx.Redirect(repoLink + "/src/" + branchName + "/" + treename[:len(treename)-1])
		return
	}

	ctx.Data["IsRepoToolbarSource"] = true

	isViewBranch := ctx.Repo.IsBranch
	ctx.Data["IsViewBranch"] = isViewBranch

	treePath := treename
	if len(treePath) != 0 {
		treePath = treePath + "/"
	}

	entry, err := ctx.Repo.Commit.GetTreeEntryByPath(treename)
	if err != nil && err != git.ErrNotExist {
		ctx.Handle(404, "GetTreeEntryByPath", err)
		return
	}

	if len(treename) != 0 && entry == nil {
		ctx.Handle(404, "repo.Home", nil)
		return
	}

	if entry != nil && !entry.IsDir() {
		blob := entry.Blob()

		if dataRc, err := blob.Data(); err != nil {
			ctx.Handle(404, "blob.Data", err)
			return
		} else {
			ctx.Data["FileSize"] = blob.Size()
			ctx.Data["IsFile"] = true
			ctx.Data["FileName"] = blob.Name()
			ext := path.Ext(blob.Name())
			if len(ext) > 0 {
				ext = ext[1:]
			}
			ctx.Data["FileExt"] = ext
			ctx.Data["FileLink"] = rawLink + "/" + treename

			buf := make([]byte, 1024)
			n, _ := dataRc.Read(buf)
			if n > 0 {
				buf = buf[:n]
			}

			_, isTextFile := base.IsTextFile(buf)
			_, isImageFile := base.IsImageFile(buf)
			ctx.Data["IsFileText"] = isTextFile

			switch {
			case isImageFile:
				ctx.Data["IsImageFile"] = true
			case isTextFile:
				d, _ := ioutil.ReadAll(dataRc)
				buf = append(buf, d...)
				readmeExist := base.IsMarkdownFile(blob.Name()) || base.IsReadmeFile(blob.Name())
				ctx.Data["ReadmeExist"] = readmeExist
				if readmeExist {
					ctx.Data["FileContent"] = string(base.RenderMarkdown(buf, branchLink))
				} else {
					if err, content := base.ToUtf8WithErr(buf); err != nil {
						if err != nil {
							log.Error(4, "Convert content encoding: %s", err)
						}
						ctx.Data["FileContent"] = string(buf)
					} else {
						ctx.Data["FileContent"] = content
					}
				}
			}
		}
	} else {
		// Directory and file list.
		tree, err := ctx.Repo.Commit.SubTree(treename)
		if err != nil {
			ctx.Handle(404, "SubTree", err)
			return
		}

		entries, err := tree.ListEntries(treename)
		if err != nil {
			ctx.Handle(500, "ListEntries", err)
			return
		}
		entries.Sort()

		files := make([][]interface{}, 0, len(entries))
		for _, te := range entries {
			if te.Type != git.COMMIT {
				c, err := ctx.Repo.Commit.GetCommitOfRelPath(filepath.Join(treePath, te.Name()))
				if err != nil {
					ctx.Handle(500, "GetCommitOfRelPath", err)
					return
				}
				files = append(files, []interface{}{te, c})
			} else {
				sm, err := ctx.Repo.Commit.GetSubModule(path.Join(treename, te.Name()))
				if err != nil {
					ctx.Handle(500, "GetSubModule", err)
					return
				}
				smUrl := ""
				if sm != nil {
					smUrl = sm.Url
				}

				c, err := ctx.Repo.Commit.GetCommitOfRelPath(filepath.Join(treePath, te.Name()))
				if err != nil {
					ctx.Handle(500, "GetCommitOfRelPath", err)
					return
				}
				files = append(files, []interface{}{te, git.NewSubModuleFile(c, smUrl, te.Id.String())})
			}
		}
		ctx.Data["Files"] = files

		var readmeFile *git.Blob

		for _, f := range entries {
			if f.IsDir() || !base.IsReadmeFile(f.Name()) {
				continue
			} else {
				readmeFile = f.Blob()
				break
			}
		}

		if readmeFile != nil {
			ctx.Data["ReadmeInList"] = true
			ctx.Data["ReadmeExist"] = true
			if dataRc, err := readmeFile.Data(); err != nil {
				ctx.Handle(404, "repo.SinglereadmeFile.LookupBlob", err)
				return
			} else {

				buf := make([]byte, 1024)
				n, _ := dataRc.Read(buf)
				if n > 0 {
					buf = buf[:n]
				}

				ctx.Data["FileSize"] = readmeFile.Size()
				ctx.Data["FileLink"] = rawLink + "/" + treename
				_, isTextFile := base.IsTextFile(buf)
				ctx.Data["FileIsText"] = isTextFile
				ctx.Data["FileName"] = readmeFile.Name()
				if isTextFile {
					d, _ := ioutil.ReadAll(dataRc)
					buf = append(buf, d...)
					switch {
					case base.IsMarkdownFile(readmeFile.Name()):
						buf = base.RenderMarkdown(buf, branchLink)
					default:
						buf = bytes.Replace(buf, []byte("\n"), []byte(`<br>`), -1)
					}
					ctx.Data["FileContent"] = string(buf)
				}
			}
		}

		lastCommit := ctx.Repo.Commit
		if len(treePath) > 0 {
			c, err := ctx.Repo.Commit.GetCommitOfRelPath(treePath)
			if err != nil {
				ctx.Handle(500, "GetCommitOfRelPath", err)
				return
			}
			lastCommit = c
		}
		ctx.Data["LastCommit"] = lastCommit
		ctx.Data["LastCommitUser"] = models.ValidateCommitWithEmail(lastCommit)
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName

	var treenames []string
	Paths := make([]string, 0)

	if len(treename) > 0 {
		treenames = strings.Split(treename, "/")
		for i, _ := range treenames {
			Paths = append(Paths, strings.Join(treenames[0:i+1], "/"))
		}

		ctx.Data["HasParentPath"] = true
		if len(Paths)-2 >= 0 {
			ctx.Data["ParentPath"] = "/" + Paths[len(Paths)-2]
		}
	}

	ctx.Data["Paths"] = Paths
	ctx.Data["TreeName"] = treename
	ctx.Data["Treenames"] = treenames
	ctx.Data["TreePath"] = treePath
	ctx.Data["BranchLink"] = branchLink
	ctx.HTML(200, HOME)
}
