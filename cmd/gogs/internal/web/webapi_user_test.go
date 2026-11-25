package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
)

func TestPostUserSignIn_AntiBotVerification(t *testing.T) {
	conf.SetMockAuth(t, conf.AuthOpts{AntiBotVerificationStrategy: conf.AntiBotVerificationCaptcha, AntiBotVerificationOnLogIn: true})
	// No database: a request that reaches the password check would panic, so
	// passing proves the anti-bot payload is checked first.
	before := database.Handle
	database.Handle = nil
	t.Cleanup(func() { database.Handle = before })

	verify := antiBotVerifier(func(payload string) bool { return payload == "right" })
	for _, payload := range []string{"", "wrong"} {
		statusCode, resp, err := postUserSignIn(
			logx.NewNoopLogger(),
			httptest.NewRequest(http.MethodPost, "/api/web/user/sign-in", nil),
			httptest.NewRecorder(),
			nil,
			nil,
			i18n.Locale{Lang: "en-US"},
			verify,
			userSignInRequest{Username: "alice", Password: "password", AntiBotPayload: payload},
		)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, statusCode)
		body, ok := resp.(*bindingErrorResponse)
		require.True(t, ok)
		assert.Contains(t, body.Fields, "antiBotPayload")
	}
}
