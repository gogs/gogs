package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRepoSettingsVisibility(t *testing.T) {
	tests := []struct {
		name              string
		repoIsFork        bool
		basePrivate       bool
		baseUnlisted      bool
		forcePrivate      bool
		actorIsAdmin      bool
		requestedPrivate  bool
		requestedUnlisted bool
		wantPrivate       bool
		wantUnlisted      bool
	}{
		{
			name:              "regular user cannot make repository public when private is forced",
			forcePrivate:      true,
			requestedPrivate:  false,
			requestedUnlisted: true,
			wantPrivate:       true,
			wantUnlisted:      true,
		},
		{
			name:              "site admin can make repository public when private is forced",
			forcePrivate:      true,
			actorIsAdmin:      true,
			requestedPrivate:  false,
			requestedUnlisted: true,
			wantPrivate:       false,
			wantUnlisted:      true,
		},
		{
			name:             "fork visibility follows base repository",
			repoIsFork:       true,
			basePrivate:      true,
			baseUnlisted:     true,
			requestedPrivate: false,
			wantPrivate:      true,
			wantUnlisted:     true,
		},
		{
			name:             "forced private overrides public fork base for regular users",
			repoIsFork:       true,
			forcePrivate:     true,
			requestedPrivate: false,
			wantPrivate:      true,
			wantUnlisted:     false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			private, unlisted := normalizeRepoSettingsVisibility(
				test.repoIsFork,
				test.basePrivate,
				test.baseUnlisted,
				test.forcePrivate,
				test.actorIsAdmin,
				test.requestedPrivate,
				test.requestedUnlisted,
			)

			assert.Equal(t, test.wantPrivate, private)
			assert.Equal(t, test.wantUnlisted, unlisted)
		})
	}
}
