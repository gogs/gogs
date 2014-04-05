// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/webdav"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Create(ctx *middleware.Context, form auth.CreateRepoForm) {
	ctx.Data["Title"] = "Create repository"
	ctx.Data["PageIsNewRepo"] = true // For navbar arrow.
	ctx.Data["LanguageIgns"] = models.LanguageIgns
	ctx.Data["Licenses"] = models.Licenses

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "repo/create")
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, "repo/create")
		return
	}

	_, err := models.CreateRepository(ctx.User, form.RepoName, form.Description,
		form.Language, form.License, form.Visibility == "private", form.InitReadme == "on")
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
	ctx.Handle(200, "repo.Create", err)
}

func Single(ctx *middleware.Context, params martini.Params) {
	branchName := ctx.Repo.BranchName
	commitId := ctx.Repo.CommitId
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

	// Branches.
	brs, err := models.GetBranches(userName, repoName)
	if err != nil {
		ctx.Handle(404, "repo.Single(GetBranches)", err)
		return
	}

	ctx.Data["Branches"] = brs

	isViewBranch := ctx.Repo.IsBranch
	ctx.Data["IsViewBranch"] = isViewBranch

	repoFile, err := models.GetTargetFile(userName, repoName,
		branchName, commitId, treename)

	if err != nil && err != models.ErrRepoFileNotExist {
		ctx.Handle(404, "repo.Single(GetTargetFile)", err)
		return
	}

	if len(treename) != 0 && repoFile == nil {
		ctx.Handle(404, "repo.Single", nil)
		return
	}

	if repoFile != nil && repoFile.IsFile() {
		if blob, err := repoFile.LookupBlob(); err != nil {
			ctx.Handle(404, "repo.Single(repoFile.LookupBlob)", err)
		} else {
			ctx.Data["FileSize"] = repoFile.Size
			ctx.Data["IsFile"] = true
			ctx.Data["FileName"] = repoFile.Name
			ext := path.Ext(repoFile.Name)
			if len(ext) > 0 {
				ext = ext[1:]
			}
			ctx.Data["FileExt"] = ext
			ctx.Data["FileLink"] = rawLink + "/" + treename

			data := blob.Contents()
			_, isTextFile := base.IsTextFile(data)
			_, isImageFile := base.IsImageFile(data)
			ctx.Data["FileIsText"] = isTextFile

			if isImageFile {
				ctx.Data["IsImageFile"] = true
			} else {
				readmeExist := base.IsMarkdownFile(repoFile.Name) || base.IsReadmeFile(repoFile.Name)
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
		files, err := models.GetReposFiles(userName, repoName, ctx.Repo.CommitId, treename)
		if err != nil {
			ctx.Handle(404, "repo.Single(GetReposFiles)", err)
			return
		}

		ctx.Data["Files"] = files

		var readmeFile *models.RepoFile

		for _, f := range files {
			if !f.IsFile() || !base.IsReadmeFile(f.Name) {
				continue
			} else {
				readmeFile = f
				break
			}
		}

		if readmeFile != nil {
			ctx.Data["ReadmeInSingle"] = true
			ctx.Data["ReadmeExist"] = true
			if blob, err := readmeFile.LookupBlob(); err != nil {
				ctx.Handle(404, "repo.Single(readmeFile.LookupBlob)", err)
				return
			} else {
				ctx.Data["FileSize"] = readmeFile.Size
				ctx.Data["FileLink"] = rawLink + "/" + treename
				data := blob.Contents()
				_, isTextFile := base.IsTextFile(data)
				ctx.Data["FileIsText"] = isTextFile
				ctx.Data["FileName"] = readmeFile.Name
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
	ctx.Data["BranchLink"] = branchLink
	ctx.HTML(200, "repo/single")
}

func SingleDownload(ctx *middleware.Context, params martini.Params) {
	// Get tree path
	treename := params["_1"]

	branchName := params["branchname"]
	userName := params["username"]
	repoName := params["reponame"]

	var commitId string
	if !models.IsBranchExist(userName, repoName, branchName) {
		commitId = branchName
		branchName = ""
	}

	repoFile, err := models.GetTargetFile(userName, repoName,
		branchName, commitId, treename)

	if err != nil {
		ctx.Handle(404, "repo.SingleDownload(GetTargetFile)", err)
		return
	}

	blob, err := repoFile.LookupBlob()
	if err != nil {
		ctx.Handle(404, "repo.SingleDownload(LookupBlob)", err)
		return
	}

	data := blob.Contents()
	contentType, isTextFile := base.IsTextFile(data)
	_, isImageFile := base.IsImageFile(data)
	ctx.Res.Header().Set("Content-Type", contentType)
	if !isTextFile && !isImageFile {
		ctx.Res.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(treename))
		ctx.Res.Header().Set("Content-Transfer-Encoding", "binary")
	}
	ctx.Res.Write(data)
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
	ctx.ResponseWriter.Header().Set("WWW-Authenticate", `Basic realm="Gogs Auth"`)
	ctx.Data["ErrorMsg"] = "no basic auth and digit auth"
	ctx.HTML(401, fmt.Sprintf("status/401"))
}

func Http(ctx *middleware.Context, params martini.Params) {
	username := params["username"]
	reponame := params["reponame"]
	if strings.HasSuffix(reponame, ".git") {
		reponame = reponame[:len(reponame)-4]
	}

	repoUser, err := models.GetUserByName(username)
	if err != nil {
		ctx.Handle(500, "repo.GetUserByName", nil)
		return
	}

	repo, err := models.GetRepositoryByName(repoUser.Id, reponame)
	if err != nil {
		ctx.Handle(500, "repo.GetRepositoryByName", nil)
		return
	}

	isPull := webdav.IsPullMethod(ctx.Req.Method)
	var askAuth = !(!repo.IsPrivate && isPull)

	//authRequired(ctx)
	//return

	// check access
	if askAuth {
		// check digit auth

		// check basic auth
		baHead := ctx.Req.Header.Get("Authorization")
		if baHead != "" {
			auths := strings.Fields(baHead)
			if len(auths) != 2 || auths[0] != "Basic" {
				ctx.Handle(401, "no basic auth and digit auth", nil)
				return
			}
			authUsername, passwd, err := basicDecode(auths[1])
			if err != nil {
				ctx.Handle(401, "no basic auth and digit auth", nil)
				return
			}

			authUser, err := models.GetUserByName(authUsername)
			if err != nil {
				ctx.Handle(401, "no basic auth and digit auth", nil)
				return
			}

			newUser := &models.User{Passwd: passwd}
			newUser.EncodePasswd()
			if authUser.Passwd != newUser.Passwd {
				ctx.Handle(401, "no basic auth and digit auth", nil)
				return
			}

			var tp = models.AU_WRITABLE
			if isPull {
				tp = models.AU_READABLE
			}

			has, err := models.HasAccess(authUsername, username+"/"+reponame, tp)
			if err != nil || !has {
				ctx.Handle(401, "no basic auth and digit auth", nil)
				return
			}
		} else {
			authRequired(ctx)
			return
		}
	}

	dir := models.RepoPath(username, reponame)

	prefix := path.Join("/", username, params["reponame"])
	server := webdav.NewServer(
		dir, prefix, true)

	server.ServeHTTP(ctx.ResponseWriter, ctx.Req)
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

	switch ctx.Query("action") {
	case "update":
		isNameChanged := false
		newRepoName := ctx.Query("name")
		// Check if repository name has been changed.
		if ctx.Repo.Repository.Name != newRepoName {
			isExist, err := models.IsRepositoryExist(ctx.Repo.Owner, newRepoName)
			if err != nil {
				ctx.Handle(404, "repo.SettingPost(update: check existence)", err)
				return
			} else if isExist {
				ctx.RenderWithErr("Repository name has been taken in your repositories.", "repo/setting", nil)
				return
			} else if err = models.ChangeRepositoryName(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName); err != nil {
				ctx.Handle(404, "repo.SettingPost(change repository name)", err)
				return
			}
			log.Trace("%s Repository name changed: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newRepoName)

			isNameChanged = true
			ctx.Repo.Repository.Name = newRepoName
		}

		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		if err := models.UpdateRepository(ctx.Repo.Repository); err != nil {
			ctx.Handle(404, "repo.SettingPost(update)", err)
			return
		}

		ctx.Data["IsSuccess"] = true
		if isNameChanged {
			ctx.Redirect(fmt.Sprintf("/%s/%s/settings", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
		} else {
			ctx.HTML(200, "repo/setting")
		}
		log.Trace("%s Repository updated: %s/%s", ctx.Req.RequestURI, ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
	case "transfer":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		newOwner := ctx.Query("owner")
		// Check if new owner exists.
		isExist, err := models.IsUserExist(newOwner)
		if err != nil {
			ctx.Handle(404, "repo.SettingPost(transfer: check existence)", err)
			return
		} else if !isExist {
			ctx.RenderWithErr("Please make sure you entered owner name is correct.", "repo/setting", nil)
			return
		} else if err = models.TransferOwnership(ctx.User, newOwner, ctx.Repo.Repository); err != nil {
			ctx.Handle(404, "repo.SettingPost(transfer repository)", err)
			return
		}
		log.Trace("%s Repository transfered: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newOwner)

		ctx.Redirect("/")
		return
	case "delete":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		if err := models.DeleteRepository(ctx.User.Id, ctx.Repo.Repository.Id, ctx.User.LowerName); err != nil {
			ctx.Handle(200, "repo.Delete", err)
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
