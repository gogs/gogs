package database

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
)

// SetEngine connects to the database.
func SetEngine() (*gorm.DB, error) {
	if conf.Database.Type == "sqlite3" {
		if err := os.MkdirAll(filepath.Dir(conf.Database.Path), os.ModePerm); err != nil {
			return nil, errors.Wrap(err, "create directories")
		}
	}

	w, err := newLogWriter()
	if err != nil {
		return nil, errors.Wrap(err, "new log writer")
	}
	return NewConnection(w)
}

func NewEngine() error {
	db, err := SetEngine()
	if err != nil {
		return err
	}

	if err = migrations.Migrate(db); err != nil {
		return errors.Newf("migrate: %v", err)
	}
	return nil
}

type Statistic struct {
	Counter struct {
		User, Org, PublicKey,
		Repo, Watch, Star, Action, Access,
		Issue, Comment, Oauth, Follow,
		Mirror, Release, LoginSource, Webhook,
		Milestone, Label, HookTask,
		Team, UpdateTask, Attachment int64
	}
}

func GetStatistic(ctx context.Context) (stats Statistic) {
	count := func(table any) int64 {
		var n int64
		_ = Handle.db.WithContext(ctx).Model(table).Count(&n).Error
		return n
	}

	stats.Counter.User = Handle.Users().Count(ctx)
	stats.Counter.Org = CountOrganizations()
	stats.Counter.PublicKey = count(new(PublicKey))
	stats.Counter.Repo = CountRepositories(true)
	stats.Counter.Watch = count(new(Watch))
	stats.Counter.Star = count(new(Star))
	stats.Counter.Action = count(new(Action))
	stats.Counter.Access = count(new(Access))
	stats.Counter.Issue = count(new(Issue))
	stats.Counter.Comment = count(new(Comment))
	stats.Counter.Oauth = 0
	stats.Counter.Follow = count(new(Follow))
	stats.Counter.Mirror = count(new(Mirror))
	stats.Counter.Release = count(new(Release))
	stats.Counter.LoginSource = Handle.LoginSources().Count(ctx)
	stats.Counter.Webhook = count(new(Webhook))
	stats.Counter.Milestone = count(new(Milestone))
	stats.Counter.Label = count(new(Label))
	stats.Counter.HookTask = count(new(HookTask))
	stats.Counter.Team = count(new(Team))
	stats.Counter.Attachment = count(new(Attachment))
	return stats
}

func Ping() error {
	if Handle == nil {
		return errors.New("database not available")
	}
	sqlDB, err := Handle.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
