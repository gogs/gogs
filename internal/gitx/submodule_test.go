package gitx

import (
	"testing"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
)

func TestInferSubmoduleURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		submodule *git.Submodule
		expURL    string
	}{
		{
			name: "HTTPS URL",
			submodule: &git.Submodule{
				URL:    "https://github.com/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL with port",
			submodule: &git.Submodule{
				URL:    "ssh://user@github.com:22/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name:    "same instance SSH URL preserves web base",
			baseURL: "https://gogs.example.com:8080/user/repo",
			submodule: &git.Submodule{
				URL:    "ssh://git@gogs.example.com:22/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com:8080/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL in SCP syntax",
			submodule: &git.Submodule{
				URL:    "git@github.com:gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name:    "same instance SCP URL with SSH port preserves web base",
			baseURL: "https://gogs.example.com:8080/user/repo",
			submodule: &git.Submodule{
				URL:    "git@gogs.example.com:2222/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com:8080/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name:    "same instance SCP URL keeps numeric namespace when port is ambiguous",
			baseURL: "https://gogs.example.com:8080/user/repo",
			submodule: &git.Submodule{
				URL:    "git@gogs.example.com:2222/repo.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com:8080/2222/repo/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "external SCP URL keeps numeric namespace",
			submodule: &git.Submodule{
				URL:    "git@example.com:2222/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://example.com/2222/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "relative path",
			submodule: &git.Submodule{
				URL:    "../repo2.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com/user/repo/../repo2/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "bad URL",
			submodule: &git.Submodule{
				URL:    "ftp://example.com",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "ftp://example.com",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseURL := test.baseURL
			if baseURL == "" {
				baseURL = "https://gogs.example.com/user/repo"
			}
			assert.Equal(t, test.expURL, InferSubmoduleURL(baseURL, test.submodule))
		})
	}
}
