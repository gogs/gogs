package integrationtest

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositorySourceVisibility(t *testing.T) {
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
	otherClient := newClient(t)
	otherClient.signIn(other)
	ownerClient := newClient(t)
	ownerClient.signIn(owner)

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

func TestIssueCreatedThroughAPIV1(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	token, err := CreateAccessToken(t.Context(), owner.ID)
	require.NoError(t, err)

	api := newClient(t)
	api.header.Set("Authorization", "token "+token)
	var created struct {
		Number int64 `json:"number"`
	}
	api.doJSON(http.MethodPost, "/api/v1/repos/"+repo.Owner+"/"+repo.Name+"/issues", map[string]any{
		"title": "Crash on start",
		"body":  "It **crashes**.",
	}, http.StatusCreated, &created)

	var issue struct {
		Index       int64  `json:"index"`
		Title       string `json:"title"`
		ContentHTML string `json:"contentHTML"`
		Poster      struct {
			Name string `json:"name"`
		} `json:"poster"`
	}
	newClient(t).doJSON(http.MethodGet, "/api/web/"+repo.Owner+"/"+repo.Name+"/issues/"+strconv.FormatInt(created.Number, 10), nil, http.StatusOK, &issue)
	assert.Equal(t, created.Number, issue.Index)
	assert.Equal(t, "Crash on start", issue.Title)
	assert.Contains(t, issue.ContentHTML, "<strong>crashes</strong>")
	assert.Equal(t, owner.Name, issue.Poster.Name)
}
