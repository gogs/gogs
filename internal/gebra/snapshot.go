package gebra

import (
	"context"
	"database/sql"
	"sync"

	"github.com/cockroachdb/errors"
)

// snapshot runs the statements of one query so that they all read one point
// in time, which an applier batch or a write that commits between two of them
// cannot split.
//
// On Postgres and MySQL it is a read-only repeatable read transaction. On
// SQLite, which runs in one process and whose shared cache locks whole tables
// until a transaction ends, it holds off the engine's write transactions
// instead, so that its statements can run on any connection without a
// transaction.
type snapshot struct {
	db dbtx
	// mu serializes statements on the transaction's connection, which the
	// parallel forks of an evaluator share.
	mu  *sync.Mutex
	end func()
}

func (e *Engine) beginSnapshot(ctx context.Context) (*snapshot, error) {
	if hook := e.opts.hooks.beginSnapshot; hook != nil {
		hook(ctx)
	}
	if e.db.dialect == DialectSQLite {
		e.sqliteWrites.RLock()
		return &snapshot{db: e.db.handle(), end: e.sqliteWrites.RUnlock}, nil
	}
	s := e.db.begin(ctx, "BEGIN")
	tx, err := e.db.pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	s.end(0, err)
	if err != nil {
		return nil, errors.Wrap(err, "begin snapshot")
	}
	end := func() {
		s := e.db.begin(ctx, "ROLLBACK")
		s.end(0, ignoreDone(tx.Rollback()))
	}
	return &snapshot{db: dbtx{database: e.db, q: tx}, mu: new(sync.Mutex), end: end}, nil
}

func (s *snapshot) run(fn func(db dbtx) error) error {
	if s.mu != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return fn(s.db)
}
