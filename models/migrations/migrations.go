// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const _MIN_DB_VER = 0

type Migration interface {
	Description() string
	Migrate(*xorm.Engine) error
}

type migration struct {
	description string
	migrate     func(*xorm.Engine) error
}

func NewMigration(desc string, fn func(*xorm.Engine) error) Migration {
	return &migration{desc, fn}
}

func (m *migration) Description() string {
	return m.description
}

func (m *migration) Migrate(x *xorm.Engine) error {
	return m.migrate(x)
}

// The version table. Should have only one row with id==1
type Version struct {
	Id      int64
	Version int64
}

// This is a sequence of migrations. Add new migrations to the bottom of the list.
// If you want to "retire" a migration, remove it from the top of the list and
// update _MIN_VER_DB accordingly
var migrations = []Migration{
	NewMigration("generate collaboration from access", accessToCollaboration), // V0 -> V1
}

// Migrate database to current version
func Migrate(x *xorm.Engine) error {
	if err := x.Sync(new(Version)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	currentVersion := &Version{Id: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return fmt.Errorf("get: %v", err)
	} else if !has {
		// If the user table does not exist it is a fresh installation and we
		// can skip all migrations.
		needsMigration, err := x.IsTableExist("user")
		if err != nil {
			return err
		}
		if needsMigration {
			isEmpty, err := x.IsTableEmpty("user")
			if err != nil {
				return err
			}
			// If the user table is empty it is a fresh installation and we can
			// skip all migrations.
			needsMigration = !isEmpty
		}
		if !needsMigration {
			currentVersion.Version = int64(_MIN_DB_VER + len(migrations))
		}

		if _, err = x.InsertOne(currentVersion); err != nil {
			return fmt.Errorf("insert: %v", err)
		}
	}

	v := currentVersion.Version
	for i, m := range migrations[v-_MIN_DB_VER:] {
		log.Info("Migration: %s", m.Description())
		if err = m.Migrate(x); err != nil {
			return fmt.Errorf("do migrate: %v", err)
		}
		currentVersion.Version = v + int64(i) + 1
		if _, err = x.Id(1).Update(currentVersion); err != nil {
			return err
		}
	}
	return nil
}

func sessionRelease(sess *xorm.Session) {
	if !sess.IsCommitedOrRollbacked {
		sess.Rollback()
	}
	sess.Close()
}

func accessToCollaboration(x *xorm.Engine) error {
	type Collaboration struct {
		ID      int64 `xorm:"pk autoincr"`
		RepoID  int64 `xorm:"UNIQUE(s) INDEX NOT NULL"`
		UserID  int64 `xorm:"UNIQUE(s) INDEX NOT NULL"`
		Created time.Time
	}

	x.Sync(new(Collaboration))

	results, err := x.Query("SELECT u.id AS `uid`, a.repo_name AS `repo`, a.mode AS `mode`, a.created as `created` FROM `access` a JOIN `user` u ON a.user_name=u.lower_name")
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	offset := strings.Split(time.Now().String(), " ")[2]
	for _, result := range results {
		mode := com.StrTo(result["mode"]).MustInt64()
		// Collaborators must have write access.
		if mode < 2 {
			continue
		}

		userID := com.StrTo(result["uid"]).MustInt64()
		repoRefName := string(result["repo"])

		var created time.Time
		switch {
		case setting.UseSQLite3:
			created, _ = time.Parse(time.RFC3339, string(result["created"]))
		case setting.UseMySQL:
			created, _ = time.Parse("2006-01-02 15:04:05-0700", string(result["created"])+offset)
		case setting.UsePostgreSQL:
			created, _ = time.Parse("2006-01-02T15:04:05Z-0700", string(result["created"])+offset)
		}

		// find owner of repository
		parts := strings.SplitN(repoRefName, "/", 2)
		ownerName := parts[0]
		repoName := parts[1]

		results, err := sess.Query("SELECT u.id as `uid`, ou.uid as `memberid` FROM `user` u LEFT JOIN org_user ou ON ou.org_id=u.id WHERE u.lower_name=?", ownerName)
		if err != nil {
			return err
		}
		if len(results) < 1 {
			continue
		}

		ownerID := com.StrTo(results[0]["uid"]).MustInt64()
		if ownerID == userID {
			continue
		}

		// test if user is member of owning organization
		isMember := false
		for _, member := range results {
			memberID := com.StrTo(member["memberid"]).MustInt64()
			// We can skip all cases that a user is member of the owning organization
			if memberID == userID {
				isMember = true
			}
		}
		if isMember {
			continue
		}

		results, err = sess.Query("SELECT id FROM `repository` WHERE owner_id=? AND lower_name=?", ownerID, repoName)
		if err != nil {
			return err
		} else if len(results) < 1 {
			continue
		}

		collaboration := &Collaboration{
			UserID: userID,
			RepoID: com.StrTo(results[0]["id"]).MustInt64(),
		}
		has, err := sess.Get(collaboration)
		if err != nil {
			return err
		} else if has {
			continue
		}

		collaboration.Created = created
		if _, err = sess.InsertOne(collaboration); err != nil {
			return err
		}
	}

	return sess.Commit()
}
