// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/middleware"
)

func Members(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Members"
	ctx.HTML(200, "org/members")
}
