package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	"github.com/stretchr/testify/assert"
	unknwoni18n "github.com/unknwon/i18n"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func TestPageAccess(t *testing.T) {
	originalSubpath, originalConfirmation := conf.Server.Subpath, conf.Auth.RequireEmailConfirmation
	t.Cleanup(func() {
		conf.Server.Subpath, conf.Auth.RequireEmailConfirmation = originalSubpath, originalConfirmation
	})
	conf.Server.Subpath = "/gogs"
	conf.Auth.RequireEmailConfirmation = true

	serve := func(t *testing.T, user *database.User, path string) *httptest.ResponseRecorder {
		t.Helper()
		webApp := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("web app " + context.WebContextFrom(r).Page))
		})
		f := flamego.New()
		f.Use(func(c flamego.Context) {
			c.Map(user, i18n.Locale{Locale: unknwoni18n.Locale{Lang: "en-US"}})
		})
		f.Get("/{owner}/{repo}", newPageContexter(webApp), pageAccess(true), func(c flamego.Context) string {
			return "page"
		})
		resp := httptest.NewRecorder()
		f.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		return resp
	}

	t.Run("anonymous visitor is sent to sign in and back", func(t *testing.T) {
		resp := serve(t, nil, "/alice/repo?type=open&page=2")
		assert.Equal(t, http.StatusFound, resp.Code)
		assert.Equal(t, "/gogs/user/sign-in?redirect_to=%2Fgogs%2Falice%2Frepo%3Ftype%3Dopen%26page%3D2", resp.Header().Get("Location"))
	})

	t.Run("unconfirmed user is sent to activate", func(t *testing.T) {
		resp := serve(t, &database.User{ID: 1, IsActive: false}, "/alice/repo")
		assert.Equal(t, http.StatusFound, resp.Code)
		assert.Equal(t, "/gogs/user/activate", resp.Header().Get("Location"))
	})

	t.Run("user prohibited from signing in sees why", func(t *testing.T) {
		resp := serve(t, &database.User{ID: 1, IsActive: true, ProhibitLogin: true}, "/alice/repo")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "web app "+context.WebPageProhibitLogin, resp.Body.String())
	})

	// Frontend assets match "/{owner}/{repo}" too, and the activation page
	// cannot render without them.
	t.Run("frontend assets are served to an unconfirmed user", func(t *testing.T) {
		for _, path := range []string{"/assets/index.js", "/src/main.tsx"} {
			resp := serve(t, &database.User{ID: 1, IsActive: false}, path)
			assert.Equal(t, http.StatusOK, resp.Code, path)
			assert.Equal(t, "web app ", resp.Body.String(), path)
		}
	})

	// A user or organization named "src" is allowed, so its repositories must stay
	// reachable for viewers who may see them, and a missing asset must stay a 404
	// from the normal routes.
	t.Run("asset paths go through the normal routes for viewers who may see them", func(t *testing.T) {
		for _, path := range []string{"/src/app", "/assets/stale.js"} {
			resp := serve(t, &database.User{ID: 1, IsActive: true}, path)
			assert.Equal(t, http.StatusOK, resp.Code, path)
			assert.Equal(t, "page", resp.Body.String(), path)
		}
	})
}
