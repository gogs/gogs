package context

import (
	stdctx "context"
	"net/http"
	"strings"

	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/template"
)

// Context represents context of a request.
type Context struct {
	*macaron.Context
	Session session.Store

	Link        string // Current request URL
	User        *database.User
	IsLogged    bool
	IsBasicAuth bool
	IsTokenAuth bool

	webHandler http.Handler
}

// RawRedirect simply calls underlying Redirect method with no escape.
func (c *Context) RawRedirect(location string, status ...int) {
	c.Context.Redirect(location, status...)
}

// Redirect responses redirection with given location and status.
// It escapes special characters in the location string.
func (c *Context) Redirect(location string, status ...int) {
	c.Context.Redirect(template.EscapePound(location), status...)
}

// RedirectSubpath responses redirection with given location and status.
// It prepends setting.Server.Subpath to the location string.
func (c *Context) RedirectSubpath(location string, status ...int) {
	c.Redirect(conf.Server.Subpath+location, status...)
}

// WebContext carries per-request inputs into the web handler so it can
// render the React shell. Fields are read by helpers like WebContextFrom.
type WebContext struct {
	Lang       string
	SubURL     string
	StatusCode int
	// Page is the page the shell shows in place of the one at the path, e.g.,
	// WebPageServerError.
	Page string
	// Error is the reason of a server error, for viewers allowed to see it.
	Error string
	// Version is the version of Gogs, for site admins on a server error.
	Version string
}

// Pages the server shows through the shell regardless of the path.
const (
	WebPageServerError   = "serverError"
	WebPageProhibitLogin = "prohibitLogin"
)

// WebContextKey is the request context key for WebContext values. Exported
// so callers outside this package (e.g. the web NotFound handler) can attach
// a WebContext when the request bypasses Contexter.
type WebContextKey struct{}

// WebContextFrom returns the WebContext attached to r, or a zero value with
// sensible defaults when nothing was attached.
func WebContextFrom(r *http.Request) WebContext {
	wr, ok := r.Context().Value(WebContextKey{}).(WebContext)
	if !ok {
		return WebContext{Lang: "en-US"}
	}
	if wr.Lang == "" {
		wr.Lang = "en-US"
	}
	return wr
}

// NotFound renders the React 404 page through the web handler with a 404
// status.
func (c *Context) NotFound() {
	c.serveWeb(WebContext{
		Lang:       c.Language(),
		SubURL:     conf.Server.Subpath,
		StatusCode: http.StatusNotFound,
	})
}

// ServeWeb delegates the current request to the web handler. The web frontend
// decides what to render based on the request path.
func (c *Context) ServeWeb() {
	c.serveWeb(WebContext{
		Lang:   c.Language(),
		SubURL: conf.Server.Subpath,
	})
}

// UserKey is the request context key for the signed-in user (*database.User) of
// a request handed to the web handler.
type UserKey struct{}

func (c *Context) serveWeb(wr WebContext) {
	ctx := stdctx.WithValue(c.Req.Context(), WebContextKey{}, wr)
	// The web handler checks what the viewer may see, e.g., when a page that is
	// not found falls through to it.
	ctx = stdctx.WithValue(ctx, UserKey{}, c.User)
	c.webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
}

// Error renders the 500 page.
func (c *Context) Error(err error, msg string) {
	log.ErrorDepth(4, "%s: %v", msg, err)

	wc := WebContext{
		Lang:       c.Language(),
		SubURL:     conf.Server.Subpath,
		StatusCode: http.StatusInternalServerError,
		Page:       WebPageServerError,
	}
	// Only in non-production mode or admin can see the actual error message.
	if !conf.IsProdMode() || (c.IsLogged && c.User.IsAdmin) {
		wc.Error = err.Error()
	}
	if c.IsLogged && c.User.IsAdmin {
		wc.Version = conf.App.Version
	}
	c.serveWeb(wc)
}

// Contexter initializes a classic context for a request. webHandler
// receives 404 responses so the React frontend can render its own 404 page.
func Contexter(store Store, webHandler http.Handler) macaron.Handler {
	return func(ctx *macaron.Context, l i18n.Locale, sess session.Store) {
		c := &Context{
			Context:    ctx,
			Session:    sess,
			Link:       conf.Server.Subpath + strings.TrimSuffix(ctx.Req.URL.Path, "/"),
			webHandler: webHandler,
		}

		if len(conf.HTTP.AccessControlAllowOrigin) > 0 {
			c.Header().Set("Access-Control-Allow-Origin", conf.HTTP.AccessControlAllowOrigin)
			c.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Header().Set("Access-Control-Max-Age", "3600")
			c.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")
		}

		// Get user from session or header when possible
		c.User, c.IsBasicAuth, c.IsTokenAuth = authenticatedUser(store, c.Context, c.Session)

		c.IsLogged = c.User != nil

		// If request sends files, parse them here otherwise the Query() can't be parsed.
		if c.Req.Method == "POST" && strings.Contains(c.Req.Header.Get("Content-Type"), "multipart/form-data") {
			if err := c.Req.ParseMultipartForm(conf.Attachment.MaxSize << 20); err != nil && !strings.Contains(err.Error(), "EOF") { // 32MB max size
				c.Error(err, "parse multipart form")
				return
			}
		}

		log.Trace("Session ID: %s", sess.ID())

		// 🚨 SECURITY: Prevent MIME type sniffing in some browsers,
		// see https://github.com/gogs/gogs/issues/5397 for details.
		c.Header().Set("X-Content-Type-Options", "nosniff")
		c.Header().Set("X-Frame-Options", "deny")

		ctx.Map(c)
	}
}
