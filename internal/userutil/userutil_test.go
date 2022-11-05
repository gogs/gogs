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
	"gogs.io/gogs/public"
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

	avatarPath := CustomAvatarPath(1)
	defer func() { _ = os.Remove(avatarPath) }()

	err := GenerateRandomAvatar(1, "alice", "alice@example.com")
	require.NoError(t, err)
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)
}

func TestSaveAvatar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	conf.SetMockPicture(t,
		conf.PictureOpts{
			AvatarUploadPath: os.TempDir(),
		},
	)

	avatar, err := public.Files.ReadFile("img/avatar_default.png")
	require.NoError(t, err)

	avatarPath := CustomAvatarPath(1)
	defer func() { _ = os.Remove(avatarPath) }()

	err = SaveAvatar(1, avatar)
	require.NoError(t, err)
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)
}

func TestEncodePassword(t *testing.T) {
	want := EncodePassword("123456", "rands")
	tests := []struct {
		name      string
		password  string
		rands     string
		wantEqual bool
	}{
		{
			name:      "correct",
			password:  "123456",
			rands:     "rands",
			wantEqual: true,
		},

		{
			name:      "wrong password",
			password:  "111333",
			rands:     "rands",
			wantEqual: false,
		},
		{
			name:      "wrong salt",
			password:  "111333",
			rands:     "salt",
			wantEqual: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := EncodePassword(test.password, test.rands)
			if test.wantEqual {
				assert.Equal(t, want, got)
			} else {
				assert.NotEqual(t, want, got)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	want := EncodePassword("123456", "rands")
	tests := []struct {
		name      string
		password  string
		rands     string
		wantEqual bool
	}{
		{
			name:      "correct",
			password:  "123456",
			rands:     "rands",
			wantEqual: true,
		},

		{
			name:      "wrong password",
			password:  "111333",
			rands:     "rands",
			wantEqual: false,
		},
		{
			name:      "wrong salt",
			password:  "111333",
			rands:     "salt",
			wantEqual: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ValidatePassword(want, test.rands, test.password)
			assert.Equal(t, test.wantEqual, got)
		})
	}
}

func TestMailResendCacheKey(t *testing.T) {
	got := MailResendCacheKey(1)
	assert.Equal(t, "mailResend::1", got)
}

func TestTwoFactorCacheKey(t *testing.T) {
	got := TwoFactorCacheKey(1, "113654")
	assert.Equal(t, "twoFactor::1::113654", got)
}

func TestRandomSalt(t *testing.T) {
	salt1, err := RandomSalt()
	require.NoError(t, err)
	salt2, err := RandomSalt()
	require.NoError(t, err)
	assert.NotEqual(t, salt1, salt2)
}
