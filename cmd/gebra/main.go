// Gebra is a standalone fine-grained authorization server that speaks
// OpenFGA's HTTP API.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	_ "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/gebra"
	"gogs.io/gogs/internal/gebra/server"
)

func main() {
	addr := flag.String("addr", ":8080", "Address to listen on")
	dbType := flag.String("db-type", "sqlite3", "Database type: sqlite3, postgres, or mysql")
	dsn := flag.String("db-dsn", "file:gebra.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)", "Database connection string for the database type")
	maxOpenConns := flag.Int("db-max-open-conns", 50, "Maximum open database connections")
	changeRetention := flag.Duration("change-retention", gebra.DefaultChangeRetention, "How long the change log keeps changes, 0 keeps them forever")
	maxStaleness := flag.Duration("max-staleness", time.Second, "How far a store's view may lag behind its oldest pending write before queries with the default consistency evaluate the graph instead")
	flag.Parse()

	logger := logx.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	opts := gebra.Options{ChangeRetention: *changeRetention, MaxStaleness: *maxStaleness}
	if opts.ChangeRetention <= 0 {
		// The engine takes zero as its default retention.
		opts.ChangeRetention = -1
	}
	if err := run(ctx, logger, *addr, *dbType, *dsn, *maxOpenConns, opts); err != nil {
		logger.FatalContext(ctx, "Failed to run Gebra", "error", err)
	}
}

func run(ctx context.Context, logger *logx.Logger, addr, dbType, dsn string, maxOpenConns int, opts gebra.Options) error {
	metrics, shutdownTelemetry, err := setUpTelemetry(ctx, logger)
	if err != nil {
		return errors.Wrap(err, "set up telemetry")
	}
	defer shutdownTelemetry(5 * time.Second)

	var db *sql.DB
	var dialect gebra.Dialect
	switch dbType {
	case "sqlite3":
		db, err = sql.Open("sqlite", dsn)
		dialect = gebra.DialectSQLite
	case "postgres":
		cfg, err := pgx.ParseConfig(dsn)
		if err != nil {
			return errors.Wrap(err, "parse DSN")
		}
		// Gebra's queries are short index lookups, for which starting parallel
		// workers costs more than it saves.
		cfg.RuntimeParams["max_parallel_workers_per_gather"] = "0"
		db, dialect = stdlib.OpenDB(*cfg), gebra.DialectPostgres
	case "mysql":
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return errors.Wrap(err, "parse DSN")
		}
		cfg.ParseTime = true
		connector, err := mysql.NewConnector(cfg)
		if err != nil {
			return errors.Wrap(err, "create connector")
		}
		db, dialect = sql.OpenDB(connector), gebra.DialectMySQL
	default:
		return errors.Newf("unknown database type %q", dbType)
	}
	if err != nil {
		return errors.Wrap(err, "open database")
	}
	defer func() { _ = db.Close() }()
	if dbType == "sqlite3" {
		// SQLite allows one writer at a time.
		maxOpenConns = 1
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)

	engine, err := gebra.New(ctx, logger.Scoped("gebra"), db, dialect, opts)
	if err != nil {
		return errors.Wrap(err, "create engine")
	}
	defer engine.Close()

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics)
	mux.Handle("/", newTracingHandler(server.New(logger.Scoped("server"), engine)))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	logger.InfoContext(ctx, "Listening", "addr", addr, "databaseType", dbType)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return errors.Wrap(err, "serve")
	}
	return nil
}

// newTracingHandler records a span for every request, named after its route
// once the request was routed, so that Gebra's spans are grouped under it.
func newTracingHandler(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, "", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if r.Pattern != "" {
			return r.Pattern
		}
		return r.Method + " " + r.URL.Path
	}))
}
