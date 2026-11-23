package database

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepoPathAndWikiPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	const root = "/home/git/gogs-repositories"

	tests := []struct {
		repoName string
		wantRepo string
		wantWiki string
	}{
		{
			repoName: "Example",
			wantRepo: "/home/git/gogs-repositories/alice/example.git",
			wantWiki: "/home/git/gogs-repositories/alice/example.wiki.git",
		},
		{
			repoName: "../../bob/secret",
			wantRepo: "/home/git/gogs-repositories/alice/bob/secret.git",
			wantWiki: "/home/git/gogs-repositories/alice/bob/secret.wiki.git",
		},
		{
			repoName: `..\..\bob\secret`,
			wantRepo: "/home/git/gogs-repositories/alice/bob/secret.git",
			wantWiki: "/home/git/gogs-repositories/alice/bob/secret.wiki.git",
		},
	}
	for _, test := range tests {
		t.Run(test.repoName, func(t *testing.T) {
			assert.Equal(t, test.wantRepo, RepoPath(root, "alice", test.repoName))
			assert.Equal(t, test.wantWiki, WikiPath(root, "alice", test.repoName))
		})
	}
}

func TestCleanWikiPageName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "Home", want: "Home"},
		// Typed titles are not URLs, so they are not unescaped.
		{input: "C++", want: "C++"},
		{input: "100% done", want: "100% done"},
		{input: "../../../../tmp/target_file", want: "tmp target_file"},
		{input: "..\\..\\tmp\\target_file", want: "tmp target_file"},
		{input: "A/B", want: "A B"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			assert.Equal(t, test.want, CleanWikiPageName(test.input))
		})
	}
}
