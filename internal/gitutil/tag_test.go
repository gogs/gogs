// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"testing"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
)

func TestModuler_ListTagsAfter(t *testing.T) {
	SetMockModuleStore(t, &MockModuleStore{
		repoTags: func(string, ...git.TagsOptions) ([]string, error) {
			return []string{
				"v2.3.0", "v2.2.1", "v2.1.0",
				"v1.3.0", "v1.2.0", "v1.1.0",
				"v0.8.0", "v0.5.0", "v0.1.0",
			}, nil
		},

		listTagsAfter: Module.ListTagsAfter,
	})

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
			tagsPage, err := Module.ListTagsAfter("", test.after, 3)
			if err != nil {
				t.Fatal(err)
			}

			assert.Equal(t, test.expTagsPage, tagsPage)
		})
	}
}
