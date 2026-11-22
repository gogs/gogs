package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbx"
)

// Tables is the list of struct-to-table mappings.
//
// NOTE: Lines are sorted in alphabetical order, each letter in its own line.
//
// ⚠️ WARNING: This list is meant to be read-only.
var Tables = []any{
	new(Access), new(AccessToken), new(Action), new(Attachment),
	new(Collaboration), new(Comment),
	new(DeployKey),
	new(EmailAddress),
	new(Follow),
	new(HookTask),
	new(Issue), new(IssueLabel), new(IssueUser),
	new(Label), new(LFSObject), new(LoginSource),
	new(Milestone), new(Mirror),
	new(Notice),
	new(OrgUser),
	new(Passkey), new(ProtectBranch), new(PublicKey), new(PullRequest),
	new(Release), new(Repository),
	new(Star),
	new(Team), new(TeamRepo), new(TeamUser), new(TwoFactor), new(TwoFactorRecoveryCode),
	new(Upload), new(User),
	new(Watch), new(Webhook),
}

// NewConnection returns a new database connection with the given logger. When
// logQueries is true, every query is logged at the debug level. Otherwise only
// failed and slow queries are.
func NewConnection(logger *logx.Logger, logQueries bool) (*gorm.DB, error) {
	level := gormlogger.Warn
	if logQueries {
		level = gormlogger.Info
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
	if err = db.Use(tracingPlugin{}); err != nil {
		return nil, errors.Wrap(err, "use tracing plugin")
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
		if statementBinlog = usesStatementBinlog(db); statementBinlog {
			logger.Warn("MySQL logs statements to its binary log, so changes to repositories and teams stay at REPEATABLE READ and concurrent ones may race. Set binlog_format to ROW or MIXED to avoid it.")
		}
	case "sqlite3":
		conf.UseSQLite3 = true
	default:
		panic("unreachable")
	}

	loadedLoginSourceFilesStore, err = loadLoginSourceFiles(logger, filepath.Join(conf.CustomDir(), "conf", "auth.d"), db.NowFunc)
	if err != nil {
		return nil, errors.Wrap(err, "load login source files")
	}

	Handle = &DB{db: db, logger: logger}
	return db, nil
}

// SetEngine connects to the database. When logQueries is true, every query is
// logged at the debug level. Otherwise only failed and slow queries are, which
// suits commands whose console output is read by people, such as backups.
func SetEngine(logger *logx.Logger, logQueries bool) (*gorm.DB, error) {
	if conf.Database.Type == "sqlite3" {
		if err := os.MkdirAll(filepath.Dir(conf.Database.Path), os.ModePerm); err != nil {
			return nil, errors.Wrap(err, "create directories")
		}
	}
	return NewConnection(logger, logQueries)
}

func NewEngine(logger *logx.Logger) error {
	db, err := SetEngine(logger, !conf.IsProdMode())
	if err != nil {
		return err
	}

	if err = migrations.Migrate(logger, db); err != nil {
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

func GetStatistic(ctx context.Context, logger *logx.Logger) (stats Statistic) {
	count := func(table any) int64 {
		var n int64
		_ = Handle.db.WithContext(ctx).Model(table).Count(&n).Error
		return n
	}

	stats.Counter.User = Handle.Users().Count(ctx)
	stats.Counter.Org = Handle.Organizations().Count(ctx)
	stats.Counter.PublicKey = count(new(PublicKey))
	stats.Counter.Repo = count(new(Repository))
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

// statementBinlog is true when MySQL logs statements, rather than rows, to its
// binary log, which rejects writes of READ COMMITTED transactions.
var statementBinlog bool

// usesStatementBinlog reports whether the MySQL server logs statements to its
// binary log. Servers without the setting, e.g., MySQL 9, always log rows.
func usesStatementBinlog(db *gorm.DB) bool {
	var logBin bool
	var format string
	if err := db.Raw("SELECT @@log_bin, @@binlog_format").Row().Scan(&logBin, &format); err != nil {
		return false
	}
	return logBin && strings.EqualFold(format, "STATEMENT")
}

// DB is the database handler for the storage layer.
type DB struct {
	db *gorm.DB
	// logger is for methods whose signatures are fixed by GORM hooks or
	// templates, so they cannot receive a logger from their caller.
	logger *logx.Logger
}

// handleLogger returns the logger of the global database handle. Only methods
// whose signatures are fixed by GORM hooks or templates use it, because their
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

func (db *DB) Attachments() *AttachmentsStore {
	return newAttachmentsStore(db.db)
}

func (db *DB) Collaborations() *CollaborationsStore {
	return newCollaborationsStore(db.db)
}

func (db *DB) Comments() *CommentsStore {
	return newCommentsStore(db.db)
}

func (db *DB) DeployKeys() *DeployKeysStore {
	return newDeployKeysStore(db.db)
}

func (db *DB) HookTasks() *HookTasksStore {
	return newHookTasksStore(db.db)
}

func (db *DB) Issues() *IssuesStore {
	return newIssuesStore(db.db)
}

func (db *DB) Labels() *LabelsStore {
	return newLabelsStore(db.db)
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

func (db *DB) Milestones() *MilestonesStore {
	return newMilestonesStore(db.db)
}

func (db *DB) Mirrors() *MirrorsStore {
	return newMirrorsStore(db.db)
}

func (db *DB) Notices() *NoticesStore {
	return newNoticesStore(db.db)
}

func (db *DB) Organizations() *OrganizationsStore {
	return newOrganizationsStore(db.db)
}

func (db *DB) Passkeys() *PasskeysStore {
	return newPasskeysStore(db.db)
}

func (db *DB) Permissions() *PermissionsStore {
	return newPermissionsStore(db.db)
}

func (db *DB) ProtectBranches() *ProtectBranchesStore {
	return newProtectBranchesStore(db.db)
}

func (db *DB) PublicKeys() *PublicKeysStore {
	return newPublicKeysStore(db.db)
}

func (db *DB) PullRequests() *PullRequestsStore {
	return newPullRequestsStore(db.db)
}

func (db *DB) Releases() *ReleasesStore {
	return newReleasesStore(db.db)
}

func (db *DB) Repositories() *RepositoriesStore {
	return newReposStore(db.db)
}

func (db *DB) Teams() *TeamsStore {
	return newTeamsStore(db.db)
}

func (db *DB) TwoFactors() *TwoFactorsStore {
	return newTwoFactorsStore(db.db)
}

func (db *DB) Uploads() *UploadsStore {
	return newUploadsStore(db.db)
}

func (db *DB) Users() *UsersStore {
	return newUsersStore(db.db)
}

func (db *DB) Webhooks() *WebhooksStore {
	return newWebhooksStore(db.db)
}

// ConnectionStats tells how busy the database connections are.
type ConnectionStats struct {
	InUse int
	Idle  int
	// Limit is how many connections the pool may open at most, or 0 when there
	// is no limit.
	Limit int
	// WaitCount is how many times a query waited for a connection since the
	// server started, and WaitDuration how long they waited in total.
	WaitCount    int64
	WaitDuration time.Duration
}

// GetConnectionStats returns how busy the database connections are.
func GetConnectionStats() ConnectionStats {
	if Handle == nil {
		return ConnectionStats{}
	}
	sqlDB, err := Handle.db.DB()
	if err != nil {
		return ConnectionStats{}
	}
	stats := sqlDB.Stats()
	return ConnectionStats{
		InUse:        stats.InUse,
		Idle:         stats.Idle,
		Limit:        stats.MaxOpenConnections,
		WaitCount:    stats.WaitCount,
		WaitDuration: stats.WaitDuration,
	}
}
