package migrations

import (
	"gorm.io/gorm"
)

// copyLFSObjectsToForks gives forks the LFS object records of their base
// repositories, which forking now copies but used to leave out. Each pass
// copies one level down chains of forks, until no fork lacks a record of its
// base repository.
func copyLFSObjectsToForks(db *gorm.DB) error {
	for {
		result := db.Exec(`
INSERT INTO lfs_object (repo_id, oid, size, storage, created_at)
SELECT fork.id, base.oid, base.size, base.storage, base.created_at
FROM repository fork
JOIN lfs_object base ON base.repo_id = fork.fork_id
WHERE fork.fork_id > 0 AND NOT EXISTS (
	SELECT 1 FROM lfs_object own WHERE own.repo_id = fork.id AND own.oid = base.oid
)`)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
	}
}
