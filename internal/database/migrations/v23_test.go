package migrations

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/repox"
)

func TestBackfillRepositoryDefaultBranch(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	conf.SetMockRepository(
		t,
		conf.RepositoryOpts{
			Root:          t.TempDir(),
			DefaultBranch: "main",
		},
	)

	db := dbtest.NewDB(t, "backfillRepositoryDefaultBranch", new(repositoryV22), new(userV22))
	require.NoError(t, db.Create(&userV22{ID: 1, Name: "alice"}).Error)
	require.NoError(t, db.Create(
		[]*repositoryV22{
			{ID: 1, OwnerID: 1, LowerName: "empty", DefaultBranch: ""},
			{ID: 2, OwnerID: 1, LowerName: "missing", DefaultBranch: ""},
			{ID: 3, OwnerID: 1, LowerName: "saved", DefaultBranch: "develop"},
		},
	).Error)

	output, err := exec.Command("git", "init", "--bare", "--initial-branch=trunk", repox.RepositoryPath("alice", "empty")).CombinedOutput()
	require.NoError(t, err, string(output))

	err = backfillRepositoryDefaultBranch(db)
	require.NoError(t, err)

	var got []*repositoryV22
	require.NoError(t, db.Order("id").Find(&got).Error)
	want := []*repositoryV22{
		{ID: 1, OwnerID: 1, LowerName: "empty", DefaultBranch: "trunk"},
		{ID: 2, OwnerID: 1, LowerName: "missing", DefaultBranch: "main"},
		{ID: 3, OwnerID: 1, LowerName: "saved", DefaultBranch: "develop"},
	}
	assert.Equal(t, want, got)

	// Re-run should be skipped
	err = backfillRepositoryDefaultBranch(db)
	require.Equal(t, errMigrationSkipped, err)
}
