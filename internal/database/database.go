package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

// Tables is the list of struct-to-table mappings.
//
// NOTE: Lines are sorted in alphabetical order, each letter in its own line.
//
// ⚠️ WARNING: This list is meant to be read-only.
var Tables = []any{
	new(Access), new(AccessToken), new(Action),
	new(EmailAddress),
	new(Follow),
	new(LFSObject), new(LoginSource),
	new(Notice),
}

// NewConnection returns a new database connection with the given logger.
func NewConnection(logger *logx.Logger) (*gorm.DB, error) {
	level := gormlogger.Info
	if conf.IsProdMode() {
		level = gormlogger.Warn
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	gormlogger.Default = dbx.NewLogger(logger.Scoped("gorm"), level, 100*time.Millisecond)

	db, err := dbx.OpenDB(
		conf.Database,
		&gorm.Config{
			SkipDefaultTransaction: true,
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			NowFunc: func() time.Time {
				return time.Now().UTC().Truncate(time.Microsecond)
			},
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "open database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "get underlying *sql.DB")
	}
	sqlDB.SetMaxOpenConns(conf.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(conf.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Minute)

	switch conf.Database.Type {
	case "postgres":
		conf.UsePostgreSQL = true
	case "mysql":
		conf.UseMySQL = true
		db = db.Set("gorm:table_options", "ENGINE=InnoDB").Session(&gorm.Session{})
	case "sqlite3":
		conf.UseSQLite3 = true
	default:
		panic("unreachable")
	}

	// NOTE: GORM has problem detecting existing columns, see
	// https://github.com/gogs/gogs/issues/6091. Therefore, only use it to create new
	// tables, and do customize migration with future changes.
	for _, table := range Tables {
		if db.Migrator().HasTable(table) {
			continue
		}

		name := strings.TrimPrefix(fmt.Sprintf("%T", table), "*database.")
		err = db.Migrator().AutoMigrate(table)
		if err != nil {
			return nil, errors.Wrapf(err, "auto migrate %q", name)
		}
		logger.Debug("Auto migrated table", "tableName", name)
	}

	loadedLoginSourceFilesStore, err = loadLoginSourceFiles(filepath.Join(conf.CustomDir(), "conf", "auth.d"), db.NowFunc)
	if err != nil {
		return nil, errors.Wrap(err, "load login source files")
	}

	// Initialize the database handle.
	Handle = &DB{db: db, logger: logger}
	return db, nil
}

// DB is the database handler for the storage layer.
type DB struct {
	db *gorm.DB
	// logger is for methods whose signatures are fixed by xorm hooks or
	// templates, so they cannot receive a logger from their caller.
	logger *logx.Logger
}

// handleLogger returns the logger of the global database handle. Only methods
// whose signatures are fixed by xorm hooks or templates use it, because their
// callers cannot pass a logger. The handle is nil during installation mode and
// in tests, which get a no-op logger.
func handleLogger() *logx.Logger {
	if Handle == nil || Handle.logger == nil {
		return logx.NewNoopLogger()
	}
	return Handle.logger
}

// Handle is the global database handle. It could be `nil` during the
// installation mode.
//
// NOTE: Because we need to register all the routes even during the installation
// mode (which initially has no database configuration), we have to use a global
// variable since we can't pass a database handler around before it's available.
//
// NOTE: It is not guarded by a mutex because it is only written once either
// during the service start or during the installation process (which is a
// single-thread process).
var Handle *DB

func (db *DB) AccessTokens() *AccessTokensStore {
	return newAccessTokensStore(db.db)
}

func (db *DB) Actions() *ActionsStore {
	return newActionsStore(db.db)
}

func (db *DB) LFS() *LFSStore {
	return newLFSStore(db.db)
}

// NOTE: It is not guarded by a mutex because it only gets written during the
// service start.
var loadedLoginSourceFilesStore loginSourceFilesStore

func (db *DB) LoginSources() *LoginSourcesStore {
	return newLoginSourcesStore(db.db, loadedLoginSourceFilesStore)
}

func (db *DB) Notices() *NoticesStore {
	return newNoticesStore(db.db)
}

func (db *DB) Organizations() *OrganizationsStore {
	return newOrganizationsStoreStore(db.db)
}

func (db *DB) Permissions() *PermissionsStore {
	return newPermissionsStore(db.db)
}

func (db *DB) PublicKey() *PublicKeysStore {
	return newPublicKeysStore(db.db)
}

func (db *DB) Repositories() *RepositoriesStore {
	return newReposStore(db.db)
}

func (db *DB) TwoFactors() *TwoFactorsStore {
	return newTwoFactorsStore(db.db)
}

func (db *DB) Users() *UsersStore {
	return newUsersStore(db.db)
}
