package database

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osx"
)

// PublicKeysStore is the storage layer for public keys.
type PublicKeysStore struct {
	db *gorm.DB
}

func newPublicKeysStore(db *gorm.DB) *PublicKeysStore {
	return &PublicKeysStore{db: db}
}

// Add adds a new public key of given user to the database and the
// "authorized_keys" file. The content must already be checked by
// CheckPublicKeyString. It returns ErrKeyAlreadyExist when the key is used by a
// user key, whatever its comment, and ErrKeyNameAlreadyUsed when the user has a key of
// the same name.
func (s *PublicKeysStore) Add(ctx context.Context, logger *logx.Logger, authorizedKeys AuthorizedKeysOptions, ownerID int64, name, content string) (*PublicKey, error) {
	fingerprint, err := publicKeyFingerprint(content)
	if err != nil {
		return nil, err
	}
	db := s.db.WithContext(ctx)
	if err = checkKeyContent(db, fingerprint, content); err != nil {
		return nil, err
	}

	var count int64
	err = db.Model(new(PublicKey)).Where("owner_id = ? AND name = ?", ownerID, name).Count(&count).Error
	if err != nil {
		return nil, errors.Wrap(err, "count keys by name")
	} else if count > 0 {
		return nil, ErrKeyNameAlreadyUsed{ownerID, name}
	}

	key := &PublicKey{
		OwnerID:     ownerID,
		Name:        name,
		Fingerprint: fingerprint,
		Content:     content,
		Mode:        AccessModeWrite,
		Type:        KeyTypeUser,
	}
	if err = db.Create(key).Error; err != nil {
		// The user added the same key at the same time.
		if isUniqueViolation(err) {
			return nil, ErrKeyAlreadyExist{ownerID, content}
		}
		return nil, errors.Wrap(err, "add key")
	}
	// The key is already added, and resyncing SSH keys adds its line.
	if err = appendAuthorizedKeysToFile(logger, authorizedKeys, key); err != nil {
		logger.Error("Failed to append public key to authorized_keys", "keyID", key.ID, "error", err)
	}
	return key, nil
}

// GetByID returns the public key with given ID. It returns ErrKeyNotExist when
// not found.
func (s *PublicKeysStore) GetByID(ctx context.Context, id int64) (*PublicKey, error) {
	key := new(PublicKey)
	err := s.db.WithContext(ctx).Where("id = ?", id).First(key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKeyNotExist{id}
		}
		return nil, err
	}
	return key, nil
}

// GetByContent returns the public key whose content starts with given
// content, so that the comment of the stored key is ignored. It returns
// ErrKeyNotExist when not found.
func (s *PublicKeysStore) GetByContent(ctx context.Context, content string) (*PublicKey, error) {
	key := new(PublicKey)
	err := s.db.WithContext(ctx).Where("content LIKE ?", content+"%").First(key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrKeyNotExist{}
		}
		return nil, err
	}
	return key, nil
}

// List returns all public keys of given user.
func (s *PublicKeysStore) List(ctx context.Context, ownerID int64) ([]*PublicKey, error) {
	keys := make([]*PublicKey, 0, 5)
	return keys, s.db.WithContext(ctx).Where("owner_id = ?", ownerID).Find(&keys).Error
}

// Touch marks the public key with given ID as used now.
func (s *PublicKeysStore) Touch(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).
		Model(new(PublicKey)).
		Where("id = ?", id).
		Update("updated_unix", s.db.NowFunc().Unix()).
		Error
}

// Delete deletes the public key with given ID from the database and the
// "authorized_keys" file. Only the owner of the key and site admins can delete
// it, and others get ErrKeyAccessDenied. Deleting a key that does not exist is
// not an error.
func (s *PublicKeysStore) Delete(ctx context.Context, logger *logx.Logger, authorizedKeys AuthorizedKeysOptions, doer *User, id int64) error {
	key, err := s.GetByID(ctx, id)
	if err != nil {
		if IsErrKeyNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "get key")
	}

	if !doer.IsAdmin && doer.ID != key.OwnerID {
		return ErrKeyAccessDenied{doer.ID, key.ID, "public"}
	}

	if err = deletePublicKeys(s.db.WithContext(ctx), id); err != nil {
		return errors.Wrap(err, "delete key")
	}

	logger.Debug("Rewriting authorized_keys file")
	return s.RewriteAuthorizedKeys(authorizedKeys)
}

// AuthorizedKeysOptions are the settings that the "authorized_keys" file is
// written with.
type AuthorizedKeysOptions struct {
	// Path is the path of the file, or empty when the builtin SSH server is
	// used, which does not read it.
	Path string
	// CustomConf is the path of the custom configuration file that each line runs
	// "gogs serv" with.
	CustomConf string
}

// NewAuthorizedKeysOptions returns the settings of the "authorized_keys" file
// under the SSH root path, which has no path with the builtin SSH server.
func NewAuthorizedKeysOptions(sshOpts conf.SSHOpts, customConf string) AuthorizedKeysOptions {
	opts := AuthorizedKeysOptions{CustomConf: customConf}
	if !sshOpts.StartBuiltinServer {
		opts.Path = filepath.Join(sshOpts.RootPath, "authorized_keys")
	}
	return opts
}

// RewriteAuthorizedKeys rewrites the "authorized_keys" file with all public
// keys stored in the database. It leaves the file alone when it has no path,
// i.e., with the builtin SSH server, which does not read it.
func (s *PublicKeysStore) RewriteAuthorizedKeys(authorizedKeys AuthorizedKeysOptions) error {
	if authorizedKeys.Path == "" {
		return nil
	}

	sshOpLocker.Lock()
	defer sshOpLocker.Unlock()

	fpath := authorizedKeys.Path
	err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create SSH root path")
	}
	tempPath := fpath + ".tmp"
	f, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return errors.Wrap(err, "create temporary file")
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tempPath)
	}()

	// NOTE: More recently updated keys are more likely to be used more frequently,
	// putting them in the earlier lines could speed up the key lookup by SSHD.
	rows, err := s.db.Model(&PublicKey{}).Order("updated_unix DESC").Rows()
	if err != nil {
		return errors.Wrap(err, "iterate public keys")
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key PublicKey
		err = s.db.ScanRows(rows, &key)
		if err != nil {
			return errors.Wrap(err, "scan rows")
		}

		_, err = f.WriteString(key.AuthorizedString(authorizedKeys.CustomConf))
		if err != nil {
			return errors.Wrapf(err, "write key %d", key.ID)
		}
	}
	if err = rows.Err(); err != nil {
		return errors.Wrap(err, "check rows.Err")
	}

	err = f.Close()
	if err != nil {
		return errors.Wrap(err, "close temporary file")
	}
	if osx.Exist(fpath) {
		err = os.Remove(fpath)
		if err != nil {
			return errors.Wrap(err, "remove")
		}
	}
	err = os.Rename(tempPath, fpath)
	if err != nil {
		return errors.Wrap(err, "rename")
	}
	return nil
}
