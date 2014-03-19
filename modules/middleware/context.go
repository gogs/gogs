// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
)

// Context represents context of a request.
type Context struct {
	*Render
	c        martini.Context
	p        martini.Params
	Req      *http.Request
	Res      http.ResponseWriter
	Session  sessions.Session
	User     *models.User
	IsSigned bool

	Repo struct {
		IsValid    bool
		IsOwner    bool
		Repository *models.Repository
		Owner      *models.User
	}
}

// Query querys form parameter.
func (ctx *Context) Query(name string) string {
	ctx.Req.ParseForm()
	return ctx.Req.Form.Get(name)
}

// func (ctx *Context) Param(name string) string {
// 	return ctx.p[name]
// }

// HasError returns true if error occurs in form validation.
func (ctx *Context) HasError() bool {
	hasErr, ok := ctx.Data["HasError"]
	if !ok {
		return false
	}
	return hasErr.(bool)
}

// RenderWithErr used for page has form validation but need to prompt error to users.
func (ctx *Context) RenderWithErr(msg, tpl string, form auth.Form) {
	ctx.Data["HasError"] = true
	ctx.Data["ErrorMsg"] = msg
	auth.AssignForm(form, ctx.Data)
	ctx.HTML(200, tpl, ctx.Data)
}

// Handle handles and logs error by given status.
func (ctx *Context) Handle(status int, title string, err error) {
	log.Error("%s: %v", title, err)
	if martini.Dev == martini.Prod {
		ctx.HTML(500, "status/500", ctx.Data)
		return
	}

	ctx.Data["ErrorMsg"] = err
	ctx.HTML(status, fmt.Sprintf("status/%d", status), ctx.Data)
}

// InitContext initializes a classic context for a request.
func InitContext() martini.Handler {
	return func(res http.ResponseWriter, r *http.Request, c martini.Context,
		session sessions.Session, rd *Render) {

		ctx := &Context{
			c: c,
			// p:      p,
			Req:     r,
			Res:     res,
			Session: session,
			Render:  rd,
		}

		// Get user from session if logined.
		user := auth.SignedInUser(session)
		ctx.User = user
		ctx.IsSigned = user != nil

		ctx.Data["IsSigned"] = ctx.IsSigned

		if user != nil {
			ctx.Data["SignedUser"] = user
			ctx.Data["SignedUserId"] = user.Id
			ctx.Data["SignedUserName"] = user.LowerName
		}

		ctx.Data["PageStartTime"] = time.Now()

		c.Map(ctx)

		c.Next()
	}
}
