// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	PULLS base.TplName = "repo/pulls"
)

func Pulls(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarPulls"] = true
	ctx.HTML(200, PULLS)
}
