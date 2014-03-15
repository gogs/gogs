// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

// Context represents context of a request.
type Context struct {
	c        martini.Context
	p        martini.Params
	Req      *http.Request
	Res      http.ResponseWriter
	Session  sessions.Session
	Data     base.TmplData
	Render   render.Render
	User     *models.User
	IsSigned bool
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
	ctx.Render.HTML(200, tpl, ctx.Data)
}

// Handle handles and logs error by given status.
func (ctx *Context) Handle(status int, title string, err error) {
	ctx.Data["ErrorMsg"] = err
	log.Error("%s: %v", title, err)
	ctx.Render.HTML(status, fmt.Sprintf("status/%d", status), ctx.Data)
}

// InitContext initializes a classic context for a request.
func InitContext() martini.Handler {
	return func(res http.ResponseWriter, r *http.Request, c martini.Context,
		session sessions.Session, rd render.Render) {

		data := base.TmplData{}

		ctx := &Context{
			c: c,
			// p:      p,
			Req:     r,
			Res:     res,
			Session: session,
			Data:    data,
			Render:  rd,
		}

		// Get user from session if logined.
		user := auth.SignedInUser(session)
		ctx.User = user
		ctx.IsSigned = user != nil

		data["IsSigned"] = ctx.IsSigned

		if user != nil {
			data["SignedUser"] = user
			data["SignedUserId"] = user.Id
			data["SignedUserName"] = user.LowerName
		}

		c.Map(ctx)
		c.Map(data)

		c.Next()
	}
}
