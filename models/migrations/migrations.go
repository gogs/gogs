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
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/pkg/tool"
)

const _MIN_DB_VER = 10

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
	ID      int64
	Version int64
}

// This is a sequence of migrations. Add new migrations to the bottom of the list.
// If you want to "retire" a migration, remove it from the top of the list and
// update _MIN_VER_DB accordingly
var migrations = []Migration{
	// v0 -> v4 : before 0.6.0 -> last support 0.7.33
	// v4 -> v10: before 0.7.0 -> last support 0.9.141
	NewMigration("generate rands and salt for organizations", generateOrgRandsAndSalt),           // V10 -> V11:v0.8.5
	NewMigration("convert date to unix timestamp", convertDateToUnix),                            // V11 -> V12:v0.9.2
	NewMigration("convert LDAP UseSSL option to SecurityProtocol", ldapUseSSLToSecurityProtocol), // V12 -> V13:v0.9.37

	// v13 -> v14:v0.9.87
	NewMigration("set comment updated with created", setCommentUpdatedWithCreated),
	// v14 -> v15:v0.9.147
	NewMigration("generate and migrate Git hooks", generateAndMigrateGitHooks),
	// v15 -> v16:v0.10.16
	NewMigration("update repository sizes", updateRepositorySizes),
	// v16 -> v17:v0.10.31
	NewMigration("remove invalid protect branch whitelist", removeInvalidProtectBranchWhitelist),
}

// Migrate database to current version
func Migrate(x *xorm.Engine) error {
	if err := x.Sync(new(Version)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	currentVersion := &Version{ID: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return fmt.Errorf("get: %v", err)
	} else if !has {
		// If the version record does not exist we think
		// it is a fresh installation and we can skip all migrations.
		currentVersion.ID = 0
		currentVersion.Version = int64(_MIN_DB_VER + len(migrations))

		if _, err = x.InsertOne(currentVersion); err != nil {
			return fmt.Errorf("insert: %v", err)
		}
	}

	v := currentVersion.Version
	if _MIN_DB_VER > v {
		log.Fatal(0, `
Hi there, thank you for using Gogs for so long!
However, Gogs has stopped supporting auto-migration from your previously installed version.
But the good news is, it's very easy to fix this problem!
You can migrate your older database using a previous release, then you can upgrade to the newest version.

Please save following instructions to somewhere and start working:

- If you were using below 0.6.0 (e.g. 0.5.x), download last supported archive from following link:
	https://github.com/gogs/gogs/releases/tag/v0.7.33
- If you were using below 0.7.0 (e.g. 0.6.x), download last supported archive from following link:
	https://github.com/gogs/gogs/releases/tag/v0.9.141

Once finished downloading,

1. Extract the archive and to upgrade steps as usual.
2. Run it once. To verify, you should see some migration traces.
3. Once it starts web server successfully, stop it.
4. Now it's time to put back the release archive you originally intent to upgrade.
5. Enjoy!

In case you're stilling getting this notice, go through instructions again until it disappears.`)
		return nil
	}

	if int(v-_MIN_DB_VER) > len(migrations) {
		// User downgraded Gogs.
		currentVersion.Version = int64(len(migrations) + _MIN_DB_VER)
		_, err = x.Id(1).Update(currentVersion)
		return err
	}
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

func generateOrgRandsAndSalt(x *xorm.Engine) (err error) {
	type User struct {
		ID    int64  `xorm:"pk autoincr"`
		Rands string `xorm:"VARCHAR(10)"`
		Salt  string `xorm:"VARCHAR(10)"`
	}

	orgs := make([]*User, 0, 10)
	if err = x.Where("type=1").And("rands=''").Find(&orgs); err != nil {
		return fmt.Errorf("select all organizations: %v", err)
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for _, org := range orgs {
		if org.Rands, err = tool.RandomString(10); err != nil {
			return err
		}
		if org.Salt, err = tool.RandomString(10); err != nil {
			return err
		}
		if _, err = sess.Id(org.ID).Update(org); err != nil {
			return err
		}
	}

	return sess.Commit()
}

type TAction struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
}

func (t *TAction) TableName() string { return "action" }

type TNotice struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
}

func (t *TNotice) TableName() string { return "notice" }

type TComment struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
}

func (t *TComment) TableName() string { return "comment" }

type TIssue struct {
	ID           int64 `xorm:"pk autoincr"`
	DeadlineUnix int64
	CreatedUnix  int64
	UpdatedUnix  int64
}

func (t *TIssue) TableName() string { return "issue" }

type TMilestone struct {
	ID             int64 `xorm:"pk autoincr"`
	DeadlineUnix   int64
	ClosedDateUnix int64
}

func (t *TMilestone) TableName() string { return "milestone" }

type TAttachment struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
}

func (t *TAttachment) TableName() string { return "attachment" }

type TLoginSource struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TLoginSource) TableName() string { return "login_source" }

type TPull struct {
	ID         int64 `xorm:"pk autoincr"`
	MergedUnix int64
}

func (t *TPull) TableName() string { return "pull_request" }

type TRelease struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
}

func (t *TRelease) TableName() string { return "release" }

type TRepo struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TRepo) TableName() string { return "repository" }

type TMirror struct {
	ID             int64 `xorm:"pk autoincr"`
	UpdatedUnix    int64
	NextUpdateUnix int64
}

func (t *TMirror) TableName() string { return "mirror" }

type TPublicKey struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TPublicKey) TableName() string { return "public_key" }

type TDeployKey struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TDeployKey) TableName() string { return "deploy_key" }

type TAccessToken struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TAccessToken) TableName() string { return "access_token" }

type TUser struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TUser) TableName() string { return "user" }

type TWebhook struct {
	ID          int64 `xorm:"pk autoincr"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (t *TWebhook) TableName() string { return "webhook" }

func convertDateToUnix(x *xorm.Engine) (err error) {
	log.Info("This migration could take up to minutes, please be patient.")
	type Bean struct {
		ID         int64 `xorm:"pk autoincr"`
		Created    time.Time
		Updated    time.Time
		Merged     time.Time
		Deadline   time.Time
		ClosedDate time.Time
		NextUpdate time.Time
	}

	var tables = []struct {
		name string
		cols []string
		bean interface{}
	}{
		{"action", []string{"created"}, new(TAction)},
		{"notice", []string{"created"}, new(TNotice)},
		{"comment", []string{"created"}, new(TComment)},
		{"issue", []string{"deadline", "created", "updated"}, new(TIssue)},
		{"milestone", []string{"deadline", "closed_date"}, new(TMilestone)},
		{"attachment", []string{"created"}, new(TAttachment)},
		{"login_source", []string{"created", "updated"}, new(TLoginSource)},
		{"pull_request", []string{"merged"}, new(TPull)},
		{"release", []string{"created"}, new(TRelease)},
		{"repository", []string{"created", "updated"}, new(TRepo)},
		{"mirror", []string{"updated", "next_update"}, new(TMirror)},
		{"public_key", []string{"created", "updated"}, new(TPublicKey)},
		{"deploy_key", []string{"created", "updated"}, new(TDeployKey)},
		{"access_token", []string{"created", "updated"}, new(TAccessToken)},
		{"user", []string{"created", "updated"}, new(TUser)},
		{"webhook", []string{"created", "updated"}, new(TWebhook)},
	}

	for _, table := range tables {
		log.Info("Converting table: %s", table.name)
		if err = x.Sync2(table.bean); err != nil {
			return fmt.Errorf("Sync [table: %s]: %v", table.name, err)
		}

		offset := 0
		for {
			beans := make([]*Bean, 0, 100)
			if err = x.Sql(fmt.Sprintf("SELECT * FROM `%s` ORDER BY id ASC LIMIT 100 OFFSET %d",
				table.name, offset)).Find(&beans); err != nil {
				return fmt.Errorf("select beans [table: %s, offset: %d]: %v", table.name, offset, err)
			}
			log.Trace("Table [%s]: offset: %d, beans: %d", table.name, offset, len(beans))
			if len(beans) == 0 {
				break
			}
			offset += 100

			baseSQL := "UPDATE `" + table.name + "` SET "
			for _, bean := range beans {
				valSQLs := make([]string, 0, len(table.cols))
				for _, col := range table.cols {
					fieldSQL := ""
					fieldSQL += col + "_unix = "

					switch col {
					case "deadline":
						if bean.Deadline.IsZero() {
							continue
						}
						fieldSQL += com.ToStr(bean.Deadline.Unix())
					case "created":
						fieldSQL += com.ToStr(bean.Created.Unix())
					case "updated":
						fieldSQL += com.ToStr(bean.Updated.Unix())
					case "closed_date":
						fieldSQL += com.ToStr(bean.ClosedDate.Unix())
					case "merged":
						fieldSQL += com.ToStr(bean.Merged.Unix())
					case "next_update":
						fieldSQL += com.ToStr(bean.NextUpdate.Unix())
					}

					valSQLs = append(valSQLs, fieldSQL)
				}

				if len(valSQLs) == 0 {
					continue
				}

				if _, err = x.Exec(baseSQL + strings.Join(valSQLs, ",") + " WHERE id = " + com.ToStr(bean.ID)); err != nil {
					return fmt.Errorf("update bean [table: %s, id: %d]: %v", table.name, bean.ID, err)
				}
			}
		}
	}

	return nil
}
