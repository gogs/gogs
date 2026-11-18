package database

import (
	"context"
	"database/sql"
	"math/rand/v2"
	"slices"
	"time"

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
// Repositories that use the same content share one public key. It returns
// ErrKeyAlreadyExist when the content is used by a user key,
// ErrDeployKeyAlreadyExist when the repository already has the key, and
// ErrDeployKeyNameAlreadyUsed when the repository has a key of the same name.
func (s *DeployKeysStore) Add(ctx context.Context, logger *logx.Logger, repoID int64, name, content string) (*DeployKey, error) {
	// Two repositories adding the same new key at the same time would each add
	// a public key for it. Serializable transactions make one of them fail
	// instead, and its next attempt finds the public key the other added.
	const attempts = 5
	for attempt := 1; ; attempt++ {
		key, pkey, err := s.add(ctx, repoID, name, content)
		if err != nil {
			if attempt < attempts && isSerializationFailure(err) {
				// At random, so that the transactions that failed together do not
				// conflict again.
				time.Sleep(time.Duration(1+rand.IntN(10*attempt)) * time.Millisecond)
				continue
			}
			return nil, err
		}

		if pkey != nil {
			if err = appendAuthorizedKeysToFile(logger, pkey); err != nil {
				return nil, errors.Wrap(err, "append to authorized_keys")
			}
		}
		key.Content = content
		return key, nil
	}
}

// add adds the deploy key in one serializable transaction, and returns the
// public key it added, if any.
func (s *DeployKeysStore) add(ctx context.Context, repoID int64, name, content string) (key *DeployKey, added *PublicKey, _ error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Locked like lockPublicKeys does, so that deleting the last deploy key of
		// the public key waits for this one to be added. Locked before any other
		// read, which takes shared locks on MySQL that this would have to upgrade.
		pkey := new(PublicKey)
		err := tx.Where("content = ? AND mode = ? AND type = ?", content, AccessModeRead, KeyTypeDeploy).
			Order("id").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(pkey).
			Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrap(err, "get existing key")
		}
		notFound := errors.Is(err, gorm.ErrRecordNotFound)
		if err = checkKeyContent(tx, content); err != nil {
			return err
		}
		if notFound {
			pkey = &PublicKey{
				Content: content,
				Mode:    AccessModeRead,
				Type:    KeyTypeDeploy,
			}
			if err = addKey(tx, pkey); err != nil {
				return errors.Wrap(err, "add key")
			}
			added = pkey
		}

		key, err = addDeployKey(tx, pkey.ID, repoID, name, pkey.Fingerprint)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, nil, err
	}
	return key, added, nil
}

// isSerializationFailure returns true when the error is a transaction that the
// database rolled back for conflicting with another, which succeeds when
// tried again.
func isSerializationFailure(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// SQLITE_BUSY and SQLITE_LOCKED, which the shared cache returns without
		// waiting.
		code := sqliteErr.Code() & 0xff
		return code == 5 || code == 6
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// serialization_failure and deadlock_detected.
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		// ER_LOCK_DEADLOCK.
		return mysqlErr.Number == 1213
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
func (s *DeployKeysStore) Delete(ctx context.Context, logger *logx.Logger, doer *User, id int64) error {
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
	return newPublicKeysStore(s.db).RewriteAuthorizedKeys()
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
