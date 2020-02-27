// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-macaron/csrf"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/tool"
)

type ToggleOptions struct {
	SignInRequired  bool
	SignOutRequired bool
	AdminRequired   bool
	DisableCSRF     bool
}

func Toggle(options *ToggleOptions) macaron.Handler {
	return func(c *Context) {
		// Cannot view any page before installation.
		if !conf.Security.InstallLock {
			c.Redirect(conf.Server.Subpath + "/install")
			return
		}

		// Check prohibit login users.
		if c.IsLogged && c.User.ProhibitLogin {
			c.Data["Title"] = c.Tr("auth.prohibit_login")
			c.HTML(200, "user/auth/prohibit_login")
			return
		}

		// Check non-logged users landing page.
		if !c.IsLogged && c.Req.RequestURI == "/" && conf.Server.LandingURL != "/" {
			c.Redirect(conf.Server.LandingURL)
			return
		}

		// Redirect to dashboard if user tries to visit any non-login page.
		if options.SignOutRequired && c.IsLogged && c.Req.RequestURI != "/" {
			c.Redirect(conf.Server.Subpath + "/")
			return
		}

		if !options.SignOutRequired && !options.DisableCSRF && c.Req.Method == "POST" && !auth.IsAPIPath(c.Req.URL.Path) {
			csrf.Validate(c.Context, c.csrf)
			if c.Written() {
				return
			}
		}

		if options.SignInRequired {
			if !c.IsLogged {
				// Restrict API calls with error message.
				if auth.IsAPIPath(c.Req.URL.Path) {
					c.JSON(403, map[string]string{
						"message": "Only signed in user is allowed to call APIs.",
					})
					return
				}

				c.SetCookie("redirect_to", url.QueryEscape(conf.Server.Subpath+c.Req.RequestURI), 0, conf.Server.Subpath)
				c.Redirect(conf.Server.Subpath + "/user/login")
				return
			} else if !c.User.IsActive && conf.Auth.RequireEmailConfirmation {
				c.Data["Title"] = c.Tr("auth.active_your_account")
				c.HTML(200, "user/auth/activate")
				return
			}
		}

		// Redirect to log in page if auto-signin info is provided and has not signed in.
		if !options.SignOutRequired && !c.IsLogged && !auth.IsAPIPath(c.Req.URL.Path) &&
			len(c.GetCookie(conf.Security.CookieUsername)) > 0 {
			c.SetCookie("redirect_to", url.QueryEscape(conf.Server.Subpath+c.Req.RequestURI), 0, conf.Server.Subpath)
			c.Redirect(conf.Server.Subpath + "/user/login")
			return
		}

		if options.AdminRequired {
			if !c.User.IsAdmin {
				c.Error(403)
				return
			}
			c.Data["PageIsAdmin"] = true
		}
	}
}

// RequireBasicAuth verifies HTTP Basic Authentication header with given credentials
func (c *Context) RequireBasicAuth(username, password string) {
	fields := strings.Fields(c.Req.Header.Get("Authorization"))
	if len(fields) != 2 || fields[0] != "Basic" {
		c.Status(http.StatusUnauthorized)
		return
	}

	uname, passwd, _ := tool.BasicAuthDecode(fields[1])
	if uname != username || passwd != password {
		c.Status(http.StatusForbidden)
		return
	}
}
