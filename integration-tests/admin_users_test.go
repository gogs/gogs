package integrationtests

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/database"
)

// adminSeedAuthSource creates an SMTP authentication source that is not
// active, so that signing in of other tests never tries it, and deletes it
// when the test ends unless the test did.
func adminSeedAuthSource(t *testing.T) *database.LoginSource {
	t.Helper()

	source, err := database.Handle.LoginSources().Create(t.Context(), database.CreateLoginSourceOptions{
		Type: auth.SMTP,
		Name: UniqueName("auth-"),
		Config: &smtp.Config{
			Auth: smtp.Plain,
			Host: "127.0.0.1",
			Port: 1,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(context.Background(), source.ID) })
	return source
}

func TestAdminUsers(t *testing.T) {
	admin, nonAdmin, anonymous := adminClients(t)

	name := UniqueName("admin-created-")
	password := UniqueName("password-")
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	t.Run("create", func(t *testing.T) {
		body := map[string]any{"name": name, "email": name + "@example.com", "password": password}
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPost, "/api/web/admin/users", body)
		_, err := database.Handle.Users().GetByUsername(t.Context(), name)
		require.True(t, database.IsErrUserNotExist(err), "user created by a request that was turned away: %v", err)

		admin.doJSON(http.MethodPost, "/api/web/admin/users", body, http.StatusCreated, &created)
		assert.Equal(t, name, created.Name)
		u, err := database.Handle.Users().GetByUsername(t.Context(), name)
		require.NoError(t, err)
		assert.Equal(t, created.ID, u.ID)
		assert.Equal(t, name+"@example.com", u.Email)
		assert.True(t, u.IsActive)
		assert.False(t, u.IsAdmin)

		c := newClient(t)
		c.signIn(&User{Name: name, Password: password})
		assert.Equal(t, name, c.username())

		status, respBody := admin.do(http.MethodPost, "/api/web/admin/users", body)
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "name")

		// Local users need a password to sign in with.
		other := UniqueName("admin-created-")
		status, respBody = admin.do(http.MethodPost, "/api/web/admin/users", map[string]any{"name": other, "email": other + "@example.com"})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "password")
		_, err = database.Handle.Users().GetByUsername(t.Context(), other)
		assert.True(t, database.IsErrUserNotExist(err), "user created without a password: %v", err)
	})
	require.NotZero(t, created.ID, "user not created")
	path := "/api/web/admin/users/" + strconv.FormatInt(created.ID, 10)

	t.Run("list", func(t *testing.T) {
		listPath := "/api/web/admin/users?q=" + url.QueryEscape(name)
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, listPath, nil)

		type item struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Email    string `json:"email"`
			IsActive bool   `json:"isActive"`
			IsAdmin  bool   `json:"isAdmin"`
		}
		var resp struct {
			Items []item `json:"items"`
			Total int64  `json:"total"`
		}
		admin.doJSON(http.MethodGet, listPath, nil, http.StatusOK, &resp)
		assert.Equal(t, []item{{ID: created.ID, Name: name, Email: name + "@example.com", IsActive: true}}, resp.Items)
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("get", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, path, nil)

		var resp struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Email       string `json:"email"`
			LoginSource int64  `json:"loginSource"`
			IsActive    bool   `json:"isActive"`
			IsAdmin     bool   `json:"isAdmin"`
		}
		admin.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		assert.Equal(t, created.ID, resp.ID)
		assert.Equal(t, name, resp.Name)
		assert.Equal(t, name+"@example.com", resp.Email)
		assert.Zero(t, resp.LoginSource)
		assert.True(t, resp.IsActive)
		assert.False(t, resp.IsAdmin)

		// Organizations have their own settings.
		org := adminSeedOrganization(t, created.ID)
		status, body := admin.do(http.MethodGet, "/api/web/admin/users/"+strconv.FormatInt(org.ID, 10), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = admin.do(http.MethodGet, "/api/web/admin/users/999999999", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("update", func(t *testing.T) {
		newEmail := UniqueName("updated-") + "@example.com"
		newPassword := UniqueName("password-")
		body := map[string]any{
			"email":           newEmail,
			"password":        newPassword,
			"fullName":        "Updated Name",
			"website":         "https://example.com",
			"location":        "Earth",
			"maxRepoCreation": 3,
			"isActive":        true,
			"isAdmin":         true,
			"allowGitHook":    true,
		}
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPatch, path, body)
		u, err := database.Handle.Users().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		require.False(t, u.IsAdmin, "user updated by a request that was turned away")

		status, respBody := admin.do(http.MethodPatch, path, body)
		require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
		u, err = database.Handle.Users().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, newEmail, u.Email)
		assert.Equal(t, "Updated Name", u.FullName)
		assert.Equal(t, "https://example.com", u.Website)
		assert.Equal(t, "Earth", u.Location)
		assert.Equal(t, 3, u.MaxRepoCreation)
		assert.True(t, u.IsAdmin)
		assert.True(t, u.AllowGitHook)
		assert.False(t, u.AllowImportLocal)
		assert.False(t, u.ProhibitLogin)

		c := newClient(t)
		c.signIn(&User{Name: name, Password: newPassword})
		assert.Equal(t, name, c.username())

		// An email of another user is taken.
		other := seedUser(t, CreateUserOptions{})
		body["email"] = other.Email
		status, respBody = admin.do(http.MethodPatch, path, body)
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "email")
	})

	t.Run("delete", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodDelete, path, adminNoBody)
		_, err := database.Handle.Users().GetByID(t.Context(), created.ID)
		require.NoError(t, err, "user deleted by a request that was turned away")

		// A user who owns repositories or is in organizations is kept.
		owner := seedUser(t, CreateUserOptions{})
		seedRepository(t, owner.ID, CreateRepositoryOptions{})
		status, body := admin.do(http.MethodDelete, "/api/web/admin/users/"+strconv.FormatInt(owner.ID, 10), adminNoBody)
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)
		_, err = database.Handle.Users().GetByID(t.Context(), owner.ID)
		assert.NoError(t, err)

		deletable := seedUser(t, CreateUserOptions{})
		deletablePath := "/api/web/admin/users/" + strconv.FormatInt(deletable.ID, 10)
		status, body = admin.do(http.MethodDelete, deletablePath, adminNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err = database.Handle.Users().GetByID(t.Context(), deletable.ID)
		assert.True(t, database.IsErrUserNotExist(err), "user not deleted: %v", err)

		status, body = admin.do(http.MethodDelete, deletablePath, adminNoBody)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestAdminUserOptions(t *testing.T) {
	admin, nonAdmin, anonymous := adminClients(t)
	source := adminSeedAuthSource(t)
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/user-options", nil)

	type loginSource struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	var resp struct {
		LoginSources              []loginSource `json:"loginSources"`
		CanSendEmail              bool          `json:"canSendEmail"`
		LocalPathMigrationEnabled bool          `json:"localPathMigrationEnabled"`
	}
	admin.doJSON(http.MethodGet, "/api/web/admin/user-options", nil, http.StatusOK, &resp)
	assert.Contains(t, resp.LoginSources, loginSource{ID: source.ID, Name: source.Name})
	// Email is enabled by writeConfig, and local path migration is off by
	// default.
	assert.True(t, resp.CanSendEmail)
	assert.False(t, resp.LocalPathMigrationEnabled)
}

func TestAdminUserOfAuthSource(t *testing.T) {
	admin, _, _ := adminClients(t)
	source := adminSeedAuthSource(t)
	name := UniqueName("external-")

	// Users of an authentication source sign in there, so they need no
	// password but the name they sign in with.
	body := map[string]any{"name": name, "email": name + "@example.com", "loginSource": source.ID}
	status, respBody := admin.do(http.MethodPost, "/api/web/admin/users", body)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
	assert.Contains(t, adminFieldErrors(t, respBody), "loginName")

	body["loginName"] = name
	var created struct {
		ID int64 `json:"id"`
	}
	admin.doJSON(http.MethodPost, "/api/web/admin/users", body, http.StatusCreated, &created)
	u, err := database.Handle.Users().GetByID(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, source.ID, u.LoginSource)
	assert.Equal(t, name, u.LoginName)

	sourcePath := "/api/web/admin/auths/" + strconv.FormatInt(source.ID, 10)
	status, respBody = admin.do(http.MethodDelete, sourcePath, adminNoBody)
	assert.Equal(t, http.StatusConflict, status, "response: %s", respBody)
	_, err = database.Handle.LoginSources().GetByID(t.Context(), source.ID)
	assert.NoError(t, err, "authentication source in use deleted")

	status, respBody = admin.do(http.MethodDelete, "/api/web/admin/users/"+strconv.FormatInt(created.ID, 10), adminNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
	status, respBody = admin.do(http.MethodDelete, sourcePath, adminNoBody)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", respBody)

	status, respBody = admin.do(http.MethodPost, "/api/web/admin/users", map[string]any{
		"name":        UniqueName("external-"),
		"email":       UniqueName("external-") + "@example.com",
		"loginSource": 999999999,
		"loginName":   name,
	})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
	assert.Contains(t, adminFieldErrors(t, respBody), "loginSource")
}

func TestAdminAuthSources(t *testing.T) {
	admin, nonAdmin, anonymous := adminClients(t)
	name := UniqueName("auth-")
	body := map[string]any{
		"type":           auth.SMTP,
		"name":           name,
		"isActive":       false,
		"smtpAuth":       smtp.Plain,
		"smtpHost":       "127.0.0.1",
		"smtpPort":       1,
		"allowedDomains": "example.com",
	}
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	t.Run("create", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPost, "/api/web/admin/auths", body)

		admin.doJSON(http.MethodPost, "/api/web/admin/auths", body, http.StatusCreated, &created)
		assert.Equal(t, name, created.Name)
		source, err := database.Handle.LoginSources().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, auth.SMTP, source.Type)
		assert.False(t, source.IsActived)
		assert.False(t, source.IsDefault)
		assert.Equal(t, "127.0.0.1", source.SMTP().Host)
		assert.Equal(t, 1, source.SMTP().Port)

		status, respBody := admin.do(http.MethodPost, "/api/web/admin/auths", body)
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "name")

		invalid := map[string]any{"type": auth.SMTP, "name": UniqueName("auth-"), "smtpAuth": smtp.Plain, "smtpPort": 1}
		status, respBody = admin.do(http.MethodPost, "/api/web/admin/auths", invalid)
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "smtpHost")

		invalid = map[string]any{"type": 0, "name": UniqueName("auth-")}
		status, respBody = admin.do(http.MethodPost, "/api/web/admin/auths", invalid)
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "type")
	})
	require.NotZero(t, created.ID, "authentication source not created")
	t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(context.Background(), created.ID) })
	path := "/api/web/admin/auths/" + strconv.FormatInt(created.ID, 10)

	t.Run("list", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/auths", nil)

		type item struct {
			ID        int64     `json:"id"`
			Name      string    `json:"name"`
			Type      auth.Type `json:"type"`
			TypeName  string    `json:"typeName"`
			IsActive  bool      `json:"isActive"`
			IsDefault bool      `json:"isDefault"`
			FromFile  bool      `json:"fromFile"`
		}
		var resp struct {
			Items []item `json:"items"`
		}
		admin.doJSON(http.MethodGet, "/api/web/admin/auths", nil, http.StatusOK, &resp)
		assert.Contains(t, resp.Items, item{ID: created.ID, Name: name, Type: auth.SMTP, TypeName: "SMTP"})
	})

	t.Run("get", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, path, nil)

		var resp struct {
			ID             int64  `json:"id"`
			Name           string `json:"name"`
			TypeName       string `json:"typeName"`
			SMTPAuth       string `json:"smtpAuth"`
			SMTPHost       string `json:"smtpHost"`
			SMTPPort       int    `json:"smtpPort"`
			AllowedDomains string `json:"allowedDomains"`
		}
		admin.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		assert.Equal(t, created.ID, resp.ID)
		assert.Equal(t, name, resp.Name)
		assert.Equal(t, "SMTP", resp.TypeName)
		assert.Equal(t, smtp.Plain, resp.SMTPAuth)
		assert.Equal(t, "127.0.0.1", resp.SMTPHost)
		assert.Equal(t, 1, resp.SMTPPort)
		assert.Equal(t, "example.com", resp.AllowedDomains)

		status, respBody := admin.do(http.MethodGet, "/api/web/admin/auths/999999999", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", respBody)
	})

	t.Run("update", func(t *testing.T) {
		newName := UniqueName("auth-")
		update := map[string]any{
			"name":     newName,
			"isActive": false,
			"smtpAuth": smtp.Login,
			"smtpHost": "localhost",
			"smtpPort": 2,
			"tls":      true,
		}
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPatch, path, update)
		source, err := database.Handle.LoginSources().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		require.Equal(t, name, source.Name, "updated by a request that was turned away")

		status, respBody := admin.do(http.MethodPatch, path, update)
		require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
		source, err = database.Handle.LoginSources().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, newName, source.Name)
		assert.False(t, source.IsActived)
		assert.Equal(t, auth.SMTP, source.Type)
		assert.Equal(t, smtp.Login, source.SMTP().Auth)
		assert.Equal(t, "localhost", source.SMTP().Host)
		assert.Equal(t, 2, source.SMTP().Port)
		assert.True(t, source.SMTP().TLS)

		update["smtpHost"] = ""
		status, respBody = admin.do(http.MethodPatch, path, update)
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)
		assert.Contains(t, adminFieldErrors(t, respBody), "smtpHost")
	})

	t.Run("delete", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodDelete, path, adminNoBody)
		_, err := database.Handle.LoginSources().GetByID(t.Context(), created.ID)
		require.NoError(t, err, "deleted by a request that was turned away")

		status, respBody := admin.do(http.MethodDelete, path, adminNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
		_, err = database.Handle.LoginSources().GetByID(t.Context(), created.ID)
		assert.True(t, database.IsErrLoginSourceNotExist(err), "not deleted: %v", err)
		status, respBody = admin.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", respBody)
	})
}

func TestAdminLDAPAuthSourceBindPassword(t *testing.T) {
	admin, _, _ := adminClients(t)
	bindPassword := UniqueName("bind-")
	body := map[string]any{
		"type":             auth.LDAP,
		"name":             UniqueName("ldap-"),
		"securityProtocol": 0,
		"host":             "127.0.0.1",
		"port":             1,
		"bindDN":           "cn=admin,dc=example,dc=com",
		"bindPassword":     bindPassword,
		"userBase":         "ou=users,dc=example,dc=com",
		"filter":           "(uid=%s)",
		"attributeMail":    "mail",
	}
	var created struct {
		ID int64 `json:"id"`
	}
	admin.doJSON(http.MethodPost, "/api/web/admin/auths", body, http.StatusCreated, &created)
	t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(context.Background(), created.ID) })
	path := "/api/web/admin/auths/" + strconv.FormatInt(created.ID, 10)

	// The bind password is never sent back, only whether there is one.
	status, respBody := admin.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, status, "response: %s", respBody)
	assert.NotContains(t, string(respBody), bindPassword)
	assert.Contains(t, string(respBody), `"hasBindPassword":true`)

	// An update without a bind password keeps the one there is.
	delete(body, "bindPassword")
	body["host"] = "localhost"
	status, respBody = admin.do(http.MethodPatch, path, body)
	require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
	source, err := database.Handle.LoginSources().GetByID(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "localhost", source.LDAP().Host)
	assert.Equal(t, bindPassword, source.LDAP().BindPassword)
}
