package gebra

import (
	"time"
)

// The row types hold rows of the engine's tables, whose schema the
// migrations define.

type storeRow struct {
	ID   string
	Name string
	// ModelID is the latest authorization model. Writes validate tuples
	// against it, and the applier builds a view for it, which makes it the
	// live model once complete.
	ModelID string
	// Revision increases with every model write.
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type tupleRow struct {
	ID               int64
	StoreID          string
	ObjectType       string
	ObjectID         string
	Relation         string
	UserType         string
	UserID           string
	UserRelation     string
	ConditionName    string
	ConditionContext string
	CreatedAt        time.Time
}

// changeRow is a tuple change in the change log. The store's applier is the
// only writer of its changes and commits them one batch after another, so IDs
// of a store's changes commit in increasing order, and a reader that has seen
// ID n never sees a smaller one later.
type changeRow struct {
	ID               int64
	StoreID          string
	Operation        string
	ObjectType       string
	ObjectID         string
	Relation         string
	UserType         string
	UserID           string
	UserRelation     string
	ConditionName    string
	ConditionContext string
	CreatedAt        time.Time
}

// changeColumns are the columns of gebra_change that a change log row sets.
var changeColumns = []string{"store_id", "operation", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "condition_name", "condition_context", "created_at"}

// attributeRow is an object attribute that list queries sort by. Value is
// encoded so that byte order matches the attribute type's order.
type attributeRow struct {
	StoreID    string
	ObjectType string
	Name       string
	ObjectID   string
	Value      string
}

// inboxRow is a committed write that the store's applier has not applied yet.
// A write inserts it in its own transaction after changing gebra_tuple, and
// the applier deletes it in the transaction that applies it, so the table
// only holds pending writes. The ID is the write's revision.
//
// Two writes of the same tuple serialize on the tuple's row or unique index
// entry, and each takes its ID after its tuple changes, so the later one has
// the higher ID. Writes of different tuples can commit out of ID order.
type inboxRow struct {
	ID      int64
	StoreID string
	// Kind is inboxTuples or inboxModel.
	Kind string
	// Changes holds the tuple changes as JSON, deletes first.
	Changes string
	// Tuples is the number of changes, which bounds applier batches.
	Tuples    int
	CreatedAt time.Time
}

// gebra_pending holds the tuple changes of the writes in the inbox. A write
// inserts its pending rows in the transaction that inserts its inbox row, and
// the applier deletes them in the transaction that applies the write, so
// queries that use the view can read the tuples that the view reflects:
// gebra_tuple without the keys that pending changes touch, plus the rows that
// the earliest pending change of a key deleted. A delete keeps the deleted
// row's condition. A write only needs its key.

const (
	// inboxTuples is a tuple write.
	inboxTuples = "tuples"
	// inboxModel is a model write. The applier consumes it like a tuple
	// write that changes nothing, and builds a view for the store's latest
	// model when that differs from the models of its views.
	inboxModel = "model"
)

// viewStateRow records what a store's materialized view includes. Only the
// store's applier writes it, and every applier transaction locks it first.
//
// The view tables hold generations of a store's view, each built for one
// model. Gen is the serving generation, which queries with the live model
// read, and BuildGen the one the applier builds for a newer model next to
// it. The applier maintains both with every batch, and flips Gen to BuildGen
// in the transaction that completes the build.
type viewStateRow struct {
	StoreID string
	// Applied is the highest inbox ID applied so far. Lower IDs can still be
	// pending, because writes commit out of ID order.
	Applied int64
	// Gen is the serving generation, or zero before the first build of the
	// store completed.
	Gen int64
	// ModelID is the authorization model of the serving generation, which
	// queries use when they name no model: the live model.
	ModelID string
	// The row counts of the serving generation change in the transactions
	// that change the rows, so they are exact without counting the tables.
	MemberRows int64
	EdgeRows   int64
	ReachRows  int64
	// LastGen is the highest generation the store ever had, so that a new
	// build never reuses the generation of rows that are still to be deleted.
	LastGen int64

	// BuildGen is the generation being built, or zero.
	BuildGen     int64
	BuildModelID string
	// BuildPhase is buildMembers or buildReach.
	BuildPhase string
	// The build cursor of buildMembers is the key of the last tuple that the
	// build covered, in the order of the tuple table's unique index. Its
	// columns have the types of the tuple columns, so that comparing a key
	// with the cursor in SQL uses the order that the build reads tuples in.
	BuildObjectType   string
	BuildObjectID     string
	BuildRelation     string
	BuildUserType     string
	BuildUserID       string
	BuildUserRelation string
	// BuildSetKey is the build cursor of buildReach: the last set whose
	// reach rows the build covered, in the order of the set key columns.
	BuildSetKey string
	// BuildDone counts the tuples or sets that the current phase covered in
	// BuildChunks chunks, out of an estimated BuildTotal.
	BuildDone   int64
	BuildTotal  int64
	BuildChunks int64
	// The row counts of the building generation.
	BuildMemberRows int64
	BuildEdgeRows   int64
	BuildReachRows  int64
	BuildStartedAt  *time.Time
	UpdatedAt       time.Time

	// LatestModelID is the store's latest model when the row was locked. It
	// is not a column.
	LatestModelID string
}

// viewGCRow is a generation of a store's view that the applier no longer
// maintains: the serving generation before a flip, or a cancelled build.
// Maintenance deletes its rows in batches once DeleteAfter has passed, and
// then the record.
type viewGCRow struct {
	StoreID string
	Gen     int64
	// DeleteAfter is the retirement time plus viewGCGrace for a former
	// serving generation, which queries that resolved it before the flip may
	// still read, and the cancellation time for a build, which no query reads.
	DeleteAfter time.Time
}

// The view tables hold the materialized view of union-only relations. A node
// of the view is a set "type:id#relation". Every row belongs to a generation,
// which leads every index after the store, so that a generation's rows are
// one range of each index.
//
//   - viewMemberRow: a subject (or wildcard) that a tuple puts directly in a set.
//   - viewEdgeRow: a set that a tuple puts inside another set, through a
//     userset tuple or a tuple to userset. Refs is the number of tuples that
//     produce the edge.
//   - viewReachRow: the transitive closure. Every set that a member set or an
//     edge child (an "anchored" set) is contained in, directly or indirectly,
//     including the containment that computed usersets imply.

// The user index ends with set_id so that a list reads a user's sets of a
// relation in sort order and stops at the page size. It also covers set_key,
// in INCLUDE on Postgres and through the primary key on MySQL, so that joining
// a user's sets to the closure reads only the index.
type viewMemberRow struct {
	StoreID  string
	Gen      int64
	SetKey   string
	UserType string
	UserID   string
	SetType  string
	SetRel   string
	SetID    string
}

type viewEdgeRow struct {
	StoreID   string
	Gen       int64
	ChildKey  string
	ParentKey string
	Refs      int64
}

type viewReachRow struct {
	StoreID  string
	Gen      int64
	DescKey  string
	AncType  string
	AncRel   string
	AncID    string
	DescType string
	DescRel  string
	DescID   string
}
