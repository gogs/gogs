package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

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
	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		c.ResponseWriter().Header().Set("Content-Type", "application/json; charset=utf-8")
		c.ResponseWriter().Header().Set("Cache-Control", "no-store")
		c.ResponseWriter().WriteHeader(statusCode)
		if err != nil {
			resp = map[string]any{
				"error": err.Error(),
			}
		}
		_ = json.NewEncoder(c.ResponseWriter()).Encode(resp)
	})

	f.Group("/api/web", func() {
		f.Get("/user-info", userInfoHandler)
	}, webAPIInjector)
}

type userInfo struct {
	Username              string `json:"username"`
	AvatarURL             string `json:"avatarURL"`
	IsAdmin               bool   `json:"isAdmin"`
	CanCreateOrganization bool   `json:"canCreateOrganization"`
}

func userInfoHandler(user *database.User) (statusCode int, resp *userInfo, err error) {
	if user == nil {
		return http.StatusUnauthorized, nil, errors.New("unauthorized")
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
