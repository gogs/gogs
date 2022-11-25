// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repoutil

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
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
