// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

type user struct {
	UserName   string `json:"username"`
	AvatarLink string `json:"avatar"`
}

func SearchUser(ctx *middleware.Context) {
	q := ctx.Query("q")
	limit, err := base.StrTo(ctx.Query("limit")).Int()
	if err != nil {
		limit = 10
	}

	us, err := models.SearchUserByName(q, limit)
	if err != nil {
		ctx.JSON(500, nil)
		return
	}

	results := make([]*user, len(us))
	for i := range us {
		results[i] = &user{us[i].Name, us[i].AvatarLink()}
	}

	ctx.Render.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}
