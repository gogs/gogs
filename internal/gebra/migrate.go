package gebra

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	"github.com/cockroachdb/errors"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// The migrations of each database are SQL files in migrations/<dialect>,
// named "<version>_<description>.sql", with the UTC time of writing them as
// the version in the form of YYYYMMDDHHMMSS. goose records the applied ones
// in gebra_db_version, apart from the migrations of the application that
// shares the database.
//
//go:embed migrations
var migrationFiles embed.FS

// migrate brings the engine's tables to the current version. Engines that
// start at the same time run the migrations one after the other: Postgres and
// MySQL lock the database for them, and on SQLite only one process opens it.
func (e *Engine) migrate(ctx context.Context) error {
	dir, gooseDialect := "sqlite3", goose.DialectSQLite3
	opts := []goose.ProviderOption{goose.WithDisableGlobalRegistry(true), goose.WithTableName("gebra_db_version")}
	switch e.db.dialect {
	case DialectPostgres:
		dir, gooseDialect = "postgres", goose.DialectPostgres
		locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(applierLockKey("migrations")))
		if err != nil {
			return errors.Wrap(err, "create migration lock")
		}
		opts = append(opts, goose.WithSessionLocker(locker))
	case DialectMySQL:
		dir, gooseDialect = "mysql", goose.DialectMySQL
		opts = append(opts, goose.WithSessionLocker(mysqlMigrationLocker{}))
	}
	files, err := fs.Sub(migrationFiles, "migrations/"+dir)
	if err != nil {
		return errors.Wrap(err, "open migrations")
	}
	provider, err := goose.NewProvider(gooseDialect, e.db.pool, files, opts...)
	if err != nil {
		return errors.Wrap(err, "create migration provider")
	}
	if _, err := provider.Up(ctx); err != nil {
		return errors.Wrap(err, "run migrations")
	}
	return nil
}

// mysqlMigrationLocker holds a GET_LOCK lock of the database while migrations
// run.
type mysqlMigrationLocker struct{}

// mysqlMigrationLockWait is how long, in seconds, an engine waits for another
// one's migrations.
const mysqlMigrationLockWait = 600

func (mysqlMigrationLocker) name(ctx context.Context, conn *sql.Conn) (string, error) {
	var database string
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database); err != nil {
		return "", errors.Wrap(err, "read database name")
	}
	return mysqlLockName(database, "migrations"), nil
}

func (l mysqlMigrationLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	name, err := l.name(ctx, conn)
	if err != nil {
		return err
	}
	var ok sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, mysqlMigrationLockWait).Scan(&ok); err != nil {
		return errors.Wrap(err, "lock migrations")
	}
	if ok.Int64 != 1 {
		return errors.Newf("another engine held the migration lock for %d seconds", mysqlMigrationLockWait)
	}
	return nil
}

func (l mysqlMigrationLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	name, err := l.name(ctx, conn)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", name)
	return errors.Wrap(err, "unlock migrations")
}
