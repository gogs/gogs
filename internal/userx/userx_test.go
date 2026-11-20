package userx

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/tool"
)

func TestTimeLimitedUserCode(t *testing.T) {
	conf.SetMockAuth(t,
		conf.AuthOpts{
			ActivateCodeLives:      10,
			ResetPasswordCodeLives: 10,
		},
	)

	activateCode := GenerateActivateCode(1, "alice@example.com", "Alice", "123456", "rands")
	resetPasswordCode := GenerateResetPasswordCode(1, "alice@example.com", "Alice", "123456", "rands")
	confirmEmailCode := GenerateConfirmEmailCode(1, "alice@example.com", "Alice", "123456", "rands")

	require.True(t, VerifyActivateCode(activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	require.True(t, VerifyResetPasswordCode(resetPasswordCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	require.True(t, VerifyConfirmEmailCode(confirmEmailCode, 1, "alice@example.com", "Alice", "123456", "rands"))

	t.Run("codes are not interchangeable", func(t *testing.T) {
		assert.False(t, VerifyActivateCode(resetPasswordCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyResetPasswordCode(activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyActivateCode(confirmEmailCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyConfirmEmailCode(activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	})

	t.Run("user information changed", func(t *testing.T) {
		assert.False(t, VerifyActivateCode(activateCode, 1, "alice@example.com", "Alice", "123456", "new-rands"))
		assert.False(t, VerifyResetPasswordCode(resetPasswordCode, 1, "alice@example.com", "Alice", "654321", "rands"))
	})

	t.Run("malformed code", func(t *testing.T) {
		assert.False(t, VerifyActivateCode("", 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyActivateCode(activateCode[:tool.TimeLimitCodeLength], 1, "alice@example.com", "Alice", "123456", "rands"))
	})
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

	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	avatar := img.Bytes()

	avatarPath := CustomAvatarPath(1)
	defer func() { _ = os.Remove(avatarPath) }()

	err := SaveAvatar(1, avatar)
	require.NoError(t, err)
	got := osx.IsFile(avatarPath)
	assert.True(t, got)
}

func TestSaveGeneratedAvatar(t *testing.T) {
	conf.SetMockPicture(t,
		conf.PictureOpts{
			AvatarUploadPath: t.TempDir(),
		},
	)

	require.NoError(t, SaveGeneratedAvatar(1))
	data, err := os.ReadFile(CustomAvatarPath(1))
	require.NoError(t, err)
	_, err = png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)

	// An avatar uploaded while the generated one was drawn is kept.
	require.NoError(t, os.WriteFile(CustomAvatarPath(2), []byte("uploaded"), 0o600))
	require.NoError(t, SaveGeneratedAvatar(2))
	data, err = os.ReadFile(CustomAvatarPath(2))
	require.NoError(t, err)
	assert.Equal(t, "uploaded", string(data))

	entries, err := os.ReadDir(conf.Picture.AvatarUploadPath)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "temporary files are removed")
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
