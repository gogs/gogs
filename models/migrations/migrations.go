package migrations

import (
	"errors"
	"time"

	"github.com/go-xorm/xorm"
)

type migration func(*xorm.Engine) error

// The version table. Should have only one row with id==1
type Version struct {
	Id      int64
	Version int64
}

// This is a sequence of migrations. Add new migrations to the bottom of the list.
// If you want to "retire" a migration, replace it with "expiredMigration"
var migrations = []migration{
	prepareToCommitComments,
}

// Migrate database to current version
func Migrate(x *xorm.Engine) error {
	if err := x.Sync(new(Version)); err != nil {
		return err
	}

	currentVersion := &Version{Id: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return err
	} else if !has {
		if _, err = x.InsertOne(currentVersion); err != nil {
			return err
		}
	}

	v := currentVersion.Version

	for i, migration := range migrations[v:] {
		if err = migration(x); err != nil {
			return err
		}
		currentVersion.Version = v + int64(i) + 1
		if _, err = x.Id(1).Update(currentVersion); err != nil {
			return err
		}
	}
	return nil
}

func expiredMigration(x *xorm.Engine) error {
	return errors.New("You are migrating from a too old gogs version")
}

func prepareToCommitComments(x *xorm.Engine) error {
	type Comment struct {
		Id       int64
		Type     int64
		PosterId int64
		IssueId  int64
		CommitId string
		Line     string
		Content  string    `xorm:"TEXT"`
		Created  time.Time `xorm:"CREATED"`
	}
	x.Sync(new(Comment))

	sql := `UPDATE comment SET commit_id = '', line = '' WHERE commit_id = '0' AND line = '0'`
	_, err := x.Exec(sql)
	if err != nil {
		return err
	}
	return nil
}
