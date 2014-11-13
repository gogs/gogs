// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

type ApiUser struct {
	Id        int64  `json:"id"`
	UserName  string `json:"username"`
	AvatarUrl string `json:"avatar_url"`
}

func SearchUsers(ctx *middleware.Context) {
	opt := models.SearchOption{
		Keyword: ctx.Query("q"),
		Limit:   com.StrTo(ctx.Query("limit")).MustInt(),
	}
	if opt.Limit == 0 {
		opt.Limit = 10
	}

	us, err := models.SearchUserByName(opt)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*ApiUser, len(us))
	for i := range us {
		results[i] = &ApiUser{
			UserName:  us[i].Name,
			AvatarUrl: us[i].AvatarLink(),
		}
	}

	ctx.Render.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}
