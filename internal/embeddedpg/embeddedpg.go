package embeddedpg

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	embpg "github.com/fergusstrange/embedded-postgres"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

// LocalPostgres wraps embedded PostgreSQL server functionality.
type LocalPostgres struct {
	srv      *embpg.EmbeddedPostgres
	baseDir  string
	tcpPort  uint32
	database string
	user     string
	pass     string
}

// Initialize creates a LocalPostgres with default settings based on workDir.
func Initialize(workDir string) *LocalPostgres {
	storageBase := filepath.Join(workDir, "data", "local-postgres")
	return &LocalPostgres{
		baseDir:  storageBase,
		tcpPort:  15432,
		database: "gogs",
		user:     "gogs",
		pass:     "gogs",
	}
}

// Launch starts the embedded PostgreSQL server and blocks until ready.
func (pg *LocalPostgres) Launch() error {
	log.Info("Launching local PostgreSQL server...")

	if err := os.MkdirAll(pg.baseDir, 0o700); err != nil {
		return errors.Wrap(err, "mkdir local postgres base")
	}

	opts := embpg.DefaultConfig().
		Username(pg.user).
		Password(pg.pass).
		Database(pg.database).
		Port(pg.tcpPort).
		DataPath(pg.baseDir).
		RuntimePath(filepath.Join(pg.baseDir, "rt")).
		BinariesPath(filepath.Join(pg.baseDir, "bin")).
		CachePath(filepath.Join(pg.baseDir, "dl")).
		StartTimeout(45 * time.Second).
		Logger(os.Stderr)

	pg.srv = embpg.NewDatabase(opts)

	if err := pg.srv.Start(); err != nil {
		return errors.Wrap(err, "launch embedded pg")
	}

	log.Info("Local PostgreSQL ready on port %d", pg.tcpPort)
	return nil
}

// Shutdown stops the embedded PostgreSQL server gracefully.
func (pg *LocalPostgres) Shutdown() error {
	if pg.srv == nil {
		return nil
	}

	log.Info("Shutting down local PostgreSQL...")
	if err := pg.srv.Stop(); err != nil {
		return errors.Wrap(err, "shutdown embedded pg")
	}

	log.Info("Local PostgreSQL shutdown complete")
	return nil
}

// ConfigureGlobalDatabase modifies global conf.Database to point to this instance.
func (pg *LocalPostgres) ConfigureGlobalDatabase() {
	conf.Database.Type = "postgres"
	conf.Database.Host = fmt.Sprintf("localhost:%d", pg.tcpPort)
	conf.Database.Name = pg.database
	conf.Database.User = pg.user
	conf.Database.Password = pg.pass
	conf.Database.SSLMode = "disable"

	log.Trace("Global database configured for local PostgreSQL")
}
