// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"fmt"
	"path"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

type repo struct {
	RepoLink string `json:"repolink"`
}

func SearchRepos(ctx *middleware.Context) {
	opt := models.SearchOption{
		Keyword: path.Base(ctx.Query("q")),
		Uid:     com.StrTo(ctx.Query("uid")).MustInt64(),
		Limit:   com.StrTo(ctx.Query("limit")).MustInt(),
	}
	if opt.Limit == 0 {
		opt.Limit = 10
	}

	repos, err := models.SearchRepositoryByName(opt)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*repo, len(repos))
	for i := range repos {
		if err = repos[i].GetOwner(); err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		results[i] = &repo{
			RepoLink: path.Join(repos[i].Owner.Name, repos[i].Name),
		}
	}

	ctx.Render.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

func Migrate(ctx *middleware.Context, form auth.MigrateRepoForm) {
	u, err := models.GetUserByName(ctx.Query("username"))
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if !u.ValidtePassword(ctx.Query("password")) {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": "username or password is not correct",
		})
		return
	}

	ctxUser := u
	// Not equal means current user is an organization.
	if form.Uid != u.Id {
		org, err := models.GetUserById(form.Uid)
		if err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		ctxUser = org
	}

	if ctx.HasError() {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": ctx.GetErrMsg(),
		})
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOrgOwner(u.Id) {
			ctx.JSON(403, map[string]interface{}{
				"ok":    false,
				"error": "given user is not owner of organization",
			})
			return
		}
	}

	authStr := strings.Replace(fmt.Sprintf("://%s:%s",
		form.AuthUserName, form.AuthPasswd), "@", "%40", -1)
	url := strings.Replace(form.HttpsUrl, "://", authStr+"@", 1)
	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.JSON(200, map[string]interface{}{
			"ok":   true,
			"data": "/" + ctxUser.Name + "/" + form.RepoName,
		})
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.Id, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	ctx.JSON(500, map[string]interface{}{
		"ok":    false,
		"error": err.Error(),
	})
}
