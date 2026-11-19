package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/dbtest"
)

// TestRestoreScenarios restores backups that released versions made, whose rows
// have the fields of those versions, and checks the data that restoring decides:
// renamed fields, fields added since, and data that migrations rewrite on
// upgrade but restoring inserts after them.
//
// Add a backup in testdata/backup/<version> when a released version writes rows
// that the current structs or migrations read differently.
func TestRestoreScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	// Repository 1 belongs to user 1 and repository 2 to organization 2, and the
	// main branch of each has a push allowlist.
	checkCommon := func(t *testing.T, db *gorm.DB) {
		var password string
		require.NoError(t, db.Table("user").Where("id = 1").Select("passwd").Scan(&password).Error)
		assert.Equal(t, "hashed-password", password)

		type repository struct {
			ID                int64
			IsPrivate         bool
			IsUnlisted        bool
			PullsAllowMerge   bool
			PullsAllowSquash  bool
			PullsDeleteBranch bool
		}
		var repos []repository
		require.NoError(t, db.Table("repository").Order("id").Find(&repos).Error)
		assert.Equal(t, []repository{
			{ID: 1, IsPrivate: true, PullsAllowMerge: true},
			{ID: 2, PullsAllowMerge: true},
		}, repos)

		type protectBranch struct {
			ID               int64
			EnableWhitelist  bool   `gorm:"column:enable_whitelist"`
			WhitelistUserIDs string `gorm:"column:whitelist_user_i_ds"`
			WhitelistTeamIDs string `gorm:"column:whitelist_team_i_ds"`
		}
		var branches []protectBranch
		require.NoError(t, db.Table("protect_branch").Order("id").Find(&branches).Error)
		assert.Equal(t, []protectBranch{
			{ID: 1, EnableWhitelist: false, WhitelistUserIDs: "1"},
			{ID: 2, EnableWhitelist: true, WhitelistUserIDs: "1", WhitelistTeamIDs: "3"},
		}, branches)
	}

	scenarios := []struct {
		version string
		check   func(t *testing.T, db *gorm.DB)
	}{
		{
			version: "v0.12.11",
			check: func(t *testing.T, db *gorm.DB) {
				checkCommon(t, db)

				var hashes []string
				require.NoError(t, db.Table("access_token").Order("id").Pluck("sha256", &hashes).Error)
				assert.Equal(t, []string{
					cryptox.SHA256("73da7bb9d2a475bbc2ab79da7d4e94940cb9f9d5"),
					cryptox.SHA256("4c6dc8d6a4b9ed1d0c1b5bfb1b8b1b5b1b8b1b5b"),
				}, hashes)

				// Attachments from before uploaders were recorded have no uploader.
				var withoutUploader int64
				require.NoError(t, db.Table("attachment").Where("uploader_id IS NULL").Count(&withoutUploader).Error)
				assert.Equal(t, int64(1), withoutUploader)

				for _, table := range []string{"email_address", "star", "issue_user"} {
					var userIDs []int64
					require.NoError(t, db.Table(table).Pluck("uid", &userIDs).Error)
					assert.Equal(t, []int64{1}, userIDs, "table %q", table)
				}
			},
		},
		{
			version: "v0.14.4",
			check: func(t *testing.T, db *gorm.DB) {
				checkCommon(t, db)

				var hashes []string
				require.NoError(t, db.Table("access_token").Order("id").Pluck("sha256", &hashes).Error)
				assert.Equal(t, []string{cryptox.SHA256("token")}, hashes)
			},
		},
	}
	for _, s := range scenarios {
		t.Run(s.version, func(t *testing.T) {
			db := dbtest.NewDB(t, "restoreScenario"+s.version)

			err := ImportDatabase(context.Background(), logx.NewNoopLogger(), db, filepath.Join("testdata", "backup", s.version), false)
			require.NoError(t, err)
			s.check(t, db)
		})
	}
}
