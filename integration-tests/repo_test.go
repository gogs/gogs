package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestRepositorySourceVisibility(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	srcPath := func(repo *Repository) string {
		return "/api/web/" + repo.Owner + "/" + repo.Name + "/src"
	}
	type entry struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	assertREADME := func(c *client, repo *Repository) {
		t.Helper()

		var resp struct {
			Kind    string  `json:"kind"`
			Entries []entry `json:"entries"`
		}
		c.doJSON(http.MethodGet, srcPath(repo), nil, http.StatusOK, &resp)
		assert.Equal(t, "dir", resp.Kind)
		assert.Equal(t, []entry{{Name: "README.md", Type: "file"}}, resp.Entries)
	}

	anonymous := newClient(t)
	otherClient := authenticatedClient(t, other)
	ownerClient := authenticatedClient(t, owner)

	for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient, "owner": ownerClient} {
		assertREADME(c, public)
		if c == ownerClient {
			assertREADME(c, private)
			continue
		}
		status, body := c.do(http.MethodGet, srcPath(private), nil)
		assert.Equal(t, http.StatusNotFound, status, "%s viewer, response: %s", name, body)
	}
}

func TestRepositoryExternalWikiAndTracker(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	collaborator := seedUser(t, CreateUserOptions{})
	prohibited := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	orgPrivate := seedRepository(t, orgSeed(t, owner.ID).ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	repoSettingsAddCollaborator(t, private, collaborator, database.AccessModeRead)
	repoSettingsAddCollaborator(t, private, prohibited, database.AccessModeRead)

	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	collaboratorClient := authenticatedClient(t, collaborator)
	// The prohibited collaborator stays authenticated by the session they had
	// before they were prohibited from signing in.
	prohibitedClient := authenticatedClient(t, prohibited)
	prohibitLogin := true
	require.NoError(t, database.Handle.Users().Update(t.Context(), prohibited.ID, database.UpdateUserOptions{ProhibitLogin: &prohibitLogin}))
	anonymous := newClient(t)

	pullCommitToNewBranch(t, ownerClient, repo, "feature", "feature.txt", "Feature.\n")
	pull := pullCreate(t, ownerClient, repo, "feature", "Feature")

	// The query and fragment must reach the external site unescaped.
	const trackerURL = "https://tracker.example.com/issues?project=x#open"
	const wikiURL = "https://wiki.example.com/doku.php?id=start&do=show#intro"
	api := apiV1Client(t, owner)
	for _, r := range []*Repository{repo, private, orgPrivate} {
		status, body := api.do(http.MethodPatch, apiV1RepoPath(r, "/issue-tracker"), map[string]any{
			"enable_issues":           true,
			"enable_external_tracker": true,
			"external_tracker_url":    trackerURL,
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, body = api.do(http.MethodPatch, apiV1RepoPath(r, "/wiki"), map[string]any{
			"enable_wiki":          true,
			"enable_external_wiki": true,
			"external_wiki_url":    wikiURL,
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}

	for name, c := range map[string]*client{"anonymous": anonymous, "owner": ownerClient} {
		t.Run(name, func(t *testing.T) {
			for _, elems := range [][]any{{"issues"}, {"issues", "new"}, {"issues", pull + 1}} {
				userAssertRedirect(t, c, issuePagePath(repo, elems...), http.StatusFound, trackerURL)
			}
			for _, elems := range [][]any{{"wiki"}, {"wiki", "Home"}, {"wiki", "_pages"}, {"wiki", "_new"}, {"wiki", "Home", "_edit"}} {
				userAssertRedirect(t, c, issuePagePath(repo, elems...), http.StatusFound, wikiURL)
			}
			// Pull requests share their numbers with issues and stay here.
			userAssertPage(t, c, issuePagePath(repo, "issues", pull), http.StatusOK)

			var header struct {
				ExternalTrackerURL string `json:"externalTrackerURL"`
				ExternalWikiURL    string `json:"externalWikiURL"`
			}
			c.doJSON(http.MethodGet, issueAPIPath(repo, "header"), nil, http.StatusOK, &header)
			assert.Equal(t, trackerURL, header.ExternalTrackerURL)
			assert.Equal(t, wikiURL, header.ExternalWikiURL)
		})
	}

	assertNoRedirect := func(t *testing.T, name string, c *client, r *Repository, elems ...any) {
		t.Helper()
		path := issuePagePath(r, elems...)
		status, header, body := c.doRaw(http.MethodGet, path, "", nil)
		assert.NotEqual(t, http.StatusFound, status, "%s, GET %s, response: %s", name, path, body)
		assert.Empty(t, header.Get("Location"), "%s, GET %s", name, path)
	}

	// Viewers who cannot read the repository do not learn where its issues and
	// wiki are kept.
	userAssertRedirect(t, collaboratorClient, issuePagePath(private, "wiki"), http.StatusFound, wikiURL)
	userAssertRedirect(t, ownerClient, issuePagePath(orgPrivate, "issues"), http.StatusFound, trackerURL)
	for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient, "prohibited": prohibitedClient} {
		for _, r := range []*Repository{private, orgPrivate} {
			assertNoRedirect(t, name, c, r, "issues")
			assertNoRedirect(t, name, c, r, "wiki")
		}
	}

	t.Run("unsafe URLs", func(t *testing.T) {
		for _, unsafe := range []string{"", "javascript:alert(1)", "data:text/html,hi", "wiki.example.com", "//wiki.example.com"} {
			status, body := api.do(http.MethodPatch, apiV1RepoPath(repo, "/wiki"), map[string]any{"external_wiki_url": unsafe})
			assert.Equal(t, http.StatusUnprocessableEntity, status, "API v1 wiki URL %q, response: %s", unsafe, body)
			status, body = api.do(http.MethodPatch, apiV1RepoPath(repo, "/issue-tracker"), map[string]any{"external_tracker_url": unsafe})
			assert.Equal(t, http.StatusUnprocessableEntity, status, "API v1 tracker URL %q, response: %s", unsafe, body)

			status, body = ownerClient.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), map[string]any{
				"enableWiki":            true,
				"enableExternalWiki":    true,
				"externalWikiURL":       unsafe,
				"enableIssues":          true,
				"enableExternalTracker": true,
				"externalTrackerURL":    trackerURL,
				"enablePulls":           true,
				"pullsAllowMerge":       true,
			})
			require.Equal(t, http.StatusUnprocessableEntity, status, "wiki URL %q, response: %s", unsafe, body)
			assert.NotEmpty(t, repoSettingsFieldError(t, body, "externalWikiURL"))
			status, body = ownerClient.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), map[string]any{
				"enableWiki":            true,
				"enableExternalWiki":    true,
				"externalWikiURL":       wikiURL,
				"enableIssues":          true,
				"enableExternalTracker": true,
				"externalTrackerURL":    unsafe,
				"enablePulls":           true,
				"pullsAllowMerge":       true,
			})
			require.Equal(t, http.StatusUnprocessableEntity, status, "tracker URL %q, response: %s", unsafe, body)
			assert.NotEmpty(t, repoSettingsFieldError(t, body, "externalTrackerURL"))
		}
		got, err := getRepository(t, repo.ID)
		require.NoError(t, err)
		assert.Equal(t, wikiURL, got.ExternalWikiURL, "changed by a refused request")
		assert.Equal(t, trackerURL, got.ExternalTrackerURL, "changed by a refused request")

		// URLs stored before they were checked are not used.
		got.ExternalWikiURL = "javascript:alert(1)"
		got.ExternalTrackerURL = ""
		require.NoError(t, database.UpdateRepository(logger, got, false))
		assertNoRedirect(t, "owner", ownerClient, repo, "wiki")
		assertNoRedirect(t, "owner", ownerClient, repo, "issues")
		var header map[string]any
		ownerClient.doJSON(http.MethodGet, issueAPIPath(repo, "header"), nil, http.StatusOK, &header)
		assert.NotContains(t, header, "externalWikiURL")
		assert.NotContains(t, header, "externalTrackerURL")
	})
}
