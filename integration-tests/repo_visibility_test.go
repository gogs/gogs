package integrationtests

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/dbx"
)

// repoVisibilityForcePrivateClient returns a client of the server with
// FORCE_PRIVATE, signed in as the user.
func repoVisibilityForcePrivateClient(t *testing.T, u *User) *client {
	t.Helper()

	c := newClient(t)
	c.baseURL = forcePrivateServerURL
	c.signIn(u)
	return c
}

// repoVisibilityGet returns the repository with the given ID, failing the test
// when it cannot.
func repoVisibilityGet(t *testing.T, id int64) *database.Repository {
	t.Helper()

	repo, err := getRepository(t, id)
	require.NoError(t, err)
	return repo
}

// repoVisibilityPatch saves the general settings of the repository with the
// visibility.
func repoVisibilityPatch(t *testing.T, c *client, repo *Repository, private, unlisted bool) {
	t.Helper()

	status, body := c.do(http.MethodPatch, repoSettingsAPIPath(repo, "/settings"), map[string]any{
		"name":     repo.Name,
		"private":  private,
		"unlisted": unlisted,
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
}

// repoVisibilityPutPublicAccess saves the advanced settings of the repository
// with the wiki and the issues on, and their public access as given.
func repoVisibilityPutPublicAccess(t *testing.T, c *client, repo *Repository, publicWiki, publicIssues bool) {
	t.Helper()

	status, body := c.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), map[string]any{
		"enableWiki":        true,
		"allowPublicWiki":   publicWiki,
		"enableIssues":      true,
		"allowPublicIssues": publicIssues,
		"enablePulls":       true,
		"pullsAllowMerge":   true,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
}

// repoVisibilityFork forks the repository through the web API as the client
// and returns the fork.
func repoVisibilityFork(t *testing.T, c *client, base *Repository, forker *User) *Repository {
	t.Helper()

	var created repoSettingsCreated
	c.doJSON(http.MethodPost, "/api/web/repo/fork/"+strconv.FormatInt(base.ID, 10), map[string]any{"name": UniqueName("fork-")}, http.StatusCreated, &created)
	fork, err := getRepositoryByName(t, forker.ID, created.Name)
	require.NoError(t, err)
	return &Repository{ID: fork.ID, Owner: created.Owner, Name: created.Name}
}

type repoVisibilitySettings struct {
	ForcePrivate         bool `json:"forcePrivate"`
	CanAllowPublicWiki   bool `json:"canAllowPublicWiki"`
	CanAllowPublicIssues bool `json:"canAllowPublicIssues"`
}

func TestRepoForcePrivate(t *testing.T) {
	t.Parallel()

	admin := seedUser(t, CreateUserOptions{Admin: true})
	owner := seedUser(t, CreateUserOptions{})
	ownerClient := repoVisibilityForcePrivateClient(t, owner)
	adminClient := repoVisibilityForcePrivateClient(t, admin)

	t.Run("API v1 creates private repositories", func(t *testing.T) {
		create := map[string]any{"name": UniqueName("repo-"), "private": false}
		forcePrivateClient := apiV1Client(t, owner)
		forcePrivateClient.baseURL = forcePrivateServerURL
		var created apiV1Repo
		forcePrivateClient.doJSON(http.MethodPost, "/api/v1/user/repos", create, http.StatusCreated, &created)
		assert.True(t, created.Private)
		assert.True(t, repoVisibilityGet(t, created.ID).IsPrivate)

		// The server without FORCE_PRIVATE creates the repository as asked.
		create["name"] = UniqueName("repo-")
		apiV1Client(t, owner).doJSON(http.MethodPost, "/api/v1/user/repos", create, http.StatusCreated, &created)
		assert.False(t, repoVisibilityGet(t, created.ID).IsPrivate)
	})

	t.Run("only site admins make a private repository public", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})

		var settings repoVisibilitySettings
		ownerClient.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
		assert.True(t, settings.ForcePrivate)
		adminClient.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
		assert.False(t, settings.ForcePrivate)

		repoVisibilityPatch(t, ownerClient, repo, false, false)
		assert.True(t, repoVisibilityGet(t, repo.ID).IsPrivate, "owner made the repository public")

		repoVisibilityPatch(t, adminClient, repo, false, false)
		assert.False(t, repoVisibilityGet(t, repo.ID).IsPrivate)

		// A public repository stays public when its owner saves its settings, and
		// its owner can make it private again.
		ownerClient.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
		assert.False(t, settings.ForcePrivate)
		repoVisibilityPatch(t, ownerClient, repo, false, false)
		assert.False(t, repoVisibilityGet(t, repo.ID).IsPrivate)
		repoVisibilityPatch(t, ownerClient, repo, true, false)
		assert.True(t, repoVisibilityGet(t, repo.ID).IsPrivate)
	})

	t.Run("forks of public repositories are private", func(t *testing.T) {
		forker := seedUser(t, CreateUserOptions{})
		forkerClient := repoVisibilityForcePrivateClient(t, forker)
		base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

		var options struct {
			ForcePrivate bool `json:"forcePrivate"`
		}
		forkerClient.doJSON(http.MethodGet, "/api/web/repo/fork/"+strconv.FormatInt(base.ID, 10), nil, http.StatusOK, &options)
		assert.True(t, options.ForcePrivate)
		fork := repoVisibilityFork(t, forkerClient, base, forker)
		assert.True(t, repoVisibilityGet(t, fork.ID).IsPrivate)

		// A visibility change of the base no longer makes the fork follow it to
		// public.
		repoVisibilityPatch(t, ownerClient, base, false, true)
		got := repoVisibilityGet(t, fork.ID)
		assert.True(t, got.IsPrivate, "fork followed its base to public")
		assert.True(t, got.IsUnlisted)

		repoVisibilityPatch(t, forkerClient, fork, false, false)
		assert.True(t, repoVisibilityGet(t, fork.ID).IsPrivate, "owner made the fork public")

		// Forks have their own visibility, which a site admin saving other settings
		// keeps, and changes in the danger zone.
		var settings struct {
			VisibilityFollowsBase bool `json:"visibilityFollowsBase"`
			CanBePublic           bool `json:"canBePublic"`
			CanBeListed           bool `json:"canBeListed"`
		}
		adminClient.doJSON(http.MethodGet, repoSettingsAPIPath(fork, "/settings"), nil, http.StatusOK, &settings)
		assert.False(t, settings.VisibilityFollowsBase)
		assert.True(t, settings.CanBePublic)
		assert.False(t, settings.CanBeListed)
		repoVisibilityPatch(t, adminClient, fork, true, false)
		assert.True(t, repoVisibilityGet(t, fork.ID).IsPrivate, "site admin saving other settings made the fork public")

		repoVisibilityPatch(t, adminClient, fork, false, false)
		got = repoVisibilityGet(t, fork.ID)
		assert.False(t, got.IsPrivate)
		assert.True(t, got.IsUnlisted, "fork more visible than its unlisted base")

		// Listing the base keeps the fork as it is, rather than listing it too.
		repoVisibilityPatch(t, ownerClient, base, false, false)
		got = repoVisibilityGet(t, fork.ID)
		assert.False(t, got.IsPrivate)
		assert.True(t, got.IsUnlisted, "fork followed its base to listed")
	})

	t.Run("private forks of unlisted repositories lose public access", func(t *testing.T) {
		forker := seedUser(t, CreateUserOptions{})
		forkerClient := repoVisibilityForcePrivateClient(t, forker)
		base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		fork := repoVisibilityFork(t, forkerClient, base, forker)

		// Explore lists private repositories that allow public access, which a fork
		// of a listed public repository may be.
		repoVisibilityPutPublicAccess(t, forkerClient, fork, true, true)
		got := repoVisibilityGet(t, fork.ID)
		assert.True(t, got.AllowPublicWiki)
		assert.True(t, got.AllowPublicIssues)

		repoVisibilityPatch(t, ownerClient, base, false, true)
		got = repoVisibilityGet(t, fork.ID)
		assert.False(t, got.AllowPublicWiki)
		assert.False(t, got.AllowPublicIssues)

		var settings repoVisibilitySettings
		forkerClient.doJSON(http.MethodGet, repoSettingsAPIPath(fork, "/settings"), nil, http.StatusOK, &settings)
		assert.False(t, settings.CanAllowPublicWiki)
		assert.False(t, settings.CanAllowPublicIssues)
	})
}

func TestRepoForkVisibility(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	forker := seedUser(t, CreateUserOptions{})
	ownerClient := authenticatedClient(t, owner)
	forkerClient := authenticatedClient(t, forker)

	t.Run("forks follow their base", func(t *testing.T) {
		base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		fork := repoVisibilityFork(t, forkerClient, base, forker)
		assert.False(t, repoVisibilityGet(t, fork.ID).IsPrivate)
		var settings struct {
			VisibilityFollowsBase bool `json:"visibilityFollowsBase"`
		}
		forkerClient.doJSON(http.MethodGet, repoSettingsAPIPath(fork, "/settings"), nil, http.StatusOK, &settings)
		assert.True(t, settings.VisibilityFollowsBase)

		for _, tc := range []struct {
			private, unlisted bool
		}{
			{private: true},
			{unlisted: true},
			{},
		} {
			repoVisibilityPatch(t, ownerClient, base, tc.private, tc.unlisted)
			got := repoVisibilityGet(t, fork.ID)
			assert.Equal(t, tc.private, got.IsPrivate, "base private %t, unlisted %t", tc.private, tc.unlisted)
			assert.Equal(t, tc.unlisted, got.IsUnlisted, "base private %t, unlisted %t", tc.private, tc.unlisted)
		}
	})

	t.Run("saving a fork makes it no more visible than its base", func(t *testing.T) {
		base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
		repoSettingsAddCollaborator(t, base, forker, database.AccessModeRead)
		fork := repoVisibilityFork(t, forkerClient, base, forker)
		require.True(t, repoVisibilityGet(t, fork.ID).IsPrivate)

		// A fork can only be public with a private base by a change outside the
		// stores, e.g., by an earlier version.
		db, err := dbx.OpenDB(cfg.Database, &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.Exec("UPDATE repository SET is_private = ? WHERE id = ?", false, fork.ID).Error)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		require.False(t, repoVisibilityGet(t, fork.ID).IsPrivate)

		// The issue tracker of API v1 does not touch visibility.
		status, body := apiV1Client(t, forker).do(http.MethodPatch, apiV1RepoPath(fork, "/issue-tracker"), map[string]any{"enable_issues": true})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.True(t, repoVisibilityGet(t, fork.ID).IsPrivate)
	})

	t.Run("public access of forks is limited to their base", func(t *testing.T) {
		base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
		repoVisibilityPutPublicAccess(t, ownerClient, base, false, true)
		repoSettingsAddCollaborator(t, base, forker, database.AccessModeRead)
		fork := repoVisibilityFork(t, forkerClient, base, forker)

		var settings repoVisibilitySettings
		forkerClient.doJSON(http.MethodGet, repoSettingsAPIPath(fork, "/settings"), nil, http.StatusOK, &settings)
		assert.False(t, settings.CanAllowPublicWiki)
		assert.True(t, settings.CanAllowPublicIssues)

		repoVisibilityPutPublicAccess(t, forkerClient, fork, true, true)
		got := repoVisibilityGet(t, fork.ID)
		assert.False(t, got.AllowPublicWiki)
		assert.True(t, got.AllowPublicIssues)

		status, body := apiV1Client(t, forker).do(http.MethodPatch, apiV1RepoPath(fork, "/wiki"), map[string]any{"allow_public_wiki": true})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.False(t, repoVisibilityGet(t, fork.ID).AllowPublicWiki)

		// Closing public issues on the base closes them on its forks.
		repoVisibilityPutPublicAccess(t, ownerClient, base, false, false)
		assert.False(t, repoVisibilityGet(t, fork.ID).AllowPublicIssues)
	})
}
