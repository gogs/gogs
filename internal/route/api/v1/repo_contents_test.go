package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPutContentsBranches(t *testing.T) {
	tests := []struct {
		name          string
		targetBranch  string
		existing      map[string]bool
		wantOldBranch string
		wantNewBranch string
	}{
		{
			name:          "empty target uses default branch",
			existing:      map[string]bool{"main": true},
			wantOldBranch: "main",
			wantNewBranch: "main",
		},
		{
			name:          "existing target updates target branch",
			targetBranch:  "release",
			existing:      map[string]bool{"main": true, "release": true},
			wantOldBranch: "release",
			wantNewBranch: "release",
		},
		{
			name:          "missing target creates from default branch",
			targetBranch:  "feature",
			existing:      map[string]bool{"main": true},
			wantOldBranch: "main",
			wantNewBranch: "feature",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldBranch, newBranch := putContentsBranches("main", test.targetBranch, func(branch string) bool {
				return test.existing[branch]
			})

			assert.Equal(t, test.wantOldBranch, oldBranch)
			assert.Equal(t, test.wantNewBranch, newBranch)
		})
	}
}
