package repo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func TestHTTP(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "alice", "repo.git")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))
	output, err := exec.Command("git", "init", "--bare", repoPath).CombinedOutput()
	require.NoError(t, err, string(output))

	oldRoot := conf.Repository.Root
	conf.Repository.Root = root
	t.Cleanup(func() { conf.Repository.Root = oldRoot })

	var isPull bool
	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Route("/:username/:reponame/*", "GET,POST",
		func(c *macaron.Context) {
			isPull = gitHTTPIsPull(c, gitHTTPAction(c))
			c.Map(&HTTPContext{
				Context:  c,
				AuthUser: &database.User{},
			})
		},
		HTTP,
	)

	tests := []struct {
		name            string
		method          string
		url             string
		wantStatus      int
		wantContentType string
	}{
		{
			name:            "info refs for upload-pack",
			method:          http.MethodGet,
			url:             "/alice/repo.git/info/refs?service=git-upload-pack",
			wantStatus:      http.StatusOK,
			wantContentType: "application/x-git-upload-pack-advertisement",
		},
		{
			name:            "info refs for receive-pack",
			method:          http.MethodGet,
			url:             "/alice/repo.git/info/refs?service=git-receive-pack",
			wantStatus:      http.StatusOK,
			wantContentType: "application/x-git-receive-pack-advertisement",
		},
		{
			name:            "info refs with mixed-case owner and repository names",
			method:          http.MethodGet,
			url:             "/Alice/Repo.git/info/refs?service=git-upload-pack",
			wantStatus:      http.StatusOK,
			wantContentType: "application/x-git-upload-pack-advertisement",
		},
		{
			name:       "info refs without service",
			method:     http.MethodGet,
			url:        "/alice/repo.git/info/refs",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HEAD",
			method:     http.MethodGet,
			url:        "/alice/repo.git/HEAD",
			wantStatus: http.StatusOK,
		},
		{
			name:            "receive-pack",
			method:          http.MethodPost,
			url:             "/alice/repo.git/git-receive-pack",
			wantContentType: "application/x-git-receive-pack-result",
		},

		{
			name:       "info refs for mixed-case receive-pack",
			method:     http.MethodGet,
			url:        "/alice/repo.git/info/refs?service=git-RECEIVE-pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "info refs for receive-pack without prefix",
			method:     http.MethodGet,
			url:        "/alice/repo.git/info/refs?service=receive-pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "uppercase info refs",
			method:     http.MethodGet,
			url:        "/alice/repo.git/INFO/REFS?service=git-receive-pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "mixed-case receive-pack",
			method:     http.MethodPost,
			url:        "/alice/repo.git/git-RECEIVE-pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "title-case receive-pack",
			method:     http.MethodPost,
			url:        "/alice/repo.git/git-Receive-Pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "uppercase receive-pack",
			method:     http.MethodPost,
			url:        "/alice/repo.git/GIT-RECEIVE-PACK",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "percent-encoded mixed-case receive-pack",
			method:     http.MethodPost,
			url:        "/alice/repo.git/git-%52eceive-pack",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "mixed-case upload-pack",
			method:     http.MethodPost,
			url:        "/alice/repo.git/git-UPLOAD-pack",
			wantStatus: http.StatusNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.url, strings.NewReader(""))
			if test.method == http.MethodPost {
				service := strings.TrimPrefix(strings.ToLower(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]), "git-")
				r.Header.Set("Content-Type", "application/x-git-"+service+"-request")
			}
			isPull = false
			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			// 🚨 SECURITY: A request served as receive-pack must have been authorized
			// as a push.
			if strings.Contains(rr.Header().Get("Content-Type"), "receive-pack") {
				assert.False(t, isPull, "receive-pack was served to a request authorized as a pull")
			}

			if test.wantStatus != 0 {
				assert.Equal(t, test.wantStatus, rr.Code)
			}
			if test.wantContentType != "" {
				assert.Equal(t, test.wantContentType, rr.Header().Get("Content-Type"))
			}
		})
	}
}
