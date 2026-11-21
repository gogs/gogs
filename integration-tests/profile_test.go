package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

type orgProfile struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Organizations []struct {
		Name string `json:"name"`
	} `json:"organizations"`
	FollowerCount     int  `json:"followerCount"`
	FollowingCount    int  `json:"followingCount"`
	ViewerIsSelf      bool `json:"viewerIsSelf"`
	ViewerIsFollowing bool `json:"viewerIsFollowing"`
}

func (p *orgProfile) orgNames() []string {
	names := make([]string, 0, len(p.Organizations))
	for _, org := range p.Organizations {
		names = append(names, org.Name)
	}
	return names
}

func orgGetProfile(t *testing.T, c *client, name string) *orgProfile {
	t.Helper()

	var p orgProfile
	c.doJSON(http.MethodGet, "/api/web/users/"+name, nil, http.StatusOK, &p)
	return &p
}

// orgCreateRepoThroughWeb creates a repository with a commit the way the web
// app does, which records the action in the activity feeds.
func orgCreateRepoThroughWeb(t *testing.T, c *client, ownerID int64, private bool) string {
	t.Helper()

	name := UniqueName("repo-")
	var created struct {
		Name string `json:"name"`
	}
	c.doJSON(http.MethodPost, "/api/web/repo/create", map[string]any{
		"ownerID":  ownerID,
		"name":     name,
		"private":  private,
		"autoInit": true,
		"readme":   "Default",
	}, http.StatusCreated, &created)
	require.Equal(t, name, created.Name)
	return name
}

type orgActivityItem struct {
	OpType    int    `json:"opType"`
	ActorName string `json:"actorName"`
	RepoURL   string `json:"repoURL"`
}

func orgActivity(t *testing.T, c *client, path string) []orgActivityItem {
	t.Helper()

	var feed struct {
		Items []orgActivityItem `json:"items"`
	}
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &feed)
	return feed.Items
}

func TestProfileUser(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	viewer := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, viewer.ID)
	orgAddMember(t, org, u.ID)

	anonymous := orgGetProfile(t, newClient(t), u.Name)
	assert.Equal(t, "user", anonymous.Kind)
	assert.Equal(t, u.Name, anonymous.Name)
	assert.Empty(t, anonymous.Email, "email shown to anonymous viewers")
	assert.False(t, anonymous.ViewerIsSelf)
	assert.Empty(t, anonymous.orgNames(), "private membership shown")

	self := orgGetProfile(t, authenticatedClient(t, u), u.Name)
	assert.Equal(t, u.Email, self.Email)
	assert.True(t, self.ViewerIsSelf)
	assert.Equal(t, []string{org.Name}, self.orgNames())

	viewerClient := authenticatedClient(t, viewer)
	other := orgGetProfile(t, viewerClient, u.Name)
	assert.Equal(t, u.Email, other.Email)
	assert.False(t, other.ViewerIsSelf)
	assert.Empty(t, other.orgNames(), "private membership shown")

	require.NoError(t, database.ChangeOrgUserStatus(org.ID, u.ID, true))
	assert.Equal(t, []string{org.Name}, orgGetProfile(t, viewerClient, u.Name).orgNames())

	status, body := newClient(t).do(http.MethodGet, "/api/web/users/"+UniqueName("missing-"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestProfileOrganization(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	org := orgSeed(t, owner.ID)

	type profile struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Members []struct {
			Name string `json:"name"`
		} `json:"members"`
		Teams []struct {
			LowerName string `json:"lowerName"`
		} `json:"teams"`
		ViewerIsMember bool `json:"viewerIsMember"`
		ViewerIsOwner  bool `json:"viewerIsOwner"`
	}
	var got profile
	authenticatedClient(t, owner).doJSON(http.MethodGet, "/api/web/users/"+org.Name, nil, http.StatusOK, &got)
	assert.Equal(t, "organization", got.Kind)
	assert.Equal(t, org.Name, got.Name)
	assert.True(t, got.ViewerIsOwner)
	assert.True(t, got.ViewerIsMember)
	require.Len(t, got.Members, 1)
	assert.Equal(t, owner.Name, got.Members[0].Name)
	require.Len(t, got.Teams, 1)
	assert.Equal(t, "owners", got.Teams[0].LowerName)

	got = profile{}
	newClient(t).doJSON(http.MethodGet, "/api/web/users/"+org.Name, nil, http.StatusOK, &got)
	assert.Equal(t, "organization", got.Kind)
	assert.False(t, got.ViewerIsMember)
	assert.Empty(t, got.Teams, "teams shown to non-members")

	for _, path := range []string{"/activity", "/followers", "/following"} {
		status, body := newClient(t).do(http.MethodGet, "/api/web/users/"+org.Name+path, nil)
		assert.Equal(t, http.StatusNotFound, status, "GET %s, response: %s", path, body)
	}
	ownerClient := authenticatedClient(t, owner)
	status, body := ownerClient.do(http.MethodPost, "/api/web/users/"+org.Name+"/follow", orgNoBody)
	assert.Equal(t, http.StatusNotFound, status, "following an organization, response: %s", body)
	assert.False(t, database.Handle.Users().IsFollowing(t.Context(), owner.ID, org.ID))

	// Follows from before organizations were refused can be removed.
	require.NoError(t, database.Handle.Users().Follow(t.Context(), owner.ID, org.ID))
	status, body = ownerClient.do(http.MethodDelete, "/api/web/users/"+org.Name+"/follow", orgNoBody)
	assert.Equal(t, http.StatusOK, status, "unfollowing an organization, response: %s", body)
	assert.False(t, database.Handle.Users().IsFollowing(t.Context(), owner.ID, org.ID))
}

func TestProfileRepos(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, u.ID, CreateRepositoryOptions{})
	private := seedRepository(t, u.ID, CreateRepositoryOptions{Private: true})
	org := orgSeed(t, u.ID)
	orgPublic := seedRepository(t, org.ID, CreateRepositoryOptions{})
	orgPrivate := seedRepository(t, org.ID, CreateRepositoryOptions{Private: true})
	ownerClient := authenticatedClient(t, u)
	otherClient := authenticatedClient(t, seedUser(t, CreateUserOptions{}))

	repoNames := func(c *client, owner string) []string {
		t.Helper()
		var resp struct {
			Items []struct {
				Owner string `json:"owner"`
				Name  string `json:"name"`
			} `json:"items"`
			TotalPages int `json:"totalPages"`
		}
		c.doJSON(http.MethodGet, "/api/web/users/"+owner+"/repos", nil, http.StatusOK, &resp)
		assert.Equal(t, 1, resp.TotalPages)
		var names []string
		for _, item := range resp.Items {
			assert.Equal(t, owner, item.Owner)
			names = append(names, item.Name)
		}
		return names
	}

	assert.ElementsMatch(t, []string{public.Name, private.Name}, repoNames(ownerClient, u.Name))
	assert.Equal(t, []string{public.Name}, repoNames(otherClient, u.Name))
	assert.Equal(t, []string{public.Name}, repoNames(newClient(t), u.Name))

	assert.ElementsMatch(t, []string{orgPublic.Name, orgPrivate.Name}, repoNames(ownerClient, org.Name))
	assert.Equal(t, []string{orgPublic.Name}, repoNames(otherClient, org.Name))
	assert.Equal(t, []string{orgPublic.Name}, repoNames(newClient(t), org.Name))
}

func TestProfileActivity(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, u)
	public := orgCreateRepoThroughWeb(t, c, u.ID, false)
	private := orgCreateRepoThroughWeb(t, c, u.ID, true)

	createdRepos := func(items []orgActivityItem) []string {
		var urls []string
		for _, item := range items {
			if item.OpType == int(database.ActionCreateRepo) {
				assert.Equal(t, u.Name, item.ActorName)
				urls = append(urls, item.RepoURL)
			}
		}
		return urls
	}
	path := "/api/web/users/" + u.Name + "/activity"
	assert.Equal(t, []string{"/" + u.Name + "/" + private, "/" + u.Name + "/" + public}, createdRepos(orgActivity(t, c, path)))
	assert.Equal(t, []string{"/" + u.Name + "/" + public}, createdRepos(orgActivity(t, newClient(t), path)), "private activity shown to anonymous viewers")
	other := authenticatedClient(t, seedUser(t, CreateUserOptions{}))
	assert.Equal(t, []string{"/" + u.Name + "/" + public}, createdRepos(orgActivity(t, other, path)), "private activity shown to others")
}

func TestProfileFollow(t *testing.T) {
	t.Parallel()

	followee := seedUser(t, CreateUserOptions{})
	follower := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, follower)
	followPath := "/api/web/users/" + followee.Name + "/follow"

	people := func(path string) []string {
		t.Helper()
		var resp struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		newClient(t).doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		names := []string{}
		for _, item := range resp.Items {
			names = append(names, item.Name)
		}
		return names
	}
	followersPath := "/api/web/users/" + followee.Name + "/followers"
	followingPath := "/api/web/users/" + follower.Name + "/following"

	status, body := newClient(t).do(http.MethodPost, followPath, orgNoBody)
	assert.Equal(t, http.StatusUnauthorized, status, "signed out, response: %s", body)
	assert.Empty(t, people(followersPath))

	var resp struct {
		FollowerCount int `json:"followerCount"`
	}
	c.doJSON(http.MethodPost, followPath, orgNoBody, http.StatusOK, &resp)
	assert.Equal(t, 1, resp.FollowerCount)
	assert.True(t, database.Handle.Users().IsFollowing(t.Context(), follower.ID, followee.ID))
	assert.Equal(t, []string{follower.Name}, people(followersPath))
	assert.Equal(t, []string{followee.Name}, people(followingPath))
	assert.Empty(t, people("/api/web/users/"+followee.Name+"/following"))

	profile := orgGetProfile(t, c, followee.Name)
	assert.True(t, profile.ViewerIsFollowing)
	assert.Equal(t, 1, profile.FollowerCount)
	assert.Equal(t, 1, orgGetProfile(t, c, follower.Name).FollowingCount)

	c.doJSON(http.MethodDelete, followPath, orgNoBody, http.StatusOK, &resp)
	assert.Equal(t, 0, resp.FollowerCount)
	assert.False(t, database.Handle.Users().IsFollowing(t.Context(), follower.ID, followee.ID))
	assert.Empty(t, people(followersPath))
	assert.Empty(t, people(followingPath))
	assert.False(t, orgGetProfile(t, c, followee.Name).ViewerIsFollowing)
}
