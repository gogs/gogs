// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-macaron/cache"
	"github.com/go-macaron/csrf"
	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"github.com/unknwon/com"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/template"
)

// Context represents context of a request.
type Context struct {
	*macaron.Context
	Cache   cache.Cache
	csrf    csrf.CSRF
	Flash   *session.Flash
	Session session.Store

	Link        string // Current request URL
	User        *db.User
	IsLogged    bool
	IsBasicAuth bool
	IsTokenAuth bool

	Repo *Repository
	Org  *Organization
}

// Title sets "Title" field in template data.
func (c *Context) Title(locale string) {
	c.Data["Title"] = c.Tr(locale)
}

// PageIs sets "PageIsxxx" field in template data.
func (c *Context) PageIs(name string) {
	c.Data["PageIs"+name] = true
}

// Require sets "Requirexxx" field in template data.
func (c *Context) Require(name string) {
	c.Data["Require"+name] = true
}

func (c *Context) RequireHighlightJS() {
	c.Require("HighlightJS")
}

func (c *Context) RequireSimpleMDE() {
	c.Require("SimpleMDE")
}

func (c *Context) RequireAutosize() {
	c.Require("Autosize")
}

func (c *Context) RequireDropzone() {
	c.Require("Dropzone")
}

// FormErr sets "Err_xxx" field in template data.
func (c *Context) FormErr(names ...string) {
	for i := range names {
		c.Data["Err_"+names[i]] = true
	}
}

// UserID returns ID of current logged in user.
// It returns 0 if visitor is anonymous.
func (c *Context) UserID() int64 {
	if !c.IsLogged {
		return 0
	}
	return c.User.ID
}

// HasError returns true if error occurs in form validation.
func (c *Context) HasApiError() bool {
	hasErr, ok := c.Data["HasError"]
	if !ok {
		return false
	}
	return hasErr.(bool)
}

func (c *Context) GetErrMsg() string {
	return c.Data["ErrorMsg"].(string)
}

// HasError returns true if error occurs in form validation.
func (c *Context) HasError() bool {
	hasErr, ok := c.Data["HasError"]
	if !ok {
		return false
	}
	c.Flash.ErrorMsg = c.Data["ErrorMsg"].(string)
	c.Data["Flash"] = c.Flash
	return hasErr.(bool)
}

// HasValue returns true if value of given name exists.
func (c *Context) HasValue(name string) bool {
	_, ok := c.Data[name]
	return ok
}

// HTML responses template with given status.
func (c *Context) HTML(status int, name string) {
	log.Trace("Template: %s", name)
	c.Context.HTML(status, name)
}

// Success responses template with status http.StatusOK.
func (c *Context) Success(name string) {
	c.HTML(http.StatusOK, name)
}

// JSONSuccess responses JSON with status http.StatusOK.
func (c *Context) JSONSuccess(data interface{}) {
	c.JSON(http.StatusOK, data)
}

// RawRedirect simply calls underlying Redirect method with no escape.
func (c *Context) RawRedirect(location string, status ...int) {
	c.Context.Redirect(location, status...)
}

// Redirect responses redirection wtih given location and status.
// It escapes special characters in the location string.
func (c *Context) Redirect(location string, status ...int) {
	c.Context.Redirect(template.EscapePound(location), status...)
}

// SubURLRedirect responses redirection wtih given location and status.
// It prepends setting.Server.Subpath to the location string.
func (c *Context) SubURLRedirect(location string, status ...int) {
	c.Redirect(conf.Server.Subpath+location, status...)
}

// RenderWithErr used for page has form validation but need to prompt error to users.
func (c *Context) RenderWithErr(msg, tpl string, f interface{}) {
	if f != nil {
		form.Assign(f, c.Data)
	}
	c.Flash.ErrorMsg = msg
	c.Data["Flash"] = c.Flash
	c.HTML(http.StatusOK, tpl)
}

// Handle handles and logs error by given status.
func (c *Context) Handle(status int, msg string, err error) {
	switch status {
	case http.StatusNotFound:
		c.Data["Title"] = "Page Not Found"
	case http.StatusInternalServerError:
		c.Data["Title"] = "Internal Server Error"
		log.Error("%s: %v", msg, err)
		if !conf.IsProdMode() || (c.IsLogged && c.User.IsAdmin) {
			c.Data["ErrorMsg"] = err
		}
	}
	c.HTML(status, fmt.Sprintf("status/%d", status))
}

// NotFound renders the 404 page.
func (c *Context) NotFound() {
	c.Handle(http.StatusNotFound, "", nil)
}

// ServerError renders the 500 page.
func (c *Context) ServerError(msg string, err error) {
	c.Handle(http.StatusInternalServerError, msg, err)
}

// NotFoundOrServerError use error check function to determine if the error
// is about not found. It responses with 404 status code for not found error,
// or error context description for logging purpose of 500 server error.
func (c *Context) NotFoundOrServerError(msg string, errck func(error) bool, err error) {
	if errck(err) {
		c.NotFound()
		return
	}
	c.ServerError(msg, err)
}

func (c *Context) HandleText(status int, msg string) {
	c.PlainText(status, []byte(msg))
}

func (c *Context) ServeContent(name string, r io.ReadSeeker, params ...interface{}) {
	modtime := time.Now()
	for _, p := range params {
		switch v := p.(type) {
		case time.Time:
			modtime = v
		}
	}
	c.Resp.Header().Set("Content-Description", "File Transfer")
	c.Resp.Header().Set("Content-Type", "application/octet-stream")
	c.Resp.Header().Set("Content-Disposition", "attachment; filename="+name)
	c.Resp.Header().Set("Content-Transfer-Encoding", "binary")
	c.Resp.Header().Set("Expires", "0")
	c.Resp.Header().Set("Cache-Control", "must-revalidate")
	c.Resp.Header().Set("Pragma", "public")
	http.ServeContent(c.Resp, c.Req.Request, name, modtime, r)
}

// Contexter initializes a classic context for a request.
func Contexter() macaron.Handler {
	return func(ctx *macaron.Context, l i18n.Locale, cache cache.Cache, sess session.Store, f *session.Flash, x csrf.CSRF) {
		c := &Context{
			Context: ctx,
			Cache:   cache,
			csrf:    x,
			Flash:   f,
			Session: sess,
			Link:    conf.Server.Subpath + strings.TrimSuffix(ctx.Req.URL.Path, "/"),
			Repo: &Repository{
				PullRequest: &PullRequest{},
			},
			Org: &Organization{},
		}
		c.Data["Link"] = template.EscapePound(c.Link)
		c.Data["PageStartTime"] = time.Now()

		// Quick responses appropriate go-get meta with status 200
		// regardless of if user have access to the repository,
		// or the repository does not exist at all.
		// This is particular a workaround for "go get" command which does not respect
		// .netrc file.
		if c.Query("go-get") == "1" {
			ownerName := c.Params(":username")
			repoName := c.Params(":reponame")
			branchName := "master"

			owner, err := db.GetUserByName(ownerName)
			if err != nil {
				c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
				return
			}

			repo, err := db.GetRepositoryByName(owner.ID, repoName)
			if err == nil && len(repo.DefaultBranch) > 0 {
				branchName = repo.DefaultBranch
			}

			prefix := conf.Server.ExternalURL + path.Join(ownerName, repoName, "src", branchName)
			insecureFlag := ""
			if !strings.HasPrefix(conf.Server.ExternalURL, "https://") {
				insecureFlag = "--insecure "
			}
			c.PlainText(http.StatusOK, []byte(com.Expand(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="{GoGetImport} git {CloneLink}">
		<meta name="go-source" content="{GoGetImport} _ {GoDocDirectory} {GoDocFile}">
	</head>
	<body>
		go get {InsecureFlag}{GoGetImport}
	</body>
</html>
`, map[string]string{
				"GoGetImport":    path.Join(conf.Server.URL.Host, conf.Server.Subpath, repo.FullName()),
				"CloneLink":      db.ComposeHTTPSCloneURL(ownerName, repoName),
				"GoDocDirectory": prefix + "{/dir}",
				"GoDocFile":      prefix + "{/dir}/{file}#L{line}",
				"InsecureFlag":   insecureFlag,
			})))
			return
		}

		if len(conf.HTTP.AccessControlAllowOrigin) > 0 {
			c.Header().Set("Access-Control-Allow-Origin", conf.HTTP.AccessControlAllowOrigin)
			c.Header().Set("'Access-Control-Allow-Credentials' ", "true")
			c.Header().Set("Access-Control-Max-Age", "3600")
			c.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")
		}

		// Get user from session or header when possible
		c.User, c.IsBasicAuth, c.IsTokenAuth = auth.SignedInUser(c.Context, c.Session)

		if c.User != nil {
			c.IsLogged = true
			c.Data["IsLogged"] = c.IsLogged
			c.Data["LoggedUser"] = c.User
			c.Data["LoggedUserID"] = c.User.ID
			c.Data["LoggedUserName"] = c.User.Name
			c.Data["IsAdmin"] = c.User.IsAdmin
		} else {
			c.Data["LoggedUserID"] = 0
			c.Data["LoggedUserName"] = ""
		}

		// If request sends files, parse them here otherwise the Query() can't be parsed and the CsrfToken will be invalid.
		if c.Req.Method == "POST" && strings.Contains(c.Req.Header.Get("Content-Type"), "multipart/form-data") {
			if err := c.Req.ParseMultipartForm(conf.AttachmentMaxSize << 20); err != nil && !strings.Contains(err.Error(), "EOF") { // 32MB max size
				c.ServerError("ParseMultipartForm", err)
				return
			}
		}

		c.Data["CSRFToken"] = x.GetToken()
		c.Data["CSRFTokenHTML"] = template.Safe(`<input type="hidden" name="_csrf" value="` + x.GetToken() + `">`)
		log.Trace("Session ID: %s", sess.ID())
		log.Trace("CSRF Token: %v", c.Data["CSRFToken"])

		c.Data["ShowRegistrationButton"] = !conf.Auth.DisableRegistration
		c.Data["ShowFooterBranding"] = conf.ShowFooterBranding

		c.renderNoticeBanner()

		ctx.Map(c)
	}
}
