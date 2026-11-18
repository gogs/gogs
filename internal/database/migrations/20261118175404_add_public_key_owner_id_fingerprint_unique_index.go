package migrations

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
)

// ownerFingerprintIndex is the name of the unique index on the owners and
// fingerprints of public keys.
const ownerFingerprintIndex = "UQE_public_key_owner_id_fingerprint"

// keyTypeDeploy is the type of public keys of deploy keys.
const keyTypeDeploy = 2

// addPublicKeyOwnerIDFingerprintUniqueIndex makes each user have a key only
// once, and repositories that add the same deploy key share one public key, as
// they are meant to.
func addPublicKeyOwnerIDFingerprintUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("public_key", ownerFingerprintIndex) {
		return errMigrationSkipped
	}
	return MergeDuplicatePublicKeys(db)
}

// MergeDuplicatePublicKeys deletes deploy keys of repositories that no longer
// exist and public keys of deploy keys that no deploy key uses, merges the
// public keys that have the same owner and fingerprint into the one added first,
// removes the deleted ones from the authorized_keys file, and then adds the unique index on the owners and
// fingerprints of public keys. Before the index, the same key with another
// comment, or added twice at the same time, got a public key of its own. Public
// keys of deploy keys have no owner, so they are unique by their fingerprints.
// Restoring a backup calls it after importing the public keys.
func MergeDuplicatePublicKeys(db *gorm.DB) error {
	err := db.Transaction(mergePublicKeys)
	if err != nil {
		return errors.Wrap(err, "merge public keys")
	}
	err = db.Exec("CREATE UNIQUE INDEX " + quote(db, ownerFingerprintIndex) + " ON " + quote(db, "public_key") +
		" (" + quote(db, "owner_id") + ", " + quote(db, "fingerprint") + ")").Error
	return errors.Wrap(err, "create index")
}

// AllowDuplicatePublicKeys drops the index that MergeDuplicatePublicKeys adds,
// so that restoring a backup from before the index can import its duplicates,
// which MergeDuplicatePublicKeys then merges.
func AllowDuplicatePublicKeys(db *gorm.DB) error {
	if !db.Migrator().HasIndex("public_key", ownerFingerprintIndex) {
		return nil
	}
	return db.Migrator().DropIndex("public_key", ownerFingerprintIndex)
}

func mergePublicKeys(tx *gorm.DB) error {
	removed, err := deleteOrphanDeployKeys(tx)
	if err != nil {
		return errors.Wrap(err, "delete orphan deploy keys")
	}

	type publicKey struct {
		ID          int64
		OwnerID     int64
		Content     string
		Fingerprint string
	}
	var pkeys []publicKey
	err = tx.Table("public_key").Order("id").Find(&pkeys).Error
	if err != nil {
		return errors.Wrap(err, "list public keys")
	}

	type ownerFingerprint struct {
		ownerID     int64
		fingerprint string
	}
	kept := make(map[ownerFingerprint]int64, len(pkeys))
	for _, pkey := range pkeys {
		// Fingerprints computed by "ssh-keygen" before OpenSSH 6.8 are MD5.
		fingerprint := pkey.Fingerprint
		if key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pkey.Content)); err == nil {
			fingerprint = ssh.FingerprintSHA256(key)
		}

		keptID, ok := kept[ownerFingerprint{pkey.OwnerID, fingerprint}]
		if !ok {
			kept[ownerFingerprint{pkey.OwnerID, fingerprint}] = pkey.ID
			if fingerprint == pkey.Fingerprint {
				continue
			}
			err = tx.Table("public_key").Where("id = ?", pkey.ID).Update("fingerprint", fingerprint).Error
			if err != nil {
				return errors.Wrapf(err, "update fingerprint of public key %d", pkey.ID)
			}
			err = tx.Table("deploy_key").Where("key_id = ?", pkey.ID).Update("fingerprint", fingerprint).Error
			if err != nil {
				return errors.Wrapf(err, "update fingerprint of deploy keys of public key %d", pkey.ID)
			}
			continue
		}

		// Only public keys of deploy keys are referenced, by their deploy keys.
		type deployKey struct {
			ID     int64
			RepoID int64
		}
		var keys []deployKey
		err = tx.Table("deploy_key").Where("key_id = ?", pkey.ID).Find(&keys).Error
		if err != nil {
			return errors.Wrapf(err, "list deploy keys of public key %d", pkey.ID)
		}
		for _, key := range keys {
			var count int64
			err = tx.Table("deploy_key").Where("key_id = ? AND repo_id = ?", keptID, key.RepoID).Count(&count).Error
			if err != nil {
				return errors.Wrapf(err, "count deploy keys of repository %d", key.RepoID)
			}
			// The repository has the key twice, under different comments.
			if count > 0 {
				err = tx.Exec("DELETE FROM "+quote(tx, "deploy_key")+" WHERE id = ?", key.ID).Error
			} else {
				err = tx.Table("deploy_key").Where("id = ?", key.ID).
					Updates(map[string]any{"key_id": keptID, "fingerprint": fingerprint}).Error
			}
			if err != nil {
				return errors.Wrapf(err, "merge deploy key %d", key.ID)
			}
		}

		err = tx.Exec("DELETE FROM "+quote(tx, "public_key")+" WHERE id = ?", pkey.ID).Error
		if err != nil {
			return errors.Wrapf(err, "delete public key %d", pkey.ID)
		}
		removed[strconv.FormatInt(pkey.ID, 10)] = true
	}

	// Before the transaction commits, so that failing to update the file rolls
	// back the deletions. Should the commit fail after, the keys stay without
	// their lines until SSH keys are resynced, which is safer than lines of keys
	// that no longer exist.
	return errors.Wrap(removeAuthorizedKeys(removed), "remove merged keys from authorized_keys")
}

// deleteOrphanDeployKeys deletes the deploy keys of repositories that no longer
// exist, which deleting a repository used to leave behind, and the public keys
// of deploy keys that no deploy key uses. On MySQL 5.7, a new repository may get
// the ID of a deleted one, and with it the read access of its deploy keys. It
// returns the IDs of the public keys it deleted.
func deleteOrphanDeployKeys(tx *gorm.DB) (map[string]bool, error) {
	// NOT EXISTS rather than NOT IN, which matches nothing when the subquery has
	// a NULL, e.g., deploy_key.key_id on PostgreSQL.
	err := tx.Exec("DELETE FROM " + quote(tx, "deploy_key") + " WHERE NOT EXISTS (SELECT 1 FROM " + quote(tx, "repository") +
		" WHERE " + quote(tx, "repository") + ".id = " + quote(tx, "deploy_key") + ".repo_id)").Error
	if err != nil {
		return nil, errors.Wrap(err, "delete deploy keys")
	}

	var ids []int64
	err = tx.Raw("SELECT id FROM "+quote(tx, "public_key")+" WHERE type = ? AND NOT EXISTS (SELECT 1 FROM "+quote(tx, "deploy_key")+
		" WHERE "+quote(tx, "deploy_key")+".key_id = "+quote(tx, "public_key")+".id)", keyTypeDeploy).
		Scan(&ids).Error
	if err != nil {
		return nil, errors.Wrap(err, "list unused public keys")
	}
	removed := make(map[string]bool, len(ids))
	for _, id := range ids {
		err = tx.Exec("DELETE FROM "+quote(tx, "public_key")+" WHERE id = ?", id).Error
		if err != nil {
			return nil, errors.Wrapf(err, "delete public key %d", id)
		}
		removed[strconv.FormatInt(id, 10)] = true
	}
	return removed, nil
}

// removeAuthorizedKeys removes the lines of the key IDs from the authorized_keys
// file. The line of a deleted key would come before the line of the key it was
// merged into when the file is in the order of its keys, and a later key may get
// its key ID.
func removeAuthorizedKeys(ids map[string]bool) error {
	// The builtin SSH server does not read the file.
	if len(ids) == 0 || conf.SSH.StartBuiltinServer {
		return nil
	}

	path := filepath.Join(conf.SSH.RootPath, "authorized_keys")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "read")
	}

	var kept strings.Builder
	for line := range strings.Lines(string(data)) {
		_, rest, _ := strings.Cut(line, " serv key-")
		id, _, _ := strings.Cut(rest, " ")
		if !ids[id] {
			kept.WriteString(line)
		}
	}

	tempPath := path + ".tmp"
	if err = os.WriteFile(tempPath, []byte(kept.String()), 0o600); err != nil {
		return errors.Wrap(err, "write temporary file")
	}
	return errors.Wrap(os.Rename(tempPath, path), "rename")
}
