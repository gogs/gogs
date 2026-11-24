package migrations

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbtest"
)

// migrationScenario is a database in a state that existing installations are in
// before they upgrade.
type migrationScenario struct {
	name string
	// legacy is the version of the legacy "version" table to start from, or zero
	// to start from an empty database.
	legacy int64
	// from is the version to run the migrations up to before seeding, or zero to
	// run none.
	from int64
	// seed adds what the installations have at the starting state. It writes
	// tables without the current models, because they describe today's schema
	// rather than the one at the starting version.
	seed func(t *testing.T, db *gorm.DB)
	// check verifies the data that the migrations change after migrating to the
	// latest version.
	check func(t *testing.T, db *gorm.DB)
}

// TestMigrateScenarios migrates each scenario to the latest version, then checks
// its data and that its schema is the same as the one of a new installation.
//
// Add a scenario for a state that existing installations are in when a new
// migration may fail on it or change its data in the wrong way, e.g., rows
// written by a released version that the migration rewrites. Check the data
// that the migrations decide, not what the database keeps on its own.
func TestMigrateScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	newDB := dbtest.NewDB(t, "migrateScenarioNew")
	require.NoError(t, Migrate(logx.NewNoopLogger(), newDB))
	newSchema := dumpSchema(t, newDB)

	newKey := func() (string, string) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		key, err := ssh.NewPublicKey(pub)
		require.NoError(t, err)
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key)
	}
	sharedKey, sharedFingerprint := newKey()
	otherKey, otherFingerprint := newKey()
	orphanKey, orphanFingerprint := newKey()
	authorizedKeysLine := func(id int64, content string) string {
		return fmt.Sprintf("command=\"gogs serv key-%d --config=''\",no-pty %s\n", id, content)
	}

	scenarios := []migrationScenario{
		{
			name: "new installation",
			check: func(t *testing.T, db *gorm.DB) {
				tables, err := db.Migrator().GetTables()
				require.NoError(t, err)
				tables = slices.DeleteFunc(tables, func(name string) bool { return name == "sqlite_sequence" })
				assert.Len(t, tables, 37) // 36 tables and "goose_db_version"
				assert.Contains(t, tables, "user")
				assert.Contains(t, tables, "passkey")
				assert.NotContains(t, tables, "version")
				assert.NotContains(t, tables, "protect_branch_whitelist")
				assert.True(t, db.Migrator().HasIndex("attachment", "UQE_attachment_uuid"))
				assert.True(t, db.Migrator().HasIndex("public_key", ownerFingerprintIndex))
				assert.True(t, db.Migrator().HasIndex("issue_user", issueUserIssueIDIndex))
				for _, idx := range repoIDIndexes {
					assert.True(t, db.Migrator().HasIndex(idx.table, idx.name), idx.name)
				}
				assert.False(t, db.Migrator().HasColumn("user", "avatar"))
				assert.False(t, db.Migrator().HasColumn("user", "avatar_email"))
			},
		},
		{
			name: "new installation on MySQL with another default character set",
			seed: func(t *testing.T, db *gorm.DB) {
				if db.Name() != "mysql" {
					t.Skip("MySQL only")
				}
				require.NoError(t, db.Exec("ALTER DATABASE `"+db.Migrator().CurrentDatabase()+"` CHARACTER SET latin1 COLLATE latin1_swedish_ci").Error)
			},
			check: func(t *testing.T, db *gorm.DB) {
				// Tables that XORM used to create stay utf8mb4, as XORM created them.
				require.NoError(t, db.Exec("INSERT INTO issue (repo_id, `index`, name) VALUES (1, 1, ?)", "Emoji \U0001F600").Error)
			},
		},
		{
			name:   "Gogs 0.12",
			legacy: 19,
			seed: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Table("access_token").Create([]map[string]any{
					{"id": 1, "uid": 1, "name": "ci", "sha1": "73da7bb9d2a475bbc2ab79da7d4e94940cb9f9d5"},
					{"id": 2, "uid": 2, "name": "deploy", "sha1": "4c6dc8d6a4b9ed1d0c1b5bfb1b8b1b5b1b8b1b5b"},
				}).Error)
				const userType, orgType = 0, 1
				require.NoError(t, db.Table("user").Create([]map[string]any{
					{"id": 1, "lower_name": "alice", "name": "alice", "email": "alice@example.com", "passwd": "", "avatar": "", "avatar_email": "", "type": userType},
					{"id": 2, "lower_name": "acme", "name": "acme", "email": "acme@example.com", "passwd": "", "avatar": "", "avatar_email": "", "type": orgType},
				}).Error)
				require.NoError(t, db.Table("repository").Create([]map[string]any{
					{"id": 1, "owner_id": 1, "lower_name": "example", "name": "example"},
					{"id": 2, "owner_id": 2, "lower_name": "example", "name": "example"},
				}).Error)

				// Push allowlists on branches of a user repository and of an organization
				// repository.
				require.NoError(t, db.Table("protect_branch").Create([]map[string]any{
					{"id": 1, "repo_id": 1, "name": "main", "protected": true, "enable_whitelist": true, "whitelist_user_i_ds": "1"},
					{"id": 2, "repo_id": 2, "name": "main", "protected": true, "enable_whitelist": true, "whitelist_user_i_ds": "1"},
				}).Error)
				require.NoError(t, db.Table("protect_branch_whitelist").Create([]map[string]any{
					{"id": 1, "protect_branch_id": 1, "repo_id": 1, "name": "main", "user_id": 1},
					{"id": 2, "protect_branch_id": 2, "repo_id": 2, "name": "main", "user_id": 1},
				}).Error)

				// Deleting team 2 left its team-repo relation behind.
				require.NoError(t, db.Table("team").Create(map[string]any{"id": 1, "org_id": 2, "lower_name": "owners", "name": "Owners", "authorize": 4, "num_repos": 1}).Error)
				require.NoError(t, db.Table("team_repo").Create([]map[string]any{
					{"id": 1, "org_id": 2, "team_id": 1, "repo_id": 2},
					{"id": 2, "org_id": 2, "team_id": 2, "repo_id": 2},
				}).Error)
			},
			check: func(t *testing.T, db *gorm.DB) {
				assert.Equal(t, []string{
					sha256Hex("73da7bb9d2a475bbc2ab79da7d4e94940cb9f9d5"),
					sha256Hex("4c6dc8d6a4b9ed1d0c1b5bfb1b8b1b5b1b8b1b5b"),
				}, accessTokenSHA256s(t, db))

				// Existing repositories stay listed, keep allowing merge commits, do not allow
				// squashing, keep head branches, and have no message presets.
				var count int64
				require.NoError(t, db.Table("repository").
					Where("is_unlisted = ? AND pulls_allow_merge = ? AND pulls_allow_squash = ? AND pulls_delete_branch = ? AND pulls_merge_message = '' AND pulls_squash_message = ''", false, true, false, false).
					Count(&count).Error)
				assert.Equal(t, int64(2), count)

				// Only organization repositories keep their push allowlists.
				var enableAllowlist []bool
				require.NoError(t, db.Table("protect_branch").Order("id").Pluck("enable_whitelist", &enableAllowlist).Error)
				assert.Equal(t, []bool{false, true}, enableAllowlist)

				var teamRepoIDs []int64
				require.NoError(t, db.Table("team_repo").Order("id").Pluck("id", &teamRepoIDs).Error)
				assert.Equal(t, []int64{1}, teamRepoIDs)
			},
		},
		{
			// Gogs 0.12 created only missing tables, so databases that started on 0.11
			// keep the index on "access_token.uid" as XORM named it.
			name:   "Gogs 0.12, started on 0.11",
			legacy: 19,
			seed: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Migrator().DropIndex("access_token", "idx_access_token_uid"))
				require.NoError(t, db.Exec("CREATE INDEX "+quote(db, "IDX_access_token_uid")+" ON "+quote(db, "access_token")+" ("+quote(db, "uid")+")").Error)
			},
		},
		{
			// A bug in the legacy runner left some instances of 0.13 and 0.14 at
			// version 21 and others at 22, which the migrations treat the same.
			name:   "Gogs 0.13 and 0.14",
			legacy: 21,
			seed: func(t *testing.T, db *gorm.DB) {
				// XORM added the column on startup.
				require.NoError(t, addRepositoryIsUnlisted(db))
				require.NoError(t, db.Table("repository").Create(map[string]any{"id": 1, "owner_id": 1, "lower_name": "example", "name": "example", "is_unlisted": true}).Error)
				for repoID := int64(2); repoID <= 3; repoID++ {
					require.NoError(t, db.Table("repository").Create(map[string]any{"id": repoID, "owner_id": 1, "lower_name": fmt.Sprintf("repo-%d", repoID), "name": fmt.Sprintf("repo-%d", repoID)}).Error)
				}

				// Since 0.13, tokens store their SHA-256 and the first 40 characters of it as
				// SHA-1, so hashing the SHA-1 again breaks them.
				require.NoError(t, db.Table("access_token").Create(map[string]any{"id": 1, "uid": 1, "name": "ci", "sha1": sha256Hex("token")[:40], "sha256": sha256Hex("token")}).Error)

				const keyTypeUser = 1
				addPublicKey := func(id, ownerID int64, keyType int, content, fingerprint string) {
					require.NoError(t, db.Table("public_key").Create(map[string]any{
						"id": id, "owner_id": ownerID, "name": fmt.Sprintf("key-%d", id), "fingerprint": fingerprint, "content": content, "mode": 1, "type": keyType,
					}).Error)
				}
				addDeployKey := func(id, keyID, repoID int64, fingerprint string) {
					require.NoError(t, db.Table("deploy_key").Create(map[string]any{
						"id": id, "key_id": keyID, "repo_id": repoID, "name": fmt.Sprintf("key-%d", id), "fingerprint": fingerprint,
					}).Error)
				}
				// Deploy keys: the same key with another comment and the MD5 fingerprint of
				// an old "ssh-keygen", and another key with an MD5 fingerprint.
				addPublicKey(1, 0, keyTypeDeploy, sharedKey+" laptop", sharedFingerprint)
				addPublicKey(2, 0, keyTypeDeploy, sharedKey+" desktop", "3b:0c:aa:11")
				addPublicKey(3, 0, keyTypeDeploy, otherKey, "9f:21:bb:22")
				// User keys: user 7 has the key twice, user 8 has it once and keeps it.
				addPublicKey(4, 7, keyTypeUser, otherKey+" alice@laptop", otherFingerprint)
				addPublicKey(5, 7, keyTypeUser, otherKey+" alice@desktop", otherFingerprint)
				addPublicKey(6, 8, keyTypeUser, otherKey, otherFingerprint)
				addDeployKey(1, 1, 1, sharedFingerprint)
				// Repository 1 has the key twice, repository 2 only through the duplicate.
				addDeployKey(2, 2, 1, "3b:0c:aa:11")
				addDeployKey(3, 2, 2, "3b:0c:aa:11")
				addDeployKey(4, 3, 3, "9f:21:bb:22")
				// Left behind by deleting repository 9: a key only it used, and the shared
				// key, which repository 1 keeps. Key 14 has no deploy key at all.
				addPublicKey(13, 0, keyTypeDeploy, orphanKey, orphanFingerprint)
				addPublicKey(14, 0, keyTypeDeploy, orphanKey+" again", "aa:bb")
				addDeployKey(5, 13, 9, orphanFingerprint)
				addDeployKey(6, 1, 9, sharedFingerprint)

				rootPath := t.TempDir()
				conf.SetMockSSH(t, conf.SSHOpts{RootPath: rootPath})
				line := authorizedKeysLine
				require.NoError(t, os.WriteFile(filepath.Join(rootPath, "authorized_keys"),
					[]byte(line(2, sharedKey+" desktop")+line(1, sharedKey+" laptop")+line(3, otherKey)+line(5, otherKey+" alice@desktop")+line(4, otherKey+" alice@laptop")+line(6, otherKey)+line(52, otherKey)+line(13, orphanKey)+line(14, orphanKey+" again")+"ssh-ed25519 AAAA admin\n"),
					0o600))
			},
			check: func(t *testing.T, db *gorm.DB) {
				assert.Equal(t, []string{sha256Hex("token")}, accessTokenSHA256s(t, db))

				var isUnlisted bool
				require.NoError(t, db.Table("repository").Where("id = 1").Select("is_unlisted").Scan(&isUnlisted).Error)
				assert.True(t, isUnlisted)

				type publicKey struct {
					ID          int64
					OwnerID     int64
					Fingerprint string
				}
				var publicKeys []publicKey
				require.NoError(t, db.Table("public_key").Order("id").Find(&publicKeys).Error)
				assert.Equal(t, []publicKey{
					{1, 0, sharedFingerprint},
					{3, 0, otherFingerprint},
					{4, 7, otherFingerprint},
					{6, 8, otherFingerprint},
				}, publicKeys)

				type deployKey struct {
					ID          int64
					KeyID       int64
					RepoID      int64
					Fingerprint string
				}
				var deployKeys []deployKey
				require.NoError(t, db.Table("deploy_key").Order("id").Find(&deployKeys).Error)
				assert.Equal(t, []deployKey{
					{1, 1, 1, sharedFingerprint},
					{3, 1, 2, sharedFingerprint},
					{4, 3, 3, otherFingerprint},
				}, deployKeys)

				// Only the lines of the merged and orphan keys are gone, not the one of key
				// 52, whose ID only starts with that of key 5, or the line Gogs did not write.
				line := authorizedKeysLine
				data, err := os.ReadFile(filepath.Join(conf.SSH.RootPath, "authorized_keys"))
				require.NoError(t, err)
				assert.Equal(t, line(1, sharedKey+" laptop")+line(3, otherKey)+line(4, otherKey+" alice@laptop")+line(6, otherKey)+line(52, otherKey)+"ssh-ed25519 AAAA admin\n", string(data))
			},
		},
		{
			// The edge image ships main, where XORM and GORM added these columns and
			// tables on startup before the migrations did.
			name:   "main before goose",
			legacy: 23,
			seed: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, addRepositoryIsUnlisted(db))
				require.NoError(t, addRepositoryPullsAllowMergeAndSquash(db))
				require.NoError(t, addRepositoryPullsMergeSettings(db))
				require.NoError(t, addAttachmentUploaderID(db))
				require.NoError(t, createPasskeyTable(db))
			},
		},
		{
			// On SQLite, version 20 dropped the index on "access_token.uid" of instances
			// that the edge image has upgraded since.
			name:   "main tracked by goose, upgraded from Gogs 0.12",
			legacy: 19,
			from:   20261121220000,
		},
	}
	for i, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()

			db := dbtest.NewDB(t, fmt.Sprintf("migrateScenario%d", i))
			if s.legacy > 0 {
				createLegacyDatabase(t, db, s.legacy)
			}
			if s.from > 0 {
				require.NoError(t, migrateTo(logx.NewNoopLogger(), db, s.from))
				require.Equal(t, s.from, gooseVersion(t, db))
			}
			if s.seed != nil {
				s.seed(t, db)
			}

			require.NoError(t, Migrate(logx.NewNoopLogger(), db))
			assert.Equal(t, latestVersion(db), gooseVersion(t, db))
			if s.check != nil {
				s.check(t, db)
			}
			assertSameSchema(t, newSchema, dumpSchema(t, db))
		})
	}
}

// createLegacyDatabase turns an empty database into one that the legacy runner
// left at the given version: the schema of Gogs 0.12 at version 19, changed by
// the migrations that the legacy runner ran for each later version.
func createLegacyDatabase(t *testing.T, db *gorm.DB, version int64) {
	require.NoError(t, createBaselineTables(db))

	// Undo what versions 20 to 23 and the migrations of the columns that XORM used
	// to add on startup put in the baseline.
	require.NoError(t, db.Exec("ALTER TABLE repository DROP COLUMN is_unlisted").Error)
	if db.Name() == "sqlite" {
		// SQLite cannot drop a column with a UNIQUE constraint.
		require.NoError(t, db.Migrator().DropTable("access_token"))
		require.NoError(t, db.Exec("CREATE TABLE `access_token` (`id` integer PRIMARY KEY AUTOINCREMENT, `uid` integer, `name` text, `sha1` VARCHAR(40), `created_unix` integer, `updated_unix` integer, CONSTRAINT `uni_access_token_sha1` UNIQUE (`sha1`))").Error)
	} else {
		require.NoError(t, db.Exec("ALTER TABLE access_token DROP COLUMN sha256").Error)
		require.NoError(t, db.Migrator().DropIndex("access_token", "idx_access_token_user_id"))
	}
	// Gogs 0.12 created the table with GORM v1, which named the index after the
	// column.
	require.NoError(t, db.Exec("CREATE INDEX "+quote(db, "idx_access_token_uid")+" ON "+quote(db, "access_token")+" ("+quote(db, "uid")+")").Error)
	require.NoError(t, db.Migrator().DropIndex("action", "idx_action_user_id"))
	if db.Name() != "sqlite" {
		require.NoError(t, db.Migrator().AlterColumn(new(pullRequestPreV23), "MergeBase"))
		require.NoError(t, db.Migrator().AlterColumn(new(pullRequestPreV23), "MergedCommitID"))
		require.NoError(t, db.Migrator().AlterColumn(new(releasePreV23), "Sha1"))
		require.NoError(t, db.Migrator().AlterColumn(new(commentPreV23), "CommitSHA"))
	}

	legacyMigrations := map[int64]func(*gorm.DB) error{
		20: migrateAccessTokenToSHA256,
		21: addIndexToActionUserID,
		22: func(*gorm.DB) error { return nil },
		23: widenCommitIDColumns,
	}
	for v := int64(minDBVersion + 1); v <= version; v++ {
		err := legacyMigrations[v](db)
		if err != errMigrationSkipped {
			require.NoError(t, err, "legacy version %d", v)
		}
	}
	require.NoError(t, db.Migrator().CreateTable(new(Version)))
	require.NoError(t, db.Create(&Version{ID: 1, Version: version}).Error)
}

// The columns that store commit IDs before version 23 widened them.
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

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func accessTokenSHA256s(t *testing.T, db *gorm.DB) []string {
	var hashes []string
	require.NoError(t, db.Table("access_token").Order("id").Pluck("sha256", &hashes).Error)
	return hashes
}

// dumpSchema returns the columns and indexes of each table in the database, in
// an order that does not depend on the order of creation.
func dumpSchema(t *testing.T, db *gorm.DB) map[string]string {
	tables, err := db.Migrator().GetTables()
	require.NoError(t, err)

	schema := make(map[string]string, len(tables))
	for _, table := range tables {
		// The migrations keep the legacy "version" table of upgraded databases, which
		// nothing reads anymore.
		if table == "sqlite_sequence" || table == "version" {
			continue
		}

		columns, err := db.Migrator().ColumnTypes(table)
		require.NoError(t, err)
		var lines []string
		for _, c := range columns {
			typ, _ := c.ColumnType()
			nullable, _ := c.Nullable()
			def, hasDefault := c.DefaultValue()
			primaryKey, _ := c.PrimaryKey()
			unique, _ := c.Unique()
			autoIncrement, _ := c.AutoIncrement()
			line := fmt.Sprintf("column %s %s nullable=%t primary=%t unique=%t autoincrement=%t", c.Name(), strings.ToLower(typ), nullable, primaryKey, unique, autoIncrement)
			if hasDefault {
				line += fmt.Sprintf(" default=%q", def)
			}
			lines = append(lines, line)
		}

		indexes, err := db.Migrator().GetIndexes(table)
		require.NoError(t, err)
		for _, index := range indexes {
			unique, _ := index.Unique()
			primaryKey, _ := index.PrimaryKey()
			lines = append(lines, fmt.Sprintf("index %s (%s) primary=%t unique=%t", index.Name(), strings.Join(index.Columns(), ", "), primaryKey, unique))
		}
		slices.Sort(lines)
		schema[table] = strings.Join(lines, "\n")
	}
	return schema
}

// assertSameSchema compares the schema of an upgraded database with the one of
// a new installation table by table, so that a failure shows only the tables
// that differ.
func assertSameSchema(t *testing.T, want, got map[string]string) {
	t.Helper()
	tables := slices.Sorted(maps.Keys(want))
	for table := range got {
		if _, ok := want[table]; !ok {
			tables = append(tables, table)
		}
	}
	for _, table := range tables {
		assert.Equal(t, want[table], got[table], "schema differs from a new installation: table %q", table)
	}
}
