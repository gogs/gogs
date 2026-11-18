package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type userForProtectBranchAllowlist struct {
	ID   int64 `gorm:"primaryKey"`
	Type int
}

func (*userForProtectBranchAllowlist) TableName() string {
	return "user"
}

type repositoryForProtectBranchAllowlist struct {
	ID      int64 `gorm:"primaryKey"`
	OwnerID int64
}

func (*repositoryForProtectBranchAllowlist) TableName() string {
	return "repository"
}

type protectBranchForAllowlist struct {
	ID                 int64 `gorm:"primaryKey"`
	RepoID             int64
	Name               string
	RequirePullRequest bool
	EnableWhitelist    bool
}

func (*protectBranchForAllowlist) TableName() string {
	return "protect_branch"
}

func TestDisableProtectBranchAllowlistOfUserRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "disableProtectBranchAllowlistOfUserRepositories",
		new(userForProtectBranchAllowlist), new(repositoryForProtectBranchAllowlist), new(protectBranchForAllowlist))
	const userType, orgType = 0, 1
	require.NoError(t, db.Create([]*userForProtectBranchAllowlist{{ID: 1, Type: userType}, {ID: 2, Type: orgType}}).Error)
	require.NoError(t, db.Create([]*repositoryForProtectBranchAllowlist{{ID: 10, OwnerID: 1}, {ID: 20, OwnerID: 2}}).Error)
	require.NoError(t, db.Create([]*protectBranchForAllowlist{
		{ID: 1, RepoID: 10, Name: "main", RequirePullRequest: true, EnableWhitelist: true},
		{ID: 2, RepoID: 20, Name: "main", RequirePullRequest: true, EnableWhitelist: true},
	}).Error)

	require.NoError(t, disableProtectBranchAllowlistOfUserRepositories(db))

	var got []*protectBranchForAllowlist
	require.NoError(t, db.Order("id").Find(&got).Error)
	assert.Equal(t, []*protectBranchForAllowlist{
		{ID: 1, RepoID: 10, Name: "main", RequirePullRequest: true, EnableWhitelist: false},
		{ID: 2, RepoID: 20, Name: "main", RequirePullRequest: true, EnableWhitelist: true},
	}, got)
}
