package migrations

import (
	"errors"
	"strconv"
	"strings"
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
	accessToCollaboration,
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
		needsMigration, err := x.IsTableExist("user")
		if err != nil {
			return err
		}
		if needsMigration {
			isEmpty, err := x.IsTableEmpty("user")
			if err != nil {
				return err
			}
			needsMigration = !isEmpty
		}
		if !needsMigration {
			currentVersion.Version = int64(len(migrations))
		}

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

func mustParseInt64(in []byte) int64 {
	i, err := strconv.ParseInt(string(in), 10, 64)
	if err != nil {
		i = 0
	}
	return i
}

func accessToCollaboration(x *xorm.Engine) error {
	type Collaboration struct {
		ID      int64     `xorm:"pk autoincr"`
		RepoID  int64     `xorm:"UNIQUE(s) INDEX NOT NULL"`
		UserID  int64     `xorm:"UNIQUE(s) INDEX NOT NULL"`
		Created time.Time `xorm:"CREATED"`
	}

	x.Sync(new(Collaboration))

	sql := `SELECT u.id AS uid, a.repo_name AS repo, a.mode AS mode FROM access a JOIN user u ON a.user_name=u.lower_name`
	results, err := x.Query(sql)
	if err != nil {
		return err
	}

	for _, result := range results {
		userID := mustParseInt64(result["uid"])
		repoRefName := string(result["repo"])
		mode := mustParseInt64(result["mode"])

		//Collaborators must have write access
		if mode < 2 {
			continue
		}

		parts := strings.SplitN(repoRefName, "/", 2)
		ownerName := parts[0]
		repoName := parts[1]

		sql = `SELECT u.id as uid, ou.uid as memberid FROM user u LEFT JOIN org_user ou ON ou.org_id=u.id WHERE u.lower_name=?`
		results, err := x.Query(sql, ownerName)
		if err != nil {
			return err
		}
		if len(results) < 1 {
			continue
		}
		ownerID := mustParseInt64(results[0]["uid"])

		for _, member := range results {
			memberID := mustParseInt64(member["memberid"])
			// We can skip all cases that a user is member of the owning organization
			if memberID == userID {
				continue
			}
		}

		sql = `SELECT id FROM repository WHERE owner_id=? AND lower_name=?`
		results, err = x.Query(sql, ownerID, repoName)
		if err != nil {
			return err
		}
		if len(results) < 1 {
			continue
		}

		repoID := results[0]["id"]

		sql = `INSERT INTO collaboration (user_id, repo_id) VALUES (?,?)`
		_, err = x.Exec(sql, userID, repoID)
		if err != nil {
			return err
		}
	}
	return nil
}
