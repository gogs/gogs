package integrationtests

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
)

func TestSignUp(t *testing.T) {
	t.Parallel()

	signUp := func(c *client, name, antiBotPayload string) (int, []byte) {
		return c.do(http.MethodPost, "/api/web/user/sign-up", map[string]any{
			"userName":       name,
			"email":          name + "@example.com",
			"password":       "password",
			"antiBotPayload": antiBotPayload,
		})
	}
	assertNotCreated := func(t *testing.T, name string) {
		_, err := database.Handle.Users().GetByUsername(t.Context(), name)
		require.True(t, database.IsErrUserNotExist(err), "user created with a wrong anti-bot payload: %v", err)
	}
	assertCreated := func(t *testing.T, name string) {
		u, err := database.Handle.Users().GetByUsername(t.Context(), name)
		require.NoError(t, err)
		assert.True(t, u.IsActive)
	}

	t.Run("altcha", func(t *testing.T) {
		t.Parallel()

		// The server with FORCE_PRIVATE uses altcha.
		c := newClient(t)
		c.baseURL = forcePrivateServerURL
		name := UniqueName("signup-")
		status, body := signUp(c, name, "")
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNotCreated(t, name)

		solution := solveAltcha(t, c)
		status, body = signUp(c, name, solution)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assertCreated(t, name)

		// A solution is accepted only once.
		other := UniqueName("signup-")
		status, body = signUp(c, other, solution)
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNotCreated(t, other)
	})

	t.Run("captcha", func(t *testing.T) {
		t.Parallel()

		c := newClient(t)
		name := UniqueName("signup-")
		status, body := signUp(c, name, c.captcha()+"x")
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNotCreated(t, name)

		status, body = signUp(c, name, c.captcha())
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assertCreated(t, name)
	})
}

func TestSignInAntiBotVerification(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	// signIn returns the fields with errors, or nil when authenticated.
	signIn := func(t *testing.T, c *client, password, payload string) map[string]*string {
		status, body := c.postSignIn(map[string]any{"username": u.Name, "password": password, "antiBotPayload": payload})
		if status == http.StatusOK {
			return nil
		}
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		var resp struct {
			Fields map[string]*string `json:"fields"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
		require.NotNil(t, resp.Fields, "response: %s", body)
		return resp.Fields
	}
	antiBotVerification := func(c *client) string {
		var resp struct {
			AntiBotVerification string `json:"antiBotVerification"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/sign-in", nil, http.StatusOK, &resp)
		return resp.AntiBotVerification
	}
	// assertRejected asserts that the verification failed and said nothing about
	// the password.
	assertRejected := func(t *testing.T, c *client, fields map[string]*string, msgAndArgs ...any) {
		assert.Contains(t, fields, "antiBotPayload", msgAndArgs...)
		assert.NotContains(t, fields, "password")
		assert.Empty(t, c.username())
	}

	t.Run("captcha", func(t *testing.T) {
		t.Parallel()

		c := newClient(t)
		assert.Equal(t, "captcha", antiBotVerification(c))

		assertRejected(t, c, signIn(t, c, u.Password, ""), "authenticated without a captcha")
		assertRejected(t, c, signIn(t, c, u.Password, c.captcha()+"x"), "authenticated with a wrong captcha")

		captcha := c.captcha()
		fields := signIn(t, c, "wrong-password", captcha)
		assert.Contains(t, fields, "password")
		assert.NotContains(t, fields, "antiBotPayload")
		// The failed attempt used up the captcha.
		assertRejected(t, c, signIn(t, c, u.Password, captcha), "authenticated with a used captcha")

		require.Nil(t, signIn(t, c, u.Password, c.captcha()))
		assert.Equal(t, u.Name, c.username())
	})

	t.Run("altcha", func(t *testing.T) {
		t.Parallel()

		// The server with FORCE_PRIVATE uses altcha.
		c := newClient(t)
		c.baseURL = forcePrivateServerURL
		assert.Equal(t, "altcha", antiBotVerification(c))

		assertRejected(t, c, signIn(t, c, u.Password, ""), "authenticated without a solution")

		solution := solveAltcha(t, c)
		fields := signIn(t, c, "wrong-password", solution)
		assert.Contains(t, fields, "password")
		assert.NotContains(t, fields, "antiBotPayload")
		// The failed attempt used up the solution.
		assertRejected(t, c, signIn(t, c, u.Password, solution), "authenticated with a used solution")

		require.Nil(t, signIn(t, c, u.Password, solveAltcha(t, c)))
		assert.Equal(t, u.Name, c.username())
	})

	t.Run("API", func(t *testing.T) {
		t.Parallel()

		// The API still accepts the password without verification.
		for _, baseURL := range []string{serverURL, forcePrivateServerURL} {
			api := apiV1BasicClient(t, u.Name, u.Password)
			api.baseURL = baseURL
			status, body := api.do(http.MethodGet, "/api/v1/users/"+u.Name+"/tokens", nil)
			assert.Equal(t, http.StatusOK, status, "%s, response: %s", baseURL, body)
		}
	})
}

func TestSignInWithTwoFactor(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	tf, err := EnableTwoFactor(t.Context(), u.ID)
	require.NoError(t, err)

	signIn := func(c *client) {
		status, body := c.postSignIn(map[string]any{
			"username": u.Name,
			"password": u.Password,
		})
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		var resp struct {
			MFA bool `json:"mfa"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
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

	t.Run("password changed", func(t *testing.T) {
		c := newClient(t)
		signIn(c)
		newPassword := UniqueName("password-")
		require.NoError(t, database.Handle.Users().Update(t.Context(), u.ID, database.UpdateUserOptions{Password: &newPassword}))
		u.Password = newPassword

		code := tf.RecoveryCodes[1]
		status, body := c.do(http.MethodPost, "/api/web/user/mfa/recovery", map[string]any{"recoveryCode": code})
		require.Equal(t, http.StatusUnauthorized, status, "signed in by the password before, response: %s", body)
		assert.Empty(t, c.username())

		// The recovery code is not used up, so it still signs in.
		signIn(c)
		status, body = c.do(http.MethodPost, "/api/web/user/mfa/recovery", map[string]any{"recoveryCode": code})
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, u.Name, c.username())
	})
}

// 🚨 SECURITY: A username with a path separator maps onto the directory of
// another user's repositories, e.g., "x/../victim", so an account that owns it
// could delete them.
func TestSignInWithReverseProxy(t *testing.T) {
	t.Parallel()

	victim := seedUser(t, CreateUserOptions{})
	signIn := func(name string) *client {
		c := newClient(t)
		c.header.Set("X-WEBAUTH-USER", name)
		return c
	}

	assert.Equal(t, victim.Name, signIn(victim.Name).username())

	// Names from the proxy are not limited to the characters of the sign-up form.
	name := UniqueName("proxy-") + "@corp.com"
	assert.Equal(t, name, signIn(name).username())
	_, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)

	for _, name := range []string{"x/../" + victim.Name, `x\..\` + victim.Name} {
		assert.Empty(t, signIn(name).username(), "name %q", name)
		_, err := database.Handle.Users().GetByUsername(t.Context(), name)
		assert.True(t, database.IsErrUserNotExist(err), "user %q created: %v", name, err)
	}
}

// 🚨 SECURITY: See TestSignInWithReverseProxy.
func TestSignInWithLoginSource(t *testing.T) {
	t.Parallel()

	source, err := database.Handle.LoginSources().Create(t.Context(), database.CreateLoginSourceOptions{
		Type:      auth.SMTP,
		Name:      UniqueName("source-"),
		Activated: true,
		Config: &smtp.Config{
			Auth: smtp.Plain,
			Host: "127.0.0.1",
			Port: startSMTPStub(t),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(context.Background(), source.ID) })

	// The SMTP source names the new account after the part of the login before
	// "@".
	signIn := func(login string) (int, []byte) {
		return newClient(t).postSignIn(map[string]any{
			"username":    login,
			"password":    "password",
			"loginSource": source.ID,
		})
	}

	name := UniqueName("smtp-")
	status, body := signIn(name + "@corp.com")
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	u, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)
	assert.Equal(t, source.ID, u.LoginSource)

	victim := seedUser(t, CreateUserOptions{})
	name = "x/../" + victim.Name
	status, body = signIn(name + "@corp.com")
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	_, err = database.Handle.Users().GetByUsername(t.Context(), name)
	assert.True(t, database.IsErrUserNotExist(err), "user %q created: %v", name, err)
}

// startSMTPStub starts an SMTP server that accepts every login with any
// password, and returns its port.
func startSMTPStub(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				c := textproto.NewConn(conn)
				defer func() { _ = c.Close() }()
				_ = c.PrintfLine("220 localhost")
				for {
					line, err := c.ReadLine()
					if err != nil {
						return
					}
					verb, _, _ := strings.Cut(line, " ")
					switch strings.ToUpper(verb) {
					case "EHLO":
						_ = c.PrintfLine("250-localhost")
						_ = c.PrintfLine("250 AUTH PLAIN")
					case "AUTH":
						_ = c.PrintfLine("235 Authentication successful")
					default:
						_ = c.PrintfLine("221 Bye")
						return
					}
				}
			}()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestActivateAccount(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{Inactive: true})

	// A link that confirms the address of the account does not activate it.
	stored := userGet(t, u.ID)
	confirmEmailCode := userx.GenerateConfirmEmailCode(stored.ID, stored.Email, stored.LowerName, stored.Password, stored.Rands)
	status, body := newClient(t).do(http.MethodPost, "/api/web/user/activate/complete", map[string]any{"code": confirmEmailCode})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	c := newClient(t)
	status, body = c.do(http.MethodPost, "/api/web/user/activate/complete", map[string]any{"code": u.ActivationCode})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, u.Name, c.username())

	got, err := database.Handle.Users().GetByID(t.Context(), u.ID)
	require.NoError(t, err)
	assert.True(t, got.IsActive)

	// Activation regenerates the salt the code is derived from.
	status, body = newClient(t).do(http.MethodPost, "/api/web/user/activate/complete", map[string]any{"code": u.ActivationCode})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
}
