package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type pullRequestPreV23 struct {
	ID             int64  `gorm:"primaryKey"`
	MergeBase      string `gorm:"type:VARCHAR(40)"`
	MergedCommitID string `gorm:"type:VARCHAR(40)"`
}

func (*pullRequestPreV23) TableName() string {
	return "pull_request"
}

type releasePreV23 struct {
	ID   int64  `gorm:"primaryKey"`
	Sha1 string `gorm:"column:sha1;type:VARCHAR(40)"`
}

func (*releasePreV23) TableName() string {
	return "release"
}

type commentPreV23 struct {
	ID        int64  `gorm:"primaryKey"`
	CommitSHA string `gorm:"column:commit_sha;type:VARCHAR(40)"`
}

func (*commentPreV23) TableName() string {
	return "comment"
}

func TestWidenCommitIDColumns(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "widenCommitIDColumns", new(pullRequestPreV23), new(releasePreV23), new(commentPreV23))
	sha1ID := strings.Repeat("a", 40)
	require.NoError(t, db.Create(&pullRequestPreV23{ID: 1, MergeBase: sha1ID, MergedCommitID: sha1ID}).Error)
	require.NoError(t, db.Create(&releasePreV23{ID: 1, Sha1: sha1ID}).Error)
	require.NoError(t, db.Create(&commentPreV23{ID: 1, CommitSHA: sha1ID}).Error)

	err := widenCommitIDColumns(db)
	if db.Name() == "sqlite" {
		require.Equal(t, errMigrationSkipped, err)
	} else {
		require.NoError(t, err)
	}

	sha256ID := strings.Repeat("b", 64)
	require.NoError(t, db.Create(&pullRequestPreV23{ID: 2, MergeBase: sha256ID, MergedCommitID: sha256ID}).Error)
	require.NoError(t, db.Create(&releasePreV23{ID: 2, Sha1: sha256ID}).Error)
	require.NoError(t, db.Create(&commentPreV23{ID: 2, CommitSHA: sha256ID}).Error)

	var pull pullRequestPreV23
	require.NoError(t, db.First(&pull, 2).Error)
	assert.Equal(t, sha256ID, pull.MergeBase)
	assert.Equal(t, sha256ID, pull.MergedCommitID)

	var release releasePreV23
	require.NoError(t, db.First(&release, 2).Error)
	assert.Equal(t, sha256ID, release.Sha1)

	var comment commentPreV23
	require.NoError(t, db.First(&comment, 2).Error)
	assert.Equal(t, sha256ID, comment.CommitSHA)

	var existingPull pullRequestPreV23
	require.NoError(t, db.First(&existingPull, 1).Error)
	assert.Equal(t, sha1ID, existingPull.MergeBase)
}
