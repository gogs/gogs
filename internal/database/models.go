package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/glebarez/go-sqlite"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
	"xorm.io/core"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbx"
)

// Engine represents a XORM engine or session.
type Engine interface {
	Delete(any) (int64, error)
	Exec(...any) (sql.Result, error)
	Find(any, ...any) error
	Get(any) (bool, error)
	ID(any) *xorm.Session
	In(string, ...any) *xorm.Session
	Insert(...any) (int64, error)
	InsertOne(any) (int64, error)
	Iterate(any, xorm.IterFunc) error
	Sql(string, ...any) *xorm.Session
	Table(any) *xorm.Session
	Where(any, ...any) *xorm.Session
}

var (
	x            *xorm.Engine
	legacyTables []any
)

func init() {
	// Register the pure-Go SQLite driver as "sqlite3" for XORM compatibility.
	sql.Register("sqlite3", &sqlite.Driver{})

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

	gonicNames := []string{"SSL"}
	for _, name := range gonicNames {
		core.LintGonicMapper[name] = true
	}
}

func getEngine() (*xorm.Engine, error) {
	Param := "?"
	if strings.Contains(conf.Database.Name, Param) {
		Param = "&"
	}

	driver := conf.Database.Type
	connStr := ""
	switch conf.Database.Type {
	case "mysql":
		conf.UseMySQL = true
		if conf.Database.Host[0] == '/' { // looks like a unix socket
			connStr = fmt.Sprintf("%s:%s@unix(%s)/%s%scharset=utf8mb4&parseTime=true",
				conf.Database.User, conf.Database.Password, conf.Database.Host, conf.Database.Name, Param)
		} else {
			connStr = fmt.Sprintf("%s:%s@tcp(%s)/%s%scharset=utf8mb4&parseTime=true",
				conf.Database.User, conf.Database.Password, conf.Database.Host, conf.Database.Name, Param)
		}
		engineParams := map[string]string{"rowFormat": "DYNAMIC"}
		return xorm.NewEngineWithParams(conf.Database.Type, connStr, engineParams)

	case "postgres":
		conf.UsePostgreSQL = true
		host, port := dbx.ParsePostgreSQLHostPort(conf.Database.Host)
		connStr = fmt.Sprintf("user='%s' password='%s' host='%s' port='%s' dbname='%s' sslmode='%s' search_path='%s'",
			conf.Database.User, conf.Database.Password, host, port, conf.Database.Name, conf.Database.SSLMode, conf.Database.Schema)
		driver = "pgx"

	case "sqlite3":
		if err := os.MkdirAll(path.Dir(conf.Database.Path), os.ModePerm); err != nil {
			return nil, errors.Newf("create directories: %v", err)
		}
		conf.UseSQLite3 = true
		connStr = "file:" + conf.Database.Path + "?cache=shared&mode=rwc"

	default:
		return nil, errors.Newf("unknown database type: %s", conf.Database.Type)
	}
	return xorm.NewEngine(driver, connStr)
}

func NewTestEngine() error {
	x, err := getEngine()
	if err != nil {
		return errors.Newf("connect to database: %v", err)
	}

	if conf.UsePostgreSQL {
		x.SetSchema(conf.Database.Schema)
	}

	x.SetMapper(core.GonicMapper{})
	return x.StoreEngine("InnoDB").Sync2(legacyTables...)
}

func SetEngine(logger *logx.Logger) (*gorm.DB, error) {
	var err error
	x, err = getEngine()
	if err != nil {
		return nil, errors.Newf("connect to database: %v", err)
	}

	if conf.UsePostgreSQL {
		x.SetSchema(conf.Database.Schema)
	}

	x.SetMapper(core.GonicMapper{})

	x.SetMaxOpenConns(conf.Database.MaxOpenConns)
	x.SetMaxIdleConns(conf.Database.MaxIdleConns)
	x.SetConnMaxLifetime(time.Second)

	xormLevel := core.LOG_DEBUG
	if conf.IsProdMode() {
		xormLevel = core.LOG_ERR
	}
	x.SetLogger(&xormLogger{logger: logger.Scoped("xorm"), level: xormLevel})
	x.ShowSQL(true)

	return NewConnection(logger)
}

// xormLogger sends xorm's logs through the process logger, one structured
// line per query.
type xormLogger struct {
	logger  *logx.Logger
	level   core.LogLevel
	showSQL bool
}

var _ core.ILogger = (*xormLogger)(nil)

// maxXormArgLength caps each logged query argument, so a large value such as a
// webhook payload does not fill the log.
const maxXormArgLength = 200

func (l *xormLogger) log(level core.LogLevel, msg string) {
	if level < l.level {
		return
	}
	switch level {
	case core.LOG_ERR:
		l.logger.Error(msg)
	case core.LOG_WARNING:
		l.logger.Warn(msg)
	default:
		// xorm logs queries at its info level, which is debug output for Gogs.
		l.logger.Debug(msg)
	}
}

func (l *xormLogger) Debug(v ...any) { l.log(core.LOG_DEBUG, fmt.Sprint(v...)) }
func (l *xormLogger) Debugf(format string, v ...any) {
	l.log(core.LOG_DEBUG, fmt.Sprintf(format, v...))
}
func (l *xormLogger) Error(v ...any)                 { l.log(core.LOG_ERR, fmt.Sprint(v...)) }
func (l *xormLogger) Errorf(format string, v ...any) { l.log(core.LOG_ERR, fmt.Sprintf(format, v...)) }
func (l *xormLogger) Info(v ...any)                  { l.log(core.LOG_INFO, fmt.Sprint(v...)) }
func (l *xormLogger) Warn(v ...any)                  { l.log(core.LOG_WARNING, fmt.Sprint(v...)) }
func (l *xormLogger) Warnf(format string, v ...any) {
	l.log(core.LOG_WARNING, fmt.Sprintf(format, v...))
}

func (l *xormLogger) Infof(format string, v ...any) {
	if l.level > core.LOG_INFO {
		return
	}
	// xorm logs every query as "[SQL] %v %#v" with the query and its arguments.
	if strings.HasPrefix(format, "[SQL] ") && len(v) > 0 {
		attrs := []any{"sql", v[0]}
		if args, ok := v[len(v)-1].([]any); ok && len(v) > 1 {
			attrs = append(attrs, "args", formatXormArgs(args))
		}
		l.logger.Debug("Query", attrs...)
		return
	}
	l.log(core.LOG_INFO, fmt.Sprintf(format, v...))
}

func formatXormArgs(args []any) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		s, ok := arg.(string)
		if !ok {
			parts[i] = fmt.Sprintf("%v", arg)
			continue
		}
		if len(s) > maxXormArgLength {
			s = fmt.Sprintf("%s... (%d bytes)", s[:maxXormArgLength], len(s))
		}
		parts[i] = strconv.Quote(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (l *xormLogger) Level() core.LogLevel      { return l.level }
func (l *xormLogger) SetLevel(lv core.LogLevel) { l.level = lv }

func (l *xormLogger) ShowSQL(show ...bool) {
	l.showSQL = len(show) == 0 || show[0]
}

func (l *xormLogger) IsShowSQL() bool { return l.showSQL }

func NewEngine(logger *logx.Logger) error {
	db, err := SetEngine(logger)
	if err != nil {
		return err
	}

	if err = migrations.Migrate(logger, db); err != nil {
		return errors.Newf("migrate: %v", err)
	}

	if err = x.StoreEngine("InnoDB").Sync2(legacyTables...); err != nil {
		return errors.Wrap(err, "sync tables")
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

func GetStatistic(ctx context.Context, logger *logx.Logger) (stats Statistic) {
	stats.Counter.User = Handle.Users().Count(ctx)
	stats.Counter.Org = CountOrganizations()
	stats.Counter.PublicKey, _ = x.Count(new(PublicKey))
	stats.Counter.Repo = CountRepositories(logger, true)
	stats.Counter.Watch, _ = x.Count(new(Watch))
	stats.Counter.Star, _ = x.Count(new(Star))
	stats.Counter.Action, _ = x.Count(new(Action))
	stats.Counter.Access, _ = x.Count(new(Access))
	stats.Counter.Issue, _ = x.Count(new(Issue))
	stats.Counter.Comment, _ = x.Count(new(Comment))
	stats.Counter.Oauth = 0
	stats.Counter.Follow, _ = x.Count(new(Follow))
	stats.Counter.Mirror, _ = x.Count(new(Mirror))
	stats.Counter.Release, _ = x.Count(new(Release))
	stats.Counter.LoginSource = Handle.LoginSources().Count(ctx)
	stats.Counter.Webhook, _ = x.Count(new(Webhook))
	stats.Counter.Milestone, _ = x.Count(new(Milestone))
	stats.Counter.Label, _ = x.Count(new(Label))
	stats.Counter.HookTask, _ = x.Count(new(HookTask))
	stats.Counter.Team, _ = x.Count(new(Team))
	stats.Counter.Attachment, _ = x.Count(new(Attachment))
	return stats
}

func Ping() error {
	if x == nil {
		return errors.New("database not available")
	}
	return x.Ping()
}

// The version table. Should have only one row with id==1
type Version struct {
	ID      int64
	Version int64
}
