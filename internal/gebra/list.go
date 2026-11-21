package gebra

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
)

// Sort orders list results by an object attribute, then by ID. Objects
// without the attribute sort as if it were empty. The zero value sorts by ID.
type Sort struct {
	Attribute  string
	Descending bool
}

// ListObjectsRequest asks for the objects of a type that a user has a relation
// with.
type ListObjectsRequest struct {
	Type                 string
	Relation             string
	User                 string
	ContextualTuples     []TupleKey
	Context              map[string]any
	AuthorizationModelID string
	Consistency          Consistency
	Sort                 Sort
	// PageSize zero returns every result.
	PageSize          int
	ContinuationToken string
}

// ListObjects returns a sorted page of objects. Results are exact: every
// object the user can access is listed once, in order, across pages.
func (e *Engine) ListObjects(ctx context.Context, storeID string, req *ListObjectsRequest) (_ []string, _ string, err error) {
	ctx, o := e.startOp(ctx, "list_objects", storeID)
	var out []string
	defer func() {
		o.end(err, attribute.Int("gebra.results", len(out)), attribute.Int("gebra.page_size", req.PageSize))
	}()
	var rows []cursorRow
	emit := func(r cursorRow) error {
		rows = append(rows, r)
		out = append(out, req.Type+":"+r.ID)
		if req.PageSize > 0 && len(out) > req.PageSize {
			return errStop
		}
		return nil
	}
	page, ok, err := e.fastList(ctx, o, storeID, req)
	if err != nil {
		return nil, "", err
	}
	if ok {
		for _, r := range page {
			_ = emit(r)
		}
	} else {
		err = e.listObjects(ctx, storeID, req, req.PageSize, emit)
	}
	if err != nil && !errors.Is(err, errStop) {
		return nil, "", err
	}
	var next string
	if req.PageSize > 0 && len(out) > req.PageSize {
		out = out[:req.PageSize]
		next = rows[req.PageSize-1].token()
	}
	return out, next, nil
}

// StreamListObjects calls emit with each object as it is found, in sorted
// order, starting after the continuation token. It ignores PageSize.
func (e *Engine) StreamListObjects(ctx context.Context, storeID string, req *ListObjectsRequest, emit func(object string) error) (err error) {
	ctx, o := e.startOp(ctx, "streamed_list_objects", storeID)
	results := 0
	defer func() { o.end(err, attribute.Int("gebra.results", results)) }()
	return e.listObjects(ctx, storeID, req, 0, func(r cursorRow) error {
		results++
		return emit(req.Type + ":" + r.ID)
	})
}

var errStop = errors.New("stop")

func (e *Engine) listObjects(ctx context.Context, storeID string, req *ListObjectsRequest, pageSize int, emit func(cursorRow) error) error {
	ev, err := e.newEvaluator(ctx, storeID, req.AuthorizationModelID, req.Consistency, req.ContextualTuples, req.Context, true)
	if err != nil {
		return err
	}
	defer ev.end()
	r := ev.m.relation(req.Type, req.Relation)
	if r == nil {
		return invalidf("relation %s#%s is not defined", req.Type, req.Relation)
	}
	if o := opFromContext(ctx); o != nil {
		o.target(req.Type, req.Relation)
	}
	sub, err := parseSubject(req.User)
	if err != nil {
		return err
	}
	if err := ev.m.validateSubject(sub); err != nil {
		return err
	}
	if err := ev.m.validateSort(req.Type, req.Sort); err != nil {
		return err
	}
	after, err := parseCursor(req.ContinuationToken)
	if err != nil {
		return err
	}
	// Changes that the subject cannot reach do not change which objects it
	// has the relation with.
	scope := &pendingScope{sub: &sub, sets: newScopeSets()}
	ev.m.addScope(scope.sets, object{typ: req.Type, id: "*"}, req.Relation, map[string]bool{})
	if err := ev.settle(scope, true, r.materialized && sub.rel == ""); err != nil {
		return err
	}
	// When pending changes affect some objects of a materialized relation,
	// the view lists the others as the latest state would, and the graph
	// decides only the affected ones, which the view may lack.
	var affected map[string]bool
	if ev.partialSources != nil {
		ids, err := ev.affectedObjects(ev.partialSources, req.Type, req.Relation)
		if err != nil {
			return err
		}
		affected = make(map[string]bool, len(ids))
		for _, id := range ids {
			affected[id] = true
		}
	}

	src := &sortedSource{storeID: storeID, objectType: req.Type, sort: req.Sort}
	verify := true
	if (ev.useView || affected != nil) && sub.rel == "" {
		rels := []string{req.Relation}
		ok := r.materialized
		if ok {
			verify = false
		} else {
			rels, ok = ev.m.cover(req.Type, r.rewrite, false, map[string]bool{req.Relation: true})
		}
		if ok {
			ids := []string{sub.id}
			if sub.id != "*" {
				ids = append(ids, "*")
			}
			v := viewObjects{storeID: storeID, gen: ev.gen, m: ev.m, userType: sub.typ, userIDs: ids, objectType: req.Type, rels: rels, sort: req.Sort, collation: e.collation()}
			src.sub, src.args = v.query(nil, 0)
			// SQLite has no LATERAL, and MySQL needs only to be correct.
			if e.db.dialect == DialectPostgres && req.Sort.Attribute == "" {
				src.paged = v.query
			}
		}
	}
	var extras []string
	if src.sub == "" {
		src.sub = `SELECT DISTINCT object_id AS obj_id FROM gebra_tuple WHERE store_id = ? AND object_type = ?`
		src.args = []any{storeID, req.Type}
		if ev.applied {
			// Tuples that pending writes deleted are still there for the
			// evaluator.
			src.sub = `SELECT object_id AS obj_id FROM gebra_tuple WHERE store_id = ? AND object_type = ?
UNION
SELECT object_id FROM gebra_pending WHERE store_id = ? AND object_type = ? AND deleted = TRUE`
			src.args = append(src.args, storeID, req.Type)
		}
		for _, ts := range ev.contextual {
			for _, t := range ts {
				if t.objectType == req.Type {
					extras = append(extras, t.objectID)
				}
			}
		}
	}
	// A userset is in its own relation without any tuple naming its object,
	// e.g., doc:1#viewer has viewer on doc:1, and so in every relation that
	// computes from it. Userset subjects are always verified.
	if sub.rel != "" && sub.typ == req.Type {
		extras = append(extras, sub.id)
	}

	check := func(ev *evaluator, id string) (bool, error) {
		return ev.check(object{typ: req.Type, id: id}, req.Relation, sub)
	}
	if affected != nil {
		verify = true
		extras = append(extras, mapKeys(affected)...)
		check = func(ev *evaluator, id string) (bool, error) {
			if !affected[id] {
				return true, nil
			}
			return ev.check(object{typ: req.Type, id: id}, req.Relation, sub)
		}
	}
	return e.walkSorted(ev, src, after, extras, verify, pageSize, check, emit)
}

// fastList answers a page of objects of a materialized relation in ID order
// with one statement when it can, as fastCheck does a check: the statement
// reads the page from the view that the engine last saw serving, and checks
// that the view still serves and the store exists. A single statement reads
// one point in time, so the list needs no snapshot transaction. Otherwise it
// returns false and leaves the list to listObjects.
func (e *Engine) fastList(ctx context.Context, o *op, storeID string, req *ListObjectsRequest) ([]cursorRow, bool, error) {
	c := req.Consistency
	if req.PageSize <= 0 || e.db.dialect != DialectPostgres || req.Sort.Attribute != "" || len(req.ContextualTuples) > 0 ||
		c.MinRevision != "" || c.Preference == ConsistencyHigher {
		return nil, false, nil
	}
	// listObjects rejects an invalid consistency.
	if _, err := minRevision(c); err != nil {
		return nil, false, nil
	}
	lv, ok, err := e.liveView(ctx, storeID)
	if err != nil || !ok || req.AuthorizationModelID != "" && req.AuthorizationModelID != lv.modelID {
		return nil, false, err
	}
	m, err := e.model(ctx, e.db.handle(), storeID, lv.modelID)
	if err != nil {
		return nil, false, nil
	}
	if r := m.relation(req.Type, req.Relation); r == nil || !r.materialized {
		return nil, false, nil
	}
	sub, err := parseSubject(req.User)
	if err != nil || sub.rel != "" || m.validateSubject(sub) != nil {
		return nil, false, nil
	}
	after, err := parseCursor(req.ContinuationToken)
	if err != nil {
		return nil, false, nil
	}
	if c.Preference != ConsistencyMinimizeLatency {
		stale, err := e.viewStale(ctx, e.db.handle(), storeID)
		if err != nil || stale {
			return nil, false, err
		}
	}
	ids := []string{sub.id}
	if sub.id != "*" {
		ids = append(ids, "*")
	}
	v := viewObjects{storeID: storeID, gen: lv.gen, m: m, userType: sub.typ, userIDs: ids, objectType: req.Type, rels: []string{req.Relation}, sort: req.Sort, collation: e.collation()}
	page, args := v.query(after, req.PageSize+1)
	dir, cmp := "ASC", ">"
	if req.Sort.Descending {
		dir, cmp = "DESC", "<"
	}
	id := "c.obj_id" + e.collation()
	cursor := ""
	if after != nil {
		cursor = " WHERE " + id + " " + cmp + " ?"
		args = append(args, after.ID)
	}
	query := `SELECT s.gen, s.alive, COALESCE(c.id, '') AS id FROM (SELECT
	COALESCE((SELECT gen FROM gebra_view_state WHERE store_id = ?), 0) AS gen,
	(SELECT COUNT(*) FROM gebra_store WHERE id = ? AND deleted_at IS NULL) AS alive) s
LEFT JOIN LATERAL (SELECT c.obj_id AS id FROM (` + page + `) c` + cursor + ` ORDER BY ` + id + ` ` + dir + ` LIMIT ?) c ON TRUE
ORDER BY c.id` + e.collation() + ` ` + dir
	args = append([]any{storeID, storeID}, append(args, req.PageSize+1)...)
	conn, err := e.conn(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = conn.Close() }()
	type row struct {
		Gen   int64
		Alive int64
		ID    string
	}
	rows, err := queryStructs[row](ctx, dbtx{database: e.db, q: conn}, query, args...)
	if err != nil {
		return nil, false, errors.Wrap(err, "list view")
	}
	if len(rows) == 0 || rows[0].Alive == 0 || rows[0].Gen != lv.gen {
		e.live.Delete(storeID)
		return nil, false, nil
	}
	if o != nil {
		o.target(req.Type, req.Relation)
		o.setPath("view")
	}
	out := make([]cursorRow, 0, len(rows))
	for _, r := range rows {
		if r.ID != "" {
			out = append(out, cursorRow{ID: r.ID})
		}
	}
	return out, true, nil
}

// viewObjects reads from the view the objects of a type that subjects are in
// with any of the materialized relations: as a member of the object's set, or
// of a set that reaches it. Each part of the query reads one relation of the
// object and one relation of the subjects' sets, of those that can reach it,
// so that every part reads one range of an index in ID order.
type viewObjects struct {
	storeID    string
	gen        int64
	m          *compiledModel
	userType   string
	userIDs    []string
	objectType string
	rels       []string
	sort       Sort
	collation  string
}

// query returns the objects. With a limit, each part reads at most limit
// objects after the cursor in ID order, which include the first limit objects
// of all parts together, so that a page costs about the same however many
// objects the subjects can access.
func (v viewObjects) query(after *cursorRow, limit int) (string, []any) {
	dir, op := "ASC", ">"
	if v.sort.Descending {
		dir, op = "DESC", "<"
	}
	page := func(col string, args []any) (string, []any) {
		if limit == 0 {
			return "", args
		}
		var q string
		if after != nil {
			q = " AND " + col + v.collation + " " + op + " ?"
			args = append(args, after.ID)
		}
		return q + " ORDER BY " + col + v.collation + " " + dir + " LIMIT ?", append(args, limit)
	}
	var parts []string
	var args []any
	for _, id := range v.userIDs {
		for _, rel := range v.rels {
			q := `SELECT m.set_id AS obj_id FROM gebra_view_member m
WHERE m.store_id = ? AND m.gen = ? AND m.user_type = ? AND m.user_id = ? AND m.set_type = ? AND m.set_rel = ?`
			args = append(args, v.storeID, v.gen, v.userType, id, v.objectType, rel)
			p, a := page("m.set_id", nil)
			if limit > 0 {
				q = "(" + q + p + ")"
			}
			parts = append(parts, q)
			args = append(args, a...)
		}
	}
	for _, rel := range v.rels {
		for _, in := range v.m.inner[v.objectType][rel] {
			if limit == 0 {
				parts = append(parts, `SELECT r.anc_id FROM gebra_view_member m
JOIN gebra_view_reach r ON r.store_id = m.store_id AND r.gen = m.gen AND r.desc_key = m.set_key
WHERE m.store_id = ? AND m.gen = ? AND m.user_type = ? AND m.user_id IN ? AND m.set_type = ? AND m.set_rel = ? AND r.anc_type = ? AND r.anc_rel = ?`)
				args = append(args, v.storeID, v.gen, v.userType, v.userIDs, in[0], in[1], v.objectType, rel)
				continue
			}
			p, a := page("r.anc_id", []any{v.objectType, rel})
			parts = append(parts, `SELECT x.anc_id FROM gebra_view_member m CROSS JOIN LATERAL (
	SELECT r.anc_id FROM gebra_view_reach r
	WHERE r.store_id = m.store_id AND r.gen = m.gen AND r.desc_key = m.set_key AND r.anc_type = ? AND r.anc_rel = ?`+p+`
) x
WHERE m.store_id = ? AND m.gen = ? AND m.user_type = ? AND m.user_id IN ? AND m.set_type = ? AND m.set_rel = ?`)
			args = append(args, a...)
			args = append(args, v.storeID, v.gen, v.userType, v.userIDs, in[0], in[1])
		}
	}
	return strings.Join(parts, "\nUNION\n"), args
}

// cover returns materialized relations of the type whose union contains every
// object that the rewrite can grant, or false when the view cannot bound it.
// With allChildren, the relations cover every child of an intersection, which
// ListUsers needs because a user named in one child is listed when a wildcard
// satisfies the others.
func (m *compiledModel) cover(typ string, u *Userset, allChildren bool, visiting map[string]bool) ([]string, bool) {
	switch {
	case u.ComputedUserset != nil:
		rel := u.ComputedUserset.Relation
		r := m.relation(typ, rel)
		if r.materialized {
			return []string{rel}, true
		}
		if visiting[rel] {
			return nil, false
		}
		visiting[rel] = true
		defer delete(visiting, rel)
		return m.cover(typ, r.rewrite, allChildren, visiting)
	case u.Union != nil, u.Intersection != nil && allChildren:
		children := u.Union
		if children == nil {
			children = u.Intersection
		}
		var out []string
		for _, c := range children.Child {
			rels, ok := m.cover(typ, c, allChildren, visiting)
			if !ok {
				return nil, false
			}
			out = append(out, rels...)
		}
		slices.Sort(out)
		return slices.Compact(out), true
	case u.Intersection != nil:
		for _, c := range u.Intersection.Child {
			if rels, ok := m.cover(typ, c, allChildren, visiting); ok {
				return rels, true
			}
		}
	case u.Difference != nil:
		return m.cover(typ, u.Difference.Base, allChildren, visiting)
	}
	return nil, false
}

func (m *compiledModel) validateSort(typ string, s Sort) error {
	if s.Attribute == "" {
		return nil
	}
	if _, ok := m.types[typ].attributes[s.Attribute]; !ok {
		return invalidf("type %q has no attribute %q to sort by", typ, s.Attribute)
	}
	return nil
}

// UserFilter selects the users that ListUsers returns: objects of Type, or
// usersets Type#Relation when Relation is set.
type UserFilter struct {
	Type     string `json:"type"`
	Relation string `json:"relation,omitempty"`
}

// ListUsersRequest asks for the users that have a relation with an object.
type ListUsersRequest struct {
	Object               string
	Relation             string
	UserFilters          []UserFilter
	ContextualTuples     []TupleKey
	Context              map[string]any
	AuthorizationModelID string
	Consistency          Consistency
	Sort                 Sort
	// PageSize zero returns every result.
	PageSize          int
	ContinuationToken string
}

// User is a ListUsers result in the shape OpenFGA returns: exactly one field
// is set.
type User struct {
	Object   *UserObject   `json:"object,omitempty"`
	Userset  *UserUserset  `json:"userset,omitempty"`
	Wildcard *UserWildcard `json:"wildcard,omitempty"`
}

type UserObject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type UserUserset struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Relation string `json:"relation"`
}

type UserWildcard struct {
	Type string `json:"type"`
}

// ListUsers returns a sorted page of the users that have the relation with
// the object. A wildcard result means every user of the type, except users
// that an exclusion removes.
func (e *Engine) ListUsers(ctx context.Context, storeID string, req *ListUsersRequest) (users []User, _ string, err error) {
	ctx, o := e.startOp(ctx, "list_users", storeID)
	defer func() {
		o.end(err, attribute.Int("gebra.results", len(users)), attribute.Int("gebra.page_size", req.PageSize))
	}()
	if len(req.UserFilters) != 1 {
		return nil, "", invalidf("list users needs exactly one user filter")
	}
	filter := req.UserFilters[0]
	ev, err := e.newEvaluator(ctx, storeID, req.AuthorizationModelID, req.Consistency, req.ContextualTuples, req.Context, true)
	if err != nil {
		return nil, "", err
	}
	defer ev.end()
	obj, err := parseObject(req.Object)
	if err != nil {
		return nil, "", err
	}
	r := ev.m.relation(obj.typ, req.Relation)
	if r == nil {
		return nil, "", invalidf("relation %s#%s is not defined", obj.typ, req.Relation)
	}
	o.target(obj.typ, req.Relation)
	if err := ev.m.validateSubject(subject{typ: filter.Type, rel: filter.Relation}); err != nil {
		return nil, "", err
	}
	if err := ev.m.validateSort(filter.Type, req.Sort); err != nil {
		return nil, "", err
	}
	after, err := parseCursor(req.ContinuationToken)
	if err != nil {
		return nil, "", err
	}
	// Changes that cannot reach the object do not change its users.
	scope := &pendingScope{userType: filter.Type, sets: newScopeSets()}
	ev.m.addScope(scope.sets, obj, req.Relation, map[string]bool{})
	if err := ev.settle(scope, true, false); err != nil {
		return nil, "", err
	}

	src := &sortedSource{storeID: storeID, objectType: filter.Type, sort: req.Sort}
	verify := true
	if ev.useView && filter.Relation == "" {
		rels := []string{req.Relation}
		ok := r.materialized
		if ok {
			verify = false
		} else {
			rels, ok = ev.m.cover(obj.typ, r.rewrite, true, map[string]bool{req.Relation: true})
		}
		if ok {
			keys := make([]string, len(rels))
			for i, rel := range rels {
				keys[i] = setKey(obj.typ, obj.id, rel)
			}
			src.sub = `SELECT DISTINCT user_id AS obj_id FROM gebra_view_member
WHERE store_id = ? AND gen = ? AND user_type = ? AND (set_key IN ? OR set_key IN (
	SELECT desc_key FROM gebra_view_reach WHERE store_id = ? AND gen = ? AND anc_type = ? AND anc_id = ? AND anc_rel IN ?
))`
			src.args = []any{storeID, ev.gen, filter.Type, keys, storeID, ev.gen, obj.typ, obj.id, rels}
		}
	}
	var extras []string
	// Candidates from the view are named by tuples on granting paths, but
	// candidates from all stored tuples need that checked too.
	explicit := false
	if src.sub == "" {
		explicit = true
		// A userset filter also matches objects that tuple to usersets reach
		// through tuples without a relation on the user.
		userRels := []string{filter.Relation, ""}
		wildcardOK := filter.Relation == ""
		src.sub = `SELECT user_id AS obj_id FROM gebra_tuple WHERE store_id = ? AND user_type = ? AND user_relation IN ? AND (? OR user_id <> '*')
UNION
SELECT object_id FROM gebra_tuple WHERE store_id = ? AND object_type = ?`
		src.args = []any{storeID, filter.Type, userRels, wildcardOK, storeID, filter.Type}
		if ev.applied {
			src.sub += `
UNION
SELECT user_id FROM gebra_pending WHERE store_id = ? AND user_type = ? AND user_relation IN ? AND (? OR user_id <> '*') AND deleted = TRUE
UNION
SELECT object_id FROM gebra_pending WHERE store_id = ? AND object_type = ? AND deleted = TRUE`
			src.args = append(src.args, storeID, filter.Type, userRels, wildcardOK, storeID, filter.Type)
		}
		for _, ts := range ev.contextual {
			for _, t := range ts {
				if t.userType == filter.Type && slices.Contains(userRels, t.userRelation) && (wildcardOK || t.userID != "*") {
					extras = append(extras, t.userID)
				}
				if t.objectType == filter.Type {
					extras = append(extras, t.objectID)
				}
			}
		}
	}

	check := func(ev *evaluator, id string) (bool, error) {
		sub := subject{typ: filter.Type, id: id, rel: filter.Relation}
		ok, err := ev.check(obj, req.Relation, sub)
		if !ok || err != nil || !explicit || id == "*" || filter.Relation != "" {
			return ok, err
		}
		if ev.explicit == nil {
			ev.explicit = ev.fork()
			ev.explicit.useView = false
			ev.explicit.explicitOnly = true
		}
		// A user whose explicit path depends on a missing condition parameter
		// is only covered by the wildcard.
		named, err := ev.explicit.check(obj, req.Relation, sub)
		return named && err == nil, nil
	}
	// A userset of the object itself, e.g., doc:1#viewer for doc:1#viewer, is
	// in the relation without any tuple naming it.
	if filter.Relation != "" && filter.Type == obj.typ {
		extras = append(extras, obj.id)
	}
	var rows []cursorRow
	err = e.walkSorted(ev, src, after, extras, verify, req.PageSize, check, func(row cursorRow) error {
		rows = append(rows, row)
		var u User
		switch {
		case row.ID == "*":
			u.Wildcard = &UserWildcard{Type: filter.Type}
		case filter.Relation != "":
			u.Userset = &UserUserset{Type: filter.Type, ID: row.ID, Relation: filter.Relation}
		default:
			u.Object = &UserObject{Type: filter.Type, ID: row.ID}
		}
		users = append(users, u)
		if req.PageSize > 0 && len(users) > req.PageSize {
			return errStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, errStop) {
		return nil, "", err
	}
	var next string
	if req.PageSize > 0 && len(users) > req.PageSize {
		users = users[:req.PageSize]
		next = rows[req.PageSize-1].token()
	}
	return users, next, nil
}

// sortedSource is a SQL query that yields candidate IDs in the column obj_id,
// read in sort order with keyset pagination.
type sortedSource struct {
	storeID    string
	objectType string
	sort       Sort
	sub        string
	args       []any
	// paged, when set, returns a query like sub that may leave out
	// candidates after the first limit after the cursor.
	paged func(after *cursorRow, limit int) (string, []any)
}

type cursorRow struct {
	V  string `json:"v,omitempty"`
	ID string `json:"id"`
}

func (r cursorRow) token() string {
	b, _ := json.Marshal(r)
	return encodeToken(string(b))
}

func parseCursor(token string) (*cursorRow, error) {
	if token == "" {
		return nil, nil
	}
	s, err := decodeToken(token)
	if err != nil {
		return nil, err
	}
	var c cursorRow
	if err := json.Unmarshal([]byte(s), &c); err != nil || c.ID == "" {
		return nil, invalidf("invalid continuation token")
	}
	return &c, nil
}

func (s *sortedSource) less(a, b cursorRow) bool {
	if s.sort.Descending {
		a, b = b, a
	}
	return a.V < b.V || a.V == b.V && a.ID < b.ID
}

// collation makes the database compare strings byte by byte, as Go does, so
// keyset pagination and in-memory merging agree on the order.
func (e *Engine) collation() string {
	switch e.db.dialect {
	case DialectPostgres:
		return ` COLLATE "C"`
	case DialectMySQL:
		return " COLLATE utf8mb4_bin"
	}
	return ""
}

func (e *Engine) fetchPage(ev *evaluator, s *sortedSource, after *cursorRow, limit int) ([]cursorRow, error) {
	coll := e.collation()
	dir, op := "ASC", ">"
	if s.sort.Descending {
		dir, op = "DESC", "<"
	}
	sub, args := s.sub, slices.Clone(s.args)
	if s.paged != nil {
		sub, args = s.paged(after, limit)
	}
	id := "c.obj_id" + coll
	var q string
	if s.sort.Attribute == "" {
		q = "SELECT c.obj_id AS id, '' AS v FROM (" + sub + ") c"
		if after != nil {
			q += " WHERE " + id + " " + op + " ?"
			args = append(args, after.ID)
		}
		q += " ORDER BY " + id + " " + dir
	} else {
		v := "COALESCE(a.value, '')" + coll
		q = "SELECT c.obj_id AS id, COALESCE(a.value, '') AS v FROM (" + sub + ") c" +
			" LEFT JOIN gebra_object_attribute a ON a.store_id = ? AND a.object_type = ? AND a.name = ? AND a.object_id = c.obj_id"
		args = append(args, s.storeID, s.objectType, s.sort.Attribute)
		if after != nil {
			q += " WHERE (" + v + " " + op + " ? OR (" + v + " = ? AND " + id + " " + op + " ?))"
			args = append(args, after.V, after.V, after.ID)
		}
		q += " ORDER BY " + v + " " + dir + ", " + id + " " + dir
	}
	q += " LIMIT ?"
	args = append(args, limit)

	var rows []cursorRow
	err := ev.run(func(db dbtx) (err error) {
		rows, err = queryStructs[cursorRow](ev.ctx, db, q, args...)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "list candidates")
	}
	return rows, nil
}

// extraRows turns IDs that only contextual tuples mention into sorted rows
// after the cursor.
func (e *Engine) extraRows(ev *evaluator, s *sortedSource, ids []string, after *cursorRow) ([]cursorRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var stored []string
	err := ev.run(func(db dbtx) (err error) {
		stored, err = queryColumn[string](ev.ctx, db, "SELECT c.obj_id FROM ("+s.sub+") c WHERE c.obj_id IN ?", append(slices.Clone(s.args), ids)...)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "find stored candidates")
	}
	values := map[string]string{}
	if s.sort.Attribute != "" {
		var attrs []attributeRow
		err := ev.run(func(db dbtx) (err error) {
			attrs, err = queryStructs[attributeRow](ev.ctx, db, "SELECT object_id, value FROM gebra_object_attribute WHERE store_id = ? AND object_type = ? AND name = ? AND object_id IN ?", s.storeID, s.objectType, s.sort.Attribute, ids)
			return err
		})
		if err != nil {
			return nil, errors.Wrap(err, "read attributes")
		}
		for _, a := range attrs {
			values[a.ObjectID] = a.Value
		}
	}
	var rows []cursorRow
	for _, id := range ids {
		r := cursorRow{V: values[id], ID: id}
		if slices.Contains(stored, id) || after != nil && !s.less(*after, r) {
			continue
		}
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b cursorRow) int {
		if s.less(a, b) {
			return -1
		}
		return 1
	})
	return rows, nil
}

// verifyWorkers is how many checks run at the same time to filter candidates
// that the view over-approximates.
const verifyWorkers = 8

// walkSorted emits the candidates of the source, merged with extra IDs, in
// sort order. When verify is true, only candidates that pass check are
// emitted. Without verify, a page size makes the first read one page and one
// more row, which tells whether another page follows.
func (e *Engine) walkSorted(ev *evaluator, s *sortedSource, after *cursorRow, extraIDs []string, verify bool, pageSize int, check func(*evaluator, string) (bool, error), emit func(cursorRow) error) error {
	extras, err := e.extraRows(ev, s, extraIDs, after)
	if err != nil {
		return err
	}
	batch := 1000
	if pageSize > 0 {
		batch = min(batch, pageSize+1)
	}
	var workers []*evaluator
	if verify {
		batch = 200
		workers = make([]*evaluator, verifyWorkers)
		for i := range workers {
			workers[i] = ev.fork()
		}
	}

	for {
		if err := ev.ctx.Err(); err != nil {
			return err
		}
		rows, err := e.fetchPage(ev, s, after, batch)
		if err != nil {
			return err
		}
		done := len(rows) < batch

		merged := make([]cursorRow, 0, len(rows))
		for _, r := range rows {
			for len(extras) > 0 && s.less(extras[0], r) {
				merged = append(merged, extras[0])
				extras = extras[1:]
			}
			merged = append(merged, r)
		}
		if done {
			merged = append(merged, extras...)
		}

		// Verify in small chunks so that a full page stops verification early.
		chunk := len(merged)
		if verify {
			chunk = 4 * verifyWorkers
		}
		for len(merged) > 0 {
			n := min(chunk, len(merged))
			ok := make([]bool, n)
			if verify {
				if err := verifyRows(workers, merged[:n], ok, check); err != nil {
					return err
				}
			}
			for i, r := range merged[:n] {
				if !verify || ok[i] {
					if err := emit(r); err != nil {
						return err
					}
				}
			}
			merged = merged[n:]
		}
		if done {
			return nil
		}
		after = &rows[len(rows)-1]
		// Each fetch sorts every candidate after the cursor, so a fixed batch
		// would make reading N candidates cost N²/batch. Growing it keeps a
		// first page cheap and a full list close to linear.
		batch = min(2*batch, maxListBatch)
	}
}

// maxListBatch bounds how many candidates a list holds in memory at once.
const maxListBatch = 100_000

func verifyRows(workers []*evaluator, rows []cursorRow, ok []bool, check func(*evaluator, string) (bool, error)) error {
	next := make(chan int)
	errs := make([]error, len(workers))
	var wg sync.WaitGroup
	for w, ev := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if errs[w] != nil {
					continue
				}
				ok[i], errs[w] = check(ev, rows[i].ID)
			}
		}()
	}
	for i := range rows {
		next <- i
	}
	close(next)
	wg.Wait()
	return errors.Join(errs...)
}
