package migrations

import (
	"gorm.io/gorm"
)

// DisableProtectBranchAllowlistOfUserRepositories turns off the push allowlist
// of the protected branches of repositories that users own. The allowlist is
// only for organization repositories, and the settings of a user repository
// neither show nor edit it, yet an enabled one kept out everyone not listed by
// name, e.g., the owner. Transfers from an organization used to leave it
// enabled. Restoring a backup calls it after importing the protected branches.
func DisableProtectBranchAllowlistOfUserRepositories(db *gorm.DB) error {
	userIDs := db.Table("user").Select("id").Where("type = ?", 0)
	repoIDs := db.Table("repository").Select("id").Where("owner_id IN (?)", userIDs)
	return db.Table("protect_branch").
		Where("enable_whitelist = ? AND repo_id IN (?)", true, repoIDs).
		Update("enable_whitelist", false).
		Error
}
