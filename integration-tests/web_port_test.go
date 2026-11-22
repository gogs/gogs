package integrationtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWebPortOverride clones a repository from the clone URL given by servers
// started with `gogs web --port` while the port they are configured with is
// taken.
func TestWebPortOverride(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	tests := []struct {
		name      string
		serverURL string
	}{
		{name: "served directly", serverURL: portOverrideServerURL},
		{name: "behind a reverse proxy", serverURL: proxiedServerURL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := newClient(t)
			c.baseURL = test.serverURL
			var got struct {
				CloneURL string `json:"clone_url"`
			}
			c.doJSON(http.MethodGet, apiV1RepoPath(repo, ""), nil, http.StatusOK, &got)
			assert.Equal(t, test.serverURL+"/"+repo.Owner+"/"+repo.Name+".git", got.CloneURL)

			dir := gitClone(t, got.CloneURL)
			assert.Equal(t, gitMainOnDisk(t, repo), gitRun(t, dir, "rev-parse", "HEAD"))
		})
	}
}
