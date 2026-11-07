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
