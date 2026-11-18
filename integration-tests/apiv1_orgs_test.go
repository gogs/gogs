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
	t.Parallel()

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
		require.NoError(t, database.ChangeOrgUserStatus(created.ID, owner.ID, true))
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

		privateName := UniqueName("repo-")
		api.doJSON(http.MethodPost, "/api/v1/org/"+name+"/repos", map[string]any{"name": privateName, "private": true}, http.StatusCreated, &repo)

		// listAccess returns the repositories of the organization that the client
		// can see, with the highest access the response reports on each.
		listAccess := func(c *client) map[string]string {
			t.Helper()

			var repos []apiV1Repo
			c.doJSON(http.MethodGet, "/api/v1/orgs/"+name+"/repos", nil, http.StatusOK, &repos)
			access := make(map[string]string, len(repos))
			for _, r := range repos {
				require.NotNil(t, r.Permissions, "permissions of %q", r.Name)
				switch {
				case r.Permissions.Admin:
					access[r.Name] = "admin"
				case r.Permissions.Push:
					access[r.Name] = "write"
				case r.Permissions.Pull:
					access[r.Name] = "read"
				default:
					access[r.Name] = "none"
				}
			}
			return access
		}

		assert.Equal(t, map[string]string{repoName: "admin", privateName: "admin"}, listAccess(api), "owner")
		assert.Equal(t, map[string]string{repoName: "read"}, listAccess(other), "non-member")

		// A team gives its members its access to its repositories only.
		member := seedUser(t, CreateUserOptions{})
		admin := apiV1Client(t, seedUser(t, CreateUserOptions{Admin: true}))
		var team apiV1Team
		admin.doJSON(http.MethodPost, "/api/v1/admin/orgs/"+name+"/teams", map[string]any{"name": "developers", "permission": "write"}, http.StatusCreated, &team)
		teamPath := "/api/v1/admin/teams/" + apiV1Itoa(team.ID)
		for _, path := range []string{teamPath + "/members/" + member.Name, teamPath + "/repos/" + privateName} {
			status, body = admin.do(http.MethodPut, path, nil)
			require.Equal(t, http.StatusNoContent, status, "PUT %s, response: %s", path, body)
		}
		assert.Equal(t, map[string]string{repoName: "read", privateName: "write"}, listAccess(apiV1Client(t, member)), "team member")

		collaborator := seedUser(t, CreateUserOptions{})
		path := "/api/v1/repos/" + name + "/" + repoName + "/collaborators/" + collaborator.Name
		status, body = api.do(http.MethodPut, path, map[string]any{"permission": "write"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, map[string]string{repoName: "write"}, listAccess(apiV1Client(t, collaborator)), "outside collaborator")

		status, body = newClient(t).do(http.MethodGet, "/api/v1/orgs/"+name+"/repos", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})
}
