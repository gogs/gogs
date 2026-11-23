package database

import (
	"context"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"unknwon.dev/x/logx"
)

// DeployKeysStore is the storage layer for deploy keys.
type DeployKeysStore struct {
	db *gorm.DB
}

func newDeployKeysStore(db *gorm.DB) *DeployKeysStore {
	return &DeployKeysStore{db: db}
}

// Add gives the public key of given content read access to the repository as a
// deploy key. The content must already be checked by CheckPublicKeyString.
// Repositories that use the same key share one public key, whatever the
// comments of their contents. It returns
// ErrKeyAlreadyExist when the content is used by a user key,
// ErrDeployKeyAlreadyExist when the repository already has the key, and
// ErrDeployKeyNameAlreadyUsed when the repository has a key of the same name.
func (s *DeployKeysStore) Add(ctx context.Context, logger *logx.Logger, authorizedKeys AuthorizedKeysOptions, repoID int64, name, content string) (*DeployKey, error) {
	fingerprint, err := publicKeyFingerprint(content)
	if err != nil {
		return nil, err
	}

	// Public keys of deploy keys, which have no owner, are unique by their
	// fingerprints. Another repository adding
	// the same key at the same time makes the first attempt violate that, and the
	// second attempt then finds the public key the other one added.
	for attempt := 1; ; attempt++ {
		key, added, err := s.add(ctx, repoID, name, content, fingerprint)
		if err != nil {
			if attempt == 1 && isUniqueViolation(err) {
				continue
			}
			return nil, err
		}

		if added != nil {
			// The key is already added, and resyncing SSH keys adds its line.
			if err = appendAuthorizedKeysToFile(logger, authorizedKeys, added); err != nil {
				logger.Error("Failed to append deploy key to authorized_keys", "keyID", added.ID, "error", err)
			}
		}
		key.Content = content
		return key, nil
	}
}

// add adds the deploy key in one transaction, and returns the public key it
// added, if any.
func (s *DeployKeysStore) add(ctx context.Context, repoID int64, name, content, fingerprint string) (key *DeployKey, added *PublicKey, _ error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkKeyContent(tx, fingerprint, content); err != nil {
			return err
		}

		pkey := new(PublicKey)
		err := tx.Where("type = ? AND fingerprint = ?", KeyTypeDeploy, fingerprint).First(pkey).Error
		if err == nil {
			// Locked like lockPublicKeys does, so that deleting the last deploy key of
			// the public key waits for this one to be added. The deletion may also
			// have happened since the public key was found.
			err = tx.Where("id = ?", pkey.ID).Clauses(clause.Locking{Strength: "UPDATE"}).First(pkey).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			pkey = &PublicKey{
				Fingerprint: fingerprint,
				Content:     content,
				Mode:        AccessModeRead,
				Type:        KeyTypeDeploy,
			}
			if err = tx.Create(pkey).Error; err != nil {
				return errors.Wrap(err, "add key")
			}
			added = pkey
		} else if err != nil {
			return errors.Wrap(err, "get existing key")
		}

		key, err = addDeployKey(tx, pkey.ID, repoID, name, pkey.Fingerprint)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return key, added, nil
}

// isUniqueViolation returns true when the error is an insert that violates a
// unique index.
func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// SQLITE_CONSTRAINT_PRIMARYKEY and SQLITE_CONSTRAINT_UNIQUE.
		return sqliteErr.Code() == 1555 || sqliteErr.Code() == 2067
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// unique_violation.
		return pgErr.Code == "23505"
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		// ER_DUP_ENTRY.
		return mysqlErr.Number == 1062
	}
	return false
}

// loadContents sets the content of given deploy keys from their public keys.
// Keys whose public key is gone, e.g., deleted by a site admin, keep an empty
// content, so that they are still listed and can be deleted.
func (s *DeployKeysStore) loadContents(ctx context.Context, keys ...*DeployKey) error {
	if len(keys) == 0 {
		return nil
	}

	keyIDs := make([]int64, 0, len(keys))
	for _, key := range keys {
		keyIDs = append(keyIDs, key.KeyID)
	}
	var pkeys []*PublicKey
	err := s.db.WithContext(ctx).Where("id IN ?", keyIDs).Find(&pkeys).Error
	if err != nil {
		return errors.Wrap(err, "list public keys")
	}
	contents := make(map[int64]string, len(pkeys))
	for _, pkey := range pkeys {
		contents[pkey.ID] = pkey.Content
	}

	for _, key := range keys {
		key.Content = contents[key.KeyID]
	}
	return nil
}

func (s *DeployKeysStore) get(ctx context.Context, notFound error, query any, args ...any) (*DeployKey, error) {
	key := new(DeployKey)
	err := s.db.WithContext(ctx).Where(query, args...).First(key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, notFound
		}
		return nil, err
	}
	return key, s.loadContents(ctx, key)
}

// GetByID returns the deploy key with given ID and its content. It returns
// ErrDeployKeyNotExist when not found.
func (s *DeployKeysStore) GetByID(ctx context.Context, id int64) (*DeployKey, error) {
	return s.get(ctx, ErrDeployKeyNotExist{args: map[string]any{"deployKeyID": id}}, "id = ?", id)
}

// GetByRepoAndKey returns the deploy key of given repository that uses given
// public key, with its content. It returns ErrDeployKeyNotExist when not found.
func (s *DeployKeysStore) GetByRepoAndKey(ctx context.Context, repoID, keyID int64) (*DeployKey, error) {
	return s.get(ctx, ErrDeployKeyNotExist{args: map[string]any{"keyID": keyID, "repoID": repoID}}, "key_id = ? AND repo_id = ?", keyID, repoID)
}

// List returns all deploy keys of given repository with their contents.
func (s *DeployKeysStore) List(ctx context.Context, repoID int64) ([]*DeployKey, error) {
	keys := make([]*DeployKey, 0, 5)
	err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&keys).Error
	if err != nil {
		return nil, err
	}
	return keys, s.loadContents(ctx, keys...)
}

// Touch marks the deploy key with given ID as used now.
func (s *DeployKeysStore) Touch(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).
		Model(new(DeployKey)).
		Where("id = ?", id).
		Update("updated_unix", s.db.NowFunc().Unix()).
		Error
}

// Delete deletes the deploy key with given ID, and its public key when no other
// repository uses it. Only site admins and admins of the repository can delete
// it, and others get ErrKeyAccessDenied. Deleting a key that does not exist is
// not an error.
func (s *DeployKeysStore) Delete(ctx context.Context, logger *logx.Logger, authorizedKeys AuthorizedKeysOptions, doer *User, id int64) error {
	key, err := s.GetByID(ctx, id)
	if err != nil {
		if IsErrDeployKeyNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "get deploy key")
	}

	if !doer.IsAdmin {
		repo, err := newReposStore(s.db).GetByID(ctx, key.RepoID)
		if err != nil {
			return errors.Wrap(err, "get repository")
		}
		if !newPermissionsStore(s.db).Authorize(ctx, logger, doer.ID, repo.ID, AccessModeAdmin,
			AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			return ErrKeyAccessDenied{doer.ID, key.ID, "deploy"}
		}
	}

	var publicKeyDeleted bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := lockPublicKeys(tx, key.KeyID)
		if err != nil {
			return errors.Wrap(err, "lock public key")
		}
		err = tx.Where("id = ?", key.ID).Delete(new(DeployKey)).Error
		if err != nil {
			return errors.Wrap(err, "delete deploy key")
		}
		publicKeyDeleted, err = deleteUnusedPublicKeys(tx, key.KeyID)
		return err
	})
	if err != nil {
		return err
	}

	// Other repositories still using the same key keep its "authorized_keys" line.
	if !publicKeyDeleted {
		return nil
	}
	return newPublicKeysStore(s.db).RewriteAuthorizedKeys(authorizedKeys)
}

// lockPublicKeys locks the public keys of given IDs until the transaction ends.
// Deploy keys are only added and deleted while their public keys are locked,
// so that a public key is deleted exactly when its last deploy key is.
func lockPublicKeys(tx *gorm.DB, ids ...int64) error {
	if len(ids) == 0 {
		return nil
	}

	var locked []int64
	return tx.Model(new(PublicKey)).
		Where("id IN ?", ids).
		Order("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Pluck("id", &locked).
		Error
}

// deleteUnusedPublicKeys deletes the public keys of given IDs that no deploy
// key uses, which must be locked by lockPublicKeys, and reports whether it
// deleted any.
func deleteUnusedPublicKeys(tx *gorm.DB, ids ...int64) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}

	// A locking read sees deploy keys committed after the transaction started,
	// which a plain read on MySQL does not.
	var used []int64
	err := tx.Model(new(DeployKey)).
		Where("key_id IN ?", ids).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Pluck("key_id", &used).
		Error
	if err != nil {
		return false, errors.Wrap(err, "list deploy keys")
	}

	unused := slices.DeleteFunc(slices.Clone(ids), func(id int64) bool {
		return slices.Contains(used, id)
	})
	if len(unused) == 0 {
		return false, nil
	}
	result := tx.Where("id IN ?", unused).Delete(new(PublicKey))
	if result.Error != nil {
		return false, errors.Wrap(result.Error, "delete public keys")
	}
	return result.RowsAffected > 0, nil
}
