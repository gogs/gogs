package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type repositoryPreV24 struct {
	ID        int64 `gorm:"primaryKey"`
	OwnerID   int64
	LowerName string `gorm:"size:255;not null"`
	IsPrivate bool
}

func (*repositoryPreV24) TableName() string {
	return "repository"
}

type repositoryV24 struct {
	ID         int64 `gorm:"primaryKey"`
	OwnerID    int64
	LowerName  string `gorm:"size:255;not null"`
	IsPrivate  bool
	IsUnlisted bool `gorm:"not null;default:FALSE"`
}

func (*repositoryV24) TableName() string {
	return "repository"
}

func TestAddRepositoryIsUnlisted(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addRepositoryIsUnlisted", new(repositoryPreV24))
	err := db.Create(&repositoryPreV24{ID: 1, OwnerID: 1, LowerName: "example", IsPrivate: true}).Error
	require.NoError(t, err)
	assert.False(t, db.Migrator().HasColumn(&repositoryV24{}, "IsUnlisted"))

	err = addRepositoryIsUnlisted(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasColumn(&repositoryV24{}, "IsUnlisted"))

	var got repositoryV24
	err = db.Where("id = ?", 1).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, repositoryV24{ID: 1, OwnerID: 1, LowerName: "example", IsPrivate: true}, got)

	// Re-run should be skipped
	err = addRepositoryIsUnlisted(db)
	require.Equal(t, errMigrationSkipped, err)
}
