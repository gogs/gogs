package migrations

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

const minDBVersion = 19

type Migration interface {
	Description() string
	Migrate(*gorm.DB) error
}

type migration struct {
	description string
	migrate     func(*gorm.DB) error
}

func NewMigration(desc string, fn func(*gorm.DB) error) Migration {
	return &migration{desc, fn}
}

func (m *migration) Description() string {
	return m.description
}

func (m *migration) Migrate(db *gorm.DB) error {
	return m.migrate(db)
}

// Version represents the version table. It should have only one row with `id == 1`.
type Version struct {
	ID      int64
	Version int64
}

// This is a sequence of migrations. Add new migrations to the bottom of the list.
// If you want to "retire" a migration, remove it from the top of the list and
// update _MIN_VER_DB accordingly
var migrations = []Migration{
	// v0 -> v4 : before 0.6.0 -> last support 0.7.33
	// v4 -> v10: before 0.7.0 -> last support 0.9.141
	// v10 -> v19: before 0.11.55 -> last support 0.12.0

	// Add new migration here, example:
	// v18 -> v19:v0.11.55
	// NewMigration("clean unlinked webhook and hook_tasks", cleanUnlinkedWebhookAndHookTasks),

	// v19 -> v20:v0.13.0
	NewMigration("migrate access tokens to store SHA56", migrateAccessTokenToSHA256),
	// v20 -> v21:v0.13.0
	NewMigration("add index to action.user_id", addIndexToActionUserID),
	// v21 -> v22:v0.13.0
	//
	// NOTE: There was a bug in calculating the value of the `version.version`
	// column after a migration is done, thus some instances are on v21 but some are
	// on v22. Let's make a noop v22 to make sure every instance will not miss a
	// real future migration.
	NewMigration("noop", func(*gorm.DB) error { return nil }),
	// v22 -> v23:v0.15.0
	NewMigration("widen commit ID columns for SHA-256", widenCommitIDColumns),
}

var errMigrationSkipped = errors.New("the migration has been skipped")

// Migrate migrates the database schema and/or data to the current version.
func Migrate(logger *logx.Logger, db *gorm.DB) error {
	// NOTE: GORM has problem migrating tables that happen to have columns with the
	// same name, see https://github.com/gogs/gogs/issues/7056.
	if !db.Migrator().HasTable(new(Version)) {
		err := db.AutoMigrate(new(Version))
		if err != nil {
			return errors.Wrap(err, `auto migrate "version" table`)
		}
	}

	var current Version
	err := db.Where("id = ?", 1).First(&current).Error
	if err == gorm.ErrRecordNotFound {
		err = db.Create(
			&Version{
				ID:      1,
				Version: int64(minDBVersion + len(migrations)),
			},
		).Error
		if err != nil {
			return errors.Wrap(err, "create the version record")
		}
		return nil

	} else if err != nil {
		return errors.Wrap(err, "get the version record")
	}

	if minDBVersion > current.Version {
		logger.FatalContext(context.Background(), "Auto-migration from the installed version is no longer supported",
			"dbVersion", current.Version,
			"minDBVersion", minDBVersion,
			"instructions", "Upgrade through the last release that supports your installed version first: "+
				"https://github.com/gogs/gogs/releases/tag/v0.7.33 for versions below 0.6.0, "+
				"https://github.com/gogs/gogs/releases/tag/v0.9.141 for versions below 0.7.0, "+
				"https://github.com/gogs/gogs/releases/tag/v0.12.0 for versions below 0.11.55. "+
				"Run it once until the web server starts successfully, stop it, then upgrade to the newest version as usual.",
		)
		return nil
	}

	if int(current.Version-minDBVersion) > len(migrations) {
		// User downgraded Gogs.
		current.Version = int64(len(migrations) + minDBVersion)
		return db.Where("id = ?", current.ID).Updates(current).Error
	}

	for _, m := range migrations[current.Version-minDBVersion:] {
		logger.Info("Running migration", "description", m.Description())
		if err = m.Migrate(db); err != nil {
			if err != errMigrationSkipped {
				return errors.Wrap(err, "do migrate")
			}
			logger.Debug("Migration skipped", "description", m.Description())
		}

		current.Version++
		err = db.Where("id = ?", current.ID).Updates(current).Error
		if err != nil {
			return errors.Wrap(err, "update the version record")
		}
	}
	return nil
}
