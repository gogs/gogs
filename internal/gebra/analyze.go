package gebra

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// The planner statistics of the tuple and view tables go stale when batches
// change a large share of their rows, e.g., while a new store is loaded
// through the API, and Postgres then plans scans where it should probe an
// index. The applier updates them after every flip to a new view
// generation, and after batches and build chunks that changed more than
// analyzeRows rows, or more than a tenth of the store's view rows and at
// least analyzeMinRows rows, since the last update.
const (
	analyzeMinRows = 1000
	analyzeRows    = 10_000
	// analyzeInterval is the least time between two updates that no flip
	// asked for.
	analyzeInterval = 10 * time.Second
)

// analyzedTables are the tables whose statistics the applier updates.
var analyzedTables = []string{"gebra_tuple", "gebra_pending", "gebra_view_member", "gebra_view_edge", "gebra_view_reach"}

// maybeAnalyze updates the planner statistics in the background when an
// applier transaction that changed the given number of rows, together with
// the ones before it, changed enough rows for a store whose serving view holds
// viewRows rows, or when force asks for it, e.g., after a flip.
func (e *Engine) maybeAnalyze(changed, viewRows int64, force bool) {
	since := e.sinceAnalyze.Add(changed)
	if !force && (since < analyzeMinRows || since <= analyzeRows && since*10 <= viewRows) {
		return
	}
	e.applyMu.Lock()
	defer e.applyMu.Unlock()
	if e.closed {
		return
	}
	if !e.analyzing.CompareAndSwap(false, true) {
		if force {
			// The update in progress may have read the tables before the
			// flip committed.
			e.analyzeAgain.Store(true)
		}
		return
	}
	if !force && time.Since(e.lastAnalyze) < analyzeInterval {
		e.analyzing.Store(false)
		return
	}
	e.appliers.Add(1)
	go func() {
		defer e.appliers.Done()
		defer e.analyzing.Store(false)
		for {
			e.lastAnalyze = time.Now()
			e.sinceAnalyze.Store(0)
			e.analyzeAgain.Store(false)
			if err := e.analyze(e.background); err != nil && e.background.Err() == nil {
				e.logger.WarnContext(e.background, "Failed to update planner statistics", "error", err)
			}
			if !e.analyzeAgain.Load() || e.background.Err() != nil {
				return
			}
		}
	}()
}

func (e *Engine) analyze(ctx context.Context) error {
	db := e.db.handle()
	switch e.db.dialect {
	case DialectPostgres:
		_, err := db.exec(ctx, "ANALYZE "+strings.Join(analyzedTables, ", "))
		return errors.Wrap(err, "analyze")
	case DialectMySQL:
		_, err := db.exec(ctx, "ANALYZE TABLE "+strings.Join(analyzedTables, ", "))
		return errors.Wrap(err, "analyze")
	case DialectSQLite:
		// ANALYZE writes the statistics tables, so it runs as a write.
		e.sqliteWrites.Lock()
		defer e.sqliteWrites.Unlock()
		for _, t := range analyzedTables {
			if _, err := db.exec(ctx, "ANALYZE "+t); err != nil {
				return errors.Wrapf(err, "analyze %s", t)
			}
		}
	}
	return nil
}
