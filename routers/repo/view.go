// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/template"

	"github.com/generaltso/linguist"
)

const (
	HOME     base.TplName = "repo/home"
	WATCHERS base.TplName = "repo/watchers"
	FORKS    base.TplName = "repo/forks"
)

func Home(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Repo.Repository.Name
	ctx.Data["PageIsViewCode"] = true
	ctx.Data["RequireHighlightJS"] = true

	branchName := ctx.Repo.BranchName
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	repoLink := ctx.Repo.RepoLink
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	treeLink := branchLink
	rawLink := ctx.Repo.RepoLink + "/raw/" + branchName

	// Get tree path
	treename := ctx.Repo.TreeName

	if len(treename) > 0 {
		if treename[len(treename)-1] == '/' {
			ctx.Redirect(repoLink + "/src/" + branchName + "/" + treename[:len(treename)-1])
			return
		}

		treeLink += "/" + treename
	}

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
					ctx.Data["FileContent"] = string(base.RenderMarkdown(buf, path.Dir(treeLink), ctx.Repo.Repository.ComposeMetas()))
				} else {
					if err, content := template.ToUtf8WithErr(buf); err != nil {
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
				files = append(files, []interface{}{te, git.NewSubModuleFile(c, smUrl, te.ID.String())})
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
				ctx.Handle(404, "repo.SinglereadmeFile.Data", err)
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
						buf = base.RenderMarkdown(buf, treeLink, ctx.Repo.Repository.ComposeMetas())
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

		branchId, err := ctx.Repo.GitRepo.GetCommitIdOfBranch(branchName)
		if err != nil || branchId != lastCommit.ID.String() {
			branchId = lastCommit.ID.String()
		}
		Langs := getLanguageStats(ctx, branchId)
		ctx.Data["LanguageStats"] = Langs

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

func renderItems(ctx *middleware.Context, total int, getter func(page int) ([]*models.User, error)) {
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
	ctx.Data["Watchers"] = items

	ctx.HTML(200, WATCHERS)
}

func Watchers(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.watchers")
	ctx.Data["PageIsWatchers"] = true
	renderItems(ctx, ctx.Repo.Repository.NumWatches, ctx.Repo.Repository.GetWatchers)
}

func Stars(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.stargazers")
	ctx.Data["PageIsStargazers"] = true
	renderItems(ctx, ctx.Repo.Repository.NumStars, ctx.Repo.Repository.GetStargazers)
}

func Forks(ctx *middleware.Context) {
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

func getLanguageStats(ctx *middleware.Context, branchId string) interface{} {

	all_files := linguistlstree(ctx, branchId)
	languages := map[string]float64{}

	var total_size float64
	for _, f := range all_files {
		languages[f.Language] += f.Size
		total_size += f.Size
	}

	percent := []float64{}
	results := map[float64]string{}

	for lang, size := range languages {
		p := size / total_size * 100.0
		percent = append(percent, p)
		results[p] = lang
	}

	sort.Sort(sort.Reverse(sort.Float64Slice(percent)))

	ret := []*LanguageStat{}
	for i, p := range percent {
		// limit result set
		if i > 10 {
			break
		}
		lang := results[p]
		color := linguist.GetColor(lang)
		if color == "" {
			color = "#ccc" //grey
		}
		ret = append(ret, &LanguageStat{Name: lang,
			Percent: fmt.Sprintf("%.2f%%", p),
			Color:   color})
	}
	return ret
}

type LanguageStat struct {
	Name    string
	Percent string
	Color   string
}

// see below
type file struct {
	Name     string
	Size     float64
	Language string
}

// just some utilities...
func gitcmd(ctx *middleware.Context, args ...string) string {
	stdout, _, err := com.ExecCmdDir(ctx.Repo.GitRepo.Path, "git", args...)
	tsoErr(ctx, err)
	return stdout
}
func gitcmdbytes(ctx *middleware.Context, args ...string) []byte {
	stdout, _, err := com.ExecCmdDirBytes(ctx.Repo.GitRepo.Path, "git", args...)
	tsoErr(ctx, err)
	return stdout
}
func tsoErr(ctx *middleware.Context, err error) {
	if err != nil {
		ctx.Handle(500, "*blames tso*", err)
	}
}

// returns every file in a tree
// additionally detecting programming language
func linguistlstree(ctx *middleware.Context, treeish string) (files []*file) {
	files = []*file{}
	lstext := gitcmd(ctx, "ls-tree", treeish)
	for _, ln := range strings.Split(lstext, "\n") {
		fields := strings.Split(ln, " ")
		if len(fields) != 3 {
			continue
		}
		//fmode := fields[0]
		ftype := fields[1]
		fields = strings.Split(fields[2], "\t")
		if len(fields) != 2 {
			continue
		}
		fhash := fields[0]
		fname := fields[1]

		switch ftype {
		// broken, don't know why
		//		case "tree":
		//			subdir := linguistlstree(ctx, fhash)
		//			files = append(files, subdir...)
		case "blob":
			// if it's vendored, don't even look at it
			// (vendored means files like README.md, .gitignore, etc...)
			if linguist.IsVendored(fname) {
				continue
			}

			ssize := gitcmd(ctx, "cat-file", "-s", fhash)
			fsize, err := strconv.ParseFloat(strings.TrimSpace(ssize), 64)
			tsoErr(ctx, err)

			// if it's an empty file don't even waste time
			if fsize == 0 {
				continue
			}

			f := &file{}
			f.Name = fname
			f.Size = fsize

			//
			// language detection
			//

			// by file extension
			by_ext := linguist.DetectFromFilename(fname)
			if by_ext != "" {
				f.Language = by_ext
				files = append(files, f)
				continue
			}
			// by mimetype
			// if we can't guess type by extension, then before jumping into
			// lexing and parsing things like image files or cat videos
			// ...or other binary formats which will give erroneous results...
			// ...or other binary formats which will give erroneous results...
			// with the linguist.DetectFromContents method, I posit looking
			// at mimetype with linguist.DetectMimeFromFilename
			//
			// ...however, this is not what github does at all, instead ignoring
			// binary files altogether. However, there is no law that states
			// git must be used for code only.
			by_mime, shouldIgnore, _ := linguist.DetectMimeFromFilename(fname)
			if by_mime != "" && shouldIgnore {
				f.Language = by_mime
				files = append(files, f)
				continue
			}

			// by contents
			// see also: github.com/github/linguist
			// see also: github.com/generaltso/linguist
			contents := gitcmdbytes(ctx, "cat-file", "blob", fhash)
			by_contents := linguist.DetectFromContents(contents)
			if by_contents != "" {
				f.Language = by_contents
			} else {
				f.Language = "(undetermined)"
			}
			files = append(files, f)
		}
	}
	return files
}
