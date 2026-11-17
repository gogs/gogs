package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// apiV1AssertPassword asserts that the user authenticates with the password.
func apiV1AssertPassword(t *testing.T, username, password string) {
	t.Helper()

	status, body := apiV1BasicClient(t, username, password).do(http.MethodGet, "/api/v1/users/"+username+"/tokens", nil)
	assert.Equal(t, http.StatusOK, status, "response: %s", body)
}

func TestAPIV1AdminUsers(t *testing.T) {
	admin := apiV1Client(t, seedUser(t, CreateUserOptions{Admin: true}))
	nonAdmin := apiV1Client(t, seedUser(t, CreateUserOptions{}))

	name := UniqueName("admin-created-")
	body := map[string]any{
		"username":  name,
		"email":     name + "@example.com",
		"password":  "password",
		"full_name": "Created",
	}
	status, respBody := nonAdmin.do(http.MethodPost, "/api/v1/admin/users", body)
	assert.Equal(t, http.StatusForbidden, status, "created by non-admin, response: %s", respBody)

	var created apiV1User
	admin.doJSON(http.MethodPost, "/api/v1/admin/users", body, http.StatusCreated, &created)
	assert.Equal(t, name, created.UserName)
	assert.Equal(t, name+"@example.com", created.Email)
	assert.Equal(t, "Created", created.FullName)
	u, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)
	assert.True(t, u.IsActive)
	apiV1AssertPassword(t, name, "password")

	status, respBody = admin.do(http.MethodPost, "/api/v1/admin/users", body)
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", respBody)

	t.Run("edit", func(t *testing.T) {
		edit := map[string]any{
			"email":     name + "@edited.example.com",
			"full_name": "Edited",
			"website":   "https://example.com",
			"password":  "new-password",
			"admin":     true,
		}
		status, body := nonAdmin.do(http.MethodPatch, "/api/v1/admin/users/"+name, edit)
		assert.Equal(t, http.StatusForbidden, status, "edited by non-admin, response: %s", body)

		var edited apiV1User
		admin.doJSON(http.MethodPatch, "/api/v1/admin/users/"+name, edit, http.StatusOK, &edited)
		assert.Equal(t, name+"@edited.example.com", edited.Email)
		assert.Equal(t, "Edited", edited.FullName)

		u, err := database.Handle.Users().GetByID(t.Context(), created.ID)
		require.NoError(t, err)
		assert.Equal(t, name+"@edited.example.com", u.Email)
		assert.Equal(t, "https://example.com", u.Website)
		assert.True(t, u.IsAdmin)
		apiV1AssertPassword(t, name, "new-password")
		status, body = apiV1BasicClient(t, name, "password").do(http.MethodGet, "/api/v1/users/"+name+"/tokens", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "old password, response: %s", body)
	})

	t.Run("public key", func(t *testing.T) {
		title := UniqueName("key-")
		key := apiV1PublicKey(t)
		status, body := nonAdmin.do(http.MethodPost, "/api/v1/admin/users/"+name+"/keys", map[string]any{"title": title, "key": key})
		assert.Equal(t, http.StatusForbidden, status, "added by non-admin, response: %s", body)

		var resp apiV1PublicKeyResp
		admin.doJSON(http.MethodPost, "/api/v1/admin/users/"+name+"/keys", map[string]any{"title": title, "key": key}, http.StatusCreated, &resp)
		assert.Equal(t, title, resp.Title)
		got, err := database.Handle.PublicKeys().GetByID(t.Context(), resp.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, got.OwnerID)
		assert.Equal(t, key, got.Content)
	})

	t.Run("delete", func(t *testing.T) {
		status, body := nonAdmin.do(http.MethodDelete, "/api/v1/admin/users/"+name, nil)
		assert.Equal(t, http.StatusForbidden, status, "deleted by non-admin, response: %s", body)

		status, body = admin.do(http.MethodDelete, "/api/v1/admin/users/"+name, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := database.Handle.Users().GetByID(t.Context(), created.ID)
		assert.True(t, database.IsErrUserNotExist(err), "user not deleted: %v", err)

		withRepo := seedUser(t, CreateUserOptions{})
		seedRepository(t, withRepo.ID, CreateRepositoryOptions{})
		status, body = admin.do(http.MethodDelete, "/api/v1/admin/users/"+withRepo.Name, nil)
		assert.Equal(t, http.StatusUnprocessableEntity, status, "user who owns a repository, response: %s", body)
	})
}

func TestAPIV1AdminCreateForUser(t *testing.T) {
	admin := apiV1Client(t, seedUser(t, CreateUserOptions{Admin: true}))
	nonAdmin := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	u := seedUser(t, CreateUserOptions{})

	t.Run("organization", func(t *testing.T) {
		name := UniqueName("org-")
		status, body := nonAdmin.do(http.MethodPost, "/api/v1/admin/users/"+u.Name+"/orgs", map[string]any{"username": name})
		assert.Equal(t, http.StatusForbidden, status, "created by non-admin, response: %s", body)

		var org apiV1Org
		admin.doJSON(http.MethodPost, "/api/v1/admin/users/"+u.Name+"/orgs", map[string]any{"username": name, "full_name": "Org"}, http.StatusCreated, &org)
		assert.Equal(t, name, org.UserName)
		assert.Equal(t, "Org", org.FullName)
		got, err := database.GetOrgByName(name)
		require.NoError(t, err)
		assert.True(t, got.IsOwnedBy(u.ID), "user does not own the organization")
	})

	t.Run("repository", func(t *testing.T) {
		name := UniqueName("repo-")
		status, body := nonAdmin.do(http.MethodPost, "/api/v1/admin/users/"+u.Name+"/repos", map[string]any{"name": name})
		assert.Equal(t, http.StatusForbidden, status, "created by non-admin, response: %s", body)

		var repo apiV1Repo
		admin.doJSON(http.MethodPost, "/api/v1/admin/users/"+u.Name+"/repos", map[string]any{"name": name, "private": true}, http.StatusCreated, &repo)
		assert.Equal(t, u.Name+"/"+name, repo.FullName)
		assert.True(t, repo.Private)
		got, err := getRepositoryByName(t, u.ID, name)
		require.NoError(t, err)
		assert.Equal(t, repo.ID, got.ID)
	})
}

func TestAPIV1AdminTeams(t *testing.T) {
	adminUser := seedUser(t, CreateUserOptions{Admin: true})
	admin := apiV1Client(t, adminUser)
	nonAdmin := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	orgOwner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})

	orgName := UniqueName("org-")
	var org apiV1Org
	apiV1Client(t, orgOwner).doJSON(http.MethodPost, "/api/v1/user/orgs", map[string]any{"username": orgName}, http.StatusCreated, &org)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{})

	teamBody := map[string]any{"name": "developers", "description": "Developers", "permission": "write"}
	status, body := nonAdmin.do(http.MethodPost, "/api/v1/admin/orgs/"+orgName+"/teams", teamBody)
	assert.Equal(t, http.StatusForbidden, status, "created by non-admin, response: %s", body)

	var team apiV1Team
	admin.doJSON(http.MethodPost, "/api/v1/admin/orgs/"+orgName+"/teams", teamBody, http.StatusCreated, &team)
	assert.Equal(t, apiV1Team{ID: team.ID, Name: "developers", Description: "Developers", Permission: "write"}, team)
	teamPath := "/api/v1/admin/teams/" + apiV1Itoa(team.ID)

	t.Run("members", func(t *testing.T) {
		listMembers := func() []string {
			t.Helper()

			var users []apiV1User
			admin.doJSON(http.MethodGet, teamPath+"/members", nil, http.StatusOK, &users)
			names := []string{}
			for _, u := range users {
				names = append(names, u.UserName)
			}
			return names
		}
		assert.Empty(t, listMembers())

		status, body := nonAdmin.do(http.MethodPut, teamPath+"/members/"+member.Name, nil)
		assert.Equal(t, http.StatusForbidden, status, "added by non-admin, response: %s", body)

		status, body = admin.do(http.MethodPut, teamPath+"/members/"+member.Name, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, []string{member.Name}, listMembers())

		status, body = admin.do(http.MethodDelete, teamPath+"/members/"+member.Name, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, listMembers())

		status, body = nonAdmin.do(http.MethodGet, teamPath+"/members", nil)
		assert.Equal(t, http.StatusForbidden, status, "listed by non-admin, response: %s", body)
	})

	t.Run("repositories", func(t *testing.T) {
		hasRepository := func() bool {
			t.Helper()

			got, err := database.GetTeamByID(team.ID)
			require.NoError(t, err)
			return got.HasRepository(repo.ID)
		}

		status, body := nonAdmin.do(http.MethodPut, teamPath+"/repos/"+repo.Name, nil)
		assert.Equal(t, http.StatusForbidden, status, "added by non-admin, response: %s", body)

		status, body = admin.do(http.MethodPut, teamPath+"/repos/"+repo.Name, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.True(t, hasRepository())

		status, body = admin.do(http.MethodDelete, teamPath+"/repos/"+repo.Name, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.False(t, hasRepository())

		status, body = admin.do(http.MethodPut, teamPath+"/repos/"+UniqueName("missing-"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}
