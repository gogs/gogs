package database

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/strx"
)

// BeforeCreate implements the GORM create hook.
func (t *TwoFactor) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedUnix == 0 {
		t.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (t *TwoFactor) AfterFind(_ *gorm.DB) error {
	t.Created = time.Unix(t.CreatedUnix, 0).Local()
	return nil
}

// TwoFactorsStore is the storage layer for two-factor authentication settings.
type TwoFactorsStore struct {
	db *gorm.DB
}

func newTwoFactorsStore(db *gorm.DB) *TwoFactorsStore {
	return &TwoFactorsStore{db: db}
}

// Create creates a new 2FA token and recovery codes for given user. The "key"
// is used to encrypt and later decrypt given "secret", which should be
// configured in site-level and change of the "key" will break all existing 2FA
// tokens.
func (s *TwoFactorsStore) Create(ctx context.Context, userID int64, key, secret string) error {
	encrypted, err := cryptox.AESGCMEncrypt(cryptox.MD5Bytes(key), []byte(secret))
	if err != nil {
		return errors.Wrap(err, "encrypt secret")
	}
	tf := &TwoFactor{
		UserID: userID,
		Secret: base64.StdEncoding.EncodeToString(encrypted),
	}

	recoveryCodes, err := generateRecoveryCodes(userID, 10)
	if err != nil {
		return errors.Wrap(err, "generate recovery codes")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Create(tf).Error
		if err != nil {
			return err
		}

		return tx.Create(&recoveryCodes).Error
	})
}

var _ errx.NotFound = (*ErrTwoFactorNotFound)(nil)

type ErrTwoFactorNotFound struct {
	args errx.Args
}

func IsErrTwoFactorNotFound(err error) bool {
	return errors.As(err, &ErrTwoFactorNotFound{})
}

func (err ErrTwoFactorNotFound) Error() string {
	return fmt.Sprintf("2FA does not found: %v", err.args)
}

func (ErrTwoFactorNotFound) NotFound() bool {
	return true
}

// GetByUserID returns the 2FA token of given user. It returns
// ErrTwoFactorNotFound when not found.
func (s *TwoFactorsStore) GetByUserID(ctx context.Context, userID int64) (*TwoFactor, error) {
	tf := new(TwoFactor)
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(tf).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTwoFactorNotFound{args: errx.Args{"userID": userID}}
		}
		return nil, err
	}
	return tf, nil
}

// IsEnabled returns true if the user has enabled 2FA.
func (s *TwoFactorsStore) IsEnabled(ctx context.Context, userID int64) bool {
	var count int64
	err := s.db.WithContext(ctx).Model(new(TwoFactor)).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		log.Error("Failed to count two factors [user_id: %d]: %v", userID, err)
	}
	return count > 0
}

// UseRecoveryCode validates a recovery code of given user and marks it as used
// if valid. It returns ErrTwoFactorRecoveryCodeNotFound if the code is invalid
// or already used.
func (s *TwoFactorsStore) UseRecoveryCode(ctx context.Context, userID int64, code string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var recoveryCode TwoFactorRecoveryCode
		err := tx.Where("user_id = ? AND code = ? AND is_used = ?", userID, code, false).First(&recoveryCode).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrTwoFactorRecoveryCodeNotFound{Code: code}
			}
			return errors.Wrap(err, "get unused recovery code")
		}

		err = tx.Model(&recoveryCode).Update("is_used", true).Error
		if err != nil {
			return errors.Wrap(err, "mark recovery code as used")
		}
		return nil
	})
}

// generateRecoveryCodes generates N number of recovery codes for 2FA.
func generateRecoveryCodes(userID int64, n int) ([]*TwoFactorRecoveryCode, error) {
	recoveryCodes := make([]*TwoFactorRecoveryCode, n)
	for i := range n {
		code, err := strx.RandomChars(10)
		if err != nil {
			return nil, errors.Wrap(err, "generate random characters")
		}
		recoveryCodes[i] = &TwoFactorRecoveryCode{
			UserID: userID,
			Code:   strings.ToLower(code[:5] + "-" + code[5:]),
		}
	}
	return recoveryCodes, nil
}
