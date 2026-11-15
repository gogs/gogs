package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/errx"
)

// LoginSession is a browser a user signed in on. The browser keeps the token
// of its login session in its web session, and stays signed in only while the
// login session exists, so deleting it signs the browser out.
type LoginSession struct {
	ID     int64 `gorm:"primarykey"`
	UserID int64 `gorm:"index;not null"`
	// TokenHash is the SHA-256 of the token, which only the web session holds.
	TokenHash string `gorm:"type:VARCHAR(64);unique;not null"`
	UserAgent string `gorm:"type:VARCHAR(512)"`
	IP        string `gorm:"type:VARCHAR(64)"`

	Created      time.Time `gorm:"-" json:"-"`
	CreatedUnix  int64
	LastSeen     time.Time `gorm:"-" json:"-"`
	LastSeenUnix int64
}

// BeforeCreate implements the GORM create hook.
func (s *LoginSession) BeforeCreate(tx *gorm.DB) error {
	if s.CreatedUnix == 0 {
		s.CreatedUnix = tx.NowFunc().Unix()
	}
	if s.LastSeenUnix == 0 {
		s.LastSeenUnix = s.CreatedUnix
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (s *LoginSession) AfterFind(_ *gorm.DB) error {
	s.Created = time.Unix(s.CreatedUnix, 0).Local()
	s.LastSeen = time.Unix(s.LastSeenUnix, 0).Local()
	return nil
}

// loginSessionTouchInterval is how often the last time a login session was
// seen is saved, so browsing does not write on every request.
const loginSessionTouchInterval = time.Minute

// LoginSessionsStore is the storage layer for login sessions.
type LoginSessionsStore struct {
	db *gorm.DB
}

func newLoginSessionsStore(db *gorm.DB) *LoginSessionsStore {
	return &LoginSessionsStore{db: db}
}

// Create starts a login session of the user and returns it with its token,
// which is not stored and cannot be read back.
func (s *LoginSessionsStore) Create(ctx context.Context, userID int64, userAgent, ip string) (_ *LoginSession, token string, _ error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, "", errors.Wrap(err, "generate token")
	}
	token = hex.EncodeToString(b)
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	if len(ip) > 64 {
		ip = ip[:64]
	}
	ls := &LoginSession{
		UserID:    userID,
		TokenHash: cryptox.SHA256(token),
		UserAgent: userAgent,
		IP:        ip,
	}
	return ls, token, s.db.WithContext(ctx).Create(ls).Error
}

var _ errx.NotFound = (*ErrLoginSessionNotExist)(nil)

type ErrLoginSessionNotExist struct {
	args errx.Args
}

// IsErrLoginSessionNotExist returns true if the underlying error has the type
// ErrLoginSessionNotExist.
func IsErrLoginSessionNotExist(err error) bool {
	return errors.As(err, &ErrLoginSessionNotExist{})
}

func (err ErrLoginSessionNotExist) Error() string {
	return fmt.Sprintf("login session does not exist: %v", err.args)
}

func (ErrLoginSessionNotExist) NotFound() bool {
	return true
}

// GetByToken returns the login session of the user with the given token. It
// returns ErrLoginSessionNotExist when the user has no such login session, e.g.,
// because it was revoked.
func (s *LoginSessionsStore) GetByToken(ctx context.Context, userID int64, token string) (*LoginSession, error) {
	if token == "" {
		return nil, ErrLoginSessionNotExist{args: errx.Args{"userID": userID}}
	}
	ls := new(LoginSession)
	err := s.db.WithContext(ctx).Where("user_id = ? AND token_hash = ?", userID, cryptox.SHA256(token)).First(ls).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrLoginSessionNotExist{args: errx.Args{"userID": userID}}
	} else if err != nil {
		return nil, err
	}
	return ls, nil
}

// Touch records that the login session was seen now, at most once every
// loginSessionTouchInterval.
func (s *LoginSessionsStore) Touch(ctx context.Context, ls *LoginSession) error {
	now := s.db.NowFunc()
	if now.Sub(time.Unix(ls.LastSeenUnix, 0)) < loginSessionTouchInterval {
		return nil
	}
	return s.db.WithContext(ctx).Model(ls).UpdateColumn("last_seen_unix", now.Unix()).Error
}

// List returns the login sessions of the user, the most recently seen first.
func (s *LoginSessionsStore) List(ctx context.Context, userID int64) ([]*LoginSession, error) {
	var sessions []*LoginSession
	return sessions, s.db.WithContext(ctx).Where("user_id = ?", userID).Order("last_seen_unix DESC, id DESC").Find(&sessions).Error
}

// DeleteByID signs the browser of the login session out.
//
// 🚨 SECURITY: The "userID" is required to prevent attacker deletes arbitrary
// login session that belongs to another user.
func (s *LoginSessionsStore) DeleteByID(ctx context.Context, userID, id int64) error {
	return s.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(new(LoginSession)).Error
}

// DeleteAll signs every browser of the user out, except the login session with
// the ID keepID when it is not zero.
func (s *LoginSessionsStore) DeleteAll(ctx context.Context, userID, keepID int64) error {
	return s.db.WithContext(ctx).Where("user_id = ? AND id != ?", userID, keepID).Delete(new(LoginSession)).Error
}
