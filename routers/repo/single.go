package repo

import (
	"github.com/gogits/gogs/modules/base"
	"github.com/martini-contrib/render"
)

func Single(r render.Render, data base.TmplData) {
	if !data["IsRepositoryValid"].(bool) {
		return
	}
	data["IsRepoToolbarSource"] = true
	r.HTML(200, "repo/single", data)
}
