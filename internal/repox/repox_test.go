package repox

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCloneLink(t *testing.T) {
	opts := CloneLinkOptions{
		ExternalURL: "https://example.com/",
		RunUser:     "git",
		SSHDomain:   "example.com",
		SSHPort:     22,
	}

	t.Run("regular SSH port", func(t *testing.T) {
		got := NewCloneLink(opts, "alice", "example", false)
		want := &CloneLink{
			SSH:   "git@example.com:alice/example.git",
			HTTPS: "https://example.com/alice/example.git",
		}
		assert.Equal(t, want, got)
	})

	t.Run("irregular SSH port", func(t *testing.T) {
		opts := opts
		opts.SSHPort = 2222
		got := NewCloneLink(opts, "alice", "example", false)
		want := &CloneLink{
			SSH:   "ssh://git@example.com:2222/alice/example.git",
			HTTPS: "https://example.com/alice/example.git",
		}
		assert.Equal(t, want, got)
	})

	t.Run("wiki", func(t *testing.T) {
		got := NewCloneLink(opts, "alice", "example", true)
		want := &CloneLink{
			SSH:   "git@example.com:alice/example.wiki.git",
			HTTPS: "https://example.com/alice/example.wiki.git",
		}
		assert.Equal(t, want, got)
	})
}

func TestHTMLURL(t *testing.T) {
	got := HTMLURL("https://example.com/", "alice", "example")
	want := "https://example.com/alice/example"
	assert.Equal(t, want, got)
}

func TestCompareCommitsPath(t *testing.T) {
	got := CompareCommitsPath("alice", "example", "old", "new")
	want := "alice/example/compare/old...new"
	assert.Equal(t, want, got)
}

func TestUserPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	got := UserPath("/home/git/gogs-repositories", "alice")
	want := "/home/git/gogs-repositories/alice"
	assert.Equal(t, want, got)
}

func TestRepositoryPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	got := RepositoryPath("/home/git/gogs-repositories", "alice", "example")
	want := "/home/git/gogs-repositories/alice/example.git"
	assert.Equal(t, want, got)
}
