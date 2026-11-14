package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTag(t *testing.T) {
	ctx := context.Background()
	tag, err := testrepo.Tag(ctx, "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, ObjectTag, tag.Type())
	assert.Equal(t, "b39c8508bbc4b00ad2e24d358012ea123bcafd8d", tag.ID().String())
	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", tag.CommitID().String())

	assert.Equal(t, "The version 1.1.0\n", tag.Message())
}

func TestTag_Commit(t *testing.T) {
	ctx := context.Background()
	tag, err := testrepo.Tag(ctx, "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}

	c, err := tag.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", c.ID.String())
}
