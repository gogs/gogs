package integrationtest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// repoSettingsPeople returns the names on the first page of a list of people,
// e.g., the stargazers of a repository.
func repoSettingsPeople(t *testing.T, c *client, path string) []string {
	t.Helper()

	var page struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &page)
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		names = append(names, item.Name)
	}
	return names
}

func repoSettingsGetRepo(t *testing.T, id int64) *database.Repository {
	t.Helper()

	repo, err := database.GetRepositoryByID(id)
	require.NoError(t, err)
	return repo
}

func TestRepoSettingsWatch(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	viewer := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	c := repoSettingsSignedIn(t, viewer)
	path := repoSettingsAPIPath(repo, "/watch")
	before := repoSettingsGetRepo(t, repo.ID).NumWatches

	var resp struct {
		WatchCount int `json:"watchCount"`
	}
	c.doJSON(http.MethodPost, path, repoSettingsNoBody, http.StatusOK, &resp)
	assert.Equal(t, before+1, resp.WatchCount)
	assert.True(t, database.IsWatching(viewer.ID, repo.ID))
	assert.Equal(t, before+1, repoSettingsGetRepo(t, repo.ID).NumWatches)
	assert.Contains(t, repoSettingsPeople(t, newClient(t), repoSettingsAPIPath(repo, "/watchers")), viewer.Name)

	c.doJSON(http.MethodDelete, path, repoSettingsNoBody, http.StatusOK, &resp)
	assert.Equal(t, before, resp.WatchCount)
	assert.False(t, database.IsWatching(viewer.ID, repo.ID))
	assert.NotContains(t, repoSettingsPeople(t, c, repoSettingsAPIPath(repo, "/watchers")), viewer.Name)

	status, body := newClient(t).do(http.MethodPost, path, repoSettingsNoBody)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	for _, p := range []string{"/watch", "/watchers"} {
		method := http.MethodPost
		var reqBody any = repoSettingsNoBody
		if p == "/watchers" {
			method, reqBody = http.MethodGet, nil
		}
		status, body = c.do(method, repoSettingsAPIPath(private, p), reqBody)
		assert.Equal(t, http.StatusNotFound, status, "%s, response: %s", p, body)
	}
	assert.False(t, database.IsWatching(viewer.ID, private.ID))
}

func TestRepoSettingsStar(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	viewer := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	c := repoSettingsSignedIn(t, viewer)
	path := repoSettingsAPIPath(repo, "/star")
	assert.Empty(t, repoSettingsPeople(t, c, repoSettingsAPIPath(repo, "/stargazers")))

	var resp struct {
		StarCount int `json:"starCount"`
	}
	c.doJSON(http.MethodPost, path, repoSettingsNoBody, http.StatusOK, &resp)
	assert.Equal(t, 1, resp.StarCount)
	assert.True(t, database.IsStarring(viewer.ID, repo.ID))
	assert.Equal(t, 1, repoSettingsGetRepo(t, repo.ID).NumStars)
	assert.Equal(t, []string{viewer.Name}, repoSettingsPeople(t, newClient(t), repoSettingsAPIPath(repo, "/stargazers")))

	c.doJSON(http.MethodDelete, path, repoSettingsNoBody, http.StatusOK, &resp)
	assert.Equal(t, 0, resp.StarCount)
	assert.False(t, database.IsStarring(viewer.ID, repo.ID))
	assert.Empty(t, repoSettingsPeople(t, c, repoSettingsAPIPath(repo, "/stargazers")))

	status, body := newClient(t).do(http.MethodPost, path, repoSettingsNoBody)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	status, body = c.do(http.MethodPost, repoSettingsAPIPath(private, "/star"), repoSettingsNoBody)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = c.do(http.MethodGet, repoSettingsAPIPath(private, "/stargazers"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	assert.False(t, database.IsStarring(viewer.ID, private.ID))
}

func TestRepoSettingsRemoveWatcher(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	watcher := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{Admin: true})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	require.NoError(t, database.Handle.Repositories().Watch(t.Context(), watcher.ID, repo.ID))
	before := repoSettingsGetRepo(t, repo.ID).NumWatches
	path := repoSettingsAPIPath(repo, "/watchers/"+watcher.Name)

	// Only site admins make others stop watching, not even the owner.
	status, body := repoSettingsSignedIn(t, owner).do(http.MethodDelete, path, repoSettingsNoBody)
	require.Equal(t, http.StatusForbidden, status, "response: %s", body)
	assert.True(t, database.IsWatching(watcher.ID, repo.ID))

	c := repoSettingsSignedIn(t, admin)
	var resp struct {
		WatchCount int `json:"watchCount"`
	}
	c.doJSON(http.MethodDelete, path, repoSettingsNoBody, http.StatusOK, &resp)
	assert.Equal(t, before-1, resp.WatchCount)
	assert.False(t, database.IsWatching(watcher.ID, repo.ID))
	assert.NotContains(t, repoSettingsPeople(t, c, repoSettingsAPIPath(repo, "/watchers")), watcher.Name)

	status, body = c.do(http.MethodDelete, repoSettingsAPIPath(repo, "/watchers/"+UniqueName("nobody-")), repoSettingsNoBody)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}
