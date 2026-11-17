package migrations

import (
	"gorm.io/gorm"
)

// dropProtectBranchWhitelistTable drops the table that copied the users of the
// push allowlist of protected branches and the members of its teams. The
// allowlist is now checked against the users and teams the branch protection
// stores and the current members of the teams.
func dropProtectBranchWhitelistTable(db *gorm.DB) error {
	if !db.Migrator().HasTable("protect_branch_whitelist") {
		return errMigrationSkipped
	}
	return db.Migrator().DropTable("protect_branch_whitelist")
}
