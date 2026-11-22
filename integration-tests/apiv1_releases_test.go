package integrationtests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

type apiV1Release struct {
	ID              int64               `json:"id"`
	TagName         string              `json:"tag_name"`
	TargetCommitish string              `json:"target_commitish"`
	Name            string              `json:"name"`
	Body            string              `json:"body"`
	Draft           bool                `json:"draft"`
	Prerelease      bool                `json:"prerelease"`
	Author          apiV1User           `json:"author"`
	Created         time.Time           `json:"created_at"`
	Assets          []apiV1ReleaseAsset `json:"assets"`
}

type apiV1ReleaseAsset struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	ContentType        string    `json:"content_type"`
	Size               int64     `json:"size"`
	Created            time.Time `json:"created_at"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

// apiV1UploadAsset uploads the content as a file of the name to the release.
func apiV1UploadAsset(t *testing.T, c *client, repo *Repository, releaseID int64, name string, content []byte) (int, []byte) {
	t.Helper()

	path := apiV1RepoPath(repo, "/releases/"+apiV1Itoa(releaseID)+"/assets?name="+url.QueryEscape(name))
	status, _, body := c.doRaw(http.MethodPost, path, "application/octet-stream", bytes.NewReader(content))
	return status, body
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

// releaseOldCommitTime is when the commits of releaseOldCommit were authored.
var releaseOldCommitTime = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// releaseOldCommit makes a commit on top of the parent, authored at
// releaseOldCommitTime, without updating any branch, and returns its ID.
func releaseOldCommit(t *testing.T, repoPath, parent string) string {
	t.Helper()

	cmd := exec.Command("git", "commit-tree", "-p", parent, "-m", "Old", parent+"^{tree}")
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com", "GIT_AUTHOR_DATE="+releaseOldCommitTime.Format(time.RFC3339),
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit-tree: %s", out)
	return strings.TrimSpace(string(out))
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
	oldSHA := releaseOldCommit(t, repoPath, featureSHA)
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
		Author: v1.Author, Created: v1.Created, Assets: []apiV1ReleaseAsset{},
	}, v1)
	assert.Equal(t, owner.Name, v1.Author.UserName)
	assert.Equal(t, mainSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/v1"))
	action, tag := apiV1NextReleaseEvent(t, deliveries)
	assert.Equal(t, [2]string{"published", "v1"}, [2]string{action, tag})

	// The existing tag stays on its commit, not on the target branch, and the
	// release is dated by that commit.
	existing := create(map[string]any{"tag_name": "existing"})
	assert.Equal(t, oldSHA, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/existing"))
	assert.Equal(t, releaseOldCommitTime.Unix(), existing.Created.Unix())
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

	t.Run("deleted publisher", func(t *testing.T) {
		writer := seedUser(t, CreateUserOptions{})
		status, body := api.do(http.MethodPut, apiV1RepoPath(repo, "/collaborators/"+writer.Name), map[string]any{"permission": "write"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		var rel apiV1Release
		apiV1Client(t, writer).doJSON(http.MethodPost, apiV1RepoPath(repo, "/releases"), map[string]any{"tag_name": "by-deleted"}, http.StatusCreated, &rel)
		apiV1NextReleaseEvent(t, deliveries)
		require.NoError(t, database.Handle.Users().DeleteByID(t.Context(), writer.ID, true))

		var list []apiV1Release
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/releases"), nil, http.StatusOK, &list)
		authors := make(map[string]string)
		for _, r := range list {
			authors[r.TagName] = r.Author.UserName
		}
		assert.Equal(t, "Ghost", authors["by-deleted"])
		assert.Equal(t, owner.Name, authors["v1"])
		api.doJSON(http.MethodGet, releasePath(rel.ID), nil, http.StatusOK, &rel)
		assert.Equal(t, "Ghost", rel.Author.UserName)
	})

	t.Run("assets", func(t *testing.T) {
		rel := create(map[string]any{"tag_name": "with-assets", "draft": true})
		assetPath := func(id int64) string {
			return apiV1RepoPath(repo, "/releases/assets/"+apiV1Itoa(id))
		}
		upload := func(name, content string) apiV1ReleaseAsset {
			t.Helper()
			status, body := apiV1UploadAsset(t, api, repo, rel.ID, name, []byte(content))
			require.Equal(t, http.StatusCreated, status, "response: %s", body)
			var asset apiV1ReleaseAsset
			require.NoError(t, json.Unmarshal(body, &asset))
			return asset
		}
		download := func(asset apiV1ReleaseAsset) (int, string) {
			t.Helper()
			require.True(t, strings.HasPrefix(asset.BrowserDownloadURL, serverURL+"/attachments/"), asset.BrowserDownloadURL)
			status, _, body := newClient(t).doRaw(http.MethodGet, strings.TrimPrefix(asset.BrowserDownloadURL, serverURL), "", nil)
			return status, string(body)
		}

		sums := upload("checksums.json", `{"app.tar.gz":"abc"}`)
		binary := upload("app.bin", "binary build")

		// Publishing sends the files the draft has with the release.
		api.doJSON(http.MethodPatch, releasePath(rel.ID), map[string]any{"draft": false}, http.StatusOK, &rel)
		d := repoSettingsNextDelivery(t, deliveries)
		var payload struct {
			Release apiV1Release `json:"release"`
		}
		require.NoError(t, json.Unmarshal(d.Body, &payload), "payload: %s", d.Body)
		assert.Equal(t, "with-assets", payload.Release.TagName)
		assert.Equal(t, []apiV1ReleaseAsset{sums, binary}, payload.Release.Assets)

		assert.Equal(t, "checksums.json", sums.Name)
		assert.Equal(t, "application/json", sums.ContentType)
		assert.Equal(t, int64(len(`{"app.tar.gz":"abc"}`)), sums.Size)
		assert.WithinDuration(t, time.Now(), sums.Created, time.Minute)
		status, content := download(sums)
		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, `{"app.tar.gz":"abc"}`, content)

		var list []apiV1ReleaseAsset
		api.doJSON(http.MethodGet, releasePath(rel.ID)+"/assets", nil, http.StatusOK, &list)
		assert.Equal(t, []apiV1ReleaseAsset{sums, binary}, list)
		var got apiV1Release
		api.doJSON(http.MethodGet, releasePath(rel.ID), nil, http.StatusOK, &got)
		assert.Equal(t, []apiV1ReleaseAsset{sums, binary}, got.Assets)
		var asset apiV1ReleaseAsset
		api.doJSON(http.MethodGet, assetPath(sums.ID), nil, http.StatusOK, &asset)
		assert.Equal(t, sums, asset)

		// Editing the release keeps its files.
		api.doJSON(http.MethodPatch, releasePath(rel.ID), map[string]any{"body": "With files"}, http.StatusOK, &got)
		assert.Len(t, got.Assets, 2)

		api.doJSON(http.MethodPatch, assetPath(sums.ID), map[string]any{"name": "sums.json"}, http.StatusOK, &asset)
		assert.Equal(t, "sums.json", asset.Name)
		assert.Equal(t, sums.BrowserDownloadURL, asset.BrowserDownloadURL)

		for _, name := range []string{"", "..", "dir/app.bin", `dir\app.bin`, "a\nb", strings.Repeat("a", 256), "app.bin"} {
			status, body := apiV1UploadAsset(t, api, repo, rel.ID, name, []byte("x"))
			assert.Equal(t, http.StatusUnprocessableEntity, status, "upload %q, response: %s", name, body)
		}
		for _, name := range []string{"app.bin", "a/b"} {
			status, body := api.do(http.MethodPatch, assetPath(sums.ID), map[string]any{"name": name})
			assert.Equal(t, http.StatusUnprocessableEntity, status, "rename to %q, response: %s", name, body)
		}
		api.doJSON(http.MethodGet, releasePath(rel.ID)+"/assets", nil, http.StatusOK, &list)
		assert.Len(t, list, 2, "no file stored by an invalid upload")

		// Files of releases of other repositories and files that are not
		// attached to a release are not found.
		other := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		unattached, err := database.Handle.Attachments().Create(t.Context(), owner.ID, "loose.bin", strings.NewReader("loose"))
		require.NoError(t, err)
		for _, path := range []string{
			apiV1RepoPath(other, "/releases/assets/"+apiV1Itoa(sums.ID)),
			assetPath(unattached.ID),
			assetPath(999999999),
		} {
			for _, method := range []string{http.MethodGet, http.MethodPatch, http.MethodDelete} {
				status, body := api.do(method, path, map[string]any{"name": "x.bin"})
				assert.Equal(t, http.StatusNotFound, status, "%s %s, response: %s", method, path, body)
			}
		}
		status, body := apiV1UploadAsset(t, api, other, rel.ID, "x.bin", []byte("x"))
		assert.Equal(t, http.StatusNotFound, status, "upload through another repository, response: %s", body)

		status, body = api.do(http.MethodDelete, assetPath(sums.ID), nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, body = api.do(http.MethodGet, assetPath(sums.ID), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, _ = download(sums)
		assert.Equal(t, http.StatusNotFound, status, "file of the deleted asset")

		// Deleting the release deletes its files.
		status, body = api.do(http.MethodDelete, releasePath(rel.ID), nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, _ = download(binary)
		assert.Equal(t, http.StatusNotFound, status, "file of the deleted release")
	})

	t.Run("mirror", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		mirror, _ := repoSettingsMigrate(t, authenticatedClient(t, u), u, repo, true)
		mirrorAPI := apiV1Client(t, u)
		status, body := mirrorAPI.do(http.MethodPost, apiV1RepoPath(mirror, "/releases"), map[string]any{"tag_name": "mirrored"})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

		// A draft that came with the repository cannot be published either.
		d := apiV1SeedDraft(t, mirror, u.ID, "mirrored")
		status, body = mirrorAPI.do(http.MethodPatch, apiV1RepoPath(mirror, "/releases/"+apiV1Itoa(d)), map[string]any{"draft": false})
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		assert.Empty(t, repoSettingsGit(t, database.RepoPath(mirror.Owner, mirror.Name), "tag", "--list", "mirrored"))
	})

	t.Run("publish draft with invalid tag name", func(t *testing.T) {
		// The web interface used to store drafts without checking the tag name.
		d := apiV1SeedDraft(t, repo, owner.ID, "-v6")
		status, body := api.do(http.MethodPatch, releasePath(d), map[string]any{"draft": false})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		assert.Empty(t, repoSettingsGit(t, repoPath, "for-each-ref", "refs/tags/v6", "refs/tags/-v6"))
	})
}

// apiV1SeedDraft stores a draft release of the tag in the repository without
// going through any endpoint, and returns its ID.
func apiV1SeedDraft(t *testing.T, repo *Repository, publisherID int64, tagName string) int64 {
	t.Helper()

	gitRepo, err := git.Open(database.RepoPath(repo.Owner, repo.Name))
	require.NoError(t, err)
	r := &database.Release{RepoID: repo.ID, PublisherID: publisherID, TagName: tagName, Target: "main", IsDraft: true}
	require.NoError(t, database.Handle.Releases().Create(t.Context(), logger, gitRepo, r, nil))
	return r.ID
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
	uploadAsset := func(repo *Repository, releaseID int64) apiV1ReleaseAsset {
		status, body := apiV1UploadAsset(t, api, repo, releaseID, "app.bin", []byte("binary build"))
		require.Equal(t, http.StatusCreated, status, "response: %s", body)
		var asset apiV1ReleaseAsset
		require.NoError(t, json.Unmarshal(body, &asset))
		return asset
	}
	publishedAsset := uploadAsset(public, publicReleases.published.ID)
	draftAsset := uploadAsset(public, publicReleases.draft.ID)
	privateAsset := uploadAsset(private, privateReleases.published.ID)

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

			// The files of a draft are hidden like the draft.
			for path, want := range map[string]int{
				"/releases/" + apiV1Itoa(publicReleases.draft.ID) + "/assets":     c.wantDraft,
				"/releases/assets/" + apiV1Itoa(draftAsset.ID):                    c.wantDraft,
				"/releases/" + apiV1Itoa(publicReleases.published.ID) + "/assets": http.StatusOK,
				"/releases/assets/" + apiV1Itoa(publishedAsset.ID):                http.StatusOK,
			} {
				status, body := c.client.do(http.MethodGet, apiV1RepoPath(public, path), nil)
				assert.Equal(t, want, status, "%s gets %s, response: %s", name, path, body)
			}
		}

		// 🚨 SECURITY: Unauthenticated callers do not see the email of the
		// publisher.
		for name, c := range map[string]struct {
			client    *client
			wantEmail string
		}{
			"other":     {apiV1Client(t, seedUser(t, CreateUserOptions{})), owner.Email},
			"anonymous": {newClient(t), ""},
		} {
			var got apiV1Release
			c.client.doJSON(http.MethodGet, apiV1RepoPath(public, "/releases/"+apiV1Itoa(publicReleases.published.ID)), nil, http.StatusOK, &got)
			assert.Equal(t, c.wantEmail, got.Author.Email, "%s gets", name)
			var list []apiV1Release
			c.client.doJSON(http.MethodGet, apiV1RepoPath(public, "/releases"), nil, http.StatusOK, &list)
			require.Len(t, list, 1, name)
			assert.Equal(t, c.wantEmail, list[0].Author.Email, "%s lists", name)
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
			status, body = c.do(http.MethodGet, apiV1RepoPath(private, "/releases/assets/"+apiV1Itoa(privateAsset.ID)), nil)
			assert.Equal(t, http.StatusNotFound, status, "%s gets the asset, response: %s", name, body)
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

			status, body = apiV1UploadAsset(t, c.client, public, publicReleases.published.ID, "intruder.bin", []byte("x"))
			assert.Equal(t, c.want, status, "%s uploads, response: %s", name, body)
			assetPath := apiV1RepoPath(public, "/releases/assets/"+apiV1Itoa(publishedAsset.ID))
			status, body = c.client.do(http.MethodPatch, assetPath, map[string]any{"name": "hijacked.bin"})
			assert.Equal(t, c.want, status, "%s renames the asset, response: %s", name, body)
			status, body = c.client.do(http.MethodDelete, assetPath, nil)
			assert.Equal(t, c.want, status, "%s deletes the asset, response: %s", name, body)
		}

		writer := addCollaborator(public, "write")
		status, body := apiV1UploadAsset(t, writer, public, publicReleases.published.ID, "by-writer.bin", []byte("x"))
		assert.Equal(t, http.StatusCreated, status, "response: %s", body)
		var got apiV1Release
		writer.doJSON(http.MethodPatch, path, map[string]any{"name": "By writer"}, http.StatusOK, &got)
		assert.Equal(t, "By writer", got.Name)
		var created apiV1Release
		writer.doJSON(http.MethodPost, apiV1RepoPath(public, "/releases"), map[string]any{"tag_name": "v3"}, http.StatusCreated, &created)
		status, body = writer.do(http.MethodDelete, apiV1RepoPath(public, "/releases/"+apiV1Itoa(created.ID)), nil)
		assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
	})
}
