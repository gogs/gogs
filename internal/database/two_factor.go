package database

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/pquerna/otp/totp"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
)

// TwoFactor is a 2FA token of a user.
type TwoFactor struct {
	ID          int64 `gorm:"primaryKey"`
	UserID      int64 `xorm:"UNIQUE" gorm:"unique"`
	Secret      string
	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
}

// ValidateTOTP returns true if given passcode is valid for two-factor authentication token.
// It also returns possible validation error.
func (t *TwoFactor) ValidateTOTP(passcode string) (bool, error) {
	secret, err := base64.StdEncoding.DecodeString(t.Secret)
	if err != nil {
		return false, errors.Newf("DecodeString: %v", err)
	}
	decryptSecret, err := cryptox.AESGCMDecrypt(cryptox.MD5Bytes(conf.Security.SecretKey), secret)
	if err != nil {
		return false, errors.Newf("AESGCMDecrypt: %v", err)
	}
	return totp.Validate(passcode, string(decryptSecret)), nil
}

// DeleteTwoFactor removes two-factor authentication token and recovery codes of given user.
func DeleteTwoFactor(userID int64) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Where("user_id = ?", userID).Delete(new(TwoFactor)); err != nil {
		return errors.Newf("delete two-factor: %v", err)
	} else if err = deleteRecoveryCodesByUserID(sess, userID); err != nil {
		return errors.Newf("deleteRecoveryCodesByUserID: %v", err)
	}

	return sess.Commit()
}

// TwoFactorRecoveryCode represents a two-factor authentication recovery code.
type TwoFactorRecoveryCode struct {
	ID     int64
	UserID int64
	Code   string `xorm:"VARCHAR(11)"`
	IsUsed bool
}

// GetRecoveryCodesByUserID returns all recovery codes of given user.
func GetRecoveryCodesByUserID(userID int64) ([]*TwoFactorRecoveryCode, error) {
	recoveryCodes := make([]*TwoFactorRecoveryCode, 0, 10)
	return recoveryCodes, x.Where("user_id = ?", userID).Find(&recoveryCodes)
}

func deleteRecoveryCodesByUserID(e Engine, userID int64) error {
	_, err := e.Where("user_id = ?", userID).Delete(new(TwoFactorRecoveryCode))
	return err
}

// RegenerateRecoveryCodes regenerates new set of recovery codes for given user.
func RegenerateRecoveryCodes(userID int64) error {
	recoveryCodes, err := generateRecoveryCodes(userID, 10)
	if err != nil {
		return errors.Newf("generateRecoveryCodes: %v", err)
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteRecoveryCodesByUserID(sess, userID); err != nil {
		return errors.Newf("deleteRecoveryCodesByUserID: %v", err)
	} else if _, err = sess.Insert(recoveryCodes); err != nil {
		return errors.Newf("insert new recovery codes: %v", err)
	}

	return sess.Commit()
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
