package repo

import (
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

func Single(params martini.Params, r render.Render, data base.TmplData) {
	if !data["IsRepositoryValid"].(bool) {
		return
	}

	files, err := models.GetReposFiles(params["username"], params["reponame"], "HEAD", "/")
	if err != nil {
		data["ErrorMsg"] = err
		log.Error("repo.List: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	data["IsRepoToolbarSource"] = true
	data["Files"] = files

	r.HTML(200, "repo/single", data)
}

func Setting(r render.Render, data base.TmplData) {
	if !data["IsRepositoryValid"].(bool) {
		return
	}

	data["Title"] = data["Title"].(string) + " - settings"
	data["IsRepoToolbarSetting"] = true

	r.HTML(200, "repo/setting", data)
}
