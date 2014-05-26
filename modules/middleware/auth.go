// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/url"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/setting"
)

type ToggleOptions struct {
	SignInRequire  bool
	SignOutRequire bool
	AdminRequire   bool
	DisableCsrf    bool
}

func Toggle(options *ToggleOptions) martini.Handler {
	return func(ctx *Context) {
		// Cannot view any page before installation.
		if !setting.InstallLock {
			ctx.Redirect("/install")
			return
		}

		// Redirect to dashboard if user tries to visit any non-login page.
		if options.SignOutRequire && ctx.IsSigned && ctx.Req.RequestURI != "/" {
			ctx.Redirect("/")
			return
		}

		if !options.DisableCsrf && ctx.Req.Method == "POST" && !ctx.CsrfTokenValid() {
			ctx.Error(403, "CSRF token does not match")
			return
		}

		if options.SignInRequire {
			if !ctx.IsSigned {
				// Ignore watch repository operation.
				if strings.HasSuffix(ctx.Req.RequestURI, "watch") {
					return
				}
				ctx.SetCookie("redirect_to", "/"+url.QueryEscape(ctx.Req.RequestURI))
				ctx.Redirect("/user/login")
				return
			} else if !ctx.User.IsActive && setting.Service.RegisterEmailConfirm {
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
