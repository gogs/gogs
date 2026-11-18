package integrationtests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbx"
)

type orgExploreItem struct {
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	IsPrivate bool   `json:"isPrivate"`
}

// orgExplore returns the names of the items the search of the explore list
// finds, which must fit on the first page.
func orgExplore(t *testing.T, c *client, list, keyword string) map[string]orgExploreItem {
	t.Helper()

	var resp struct {
		Items      []orgExploreItem `json:"items"`
		Page       int              `json:"page"`
		TotalPages int              `json:"totalPages"`
	}
	c.doJSON(http.MethodGet, "/api/web/explore/"+list+"?q="+url.QueryEscape(keyword), nil, http.StatusOK, &resp)
	assert.Equal(t, 1, resp.Page)
	assert.LessOrEqual(t, resp.TotalPages, 1)
	items := make(map[string]orgExploreItem, len(resp.Items))
	for _, item := range resp.Items {
		items[item.Name] = item
	}
	return items
}

func TestExploreRepos(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})

	ownerClient := authenticatedClient(t, owner)
	for name, c := range map[string]*client{"anonymous": newClient(t), "other": authenticatedClient(t, other), "owner": ownerClient} {
		repos := orgExplore(t, c, "repos", public.Name)
		assert.Equal(t, map[string]orgExploreItem{public.Name: {Owner: owner.Name, Name: public.Name}}, repos, "%s viewer", name)

		repos = orgExplore(t, c, "repos", private.Name)
		if c == ownerClient {
			assert.Equal(t, map[string]orgExploreItem{private.Name: {Owner: owner.Name, Name: private.Name, IsPrivate: true}}, repos, "%s viewer", name)
		} else {
			assert.Empty(t, repos, "%s viewer", name)
		}
	}
}

// TestReposWithoutOwner checks the lists of repositories when the owner of some
// no longer exists, e.g., left behind by an old bug or a manual database edit.
// The lists leave such repositories out, including from their totals, instead
// of failing to show their owner.
func TestReposWithoutOwner(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	deletedOwner := seedUser(t, CreateUserOptions{})
	// Created first, so they come first in lists ordered by ID. They have code,
	// so code search does not skip them as empty.
	orphans := []int64{
		seedRepository(t, deletedOwner.ID, CreateRepositoryOptions{AutoInit: true}).ID,
		seedRepository(t, deletedOwner.ID, CreateRepositoryOptions{AutoInit: true}).ID,
	}
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	// The keyword matches the description of all three repositories and nothing
	// else.
	keyword := UniqueName("orphan-")
	require.NoError(t, gormDB.Exec("UPDATE repository SET description = ? WHERE id IN ?", keyword, append(orphans, repo.ID)).Error)
	require.NoError(t, gormDB.Exec(dbx.Quote("DELETE FROM %s WHERE id = ?", "user"), deletedOwner.ID).Error)

	t.Run("explore", func(t *testing.T) {
		for name, c := range map[string]*client{"anonymous": newClient(t), "authenticated": authenticatedClient(t, owner)} {
			repos := orgExplore(t, c, "repos", keyword)
			assert.Equal(t, map[string]orgExploreItem{repo.Name: {Owner: owner.Name, Name: repo.Name}}, repos, "%s viewer", name)
		}
	})

	t.Run("API search", func(t *testing.T) {
		status, header, body := newClient(t).doRaw(http.MethodGet, "/api/v1/repos/search?limit=1&q="+keyword, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		var resp struct {
			Data []apiV1Repo `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		assert.Equal(t, []string{repo.Name}, apiV1RepoNames(resp.Data))
		assert.Empty(t, apiV1Links(t, header), "links to pages of repositories without an owner")
	})

	// Searches without a repository or owner go through the repositories of the
	// whole site that the viewer can see.
	t.Run("code search", func(t *testing.T) {
		var resp struct {
			Files []codeSearchFile `json:"files"`
		}
		// The README of an initialized repository has its name as the title.
		newClient(t).doJSON(http.MethodGet, "/api/web/search/code?q="+repo.Name, nil, http.StatusOK, &resp)
		assert.Equal(t, []string{owner.Name + "/" + repo.Name + ":README.md"}, codeSearchPaths(resp.Files))
	})

	t.Run("issue search", func(t *testing.T) {
		var resp struct {
			Items []any `json:"items"`
		}
		newClient(t).doJSON(http.MethodGet, "/api/web/search/issues?q="+keyword, nil, http.StatusOK, &resp)
		assert.Empty(t, resp.Items)
	})

	t.Run("admin", func(t *testing.T) {
		admin := authenticatedClient(t, seedUser(t, CreateUserOptions{Admin: true}))
		type item struct {
			ID    int64  `json:"id"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		}
		type listPage struct {
			Items      []item `json:"items"`
			TotalPages int    `json:"totalPages"`
			Total      int64  `json:"total"`
		}

		var resp listPage
		admin.doJSON(http.MethodGet, "/api/web/admin/repos?q="+keyword, nil, http.StatusOK, &resp)
		assert.Equal(t, []item{{ID: repo.ID, Owner: owner.Name, Name: repo.Name}}, resp.Items)
		assert.Equal(t, int64(1), resp.Total)

		// Without a keyword, the list has every repository of the site, which
		// other tests add and delete at the same time, so it is only checked to
		// list no repository without an owner on any page.
		for page, totalPages := 1, 1; page <= totalPages; page++ {
			var resp listPage
			admin.doJSON(http.MethodGet, "/api/web/admin/repos?page="+strconv.Itoa(page), nil, http.StatusOK, &resp)
			for _, item := range resp.Items {
				assert.NotContains(t, orphans, item.ID, "page %d", page)
			}
			totalPages = resp.TotalPages
		}
	})
}

func TestExploreUsers(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, u.ID)

	users := orgExplore(t, newClient(t), "users", u.Name)
	assert.Equal(t, map[string]orgExploreItem{u.Name: {Name: u.Name}}, users, "anonymous viewer")

	users = orgExplore(t, authenticatedClient(t, seedUser(t, CreateUserOptions{})), "users", u.Name)
	assert.Equal(t, map[string]orgExploreItem{u.Name: {Name: u.Name, Email: u.Email}}, users)

	assert.Empty(t, orgExplore(t, newClient(t), "users", org.Name), "organizations listed as users")
}

func TestExploreOrganizations(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, u.ID)

	for name, c := range map[string]*client{"anonymous": newClient(t), "owner": authenticatedClient(t, u)} {
		orgs := orgExplore(t, c, "organizations", org.Name)
		assert.Equal(t, map[string]orgExploreItem{org.Name: {Name: org.Name}}, orgs, "%s viewer", name)
		assert.Empty(t, orgExplore(t, c, "organizations", u.Name), "%s viewer, users listed as organizations", name)
	}
}

func TestExploreRedirect(t *testing.T) {
	t.Parallel()

	status, header, body := newClient(t).doRaw(http.MethodGet, "/explore", "", nil)
	require.Equal(t, http.StatusFound, status, "response: %.500s", body)
	assert.Equal(t, "/explore/repos", header.Get("Location"))
}
