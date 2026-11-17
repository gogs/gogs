package integrationtests

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// orgNoBody is the body of requests that carry no data, which the web API
// still requires to be JSON.
var orgNoBody = struct{}{}

// orgSeed creates an organization with a unique name owned by the user.
func orgSeed(t *testing.T, ownerID int64) *database.User {
	t.Helper()

	owner, err := database.Handle.Users().GetByID(t.Context(), ownerID)
	require.NoError(t, err)
	org := &database.User{
		Name:     UniqueName("org-"),
		IsActive: true,
		Type:     database.UserTypeOrganization,
	}
	require.NoError(t, database.Handle.Organizations().Create(t.Context(), org, owner))
	return org
}

// orgAddMember makes the user a member of the organization, with the
// membership private.
func orgAddMember(t *testing.T, org *database.User, userID int64) {
	t.Helper()
	require.NoError(t, database.Handle.Organizations().AddMember(t.Context(), org.ID, userID))
}

func orgSignedIn(t *testing.T, u *User) *client {
	t.Helper()

	c := newClient(t)
	c.signIn(u)
	return c
}

type orgMember struct {
	Name     string `json:"name"`
	IsPublic bool   `json:"isPublic"`
	IsOwner  bool   `json:"isOwner"`
}

func TestOrgCreate(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	c := orgSignedIn(t, owner)

	name := UniqueName("org-")
	var created struct {
		Name string `json:"name"`
	}
	c.doJSON(http.MethodPost, "/api/web/orgs", map[string]any{"orgName": name}, http.StatusCreated, &created)
	assert.Equal(t, name, created.Name)

	org, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)
	assert.True(t, org.IsOrganization())
	assert.True(t, database.Handle.Organizations().IsOwner(t.Context(), org.ID, owner.ID))

	status, body := c.do(http.MethodPost, "/api/web/orgs", map[string]any{"orgName": name})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", body)

	other := UniqueName("org-")
	status, body = newClient(t).do(http.MethodPost, "/api/web/orgs", map[string]any{"orgName": other})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	_, err = database.Handle.Users().GetByUsername(t.Context(), other)
	assert.True(t, database.IsErrUserNotExist(err), "organization created while signed out: %v", err)
}

func TestOrgSettings(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)

	ownerClient := orgSignedIn(t, owner)
	path := "/api/web/orgs/" + org.Name + "/settings"

	type settings struct {
		Name            string `json:"name"`
		FullName        string `json:"fullName"`
		Description     string `json:"description"`
		Website         string `json:"website"`
		Location        string `json:"location"`
		HasCustomAvatar bool   `json:"hasCustomAvatar"`
		MaxRepoCreation *int   `json:"maxRepoCreation"`
	}
	var got settings
	ownerClient.doJSON(http.MethodGet, path, nil, http.StatusOK, &got)
	assert.Equal(t, org.Name, got.Name)
	// Organizations are created with a generated avatar.
	assert.True(t, got.HasCustomAvatar)
	assert.Nil(t, got.MaxRepoCreation, "only site admins get the repository limit")

	newName := UniqueName("org-")
	update := map[string]any{
		"name":        newName,
		"fullName":    "Acme Corporation",
		"description": "We make everything.",
		"website":     "https://acme.example.com",
		"location":    "Earth",
	}
	memberClient := orgSignedIn(t, member)
	status, body := memberClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusForbidden, status, "member GET, response: %s", body)
	status, body = memberClient.do(http.MethodPatch, path, update)
	assert.Equal(t, http.StatusForbidden, status, "member PATCH, response: %s", body)
	status, body = orgSignedIn(t, outsider).do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "outsider GET, response: %s", body)

	var renamed struct {
		Name string `json:"name"`
	}
	ownerClient.doJSON(http.MethodPatch, path, update, http.StatusOK, &renamed)
	assert.Equal(t, newName, renamed.Name)

	stored, err := database.Handle.Users().GetByID(t.Context(), org.ID)
	require.NoError(t, err)
	assert.Equal(t, newName, stored.Name)
	assert.Equal(t, "Acme Corporation", stored.FullName)
	assert.Equal(t, "We make everything.", stored.Description)
	assert.Equal(t, "https://acme.example.com", stored.Website)
	assert.Equal(t, "Earth", stored.Location)

	ownerClient.doJSON(http.MethodGet, "/api/web/orgs/"+newName+"/settings", nil, http.StatusOK, &got)
	assert.Equal(t, settings{
		Name:        newName,
		FullName:    "Acme Corporation",
		Description: "We make everything.",
		Website:     "https://acme.example.com",
		Location:    "Earth",
		// The generated avatar is kept.
		HasCustomAvatar: true,
	}, got)
	status, body = ownerClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "old name, response: %s", body)
}

func TestOrgDelete(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	ownerClient := orgSignedIn(t, owner)

	t.Run("with repositories", func(t *testing.T) {
		org := orgSeed(t, owner.ID)
		seedRepository(t, org.ID, CreateRepositoryOptions{})
		status, body := ownerClient.do(http.MethodDelete, "/api/web/orgs/"+org.Name, map[string]any{"password": owner.Password})
		assert.Equal(t, http.StatusConflict, status, "response: %s", body)
		_, err := database.Handle.Users().GetByID(t.Context(), org.ID)
		assert.NoError(t, err)
	})

	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	path := "/api/web/orgs/" + org.Name

	status, body := orgSignedIn(t, member).do(http.MethodDelete, path, map[string]any{"password": member.Password})
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, path, map[string]any{"password": owner.Password + "x"})
	assert.Equal(t, http.StatusUnauthorized, status, "wrong password, response: %s", body)
	_, err := database.Handle.Users().GetByID(t.Context(), org.ID)
	require.NoError(t, err, "organization deleted before the owner confirmed")

	status, body = ownerClient.do(http.MethodDelete, path, map[string]any{"password": owner.Password})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Users().GetByID(t.Context(), org.ID)
	assert.True(t, database.IsErrUserNotExist(err), "organization still exists: %v", err)
}

func TestOrgWebhooks(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	c := orgSignedIn(t, owner)
	path := "/api/web/orgs/" + org.Name + "/settings/hooks"

	hook := map[string]any{
		"type":        "gogs",
		"url":         "http://127.0.0.1:1/hook",
		"contentType": "json",
		"secret":      "s3cret",
		"events":      "push_only",
		"active":      true,
	}
	status, body := orgSignedIn(t, member).do(http.MethodPost, path, hook)
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)

	var created struct {
		ID int64 `json:"id"`
	}
	c.doJSON(http.MethodPost, path, hook, http.StatusCreated, &created)
	stored, err := database.Handle.Webhooks().GetOfOrg(t.Context(), org.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:1/hook", stored.URL)
	assert.Equal(t, "s3cret", stored.Secret)

	var list struct {
		Items []struct {
			ID     int64  `json:"id"`
			Type   string `json:"type"`
			URL    string `json:"url"`
			Active bool   `json:"active"`
		} `json:"items"`
		Types []string `json:"types"`
	}
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, created.ID, list.Items[0].ID)
	assert.Equal(t, "gogs", list.Items[0].Type)
	assert.Equal(t, "http://127.0.0.1:1/hook", list.Items[0].URL)
	assert.True(t, list.Items[0].Active)
	assert.Contains(t, list.Types, "gogs")

	hookPath := path + "/" + strconv.FormatInt(created.ID, 10)
	hook["url"] = "http://127.0.0.1:1/changed"
	hook["events"] = "send_everything"
	hook["active"] = false
	status, body = c.do(http.MethodPatch, hookPath, hook)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	var detail struct {
		ID       int64 `json:"id"`
		Settings struct {
			URL         string `json:"url"`
			ContentType string `json:"contentType"`
			Secret      string `json:"secret"`
			Events      string `json:"events"`
			Active      bool   `json:"active"`
		} `json:"settings"`
	}
	c.doJSON(http.MethodGet, hookPath, nil, http.StatusOK, &detail)
	assert.Equal(t, created.ID, detail.ID)
	assert.Equal(t, "http://127.0.0.1:1/changed", detail.Settings.URL)
	assert.Equal(t, "json", detail.Settings.ContentType)
	assert.Equal(t, "s3cret", detail.Settings.Secret)
	assert.Equal(t, "send_everything", detail.Settings.Events)
	assert.False(t, detail.Settings.Active)

	status, body = c.do(http.MethodDelete, hookPath, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Webhooks().GetOfOrg(t.Context(), org.ID, created.ID)
	assert.True(t, database.IsErrWebhookNotExist(err), "webhook still exists: %v", err)
	status, body = c.do(http.MethodGet, hookPath, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

// orgAvatarForm returns a multipart form with the content as the avatar file.
func orgAvatarForm(t *testing.T, content []byte) (string, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("avatar", "avatar.png")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return w.FormDataContentType(), &buf
}

func TestOrgAvatar(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	c := orgSignedIn(t, owner)
	path := "/api/web/orgs/" + org.Name + "/settings/avatar"

	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8))))

	contentType, form := orgAvatarForm(t, img.Bytes())
	status, _, body := orgSignedIn(t, member).doRaw(http.MethodPut, path, contentType, form)
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)

	contentType, form = orgAvatarForm(t, []byte("not an image"))
	status, _, body = c.doRaw(http.MethodPut, path, contentType, form)
	assert.Equal(t, http.StatusBadRequest, status, "not an image, response: %s", body)

	hasCustomAvatar := func() bool {
		t.Helper()
		var settings struct {
			HasCustomAvatar bool `json:"hasCustomAvatar"`
		}
		c.doJSON(http.MethodGet, "/api/web/orgs/"+org.Name+"/settings", nil, http.StatusOK, &settings)
		stored, err := database.Handle.Users().GetByID(t.Context(), org.ID)
		require.NoError(t, err)
		assert.Equal(t, stored.UseCustomAvatar, settings.HasCustomAvatar)
		return stored.UseCustomAvatar
	}
	// Organizations are created with a generated avatar.
	require.True(t, hasCustomAvatar())

	status, _, body = orgSignedIn(t, member).doRaw(http.MethodDelete, path, "", nil)
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)
	assert.True(t, hasCustomAvatar())

	status, _, body = c.doRaw(http.MethodDelete, path, "", nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, hasCustomAvatar())

	contentType, form = orgAvatarForm(t, img.Bytes())
	status, _, body = c.doRaw(http.MethodPut, path, contentType, form)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, hasCustomAvatar())
}

func TestOrgMembers(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	ownerClient := orgSignedIn(t, owner)
	memberClient := orgSignedIn(t, member)
	path := "/api/web/orgs/" + org.Name + "/members"

	listMembers := func(c *client) map[string]orgMember {
		t.Helper()
		var resp struct {
			Items []orgMember `json:"items"`
		}
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		members := make(map[string]orgMember, len(resp.Items))
		for _, m := range resp.Items {
			members[m.Name] = m
		}
		return members
	}

	status, body := memberClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "before joining, response: %s", body)

	status, body = ownerClient.do(http.MethodPut, path+"/"+member.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, member.ID))
	assert.Equal(t, orgMember{Name: member.Name}, listMembers(memberClient)[member.Name])
	assert.True(t, listMembers(ownerClient)[owner.Name].IsOwner, "owner listed as owner")

	status, body = memberClient.do(http.MethodPut, path+"/"+outsider.Name, orgNoBody)
	assert.Equal(t, http.StatusForbidden, status, "member inviting, response: %s", body)
	assert.False(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, outsider.ID))

	t.Run("public membership", func(t *testing.T) {
		status, body := memberClient.do(http.MethodPut, path+"/"+member.Name+"/public", orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		stored, err := database.Handle.Users().GetByID(t.Context(), member.ID)
		require.NoError(t, err)
		assert.True(t, database.Handle.Organizations().IsPublicMember(t.Context(), org.ID, stored.ID))
		assert.True(t, listMembers(ownerClient)[member.Name].IsPublic)

		status, body = memberClient.do(http.MethodDelete, path+"/"+owner.Name+"/public", orgNoBody)
		assert.Equal(t, http.StatusForbidden, status, "member changing the owner, response: %s", body)

		status, body = ownerClient.do(http.MethodDelete, path+"/"+member.Name+"/public", orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.False(t, database.Handle.Organizations().IsPublicMember(t.Context(), org.ID, stored.ID))
	})

	status, body = memberClient.do(http.MethodDelete, path+"/"+owner.Name, orgNoBody)
	assert.Equal(t, http.StatusForbidden, status, "member removing the owner, response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, path+"/"+owner.Name, orgNoBody)
	assert.Equal(t, http.StatusConflict, status, "last owner leaving, response: %s", body)
	assert.True(t, database.Handle.Organizations().IsOwner(t.Context(), org.ID, owner.ID))

	status, body = memberClient.do(http.MethodDelete, path+"/"+member.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "member leaving, response: %s", body)
	assert.False(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, member.ID))

	orgAddMember(t, org, outsider.ID)
	status, body = ownerClient.do(http.MethodDelete, path+"/"+outsider.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "owner removing, response: %s", body)
	assert.False(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, outsider.ID))
	assert.NotContains(t, listMembers(ownerClient), outsider.Name)
}

func TestOrgTeams(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{})
	ownerClient := orgSignedIn(t, owner)
	memberClient := orgSignedIn(t, member)
	teamsPath := "/api/web/orgs/" + org.Name + "/teams"

	type teamSummary struct {
		Name           string `json:"name"`
		LowerName      string `json:"lowerName"`
		MemberCount    int    `json:"memberCount"`
		RepoCount      int    `json:"repoCount"`
		ViewerIsMember bool   `json:"viewerIsMember"`
	}
	listTeams := func(c *client) []teamSummary {
		t.Helper()
		var resp struct {
			Items []teamSummary `json:"items"`
		}
		c.doJSON(http.MethodGet, teamsPath, nil, http.StatusOK, &resp)
		return resp.Items
	}
	assert.Equal(t, []teamSummary{{Name: "Owners", LowerName: "owners", MemberCount: 1, RepoCount: 1, ViewerIsMember: true}}, listTeams(ownerClient))
	assert.Empty(t, listTeams(memberClient), "member sees teams they are not on")

	name := UniqueName("Team-")
	request := map[string]any{"teamName": name, "description": "Builds things.", "permission": "write"}
	status, body := memberClient.do(http.MethodPost, teamsPath, request)
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)

	var created struct {
		LowerName string `json:"lowerName"`
	}
	ownerClient.doJSON(http.MethodPost, teamsPath, request, http.StatusCreated, &created)
	require.Equal(t, strings.ToLower(name), created.LowerName)
	team, err := database.Handle.Teams().GetByName(t.Context(), org.ID, name)
	require.NoError(t, err)
	assert.Equal(t, database.AccessModeWrite, team.Authorize)

	status, body = ownerClient.do(http.MethodPost, teamsPath, request)
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", body)

	teamPath := teamsPath + "/" + created.LowerName
	var detail struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Permission  string `json:"permission"`
		IsOwnerTeam bool   `json:"isOwnerTeam"`
	}
	ownerClient.doJSON(http.MethodGet, teamPath, nil, http.StatusOK, &detail)
	assert.Equal(t, name, detail.Name)
	assert.Equal(t, "Builds things.", detail.Description)
	assert.Equal(t, "write", detail.Permission)
	assert.False(t, detail.IsOwnerTeam)

	status, body = memberClient.do(http.MethodGet, teamPath, nil)
	assert.Equal(t, http.StatusNotFound, status, "member not on the team, response: %s", body)

	newName := UniqueName("Renamed-")
	update := map[string]any{"teamName": newName, "description": "Builds more.", "permission": "admin"}
	status, body = memberClient.do(http.MethodPatch, teamPath, update)
	assert.Equal(t, http.StatusNotFound, status, "member not on the team, response: %s", body)
	var updated struct {
		LowerName string `json:"lowerName"`
	}
	ownerClient.doJSON(http.MethodPatch, teamPath, update, http.StatusOK, &updated)
	assert.Equal(t, strings.ToLower(newName), updated.LowerName)
	team, err = database.Handle.Teams().GetByID(t.Context(), team.ID)
	require.NoError(t, err)
	assert.Equal(t, newName, team.Name)
	assert.Equal(t, "Builds more.", team.Description)
	assert.Equal(t, database.AccessModeAdmin, team.Authorize)
	teamPath = teamsPath + "/" + updated.LowerName

	t.Run("members", func(t *testing.T) {
		membersPath := teamPath + "/members"
		status, body := ownerClient.do(http.MethodPut, membersPath+"/"+member.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.True(t, database.Handle.Teams().IsMember(t.Context(), org.ID, team.ID, member.ID))

		var members struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		memberClient.doJSON(http.MethodGet, membersPath, nil, http.StatusOK, &members)
		require.Len(t, members.Items, 1)
		assert.Equal(t, member.Name, members.Items[0].Name)
		assert.Equal(t, []teamSummary{{Name: newName, LowerName: updated.LowerName, MemberCount: 1, ViewerIsMember: true}}, listTeams(memberClient))

		status, body = memberClient.do(http.MethodPut, membersPath+"/"+owner.Name, orgNoBody)
		assert.Equal(t, http.StatusForbidden, status, "member adding, response: %s", body)
		assert.False(t, database.Handle.Teams().IsMember(t.Context(), org.ID, team.ID, owner.ID))

		status, body = memberClient.do(http.MethodDelete, membersPath+"/"+member.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "member leaving, response: %s", body)
		assert.False(t, database.Handle.Teams().IsMember(t.Context(), org.ID, team.ID, member.ID))
		assert.True(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, member.ID), "leaving a team keeps the organization membership")

		ownersMembers := teamsPath + "/owners/members/" + owner.Name
		status, body = ownerClient.do(http.MethodDelete, ownersMembers, orgNoBody)
		assert.Equal(t, http.StatusConflict, status, "last owner, response: %s", body)
	})

	t.Run("repositories", func(t *testing.T) {
		reposPath := teamPath + "/repos"
		status, body := memberClient.do(http.MethodPut, reposPath+"/"+repo.Name, orgNoBody)
		assert.Equal(t, http.StatusNotFound, status, "member not on the team, response: %s", body)
		status, body = ownerClient.do(http.MethodPut, teamsPath+"/owners/repos/"+repo.Name, orgNoBody)
		assert.Equal(t, http.StatusForbidden, status, "owners team, response: %s", body)
		status, body = ownerClient.do(http.MethodPut, reposPath+"/"+UniqueName("missing-"), orgNoBody)
		assert.Equal(t, http.StatusNotFound, status, "missing repository, response: %s", body)

		status, body = ownerClient.do(http.MethodPut, reposPath+"/"+repo.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		stored, err := database.Handle.Teams().GetByID(t.Context(), team.ID)
		require.NoError(t, err)
		assert.True(t, database.Handle.Teams().HasRepo(t.Context(), stored.OrgID, stored.ID, repo.ID))

		var repos struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		ownerClient.doJSON(http.MethodGet, reposPath, nil, http.StatusOK, &repos)
		require.Len(t, repos.Items, 1)
		assert.Equal(t, repo.Name, repos.Items[0].Name)

		status, body = ownerClient.do(http.MethodDelete, reposPath+"/"+repo.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.False(t, database.Handle.Teams().HasRepo(t.Context(), stored.OrgID, stored.ID, repo.ID))
		ownerClient.doJSON(http.MethodGet, reposPath, nil, http.StatusOK, &repos)
		assert.Empty(t, repos.Items)
	})

	status, body = ownerClient.do(http.MethodDelete, teamsPath+"/owners", orgNoBody)
	assert.Equal(t, http.StatusForbidden, status, "owners team, response: %s", body)

	status, body = ownerClient.do(http.MethodDelete, teamPath, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Teams().GetByID(t.Context(), team.ID)
	assert.True(t, database.IsErrTeamNotExist(err), "team still exists: %v", err)
	status, body = ownerClient.do(http.MethodGet, teamPath, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}
