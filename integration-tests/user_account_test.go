package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
)

func TestUserSignInPage(t *testing.T) {
	t.Parallel()

	newSource := func(activated bool) *database.LoginSource {
		source, err := database.Handle.LoginSources().Create(t.Context(), database.CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      UniqueName("source-"),
			Activated: activated,
			Config:    &pam.Config{ServiceName: "gogs"},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(t.Context(), source.ID) })
		return source
	}
	active := newSource(true)
	inactive := newSource(false)

	type loginSource struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		IsDefault bool   `json:"isDefault"`
	}
	var resp struct {
		LoginSources []loginSource `json:"loginSources"`
	}
	newClient(t).doJSON(http.MethodGet, "/api/web/user/sign-in", nil, http.StatusOK, &resp)
	var seeded []loginSource
	for _, s := range resp.LoginSources {
		if s.ID == active.ID || s.ID == inactive.ID {
			seeded = append(seeded, s)
		}
	}
	assert.Equal(t, []loginSource{{ID: active.ID, Name: active.Name}}, seeded)
}

func TestUserSignUpPage(t *testing.T) {
	t.Parallel()

	var resp struct {
		RegistrationDisabled bool   `json:"registrationDisabled"`
		AntiBotVerification  string `json:"antiBotVerification"`
	}
	newClient(t).doJSON(http.MethodGet, "/api/web/user/sign-up", nil, http.StatusOK, &resp)
	// The server under test allows registration with anti-bot verification.
	assert.False(t, resp.RegistrationDisabled)
	assert.Equal(t, "altcha", resp.AntiBotVerification)
}

func TestUserResetPassword(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	stored, err := database.Handle.Users().GetByID(t.Context(), u.ID)
	require.NoError(t, err)
	code := userx.GenerateResetPasswordCode(stored.ID, stored.Email, stored.LowerName, stored.Password, stored.Rands)

	type pageResp struct {
		EmailEnabled bool `json:"emailEnabled"`
		Valid        bool `json:"valid"`
	}
	c := newClient(t)
	page := func(query string) pageResp {
		var resp pageResp
		c.doJSON(http.MethodGet, "/api/web/user/reset-password"+query, nil, http.StatusOK, &resp)
		return resp
	}
	assert.Equal(t, pageResp{EmailEnabled: true}, page(""))
	assert.Equal(t, pageResp{EmailEnabled: true}, page("?code="+code+"x"))
	assert.Equal(t, pageResp{EmailEnabled: true, Valid: true}, page("?code="+code))

	type requestResp struct {
		Hours         int  `json:"hours"`
		ResendLimited bool `json:"resendLimited"`
	}
	request := func(email string) requestResp {
		var resp requestResp
		c.doJSON(http.MethodPost, "/api/web/user/reset-password", map[string]any{"email": email}, http.StatusOK, &resp)
		return resp
	}
	// Codes live for 180 minutes by default.
	assert.Equal(t, requestResp{Hours: 3}, request(u.Email))
	assert.Equal(t, requestResp{Hours: 3, ResendLimited: true}, request(u.Email), "mail resent without a limit")
	// An unknown address gets the same answer, so addresses cannot be probed.
	assert.Equal(t, requestResp{Hours: 3}, request(UniqueName("nobody-")+"@example.com"))

	complete := func(code, password string) (int, []byte) {
		return newClient(t).do(http.MethodPost, "/api/web/user/reset-password/complete", map[string]any{"code": code, "password": password})
	}
	status, body := complete(code+"x", "new-password")
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	status, body = complete(code, "new-password")
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	newClient(t).signIn(&User{Name: u.Name, Password: "new-password"})
	status, body = newClient(t).do(http.MethodPost, "/api/web/user/sign-in", map[string]any{"username": u.Name, "password": u.Password})
	assert.Equal(t, http.StatusUnauthorized, status, "old password still works, response: %s", body)

	// The code is derived from the password hash, so it is only used once.
	status, body = complete(code, "another-password")
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
}

func TestUserMFAPage(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	_, err := EnableTwoFactor(t.Context(), u.ID)
	require.NoError(t, err)

	c := newClient(t)
	status, body := c.do(http.MethodGet, "/api/web/user/mfa", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	var resp struct {
		MFA bool `json:"mfa"`
	}
	c.doJSON(http.MethodPost, "/api/web/user/sign-in", map[string]any{"username": u.Name, "password": u.Password}, http.StatusOK, &resp)
	require.True(t, resp.MFA)
	status, body = c.do(http.MethodGet, "/api/web/user/mfa", nil)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
}

func TestUserActivatePage(t *testing.T) {
	t.Parallel()

	t.Run("signed out", func(t *testing.T) {
		c := newClient(t)
		status, body := c.do(http.MethodGet, "/api/web/user/activate", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		status, body = c.do(http.MethodPost, "/api/web/user/activate", map[string]any{})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})

	t.Run("active", func(t *testing.T) {
		c := authenticatedClient(t, seedUser(t, CreateUserOptions{}))
		status, body := c.do(http.MethodGet, "/api/web/user/activate", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = c.do(http.MethodPost, "/api/web/user/activate", map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("inactive", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{Inactive: true})
		c := authenticatedClient(t, u)
		var resp struct {
			Email             string `json:"email"`
			CodeLifetimeHours int    `json:"codeLifetimeHours"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/activate", nil, http.StatusOK, &resp)
		assert.Equal(t, u.Email, resp.Email)
		assert.Equal(t, 3, resp.CodeLifetimeHours)

		// The server under test does not require confirming emails, so it
		// sends no activation mail.
		status, body := c.do(http.MethodPost, "/api/web/user/activate", map[string]any{})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
	})
}

func TestUserSignOut(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, u)
	sessionID := c.sessionID()

	status, body := c.do(http.MethodPost, "/api/web/user/sign-out", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, c.username())
	store, err := sessions.Read(t.Context(), sessionID)
	require.NoError(t, err)
	assert.Nil(t, store.Get("uid"), "session of the signed-out user still signed in")
}

func TestUserDelete(t *testing.T) {
	t.Parallel()

	deleteUser := func(c *client, password string) (int, []byte) {
		return c.do(http.MethodDelete, "/api/web/user", map[string]any{"password": password})
	}

	t.Run("signed out", func(t *testing.T) {
		status, body := deleteUser(newClient(t), "password")
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})

	t.Run("wrong password", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		c := authenticatedClient(t, u)
		status, body := deleteUser(c, u.Password+"x")
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		_, err := database.Handle.Users().GetByID(t.Context(), u.ID)
		assert.NoError(t, err)
	})

	t.Run("owns a repository", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		seedRepository(t, u.ID, CreateRepositoryOptions{})
		c := authenticatedClient(t, u)
		status, body := deleteUser(c, u.Password)
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)
		_, err := database.Handle.Users().GetByID(t.Context(), u.ID)
		assert.NoError(t, err)
	})

	t.Run("deleted", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		c := authenticatedClient(t, u)
		status, body := deleteUser(c, u.Password)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := database.Handle.Users().GetByID(t.Context(), u.ID)
		assert.True(t, database.IsErrUserNotExist(err), "user not deleted: %v", err)
		assert.Empty(t, c.username())
	})
}
