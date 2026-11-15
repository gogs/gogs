package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flamego/captcha"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	unknwoni18n "github.com/unknwon/i18n"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
)

type fakeCaptcha struct {
	captcha.Captcha
	text string
}

func (c fakeCaptcha) ValidText(t string) bool { return t == c.text }

func TestPostUserSignIn_Captcha(t *testing.T) {
	conf.SetMockAuth(t, conf.AuthOpts{EnableLoginCaptcha: true})
	// No database: a request that reaches the password check would panic, so
	// passing proves the captcha is checked first.
	before := database.Handle
	database.Handle = nil
	t.Cleanup(func() { database.Handle = before })

	l := i18n.Locale{Locale: unknwoni18n.Locale{Lang: "en-US"}}
	for _, text := range []string{"", "wrong"} {
		statusCode, resp, err := postUserSignIn(
			logx.NewNoopLogger(),
			httptest.NewRequest(http.MethodPost, "/api/web/user/sign-in", nil),
			httptest.NewRecorder(),
			nil,
			nil,
			l,
			fakeCaptcha{text: "right"},
			userSignInRequest{Username: "alice", Password: "password", Captcha: text},
		)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, statusCode)
		body, ok := resp.(*bindingErrorResponse)
		require.True(t, ok)
		assert.Contains(t, body.Fields, "captcha")
	}
}
