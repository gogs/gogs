package database

import (
	"context"
	"slices"

	"github.com/cockroachdb/errors"
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
	db := s.db.WithContext(ctx)
	if err := checkKeyContent(db, content); err != nil {
		return nil, err
	}

	pkey := &PublicKey{
		Content: content,
		Mode:    AccessModeRead,
		Type:    KeyTypeDeploy,
	}
	existing := new(PublicKey)
	err := db.Where("content = ? AND mode = ? AND type = ?", pkey.Content, pkey.Mode, pkey.Type).First(existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrap(err, "get existing key")
	}
	has := err == nil
	if has {
		pkey = existing
	}

	var key *DeployKey
	err = db.Transaction(func(tx *gorm.DB) error {
		if has {
			locked, err := lockPublicKeys(tx, pkey.ID)
			if err != nil {
				return errors.Wrap(err, "lock public key")
			}
			// The last repository that used the key deleted it after it was found.
			if len(locked) == 0 {
				has = false
				pkey = &PublicKey{
					Content: content,
					Mode:    AccessModeRead,
					Type:    KeyTypeDeploy,
				}
			}
		}
		if !has {
			if err := addKey(logger, tx, pkey); err != nil {
				return errors.Wrap(err, "add key")
			}
		}

		var err error
		key, err = addDeployKey(tx, pkey.ID, repoID, name, pkey.Fingerprint)
		return err
	})
	if err != nil {
		return nil, err
	}
	key.Content = content
	return key, nil
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
		_, err := lockPublicKeys(tx, key.KeyID)
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

// lockPublicKeys locks the public keys of given IDs until the transaction ends,
// and returns the IDs of those that exist. Deploy keys are only added and
// deleted while their public keys are locked, so that a public key is deleted
// exactly when its last deploy key is.
func lockPublicKeys(tx *gorm.DB, ids ...int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var locked []int64
	err := tx.Model(new(PublicKey)).
		Where("id IN ?", ids).
		Order("id").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Pluck("id", &locked).
		Error
	return locked, err
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
