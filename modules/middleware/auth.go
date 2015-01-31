// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/url"
	"strings"

	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/csrf"

	"github.com/gogits/gogs/modules/setting"
)

type ToggleOptions struct {
	SignInRequire  bool
	SignOutRequire bool
	AdminRequire   bool
	DisableCsrf    bool
}

func Toggle(options *ToggleOptions) macaron.Handler {
	return func(ctx *Context) {
		// Cannot view any page before installation.
		if !setting.InstallLock {
			ctx.Redirect(setting.AppSubUrl + "/install")
			return
		}

		// Checking non-logged users landing page.
		if !ctx.IsSigned && ctx.Req.RequestURI == "/" && setting.LandingPageUrl != setting.LANDING_PAGE_HOME {
			ctx.Redirect(string(setting.LandingPageUrl))
			return
		}

		// Redirect to dashboard if user tries to visit any non-login page.
		if options.SignOutRequire && ctx.IsSigned && ctx.Req.RequestURI != "/" {
			ctx.Redirect(setting.AppSubUrl + "/")
			return
		}

		if !options.SignOutRequire && !options.DisableCsrf && ctx.Req.Method == "POST" {
			csrf.Validate(ctx.Context, ctx.csrf)
			if ctx.Written() {
				return
			}
		}

		if options.SignInRequire {
			if !ctx.IsSigned {
				// Ignore watch repository operation.
				if strings.HasSuffix(ctx.Req.RequestURI, "watch") {
					return
				}
				ctx.SetCookie("redirect_to", url.QueryEscape(setting.AppSubUrl+ctx.Req.RequestURI), 0, setting.AppSubUrl)
				ctx.Redirect(setting.AppSubUrl + "/user/login")
				return
			} else if !ctx.User.IsActive && setting.Service.RegisterEmailConfirm {
				ctx.Data["Title"] = ctx.Tr("auth.active_your_account")
				ctx.HTML(200, "user/auth/activate")
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

func ApiReqToken() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsSigned {
			ctx.Error(403)
			return
		}
	}
}

func ApiReqBasicAuth() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsBasicAuth {
			ctx.Error(403)
			return
		}
	}
}
