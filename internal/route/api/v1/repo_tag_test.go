package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-macaron/binding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	gcontext "gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/dbtest"
)

// initTestRepo creates a temporary git repository with an initial commit and
// returns its path and the HEAD commit SHA.
func initTestRepo(t *testing.T, root, owner, repo string) (repoPath, commitSHA string) {
	t.Helper()

	repoPath = filepath.Join(root, strings.ToLower(owner), strings.ToLower(repo)+".git")
	err := os.MkdirAll(repoPath, os.ModePerm)
	require.NoError(t, err)

	cmds := [][]string{
		{"git", "init", repoPath},
		{"git", "-C", repoPath, "config", "user.email", "test@example.com"},
		{"git", "-C", repoPath, "config", "user.name", "Test User"},
		{"git", "-C", repoPath, "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, string(out))
	}

	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	commitSHA = strings.TrimSpace(string(out))
	return repoPath, commitSHA
}

// newTestMacaron creates a Macaron instance that injects a minimal APIContext
// backed by the given repository root, owner, and repo name.
func newTestMacaron(ownerName, repoName string) *macaron.Macaron {
	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(ctx *macaron.Context) {
		owner := &database.User{Name: ownerName}
		repo := &database.Repository{
			Name:  repoName,
			Owner: owner,
		}
		contextRepo := &gcontext.Repository{
			Repository: repo,
		}
		c := &gcontext.Context{
			Context: ctx,
			Repo:    contextRepo,
		}
		apiCtx := &gcontext.APIContext{
			Context: c,
		}
		ctx.Map(apiCtx)
	})
	return m
}

func TestCreateTag(t *testing.T) {
	root := t.TempDir()
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: root})
	db := dbtest.NewDB(t, "repo_tag", new(database.User), new(database.EmailAddress))
	database.SetMockHandle(t, db)

	const ownerName = "testuser"
	const repoName = "testrepo"
	_, commitSHA := initTestRepo(t, root, ownerName, repoName)

	tests := []struct {
		name          string
		body          map[string]string
		expStatusCode int
		expBody       string
	}{
		{
			name:          "success",
			body:          map[string]string{"name": "v1.0.0", "commit": commitSHA},
			expStatusCode: http.StatusCreated,
		},
		{
			// Reuse the same tag name to verify the "already exists" error.
			name:          "tag name already exists",
			body:          map[string]string{"name": "v1.0.0", "commit": commitSHA},
			expStatusCode: http.StatusUnprocessableEntity,
			expBody:       "tag already exists",
		},
		{
			name:          "commit does not exist",
			body:          map[string]string{"name": "v2.0.0", "commit": "0000000000000000000000000000000000000000"},
			expStatusCode: http.StatusUnprocessableEntity,
		},
		{
			name:          "missing name field",
			body:          map[string]string{"commit": commitSHA},
			expStatusCode: http.StatusUnprocessableEntity,
		},
		{
			name:          "missing commit field",
			body:          map[string]string{"name": "v3.0.0"},
			expStatusCode: http.StatusUnprocessableEntity,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newTestMacaron(ownerName, repoName)
			m.Post("/:username/:reponame", binding.Bind(createTagRequest{}), createTag)

			bodyBytes, err := json.Marshal(test.body)
			require.NoError(t, err)

			r, err := http.NewRequest(http.MethodPost, "/"+ownerName+"/"+repoName, bytes.NewReader(bodyBytes))
			require.NoError(t, err)
			r.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			assert.Equal(t, test.expStatusCode, rr.Code)
			if test.expBody != "" {
				assert.Contains(t, rr.Body.String(), test.expBody)
			}
		})
	}
}
