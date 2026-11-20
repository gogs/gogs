package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/dbtest"
)

func TestAddIssueUserIssueIDIndex(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addIssueUserIssueIDIndex")
	require.NoError(t, Migrate(logx.NewNoopLogger(), db))
	require.True(t, db.Migrator().HasIndex("issue_user", issueUserIssueIDIndex))
	require.NoError(t, db.Migrator().DropIndex("issue_user", issueUserIssueIDIndex))

	err := addIssueUserIssueIDIndex(db)
	require.NoError(t, err)
	assert.True(t, db.Migrator().HasIndex("issue_user", issueUserIssueIDIndex))

	err = addIssueUserIssueIDIndex(db)
	assert.Equal(t, errMigrationSkipped, err)
}
