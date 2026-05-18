package context

import (
	"net/http"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
)

// rememberMeSessionKey is the session key used to mark a session as
// "remember me", so the session cookie can be persisted across browser
// restarts on subsequent requests.
const rememberMeSessionKey = "rememberMe"

// RefreshRememberMeCookie re-emits the session cookie with a long Max-Age so
// the session survives browser restarts for the configured number of days.
//
// The session cookie value (the session ID) is not changed; only its Max-Age
// is extended. Macaron only sets the session cookie when a new session is
// created, so on subsequent requests we set it ourselves to slide the
// expiration forward.
func RefreshRememberMeCookie(ctx *macaron.Context) {
	days := conf.Security.LoginRememberDays
	if days <= 0 {
		return
	}
	sid := ctx.GetCookie(conf.Session.CookieName)
	if sid == "" {
		return
	}
	http.SetCookie(ctx.Resp, &http.Cookie{
		Name:     conf.Session.CookieName,
		Value:    sid,
		Path:     conf.Server.Subpath,
		MaxAge:   86400 * days,
		HttpOnly: true,
		Secure:   conf.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
