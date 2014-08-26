// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"path"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
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
