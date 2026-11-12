package database

import (
	"context"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

// Passkey is a WebAuthn credential that a user signs in with instead of a
// password and a second factor.
type Passkey struct {
	ID         int64               `gorm:"primarykey"`
	UserID     int64               `gorm:"index;not null"`
	Name       string              `gorm:"not null"`
	Credential webauthn.Credential `gorm:"type:TEXT;serializer:json;not null"`

	Created     time.Time `gorm:"-" json:"-"`
	CreatedUnix int64
	// UpdatedUnix is when the passkey was last used to sign in, and zero when
	// it was never used.
	Updated           time.Time `gorm:"-" json:"-"`
	UpdatedUnix       int64
	HasRecentActivity bool `gorm:"-" json:"-"`
	HasUsed           bool `gorm:"-" json:"-"`
}

// BeforeCreate implements the GORM create hook.
func (p *Passkey) BeforeCreate(tx *gorm.DB) error {
	if p.CreatedUnix == 0 {
		p.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (p *Passkey) AfterFind(tx *gorm.DB) error {
	p.Created = time.Unix(p.CreatedUnix, 0).Local()
	if p.UpdatedUnix > 0 {
		p.Updated = time.Unix(p.UpdatedUnix, 0).Local()
		p.HasUsed = true
		p.HasRecentActivity = p.Updated.Add(7 * 24 * time.Hour).After(tx.NowFunc())
	}
	return nil
}

// PasskeysStore is the storage layer for passkeys.
type PasskeysStore struct {
	db *gorm.DB
}

func newPasskeysStore(db *gorm.DB) *PasskeysStore {
	return &PasskeysStore{db: db}
}

// Create adds a passkey with the given name and credential to the user.
func (s *PasskeysStore) Create(ctx context.Context, userID int64, name string, credential webauthn.Credential) (*Passkey, error) {
	p := &Passkey{
		UserID:     userID,
		Name:       name,
		Credential: credential,
	}
	return p, s.db.WithContext(ctx).Create(p).Error
}

// List returns all passkeys of the user, oldest first.
func (s *PasskeysStore) List(ctx context.Context, userID int64) ([]*Passkey, error) {
	var passkeys []*Passkey
	return passkeys, s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id ASC").Find(&passkeys).Error
}

// DeleteByID deletes the passkey with the given ID.
//
// 🚨 SECURITY: The "userID" is required to prevent attacker deletes arbitrary
// passkey that belongs to another user.
func (s *PasskeysStore) DeleteByID(ctx context.Context, userID, id int64) error {
	return s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(new(Passkey)).Error
}

// MarkUsed saves the credential of the passkey as it was after signing in,
// which carries the new signature counter, and records the current time as
// when it was last used.
func (s *PasskeysStore) MarkUsed(ctx context.Context, id int64, credential webauthn.Credential) error {
	return s.db.WithContext(ctx).
		Model(&Passkey{ID: id}).
		Select("credential", "updated_unix").
		Updates(&Passkey{Credential: credential, UpdatedUnix: s.db.NowFunc().Unix()}).
		Error
}
