// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dev

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func TemplatePreview(ctx *middleware.Context, params martini.Params) {
	ctx.Data["User"] = models.User{Name: "Unknown"}
	ctx.Data["AppName"] = base.AppName
	ctx.Data["AppVer"] = base.AppVer
	ctx.Data["AppUrl"] = base.AppUrl
	ctx.Data["AppLogo"] = base.AppLogo
	ctx.Data["Code"] = "2014031910370000009fff6782aadb2162b4a997acb69d4400888e0b9274657374"
	ctx.Data["ActiveCodeLives"] = base.Service.ActiveCodeLives / 60
	ctx.Data["ResetPwdCodeLives"] = base.Service.ResetPwdCodeLives / 60
	ctx.HTML(200, params["_1"])
}
