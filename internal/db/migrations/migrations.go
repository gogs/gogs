// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"github.com/pkg/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
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
}

// Migrate migrates the database schema and/or data to the current version.
func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(new(Version))
	if err != nil {
		return errors.Wrap(err, `auto migrate "version" table`)
	}

	var current Version
	err = db.Where("id = ?", 1).First(&current).Error
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
		log.Fatal(`
Hi there, thank you for using Gogs for so long!
However, Gogs has stopped supporting auto-migration from your previously installed version.
But the good news is, it's very easy to fix this problem!
You can migrate your older database using a previous release, then you can upgrade to the newest version.

Please save following instructions to somewhere and start working:

- If you were using below 0.6.0 (e.g. 0.5.x), download last supported archive from following link:
	https://gogs.io/gogs/releases/tag/v0.7.33
- If you were using below 0.7.0 (e.g. 0.6.x), download last supported archive from following link:
	https://gogs.io/gogs/releases/tag/v0.9.141
- If you were using below 0.11.55 (e.g. 0.9.141), download last supported archive from following link:
	https://gogs.io/gogs/releases/tag/v0.12.0

Once finished downloading:

1. Extract the archive and to upgrade steps as usual.
2. Run it once. To verify, you should see some migration traces.
3. Once it starts web server successfully, stop it.
4. Now it's time to put back the release archive you originally intent to upgrade.
5. Enjoy!

In case you're stilling getting this notice, go through instructions again until it disappears.`)
		return nil
	}

	if int(current.Version-minDBVersion) > len(migrations) {
		// User downgraded Gogs.
		current.Version = int64(len(migrations) + minDBVersion)
		return db.Where("id = ?", current.ID).Updates(current).Error
	}

	for i, m := range migrations[current.Version-minDBVersion:] {
		log.Info("Migration: %s", m.Description())
		if err = m.Migrate(db); err != nil {
			return errors.Wrap(err, "do migrate")
		}

		current.Version += int64(i) + 1
		err = db.Where("id = ?", current.ID).Updates(current).Error
		if err != nil {
			return errors.Wrap(err, "update the version record")
		}
	}
	return nil
}
