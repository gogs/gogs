package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// addPullRequestReviews creates the tables of reviews and review requests of
// pull requests, and adds the columns that code comments, threads and the
// review settings of branch protection need.
func addPullRequestReviews(db *gorm.DB) error {
	type review struct {
		ID            int64  `gorm:"primarykey"`
		IssueID       int64  `gorm:"index:IDX_review_issue_id;not null"`
		ReviewerID    int64  `gorm:"index:IDX_review_reviewer_id;not null"`
		State         int    `gorm:"not null;default:0"`
		CommitSHA     string `gorm:"type:VARCHAR(64)"`
		CreatedUnix   int64
		SubmittedUnix int64 `gorm:"not null;default:0"`
	}
	type reviewRequest struct {
		ID          int64 `gorm:"primarykey"`
		IssueID     int64 `gorm:"uniqueIndex:UQE_review_request_review_request;not null"`
		ReviewerID  int64 `gorm:"uniqueIndex:UQE_review_request_review_request;not null"`
		RequesterID int64 `gorm:"not null"`
		CreatedUnix int64
	}

	changed := false
	for _, table := range []any{&review{}, &reviewRequest{}} {
		if db.Migrator().HasTable(table) {
			continue
		}
		err := db.Migrator().CreateTable(table)
		if err != nil {
			return errors.Wrapf(err, "create table %T", table)
		}
		changed = true
	}

	integer := map[string]string{
		"sqlite":   "INTEGER DEFAULT 0 NOT NULL",
		"postgres": "BIGINT DEFAULT 0 NOT NULL",
		"mysql":    "BIGINT(20) DEFAULT 0 NOT NULL",
	}
	boolean := map[string]string{
		"sqlite":   "INTEGER DEFAULT 0 NOT NULL",
		"postgres": "BOOL DEFAULT false NOT NULL",
		"mysql":    "TINYINT(1) DEFAULT 0 NOT NULL",
	}
	varchar := func(size string) map[string]string {
		return map[string]string{
			"sqlite":   "TEXT NULL",
			"postgres": "VARCHAR(" + size + ") NULL",
			"mysql":    "VARCHAR(" + size + ") NULL",
		}
	}
	err := addMissingColumns(db, "comment", map[string]map[string]string{
		"review_id":      integer,
		"reply_to_id":    integer,
		"is_pending":     boolean,
		"target_user_id": integer,
		"tree_path":      varchar("500"),
		"side":           varchar("3"),
		"start_side":     varchar("3"),
		"start_line":     integer,
		"base_sha":       varchar("64"),
		"diff_hunk": {
			"sqlite":   "TEXT NULL",
			"postgres": "TEXT NULL",
			"mysql":    "TEXT NULL",
		},
		"resolved_by_id": integer,
		"resolved_unix":  integer,
	})
	if err == nil {
		changed = true
	} else if err != errMigrationSkipped {
		return err
	}

	for _, column := range []string{"review_id", "reply_to_id"} {
		index := "IDX_comment_" + column
		if db.Migrator().HasIndex("comment", index) {
			continue
		}
		err := db.Exec("CREATE INDEX " + quote(db, index) + " ON " + quote(db, "comment") + " (" + quote(db, column) + ")").Error
		if err != nil {
			return errors.Wrapf(err, "create index %q", index)
		}
		changed = true
	}

	err = addMissingColumns(db, "protect_branch", map[string]map[string]string{
		"required_approvals":             integer,
		"dismiss_stale_approvals":        boolean,
		"require_resolved_conversations": boolean,
	})
	if err == nil {
		changed = true
	} else if err != errMigrationSkipped {
		return err
	}

	if !changed {
		return errMigrationSkipped
	}
	return nil
}
