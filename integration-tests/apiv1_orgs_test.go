package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

type apiV1Org struct {
	ID          int64  `json:"id"`
	UserName    string `json:"username"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

type apiV1Team struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permission  string `json:"permission"`
}

func apiV1OrgNames(orgs []apiV1Org) []string {
	names := []string{}
	for _, o := range orgs {
		names = append(names, o.UserName)
	}
	return names
}

func TestAPIV1Organizations(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, owner)
	other := apiV1Client(t, seedUser(t, CreateUserOptions{}))

	name := UniqueName("org-")
	created := apiV1Org{
		UserName:    name,
		FullName:    "Organization",
		Description: "Description",
		Website:     "https://example.com",
		Location:    "Earth",
	}
	var resp apiV1Org
	api.doJSON(http.MethodPost, "/api/v1/user/orgs", map[string]any{
		"username":    created.UserName,
		"full_name":   created.FullName,
		"description": created.Description,
		"website":     created.Website,
		"location":    created.Location,
	}, http.StatusCreated, &resp)
	created.ID = resp.ID
	assert.Equal(t, created, resp)

	status, body := api.do(http.MethodPost, "/api/v1/user/orgs", map[string]any{"username": name})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", body)

	var got apiV1Org
	other.doJSON(http.MethodGet, "/api/v1/orgs/"+name, nil, http.StatusOK, &got)
	assert.Equal(t, created, got)

	t.Run("memberships", func(t *testing.T) {
		var orgs []apiV1Org
		api.doJSON(http.MethodGet, "/api/v1/user/orgs", nil, http.StatusOK, &orgs)
		assert.Equal(t, []string{name}, apiV1OrgNames(orgs))

		// Only public memberships are listed to others.
		orgs = nil
		newClient(t).doJSON(http.MethodGet, "/api/v1/users/"+owner.Name+"/orgs", nil, http.StatusOK, &orgs)
		assert.Empty(t, orgs)
		require.NoError(t, database.Handle.Organizations().SetMemberVisibility(t.Context(), created.ID, owner.ID, true))
		orgs = nil
		newClient(t).doJSON(http.MethodGet, "/api/v1/users/"+owner.Name+"/orgs", nil, http.StatusOK, &orgs)
		assert.Equal(t, []string{name}, apiV1OrgNames(orgs))
	})

	t.Run("edit", func(t *testing.T) {
		edit := map[string]any{
			"full_name":   "Edited",
			"description": "Edited description",
			"website":     "https://edited.example.com",
			"location":    "Moon",
		}
		status, body := other.do(http.MethodPatch, "/api/v1/orgs/"+name, edit)
		assert.Equal(t, http.StatusForbidden, status, "edited by a non-member, response: %s", body)

		want := apiV1Org{
			ID:          created.ID,
			UserName:    name,
			FullName:    "Edited",
			Description: "Edited description",
			Website:     "https://edited.example.com",
			Location:    "Moon",
		}
		var edited apiV1Org
		api.doJSON(http.MethodPatch, "/api/v1/orgs/"+name, edit, http.StatusOK, &edited)
		assert.Equal(t, want, edited)
		got := apiV1Org{}
		api.doJSON(http.MethodGet, "/api/v1/orgs/"+name, nil, http.StatusOK, &got)
		assert.Equal(t, want, got)
	})

	t.Run("teams", func(t *testing.T) {
		var teams []apiV1Team
		api.doJSON(http.MethodGet, "/api/v1/orgs/"+name+"/teams", nil, http.StatusOK, &teams)
		require.Len(t, teams, 1)
		assert.Equal(t, "Owners", teams[0].Name)
		assert.Equal(t, "owner", teams[0].Permission)
	})

	t.Run("repositories", func(t *testing.T) {
		repoName := UniqueName("repo-")
		status, body := other.do(http.MethodPost, "/api/v1/org/"+name+"/repos", map[string]any{"name": repoName})
		assert.Equal(t, http.StatusForbidden, status, "created by a non-member, response: %s", body)

		var repo apiV1Repo
		api.doJSON(http.MethodPost, "/api/v1/org/"+name+"/repos", map[string]any{"name": repoName}, http.StatusCreated, &repo)
		assert.Equal(t, name+"/"+repoName, repo.FullName)
		assert.Equal(t, name, repo.Owner.UserName)

		var repos []apiV1Repo
		api.doJSON(http.MethodGet, "/api/v1/orgs/"+name+"/repos", nil, http.StatusOK, &repos)
		assert.Equal(t, []string{repoName}, apiV1RepoNames(repos))

		status, body = newClient(t).do(http.MethodGet, "/api/v1/orgs/"+name+"/repos", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})
}
