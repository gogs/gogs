// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repoutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/pathutil"
)

func TestNewCloneLink(t *testing.T) {
	conf.SetMockApp(t,
		conf.AppOpts{
			RunUser: "git",
		},
	)
	conf.SetMockServer(t,
		conf.ServerOpts{
			ExternalURL: "https://example.com/",
		},
	)

	t.Run("regular SSH port", func(t *testing.T) {
		conf.SetMockSSH(t,
			conf.SSHOpts{
				Domain: "example.com",
				Port:   22,
			},
		)

		got := NewCloneLink("alice", "example", false)
		want := &CloneLink{
			SSH:   "git@example.com:alice/example.git",
			HTTPS: "https://example.com/alice/example.git",
		}
		assert.Equal(t, want, got)
	})

	t.Run("irregular SSH port", func(t *testing.T) {
		conf.SetMockSSH(t,
			conf.SSHOpts{
				Domain: "example.com",
				Port:   2222,
			},
		)

		got := NewCloneLink("alice", "example", false)
		want := &CloneLink{
			SSH:   "ssh://git@example.com:2222/alice/example.git",
			HTTPS: "https://example.com/alice/example.git",
		}
		assert.Equal(t, want, got)
	})

	t.Run("wiki", func(t *testing.T) {
		conf.SetMockSSH(t,
			conf.SSHOpts{
				Domain: "example.com",
				Port:   22,
			},
		)

		got := NewCloneLink("alice", "example", true)
		want := &CloneLink{
			SSH:   "git@example.com:alice/example.wiki.git",
			HTTPS: "https://example.com/alice/example.wiki.git",
		}
		assert.Equal(t, want, got)
	})
}

func TestHTMLURL(t *testing.T) {
	conf.SetMockServer(t,
		conf.ServerOpts{
			ExternalURL: "https://example.com/",
		},
	)

	got := HTMLURL("alice", "example")
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

	conf.SetMockRepository(t,
		conf.RepositoryOpts{
			Root: "/home/git/gogs-repositories",
		},
	)

	got := UserPath("alice")
	want := "/home/git/gogs-repositories/alice"
	assert.Equal(t, want, got)
}

func TestRepositoryPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockRepository(t,
		conf.RepositoryOpts{
			Root: "/home/git/gogs-repositories",
		},
	)

	got := RepositoryPath("alice", "example")
	want := "/home/git/gogs-repositories/alice/example.git"
	assert.Equal(t, want, got)
}

func TestRepositoryLocalPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockServer(
		t,
		conf.ServerOpts{
			AppDataPath: "data",
		},
	)

	got := RepositoryLocalPath(1)
	want := "data/tmp/local-repo/1"
	assert.Equal(t, want, got)
}

func TestRepositoryLocalWikiPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockServer(
		t,
		conf.ServerOpts{
			AppDataPath: "data",
		},
	)

	got := RepositoryLocalWikiPath(1)
	want := "data/tmp/local-wiki/1"
	assert.Equal(t, want, got)
}

// TestValidatePathWithin_SymlinkTraversal ensures that ValidatePathWithin
// detects symlink traversal attacks.
func TestValidatePathWithin_SymlinkTraversal(t *testing.T) {
	// Setup mock repository root under a temp directory
	repoRoot := t.TempDir()
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: repoRoot})

	owner := "alice"
	repo := "example"
	repoPath := RepositoryPath(owner, repo)

	// create repo directory (permissions must be 0750 or less)
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	// create an external target outside the repo root
	external := t.TempDir()
	targetFile := filepath.Join(external, "outside.txt")
	if err := os.WriteFile(targetFile, []byte("malicious"), 0o600); err != nil {
		t.Fatalf("failed to write external file: %v", err)
	}

	// create a symlink inside the repo that points to the external file
	linkPath := filepath.Join(repoPath, "malicious_link")
	if err := os.Symlink(targetFile, linkPath); err != nil {
		// On some platforms symlink may require privileges; skip if not supported
		t.Skipf("symlink not supported: %v", err)
	}

	// ValidatePathWithin should detect symlink traversal when checking the link
	err := ValidatePathWithin(repoPath, "malicious_link")
	assert.Error(t, err)
	assert.True(t, pathutil.IsErrSymlinkTraversal(err), "expected symlink traversal error")
}
