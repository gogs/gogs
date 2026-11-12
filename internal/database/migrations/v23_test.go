package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type repositoryPreV23 struct {
	ID        int64 `gorm:"primaryKey"`
	OwnerID   int64
	LowerName string `gorm:"size:255;not null"`
	IsPrivate bool
}

func (*repositoryPreV23) TableName() string {
	return "repository"
}

type repositoryV23 struct {
	ID         int64 `gorm:"primaryKey"`
	OwnerID    int64
	LowerName  string `gorm:"size:255;not null"`
	IsPrivate  bool
	IsUnlisted bool `gorm:"not null;default:FALSE"`
}

func (*repositoryV23) TableName() string {
	return "repository"
}

func TestAddRepositoryIsUnlisted(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addRepositoryIsUnlisted", new(repositoryPreV23))
	err := db.Create(&repositoryPreV23{ID: 1, OwnerID: 1, LowerName: "example", IsPrivate: true}).Error
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasColumn(&repositoryV23{}, "IsUnlisted"))

	err = addRepositoryIsUnlisted(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasColumn(&repositoryV23{}, "IsUnlisted"))

	var got repositoryV23
	err = db.Where("id = ?", 1).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, repositoryV23{ID: 1, OwnerID: 1, LowerName: "example", IsPrivate: true}, got)

	// Re-run should be skipped
	err = addRepositoryIsUnlisted(db)
	require.Equal(t, errMigrationSkipped, err)
}
