package db

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/markup"
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
		assert.Equal(t, map[string]string(nil), repo.ComposeMetas())

		// Should be nil even if other settings are present
		repo.ExternalTrackerStyle = markup.ISSUE_NAME_STYLE_NUMERIC
		assert.Equal(t, map[string]string(nil), repo.ComposeMetas())
	})

	t.Run("an external issue tracker is configured", func(t *testing.T) {
		repo.EnableExternalTracker = true

		// Default to numeric issue style
		assert.Equal(t, markup.ISSUE_NAME_STYLE_NUMERIC, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.ISSUE_NAME_STYLE_NUMERIC
		assert.Equal(t, markup.ISSUE_NAME_STYLE_NUMERIC, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		repo.ExternalTrackerStyle = markup.ISSUE_NAME_STYLE_ALPHANUMERIC
		assert.Equal(t, markup.ISSUE_NAME_STYLE_ALPHANUMERIC, repo.ComposeMetas()["style"])
		repo.ExternalMetas = nil

		metas := repo.ComposeMetas()
		assert.Equal(t, "testuser", metas["user"])
		assert.Equal(t, "testrepo", metas["repo"])
		assert.Equal(t, "https://someurl.com/{user}/{repo}/{issue}", metas["format"])
	})
}
