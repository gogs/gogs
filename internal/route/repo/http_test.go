package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitHTTPActionFromPath(t *testing.T) {
	tests := []struct {
		name    string
		urlPath string
		subpath string
		owner   string
		repo    string
		want    string
	}{
		{
			name:    "info refs",
			urlPath: "/owner/repo/info/refs",
			owner:   "owner",
			repo:    "repo",
			want:    "info/refs",
		},
		{
			name:    "git suffix repo",
			urlPath: "/owner/repo.git/git-receive-pack",
			owner:   "owner",
			repo:    "repo.git",
			want:    "git-receive-pack",
		},
		{
			name:    "with subpath",
			urlPath: "/gogs/owner/repo/info/refs",
			subpath: "/gogs",
			owner:   "owner",
			repo:    "repo",
			want:    "info/refs",
		},
		{
			name:    "non git suffix",
			urlPath: "/owner/repo/src/main.go",
			owner:   "owner",
			repo:    "repo",
			want:    "src/main.go",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := gitHTTPActionFromPath(tc.urlPath, tc.subpath, tc.owner, tc.repo)
			assert.Equal(t, tc.want, got)
		})
	}
}
