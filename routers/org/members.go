// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/gogits/gogs/modules/middleware"
)

func Members(ctx *middleware.Context) {
	ctx.Data["Title"] = "Organization " + ctx.Params(":org") + " Members"
	ctx.HTML(200, "org/members")
}
