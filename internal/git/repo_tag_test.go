package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepository_Tag(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		opt    TagOptions
		expTag *Tag
	}{
		{
			name: "v1.0.0",
			expTag: &Tag{
				typ:      ObjectCommit,
				id:       MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
				commitID: MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
				refspec:  "refs/tags/v1.0.0",
			},
		}, {
			name: "v1.1.0",
			expTag: &Tag{
				typ:      ObjectTag,
				id:       MustIDFromString("b39c8508bbc4b00ad2e24d358012ea123bcafd8d"),
				commitID: MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
				refspec:  "refs/tags/v1.1.0",
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			tag, err := testrepo.Tag(ctx, test.name, test.opt)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expTag.Type(), tag.Type())
			assert.Equal(t, test.expTag.ID().String(), tag.ID().String())
			assert.Equal(t, test.expTag.CommitID().String(), tag.CommitID().String())
			assert.Equal(t, test.expTag.Refspec(), tag.Refspec())
		})
	}
}

func TestRepository_Tags(t *testing.T) {
	ctx := context.Background()
	// Make sure it does not blow up
	tags, err := testrepo.Tags(ctx, TagsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, tags)
}

func TestRepository_Tags_VersionSort(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	err = r.CreateTag(ctx, "v3.0.0", "master")
	if err != nil {
		t.Fatal(err)
	}
	err = r.CreateTag(ctx, "v2.999.0", "master")
	if err != nil {
		t.Fatal(err)
	}

	tags, err := r.Tags(ctx, TagsOptions{
		SortKey: "-version:refname",
		Pattern: "v*",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(tags) < 2 {
		t.Fatalf("Should have at least two tags but got %d", len(tags))
	}
	assert.Equal(t, "v3.0.0", tags[0])
	assert.Equal(t, "v2.999.0", tags[1])
}

func TestRepository_CreateTag(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	assert.False(t, r.HasReference(ctx, RefsTags+"v2.0.0"))

	err = r.CreateTag(ctx, "v2.0.0", "master", CreateTagOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, r.HasReference(ctx, RefsTags+"v2.0.0"))
}

func TestRepository_CreateAnnotatedTag(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	assert.False(t, r.HasReference(ctx, RefsTags+"v2.0.0"))

	err = r.CreateTag(ctx, "v2.0.0", "master", CreateTagOptions{
		Annotated: true,
		Author: &Signature{
			Name:  "alice",
			Email: "alice@example.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, r.HasReference(ctx, RefsTags+"v2.0.0"))

	tag, err := r.Tag(ctx, "v2.0.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "alice", tag.tagger.Name)
	assert.Equal(t, "alice@example.com", tag.tagger.Email)
	assert.False(t, tag.tagger.When.IsZero())
}

func TestRepository_DeleteTag(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	assert.True(t, r.HasReference(ctx, RefsTags+"v1.0.0"))

	err = r.DeleteTag(ctx, "v1.0.0", DeleteTagOptions{})
	if err != nil {
		t.Fatal(err)
	}

	assert.False(t, r.HasReference(ctx, RefsTags+"v1.0.0"))
}
