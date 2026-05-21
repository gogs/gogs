package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"

	"github.com/flamego/flamego"
	"github.com/go-macaron/session"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

type (
	webAPIUserKey    struct{}
	webAPISessionKey struct{}
	webAPIMacaronKey struct{}
)

func bridgeToWebAPI(webHandler http.Handler) func(c *context.Context) {
	return func(c *context.Context) {
		ctx := c.Req.Context()
		ctx = stdctx.WithValue(ctx, webAPIUserKey{}, c.User)
		ctx = stdctx.WithValue(ctx, webAPISessionKey{}, c.Session)
		ctx = stdctx.WithValue(ctx, webAPIMacaronKey{}, c.Context)
		webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
	}
}

func webAPIInjector(c flamego.Context) {
	user, _ := c.Request().Context().Value(webAPIUserKey{}).(*database.User)
	c.Map(user)
}

func sessionInjector(c flamego.Context) {
	sess, _ := c.Request().Context().Value(webAPISessionKey{}).(session.Store)
	mc, _ := c.Request().Context().Value(webAPIMacaronKey{}).(*macaron.Context)
	c.Map(sess)
	c.Map(mc)
}

func mountWebAPIRoutes(f *flamego.Flame) {
	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		w := c.ResponseWriter()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(statusCode)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		if resp == nil {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})

	f.Group("/api/web", func() {
		f.Group("/user", func() {
			f.Get("/info", webAPIInjector, userInfoHandler)
			f.Post("/sign-out", sessionInjector, userSignOutHandler)
		})
	})
}

type userInfo struct {
	Username              string `json:"username"`
	AvatarURL             string `json:"avatarURL"`
	IsAdmin               bool   `json:"isAdmin"`
	CanCreateOrganization bool   `json:"canCreateOrganization"`
}

func userInfoHandler(user *database.User) (statusCode int, resp *userInfo, err error) {
	if user == nil {
		return http.StatusNoContent, nil, nil
	}
	return http.StatusOK,
		&userInfo{
			Username:              user.Name,
			AvatarURL:             user.AvatarURL(),
			IsAdmin:               user.IsAdmin,
			CanCreateOrganization: user.CanCreateOrganization(),
		},
		nil
}

func userSignOutHandler(sess session.Store, mc *macaron.Context) (statusCode int, resp any, err error) {
	_ = sess.Flush()
	_ = sess.Destory(mc)
	mc.SetCookie(conf.Security.CookieUsername, "", -1, conf.Server.Subpath)
	mc.SetCookie(conf.Security.CookieRememberName, "", -1, conf.Server.Subpath)
	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	return http.StatusNoContent, nil, nil
}
