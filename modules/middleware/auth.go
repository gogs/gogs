// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"github.com/codegangsta/martini"
)

func SignInRequire(redirect bool) martini.Handler {
	return func(ctx *Context) {
		if !ctx.IsSigned {
			if redirect {
				ctx.Render.Redirect("/")
			}
			return
		}
	}
}

func SignOutRequire() martini.Handler {
	return func(ctx *Context) {
		if ctx.IsSigned {
			ctx.Render.Redirect("/")
		}
	}
}
