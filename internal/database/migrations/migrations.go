package migrations

import (
	"context"
	"database/sql"
	"embed"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
)

// minDBVersion is the oldest version of the legacy "version" table that can be
// migrated, i.e., the version of Gogs 0.12.
const minDBVersion = 19

// newMigrations returns the sequence of migrations to run against the given
// database. Add new migrations to the bottom of the list with the next version.
//
// Versions continue the numbering of the legacy "version" table, which the
// migrations used before they were tracked by goose in "goose_db_version". All
// migrations up to v23 check whether their change is already in place, so that
// they also run safely on databases migrated by the legacy runner.
func newMigrations(db *gorm.DB) []*goose.Migration {
	return []*goose.Migration{
		newMigration(db, 19, "create missing tables", createBaselineTables),
		newMigration(db, 20, "migrate access tokens to store SHA56", migrateAccessTokenToSHA256),
		newMigration(db, 21, "add index to action.user_id", addIndexToActionUserID),
		// NOTE: There was a bug in calculating the value of the legacy
		// "version.version" column after a migration is done, thus some instances are
		// on v21 but some are on v22. The noop v22 makes sure every instance runs
		// every later migration.
		newMigration(db, 22, "noop", func(*gorm.DB) error { return nil }),
		newMigration(db, 23, "add repository.is_unlisted", addRepositoryIsUnlisted),
	}
}

var errMigrationSkipped = errors.New("the migration has been skipped")

func newMigration(db *gorm.DB, version int64, description string, migrate func(*gorm.DB) error) *goose.Migration {
	return goose.NewGoMigration(
		version,
		&goose.GoFunc{
			// Migrations manage their own transactions through the given GORM handle.
			RunDB: func(ctx context.Context, _ *sql.DB) error {
				log.Info("Migration: %s", description)
				err := migrate(db.WithContext(ctx))
				if err == errMigrationSkipped {
					log.Trace("The migration %q has been skipped", description)
					return nil
				}
				return err
			},
		},
		nil,
	)
}

// Migrate migrates the database schema and/or data to the current version.
func Migrate(db *gorm.DB) error {
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

	opts := []goose.ProviderOption{
		goose.WithDisableGlobalRegistry(true),
		goose.WithGoMigrations(newMigrations(db)...),
	}
	if dialect == goose.DialectPostgres {
		store, err := database.NewStore(dialect, goose.DefaultTablename)
		if err != nil {
			return errors.Wrap(err, "new goose store")
		}
		dialect = goose.DialectCustom
		opts = append(opts, goose.WithStore(postgresStore{StoreExtender: store.(database.StoreExtender)}))
	}

	provider, err := goose.NewProvider(dialect, sqlDB, nil, opts...)
	if err != nil {
		return errors.Wrap(err, "new goose provider")
	}

	_, err = provider.Up(context.Background())
	if err != nil {
		return errors.Wrap(err, "run migrations")
	}
	return nil
}

// postgresStore creates the goose version table with a serial ID instead of
// the identity column goose uses, which requires PostgreSQL 10 while Gogs
// supports PostgreSQL 9.6.
type postgresStore struct {
	database.StoreExtender
}

func (s postgresStore) CreateVersionTable(ctx context.Context, db database.DBTxConn) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE `+s.Tablename()+` (
		id serial PRIMARY KEY,
		version_id bigint NOT NULL,
		is_applied boolean NOT NULL,
		tstamp timestamp NOT NULL DEFAULT now()
	)`)
	return err
}

// Version represents the legacy "version" table. It should have only one row
// with `id == 1`.
type Version struct {
	ID      int64
	Version int64
}

const tooOldMessage = `
Hi there, thank you for using Gogs for so long!
However, Gogs has stopped supporting auto-migration from your previously installed version.
But the good news is, it's very easy to fix this problem!
You can migrate your older database using a previous release, then you can upgrade to the newest version.

Please save following instructions to somewhere and start working:

- If you were using below 0.6.0 (e.g. 0.5.x), download last supported archive from following link:
	https://github.com/gogs/gogs/releases/tag/v0.7.33
- If you were using below 0.7.0 (e.g. 0.6.x), download last supported archive from following link:
	https://github.com/gogs/gogs/releases/tag/v0.9.141
- If you were using below 0.11.55 (e.g. 0.9.141), download last supported archive from following link:
	https://github.com/gogs/gogs/releases/tag/v0.12.0

Once finished downloading:

1. Extract the archive and to upgrade steps as usual.
2. Run it once. To verify, you should see some migration traces.
3. Once it starts web server successfully, stop it.
4. Now it's time to put back the release archive you originally intent to upgrade.
5. Enjoy!

In case you're stilling getting this notice, go through instructions again until it disappears.`

//go:embed baseline/*.sql
var baselineFS embed.FS

var baselineTablePattern = regexp.MustCompile("^CREATE (?:TABLE|(?:UNIQUE )?INDEX \\S+ ON) [`\"]?(\\w+)")

// createBaselineTables refuses databases older than Gogs 0.12, then creates the
// tables that do not exist yet, as Gogs created them on a new installation at
// schema version 22. It never changes existing tables, whose columns later
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

	var stmts []string
	exists := make(map[string]bool)
	for _, stmt := range strings.Split(string(data), ";\n") {
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
