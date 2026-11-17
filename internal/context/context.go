package context

import (
	"net/http"
)

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

// WebContextKey is the request context key for WebContext values.
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
