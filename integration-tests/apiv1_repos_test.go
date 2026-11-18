package integrationtests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestAPIV1CreateRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, owner)

	name := UniqueName("repo-")
	var created apiV1Repo
	api.doJSON(http.MethodPost, "/api/v1/user/repos", map[string]any{
		"name":        name,
		"description": "A new repository",
		"private":     true,
		"auto_init":   true,
		"readme":      "Default",
	}, http.StatusCreated, &created)
	assert.Equal(t, name, created.Name)
	assert.Equal(t, owner.Name+"/"+name, created.FullName)
	assert.Equal(t, owner.Name, created.Owner.UserName)
	assert.Equal(t, "A new repository", created.Description)
	assert.True(t, created.Private)

	repo := &Repository{Owner: owner.Name, Name: name}
	var got apiV1Repo
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, ""), nil, http.StatusOK, &got)
	assert.Equal(t, created.ID, got.ID)
	assert.True(t, got.Private)
	assert.False(t, got.Empty, "auto-initialized repository is empty")
	require.NotNil(t, got.Permissions)
	assert.True(t, got.Permissions.Admin)

	t.Run("duplicate name", func(t *testing.T) {
		status, body := api.do(http.MethodPost, "/api/v1/user/repos", map[string]any{"name": name})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	})

	status, body := newClient(t).do(http.MethodPost, "/api/v1/user/repos", map[string]any{"name": UniqueName("repo-")})
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	status, body = apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodGet, apiV1RepoPath(repo, ""), nil)
	assert.Equal(t, http.StatusNotFound, status, "private repository of another user, response: %s", body)
}

func apiV1RepoNames(repos []apiV1Repo) []string {
	names := []string{}
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return names
}

func TestAPIV1ListUserRepositories(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})

	var repos []apiV1Repo
	apiV1Client(t, owner).doJSON(http.MethodGet, "/api/v1/user/repos", nil, http.StatusOK, &repos)
	assert.ElementsMatch(t, []string{public.Name, private.Name}, apiV1RepoNames(repos))

	repos = nil
	other := apiV1Client(t, seedUser(t, CreateUserOptions{}))
	other.doJSON(http.MethodGet, "/api/v1/users/"+owner.Name+"/repos", nil, http.StatusOK, &repos)
	assert.Equal(t, []string{public.Name}, apiV1RepoNames(repos), "private repository listed to another user")
	require.NotNil(t, repos[0].Permissions)
	assert.False(t, repos[0].Permissions.Admin, "another user is admin of the repository")
	assert.False(t, repos[0].Permissions.Push, "another user can push to the repository")
	assert.True(t, repos[0].Permissions.Pull)

	collaborator := seedUser(t, CreateUserOptions{})
	status, body := apiV1Client(t, owner).do(http.MethodPut, apiV1RepoPath(public, "/collaborators/"+collaborator.Name), map[string]any{"permission": "write"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	repos = nil
	apiV1Client(t, collaborator).doJSON(http.MethodGet, "/api/v1/users/"+owner.Name+"/repos", nil, http.StatusOK, &repos)
	require.Len(t, repos, 1)
	require.NotNil(t, repos[0].Permissions)
	assert.False(t, repos[0].Permissions.Admin, "collaborator with write access is admin of the repository")
	assert.True(t, repos[0].Permissions.Push, "collaborator with write access cannot push to the repository")

	repos = nil
	apiV1Client(t, seedUser(t, CreateUserOptions{Admin: true})).doJSON(http.MethodGet, "/api/v1/users/"+owner.Name+"/repos", nil, http.StatusOK, &repos)
	assert.ElementsMatch(t, []string{public.Name}, apiV1RepoNames(repos))
	require.NotNil(t, repos[0].Permissions)
	assert.True(t, repos[0].Permissions.Admin, "site admin is not admin of the repository")

	status, body = newClient(t).do(http.MethodGet, "/api/v1/users/"+owner.Name+"/repos", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
}

func TestAPIV1SearchRepositories(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})

	search := func(c *client, query string) []string {
		t.Helper()

		var resp struct {
			OK   bool        `json:"ok"`
			Data []apiV1Repo `json:"data"`
		}
		c.doJSON(http.MethodGet, "/api/v1/repos/search?"+query, nil, http.StatusOK, &resp)
		assert.True(t, resp.OK)
		return apiV1RepoNames(resp.Data)
	}

	anonymous := newClient(t)
	ownerClient := apiV1Client(t, owner)
	uid := "uid=" + apiV1Itoa(owner.ID)
	assert.Equal(t, []string{public.Name}, search(anonymous, "q="+public.Name))
	assert.Empty(t, search(anonymous, "q="+private.Name), "private repository found by anonymous viewer")
	assert.Empty(t, search(anonymous, "q="+private.Name+"&"+uid), "private repository found by anonymous viewer")
	assert.Equal(t, []string{private.Name}, search(ownerClient, "q="+private.Name+"&"+uid))

	// Seeded repository names have no dot, which an empty query once required.
	t.Run("empty query", func(t *testing.T) {
		assert.Equal(t, []string{public.Name}, search(anonymous, uid))
		assert.Equal(t, []string{public.Name}, search(anonymous, "q=&"+uid))
		assert.ElementsMatch(t, []string{public.Name, private.Name}, search(ownerClient, uid))
	})

	t.Run("full name", func(t *testing.T) {
		assert.Empty(t, search(anonymous, "q="+owner.Name+"/"+public.Name))
	})
}

func TestAPIV1SearchRepositoriesPages(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	want := []string{}
	for range 5 {
		want = append(want, seedRepository(t, owner.ID, CreateRepositoryOptions{}).Name)
	}

	// Follows the "next" links of the Link header like an API client, so every
	// page keeps the query of the first one.
	anonymous := newClient(t)
	got := []string{}
	search := "/api/v1/repos/search?limit=2&uid=" + apiV1Itoa(owner.ID)
	for pages, next := 0, search; next != ""; pages++ {
		require.Less(t, pages, 3, "more pages than the repositories fill")

		status, header, body := anonymous.doRaw(http.MethodGet, next, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		var resp struct {
			Data []apiV1Repo `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.LessOrEqual(t, len(resp.Data), 2)
		got = append(got, apiV1RepoNames(resp.Data)...)

		next = apiV1Links(t, header)["next"]
	}
	// Repositories are listed in the order they were created, each once.
	assert.Equal(t, want, got, "pages of the search")

	t.Run("past the last page", func(t *testing.T) {
		// Page 3 is the last one with repositories, and the one before page 10.
		page := func(n string) string {
			return "/api/v1/repos/search?limit=2&page=" + n + "&uid=" + apiV1Itoa(owner.ID)
		}
		// A page whose offset overflows an int64 is past the last page too.
		for _, n := range []string{"10", "99999999999999999999"} {
			status, header, body := anonymous.doRaw(http.MethodGet, page(n), "", nil)
			require.Equal(t, http.StatusOK, status, "page %s, response: %s", n, body)
			assert.JSONEq(t, `{"ok": true, "data": []}`, string(body), "page %s", n)
			assert.Equal(t, map[string]string{"first": page("1"), "prev": page("3"), "last": page("3")}, apiV1Links(t, header), "page %s", n)
		}
	})
}

func TestAPIV1DeleteRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	status, body := apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodDelete, apiV1RepoPath(repo, ""), nil)
	assert.Equal(t, http.StatusForbidden, status, "deleted by another user, response: %s", body)

	api := apiV1Client(t, owner)
	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, ""), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, ""), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	_, err := getRepositoryByName(t, owner.ID, repo.Name)
	assert.True(t, database.IsErrRepoNotExist(err), "repository not deleted: %v", err)
}

func TestAPIV1MigrateRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	source := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	mirror := &Repository{Owner: owner.Name, Name: UniqueName("mirror-")}
	var created apiV1Repo
	api.doJSON(http.MethodPost, "/api/v1/repos/migrate", map[string]any{
		"clone_addr": serverURL + "/" + source.Owner + "/" + source.Name + ".git",
		"uid":        owner.ID,
		"repo_name":  mirror.Name,
		"mirror":     true,
	}, http.StatusCreated, &created)
	assert.Equal(t, mirror.Name, created.Name)
	assert.True(t, created.Mirror)

	var readme apiV1Content
	api.doJSON(http.MethodGet, apiV1RepoPath(mirror, "/contents/README.md"), nil, http.StatusOK, &readme)
	assert.Equal(t, "file", readme.Type)

	// A sync fetches what was pushed to the source since the migration.
	apiV1PutFile(t, api, source, "synced.txt", "Synced", "")
	status, body := api.do(http.MethodPost, apiV1RepoPath(mirror, "/mirror-sync"), nil)
	require.Equal(t, http.StatusAccepted, status, "response: %s", body)
	assert.Eventually(t, func() bool {
		status, _ := api.do(http.MethodGet, apiV1RepoPath(mirror, "/contents/synced.txt"), nil)
		return status == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond, "mirror not synced")

	status, body = api.do(http.MethodPost, apiV1RepoPath(source, "/mirror-sync"), nil)
	assert.Equal(t, http.StatusNotFound, status, "not a mirror, response: %s", body)

	other := seedUser(t, CreateUserOptions{})
	status, body = api.do(http.MethodPost, "/api/v1/repos/migrate", map[string]any{
		"clone_addr": serverURL + "/" + source.Owner + "/" + source.Name + ".git",
		"uid":        other.ID,
		"repo_name":  UniqueName("mirror-"),
	})
	assert.Equal(t, http.StatusForbidden, status, "migrated for another user, response: %s", body)
}

func TestAPIV1ListForks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	forker := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	ctx := context.WithoutCancel(t.Context())
	forkOwner, err := database.Handle.Users().GetByID(ctx, forker.ID)
	require.NoError(t, err)
	baseRepo, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	forkName := UniqueName("fork-")
	_, err = database.ForkRepository(ctx, logger, forkOwner, forkOwner, baseRepo, forkName, "")
	require.NoError(t, err)

	var forks []apiV1Repo
	apiV1Client(t, owner).doJSON(http.MethodGet, apiV1RepoPath(repo, "/forks"), nil, http.StatusOK, &forks)
	require.Len(t, forks, 1)
	assert.Equal(t, forkName, forks[0].Name)
	assert.Equal(t, forker.Name, forks[0].Owner.UserName)
	assert.True(t, forks[0].Fork)
	require.NotNil(t, forks[0].Permissions)
	assert.False(t, forks[0].Permissions.Push, "owner of the base repository can push to the fork")
}

func TestAPIV1RepositorySettings(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	other := apiV1Client(t, seedUser(t, CreateUserOptions{}))

	t.Run("issue tracker", func(t *testing.T) {
		status, body := other.do(http.MethodPatch, apiV1RepoPath(repo, "/issue-tracker"), map[string]any{"enable_issues": false})
		assert.Equal(t, http.StatusForbidden, status, "changed by another user, response: %s", body)

		status, body = api.do(http.MethodPatch, apiV1RepoPath(repo, "/issue-tracker"), map[string]any{
			"enable_issues":           true,
			"enable_external_tracker": true,
			"external_tracker_url":    "https://tracker.example.com",
			"tracker_url_format":      "https://tracker.example.com/{index}",
			"tracker_issue_style":     "numeric",
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		got, err := getRepository(t, repo.ID)
		require.NoError(t, err)
		assert.True(t, got.EnableExternalTracker)
		assert.Equal(t, "https://tracker.example.com", got.ExternalTrackerURL)
		assert.Equal(t, "https://tracker.example.com/{index}", got.ExternalTrackerFormat)
		assert.Equal(t, "numeric", got.ExternalTrackerStyle)

		// Issues are tracked elsewhere.
		status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/issues"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("wiki", func(t *testing.T) {
		status, body := other.do(http.MethodPatch, apiV1RepoPath(repo, "/wiki"), map[string]any{"enable_wiki": false})
		assert.Equal(t, http.StatusForbidden, status, "changed by another user, response: %s", body)

		status, body = api.do(http.MethodPatch, apiV1RepoPath(repo, "/wiki"), map[string]any{
			"enable_wiki":          true,
			"allow_public_wiki":    true,
			"enable_external_wiki": true,
			"external_wiki_url":    "https://wiki.example.com",
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		got, err := getRepository(t, repo.ID)
		require.NoError(t, err)
		assert.True(t, got.EnableWiki)
		assert.True(t, got.AllowPublicWiki)
		assert.True(t, got.EnableExternalWiki)
		assert.Equal(t, "https://wiki.example.com", got.ExternalWikiURL)
	})
}

func TestAPIV1Collaborators(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	collaborator := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})
	api := apiV1Client(t, owner)
	path := apiV1RepoPath(repo, "/collaborators/"+collaborator.Name)

	status, body := api.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = api.do(http.MethodPut, path, map[string]any{"permission": "write"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = api.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", body)

	type collaboratorResp struct {
		apiV1User
		Permissions struct {
			Admin bool `json:"admin"`
			Push  bool `json:"push"`
			Pull  bool `json:"pull"`
		} `json:"permissions"`
	}
	var collaborators []collaboratorResp
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/collaborators"), nil, http.StatusOK, &collaborators)
	require.Len(t, collaborators, 1)
	assert.Equal(t, collaborator.Name, collaborators[0].UserName)
	assert.False(t, collaborators[0].Permissions.Admin)
	assert.True(t, collaborators[0].Permissions.Push)
	assert.True(t, collaborators[0].Permissions.Pull)

	// A collaborator with write access sees the private repository, but does
	// not manage its collaborators.
	collaboratorAPI := apiV1Client(t, collaborator)
	status, body = collaboratorAPI.do(http.MethodGet, apiV1RepoPath(repo, ""), nil)
	assert.Equal(t, http.StatusOK, status, "response: %s", body)
	status, body = collaboratorAPI.do(http.MethodGet, apiV1RepoPath(repo, "/collaborators"), nil)
	assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

	status, body = api.do(http.MethodPut, apiV1RepoPath(repo, "/collaborators/"+UniqueName("nobody-")), map[string]any{})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	status, body = api.do(http.MethodDelete, path, nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = api.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = collaboratorAPI.do(http.MethodGet, apiV1RepoPath(repo, ""), nil)
	assert.Equal(t, http.StatusNotFound, status, "removed collaborator, response: %s", body)
}

func TestAPIV1Hooks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)

	type hook struct {
		ID     int64             `json:"id"`
		Type   string            `json:"type"`
		Config map[string]string `json:"config"`
		Events []string          `json:"events"`
		Active bool              `json:"active"`
	}
	var created hook
	api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/hooks"), map[string]any{
		"type":   "gogs",
		"config": map[string]string{"url": "http://127.0.0.1:1/created", "content_type": "json"},
		"events": []string{"push", "issues"},
		"active": true,
	}, http.StatusCreated, &created)
	assert.Equal(t, "gogs", created.Type)
	assert.Equal(t, map[string]string{"url": "http://127.0.0.1:1/created", "content_type": "json"}, created.Config)
	assert.ElementsMatch(t, []string{"push", "issues"}, created.Events)
	assert.True(t, created.Active)

	var hooks []hook
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/hooks"), nil, http.StatusOK, &hooks)
	assert.Equal(t, []hook{created}, hooks)

	var edited hook
	api.doJSON(http.MethodPatch, apiV1RepoPath(repo, "/hooks/"+apiV1Itoa(created.ID)), map[string]any{
		"config": map[string]string{"url": "http://127.0.0.1:1/edited", "content_type": "form"},
		"events": []string{"create"},
		"active": false,
	}, http.StatusOK, &edited)
	want := hook{
		ID:     created.ID,
		Type:   "gogs",
		Config: map[string]string{"url": "http://127.0.0.1:1/edited", "content_type": "form"},
		Events: []string{"create"},
	}
	assert.Equal(t, want, edited)
	hooks = nil
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/hooks"), nil, http.StatusOK, &hooks)
	assert.Equal(t, []hook{want}, hooks)

	status, body := api.do(http.MethodPost, apiV1RepoPath(repo, "/hooks"), map[string]any{
		"type":   "unknown",
		"config": map[string]string{"url": "http://127.0.0.1:1/", "content_type": "json"},
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	status, body = apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodGet, apiV1RepoPath(repo, "/hooks"), nil)
	assert.Equal(t, http.StatusForbidden, status, "listed by another user, response: %s", body)

	// An ID that is not a webhook of the repository neither edits nor deletes
	// one.
	status, body = api.do(http.MethodPatch, apiV1RepoPath(repo, "/hooks/0"), map[string]any{"active": true})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, "/hooks/0"), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	hooks = nil
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/hooks"), nil, http.StatusOK, &hooks)
	assert.Equal(t, []hook{want}, hooks)

	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, "/hooks/"+apiV1Itoa(created.ID)), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	hooks = nil
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/hooks"), nil, http.StatusOK, &hooks)
	assert.Empty(t, hooks)
}

func TestAPIV1DeployKeys(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	api := apiV1Client(t, owner)

	type deployKey struct {
		ID       int64  `json:"id"`
		Key      string `json:"key"`
		URL      string `json:"url"`
		Title    string `json:"title"`
		ReadOnly bool   `json:"read_only"`
	}
	title := UniqueName("deploy-")
	key := apiV1PublicKey(t)
	var created deployKey
	api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/keys"), map[string]any{"title": title, "key": key}, http.StatusCreated, &created)
	assert.Equal(t, title, created.Title)
	assert.Equal(t, key, created.Key)
	assert.True(t, created.ReadOnly)
	assert.Equal(t, serverURL+apiV1RepoPath(repo, "/keys/"+apiV1Itoa(created.ID)), created.URL)

	var got deployKey
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/keys/"+apiV1Itoa(created.ID)), nil, http.StatusOK, &got)
	assert.Equal(t, created, got)

	var keys []deployKey
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/keys"), nil, http.StatusOK, &keys)
	assert.Equal(t, []deployKey{created}, keys)

	status, body := apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodGet, apiV1RepoPath(repo, "/keys"), nil)
	assert.Equal(t, http.StatusForbidden, status, "listed by another user, response: %s", body)

	status, body = api.do(http.MethodDelete, apiV1RepoPath(repo, "/keys/"+apiV1Itoa(created.ID)), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/keys/"+apiV1Itoa(created.ID)), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}
