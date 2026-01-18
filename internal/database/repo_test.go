package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

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
	owner := &User{Name: "testuser"}
	opts := CreateRepoOptionsLegacy{Name: "safety-test"}

	repoPath := RepoPath(owner.Name, opts.Name)

	// Check the error for MkdirAll
	err := os.MkdirAll(repoPath, os.ModePerm)
	assert.NoError(t, err)

	canary := filepath.Join(repoPath, "canary.txt")

	// Check the error for WriteFile
	err = os.WriteFile(canary, []byte("should survive"), 0644)
	assert.NoError(t, err)

	_, err = CreateRepository(owner, owner, opts)
	if err == nil {
		t.Fatal("Expected error when directory exists, but got nil")
	}

	if !osutil.IsExist(canary) {
		t.Error("CRITICAL: The existing directory was deleted during CreateRepository failure!")
	}
}
