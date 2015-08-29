// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/url"

	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/csrf"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type ToggleOptions struct {
	SignInRequire  bool
	SignOutRequire bool
	AdminRequire   bool
	DisableCsrf    bool
}

// AutoSignIn reads cookie and try to auto-login.
func AutoSignIn(ctx *Context) (bool, error) {
	uname := ctx.GetCookie(setting.CookieUserName)
	if len(uname) == 0 {
		return false, nil
	}

	isSucceed := false
	defer func() {
		if !isSucceed {
			log.Trace("auto-login cookie cleared: %s", uname)
			ctx.SetCookie(setting.CookieUserName, "", -1, setting.AppSubUrl)
			ctx.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubUrl)
		}
	}()

	u, err := models.GetUserByName(uname)
	if err != nil {
		if !models.IsErrUserNotExist(err) {
			return false, fmt.Errorf("GetUserByName: %v", err)
		}
		return false, nil
	}

	if val, _ := ctx.GetSuperSecureCookie(
		base.EncodeMd5(u.Rands+u.Passwd), setting.CookieRememberName); val != u.Name {
		return false, nil
	}

	isSucceed = true
	ctx.Session.Set("uid", u.Id)
	ctx.Session.Set("uname", u.Name)
	return true, nil
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
			ctx.Redirect(setting.AppSubUrl + string(setting.LandingPageUrl))
			return
		}

		// Redirect to dashboard if user tries to visit any non-login page.
		if options.SignOutRequire && ctx.IsSigned && ctx.Req.RequestURI != "/" {
			ctx.Redirect(setting.AppSubUrl + "/")
			return
		}

		if !options.SignOutRequire && !options.DisableCsrf && ctx.Req.Method == "POST" && !auth.IsAPIPath(ctx.Req.URL.Path) {
			csrf.Validate(ctx.Context, ctx.csrf)
			if ctx.Written() {
				return
			}
		}

		if options.SignInRequire {
			if !ctx.IsSigned {
				// Restrict API calls with error message.
				if auth.IsAPIPath(ctx.Req.URL.Path) {
					ctx.HandleAPI(403, "Only signed in user is allowed to call APIs.")
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

// Contexter middleware already checks token for user sign in process.
func ApiReqToken() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsSigned {
			ctx.Error(401)
			return
		}
	}
}

func ApiReqBasicAuth() macaron.Handler {
	return func(ctx *Context) {
		if !ctx.IsBasicAuth {
			ctx.Error(401)
			return
		}
	}
}
