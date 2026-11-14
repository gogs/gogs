package migrations

import (
	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

type pullRequestV23 struct {
	MergeBase      string `gorm:"type:VARCHAR(64)"`
	MergedCommitID string `gorm:"type:VARCHAR(64)"`
}

func (*pullRequestV23) TableName() string {
	return "pull_request"
}

type releaseV23 struct {
	Sha1 string `gorm:"column:sha1;type:VARCHAR(64)"`
}

func (*releaseV23) TableName() string {
	return "release"
}

type commentV23 struct {
	CommitSHA string `gorm:"column:commit_sha;type:VARCHAR(64)"`
}

func (*commentV23) TableName() string {
	return "comment"
}

// widenCommitIDColumns makes the columns that store commit IDs long enough for
// the 64 characters of a SHA-256 commit ID.
func widenCommitIDColumns(db *gorm.DB) error {
	// SQLite does not enforce the length of VARCHAR, and altering a column there
	// means recreating the table.
	if db.Name() == "sqlite" {
		return errMigrationSkipped
	}

	columns := []struct {
		model any
		field string
	}{
		{new(pullRequestV23), "MergeBase"},
		{new(pullRequestV23), "MergedCommitID"},
		{new(releaseV23), "Sha1"},
		{new(commentV23), "CommitSHA"},
	}
	for _, c := range columns {
		err := db.Migrator().AlterColumn(c.model, c.field)
		if err != nil {
			return errors.Wrapf(err, "alter column %q", c.field)
		}
	}
	return nil
}
