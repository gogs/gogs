package integrationtests

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// orgOpenIssue opens an issue in the repository as the user through API v1,
// and returns its index.
func orgOpenIssue(t *testing.T, u *User, owner, repo, title string) int64 {
	t.Helper()

	return apiV1CreateIssue(t, apiV1Client(t, u), &Repository{Owner: owner, Name: repo}, map[string]any{"title": title})
}

// orgOpenPullRequest commits a file to a new branch of the repository, which
// must have a commit, and opens a pull request from it. It returns the index of
// the pull request.
func orgOpenPullRequest(t *testing.T, c *client, owner, repo, title string) int64 {
	t.Helper()

	stored, err := getRepositoryByName(t, orgOwnerID(t, owner), repo)
	require.NoError(t, err)
	branch := UniqueName("feature-")
	repoPath := "/api/web/" + owner + "/" + repo
	status, body := c.do(http.MethodPost, repoPath+"/editor/files", map[string]any{
		"branch":    stored.DefaultBranch,
		"newBranch": branch,
		"summary":   "Add feature",
		"path":      "feature.txt",
		"content":   "feature\n",
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)

	var created struct {
		Index int64 `json:"index"`
	}
	c.doJSON(http.MethodPost, repoPath+"/pulls", map[string]any{
		"title": title,
		"base":  stored.DefaultBranch,
		"head":  branch,
	}, http.StatusCreated, &created)
	return created.Index
}

func orgOwnerID(t *testing.T, name string) int64 {
	t.Helper()

	u, err := database.Handle.Users().GetByUsername(t.Context(), name)
	require.NoError(t, err)
	return u.ID
}

type orgDashboard struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Repos []struct {
		Owner     string `json:"owner"`
		Name      string `json:"name"`
		IsPrivate bool   `json:"isPrivate"`
	} `json:"repos"`
	Organizations []struct {
		Name string `json:"name"`
	} `json:"organizations"`
}

func (d *orgDashboard) repoNames() []string {
	names := []string{}
	for _, repo := range d.Repos {
		names = append(names, repo.Owner+"/"+repo.Name)
	}
	return names
}

type orgDashboardIssues struct {
	Type  string `json:"type"`
	Items []struct {
		Repo  string `json:"repo"`
		Index int64  `json:"index"`
		Title string `json:"title"`
	} `json:"items"`
	Repos []struct {
		FullName string `json:"fullName"`
		Count    int    `json:"count"`
	} `json:"repos"`
	Counts struct {
		Open   int64 `json:"open"`
		Closed int64 `json:"closed"`
	} `json:"counts"`
}

// orgDashboardIssueTitles returns the issues or pull requests listed, keyed by
// "owner/repo#index".
func orgDashboardIssueTitles(t *testing.T, c *client, path string) (map[string]string, *orgDashboardIssues) {
	t.Helper()

	var resp orgDashboardIssues
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
	titles := make(map[string]string, len(resp.Items))
	for _, item := range resp.Items {
		titles[item.Repo+"#"+strconv.FormatInt(item.Index, 10)] = item.Title
	}
	return titles, &resp
}

func orgCreatedRepoURLs(items []orgActivityItem) []string {
	urls := []string{}
	for _, item := range items {
		if item.OpType == int(database.ActionCreateRepo) {
			urls = append(urls, item.RepoURL)
		}
	}
	return urls
}

func TestDashboardUser(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, other.ID)
	orgAddMember(t, org, u.ID)
	c := authenticatedClient(t, u)
	otherClient := authenticatedClient(t, other)
	repo := orgCreateRepoThroughWeb(t, c, u.ID, true)
	fullName := u.Name + "/" + repo

	var dashboard orgDashboard
	c.doJSON(http.MethodGet, "/api/web/dashboard", nil, http.StatusOK, &dashboard)
	assert.Equal(t, "user", dashboard.Kind)
	assert.Equal(t, u.Name, dashboard.Name)
	assert.Equal(t, []string{fullName}, dashboard.repoNames())
	assert.True(t, dashboard.Repos[0].IsPrivate)
	require.Len(t, dashboard.Organizations, 1)
	assert.Equal(t, org.Name, dashboard.Organizations[0].Name)

	assert.Equal(t, []string{"/" + fullName}, orgCreatedRepoURLs(orgActivity(t, c, "/api/web/dashboard/activity")))
	assert.NotContains(t, orgCreatedRepoURLs(orgActivity(t, otherClient, "/api/web/dashboard/activity")), "/"+fullName)

	t.Run("issues", func(t *testing.T) {
		index := orgOpenIssue(t, u, u.Name, repo, "Crash on start")
		key := fullName + "#" + strconv.FormatInt(index, 10)
		titles, resp := orgDashboardIssueTitles(t, c, "/api/web/dashboard/issues")
		assert.Equal(t, map[string]string{key: "Crash on start"}, titles)
		assert.Equal(t, "your_repositories", resp.Type)
		assert.EqualValues(t, 1, resp.Counts.Open)
		require.Len(t, resp.Repos, 1)
		assert.Equal(t, fullName, resp.Repos[0].FullName)
		assert.Equal(t, 1, resp.Repos[0].Count)

		titles, _ = orgDashboardIssueTitles(t, c, "/api/web/dashboard/issues?type=created_by")
		assert.Equal(t, map[string]string{key: "Crash on start"}, titles)
		titles, _ = orgDashboardIssueTitles(t, c, "/api/web/dashboard/issues?state=closed")
		assert.Empty(t, titles)
		titles, _ = orgDashboardIssueTitles(t, otherClient, "/api/web/dashboard/issues")
		assert.NotContains(t, titles, key)
	})

	t.Run("pulls", func(t *testing.T) {
		index := orgOpenPullRequest(t, c, u.Name, repo, "Add feature")
		key := fullName + "#" + strconv.FormatInt(index, 10)
		titles, resp := orgDashboardIssueTitles(t, c, "/api/web/dashboard/pulls")
		assert.Equal(t, map[string]string{key: "Add feature"}, titles)
		require.Len(t, resp.Repos, 1)
		assert.Equal(t, fullName, resp.Repos[0].FullName)
		assert.Equal(t, 1, resp.Repos[0].Count)

		titles, _ = orgDashboardIssueTitles(t, c, "/api/web/dashboard/issues")
		assert.NotContains(t, titles, key, "pull request listed as an issue")
		titles, _ = orgDashboardIssueTitles(t, otherClient, "/api/web/dashboard/pulls")
		assert.NotContains(t, titles, key)
	})

	for _, path := range []string{"", "/activity", "/issues", "/pulls"} {
		status, body := newClient(t).do(http.MethodGet, "/api/web/dashboard"+path, nil)
		assert.Equal(t, http.StatusUnauthorized, status, "GET %s signed out, response: %s", path, body)
	}
}

func TestDashboardOrganization(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	member := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)
	orgAddMember(t, org, member.ID)
	ownerClient := authenticatedClient(t, owner)
	memberClient := authenticatedClient(t, member)
	public := orgCreateRepoThroughWeb(t, ownerClient, org.ID, false)
	private := orgCreateRepoThroughWeb(t, ownerClient, org.ID, true)
	base := "/api/web/orgs/" + org.Name + "/dashboard"

	var dashboard orgDashboard
	ownerClient.doJSON(http.MethodGet, base, nil, http.StatusOK, &dashboard)
	assert.Equal(t, "organization", dashboard.Kind)
	assert.Equal(t, org.Name, dashboard.Name)
	assert.ElementsMatch(t, []string{org.Name + "/" + public, org.Name + "/" + private}, dashboard.repoNames())

	// Members only see the private repositories of their teams.
	dashboard = orgDashboard{}
	memberClient.doJSON(http.MethodGet, base, nil, http.StatusOK, &dashboard)
	assert.Equal(t, []string{org.Name + "/" + public}, dashboard.repoNames())

	assert.ElementsMatch(t,
		[]string{"/" + org.Name + "/" + private, "/" + org.Name + "/" + public},
		orgCreatedRepoURLs(orgActivity(t, ownerClient, base+"/activity")),
	)
	assert.Equal(t, []string{"/" + org.Name + "/" + public}, orgCreatedRepoURLs(orgActivity(t, memberClient, base+"/activity")))

	issue := orgOpenIssue(t, owner, org.Name, private, "Private crash")
	issueKey := org.Name + "/" + private + "#" + strconv.FormatInt(issue, 10)
	titles, _ := orgDashboardIssueTitles(t, ownerClient, base+"/issues")
	assert.Equal(t, map[string]string{issueKey: "Private crash"}, titles)
	titles, _ = orgDashboardIssueTitles(t, memberClient, base+"/issues")
	assert.Empty(t, titles, "issues of private repositories shown to members not on their teams")

	pull := orgOpenPullRequest(t, ownerClient, org.Name, public, "Public feature")
	pullKey := org.Name + "/" + public + "#" + strconv.FormatInt(pull, 10)
	for name, c := range map[string]*client{"owner": ownerClient, "member": memberClient} {
		titles, _ = orgDashboardIssueTitles(t, c, base+"/pulls")
		assert.Equal(t, map[string]string{pullKey: "Public feature"}, titles, "%s viewer", name)
	}

	outsiderClient := authenticatedClient(t, outsider)
	for _, path := range []string{"", "/activity", "/issues", "/pulls"} {
		status, body := outsiderClient.do(http.MethodGet, base+path, nil)
		assert.Equal(t, http.StatusNotFound, status, "GET %s as outsider, response: %s", path, body)
	}
	status, body := ownerClient.do(http.MethodGet, "/api/web/orgs/"+owner.Name+"/dashboard", nil)
	assert.Equal(t, http.StatusNotFound, status, "dashboard of a user as an organization, response: %s", body)
}
