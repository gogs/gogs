package migrations

import (
	"context"
	"database/sql"
	"embed"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

// minDBVersion is the oldest version of the legacy "version" table that can be
// migrated, i.e., the version of Gogs 0.12.
const minDBVersion = 19

// newMigrations returns the sequence of migrations to run against the given
// database. Add new migrations to the bottom of the list with the next version.
//
// Versions continue the numbering of the legacy "version" table, which the
// migrations used before they were tracked by goose in "goose_db_version". All
// migrations up to v24 either check whether their change is already in place or
// make a change that is the same when repeated, so that they also run safely on
// databases migrated by the legacy runner.
func newMigrations(logger *logx.Logger, db *gorm.DB) []*goose.Migration {
	return []*goose.Migration{
		newMigration(logger, db, 19, "create missing tables", createBaselineTables),
		newMigration(logger, db, 20, "migrate access tokens to store SHA56", migrateAccessTokenToSHA256),
		newMigration(logger, db, 21, "add index to action.user_id", addIndexToActionUserID),
		// NOTE: There was a bug in calculating the value of the legacy
		// "version.version" column after a migration is done, thus some instances are
		// on v21 but some are on v22. The noop v22 makes sure every instance runs
		// every later migration.
		newMigration(logger, db, 22, "noop", func(*gorm.DB) error { return nil }),
		newMigration(logger, db, 23, "widen commit ID columns for SHA-256", widenCommitIDColumns),
		newMigration(logger, db, 24, "add repository.is_unlisted", addRepositoryIsUnlisted),
	}
}

var errMigrationSkipped = errors.New("the migration has been skipped")

func newMigration(logger *logx.Logger, db *gorm.DB, version int64, description string, migrate func(*gorm.DB) error) *goose.Migration {
	return goose.NewGoMigration(
		version,
		&goose.GoFunc{
			// Migrations manage their own transactions through the given GORM handle.
			RunDB: func(ctx context.Context, _ *sql.DB) error {
				logger.Info("Running migration", "description", description)
				err := migrate(db.WithContext(ctx))
				if err == errMigrationSkipped {
					logger.Debug("Migration skipped", "description", description)
					return nil
				}
				return err
			},
		},
		nil,
	)
}

// Migrate migrates the database schema and/or data to the current version.
func Migrate(logger *logx.Logger, db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return errors.Wrap(err, "get underlying *sql.DB")
	}

	dialect, ok := map[string]goose.Dialect{
		"mysql":    goose.DialectMySQL,
		"postgres": goose.DialectPostgres,
		"sqlite":   goose.DialectSQLite3,
	}[db.Name()]
	if !ok {
		return errors.Newf("unsupported dialect %q", db.Name())
	}

	// goose holds one connection for the whole run, while the migrations and its
	// own bookkeeping need another one from the pool.
	if sqlDB.Stats().MaxOpenConnections == 1 {
		sqlDB.SetMaxOpenConns(2)
		defer sqlDB.SetMaxOpenConns(1)
	}

	provider, err := goose.NewProvider(
		dialect,
		sqlDB,
		nil,
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(newMigrations(logger, db)...),
	)
	if err != nil {
		return errors.Wrap(err, "new goose provider")
	}

	_, err = provider.Up(db.Statement.Context)
	if err != nil {
		return errors.Wrap(err, "run migrations")
	}
	return nil
}

// Version represents the legacy "version" table. It should have only one row
// with `id == 1`.
type Version struct {
	ID      int64
	Version int64
}

const tooOldMessage = "auto-migration from the installed version is no longer supported. " +
	"Upgrade through the last release that supports your installed version first: " +
	"https://github.com/gogs/gogs/releases/tag/v0.7.33 for versions below 0.6.0, " +
	"https://github.com/gogs/gogs/releases/tag/v0.9.141 for versions below 0.7.0, " +
	"https://github.com/gogs/gogs/releases/tag/v0.12.0 for versions below 0.11.55. " +
	"Run it once until the web server starts successfully, stop it, then upgrade to the newest version as usual."

//go:embed baseline/*.sql
var baselineFS embed.FS

var baselineTablePattern = regexp.MustCompile("^CREATE (?:TABLE|(?:UNIQUE )?INDEX \\S+ ON) [`\"]?(\\w+)")

// createBaselineTables refuses databases older than Gogs 0.12, then creates the
// tables that do not exist yet, as Gogs created them on a new installation at
// schema version 23. It never changes existing tables, whose columns later
// migrations bring up to date.
func createBaselineTables(db *gorm.DB) error {
	if db.Migrator().HasTable(new(Version)) {
		var current Version
		err := db.Where("id = ?", 1).Find(&current).Error
		if err != nil {
			return errors.Wrap(err, "get the legacy version record")
		}
		if current.ID != 0 && current.Version < minDBVersion {
			return errors.New(tooOldMessage)
		}
	}

	name := map[string]string{"mysql": "mysql", "postgres": "postgres", "sqlite": "sqlite3"}[db.Name()]
	data, err := baselineFS.ReadFile("baseline/" + name + ".sql")
	if err != nil {
		return errors.Wrap(err, "read baseline")
	}

	// Git checkouts on Windows may convert the line endings of the embedded files.
	baseline := strings.ReplaceAll(string(data), "\r\n", "\n")

	var stmts []string
	exists := make(map[string]bool)
	for _, stmt := range strings.Split(baseline, ";\n") {
		stmt = strings.TrimSpace(stmt)
		for strings.HasPrefix(stmt, "--") {
			_, stmt, _ = strings.Cut(stmt, "\n")
			stmt = strings.TrimSpace(stmt)
		}
		if stmt == "" {
			continue
		}

		m := baselineTablePattern.FindStringSubmatch(stmt)
		if m == nil {
			return errors.Newf("unrecognized baseline statement: %s", stmt)
		}
		table := m[1]
		if _, ok := exists[table]; !ok {
			exists[table] = db.Migrator().HasTable(table)
		}
		if !exists[table] {
			stmts = append(stmts, stmt)
		}
	}

	// MySQL commits each statement on its own, PostgreSQL and SQLite create all
	// tables or none.
	return db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range stmts {
			if err := tx.Exec(stmt).Error; err != nil {
				return errors.Wrapf(err, "exec %q", stmt)
			}
		}
		return nil
	})
}
