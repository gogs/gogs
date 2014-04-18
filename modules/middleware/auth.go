// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/url"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/base"
)

type ToggleOptions struct {
	SignInRequire  bool
	SignOutRequire bool
	AdminRequire   bool
	DisableCsrf    bool
}

func Toggle(options *ToggleOptions) martini.Handler {
	return func(ctx *Context) {
		if !base.InstallLock {
			ctx.Redirect("/install")
			return
		}

		if options.SignOutRequire && ctx.IsSigned && ctx.Req.RequestURI != "/" {
			ctx.Redirect("/")
			return
		}

		if !options.DisableCsrf {
			if ctx.Req.Method == "POST" {
				if !ctx.CsrfTokenValid() {
					ctx.Error(403, "CSRF token does not match")
					return
				}
			}
		}

		if options.SignInRequire {
			if !ctx.IsSigned {
				ctx.SetCookie("redirect_to", "/"+url.QueryEscape(ctx.Req.RequestURI))
				ctx.Redirect("/user/login")
				return
			} else if !ctx.User.IsActive && base.Service.RegisterEmailConfirm {
				ctx.Data["Title"] = "Activate Your Account"
				ctx.HTML(200, "user/activate")
				return
			}
		}

		if options.AdminRequire {
			if !ctx.User.IsAdmin {
				ctx.Error(403)
				return
			}
			ctx.Data["PageIsAdmin"] = true
		}
	}
}
