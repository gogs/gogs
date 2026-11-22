package integrationtests

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
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

type orgMember struct {
	Name     string `json:"name"`
	IsPublic bool   `json:"isPublic"`
	IsOwner  bool   `json:"isOwner"`
}

func TestOrgCreate(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, owner)

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

	status, _, generated := c.doRaw(http.MethodGet, "/-/avatars/"+strconv.FormatInt(org.ID, 10), "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", generated)
	saved, err := os.ReadFile(userx.GeneratedAvatarPath(org.ID, avatar.Organization))
	require.NoError(t, err, "the generated avatar is not saved as an organization avatar")
	assert.Equal(t, generated, saved)

	status, body := c.do(http.MethodPost, "/api/web/orgs", map[string]any{"orgName": name})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "duplicate name, response: %s", body)

	other := UniqueName("org-")
	status, body = newClient(t).do(http.MethodPost, "/api/web/orgs", map[string]any{"orgName": other})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	_, err = database.Handle.Users().GetByUsername(t.Context(), other)
	assert.True(t, database.IsErrUserNotExist(err), "organization created while signed out: %v", err)
}

func TestOrgSettings(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)

	ownerClient := authenticatedClient(t, owner)
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
	// Organizations are created without an uploaded avatar.
	assert.False(t, got.HasCustomAvatar)
	assert.Nil(t, got.MaxRepoCreation, "only site admins get the repository limit")

	newName := UniqueName("org-")
	update := map[string]any{
		"name":        newName,
		"fullName":    "Acme Corporation",
		"description": "We make everything.",
		"website":     "https://acme.example.com",
		"location":    "Earth",
	}
	memberClient := authenticatedClient(t, member)
	status, body := memberClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusForbidden, status, "member GET, response: %s", body)
	status, body = memberClient.do(http.MethodPatch, path, update)
	assert.Equal(t, http.StatusForbidden, status, "member PATCH, response: %s", body)
	status, body = authenticatedClient(t, outsider).do(http.MethodGet, path, nil)
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
	}, got)
	status, body = ownerClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "old name, response: %s", body)
}

func TestOrgDelete(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	ownerClient := authenticatedClient(t, owner)

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
	other := orgSeed(t, owner.ID)
	orgAddMember(t, other, member.ID)
	path := "/api/web/orgs/" + org.Name
	hookIDs := map[int64]int64{}
	for _, o := range []*database.User{org, other} {
		var created struct {
			ID int64 `json:"id"`
		}
		ownerClient.doJSON(http.MethodPost, "/api/web/orgs/"+o.Name+"/settings/hooks", map[string]any{
			"type": "gogs", "url": "http://127.0.0.1:1/hook", "contentType": "json", "events": "send_everything", "active": true,
		}, http.StatusCreated, &created)
		hookIDs[o.ID] = created.ID
	}
	// An organization with repositories cannot be deleted, but the deliveries
	// of a repository that is transferred out stay with the webhook.
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{})
	apiV1CreateIssue(t, apiV1Client(t, owner), repo, map[string]any{"title": "delivered to the organization"})
	status, body := ownerClient.do(http.MethodPost, repoSettingsAPIPath(repo, "/settings/transfer"), map[string]any{"repoName": repo.Name, "newOwner": owner.Name})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	tasks, err := database.Handle.HookTasks().ListByWebhook(t.Context(), hookIDs[org.ID], 1, 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	status, body = authenticatedClient(t, member).do(http.MethodDelete, path, map[string]any{"password": member.Password})
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, path, map[string]any{"password": owner.Password + "x"})
	assert.Equal(t, http.StatusUnauthorized, status, "wrong password, response: %s", body)
	_, err = database.Handle.Users().GetByID(t.Context(), org.ID)
	require.NoError(t, err, "organization deleted before the owner confirmed")

	status, body = ownerClient.do(http.MethodDelete, path, map[string]any{"password": owner.Password})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Users().GetByID(t.Context(), org.ID)
	assert.True(t, database.IsErrUserNotExist(err), "organization still exists: %v", err)
	// Only the memberships of the deleted organization are gone.
	assert.False(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, member.ID))
	assert.False(t, database.Handle.Organizations().IsMember(t.Context(), org.ID, owner.ID))
	assert.True(t, database.Handle.Organizations().IsMember(t.Context(), other.ID, member.ID))
	assert.True(t, database.Handle.Organizations().IsMember(t.Context(), other.ID, owner.ID))
	_, err = database.Handle.Webhooks().GetOfOrg(t.Context(), org.ID, hookIDs[org.ID])
	assert.True(t, database.IsErrWebhookNotExist(err), "webhook of deleted organization still exists: %v", err)
	tasks, err = database.Handle.HookTasks().ListByWebhook(t.Context(), hookIDs[org.ID], 1, 0)
	require.NoError(t, err)
	assert.Empty(t, tasks, "deliveries of webhooks of deleted organization")
	_, err = database.Handle.Webhooks().GetOfOrg(t.Context(), other.ID, hookIDs[other.ID])
	assert.NoError(t, err)
}

func TestOrgWebhooks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	c := authenticatedClient(t, owner)
	path := "/api/web/orgs/" + org.Name + "/settings/hooks"

	hook := map[string]any{
		"type":        "gogs",
		"url":         "http://127.0.0.1:1/hook",
		"contentType": "json",
		"secret":      "s3cret",
		"events":      "push_only",
		"active":      true,
	}
	status, body := authenticatedClient(t, member).do(http.MethodPost, path, hook)
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

// Issues of a repository of the organization reach the webhooks of both the
// repository and the organization. Issues of a repository of a user reach no
// webhook of an organization.
func TestOrgWebhookEvents(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgRepo := seedRepository(t, org.ID, CreateRepositoryOptions{})
	userRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	c := authenticatedClient(t, owner)
	api := apiV1Client(t, owner)
	issuesOnly := func(url string) map[string]any {
		return map[string]any{
			"type":         "gogs",
			"url":          url,
			"contentType":  "json",
			"events":       "choose_events",
			"chosenEvents": map[string]bool{"issues": true},
			"active":       true,
		}
	}
	var created struct {
		ID int64 `json:"id"`
	}
	repoURL, repoDeliveries := repoSettingsWebhookReceiver(t)
	c.doJSON(http.MethodPost, repoSettingsAPIPath(orgRepo, "/settings/hooks"), issuesOnly(repoURL), http.StatusCreated, &created)
	repoHookID := created.ID
	orgURL, orgDeliveries := repoSettingsWebhookReceiver(t)
	c.doJSON(http.MethodPost, "/api/web/orgs/"+org.Name+"/settings/hooks", issuesOnly(orgURL), http.StatusCreated, &created)
	orgHookID := created.ID

	// The issue of the user repository is created first, so a delivery of it
	// to the organization webhook would be queued before the other.
	apiV1CreateIssue(t, api, userRepo, map[string]any{"title": "of user"})
	apiV1CreateIssue(t, api, orgRepo, map[string]any{"title": "of organization"})

	type issuesPayload struct {
		Action string `json:"action"`
		Issue  struct {
			Title string `json:"title"`
		} `json:"issue"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	for name, deliveries := range map[string]<-chan repoSettingsDelivery{"repository": repoDeliveries, "organization": orgDeliveries} {
		d := repoSettingsNextDelivery(t, deliveries)
		assert.Equal(t, "issues", d.Header.Get("X-Gogs-Event"), name)
		var payload issuesPayload
		require.NoError(t, json.Unmarshal(d.Body, &payload), "payload: %s", d.Body)
		assert.Equal(t, "opened", payload.Action, name)
		assert.Equal(t, "of organization", payload.Issue.Title, name)
		assert.Equal(t, org.Name+"/"+orgRepo.Name, payload.Repo.FullName, name)
	}

	for _, id := range []int64{repoHookID, orgHookID} {
		tasks, err := database.Handle.HookTasks().ListByWebhook(t.Context(), id, 1, 10)
		require.NoError(t, err)
		assert.Len(t, tasks, 1, "deliveries of webhook %d", id)
	}
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
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	c := authenticatedClient(t, owner)
	path := "/api/web/orgs/" + org.Name + "/settings/avatar"

	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8))))

	contentType, form := orgAvatarForm(t, img.Bytes())
	status, _, body := authenticatedClient(t, member).doRaw(http.MethodPut, path, contentType, form)
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
	// Organizations are created without an uploaded avatar.
	require.False(t, hasCustomAvatar())

	contentType, form = orgAvatarForm(t, img.Bytes())
	status, _, body = c.doRaw(http.MethodPut, path, contentType, form)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, hasCustomAvatar())

	status, _, body = authenticatedClient(t, member).doRaw(http.MethodDelete, path, "", nil)
	assert.Equal(t, http.StatusForbidden, status, "member, response: %s", body)
	assert.True(t, hasCustomAvatar())

	status, _, body = c.doRaw(http.MethodDelete, path, "", nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, hasCustomAvatar())
}

func TestOrgMembers(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	ownerClient := authenticatedClient(t, owner)
	memberClient := authenticatedClient(t, member)
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

// A member removed from an organization loses the access their teams gave
// them to its repositories, and stops watching them.
func TestOrgMemberRemovedLosesAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	memberClient := authenticatedClient(t, member)
	orgPath := "/api/web/orgs/" + org.Name

	orgAddMember(t, org, member.ID)
	var team struct {
		LowerName string `json:"lowerName"`
	}
	ownerClient.doJSON(http.MethodPost, orgPath+"/teams", map[string]any{"teamName": UniqueName("Team-"), "permission": "write"}, http.StatusCreated, &team)
	teamPath := orgPath + "/teams/" + team.LowerName
	status, body := ownerClient.do(http.MethodPut, teamPath+"/members/"+member.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodPut, teamPath+"/repos/"+repo.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	accessMode := func() database.AccessMode {
		return database.Handle.Permissions().AccessMode(t.Context(), logger, member.ID, repo.ID, database.AccessModeOptions{OwnerID: org.ID, Private: true})
	}
	require.Equal(t, database.AccessModeWrite, accessMode())
	status, body = memberClient.do(http.MethodPost, repoSettingsAPIPath(repo, "/watch"), repoSettingsNoBody)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	watches := repoSettingsGetRepo(t, repo.ID).NumWatches

	status, body = ownerClient.do(http.MethodDelete, orgPath+"/members/"+member.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, database.AccessModeNone, accessMode())
	assert.False(t, database.Handle.Repositories().IsWatching(t.Context(), member.ID, repo.ID))
	assert.Equal(t, watches-1, repoSettingsGetRepo(t, repo.ID).NumWatches)
	status, body = memberClient.do(http.MethodGet, issueAPIPath(repo, "issues"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

// A collaborator of an organization's repository gets the highest of their
// collaboration mode and the modes of their teams that have the repository,
// and keeps their teams' access when they stop being a collaborator.
func TestOrgCollaboratorAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	otherRepo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	orgPath := "/api/web/orgs/" + org.Name

	orgAddMember(t, org, member.ID)
	addTeam := func(permission string, repo *Repository) {
		t.Helper()
		var team struct {
			LowerName string `json:"lowerName"`
		}
		ownerClient.doJSON(http.MethodPost, orgPath+"/teams", map[string]any{"teamName": UniqueName("Team-"), "permission": permission}, http.StatusCreated, &team)
		teamPath := orgPath + "/teams/" + team.LowerName
		status, body := ownerClient.do(http.MethodPut, teamPath+"/members/"+member.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, body = ownerClient.do(http.MethodPut, teamPath+"/repos/"+repo.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}
	addTeam("write", repo)
	// A team that has only another repository gives no access to this one.
	addTeam("admin", otherRepo)

	accessMode := func(userID int64) database.AccessMode {
		return database.Handle.Permissions().AccessMode(t.Context(), logger, userID, repo.ID, database.AccessModeOptions{OwnerID: org.ID, Private: true})
	}
	path := repoSettingsAPIPath(repo, "/settings/collaborators")
	setMode := func(u *User, mode string) {
		t.Helper()
		status, body := ownerClient.do(http.MethodPatch, path+"/"+u.Name, map[string]any{"mode": mode})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}
	require.Equal(t, database.AccessModeWrite, accessMode(member.ID))

	for _, u := range []*User{member, outsider} {
		status, body := ownerClient.do(http.MethodPost, path, map[string]any{"username": u.Name})
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
	}
	assert.Equal(t, database.AccessModeWrite, accessMode(member.ID))
	assert.Equal(t, database.AccessModeWrite, accessMode(outsider.ID))

	setMode(member, "read")
	setMode(outsider, "read")
	assert.Equal(t, database.AccessModeWrite, accessMode(member.ID), "the team that has the repository still gives write")
	assert.Equal(t, database.AccessModeRead, accessMode(outsider.ID))

	setMode(member, "admin")
	assert.Equal(t, database.AccessModeAdmin, accessMode(member.ID))

	for _, u := range []*User{member, outsider} {
		status, body := ownerClient.do(http.MethodDelete, path+"/"+u.Name, repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}
	assert.Equal(t, database.AccessModeWrite, accessMode(member.ID), "the member keeps their team's access")
	assert.Equal(t, database.AccessModeNone, accessMode(outsider.ID))
	status, body := authenticatedClient(t, outsider).do(http.MethodGet, issueAPIPath(repo, "issues"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

// A member's access through a team follows the team's permission, and is
// revoked when the repository leaves the team or the organization, or when the
// team is deleted.
func TestOrgTeamAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	memberClient := authenticatedClient(t, member)
	orgPath := "/api/web/orgs/" + org.Name
	orgAddMember(t, org, member.ID)

	accessMode := func(ownerID int64) database.AccessMode {
		return database.Handle.Permissions().AccessMode(t.Context(), logger, member.ID, repo.ID, database.AccessModeOptions{OwnerID: ownerID, Private: true})
	}
	addTeam := func(permission string) string {
		t.Helper()
		var team struct {
			LowerName string `json:"lowerName"`
		}
		ownerClient.doJSON(http.MethodPost, orgPath+"/teams", map[string]any{"teamName": UniqueName("Team-"), "permission": permission}, http.StatusCreated, &team)
		teamPath := orgPath + "/teams/" + team.LowerName
		for _, path := range []string{"/members/" + member.Name, "/repos/" + repo.Name} {
			status, body := ownerClient.do(http.MethodPut, teamPath+path, orgNoBody)
			require.Equal(t, http.StatusNoContent, status, "PUT %s, response: %s", path, body)
		}
		return teamPath
	}

	teamPath := addTeam("write")
	require.Equal(t, database.AccessModeWrite, accessMode(org.ID))

	var updated struct {
		LowerName string `json:"lowerName"`
	}
	ownerClient.doJSON(http.MethodPatch, teamPath, map[string]any{"teamName": UniqueName("Team-"), "permission": "read"}, http.StatusOK, &updated)
	teamPath = orgPath + "/teams/" + updated.LowerName
	assert.Equal(t, database.AccessModeRead, accessMode(org.ID), "downgraded team")

	status, body := ownerClient.do(http.MethodDelete, teamPath+"/repos/"+repo.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, database.AccessModeNone, accessMode(org.ID), "repository removed from the team")

	status, body = ownerClient.do(http.MethodPut, teamPath+"/repos/"+repo.Name, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	require.Equal(t, database.AccessModeRead, accessMode(org.ID))
	status, body = ownerClient.do(http.MethodDelete, teamPath, orgNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, database.AccessModeNone, accessMode(org.ID), "team deleted")

	addTeam("admin")
	require.Equal(t, database.AccessModeAdmin, accessMode(org.ID))
	status, body = ownerClient.do(http.MethodPost, repoSettingsAPIPath(repo, "/settings/transfer"), map[string]any{"repoName": repo.Name, "newOwner": owner.Name})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, database.AccessModeNone, accessMode(owner.ID), "repository transferred out of the organization")
	status, body = memberClient.do(http.MethodGet, issueAPIPath(&Repository{ID: repo.ID, Owner: owner.Name, Name: repo.Name}, "issues"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestOrgTeams(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	repo := seedRepository(t, org.ID, CreateRepositoryOptions{})
	ownerClient := authenticatedClient(t, owner)
	memberClient := authenticatedClient(t, member)
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

// Changes to a team and its repositories that race keep the accesses of the
// members consistent, and the requests succeed when the outcome is well
// defined.
func TestOrgTeamConcurrentChanges(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	lockRepo := "SELECT id FROM repository WHERE id = ? FOR UPDATE"

	// seed creates an organization with a write team that has a private
	// repository, and a member of the organization who is not on the team yet.
	seed := func(t *testing.T, ownerClient *client) (org *database.User, member *User, repo *Repository, teamPath string) {
		t.Helper()
		org = orgSeed(t, owner.ID)
		member = seedUser(t, CreateUserOptions{})
		orgAddMember(t, org, member.ID)
		repo = seedRepository(t, org.ID, CreateRepositoryOptions{Private: true})
		var team struct {
			LowerName string `json:"lowerName"`
		}
		orgPath := "/api/web/orgs/" + org.Name
		ownerClient.doJSON(http.MethodPost, orgPath+"/teams", map[string]any{"teamName": UniqueName("Team-"), "permission": "write"}, http.StatusCreated, &team)
		teamPath = orgPath + "/teams/" + team.LowerName
		status, body := ownerClient.do(http.MethodPut, teamPath+"/repos/"+repo.Name, orgNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		return org, member, repo, teamPath
	}
	accessMode := func(t *testing.T, userID int64, repo *Repository, ownerID int64) database.AccessMode {
		return database.Handle.Permissions().AccessMode(t.Context(), logger, userID, repo.ID, database.AccessModeOptions{OwnerID: ownerID, Private: true})
	}

	t.Run("adding a member while deleting a repository of the team", func(t *testing.T) {
		ownerClient := authenticatedClient(t, owner)
		for _, deleteFirst := range []bool{true, false} {
			org, member, deleted, teamPath := seed(t, ownerClient)
			kept := seedRepository(t, org.ID, CreateRepositoryOptions{Private: true})
			status, body := ownerClient.do(http.MethodPut, teamPath+"/repos/"+kept.Name, orgNoBody)
			require.Equal(t, http.StatusNoContent, status, "response: %s", body)

			add := lockedRequest{http.MethodPut, teamPath + "/members/" + member.Name, orgNoBody}
			del := lockedRequest{http.MethodDelete, repoSettingsAPIPath(deleted, ""), map[string]any{"repoName": deleted.Name}}
			requests := []lockedRequest{add, del}
			if deleteFirst {
				requests = []lockedRequest{del, add}
			}
			statuses := ownerClient.doBothWhileLocked(lockRepo, deleted.ID, requests[0], requests[1])
			assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses, "deleteFirst: %v", deleteFirst)
			assert.Equal(t, database.AccessModeWrite, accessMode(t, member.ID, kept, org.ID), "deleteFirst: %v", deleteFirst)
		}
	})

	t.Run("deleting the team while adding a member", func(t *testing.T) {
		ownerClient := authenticatedClient(t, owner)
		org, member, repo, teamPath := seed(t, ownerClient)
		statuses := ownerClient.doBothWhileLocked(lockRepo, repo.ID,
			lockedRequest{http.MethodPut, teamPath + "/members/" + member.Name, orgNoBody},
			lockedRequest{http.MethodDelete, teamPath, orgNoBody})
		assert.Equal(t, []int{http.StatusNoContent, http.StatusNoContent}, statuses)
		assert.Equal(t, database.AccessModeNone, accessMode(t, member.ID, repo, org.ID), "the member keeps the access of the deleted team")
		status, body := authenticatedClient(t, member).do(http.MethodGet, issueAPIPath(repo, "issues"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("adding a collaborator while transferring the repository", func(t *testing.T) {
		ownerClient := authenticatedClient(t, owner)
		_, _, repo, _ := seed(t, ownerClient)
		outsider := seedUser(t, CreateUserOptions{})

		// The collaborator request loads the repository before the transfer
		// commits, and must not grant access as if the organization still
		// owned it.
		statuses := ownerClient.doBothWhileLocked(lockRepo, repo.ID,
			lockedRequest{http.MethodPost, repoSettingsAPIPath(repo, "/settings/transfer"), map[string]any{"repoName": repo.Name, "newOwner": owner.Name}},
			lockedRequest{http.MethodPost, repoSettingsAPIPath(repo, "/settings/collaborators"), map[string]any{"username": outsider.Name}})
		assert.Equal(t, []int{http.StatusOK, http.StatusNotFound}, statuses)
		assert.Equal(t, database.AccessModeNone, accessMode(t, outsider.ID, repo, owner.ID))
		collaborators, err := database.Handle.Collaborations().List(t.Context(), repo.ID)
		require.NoError(t, err)
		assert.Empty(t, collaborators)
	})
}
