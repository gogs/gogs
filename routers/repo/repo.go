// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gogits/git"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Create(ctx *middleware.Context) {
	ctx.Data["Title"] = "Create repository"
	ctx.Data["PageIsNewRepo"] = true
	ctx.Data["LanguageIgns"] = models.LanguageIgns
	ctx.Data["Licenses"] = models.Licenses
	ctx.HTML(200, "repo/create")
}

func CreatePost(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = "Create repository"
	ctx.Data["PageIsNewRepo"] = true
	ctx.Data["LanguageIgns"] = models.LanguageIgns
	ctx.Data["Licenses"] = models.Licenses

	if ctx.HasError() {
		ctx.HTML(200, "repo/create")
		return
	}

	repo, err := models.CreateRepository(ctx.User, form.RepoName, form.Description,
		form.Language, form.License, form.Private, false, form.InitReadme)
	if err == nil {
		log.Trace("%s Repository created: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, form.RepoName)
		ctx.Redirect("/" + ctx.User.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr("Repository name has already been used", "repo/create", &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "repo/create", &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctx.User.Id, repo.Id, ctx.User.Name); errDelete != nil {
			log.Error("repo.MigratePost(CreatePost): %v", errDelete)
		}
	}
	ctx.Handle(500, "repo.Create", err)
}

func Migrate(ctx *middleware.Context) {
	ctx.Data["Title"] = "Migrate repository"
	ctx.Data["PageIsNewRepo"] = true
	ctx.HTML(200, "repo/migrate")
}

func MigratePost(ctx *middleware.Context, form auth.MigrateRepoForm) {
	ctx.Data["Title"] = "Migrate repository"
	ctx.Data["PageIsNewRepo"] = true

	if ctx.HasError() {
		ctx.HTML(200, "repo/migrate")
		return
	}

	url := strings.Replace(form.Url, "://", fmt.Sprintf("://%s:%s@", form.AuthUserName, form.AuthPasswd), 1)
	repo, err := models.MigrateRepository(ctx.User, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("%s Repository migrated: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, form.RepoName)
		ctx.Redirect("/" + ctx.User.Name + "/" + form.RepoName)
		return
	} else if err == models.ErrRepoAlreadyExist {
		ctx.RenderWithErr("Repository name has already been used", "repo/migrate", &form)
		return
	} else if err == models.ErrRepoNameIllegal {
		ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), "repo/migrate", &form)
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctx.User.Id, repo.Id, ctx.User.Name); errDelete != nil {
			log.Error("repo.MigratePost(DeleteRepository): %v", errDelete)
		}
	}

	if strings.Contains(err.Error(), "Authentication failed") {
		ctx.RenderWithErr(err.Error(), "repo/migrate", &form)
		return
	}
	ctx.Handle(500, "repo.Migrate", err)
}

func Single(ctx *middleware.Context, params martini.Params) {
	branchName := ctx.Repo.BranchName
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	repoLink := ctx.Repo.RepoLink
	branchLink := ctx.Repo.RepoLink + "/src/" + branchName
	rawLink := ctx.Repo.RepoLink + "/raw/" + branchName

	// Get tree path
	treename := params["_1"]

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
		ctx.Handle(404, "repo.Single(GetTreeEntryByPath)", err)
		return
	}

	if len(treename) != 0 && entry == nil {
		ctx.Handle(404, "repo.Single", nil)
		return
	}

	if entry != nil && !entry.IsDir() {
		blob := entry.Blob()

		if data, err := blob.Data(); err != nil {
			ctx.Handle(404, "repo.Single(blob.Data)", err)
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

			_, isTextFile := base.IsTextFile(data)
			_, isImageFile := base.IsImageFile(data)
			ctx.Data["FileIsText"] = isTextFile

			if isImageFile {
				ctx.Data["IsImageFile"] = true
			} else {
				readmeExist := base.IsMarkdownFile(blob.Name()) || base.IsReadmeFile(blob.Name())
				ctx.Data["ReadmeExist"] = readmeExist
				if readmeExist {
					ctx.Data["FileContent"] = string(base.RenderMarkdown(data, ""))
				} else {
					if isTextFile {
						ctx.Data["FileContent"] = string(data)
					}
				}
			}
		}

	} else {
		// Directory and file list.
		tree, err := ctx.Repo.Commit.SubTree(treename)
		if err != nil {
			ctx.Handle(404, "repo.Single(SubTree)", err)
			return
		}
		entries := tree.ListEntries()
		entries.Sort()

		files := make([][]interface{}, 0, len(entries))

		for _, te := range entries {
			c, err := ctx.Repo.Commit.GetCommitOfRelPath(filepath.Join(treePath, te.Name()))
			if err != nil {
				ctx.Handle(404, "repo.Single(SubTree)", err)
				return
			}

			files = append(files, []interface{}{te, c})
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
			ctx.Data["ReadmeInSingle"] = true
			ctx.Data["ReadmeExist"] = true
			if data, err := readmeFile.Data(); err != nil {
				ctx.Handle(404, "repo.Single(readmeFile.LookupBlob)", err)
				return
			} else {
				ctx.Data["FileSize"] = readmeFile.Size
				ctx.Data["FileLink"] = rawLink + "/" + treename
				_, isTextFile := base.IsTextFile(data)
				ctx.Data["FileIsText"] = isTextFile
				ctx.Data["FileName"] = readmeFile.Name()
				if isTextFile {
					ctx.Data["FileContent"] = string(base.RenderMarkdown(data, branchLink))
				}
			}
		}
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

	ctx.Data["LastCommit"] = ctx.Repo.Commit
	ctx.Data["Paths"] = Paths
	ctx.Data["Treenames"] = treenames
	ctx.Data["TreePath"] = treePath
	ctx.Data["BranchLink"] = branchLink
	ctx.HTML(200, "repo/single")
}

func basicEncode(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func basicDecode(encoded string) (user string, name string, err error) {
	var s []byte
	s, err = base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return
	}

	a := strings.Split(string(s), ":")
	if len(a) == 2 {
		user, name = a[0], a[1]
	} else {
		err = errors.New("decode failed")
	}
	return
}

func authRequired(ctx *middleware.Context) {
	ctx.ResponseWriter.Header().Set("WWW-Authenticate", "Basic realm=\".\"")
	ctx.Data["ErrorMsg"] = "no basic auth and digit auth"
	ctx.HTML(401, fmt.Sprintf("status/401"))
}

func Setting(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "repo.Setting", nil)
		return
	}

	ctx.Data["IsRepoToolbarSetting"] = true

	var title string
	if t, ok := ctx.Data["Title"].(string); ok {
		title = t
	}

	ctx.Data["Title"] = title + " - settings"
	ctx.HTML(200, "repo/setting")
}

func SettingPost(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	ctx.Data["IsRepoToolbarSetting"] = true

	switch ctx.Query("action") {
	case "update":
		newRepoName := ctx.Query("name")
		// Check if repository name has been changed.
		if ctx.Repo.Repository.Name != newRepoName {
			isExist, err := models.IsRepositoryExist(ctx.Repo.Owner, newRepoName)
			if err != nil {
				ctx.Handle(500, "repo.SettingPost(update: check existence)", err)
				return
			} else if isExist {
				ctx.RenderWithErr("Repository name has been taken in your repositories.", "repo/setting", nil)
				return
			} else if err = models.ChangeRepositoryName(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName); err != nil {
				ctx.Handle(500, "repo.SettingPost(change repository name)", err)
				return
			}
			log.Trace("%s Repository name changed: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newRepoName)

			ctx.Repo.Repository.Name = newRepoName
		}

		br := ctx.Query("branch")

		if git.IsBranchExist(models.RepoPath(ctx.User.Name, ctx.Repo.Repository.Name), br) {
			ctx.Repo.Repository.DefaultBranch = br
		}
		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		ctx.Repo.Repository.IsPrivate = ctx.Query("private") == "on"
		ctx.Repo.Repository.IsGoget = ctx.Query("goget") == "on"
		if err := models.UpdateRepository(ctx.Repo.Repository); err != nil {
			ctx.Handle(404, "repo.SettingPost(update)", err)
			return
		}
		log.Trace("%s Repository updated: %s/%s", ctx.Req.RequestURI, ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)

		if ctx.Repo.Repository.IsMirror {
			if len(ctx.Query("interval")) > 0 {
				var err error
				ctx.Repo.Mirror.Interval, err = base.StrTo(ctx.Query("interval")).Int()
				if err != nil {
					log.Error("repo.SettingPost(get mirror interval): %v", err)
				} else if err = models.UpdateMirror(ctx.Repo.Mirror); err != nil {
					log.Error("repo.SettingPost(UpdateMirror): %v", err)
				}
			}
		}

		ctx.Flash.Success("Repository options has been successfully updated.")
		ctx.Redirect(fmt.Sprintf("/%s/%s/settings", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
	case "transfer":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		newOwner := ctx.Query("owner")
		// Check if new owner exists.
		isExist, err := models.IsUserExist(newOwner)
		if err != nil {
			ctx.Handle(500, "repo.SettingPost(transfer: check existence)", err)
			return
		} else if !isExist {
			ctx.RenderWithErr("Please make sure you entered owner name is correct.", "repo/setting", nil)
			return
		} else if err = models.TransferOwnership(ctx.User, newOwner, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "repo.SettingPost(transfer repository)", err)
			return
		}
		log.Trace("%s Repository transfered: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newOwner)

		ctx.Redirect("/")
	case "delete":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		if err := models.DeleteRepository(ctx.User.Id, ctx.Repo.Repository.Id, ctx.User.LowerName); err != nil {
			ctx.Handle(500, "repo.Delete", err)
			return
		}
		log.Trace("%s Repository deleted: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, ctx.Repo.Repository.LowerName)

		ctx.Redirect("/")
	}
}

func Action(ctx *middleware.Context, params martini.Params) {
	var err error
	switch params["action"] {
	case "watch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, true)
	case "unwatch":
		err = models.WatchRepo(ctx.User.Id, ctx.Repo.Repository.Id, false)
	case "desc":
		if !ctx.Repo.IsOwner {
			ctx.Error(404)
			return
		}

		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		err = models.UpdateRepository(ctx.Repo.Repository)
	}

	if err != nil {
		log.Error("repo.Action(%s): %v", params["action"], err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}
	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}
