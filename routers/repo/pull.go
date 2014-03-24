// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/modules/middleware"
)

func Pulls(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarPulls"] = true
	if len(params["branchname"]) == 0 {
		params["branchname"] = "master"
	}

	ctx.Data["Branchname"] = params["branchname"]
	ctx.HTML(200, "repo/pulls")
}
