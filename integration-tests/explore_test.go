package integrationtests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
