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
		expTag *Tag
	}{
		{
			name: "v1.0.0",
			expTag: &Tag{
				typ:      ObjectCommit,
				id:       MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
				commitID: MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
			},
		}, {
			name: "v1.1.0",
			expTag: &Tag{
				typ:      ObjectTag,
				id:       MustIDFromString("b39c8508bbc4b00ad2e24d358012ea123bcafd8d"),
				commitID: MustIDFromString("0eedd79eba4394bbef888c804e899731644367fe"),
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			tag, err := testrepo.Tag(ctx, test.name)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expTag.Type(), tag.Type())
			assert.Equal(t, test.expTag.ID().String(), tag.ID().String())
			assert.Equal(t, test.expTag.CommitID().String(), tag.CommitID().String())
		})
	}
}

func TestRepository_Tags(t *testing.T) {
	ctx := context.Background()
	// Make sure it does not blow up
	tags, err := testrepo.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assert.NotEmpty(t, tags)
}

func TestRepository_CreateTag(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	assert.False(t, r.hasReference(ctx, RefsTags+"v2.0.0"))

	err = r.CreateTag(ctx, "v2.0.0", "master")
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, r.hasReference(ctx, RefsTags+"v2.0.0"))
}

func TestRepository_DeleteTag(t *testing.T) {
	ctx := context.Background()
	r, cleanup, err := setupTempRepo()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	assert.True(t, r.hasReference(ctx, RefsTags+"v1.0.0"))

	err = r.DeleteTag(ctx, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	assert.False(t, r.hasReference(ctx, RefsTags+"v1.0.0"))
}

func TestTagsAfter(t *testing.T) {
	all := []string{
		"v2.3.0", "v2.2.1", "v2.1.0",
		"v1.3.0", "v1.2.0", "v1.1.0",
		"v0.8.0", "v0.5.0", "v0.1.0",
	}

	tests := []struct {
		name        string
		after       string
		expTagsPage *TagsPage
	}{
		{
			name: "first page",
			expTagsPage: &TagsPage{
				Tags: []string{
					"v2.3.0", "v2.2.1", "v2.1.0",
				},
				HasLatest: true,
				HasNext:   true,
			},
		},
		{
			name:  "second page",
			after: "v2.1.0",
			expTagsPage: &TagsPage{
				Tags: []string{
					"v1.3.0", "v1.2.0", "v1.1.0",
				},
				HasLatest: false,
				HasNext:   true,
			},
		},
		{
			name:  "last page",
			after: "v1.1.0",
			expTagsPage: &TagsPage{
				Tags: []string{
					"v0.8.0", "v0.5.0", "v0.1.0",
				},
				HasLatest:     false,
				PreviousAfter: "v2.1.0",
				HasNext:       false,
			},
		},

		{
			name:  "arbitrary after",
			after: "v1.2.0",
			expTagsPage: &TagsPage{
				Tags: []string{
					"v1.1.0", "v0.8.0", "v0.5.0",
				},
				HasLatest:     false,
				PreviousAfter: "v2.2.1",
				HasNext:       true,
			},
		},
		{
			name:  "after the oldest one",
			after: "v0.1.0",
			expTagsPage: &TagsPage{
				Tags:          []string{},
				HasLatest:     false,
				PreviousAfter: "v1.1.0",
				HasNext:       false,
			},
		},
		{
			name:  "after does not exist",
			after: "v2.2.9",
			expTagsPage: &TagsPage{
				Tags: []string{
					"v2.3.0", "v2.2.1", "v2.1.0",
				},
				HasLatest: true,
				HasNext:   true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expTagsPage, tagsAfter(all, test.after, 3))
		})
	}
}
