package integrationtests

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/textproto"
	"slices"
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

// captchaSessionKey is where the captcha middleware keeps the text of the last
// captcha image it served.
const captchaSessionKey = "flamego::captcha::text"

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

		c := newClient(t)
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

		// The server with FORCE_PRIVATE uses the image captcha.
		c := newClient(t)
		c.baseURL = forcePrivateServerURL
		newCaptcha := func() string {
			status, _ := c.do(http.MethodGet, "/-/captcha/image.png", nil)
			require.Equal(t, http.StatusOK, status)
			text, ok := c.sessionValue(captchaSessionKey).(string)
			require.True(t, ok, "no captcha text in session")
			return text
		}

		name := UniqueName("signup-")
		status, body := signUp(c, name, newCaptcha()+"x")
		require.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNotCreated(t, name)

		status, body = signUp(c, name, newCaptcha())
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assertCreated(t, name)
	})
}

// solveAltcha fetches an altcha challenge from the server and solves it like
// the browser does, by trying counters until the derived key starts with the
// key prefix. It returns the payload to submit.
func solveAltcha(t *testing.T, c *client) string {
	t.Helper()

	var challenge struct {
		Parameters struct {
			Algorithm string `json:"algorithm"`
			Cost      int    `json:"cost"`
			KeyLength int    `json:"keyLength"`
			KeyPrefix string `json:"keyPrefix"`
			Nonce     string `json:"nonce"`
			Salt      string `json:"salt"`
		} `json:"parameters"`
		Signature string `json:"signature"`
	}
	var raw json.RawMessage
	c.doJSON(http.MethodGet, "/-/altcha/challenge.json", nil, http.StatusOK, &raw)
	require.NoError(t, json.Unmarshal(raw, &challenge))
	p := challenge.Parameters
	require.Equal(t, "PBKDF2/SHA-256", p.Algorithm)

	nonce, err := hex.DecodeString(p.Nonce)
	require.NoError(t, err)
	salt, err := hex.DecodeString(p.Salt)
	require.NoError(t, err)
	for counter := uint32(0); counter < math.MaxUint32; counter++ {
		password := binary.BigEndian.AppendUint32(slices.Clone(nonce), counter)
		key, err := pbkdf2.Key(sha256.New, string(password), salt, p.Cost, p.KeyLength)
		require.NoError(t, err)
		derivedKey := hex.EncodeToString(key)
		if !strings.HasPrefix(derivedKey, p.KeyPrefix) {
			continue
		}

		payload, err := json.Marshal(map[string]any{
			"challenge": raw,
			"solution":  map[string]any{"counter": counter, "derivedKey": derivedKey},
		})
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(payload)
	}
	t.Fatal("no solution found")
	return ""
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
		return newClient(t).do(http.MethodPost, "/api/web/user/sign-in", map[string]any{
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
	confirmEmailCode := userx.GenerateConfirmEmailCode(cfg.Security.SecretKey, cfg.Auth.ActivateCodeLives, stored.ID, stored.Email, stored.LowerName, stored.Password, stored.Rands)
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
