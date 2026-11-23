package database

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/pquerna/otp/totp"

	"gogs.io/gogs/internal/cryptox"
)

// TwoFactor is a 2FA token of a user.
type TwoFactor struct {
	ID          int64     `gorm:"primaryKey"`
	UserID      int64     `gorm:"uniqueIndex:UQE_two_factor_user_id"`
	Secret      string    `gorm:"size:255"`
	Created     time.Time `gorm:"-" json:"-"`
	CreatedUnix int64
}

// ValidateTOTP returns true if given passcode is valid for two-factor authentication token.
// It also returns possible validation error. The token is encrypted with the
// secret key.
func (t *TwoFactor) ValidateTOTP(secretKey, passcode string) (bool, error) {
	secret, err := base64.StdEncoding.DecodeString(t.Secret)
	if err != nil {
		return false, errors.Newf("DecodeString: %v", err)
	}
	decryptSecret, err := cryptox.AESGCMDecrypt(cryptox.MD5Bytes(secretKey), secret)
	if err != nil {
		return false, errors.Newf("AESGCMDecrypt: %v", err)
	}
	return totp.Validate(passcode, string(decryptSecret)), nil
}

// TwoFactorRecoveryCode represents a two-factor authentication recovery code.
type TwoFactorRecoveryCode struct {
	ID     int64
	UserID int64
	Code   string `gorm:"size:11"`
	IsUsed bool
}

type ErrTwoFactorRecoveryCodeNotFound struct {
	Code string
}

func IsTwoFactorRecoveryCodeNotFound(err error) bool {
	_, ok := err.(ErrTwoFactorRecoveryCodeNotFound)
	return ok
}

func (err ErrTwoFactorRecoveryCodeNotFound) Error() string {
	return fmt.Sprintf("two-factor recovery code does not found [code: %s]", err.Code)
}
