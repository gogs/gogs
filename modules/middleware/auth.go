// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/modules/base"
)

// SignInRequire requires user to sign in.
func SignInRequire(redirect bool) martini.Handler {
	return func(ctx *Context) {
		if !ctx.IsSigned {
			if redirect {
				ctx.Redirect("/")
			}
			return
		} else if !ctx.User.IsActive && base.Service.RegisterEmailConfirm {
			ctx.Data["Title"] = "Activate Your Account"
			ctx.Render.HTML(200, "user/active", ctx.Data)
			return
		}
	}
}

// SignOutRequire requires user to sign out.
func SignOutRequire() martini.Handler {
	return func(ctx *Context) {
		if ctx.IsSigned {
			ctx.Redirect("/")
		}
	}
}
