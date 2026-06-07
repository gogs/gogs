package repo

import (
	"net/http"
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
			name:    "head",
			urlPath: "/owner/repo/HEAD",
			owner:   "owner",
			repo:    "repo",
			want:    "HEAD",
		},
		{
			name:    "objects info wildcard",
			urlPath: "/owner/repo/objects/info/exclude",
			owner:   "owner",
			repo:    "repo",
			want:    "objects/info/exclude",
		},
		{
			name:    "loose object",
			urlPath: "/owner/repo/objects/ab/cdef0123456789abcdef0123456789abcdef",
			owner:   "owner",
			repo:    "repo",
			want:    "objects/ab/cdef0123456789abcdef0123456789abcdef",
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

func TestGitHTTPIsPull(t *testing.T) {
	tests := []struct {
		name   string
		method string
		action string
		want   bool
	}{
		{
			name:   "upload pack post is pull",
			method: http.MethodPost,
			action: "git-upload-pack",
			want:   true,
		},
		{
			name:   "receive pack post is push",
			method: http.MethodPost,
			action: "git-receive-pack",
			want:   false,
		},
		{
			name:   "info refs get is pull",
			method: http.MethodGet,
			action: "info/refs",
			want:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := gitHTTPIsPull(tc.method, tc.action)
			assert.Equal(t, tc.want, got)
		})
	}
}
