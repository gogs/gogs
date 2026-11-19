package migrations

import (
	"gorm.io/gorm"
)

// issueUserIssueIDIndex is the name of the index on the issues of issue users.
const issueUserIssueIDIndex = "IDX_issue_user_issue_id"

// addIssueUserIssueIDIndex adds an index on issue_user.issue_id. Without it,
// MySQL locks every row of the table to update the users of one issue, so that
// changes to different issues wait for each other.
func addIssueUserIssueIDIndex(db *gorm.DB) error {
	if db.Migrator().HasIndex("issue_user", issueUserIssueIDIndex) {
		return errMigrationSkipped
	}
	return db.Exec("CREATE INDEX " + quote(db, issueUserIssueIDIndex) + " ON " + quote(db, "issue_user") +
		" (" + quote(db, "issue_id") + ")").Error
}
