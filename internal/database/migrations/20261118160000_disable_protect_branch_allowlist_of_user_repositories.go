package migrations

import (
	"gorm.io/gorm"
)

// disableProtectBranchAllowlistOfUserRepositories turns off the push allowlist
// of the protected branches of repositories that users own, which their
// settings do not show. These were left enabled by transfers from an
// organization, and kept everyone from pushing to the branch.
func disableProtectBranchAllowlistOfUserRepositories(db *gorm.DB) error {
	userIDs := db.Table("user").Select("id").Where("type = ?", 0)
	repoIDs := db.Table("repository").Select("id").Where("owner_id IN (?)", userIDs)
	return db.Table("protect_branch").
		Where("enable_whitelist = ? AND repo_id IN (?)", true, repoIDs).
		Update("enable_whitelist", false).
		Error
}
