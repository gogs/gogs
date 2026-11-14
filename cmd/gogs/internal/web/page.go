package web

import (
	stdctx "context"
	"net/http"
	"net/url"
	"strings"

	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/template"
)

// pageContext is the request context of the page routes. Their middleware
// checks what the viewer may see, then the route hands the request to the web
// app, which renders the page.
type pageContext struct {
	flamego.Context

	// User is the signed-in user, or nil for anonymous visitors.
	User *database.User
	// Repo is the repository of the page, set by withPageRepo.
	Repo *pageRepo
	// Org is the organization of the page, set by withPageOrg.
	Org *pageOrg

	locale i18n.Locale
	webApp http.Handler
}

// newPageContexter returns the middleware that maps *pageContext. The web app
// renders the pages.
func newPageContexter(webApp http.Handler) flamego.Handler {
	return func(c flamego.Context, user *database.User, l i18n.Locale) {
		c.Map(&pageContext{
			Context: c,
			User:    user,
			locale:  l,
			webApp:  webApp,
		})
	}
}

// IsLogged returns true if the viewer is signed in.
func (c *pageContext) IsLogged() bool {
	return c.User != nil
}

// UserID returns the ID of the signed-in user, or 0 for anonymous visitors.
func (c *pageContext) UserID() int64 {
	if c.User == nil {
		return 0
	}
	return c.User.ID
}

func (c *pageContext) serveWeb(wc context.WebContext) {
	wc.Lang = c.locale.Language()
	wc.SubURL = conf.Server.Subpath
	r := c.Request().Request
	ctx := stdctx.WithValue(r.Context(), context.WebContextKey{}, wc)
	// The web app checks what the viewer may see, e.g., when a page that is not
	// found falls through to it.
	ctx = stdctx.WithValue(ctx, context.UserKey{}, c.User)
	c.webApp.ServeHTTP(c.ResponseWriter(), r.WithContext(ctx))
}

// ServeWeb renders the page at the request path.
func (c *pageContext) ServeWeb() {
	c.serveWeb(context.WebContext{})
}

// NotFound renders the page that is not found, with a 404 status.
func (c *pageContext) NotFound() {
	c.serveWeb(context.WebContext{StatusCode: http.StatusNotFound})
}

// ServerError renders the page of a server error, with a 500 status. Callers
// log the error.
func (c *pageContext) ServerError(err error) {
	wc := context.WebContext{
		StatusCode: http.StatusInternalServerError,
		Page:       context.WebPageServerError,
	}
	// Only in non-production mode or admin can see the actual error message.
	if !conf.IsProdMode() || (c.IsLogged() && c.User.IsAdmin) {
		wc.Error = err.Error()
	}
	if c.IsLogged() && c.User.IsAdmin {
		wc.Version = conf.App.Version
	}
	c.serveWeb(wc)
}

// Redirect redirects to the location, with "#" escaped like the
// server-rendered pages did.
func (c *pageContext) Redirect(location string) {
	c.Context.Redirect(template.EscapePound(location))
}

// redirectToSignIn sends the viewer to sign in, and back to the current page
// afterwards.
func (c *pageContext) redirectToSignIn() {
	c.Context.Redirect(conf.Server.Subpath + "/user/sign-in?redirect_to=" + url.QueryEscape(conf.Server.Subpath+c.Request().URL.RequestURI()))
}

// pageAccess turns away the viewers a page is not for: users prohibited from
// signing in, users who have not confirmed their email when that is required,
// and, when signInRequired is true, anonymous visitors. Anonymous visitors of
// the home page go to the landing page.
func pageAccess(signInRequired bool) flamego.Handler {
	return func(c *pageContext) {
		// Frontend assets can match the page routes. Serve them to the viewers this
		// turns away, because the pages they are sent to (sign-in, account
		// activation) are built from them. Everyone else goes through the normal
		// routes, which keeps repositories of owners named like an asset path
		// reachable and missing assets a 404.
		if isWebAssetPath(c.Request().URL.Path) && turnsAway(c, signInRequired) {
			c.ServeWeb()
			return
		}

		if c.IsLogged() && c.User.ProhibitLogin {
			c.serveWeb(context.WebContext{Page: context.WebPageProhibitLogin})
			return
		}

		// The activation page offers to resend the email and shows the status.
		if c.IsLogged() && !c.User.IsActive && conf.Auth.RequireEmailConfirmation {
			c.Redirect(conf.Server.Subpath + "/user/activate")
			return
		}

		if !c.IsLogged() && c.Request().RequestURI == "/" && conf.Server.LandingURL != "/" {
			c.Redirect(conf.Server.Subpath + conf.Server.LandingURL)
			return
		}

		if signInRequired && !c.IsLogged() {
			c.redirectToSignIn()
		}
	}
}

// isWebAssetPath returns true for the paths of frontend assets, including the
// module paths of the Vite dev server.
func isWebAssetPath(p string) bool {
	return strings.HasPrefix(p, "/assets/") ||
		strings.HasPrefix(p, "/img/") ||
		strings.HasPrefix(p, "/@") ||
		strings.HasPrefix(p, "/src/") ||
		strings.HasPrefix(p, "/node_modules/")
}

// turnsAway returns true if pageAccess denies the viewer the requested page.
func turnsAway(c *pageContext, signInRequired bool) bool {
	if c.IsLogged() {
		return c.User.IsDisabled()
	}
	return signInRequired
}

// requirePageAdmin turns away users who are not site admins. Mount it after
// pageAccess(true).
func requirePageAdmin(c *pageContext) {
	if !c.User.IsAdmin {
		c.ResponseWriter().WriteHeader(http.StatusForbidden)
	}
}
