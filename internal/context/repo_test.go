package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_matchRepoRef(t *testing.T) {
	tests := []struct {
		name        string
		rawPath     string
		branches    []string
		tags        []string
		expRefName  string
		expTreePath string
		expOK       bool
	}{
		{
			name:       "prefers longer tag over branch prefix",
			rawPath:    "newbranch/v1",
			branches:   []string{"newbranch"},
			tags:       []string{"newbranch/v1"},
			expRefName: "newbranch/v1",
			expOK:      true,
		},
		{
			name:        "keeps tree path after branch",
			rawPath:     "main/docs/readme.md",
			branches:    []string{"main"},
			expRefName:  "main",
			expTreePath: "docs/readme.md",
			expOK:       true,
		},
		{
			name:        "keeps tree path after branch with slash",
			rawPath:     "feature/search/docs/readme.md",
			branches:    []string{"feature/search"},
			expRefName:  "feature/search",
			expTreePath: "docs/readme.md",
			expOK:       true,
		},
		{
			name:    "returns false when ref is missing",
			rawPath: "unknown/path",
			expOK:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hasBranch := stringSet(test.branches)
			hasTag := stringSet(test.tags)

			refName, treePath, ok := matchRepoRef(test.rawPath, hasBranch, hasTag)
			require.Equal(t, test.expOK, ok)
			assert.Equal(t, test.expRefName, refName)
			assert.Equal(t, test.expTreePath, treePath)
		})
	}
}

func stringSet(items []string) func(string) bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}

	return func(item string) bool {
		return set[item]
	}
}
