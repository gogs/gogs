// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-martini/martini"

	"github.com/gogits/cache"
	"github.com/gogits/git"
	"github.com/gogits/session"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

// Context represents context of a request.
type Context struct {
	*Render
	c        martini.Context
	p        martini.Params
	Req      *http.Request
	Res      http.ResponseWriter
	Flash    *Flash
	Session  session.SessionStore
	Cache    cache.Cache
	User     *models.User
	IsSigned bool

	csrfToken string

	Repo struct {
		IsOwner    bool
		IsWatching bool
		IsBranch   bool
		IsTag      bool
		IsCommit   bool
		Repository *models.Repository
		Owner      *models.User
		Commit     *git.Commit
		GitRepo    *git.Repository
		BranchName string
		CommitId   string
		RepoLink   string
		CloneLink  struct {
			SSH   string
			HTTPS string
			Git   string
		}
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
	ctx.Flash.Error(ctx.Data["ErrorMsg"].(string))
	return hasErr.(bool)
}

// HTML calls render.HTML underlying but reduce one argument.
func (ctx *Context) HTML(status int, name string, htmlOpt ...HTMLOptions) {
	ctx.Render.HTML(status, name, ctx.Data, htmlOpt...)
}

// RenderWithErr used for page has form validation but need to prompt error to users.
func (ctx *Context) RenderWithErr(msg, tpl string, form auth.Form) {
	ctx.Flash.Error(msg)
	if form != nil {
		auth.AssignForm(form, ctx.Data)
	}
	ctx.HTML(200, tpl)
}

// Handle handles and logs error by given status.
func (ctx *Context) Handle(status int, title string, err error) {
	log.Error("%s: %v", title, err)
	if martini.Dev == martini.Prod {
		ctx.HTML(500, "status/500")
		return
	}

	ctx.Data["ErrorMsg"] = err
	ctx.HTML(status, fmt.Sprintf("status/%d", status))
}

func (ctx *Context) Debug(msg string, args ...interface{}) {
	log.Debug(msg, args...)
}

func (ctx *Context) GetCookie(name string) string {
	cookie, err := ctx.Req.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (ctx *Context) SetCookie(name string, value string, others ...interface{}) {
	cookie := http.Cookie{}
	cookie.Name = name
	cookie.Value = value

	if len(others) > 0 {
		switch v := others[0].(type) {
		case int:
			cookie.MaxAge = v
		case int64:
			cookie.MaxAge = int(v)
		case int32:
			cookie.MaxAge = int(v)
		}
	}

	// default "/"
	if len(others) > 1 {
		if v, ok := others[1].(string); ok && len(v) > 0 {
			cookie.Path = v
		}
	} else {
		cookie.Path = "/"
	}

	// default empty
	if len(others) > 2 {
		if v, ok := others[2].(string); ok && len(v) > 0 {
			cookie.Domain = v
		}
	}

	// default empty
	if len(others) > 3 {
		switch v := others[3].(type) {
		case bool:
			cookie.Secure = v
		default:
			if others[3] != nil {
				cookie.Secure = true
			}
		}
	}

	// default false. for session cookie default true
	if len(others) > 4 {
		if v, ok := others[4].(bool); ok && v {
			cookie.HttpOnly = true
		}
	}

	ctx.Res.Header().Add("Set-Cookie", cookie.String())
}

// Get secure cookie from request by a given key.
func (ctx *Context) GetSecureCookie(Secret, key string) (string, bool) {
	val := ctx.GetCookie(key)
	if val == "" {
		return "", false
	}

	parts := strings.SplitN(val, "|", 3)

	if len(parts) != 3 {
		return "", false
	}

	vs := parts[0]
	timestamp := parts[1]
	sig := parts[2]

	h := hmac.New(sha1.New, []byte(Secret))
	fmt.Fprintf(h, "%s%s", vs, timestamp)

	if fmt.Sprintf("%02x", h.Sum(nil)) != sig {
		return "", false
	}
	res, _ := base64.URLEncoding.DecodeString(vs)
	return string(res), true
}

// Set Secure cookie for response.
func (ctx *Context) SetSecureCookie(Secret, name, value string, others ...interface{}) {
	vs := base64.URLEncoding.EncodeToString([]byte(value))
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	h := hmac.New(sha1.New, []byte(Secret))
	fmt.Fprintf(h, "%s%s", vs, timestamp)
	sig := fmt.Sprintf("%02x", h.Sum(nil))
	cookie := strings.Join([]string{vs, timestamp, sig}, "|")
	ctx.SetCookie(name, cookie, others...)
}

func (ctx *Context) CsrfToken() string {
	if len(ctx.csrfToken) > 0 {
		return ctx.csrfToken
	}

	token := ctx.GetCookie("_csrf")
	if len(token) == 0 {
		token = base.GetRandomString(30)
		ctx.SetCookie("_csrf", token)
	}
	ctx.csrfToken = token
	return token
}

func (ctx *Context) CsrfTokenValid() bool {
	token := ctx.Query("_csrf")
	if token == "" {
		token = ctx.Req.Header.Get("X-Csrf-Token")
	}
	if token == "" {
		return false
	} else if ctx.csrfToken != token {
		return false
	}
	return true
}

type Flash struct {
	url.Values
	ErrorMsg, SuccessMsg string
}

func (f *Flash) Error(msg string) {
	f.Set("error", msg)
	f.ErrorMsg = msg
}

func (f *Flash) Success(msg string) {
	f.Set("success", msg)
	f.SuccessMsg = msg
}

// InitContext initializes a classic context for a request.
func InitContext() martini.Handler {
	return func(res http.ResponseWriter, r *http.Request, c martini.Context, rd *Render) {

		ctx := &Context{
			c: c,
			// p:      p,
			Req:    r,
			Res:    res,
			Cache:  base.Cache,
			Render: rd,
		}

		ctx.Data["PageStartTime"] = time.Now()

		// start session
		ctx.Session = base.SessionManager.SessionStart(res, r)

		ctx.Flash = &Flash{}
		// Get flash.
		values, err := url.ParseQuery(ctx.GetCookie("gogs_flash"))
		if err != nil {
			log.Error("InitContext.ParseQuery(flash): %v", err)
		} else {
			ctx.Flash.Values = values
			ctx.Data["Flash"] = ctx.Flash
		}

		rw := res.(martini.ResponseWriter)
		rw.Before(func(martini.ResponseWriter) {
			ctx.Session.SessionRelease(res)

			if flash := ctx.Flash.Encode(); len(flash) > 0 {
				ctx.SetCookie("gogs_flash", ctx.Flash.Encode(), -1)
			}
		})

		// Get user from session if logined.
		user := auth.SignedInUser(ctx.Session)
		ctx.User = user
		ctx.IsSigned = user != nil

		ctx.Data["IsSigned"] = ctx.IsSigned

		if user != nil {
			ctx.Data["SignedUser"] = user
			ctx.Data["SignedUserId"] = user.Id
			ctx.Data["SignedUserName"] = user.Name
			ctx.Data["IsAdmin"] = ctx.User.IsAdmin
		}

		// get or create csrf token
		ctx.Data["CsrfToken"] = ctx.CsrfToken()
		ctx.Data["CsrfTokenHtml"] = template.HTML(`<input type="hidden" name="_csrf" value="` + ctx.csrfToken + `">`)

		c.Map(ctx)

		c.Next()
	}
}
