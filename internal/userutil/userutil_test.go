// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package userutil

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/tool"
)

func TestDashboardURLPath(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		got := DashboardURLPath("alice", false)
		want := "/"
		assert.Equal(t, want, got)
	})

	t.Run("organization", func(t *testing.T) {
		got := DashboardURLPath("acme", true)
		want := "/org/acme/dashboard/"
		assert.Equal(t, want, got)
	})
}

func TestGenerateActivateCode(t *testing.T) {
	conf.SetMockAuth(t,
		conf.AuthOpts{
			ActivateCodeLives: 10,
		},
	)

	code := GenerateActivateCode(1, "alice@example.com", "Alice", "123456", "rands")
	got := tool.VerifyTimeLimitCode("1alice@example.comalice123456rands", conf.Auth.ActivateCodeLives, code[:tool.TIME_LIMIT_CODE_LENGTH])
	assert.True(t, got)
}

func TestCustomAvatarPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockPicture(t,
		conf.PictureOpts{
			AvatarUploadPath: "data/avatars",
		},
	)

	got := CustomAvatarPath(1)
	want := "data/avatars/1"
	assert.Equal(t, want, got)
}

func TestGenerateRandomAvatar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockPicture(t,
		conf.PictureOpts{
			AvatarUploadPath: os.TempDir(),
		},
	)

	err := GenerateRandomAvatar(1, "alice", "alice@example.com")
	require.NoError(t, err)
	got := osutil.IsFile(CustomAvatarPath(1))
	assert.True(t, got)
}
