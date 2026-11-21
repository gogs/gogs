package gebra

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// leadership tracks the stores whose applier runs in this engine.
//
// On Postgres and MySQL, an engine is a store's applier while it holds the
// store's lock on its leader connection, a connection it keeps for nothing
// else: an advisory lock on Postgres, and a GET_LOCK lock on MySQL. The locks
// end with the connection's session, so another engine takes over within
// Options.ApplyInterval when this one closes, crashes, or loses the
// connection.
//
// SQLite runs in one process, so every engine leads every store.
type leadership struct {
	mu   sync.Mutex
	conn *sql.Conn
	// pid is the Postgres backend process or the MySQL connection of conn.
	pid int64
	// database is the MySQL database of conn.
	database string
	held     map[string]bool
	// tried holds when this engine last failed to become a store's applier.
	tried map[string]time.Time
}

// leaderTimeout bounds each statement on the leader connection, so that a
// broken connection cannot stall every applier of the engine.
const leaderTimeout = 5 * time.Second

// applierLockKey returns the advisory lock key of the store's applier.
func applierLockKey(storeID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("gebra applier " + storeID))
	return int64(h.Sum64())
}

// mysqlLockName returns the GET_LOCK name of the store's applier. MySQL locks
// are server-wide and their names at most 64 characters long, so the name
// holds a hash of the database, which keeps copies of a database on one server
// apart, and the store ID, which is 26 characters long.
func mysqlLockName(database, storeID string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(database))
	name := fmt.Sprintf("gebra.%016x.%s", h.Sum64(), storeID)
	if len(name) > 64 {
		name = fmt.Sprintf("gebra.%016x.%016x", h.Sum64(), uint64(applierLockKey(storeID)))
	}
	return name
}

// lead reports whether this engine is the store's applier, trying to become
// it at most twice per Options.ApplyInterval.
func (e *Engine) lead(ctx context.Context, storeID string) bool {
	dialect := e.db.dialect
	if dialect == DialectSQLite {
		return true
	}
	l := &e.leader
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held[storeID] {
		return true
	}
	if t, ok := l.tried[storeID]; ok && time.Since(t) < e.opts.ApplyInterval/2 {
		return false
	}
	l.tried[storeID] = time.Now()

	ctx, cancel := context.WithTimeout(ctx, leaderTimeout)
	defer cancel()
	if l.conn == nil {
		conn, err := e.db.pool.Conn(ctx)
		if err != nil {
			e.logger.WarnContext(ctx, "Failed to open the applier connection", "error", err)
			return false
		}
		if dialect == DialectMySQL {
			err = conn.QueryRowContext(ctx, "SELECT CONNECTION_ID(), DATABASE()").Scan(&l.pid, &l.database)
		} else {
			err = conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&l.pid)
		}
		if err != nil {
			discardConn(conn)
			e.logger.WarnContext(ctx, "Failed to open the applier connection", "error", err)
			return false
		}
		l.conn = conn
	}
	var ok bool
	var err error
	if dialect == DialectMySQL {
		// GET_LOCK returns NULL on errors, which fails the scan.
		err = l.conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", mysqlLockName(l.database, storeID)).Scan(&ok)
	} else {
		err = l.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", applierLockKey(storeID)).Scan(&ok)
	}
	if err != nil {
		e.logger.WarnContext(ctx, "Lost the applier connection", "error", err)
		e.dropLeaderConnLocked(ctx, "lost")
		return false
	}
	if ok {
		l.held[storeID] = true
		delete(l.tried, storeID)
		e.tel.leadership.Add(ctx, 1, metric.WithAttributes(attribute.String("gebra.applier.event", "acquired")))
		e.logger.InfoContext(ctx, "Became the applier of a store", "storeID", storeID)
	}
	return ok
}

// checkLeadership drops the leader connection, and with it every store this
// engine leads, when the connection no longer works.
func (e *Engine) checkLeadership(ctx context.Context) {
	l := &e.leader
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, leaderTimeout)
	defer cancel()
	if err := l.conn.PingContext(ctx); err != nil {
		e.logger.WarnContext(ctx, "Lost the applier connection", "stores", len(l.held), "error", err)
		e.dropLeaderConnLocked(ctx, "lost")
	}
}

// releaseLeadership closes the leader connection, which ends this engine's
// leadership of every store.
func (e *Engine) releaseLeadership() {
	e.leader.mu.Lock()
	defer e.leader.mu.Unlock()
	if e.leader.conn != nil {
		e.dropLeaderConnLocked(context.Background(), "released")
	}
}

func (e *Engine) dropLeaderConnLocked(ctx context.Context, event string) {
	l := &e.leader
	discardConn(l.conn)
	l.conn = nil
	l.pid = 0
	if n := len(l.held); n > 0 {
		e.tel.leadership.Add(ctx, int64(n), metric.WithAttributes(attribute.String("gebra.applier.event", event)))
	}
	l.held = map[string]bool{}
}

// discardConn closes the connection's session instead of returning it to the
// pool, where it would keep its advisory locks.
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}
