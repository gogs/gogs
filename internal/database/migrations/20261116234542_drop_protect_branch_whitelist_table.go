package migrations

import (
	"gorm.io/gorm"
)

// dropProtectBranchWhitelistTable drops protect_branch_whitelist, which held
// the users allowed to push to protected branches, with the members of
// whitelisted teams copied in when the branch was saved. Pushes check the users
// and teams of protect_branch instead.
func dropProtectBranchWhitelistTable(db *gorm.DB) error {
	if !db.Migrator().HasTable("protect_branch_whitelist") {
		return errMigrationSkipped
	}
	return db.Migrator().DropTable("protect_branch_whitelist")
}
