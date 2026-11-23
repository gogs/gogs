package userx

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/osx"
)

func TestVerifyTimeLimitCode(t *testing.T) {
	const format = "200601021504"
	issued := time.Now().Add(-30 * time.Minute).Format(format)
	code := createTimeLimitCode("secret", "data", 60, issued)

	assert.True(t, verifyTimeLimitCode("secret", "data", 60, code))
	assert.False(t, verifyTimeLimitCode("secret", "other", 60, code))

	t.Run("expires by the issued lifetime", func(t *testing.T) {
		assert.False(t, verifyTimeLimitCode("secret", "data", 60, createTimeLimitCode("secret", "data", 20, issued)))
	})

	t.Run("expires by a lowered configured lifetime", func(t *testing.T) {
		assert.False(t, verifyTimeLimitCode("secret", "data", 20, code))
	})

	t.Run("raised configured lifetime does not extend", func(t *testing.T) {
		assert.False(t, verifyTimeLimitCode("secret", "data", 600, createTimeLimitCode("secret", "data", 20, issued)))
	})
}

func TestTimeLimitedUserCode(t *testing.T) {
	activateCode := GenerateActivateCode("secret", 10, 1, "alice@example.com", "Alice", "123456", "rands")
	resetPasswordCode := GenerateResetPasswordCode("secret", 10, 1, "alice@example.com", "Alice", "123456", "rands")
	confirmEmailCode := GenerateConfirmEmailCode("secret", 10, 1, "alice@example.com", "Alice", "123456", "rands")

	require.True(t, VerifyActivateCode("secret", 10, activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	require.True(t, VerifyResetPasswordCode("secret", 10, resetPasswordCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	require.True(t, VerifyConfirmEmailCode("secret", 10, confirmEmailCode, 1, "alice@example.com", "Alice", "123456", "rands"))

	t.Run("codes are not interchangeable", func(t *testing.T) {
		assert.False(t, VerifyActivateCode("secret", 10, resetPasswordCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyResetPasswordCode("secret", 10, activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyActivateCode("secret", 10, confirmEmailCode, 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyConfirmEmailCode("secret", 10, activateCode, 1, "alice@example.com", "Alice", "123456", "rands"))
	})

	t.Run("user information changed", func(t *testing.T) {
		assert.False(t, VerifyActivateCode("secret", 10, activateCode, 1, "alice@example.com", "Alice", "123456", "new-rands"))
		assert.False(t, VerifyResetPasswordCode("secret", 10, resetPasswordCode, 1, "alice@example.com", "Alice", "654321", "rands"))
	})

	t.Run("malformed code", func(t *testing.T) {
		assert.False(t, VerifyActivateCode("secret", 10, "", 1, "alice@example.com", "Alice", "123456", "rands"))
		assert.False(t, VerifyActivateCode("secret", 10, activateCode[:TimeLimitCodeLength], 1, "alice@example.com", "Alice", "123456", "rands"))
	})
}

func TestCustomAvatarPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	got := CustomAvatarPath("data/avatars", 1)
	want := "data/avatars/1"
	assert.Equal(t, want, got)
}

func TestSaveAvatar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping testing on Windows")
		return
	}

	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	avatar := img.Bytes()

	avatarPath := CustomAvatarPath(os.TempDir(), 1)
	defer func() { _ = os.Remove(avatarPath) }()

	err := SaveAvatar(os.TempDir(), 1, avatar)
	require.NoError(t, err)
	got := osx.IsFile(avatarPath)
	assert.True(t, got)
}

func TestSaveGeneratedAvatar(t *testing.T) {
	appDataPath := t.TempDir()
	require.NoError(t, SaveGeneratedAvatar(appDataPath, 1, avatar.Person))
	data, err := os.ReadFile(GeneratedAvatarPath(appDataPath, 1, avatar.Person))
	require.NoError(t, err)
	_, err = png.Decode(bytes.NewReader(data))
	assert.NoError(t, err)

	entries, err := os.ReadDir(filepath.Dir(GeneratedAvatarPath(appDataPath, 1, avatar.Person)))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "temporary files are removed")
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
