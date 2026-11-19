package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type orphanedTeamReposTeam struct {
	ID    int64
	OrgID int64
}

func (*orphanedTeamReposTeam) TableName() string {
	return "team"
}

type orphanedTeamReposTeamRepo struct {
	ID     int64
	OrgID  int64
	TeamID int64
	RepoID int64
}

func (*orphanedTeamReposTeamRepo) TableName() string {
	return "team_repo"
}

func TestDeleteOrphanedTeamRepos(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "deleteOrphanedTeamRepos", new(orphanedTeamReposTeam), new(orphanedTeamReposTeamRepo))
	require.NoError(t, db.Create([]*orphanedTeamReposTeam{{ID: 1, OrgID: 10}, {ID: 3, OrgID: 10}}).Error)
	require.NoError(t, db.Create([]*orphanedTeamReposTeamRepo{
		{ID: 1, OrgID: 10, TeamID: 1, RepoID: 100},
		{ID: 2, OrgID: 10, TeamID: 2, RepoID: 100},
		{ID: 3, OrgID: 10, TeamID: 3, RepoID: 101},
		{ID: 4, OrgID: 10, TeamID: 2, RepoID: 101},
	}).Error)

	wantIDs := []int64{1, 3}
	for range 2 {
		require.NoError(t, deleteOrphanedTeamRepos(db))

		var gotIDs []int64
		require.NoError(t, db.Model(new(orphanedTeamReposTeamRepo)).Order("id").Pluck("id", &gotIDs).Error)
		assert.Equal(t, wantIDs, gotIDs)
	}
}
