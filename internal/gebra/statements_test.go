package gebra

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type countStatementsKey struct{}

// countStatement is a testHooks.statement that counts the statements that
// run with a context from countingContext, so that a test can count the round
// trips of one operation while the engine's background work runs.
func countStatement(ctx context.Context) {
	if n, ok := ctx.Value(countStatementsKey{}).(*atomic.Int64); ok {
		n.Add(1)
	}
}

func countingContext(ctx context.Context) (context.Context, *atomic.Int64) {
	n := new(atomic.Int64)
	return context.WithValue(ctx, countStatementsKey{}, n), n
}

// TestApplierStatements counts the statements that one incremental applier
// batch issues, including the BEGIN and COMMIT of its transaction, so that
// changes to the applier do not add round trips per write or per tuple
// unnoticed. The counts do not grow with the number of writes or tuples until
// a batch exceeds the limits of one statement.
func TestApplierStatements(t *testing.T) {
	ctx := context.Background()
	scenarios := []struct {
		name  string
		setup func(t *testing.T, e *Engine, storeID string)
		write func(t *testing.T, e *Engine, storeID string)
		want  int64
	}{
		{
			name: "one single-tuple write",
			want: 11,
			write: func(t *testing.T, e *Engine, storeID string) {
				mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})
			},
		},
		{
			name: "100 single-tuple writes",
			want: 16,
			write: func(t *testing.T, e *Engine, storeID string) {
				for i := range 100 {
					mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{benchMixedKey(i)}})
				}
			},
		},
		{
			name: "one 100-tuple write",
			want: 16,
			write: func(t *testing.T, e *Engine, storeID string) {
				req := &WriteRequest{}
				for i := range 100 {
					req.Writes = append(req.Writes, benchMixedKey(i))
				}
				mustWrite(t, e, storeID, req)
			},
		},
		{
			name: "group membership add with many reach rows",
			want: 17,
			setup: func(t *testing.T, e *Engine, storeID string) {
				req := &WriteRequest{}
				for i := range 200 {
					g := fmt.Sprintf("group:g%03d", i)
					req.Writes = append(req.Writes, tk(fmt.Sprintf("user:u%03d", i), "member", g), tk(g+"#member", "member", "group:hub"))
				}
				for i := range 20 {
					req.Writes = append(req.Writes, tk("group:top#member", "viewer", fmt.Sprintf("folder:f%02d", i)))
				}
				mustWrite(t, e, storeID, req)
			},
			write: func(t *testing.T, e *Engine, storeID string) {
				mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("group:hub#member", "member", "group:top")}})
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, hooks: testHooks{statement: countStatement}})
			if sc.setup != nil {
				sc.setup(t, e, storeID)
				drainView(t, e, storeID)
			}
			sc.write(t, e, storeID)
			batchCtx, n := countingContext(ctx)
			more, err := e.applyBatch(batchCtx, storeID)
			require.NoError(t, err)
			require.False(t, more)
			requireNothingPending(t, e, storeID)
			require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID))
			t.Logf("%s: %d statements on %s", sc.name, n.Load(), e.db.dialect)
			require.LessOrEqual(t, n.Load(), sc.want)
		})
	}
}

// benchMixedKey returns distinct tuples of every kind the view maintains:
// direct members, userset edges, and tuple to userset edges.
func benchMixedKey(i int) TupleKey {
	switch i % 4 {
	case 0:
		return tk(fmt.Sprintf("user:u%03d", i), "member", fmt.Sprintf("group:g%02d", i%10))
	case 1:
		return tk(fmt.Sprintf("group:g%02d#member", i%10), "viewer", fmt.Sprintf("folder:f%03d", i))
	case 2:
		return tk(fmt.Sprintf("folder:f%03d", i-1), "parent", fmt.Sprintf("doc:d%03d", i))
	}
	return tk(fmt.Sprintf("user:u%03d", i), "editor", fmt.Sprintf("doc:d%03d", i-1))
}

// checkStatementsModel is the benchmark's model with a chain of three tuple
// to usersets.
const checkStatementsModel = benchLadderModel + `type space
  relations
    define viewer: [user, team#member]
type project
  relations
    define space: [space]
    define viewer: [user] or viewer from space
type board
  relations
    define project: [project]
    define viewer: [user] or viewer from project
type card
  relations
    define board: [board]
    define viewer: [user] or viewer from board
`

type countSnapshotsKey struct{}

// TestCheckStatements counts the round trips of checks with the default
// consistency: statements, including the BEGIN and ROLLBACK of each snapshot
// on Postgres and MySQL. SQLite snapshots run neither, and count them as if
// they did. A check whose rewrite only
// reads the view runs as one statement after the one that reads the store's
// models.
func TestCheckStatements(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, checkStatementsModel, Options{manualApply: true, hooks: testHooks{
		statement: countStatement,
		beginSnapshot: func(ctx context.Context) {
			if n, ok := ctx.Value(countSnapshotsKey{}).(*atomic.Int64); ok {
				n.Add(1)
			}
		},
	}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{
		tk("user:anne", "member", "team:t1"),
		tk("team:t1#member", "reader", "repo:r1"),
		tk("user:bob", "member", "org:o1"),
		tk("org:o1", "owner_org", "repo:r1"),
		tk("user:carol", "reader", "repo:r1"),
		tk("user:carol", "blocked", "repo:r1"),
		tk("board:b1", "board", "card:c1"),
		tk("project:p1", "project", "board:b1"),
		tk("space:s1", "space", "project:p1"),
		tk("user:dan", "viewer", "space:s1"),
	}})
	drainView(t, e, storeID)

	for _, sc := range []struct {
		name    string
		key     CheckTupleKey
		allowed bool
		want    int64
	}{
		{"reader", CheckTupleKey{User: "user:anne", Relation: "reader", Object: "repo:r1"}, true, 2},
		{"exclusion allows", CheckTupleKey{User: "user:anne", Relation: "can_read", Object: "repo:r1"}, true, 2},
		{"exclusion denies", CheckTupleKey{User: "user:carol", Relation: "can_read", Object: "repo:r1"}, false, 2},
		{"exclusion through tuple to userset", CheckTupleKey{User: "user:bob", Relation: "can_read", Object: "repo:r1"}, true, 2},
		{"userset subject", CheckTupleKey{User: "team:t1#member", Relation: "reader", Object: "repo:r1"}, true, 2},
		{"userset subject with exclusion", CheckTupleKey{User: "team:t1#member", Relation: "can_read", Object: "repo:r1"}, true, 2},
		{"three tuple to usersets", CheckTupleKey{User: "user:dan", Relation: "viewer", Object: "card:c1"}, true, 2},
		{"three tuple to usersets denies", CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "card:c1"}, false, 2},
	} {
		t.Run(sc.name, func(t *testing.T) {
			// The time of the oldest pending write stays cached.
			e.oldestMu.Lock()
			e.oldest[storeID] = &oldestPending{read: time.Now().Add(time.Hour)}
			e.oldestMu.Unlock()
			checkCtx, statements := countingContext(ctx)
			snapshots := new(atomic.Int64)
			checkCtx = context.WithValue(checkCtx, countSnapshotsKey{}, snapshots)
			allowed, err := e.Check(checkCtx, storeID, &CheckRequest{TupleKey: sc.key})
			require.NoError(t, err)
			require.Equal(t, sc.allowed, allowed)
			trips := statements.Load()
			if e.db.dialect == DialectSQLite {
				trips += 2 * snapshots.Load()
			}
			t.Logf("%s: %d statements, %d snapshots, %d round trips", sc.name, statements.Load(), snapshots.Load(), trips)
			require.LessOrEqual(t, trips, sc.want)
		})
	}
}
