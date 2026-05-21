package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"

	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

// webAPIBridgeKey hands off the macaron-resolved user to flamego DI via
// the request context, since macaron and flamego use separate injectors.
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
