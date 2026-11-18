package integrationtests

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

type apiV1Release struct {
	ID              int64     `json:"id"`
	TagName         string    `json:"tag_name"`
	TargetCommitish string    `json:"target_commitish"`
	Name            string    `json:"name"`
	Body            string    `json:"body"`
	Draft           bool      `json:"draft"`
	Prerelease      bool      `json:"prerelease"`
	Author          apiV1User `json:"author"`
	Created         time.Time `json:"created_at"`
}

// apiV1NextReleaseEvent returns the action and tag of the next release webhook
// delivery.
func apiV1NextReleaseEvent(t *testing.T, deliveries <-chan repoSettingsDelivery) (action, tag string) {
	t.Helper()

	d := repoSettingsNextDelivery(t, deliveries)
	require.Equal(t, "release", d.Header.Get("X-Gogs-Event"))
	var payload struct {
		Action  string       `json:"action"`
		Release apiV1Release `json:"release"`
	}
	require.NoError(t, json.Unmarshal(d.Body, &payload), "payload: %s", d.Body)
	return payload.Action, payload.Release.TagName
}

func TestAPIV1Releases(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	webhookURL, deliveries := repoSettingsWebhookReceiver(t)
	repoSettingsCreateWebhookFor(t, authenticatedClient(t, owner), repo, webhookURL, true, map[string]bool{"release": true})

	mainSHA := repoSettingsGit(t, repoPath, "rev-parse", "refs/heads/main")
	featureSHA := apiV1PutFile(t, api, repo, "feature.txt", "Feature", "feature")
	// An existing tag on an old commit that no branch points at.
	commitTree := exec.Command("git", "commit-tree", "-p", featureSHA, "-m", "Old", featureSHA+"^{tree}")
	commitTree.Dir = repoPath
	commitTree.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com", "GIT_AUTHOR_DATE=2001-02-03T04:05:06Z",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	out, err := commitTree.CombinedOutput()
	require.NoError(t, err, "git commit-tree: %s", out)
	oldSHA := strings.TrimSpace(string(out))
	repoSettingsGit(t, repoPath, "tag", "existing", oldSHA)

	create := func(body map[string]any) apiV1Release {
		t.Helper()
		var got apiV1Release
		api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/releases"), body, http.StatusCreated, &got)
		require.NotZero(t, got.ID)
		return got
	}
	releasePath := func(id int64) string {
		return apiV1RepoPath(repo, "/releases/"+apiV1Itoa(id))
	}

	v1 := create(map[string]any{"tag_name": "v1", "name": "First", "body": "Notes", "prerelease": true})
	assert.Equal(t, apiV1Release{
		ID: v1.ID, TagName: "v1", TargetCommitish: "main", Name: "First", Body: "Notes", Prerelease: true,
		Author: v1.Author, Created: v1.Created,
	}, v1)
	assert.Equal(t, owner.Name, v1.Author.UserName)
	assert.Equal(t, mainSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/v1"))
	action, tag := apiV1NextReleaseEvent(t, deliveries)
	assert.Equal(t, [2]string{"published", "v1"}, [2]string{action, tag})

	// The existing tag stays on its commit, not on the target branch, and the
	// release is dated by that commit.
	existing := create(map[string]any{"tag_name": "existing"})
	assert.Equal(t, oldSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/existing"))
	assert.Equal(t, time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC).Unix(), existing.Created.Unix())
	_, tag = apiV1NextReleaseEvent(t, deliveries)
	assert.Equal(t, "existing", tag)

	draft := create(map[string]any{"tag_name": "v2", "target_commitish": "feature", "name": "Upcoming", "draft": true})
	assert.True(t, draft.Draft)
	assert.Equal(t, "feature", draft.TargetCommitish)
	assert.Empty(t, repoSettingsGit(t, repoPath, "tag", "--list", "v2"), "a draft has no tag")

	t.Run("invalid creation", func(t *testing.T) {
		pwned := filepath.Join(t.TempDir(), "pwned")
		for _, body := range []map[string]any{
			{},
			{"tag_name": "v1"},
			// Tag names are unique regardless of their case.
			{"tag_name": "V1"},
			{"tag_name": "-x"},
			{"tag_name": "a b"},
			{"tag_name": "v3", "target_commitish": "nope"},
			{"tag_name": "v3", "target_commitish": "--output=" + pwned},
			// Only branches are targets.
			{"tag_name": "v3", "target_commitish": mainSHA},
			{"tag_name": "v3", "name": strings.Repeat("a", 256)},
			{"tag_name": strings.Repeat("v", 256)},
		} {
			status, resp := api.do(http.MethodPost, apiV1RepoPath(repo, "/releases"), body)
			assert.Equal(t, http.StatusUnprocessableEntity, status, "%v, response: %s", body, resp)
		}
		assert.NoFileExists(t, pwned)
		assert.Empty(t, repoSettingsGit(t, repoPath, "for-each-ref", "refs/tags/v3", "refs/tags/V1", "refs/tags/-x"))
	})

	t.Run("get and edit", func(t *testing.T) {
		var got apiV1Release
		api.doJSON(http.MethodGet, releasePath(draft.ID), nil, http.StatusOK, &got)
		assert.Equal(t, draft, got)

		// Only the fields sent change.
		api.doJSON(http.MethodPatch, releasePath(draft.ID), map[string]any{"name": "Renamed"}, http.StatusOK, &got)
		want := draft
		want.Name = "Renamed"
		assert.Equal(t, want, got)
		assert.Empty(t, repoSettingsGit(t, repoPath, "tag", "--list", "v2"), "editing a draft does not tag it")

		api.doJSON(http.MethodPatch, releasePath(draft.ID), map[string]any{"draft": false, "body": "Out now"}, http.StatusOK, &got)
		assert.False(t, got.Draft)
		assert.Equal(t, "Renamed", got.Name)
		assert.Equal(t, "Out now", got.Body)
		assert.Equal(t, featureSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/v2"), "published on the target branch")
		action, tag := apiV1NextReleaseEvent(t, deliveries)
		assert.Equal(t, [2]string{"published", "v2"}, [2]string{action, tag})

		// A name that does not fit fails before the draft is published.
		d := create(map[string]any{"tag_name": "v5", "draft": true})
		status, body := api.do(http.MethodPatch, releasePath(d.ID), map[string]any{"draft": false, "name": strings.Repeat("é", 256)})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		assert.Empty(t, repoSettingsGit(t, repoPath, "tag", "--list", "v5"))
		api.doJSON(http.MethodPatch, releasePath(d.ID), map[string]any{"name": strings.Repeat("é", 255)}, http.StatusOK, &got)
		assert.True(t, got.Draft)
		assert.Equal(t, strings.Repeat("é", 255), got.Name)

		status, body = api.do(http.MethodPatch, releasePath(draft.ID), map[string]any{"draft": true})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		api.doJSON(http.MethodGet, releasePath(draft.ID), nil, http.StatusOK, &got)
		assert.False(t, got.Draft)

		// Releases of other repositories and unknown IDs are not found.
		other := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
			for _, path := range []string{
				apiV1RepoPath(other, "/releases/"+apiV1Itoa(v1.ID)),
				apiV1RepoPath(repo, "/releases/999999999"),
			} {
				status, body := api.do(method, path, map[string]any{"name": "x"})
				assert.Equal(t, http.StatusNotFound, status, "%s %s, response: %s", method, path, body)
			}
		}
		api.doJSON(http.MethodGet, releasePath(v1.ID), nil, http.StatusOK, &got)
		assert.Equal(t, "First", got.Name)
	})

	t.Run("publish draft without target branch", func(t *testing.T) {
		d := create(map[string]any{"tag_name": "v4", "target_commitish": "feature", "draft": true})
		repoSettingsGit(t, repoPath, "branch", "-D", "feature")
		t.Cleanup(func() { repoSettingsGit(t, repoPath, "branch", "feature", featureSHA) })

		status, body := api.do(http.MethodPatch, releasePath(d.ID), map[string]any{"draft": false})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		var got apiV1Release
		api.doJSON(http.MethodGet, releasePath(d.ID), nil, http.StatusOK, &got)
		assert.True(t, got.Draft)
		assert.Empty(t, repoSettingsGit(t, repoPath, "tag", "--list", "v4"))

		status, body = api.do(http.MethodDelete, releasePath(d.ID), nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})

	t.Run("delete", func(t *testing.T) {
		status, body := api.do(http.MethodDelete, releasePath(existing.ID), nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, repoSettingsGit(t, repoPath, "tag", "--list", "existing"), "the tag is deleted with the release")
		status, body = api.do(http.MethodGet, releasePath(existing.ID), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = api.do(http.MethodDelete, releasePath(existing.ID), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		// The tag can be released again.
		again := create(map[string]any{"tag_name": "existing"})
		assert.Equal(t, mainSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/existing"))
		_, tag := apiV1NextReleaseEvent(t, deliveries)
		assert.Equal(t, "existing", tag)
		status, body = api.do(http.MethodDelete, releasePath(again.ID), nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})

	t.Run("mirror", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		mirror, _ := repoSettingsMigrate(t, authenticatedClient(t, u), u, repo, true)
		status, body := apiV1Client(t, u).do(http.MethodPost, apiV1RepoPath(mirror, "/releases"), map[string]any{"tag_name": "mirrored"})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
	})
}

// Drafts are only visible to users with write access, and writing needs write
// access.
func TestAPIV1ReleasesAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, owner)
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	type releases struct{ published, draft apiV1Release }
	seed := func(repo *Repository) releases {
		var r releases
		api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/releases"), map[string]any{"tag_name": "v1"}, http.StatusCreated, &r.published)
		api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/releases"), map[string]any{"tag_name": "v2", "draft": true}, http.StatusCreated, &r.draft)
		return r
	}
	publicReleases := seed(public)
	privateReleases := seed(private)

	addCollaborator := func(repo *Repository, permission string) *client {
		u := seedUser(t, CreateUserOptions{})
		status, body := api.do(http.MethodPut, apiV1RepoPath(repo, "/collaborators/"+u.Name), map[string]any{"permission": permission})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		return apiV1Client(t, u)
	}
	listTags := func(c *client, repo *Repository) []string {
		t.Helper()
		var list []apiV1Release
		c.doJSON(http.MethodGet, apiV1RepoPath(repo, "/releases"), nil, http.StatusOK, &list)
		var tags []string
		for _, r := range list {
			tags = append(tags, r.TagName)
		}
		return tags
	}
	getStatus := func(c *client, repo *Repository, id int64) int {
		t.Helper()
		status, _ := c.do(http.MethodGet, apiV1RepoPath(repo, "/releases/"+apiV1Itoa(id)), nil)
		return status
	}

	t.Run("public repository", func(t *testing.T) {
		for name, c := range map[string]struct {
			client    *client
			wantTags  []string
			wantDraft int
		}{
			"owner":     {api, []string{"v1", "v2"}, http.StatusOK},
			"writer":    {addCollaborator(public, "write"), []string{"v1", "v2"}, http.StatusOK},
			"reader":    {addCollaborator(public, "read"), []string{"v1"}, http.StatusNotFound},
			"other":     {apiV1Client(t, seedUser(t, CreateUserOptions{})), []string{"v1"}, http.StatusNotFound},
			"anonymous": {newClient(t), []string{"v1"}, http.StatusNotFound},
		} {
			assert.ElementsMatch(t, c.wantTags, listTags(c.client, public), name)
			assert.Equal(t, c.wantDraft, getStatus(c.client, public, publicReleases.draft.ID), "%s gets the draft", name)
			assert.Equal(t, http.StatusOK, getStatus(c.client, public, publicReleases.published.ID), "%s gets the published release", name)
		}
	})

	t.Run("private repository", func(t *testing.T) {
		for name, c := range map[string]*client{
			"other":     apiV1Client(t, seedUser(t, CreateUserOptions{})),
			"anonymous": newClient(t),
		} {
			status, body := c.do(http.MethodGet, apiV1RepoPath(private, "/releases"), nil)
			assert.Equal(t, http.StatusNotFound, status, "%s lists, response: %s", name, body)
			for _, id := range []int64{privateReleases.published.ID, privateReleases.draft.ID} {
				assert.Equal(t, http.StatusNotFound, getStatus(c, private, id), "%s gets %d", name, id)
			}
		}
		reader := addCollaborator(private, "read")
		assert.Equal(t, []string{"v1"}, listTags(reader, private))
		assert.Equal(t, http.StatusNotFound, getStatus(reader, private, privateReleases.draft.ID))
	})

	t.Run("writing", func(t *testing.T) {
		path := apiV1RepoPath(public, "/releases/"+apiV1Itoa(publicReleases.published.ID))
		for name, c := range map[string]struct {
			client *client
			want   int
		}{
			"reader":    {addCollaborator(public, "read"), http.StatusForbidden},
			"other":     {apiV1Client(t, seedUser(t, CreateUserOptions{})), http.StatusForbidden},
			"anonymous": {newClient(t), http.StatusUnauthorized},
		} {
			status, body := c.client.do(http.MethodPost, apiV1RepoPath(public, "/releases"), map[string]any{"tag_name": "v9"})
			assert.Equal(t, c.want, status, "%s creates, response: %s", name, body)
			status, body = c.client.do(http.MethodPatch, path, map[string]any{"name": "Hijacked"})
			assert.Equal(t, c.want, status, "%s edits, response: %s", name, body)
			status, body = c.client.do(http.MethodDelete, path, nil)
			assert.Equal(t, c.want, status, "%s deletes, response: %s", name, body)
		}

		writer := addCollaborator(public, "write")
		var got apiV1Release
		writer.doJSON(http.MethodPatch, path, map[string]any{"name": "By writer"}, http.StatusOK, &got)
		assert.Equal(t, "By writer", got.Name)
		var created apiV1Release
		writer.doJSON(http.MethodPost, apiV1RepoPath(public, "/releases"), map[string]any{"tag_name": "v3"}, http.StatusCreated, &created)
		status, body := writer.do(http.MethodDelete, apiV1RepoPath(public, "/releases/"+apiV1Itoa(created.ID)), nil)
		assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})
}
