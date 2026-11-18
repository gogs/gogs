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

// deployFingerprintIndex is the name of the unique index on the fingerprints of
// public keys of deploy keys.
const deployFingerprintIndex = "UQE_public_key_deploy_fingerprint"

// keyTypeDeploy is the type of public keys of deploy keys.
const keyTypeDeploy = 2

// addPublicKeyDeployFingerprintUniqueIndex makes repositories that add the same
// deploy key share one public key, as they are meant to.
func addPublicKeyDeployFingerprintUniqueIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("public_key", deployFingerprintIndex) {
		return errMigrationSkipped
	}
	return AddDeployFingerprintIndex(db)
}

// AddDeployFingerprintIndex merges the public keys of deploy keys that have the
// same fingerprint into the one added first, removes the others from the
// authorized_keys file, and then adds the unique index on the fingerprints of
// public keys of deploy keys. Before the index, the same key with another
// comment, or added by two repositories at the same time, got its own public
// key.
func AddDeployFingerprintIndex(db *gorm.DB) error {
	err := db.Transaction(mergeDeployPublicKeys)
	if err != nil {
		return errors.Wrap(err, "merge public keys of deploy keys")
	}

	var stmt string
	switch db.Name() {
	case "mysql":
		// MySQL has no partial indexes, so the index is on a column that only has
		// the fingerprints of deploy keys. The binary collation keeps fingerprints
		// that only differ in case apart.
		stmt = "ALTER TABLE `public_key` " +
			"ADD COLUMN `deploy_fingerprint` VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin " +
			"AS (CASE WHEN `type` = 2 THEN `fingerprint` END) STORED, " +
			"ADD UNIQUE INDEX `" + deployFingerprintIndex + "` (`deploy_fingerprint`)"
	default:
		stmt = "CREATE UNIQUE INDEX " + quote(db, deployFingerprintIndex) + " ON " + quote(db, "public_key") +
			" (" + quote(db, "fingerprint") + ") WHERE " + quote(db, "type") + " = 2"
	}
	return errors.Wrap(db.Exec(stmt).Error, "create index")
}

// DropDeployFingerprintIndex drops the index that AddDeployFingerprintIndex
// adds, e.g., to restore a backup that may have duplicates.
func DropDeployFingerprintIndex(db *gorm.DB) error {
	if !db.Migrator().HasIndex("public_key", deployFingerprintIndex) {
		return nil
	}
	if db.Name() == "mysql" {
		return db.Migrator().DropColumn("public_key", "deploy_fingerprint")
	}
	return db.Migrator().DropIndex("public_key", deployFingerprintIndex)
}

func mergeDeployPublicKeys(tx *gorm.DB) error {
	type publicKey struct {
		ID          int64
		Content     string
		Fingerprint string
	}
	var pkeys []publicKey
	err := tx.Table("public_key").Where("type = ?", keyTypeDeploy).Order("id").Find(&pkeys).Error
	if err != nil {
		return errors.Wrap(err, "list public keys")
	}

	kept := make(map[string]int64, len(pkeys))
	removed := make(map[string]bool)
	for _, pkey := range pkeys {
		// Fingerprints computed by "ssh-keygen" before OpenSSH 6.8 are MD5.
		fingerprint := pkey.Fingerprint
		if key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pkey.Content)); err == nil {
			fingerprint = ssh.FingerprintSHA256(key)
		}

		keptID, ok := kept[fingerprint]
		if !ok {
			kept[fingerprint] = pkey.ID
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

	// Before the transaction commits, so that its keys are still in the file
	// when it fails.
	return errors.Wrap(removeAuthorizedKeys(removed), "remove merged keys from authorized_keys")
}

// removeAuthorizedKeys removes the lines of the key IDs from the authorized_keys
// file. The line of a deleted key would come before the line of the key it was
// merged into when the file is in the order of its keys, and a later key may get
// its key ID.
func removeAuthorizedKeys(ids map[string]bool) error {
	if len(ids) == 0 {
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
