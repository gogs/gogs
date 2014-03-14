package repo

import (
	"strings"
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
	if params["branchname"] == "" {
		params["branchname"] = "master"
	}
	treename := params["_1"]
	files, err := models.GetReposFiles(params["username"], params["reponame"],
		params["branchname"], treename)
	if err != nil {
		log.Handle(200, "repo.Single", data, r, err)
		return
	}

	data["Username"] = params["username"]
	data["Reponame"] = params["reponame"]
	data["Branchname"] = params["branchname"]
	treenames := strings.Split(treename, "/")
	Paths := make([]string, 0)
	for i, _ := range treenames {
		Paths = append(Paths, strings.Join(treenames[0:i+1], "/"))
	}
	data["Paths"] = Paths
	data["Treenames"] = treenames
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
