package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommit_Submodule(t *testing.T) {
	ctx := context.Background()

	c, err := testrepo.CatFileCommit(ctx, "4e59b72440188e7c2578299fc28ea425fbe9aece")
	if err != nil {
		t.Fatal(err)
	}

	mod, err := c.Submodule(ctx, "gogs/docs-api")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "gogs/docs-api", mod.Name)
	assert.Equal(t, "https://github.com/gogs/docs-api.git", mod.URL)
	assert.Equal(t, "6b08f76a5313fa3d26859515b30aa17a5faa2807", mod.Commit)

	_, err = c.Submodule(ctx, "404")
	assert.Equal(t, ErrSubmoduleNotExist, err)
}

func TestInferSubmoduleURL(t *testing.T) {
	tests := []struct {
		name      string
		submodule *Submodule
		expURL    string
	}{
		{
			name: "HTTPS URL",
			submodule: &Submodule{
				URL:    "https://github.com/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL with port",
			submodule: &Submodule{
				URL:    "ssh://user@github.com:22/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL in SCP syntax",
			submodule: &Submodule{
				URL:    "git@github.com:gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "relative path",
			submodule: &Submodule{
				URL:    "../repo2.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com/user/repo/../repo2/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "bad URL",
			submodule: &Submodule{
				URL:    "ftp://example.com",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "ftp://example.com",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expURL, InferSubmoduleURL("https://gogs.example.com/user/repo", test.submodule))
		})
	}
}
