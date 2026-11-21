package integrationtests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestAPIV1Users(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	anonymous := newClient(t)
	api := apiV1Client(t, seedUser(t, CreateUserOptions{}))

	t.Run("search", func(t *testing.T) {
		type searchResult struct {
			OK   bool        `json:"ok"`
			Data []apiV1User `json:"data"`
		}
		var resp searchResult
		anonymous.doJSON(http.MethodGet, "/api/v1/users/search?q="+u.Name, nil, http.StatusOK, &resp)
		assert.True(t, resp.OK)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, apiV1User{ID: u.ID, UserName: u.Name, Login: u.Name}, resp.Data[0], "email shown to anonymous viewer")

		resp = searchResult{}
		api.doJSON(http.MethodGet, "/api/v1/users/search?q="+u.Name, nil, http.StatusOK, &resp)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, u.Email, resp.Data[0].Email)
	})

	t.Run("profile", func(t *testing.T) {
		var got apiV1User
		anonymous.doJSON(http.MethodGet, "/api/v1/users/"+u.Name, nil, http.StatusOK, &got)
		assert.Equal(t, apiV1User{ID: u.ID, UserName: u.Name, Login: u.Name}, got, "email shown to anonymous viewer")

		got = apiV1User{}
		api.doJSON(http.MethodGet, "/api/v1/users/"+u.Name, nil, http.StatusOK, &got)
		assert.Equal(t, u.Email, got.Email)

		status, body := anonymous.do(http.MethodGet, "/api/v1/users/"+UniqueName("nobody-"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestAPIV1AccessTokens(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	path := "/api/v1/users/" + u.Name + "/tokens"

	basic := apiV1BasicClient(t, u.Name, u.Password)

	type token struct {
		Name string `json:"name"`
		Sha1 string `json:"sha1"`
	}
	create := func(t *testing.T, c *client, path string) token {
		t.Helper()

		name := UniqueName("token-")
		var created token
		c.doJSON(http.MethodPost, path, map[string]any{"name": name}, http.StatusCreated, &created)
		assert.Equal(t, name, created.Name)
		require.Len(t, created.Sha1, 40)
		return created
	}
	tokenClient := func(sha1 string) *client {
		c := newClient(t)
		c.header.Set("Authorization", "token "+sha1)
		return c
	}
	assertAuthenticates := func(t *testing.T, sha1 string, want bool) {
		t.Helper()

		status, body := tokenClient(sha1).do(http.MethodGet, "/api/v1/user", nil)
		if want {
			assert.Equal(t, http.StatusOK, status, "response: %s", body)
		} else {
			assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		}
	}
	listNames := func(t *testing.T) []string {
		t.Helper()

		var tokens []token
		basic.doJSON(http.MethodGet, path, nil, http.StatusOK, &tokens)
		names := []string{}
		for _, token := range tokens {
			names = append(names, token.Name)
		}
		return names
	}

	created := create(t, basic, path)
	assert.Equal(t, []string{created.Name}, listNames(t))

	// The new token authenticates its user.
	withToken := tokenClient(created.Sha1)
	var me apiV1User
	withToken.doJSON(http.MethodGet, "/api/v1/user", nil, http.StatusOK, &me)
	assert.Equal(t, u.Name, me.UserName)

	status, body := basic.do(http.MethodPost, path, map[string]any{"name": created.Name})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", body)

	// Tokens are only listed and created with the password.
	status, body = withToken.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	status, body = withToken.do(http.MethodPost, path, map[string]any{"name": UniqueName("token-")})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

	t.Run("delete itself", func(t *testing.T) {
		deleted := create(t, basic, path)
		// A newer token, so deleting the newest token of the user is not mistaken
		// for deleting the one that authenticates.
		newer := create(t, basic, path)
		other := seedUser(t, CreateUserOptions{})
		otherToken := create(t, apiV1BasicClient(t, other.Name, other.Password), "/api/v1/users/"+other.Name+"/tokens")

		// Only the token itself can delete it.
		for name, c := range map[string]*client{
			"anonymous":      newClient(t),
			"password":       basic,
			"unknown token":  tokenClient(strings.Repeat("0", 40)),
			"token as Basic": apiV1BasicClient(t, u.Name, deleted.Sha1),
		} {
			status, body := c.do(http.MethodDelete, "/api/v1/user/token", nil)
			assert.Equal(t, http.StatusUnauthorized, status, "%s, response: %s", name, body)
		}
		assertAuthenticates(t, deleted.Sha1, true)

		status, body := tokenClient(deleted.Sha1).do(http.MethodDelete, "/api/v1/user/token", nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, body)
		assertAuthenticates(t, deleted.Sha1, false)
		assert.Equal(t, []string{created.Name, newer.Name}, listNames(t))
		assertAuthenticates(t, created.Sha1, true)
		assertAuthenticates(t, newer.Sha1, true)
		assertAuthenticates(t, otherToken.Sha1, true)

		status, body = tokenClient(deleted.Sha1).do(http.MethodDelete, "/api/v1/user/token", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "deleted twice, response: %s", body)

		// The name of a deleted token is free again.
		status, body = basic.do(http.MethodPost, path, map[string]any{"name": deleted.Name})
		assert.Equal(t, http.StatusCreated, status, "response: %s", body)
	})
}

func TestAPIV1AuthenticatedUser(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})

	var got apiV1User
	apiV1Client(t, u).doJSON(http.MethodGet, "/api/v1/user", nil, http.StatusOK, &got)
	assert.Equal(t, apiV1User{ID: u.ID, UserName: u.Name, Login: u.Name, Email: u.Email}, got)

	status, body := newClient(t).do(http.MethodGet, "/api/v1/user", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
}

func TestAPIV1UserEmails(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, u)

	type email struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}
	listEmails := func() []email {
		var emails []email
		api.doJSON(http.MethodGet, "/api/v1/user/emails", nil, http.StatusOK, &emails)
		return emails
	}
	assert.Equal(t, []email{{Email: u.Email, Primary: true}}, listEmails())

	added := UniqueName("email-") + "@example.com"
	var created []email
	api.doJSON(http.MethodPost, "/api/v1/user/emails", map[string]any{"emails": []string{added}}, http.StatusCreated, &created)
	assert.Equal(t, []email{{Email: added}}, created)
	assert.ElementsMatch(t, []email{{Email: u.Email, Primary: true}, {Email: added}}, listEmails())

	status, body := apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodPost, "/api/v1/user/emails", map[string]any{"emails": []string{added}})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "email of another user, response: %s", body)

	status, body = api.do(http.MethodDelete, "/api/v1/user/emails", map[string]any{"emails": []string{u.Email}})
	assert.Equal(t, http.StatusBadRequest, status, "primary email, response: %s", body)

	status, body = api.do(http.MethodDelete, "/api/v1/user/emails", map[string]any{"emails": []string{added}})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, []email{{Email: u.Email, Primary: true}}, listEmails())
}

func TestAPIV1Following(t *testing.T) {
	t.Parallel()

	follower := seedUser(t, CreateUserOptions{})
	followee := seedUser(t, CreateUserOptions{})
	followerAPI := apiV1Client(t, follower)
	followeeAPI := apiV1Client(t, followee)

	listNames := func(c *client, path string) []string {
		t.Helper()

		var users []apiV1User
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &users)
		names := []string{}
		for _, u := range users {
			names = append(names, u.UserName)
		}
		return names
	}
	assertStatus := func(c *client, method, path string, want int) {
		t.Helper()

		status, body := c.do(method, path, nil)
		assert.Equal(t, want, status, "%s %s, response: %s", method, path, body)
	}

	assertStatus(followerAPI, http.MethodGet, "/api/v1/user/following/"+followee.Name, http.StatusNotFound)
	assertStatus(followerAPI, http.MethodPut, "/api/v1/user/following/"+followee.Name, http.StatusNoContent)

	assertStatus(followerAPI, http.MethodGet, "/api/v1/user/following/"+followee.Name, http.StatusNoContent)
	assert.Equal(t, []string{followee.Name}, listNames(followerAPI, "/api/v1/user/following"))
	assert.Equal(t, []string{follower.Name}, listNames(followeeAPI, "/api/v1/user/followers"))
	assert.Equal(t, []string{follower.Name}, listNames(followerAPI, "/api/v1/users/"+followee.Name+"/followers"))
	assert.Equal(t, []string{followee.Name}, listNames(followeeAPI, "/api/v1/users/"+follower.Name+"/following"))
	assertStatus(followeeAPI, http.MethodGet, "/api/v1/users/"+follower.Name+"/following/"+followee.Name, http.StatusNoContent)
	assertStatus(followeeAPI, http.MethodGet, "/api/v1/users/"+followee.Name+"/following/"+follower.Name, http.StatusNotFound)
	assert.True(t, database.Handle.Users().IsFollowing(t.Context(), follower.ID, followee.ID))

	assertStatus(followerAPI, http.MethodDelete, "/api/v1/user/following/"+followee.Name, http.StatusNoContent)
	assertStatus(followerAPI, http.MethodGet, "/api/v1/user/following/"+followee.Name, http.StatusNotFound)
	assert.Empty(t, listNames(followeeAPI, "/api/v1/user/followers"))

	assertStatus(newClient(t), http.MethodGet, "/api/v1/users/"+followee.Name+"/followers", http.StatusUnauthorized)
	assertStatus(followerAPI, http.MethodPut, "/api/v1/user/following/"+UniqueName("nobody-"), http.StatusNotFound)

	// Organizations cannot be followed.
	orgName := UniqueName("org-")
	followerAPI.doJSON(http.MethodPost, "/api/v1/user/orgs", map[string]any{"username": orgName}, http.StatusCreated, &apiV1Org{})
	assertStatus(followerAPI, http.MethodPut, "/api/v1/user/following/"+orgName, http.StatusNotFound)
	assert.Empty(t, listNames(followerAPI, "/api/v1/user/following"))
	assertStatus(followerAPI, http.MethodDelete, "/api/v1/user/following/"+orgName, http.StatusNotFound)
}

type apiV1PublicKeyResp struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func TestAPIV1UserKeys(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, u)

	title := UniqueName("key-")
	key := apiV1PublicKey(t)
	var created apiV1PublicKeyResp
	api.doJSON(http.MethodPost, "/api/v1/user/keys", map[string]any{"title": title, "key": key}, http.StatusCreated, &created)
	assert.Equal(t, title, created.Title)
	assert.Equal(t, key, created.Key)
	assert.Equal(t, serverURL+"/api/v1/user/keys/"+apiV1Itoa(created.ID), created.URL)

	var got apiV1PublicKeyResp
	api.doJSON(http.MethodGet, "/api/v1/user/keys/"+apiV1Itoa(created.ID), nil, http.StatusOK, &got)
	assert.Equal(t, created, got)

	var keys []apiV1PublicKeyResp
	api.doJSON(http.MethodGet, "/api/v1/user/keys", nil, http.StatusOK, &keys)
	assert.Equal(t, []apiV1PublicKeyResp{created}, keys)

	other := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	keys = nil
	other.doJSON(http.MethodGet, "/api/v1/users/"+u.Name+"/keys", nil, http.StatusOK, &keys)
	assert.Equal(t, []apiV1PublicKeyResp{created}, keys)

	status, body := api.do(http.MethodPost, "/api/v1/user/keys", map[string]any{"title": UniqueName("key-"), "key": "not a key"})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	var invalid struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(body, &invalid), "response: %s", body)
	assert.Regexp(t, `^Invalid key content: [^%]+$`, invalid.Message)

	status, body = other.do(http.MethodDelete, "/api/v1/user/keys/"+apiV1Itoa(created.ID), nil)
	assert.Equal(t, http.StatusForbidden, status, "key of another user, response: %s", body)

	status, body = api.do(http.MethodDelete, "/api/v1/user/keys/"+apiV1Itoa(created.ID), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err := database.Handle.PublicKeys().GetByID(t.Context(), created.ID)
	assert.True(t, database.IsErrKeyNotExist(err), "key not deleted: %v", err)

	t.Run("deleted key", func(t *testing.T) {
		status, body := api.do(http.MethodGet, "/api/v1/user/keys/"+apiV1Itoa(created.ID), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestAPIV1BasicAuthTwoFactor(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	_, err := EnableTwoFactor(t.Context(), u.ID)
	require.NoError(t, err)

	// The password alone would otherwise create an access token, which skips the
	// second factor for good.
	basic := apiV1BasicClient(t, u.Name, u.Password)
	tokensPath := "/api/v1/users/" + u.Name + "/tokens"
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, tokensPath, nil},
		{http.MethodPost, tokensPath, map[string]any{"name": UniqueName("token-")}},
	} {
		status, body := basic.do(req.method, req.path, req.body)
		assert.Equal(t, http.StatusUnauthorized, status, "%s %s, response: %s", req.method, req.path, body)
	}
	tokens, err := database.Handle.AccessTokens().List(t.Context(), u.ID)
	require.NoError(t, err)
	assert.Empty(t, tokens)

	// Access tokens keep working.
	var me apiV1User
	apiV1Client(t, u).doJSON(http.MethodGet, "/api/v1/user", nil, http.StatusOK, &me)
	assert.Equal(t, u.Name, me.UserName)

	// A token revokes itself without the password.
	status, body := apiV1Client(t, u).do(http.MethodDelete, "/api/v1/user/token", nil)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
}
