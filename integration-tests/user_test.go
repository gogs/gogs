package integrationtests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestSignUp(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	signUp := func(name, captcha string) (int, []byte) {
		return c.do(http.MethodPost, "/api/web/user/sign-up", map[string]any{
			"userName": name,
			"email":    name + "@example.com",
			"password": "password",
			"captcha":  captcha,
		})
	}

	name := UniqueName("signup-")
	status, body := signUp(name, c.captcha()+"x")
	require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	_, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.True(t, database.IsErrUserNotExist(err), "user created with the wrong captcha: %v", err)

	status, body = signUp(name, c.captcha())
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	u, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)
	assert.True(t, u.IsActive)
}

func TestSignInWithCaptcha(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	c := newClient(t)
	c.baseURL = hardenedServerURL

	captchaEnabled := func(c *client) bool {
		var resp struct {
			CaptchaEnabled bool `json:"captchaEnabled"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/sign-in", nil, http.StatusOK, &resp)
		return resp.CaptchaEnabled
	}
	assert.True(t, captchaEnabled(c))
	assert.False(t, captchaEnabled(newClient(t)))

	signIn := func(password string, captcha *string) map[string]*string {
		req := map[string]any{"username": u.Name, "password": password}
		if captcha != nil {
			req["captcha"] = *captcha
		}
		status, body := c.do(http.MethodPost, "/api/web/user/sign-in", req)
		if status == http.StatusOK {
			return nil
		}
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		var resp struct {
			Fields map[string]*string `json:"fields"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
		return resp.Fields
	}
	ptr := func(s string) *string { return &s }

	fields := signIn(u.Password, nil)
	assert.Contains(t, fields, "captcha", "signed in without a captcha")
	assert.NotContains(t, fields, "password")
	assert.Empty(t, c.username())

	fields = signIn(u.Password, ptr(c.captcha()+"x"))
	assert.Contains(t, fields, "captcha", "signed in with a wrong captcha")
	assert.NotContains(t, fields, "password")
	assert.Empty(t, c.username())

	captcha := c.captcha()
	fields = signIn("wrong-password", &captcha)
	assert.Contains(t, fields, "password")
	assert.NotContains(t, fields, "captcha")

	// The failed attempt used up the captcha.
	fields = signIn(u.Password, &captcha)
	assert.Contains(t, fields, "captcha", "signed in with a used captcha")
	assert.Empty(t, c.username())

	require.Nil(t, signIn(u.Password, ptr(c.captcha())))
	assert.Equal(t, u.Name, c.username())

	// The API still accepts the password without a captcha.
	api := apiV1BasicClient(t, u.Name, u.Password)
	api.baseURL = hardenedServerURL
	status, body := api.do(http.MethodGet, "/api/v1/users/"+u.Name+"/tokens", nil)
	assert.Equal(t, http.StatusOK, status, "response: %s", body)
}

func TestSignInWithTwoFactor(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	tf, err := EnableTwoFactor(t.Context(), u.ID)
	require.NoError(t, err)

	signIn := func(c *client) {
		var resp struct {
			MFA bool `json:"mfa"`
		}
		c.doJSON(http.MethodPost, "/api/web/user/sign-in", map[string]any{
			"username": u.Name,
			"password": u.Password,
		}, http.StatusOK, &resp)
		require.True(t, resp.MFA)
		assert.Equal(t, u.ID, c.sessionValue("mfaUserID"))
		assert.Empty(t, c.username(), "signed in before the second factor")
	}

	t.Run("passcode", func(t *testing.T) {
		c := newClient(t)
		signIn(c)
		passwordSessionID := c.sessionID()

		passcode, err := totp.GenerateCode(tf.Secret, time.Now())
		require.NoError(t, err)
		status, body := c.do(http.MethodPost, "/api/web/user/mfa", map[string]any{"passcode": passcode})
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, u.Name, c.username())
		assert.NotEqual(t, passwordSessionID, c.sessionID(), "session ID not regenerated on sign-in")
		assert.Equal(t, u.ID, c.sessionValue("uid"))
		assert.Nil(t, c.sessionValue("mfaUserID"))

		// A passcode is only accepted once, even in another session.
		other := newClient(t)
		signIn(other)
		status, body = other.do(http.MethodPost, "/api/web/user/mfa", map[string]any{"passcode": passcode})
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, other.username())
	})

	t.Run("recovery code", func(t *testing.T) {
		code := tf.RecoveryCodes[0]
		c := newClient(t)
		signIn(c)
		status, body := c.do(http.MethodPost, "/api/web/user/mfa/recovery", map[string]any{"recoveryCode": code})
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, u.Name, c.username())

		recoveryCodes, err := database.Handle.TwoFactors().ListRecoveryCodes(t.Context(), u.ID)
		require.NoError(t, err)
		for _, rc := range recoveryCodes {
			assert.Equal(t, rc.Code == code, rc.IsUsed, "recovery code %q", rc.Code)
		}

		other := newClient(t)
		signIn(other)
		status, body = other.do(http.MethodPost, "/api/web/user/mfa/recovery", map[string]any{"recoveryCode": code})
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, other.username())
	})
}

func TestActivateAccount(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{Inactive: true})

	c := newClient(t)
	status, body := c.do(http.MethodPost, "/api/web/user/activate/complete", map[string]any{"code": u.ActivationCode})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, u.Name, c.username())

	got, err := database.Handle.Users().GetByID(t.Context(), u.ID)
	require.NoError(t, err)
	assert.True(t, got.IsActive)

	// Activation regenerates the salt the code is derived from.
	status, body = newClient(t).do(http.MethodPost, "/api/web/user/activate/complete", map[string]any{"code": u.ActivationCode})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
}
