// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"

	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
	"github.com/martini-contrib/sessions"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

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

func (ctx *Context) Query(name string) string {
	ctx.Req.ParseForm()
	return ctx.Req.Form.Get(name)
}

// func (ctx *Context) Param(name string) string {
// 	return ctx.p[name]
// }

func (ctx *Context) Log(status int, title string, err error) {
	log.Handle(status, title, ctx.Data, ctx.Render, err)
}

func InitContext() martini.Handler {
	return func(res http.ResponseWriter, r *http.Request, c martini.Context,
		session sessions.Session, rd render.Render) {

		data := base.TmplData{}

		ctx := &Context{
			c: c,
			// p:      p,
			Req:    r,
			Res:    res,
			Data:   data,
			Render: rd,
		}

		// Get user from session if logined.
		user := auth.SignedInUser(session)
		ctx.User = user
		ctx.IsSigned = ctx != nil

		data["IsSigned"] = true
		data["SignedUser"] = user
		data["SignedUserId"] = user.Id
		data["SignedUserName"] = user.LowerName

		c.Map(ctx)
		c.Map(data)

		c.Next()
	}
}
