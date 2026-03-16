package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
)

// Passkey is a registered WebAuthn credential for a user.
type Passkey struct {
	ID           int64 `gorm:"primaryKey"`
	UserID       int64 `gorm:"index;not null"`
	Name         string
	CredentialID string `gorm:"type:VARCHAR(512);unique;not null"`
	Credential   string `gorm:"type:TEXT;not null"`

	Created      time.Time `gorm:"-" json:"-"`
	CreatedUnix  int64
	Updated      time.Time `gorm:"-" json:"-"`
	UpdatedUnix  int64
	LastUsed     time.Time `gorm:"-" json:"-"`
	LastUsedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (p *Passkey) BeforeCreate(tx *gorm.DB) error {
	now := tx.NowFunc().Unix()
	if p.CreatedUnix == 0 {
		p.CreatedUnix = now
	}
	if p.UpdatedUnix == 0 {
		p.UpdatedUnix = now
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (p *Passkey) AfterFind(_ *gorm.DB) error {
	p.Created = time.Unix(p.CreatedUnix, 0).Local()
	p.Updated = time.Unix(p.UpdatedUnix, 0).Local()
	if p.LastUsedUnix > 0 {
		p.LastUsed = time.Unix(p.LastUsedUnix, 0).Local()
	}
	return nil
}

// CredentialStruct decodes the stored WebAuthn credential.
func (p *Passkey) CredentialStruct() (webauthn.Credential, error) {
	var credential webauthn.Credential
	err := json.Unmarshal([]byte(p.Credential), &credential)
	if err != nil {
		return webauthn.Credential{}, errors.Wrap(err, "unmarshal credential")
	}
	return credential, nil
}

// PasskeysStore is the storage layer for user passkeys.
type PasskeysStore struct {
	db *gorm.DB
}

// newPasskeysStore creates a passkey store backed by the given GORM handle.
func newPasskeysStore(db *gorm.DB) *PasskeysStore {
	return &PasskeysStore{db: db}
}

// ErrPasskeyAlreadyExist indicates the credential ID has already been
// registered by another passkey.
type ErrPasskeyAlreadyExist struct {
	args errx.Args
}

// IsErrPasskeyAlreadyExist returns true if the error indicates an existing
// passkey credential with the same credential ID.
func IsErrPasskeyAlreadyExist(err error) bool {
	return errors.As(err, &ErrPasskeyAlreadyExist{})
}

// Error implements the error interface.
func (err ErrPasskeyAlreadyExist) Error() string {
	return fmt.Sprintf("passkey already exists: %v", err.args)
}

var _ errx.NotFound = (*ErrPasskeyNotFound)(nil)

// ErrPasskeyNotFound indicates the target passkey does not exist for the
// requested user.
type ErrPasskeyNotFound struct {
	args errx.Args
}

// IsErrPasskeyNotFound returns true if the error indicates a missing passkey.
func IsErrPasskeyNotFound(err error) bool {
	return errors.As(err, &ErrPasskeyNotFound{})
}

// Error implements the error interface.
func (err ErrPasskeyNotFound) Error() string {
	return fmt.Sprintf("passkey does not exist: %v", err.args)
}

// NotFound marks this error as a not-found error for shared helpers.
func (ErrPasskeyNotFound) NotFound() bool {
	return true
}

// Create stores a passkey credential for the given user.
func (s *PasskeysStore) Create(ctx context.Context, userID int64, name string, credential webauthn.Credential) (*Passkey, error) {
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	err := s.db.WithContext(ctx).Where("credential_id = ?", credentialID).First(new(Passkey)).Error
	if err == nil {
		return nil, ErrPasskeyAlreadyExist{args: errx.Args{"credentialID": credentialID}}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrap(err, "check duplicate credential")
	}

	rawCredential, err := json.Marshal(credential)
	if err != nil {
		return nil, errors.Wrap(err, "marshal credential")
	}

	passkey := &Passkey{
		UserID:       userID,
		Name:         name,
		CredentialID: credentialID,
		Credential:   string(rawCredential),
	}
	err = s.db.WithContext(ctx).Create(passkey).Error
	if err != nil {
		return nil, errors.Wrap(err, "create passkey")
	}
	return passkey, nil
}

// ListByUserID returns all passkeys belongs to the user.
func (s *PasskeysStore) ListByUserID(ctx context.Context, userID int64) ([]*Passkey, error) {
	var passkeys []*Passkey
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id ASC").Find(&passkeys).Error
	return passkeys, err
}

// DeleteByID deletes a passkey by ID.
//
// 🚨 SECURITY: The "userID" is required to prevent deletion of passkeys
// from other users.
func (s *PasskeysStore) DeleteByID(ctx context.Context, userID, passkeyID int64) error {
	tx := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", passkeyID, userID).Delete(new(Passkey))
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "delete passkey")
	}
	if tx.RowsAffected == 0 {
		return ErrPasskeyNotFound{args: errx.Args{"userID": userID, "passkeyID": passkeyID}}
	}
	return nil
}

// UpdateCredential updates a passkey credential after successful assertion.
func (s *PasskeysStore) UpdateCredential(ctx context.Context, userID, passkeyID int64, credential webauthn.Credential) error {
	rawCredential, err := json.Marshal(credential)
	if err != nil {
		return errors.Wrap(err, "marshal credential")
	}

	now := s.db.NowFunc().Unix()
	tx := s.db.WithContext(ctx).
		Model(new(Passkey)).
		Where("id = ? AND user_id = ?", passkeyID, userID).
		Updates(map[string]any{
			"credential":     string(rawCredential),
			"updated_unix":   now,
			"last_used_unix": now,
		})
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "update credential")
	}
	if tx.RowsAffected == 0 {
		return ErrPasskeyNotFound{args: errx.Args{"userID": userID, "passkeyID": passkeyID}}
	}
	return nil
}
