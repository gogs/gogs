package repo

import (
	"github.com/codegangsta/martini"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"
	"net/http"
)


func Single(params martini.Params, req *http.Request, r render.Render, data base.TmplData, session sessions.Session) {
	var (
		user *models.User
		err  error
	)
	// get repository owner
	isOwner := (data["SignedUserName"] == params["username"])
	if !isOwner {
		user, err = models.GetUserByName(params["username"])
		if err != nil {
			data["ErrorMsg"] = err
			//log.Error("repo.Single: %v", err)
			r.HTML(200, "base/error", data)
			return
		}
	} else {
		user = auth.SignedInUser(session)
	}
	if user == nil {
		data["ErrorMsg"] = "invliad user account for single repository"
		//log.Error("repo.Single: %v", err)
		r.HTML(200, "base/error", data)
		return
	}
	data["IsRepositoryOwner"] = isOwner

	// get repository
	repo, err := models.GetRepositoryByName(user, params["reponame"])
	if err != nil {
		data["ErrorMsg"] = err
		//log.Error("repo.Single: %v", err)
		r.HTML(200, "base/error", data)
		return
	}

	data["Repository"] = repo
	data["Owner"] = user
	data["Title"] = user.Name + "/" + repo.Name
	data["RepositoryLink"] = data["Title"]
	data["IsRepoToolbarSource"] = true

	r.HTML(200, "repo/single", data)
}
