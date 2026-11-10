package userx

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/image/draw"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/strx"
	"gogs.io/gogs/internal/tool"
)

// DashboardURLPath returns the URL path to the user or organization dashboard.
func DashboardURLPath(name string, isOrganization bool) string {
	if isOrganization {
		return conf.Server.Subpath + "/org/" + name + "/dashboard/"
	}
	return conf.Server.Subpath + "/"
}

// codePurpose is signed into every time-limited user code so that a code issued
// for one purpose cannot be redeemed for another.
type codePurpose string

const (
	codePurposeActivate      codePurpose = "activate"
	codePurposeActivateEmail codePurpose = "activate_email"
	codePurposeResetPassword codePurpose = "reset_password"
)

// GenerateActivateCode generates an activate code based on user information and
// the given email.
func GenerateActivateCode(userID int64, email, name, password, rands string) string {
	return generateTimeLimitedUserCode(codePurposeActivate, userID, email, name, password, rands, conf.Auth.ActivateCodeLives)
}

// VerifyActivateCode returns true if the code is an unexpired activate code for
// the given user information and email.
func VerifyActivateCode(code string, userID int64, email, name, password, rands string) bool {
	return verifyTimeLimitedUserCode(codePurposeActivate, code, userID, email, name, password, rands, conf.Auth.ActivateCodeLives)
}

// GenerateActivateEmailCode generates a code to confirm the given email address
// based on user information.
func GenerateActivateEmailCode(userID int64, email, name, password, rands string) string {
	return generateTimeLimitedUserCode(codePurposeActivateEmail, userID, email, name, password, rands, conf.Auth.ActivateCodeLives)
}

// VerifyActivateEmailCode returns true if the code is an unexpired code to
// confirm the given email address for the given user information.
func VerifyActivateEmailCode(code string, userID int64, email, name, password, rands string) bool {
	return verifyTimeLimitedUserCode(codePurposeActivateEmail, code, userID, email, name, password, rands, conf.Auth.ActivateCodeLives)
}

// GenerateResetPasswordCode generates a password-reset code based on user
// information and the given email. The token's lifetime is bound to
// [conf.AuthOpts.ResetPasswordCodeLives].
func GenerateResetPasswordCode(userID int64, email, name, password, rands string) string {
	return generateTimeLimitedUserCode(codePurposeResetPassword, userID, email, name, password, rands, conf.Auth.ResetPasswordCodeLives)
}

// VerifyResetPasswordCode returns true if the code is an unexpired
// password-reset code for the given user information and email.
func VerifyResetPasswordCode(code string, userID int64, email, name, password, rands string) bool {
	return verifyTimeLimitedUserCode(codePurposeResetPassword, code, userID, email, name, password, rands, conf.Auth.ResetPasswordCodeLives)
}

func timeLimitedUserCodeData(purpose codePurpose, userID int64, email, name, password, rands string) string {
	return fmt.Sprintf("%s:%d%s%s%s%s", purpose, userID, email, strings.ToLower(name), password, rands)
}

func generateTimeLimitedUserCode(purpose codePurpose, userID int64, email, name, password, rands string, minutes int) string {
	code := tool.CreateTimeLimitCode(
		timeLimitedUserCodeData(purpose, userID, email, name, password, rands),
		minutes,
		nil,
	)

	// Add tailing hex username
	code += hex.EncodeToString([]byte(strings.ToLower(name)))
	return code
}

func verifyTimeLimitedUserCode(purpose codePurpose, code string, userID int64, email, name, password, rands string, minutes int) bool {
	if len(code) <= tool.TimeLimitCodeLength {
		return false
	}
	return tool.VerifyTimeLimitCode(
		timeLimitedUserCodeData(purpose, userID, email, name, password, rands),
		minutes,
		code[:tool.TimeLimitCodeLength],
	)
}

// CustomAvatarPath returns the absolute path of the user custom avatar file.
func CustomAvatarPath(userID int64) string {
	return filepath.Join(conf.Picture.AvatarUploadPath, strconv.FormatInt(userID, 10))
}

// GenerateRandomAvatar generates a random avatar and stores to local file
// system using given user information.
func GenerateRandomAvatar(userID int64, name, email string) error {
	seed := email
	if seed == "" {
		seed = name
	}

	img, err := avatar.RandomImage([]byte(seed))
	if err != nil {
		return errors.Wrap(err, "generate random image")
	}

	avatarPath := CustomAvatarPath(userID)
	err = os.MkdirAll(filepath.Dir(avatarPath), os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create avatar directory")
	}

	f, err := os.Create(avatarPath)
	if err != nil {
		return errors.Wrap(err, "create avatar file")
	}
	defer func() { _ = f.Close() }()

	if err = png.Encode(f, img); err != nil {
		return errors.Wrap(err, "encode avatar image to file")
	}
	return nil
}

// SaveAvatar saves the given avatar for the user.
func SaveAvatar(userID int64, data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "decode image")
	}

	avatarPath := CustomAvatarPath(userID)
	err = os.MkdirAll(filepath.Dir(avatarPath), os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create avatar directory")
	}

	f, err := os.Create(avatarPath)
	if err != nil {
		return errors.Wrap(err, "create avatar file")
	}
	defer func() { _ = f.Close() }()

	dst := image.NewRGBA(image.Rect(0, 0, avatar.DefaultSize, avatar.DefaultSize))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	if err = png.Encode(f, dst); err != nil {
		return errors.Wrap(err, "encode avatar image to file")
	}
	return nil
}

// EncodePassword encodes password using PBKDF2 SHA256 with given salt.
func EncodePassword(password, salt string) string {
	newPasswd := pbkdf2.Key([]byte(password), []byte(salt), 10000, 50, sha256.New)
	return fmt.Sprintf("%x", newPasswd)
}

// ValidatePassword returns true if the given password matches the encoded
// version with given salt.
func ValidatePassword(encoded, salt, password string) bool {
	got := EncodePassword(password, salt)
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(got)) == 1
}

// MailResendCacheKey returns the key used for caching mail resend.
func MailResendCacheKey(userID int64) string {
	return fmt.Sprintf("mailResend::%d", userID)
}

// TwoFactorCacheKey returns the key used for caching two factor passcode.
func TwoFactorCacheKey(userID int64, passcode string) string {
	return fmt.Sprintf("twoFactor::%d::%s", userID, passcode)
}

// RandomSalt returns randomly generated 10-character string that can be used as
// the user salt.
func RandomSalt() (string, error) {
	return strx.RandomChars(10)
}
