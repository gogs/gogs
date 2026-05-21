package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"

	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

type userInfo struct {
	Username              string `json:"username"`
	AvatarURL             string `json:"avatarURL"`
	IsAdmin               bool   `json:"isAdmin"`
	CanCreateOrganization bool   `json:"canCreateOrganization"`
}

type webAPIBridgeKey struct{}

func bridgeToWebAPI(webHandler http.Handler) func(c *context.Context) {
	return func(c *context.Context) {
		ctx := stdctx.WithValue(c.Req.Context(), webAPIBridgeKey{}, c.User)
		webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
	}
}

func webAPIInjector(c flamego.Context) {
	user, _ := c.Request().Context().Value(webAPIBridgeKey{}).(*database.User)
	c.Map(user)
}

func mountWebAPIRoutes(f *flamego.Flame) {
	f.Group("/api/web", func() {
		f.Get("/user-info", userInfoHandler)
	}, webAPIInjector)
}

func userInfoHandler(w http.ResponseWriter, user *database.User) {
	var resp *userInfo
	if user != nil {
		resp = &userInfo{
			Username:              user.Name,
			AvatarURL:             user.AvatarURL(),
			IsAdmin:               user.IsAdmin,
			CanCreateOrganization: user.CanCreateOrganization(),
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}
