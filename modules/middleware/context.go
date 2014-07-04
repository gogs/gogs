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
	"io"
	"net/http"
	"net/url"
	"path/filepath"
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
	"github.com/gogits/gogs/modules/setting"
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
		IsOwner     bool
		IsTrueOwner bool
		IsWatching  bool
		IsBranch    bool
		IsTag       bool
		IsCommit    bool
		HasAccess   bool
		Repository  *models.Repository
		Owner       *models.User
		Commit      *git.Commit
		Tag         *git.Tag
		GitRepo     *git.Repository
		BranchName  string
		TagName     string
		CommitId    string
		RepoLink    string
		CloneLink   struct {
			SSH   string
			HTTPS string
			Git   string
		}
		Mirror *models.Mirror
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
func (ctx *Context) HasApiError() bool {
	hasErr, ok := ctx.Data["HasError"]
	if !ok {
		return false
	}
	return hasErr.(bool)
}

func (ctx *Context) GetErrMsg() string {
	return ctx.Data["ErrorMsg"].(string)
}

// HasError returns true if error occurs in form validation.
func (ctx *Context) HasError() bool {
	hasErr, ok := ctx.Data["HasError"]
	if !ok {
		return false
	}
	ctx.Flash.ErrorMsg = ctx.Data["ErrorMsg"].(string)
	ctx.Data["Flash"] = ctx.Flash
	return hasErr.(bool)
}

// HTML calls render.HTML underlying but reduce one argument.
func (ctx *Context) HTML(status int, name base.TplName, htmlOpt ...HTMLOptions) {
	ctx.Render.HTML(status, string(name), ctx.Data, htmlOpt...)
}

// RenderWithErr used for page has form validation but need to prompt error to users.
func (ctx *Context) RenderWithErr(msg string, tpl base.TplName, form auth.Form) {
	if form != nil {
		auth.AssignForm(form, ctx.Data)
	}
	ctx.Flash.ErrorMsg = msg
	ctx.Data["Flash"] = ctx.Flash
	ctx.HTML(200, tpl)
}

// Handle handles and logs error by given status.
func (ctx *Context) Handle(status int, title string, err error) {
	if err != nil {
		log.Error("%s: %v", title, err)
		if martini.Dev != martini.Prod {
			ctx.Data["ErrorMsg"] = err
		}
	}

	switch status {
	case 404:
		ctx.Data["Title"] = "Page Not Found"
	case 500:
		ctx.Data["Title"] = "Internal Server Error"
	}
	ctx.HTML(status, base.TplName(fmt.Sprintf("status/%d", status)))
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

func (ctx *Context) ServeFile(file string, names ...string) {
	var name string
	if len(names) > 0 {
		name = names[0]
	} else {
		name = filepath.Base(file)
	}
	ctx.Res.Header().Set("Content-Description", "File Transfer")
	ctx.Res.Header().Set("Content-Type", "application/octet-stream")
	ctx.Res.Header().Set("Content-Disposition", "attachment; filename="+name)
	ctx.Res.Header().Set("Content-Transfer-Encoding", "binary")
	ctx.Res.Header().Set("Expires", "0")
	ctx.Res.Header().Set("Cache-Control", "must-revalidate")
	ctx.Res.Header().Set("Pragma", "public")
	http.ServeFile(ctx.Res, ctx.Req, file)
}

func (ctx *Context) ServeContent(name string, r io.ReadSeeker, params ...interface{}) {
	modtime := time.Now()
	for _, p := range params {
		switch v := p.(type) {
		case time.Time:
			modtime = v
		}
	}
	ctx.Res.Header().Set("Content-Description", "File Transfer")
	ctx.Res.Header().Set("Content-Type", "application/octet-stream")
	ctx.Res.Header().Set("Content-Disposition", "attachment; filename="+name)
	ctx.Res.Header().Set("Content-Transfer-Encoding", "binary")
	ctx.Res.Header().Set("Expires", "0")
	ctx.Res.Header().Set("Cache-Control", "must-revalidate")
	ctx.Res.Header().Set("Pragma", "public")
	http.ServeContent(ctx.Res, ctx.Req, name, modtime, r)
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
			Cache:  setting.Cache,
			Render: rd,
		}

		ctx.Data["PageStartTime"] = time.Now()

		// start session
		ctx.Session = setting.SessionManager.SessionStart(res, r)

		// Get flash.
		values, err := url.ParseQuery(ctx.GetCookie("gogs_flash"))
		if err != nil {
			log.Error("InitContext.ParseQuery(flash): %v", err)
		} else if len(values) > 0 {
			ctx.Flash = &Flash{Values: values}
			ctx.Flash.ErrorMsg = ctx.Flash.Get("error")
			ctx.Flash.SuccessMsg = ctx.Flash.Get("success")
			ctx.Data["Flash"] = ctx.Flash
			ctx.SetCookie("gogs_flash", "", -1)
		}
		ctx.Flash = &Flash{Values: url.Values{}}

		rw := res.(martini.ResponseWriter)
		rw.Before(func(martini.ResponseWriter) {
			ctx.Session.SessionRelease(res)

			if flash := ctx.Flash.Encode(); len(flash) > 0 {
				ctx.SetCookie("gogs_flash", ctx.Flash.Encode(), 0)
			}
		})

		// Get user from session if logined.
		user := auth.SignedInUser(ctx.req.Header, ctx.Session)
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
