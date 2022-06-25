// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/unknwon/com"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptoutil"
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
		return false, fmt.Errorf("DecodeString: %v", err)
	}
	decryptSecret, err := com.AESGCMDecrypt(cryptoutil.MD5Bytes(conf.Security.SecretKey), secret)
	if err != nil {
		return false, fmt.Errorf("AESGCMDecrypt: %v", err)
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
		return fmt.Errorf("delete two-factor: %v", err)
	} else if err = deleteRecoveryCodesByUserID(sess, userID); err != nil {
		return fmt.Errorf("deleteRecoveryCodesByUserID: %v", err)
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
		return fmt.Errorf("generateRecoveryCodes: %v", err)
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteRecoveryCodesByUserID(sess, userID); err != nil {
		return fmt.Errorf("deleteRecoveryCodesByUserID: %v", err)
	} else if _, err = sess.Insert(recoveryCodes); err != nil {
		return fmt.Errorf("insert new recovery codes: %v", err)
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

// UseRecoveryCode validates recovery code of given user and marks it is used if valid.
func UseRecoveryCode(_ int64, code string) error {
	recoveryCode := new(TwoFactorRecoveryCode)
	has, err := x.Where("code = ?", code).And("is_used = ?", false).Get(recoveryCode)
	if err != nil {
		return fmt.Errorf("get unused code: %v", err)
	} else if !has {
		return ErrTwoFactorRecoveryCodeNotFound{Code: code}
	}

	recoveryCode.IsUsed = true
	if _, err = x.Id(recoveryCode.ID).Cols("is_used").Update(recoveryCode); err != nil {
		return fmt.Errorf("mark code as used: %v", err)
	}

	return nil
}
