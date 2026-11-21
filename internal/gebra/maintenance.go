package gebra

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/cockroachdb/errors"
)

// DefaultChangeRetention is how long the change log keeps a change when
// Options.ChangeRetention is zero.
const DefaultChangeRetention = 7 * 24 * time.Hour

// ErrChangesExpired is returned by ReadChanges when its continuation token
// points before changes that the change log no longer keeps. The client must
// read again without the token, e.g., from a start time.
var ErrChangesExpired = errors.New("changes after the continuation token are older than the change log retention and were deleted")

const (
	// maintenanceInterval is how often an engine deletes expired changes and
	// the data of deleted stores.
	maintenanceInterval = time.Minute
	// purgeBatchSize bounds the rows that one maintenance transaction
	// deletes, so that none holds its locks for long.
	purgeBatchSize = 5000
)

// maintenance periodically runs maintain until the engine closes.
func (e *Engine) maintenance() {
	defer e.appliers.Done()
	ticker := time.NewTicker(maintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.background.Done():
			return
		case <-ticker.C:
		}
		if err := e.maintain(e.background, time.Now()); err != nil && e.background.Err() == nil {
			e.logger.ErrorContext(e.background, "Failed to maintain stores", "error", err)
		}
	}
}

// maintain deletes the changes that are older than Options.ChangeRetention at
// now, the view generations that no query reads anymore, and the pending
// writes, view, and change log of deleted stores. Every engine runs it, and
// runs that overlap delete each row once.
func (e *Engine) maintain(ctx context.Context, now time.Time) error {
	return errors.Join(e.expireChanges(ctx, now), e.collectViewGens(ctx, now), e.purgeDeletedStores(ctx))
}

// collectViewGens deletes the rows of the view generations that were retired
// before now, in batches, and then their records.
func (e *Engine) collectViewGens(ctx context.Context, now time.Time) error {
	gens, err := queryStructs[viewGCRow](ctx, e.db.handle(), "SELECT store_id, gen FROM gebra_view_gc WHERE delete_after <= ? ORDER BY store_id, gen", now.UTC())
	if err != nil {
		return errors.Wrap(err, "find retired view generations")
	}
	var errs []error
	for _, g := range gens {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := e.deleteGen(ctx, g.StoreID, g.Gen)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "delete view generation %d of store %q", g.Gen, g.StoreID))
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) deleteGen(ctx context.Context, storeID string, gen int64) error {
	for _, table := range []string{"gebra_view_reach", "gebra_view_edge", "gebra_view_member"} {
		if err := e.deleteInBatches(ctx, table, purgeBatchSize, "store_id = ? AND gen = ?", storeID, gen); err != nil {
			return err
		}
	}
	err := e.writeTx(ctx, func(tx dbtx) error {
		_, err := tx.exec(ctx, "DELETE FROM gebra_view_gc WHERE store_id = ? AND gen = ?", storeID, gen)
		return err
	})
	return errors.Wrap(err, "delete view generation record")
}

func (e *Engine) expireChanges(ctx context.Context, now time.Time) error {
	if e.opts.ChangeRetention < 0 {
		return nil
	}
	db := e.db.handle()
	storeIDs, err := queryColumn[string](ctx, db, "SELECT id FROM gebra_store WHERE deleted_at IS NULL ORDER BY id")
	if err != nil {
		return errors.Wrap(err, "list stores")
	}
	before := now.Add(-e.opts.ChangeRetention)
	var errs []error
	for _, storeID := range storeIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cutoff, err := changeCutoff(ctx, db, storeID, before)
		if err == nil && cutoff > 0 {
			err = e.deleteChangesThrough(ctx, storeID, cutoff)
		}
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "expire changes of store %q", storeID))
		}
	}
	return errors.Join(errs...)
}

// changeCutoff returns the highest change ID of the store up to which every
// change of the store was created before the given time, or zero when there
// is none. Retention deletes the store's changes up to it, so a caller that
// must keep changes longer can lower it.
func changeCutoff(ctx context.Context, db dbtx, storeID string, before time.Time) (int64, error) {
	// A change has the time of its write, and the applier can apply an old
	// write after newer ones, so the cutoff stops below the first change that
	// is not old enough instead of taking every old one.
	var cutoff int64
	_, err := db.queryRow(ctx, `SELECT COALESCE(MAX(id), 0) FROM gebra_change
		WHERE store_id = ? AND id < COALESCE(
			(SELECT id FROM gebra_change WHERE store_id = ? AND created_at >= ? ORDER BY id LIMIT 1), ?)`,
		[]any{storeID, storeID, before.UTC(), int64(math.MaxInt64)}, &cutoff)
	if err != nil {
		return 0, errors.Wrap(err, "find change cutoff")
	}
	return cutoff, nil
}

// deleteChangesThrough deletes the store's changes up to the change ID.
func (e *Engine) deleteChangesThrough(ctx context.Context, storeID string, cutoff int64) error {
	// The cutoff is recorded before any change is deleted, and ReadChanges
	// reads it after the changes, so a read that missed deleted changes sees
	// the cutoff.
	err := e.writeTx(ctx, func(tx dbtx) error {
		now := time.Now().UTC()
		// gebra_change_cutoff holds the highest ID of the store's changes
		// that retention deleted. Any change of the store up to it may be
		// gone, and every change after it is kept.
		_, err := tx.insertRows(ctx, "gebra_change_cutoff", []string{"store_id", "change_id", "updated_at"}, [][]any{{storeID, 0, now}}, onConflict{ignore: true})
		if err != nil {
			return errors.Wrap(err, "create change cutoff")
		}
		_, err = tx.exec(ctx, "UPDATE gebra_change_cutoff SET change_id = ?, updated_at = ? WHERE store_id = ? AND change_id < ?", cutoff, now, storeID, cutoff)
		return errors.Wrap(err, "update change cutoff")
	})
	if err != nil {
		return err
	}
	return e.deleteInBatches(ctx, "gebra_change", purgeBatchSize, "store_id = ? AND id <= ?", storeID, cutoff)
}

// checkChangesKept returns ErrChangesExpired when retention deleted changes of
// the store with IDs above after.
func checkChangesKept(ctx context.Context, db dbtx, storeID string, after int64) error {
	cutoffs, err := queryColumn[int64](ctx, db, "SELECT change_id FROM gebra_change_cutoff WHERE store_id = ?", storeID)
	if err != nil {
		return errors.Wrap(err, "read change cutoff")
	}
	if len(cutoffs) > 0 && after < cutoffs[0] {
		return errors.Wrapf(ErrChangesExpired, "store %q", storeID)
	}
	return nil
}

// purgeDeletedStores deletes the pending writes, view, change log, and view
// state of deleted stores. Their tuples and models stay. A write that raced
// with the deletion may add pending writes later, which the next run deletes.
func (e *Engine) purgeDeletedStores(ctx context.Context) error {
	storeIDs, err := queryColumn[string](ctx, e.db.handle(), `SELECT s.id FROM gebra_store s
		WHERE s.deleted_at IS NOT NULL AND (
			EXISTS (SELECT 1 FROM gebra_view_state t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_inbox t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_pending t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_change t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_change_cutoff t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_view_member t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_view_edge t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_view_reach t WHERE t.store_id = s.id)
			OR EXISTS (SELECT 1 FROM gebra_view_gc t WHERE t.store_id = s.id))
		ORDER BY s.id`)
	if err != nil {
		return errors.Wrap(err, "find deleted stores")
	}
	var errs []error
	for _, storeID := range storeIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := e.purgeStore(ctx, storeID); err != nil {
			errs = append(errs, errors.Wrapf(err, "purge deleted store %q", storeID))
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) purgeStore(ctx context.Context, storeID string) error {
	// Applier batches lock the view state row first and fail without it, so
	// deleting it waits for a batch that is still running and keeps later
	// batches from writing the view or the change log again.
	err := e.writeTx(ctx, func(tx dbtx) error {
		_, err := tx.exec(ctx, "DELETE FROM gebra_view_state WHERE store_id = ?", storeID)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "delete view state")
	}
	for _, table := range []string{
		"gebra_inbox", "gebra_pending", "gebra_change", "gebra_view_member", "gebra_view_edge", "gebra_view_reach", "gebra_change_cutoff",
		"gebra_view_gc",
	} {
		if err := e.deleteInBatches(ctx, table, purgeBatchSize, "store_id = ?", storeID); err != nil {
			return err
		}
	}
	return nil
}

// deleteInBatches deletes the table's rows that match the condition, at most
// batchSize rows per transaction.
func (e *Engine) deleteInBatches(ctx context.Context, table string, batchSize int, cond string, args ...any) error {
	var stmt string
	switch e.db.dialect {
	case DialectPostgres:
		stmt = fmt.Sprintf(`DELETE FROM %[1]s WHERE ctid = ANY(ARRAY(SELECT ctid FROM %[1]s WHERE %[2]s LIMIT %[3]d))`, table, cond, batchSize)
	case DialectMySQL:
		stmt = fmt.Sprintf(`DELETE FROM %s WHERE %s LIMIT %d`, table, cond, batchSize)
	default:
		stmt = fmt.Sprintf(`DELETE FROM %[1]s WHERE rowid IN (SELECT rowid FROM %[1]s WHERE %[2]s LIMIT %[3]d)`, table, cond, batchSize)
	}
	for {
		var deleted int64
		err := e.writeTx(ctx, func(tx dbtx) (err error) {
			deleted, err = tx.exec(ctx, stmt, args...)
			return err
		})
		if err != nil {
			return errors.Wrapf(err, "delete from %s", table)
		}
		if deleted < int64(batchSize) {
			return nil
		}
	}
}
