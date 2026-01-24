package database

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbutil"
)

var (
	db           *gorm.DB
	legacyTables []any
	HasEngine    bool
)

func init() {
	legacyTables = append(legacyTables,
		new(User), new(PublicKey), new(TwoFactor), new(TwoFactorRecoveryCode),
		new(Repository), new(DeployKey), new(Collaboration), new(Upload),
		new(Watch), new(Star),
		new(Issue), new(PullRequest), new(Comment), new(Attachment), new(IssueUser),
		new(Label), new(IssueLabel), new(Milestone),
		new(Mirror), new(Release), new(Webhook), new(HookTask),
		new(ProtectBranch), new(ProtectBranchWhitelist),
		new(Team), new(OrgUser), new(TeamUser), new(TeamRepo),
	)
}

func getGormDB(gormLogger logger.Writer) (*gorm.DB, error) {
	if conf.Database.Type == "sqlite3" {
		if err := os.MkdirAll(path.Dir(conf.Database.Path), os.ModePerm); err != nil {
			return nil, errors.Newf("create directories: %v", err)
		}
	}

	level := logger.Info
	if conf.IsProdMode() {
		level = logger.Warn
	}

	logger.Default = logger.New(gormLogger, logger.Config{
		SlowThreshold: 100 * time.Millisecond,
		LogLevel:      level,
	})

	gormDB, err := dbutil.OpenDB(
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

	sqlDB, err := gormDB.DB()
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
		gormDB = gormDB.Set("gorm:table_options", "ENGINE=InnoDB").Session(&gorm.Session{})
	case "sqlite3":
		conf.UseSQLite3 = true
	case "mssql":
		conf.UseMSSQL = true
	}

	return gormDB, nil
}

func NewTestEngine() error {
	var err error
	db, err = getGormDB(&dbutil.Logger{Writer: os.Stdout})
	if err != nil {
		return errors.Newf("connect to database: %v", err)
	}

	for _, table := range legacyTables {
		if db.Migrator().HasTable(table) {
			continue
		}
		if err = db.Migrator().AutoMigrate(table); err != nil {
			return errors.Wrap(err, "auto migrate")
		}
	}
	return nil
}

func SetEngine() (*gorm.DB, error) {
	var logPath string
	if conf.HookMode {
		logPath = filepath.Join(conf.Log.RootPath, "hooks", "gorm.log")
	} else {
		logPath = filepath.Join(conf.Log.RootPath, "gorm.log")
	}
	sec := conf.File.Section("log.gorm")
	fileWriter, err := log.NewFileWriter(logPath,
		log.FileRotationConfig{
			Rotate:  sec.Key("ROTATE").MustBool(true),
			Daily:   sec.Key("ROTATE_DAILY").MustBool(true),
			MaxSize: sec.Key("MAX_SIZE").MustInt64(100) * 1024 * 1024,
			MaxDays: sec.Key("MAX_DAYS").MustInt64(3),
		},
	)
	if err != nil {
		return nil, errors.Newf("create 'gorm.log': %v", err)
	}

	var gormLogger logger.Writer
	if conf.HookMode {
		gormLogger = &dbutil.Logger{Writer: fileWriter}
	} else {
		gormLogger, err = newLogWriter()
		if err != nil {
			return nil, errors.Wrap(err, "new log writer")
		}
	}

	db, err = getGormDB(gormLogger)
	if err != nil {
		return nil, err
	}

	return NewConnection(gormLogger)
}

func NewEngine() error {
	gormDB, err := SetEngine()
	if err != nil {
		return err
	}

	if err = migrations.Migrate(gormDB); err != nil {
		return errors.Newf("migrate: %v", err)
	}

	for _, table := range legacyTables {
		if gormDB.Migrator().HasTable(table) {
			continue
		}
		name := strings.TrimPrefix(fmt.Sprintf("%T", table), "*database.")
		if err = gormDB.Migrator().AutoMigrate(table); err != nil {
			return errors.Wrapf(err, "auto migrate %q", name)
		}
		log.Trace("Auto migrated %q", name)
	}

	HasEngine = true
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
	stats.Counter.User = Handle.Users().Count(ctx)
	stats.Counter.Org = CountOrganizations()
	var count int64
	db.Model(new(PublicKey)).Count(&count)
	stats.Counter.PublicKey = count
	stats.Counter.Repo = CountRepositories(true)
	db.Model(new(Watch)).Count(&count)
	stats.Counter.Watch = count
	db.Model(new(Star)).Count(&count)
	stats.Counter.Star = count
	db.Model(new(Action)).Count(&count)
	stats.Counter.Action = count
	db.Model(new(Access)).Count(&count)
	stats.Counter.Access = count
	db.Model(new(Issue)).Count(&count)
	stats.Counter.Issue = count
	db.Model(new(Comment)).Count(&count)
	stats.Counter.Comment = count
	stats.Counter.Oauth = 0
	db.Model(new(Follow)).Count(&count)
	stats.Counter.Follow = count
	db.Model(new(Mirror)).Count(&count)
	stats.Counter.Mirror = count
	db.Model(new(Release)).Count(&count)
	stats.Counter.Release = count
	stats.Counter.LoginSource = Handle.LoginSources().Count(ctx)
	db.Model(new(Webhook)).Count(&count)
	stats.Counter.Webhook = count
	db.Model(new(Milestone)).Count(&count)
	stats.Counter.Milestone = count
	db.Model(new(Label)).Count(&count)
	stats.Counter.Label = count
	db.Model(new(HookTask)).Count(&count)
	stats.Counter.HookTask = count
	db.Model(new(Team)).Count(&count)
	stats.Counter.Team = count
	db.Model(new(Attachment)).Count(&count)
	stats.Counter.Attachment = count
	return stats
}

func Ping() error {
	if db == nil {
		return errors.New("database not available")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// The version table. Should have only one row with id==1
type Version struct {
	ID      int64
	Version int64
}
