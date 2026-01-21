package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osutil"
)

func TestRepository_ComposeMetas(t *testing.T) {
	repo := &Repository{
		Name: "testrepo",
		Owner: &User{
			Name: "testuser",
		},
		ExternalTrackerFormat: "https://someurl.com/{user}/{repo}/{issue}",
	}

	t.Run("no external tracker is configured", func(t *testing.T) {
		repo.EnableExternalTracker = false

		metas := repo.ComposeMetas()
		assert.Equal(t, metas["repoLink"], repo.Link())

		// Should not have format and style if no external tracker is configured
		_, ok := metas["format"]
		assert.False(t, ok)
		_, ok = metas["style"]
		assert.False(t, ok)
	})

	t.Run("an external issue tracker is configured", func(t *testing.T) {
		repo.ExternalMetas = nil
		repo.EnableExternalTracker = true

		// Default to numeric issue style
		assert.Equal(t, markup.IssueNameStyleNumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.IssueNameStyleNumeric
		assert.Equal(t, markup.IssueNameStyleNumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.IssueNameStyleAlphanumeric
		assert.Equal(t, markup.IssueNameStyleAlphanumeric, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		metas := repo.ComposeMetas()
		assert.Equal(t, "testuser", metas["user"])
		assert.Equal(t, "testrepo", metas["repo"])
		assert.Equal(t, "https://someurl.com/{user}/{repo}/{issue}", metas["format"])
	})
}
func Test_CreateRepository_PreventDeletion(t *testing.T) {
	conf.Repository.MaxCreationLimit = -1

	// 2. Setup user with permission
	owner := &User{Name: "testuser", MaxRepoCreation: -1}
	opts := CreateRepoOptionsLegacy{Name: "safety-test"}
	repoPath := RepoPath(owner.Name, opts.Name)

	_ = os.RemoveAll(repoPath)
	require.NoError(t, os.MkdirAll(repoPath, os.ModePerm))

	canary := filepath.Join(repoPath, "canary.txt")
	require.NoError(t, os.WriteFile(canary, []byte("should survive"), 0o644))

	// 3. This call will now pass the limit check and hit the directory check
	_, err := CreateRepository(owner, owner, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repository directory already exists")
	assert.True(t, osutil.IsExist(canary))
}

func Test_CreateRepository(t *testing.T) {
	// 1. Initialize the test database
	if x == nil {
		t.Skip("Database engine not initialized. Ensure you are running tests with a test database setup.")
	}

	// 2. Bypass the repo limit check
	conf.Repository.MaxCreationLimit = -1

	owner := &User{ID: 1, Name: "unknwon", MaxRepoCreation: -1}
	opts := CreateRepoOptionsLegacy{
		Name:  "test-alias-repo",
		Alias: "My 🚀 Alias",
	}

	// 3. Run the function
	repo, err := CreateRepository(owner, owner, opts)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	assert.Equal(t, "My 🚀 Alias", repo.Alias)
}
