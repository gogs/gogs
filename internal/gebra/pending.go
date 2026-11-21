package gebra

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// A query whose consistency asks for writes that the view lacks can still use
// the view when no pending change can affect its answer: the view then
// answers as the latest state would, and that state includes every write the
// query asks for.
//
// The view answers whether a subject is in a materialized set. Under the view
// model, every tuple change touches member rows and edges, which viewEffects
// returns, and the view state that includes the pending changes differs from
// the applied one only in touched rows. A plain subject U is in a set X when a
// member row puts U, or its type's wildcard, in a set k with a path of edges
// from k to X. Take a path that exists in one of the two states and not in
// the other, and its touched element nearest to X: the rest of the path up to
// X is untouched, so it exists in the applied view too. That element is
//
//   - a touched member row of U in a set k that reaches X in the applied view,
//     or
//   - a touched edge c → p whose parent p reaches X in the applied view, and
//     whose child c the path reaches from U. Every element of that part of the
//     path exists in the applied view or is touched, so U reaches c in the
//     applied view plus all touched rows. reachedChildren computes that
//     closure: from the applied view, from touched members, and from the
//     parents of touched edges whose child it reached already.
//
// A userset subject S is in X when a path of edges leads from S to X, which
// the same argument covers with S in place of the member row.
//
// Once the view answers such a query, evaluation reads the latest tuples of
// the sets of relations that the view does not hold, so changes of those sets
// cannot make its answer wrong. They count as relevant anyway, as do changes
// of tuples that can have a condition, which only such relations hold. With
// more than maxPendingChanges pending, every change counts as relevant.

// maxPendingChanges bounds the pending changes that a query reads to decide
// whether they affect its answer. With more, they all count as relevant.
const maxPendingChanges = 1000

// pendingChange is a tuple change that the view does not include yet. A
// write's condition is unknown.
type pendingChange struct {
	ObjectType, ObjectID, Relation string
	UserType, UserID, UserRelation string
	ConditionName                  string
	Deleted                        bool
}

// conditional reports whether the change's tuple may have a condition.
func (ch *pendingChange) conditional(m *compiledModel) bool {
	if ch.ConditionName != "" {
		return true
	}
	r := m.relation(ch.ObjectType, ch.Relation)
	if r == nil || ch.Deleted {
		return false
	}
	for _, ref := range r.direct {
		if ref.Condition != "" && ref.Type == ch.UserType && ref.Relation == ch.UserRelation && (ref.Wildcard != nil) == (ch.UserID == "*") {
			return true
		}
	}
	return false
}

// effects returns what the change touches in the view under the model. A
// conditional tuple touches nothing, since only relations without conditions
// are materialized.
func (ch *pendingChange) effects(m *compiledModel) (*viewMemberRow, []viewEdge) {
	return m.viewEffects(&tuple{
		objectType: ch.ObjectType, objectID: ch.ObjectID, relation: ch.Relation,
		userType: ch.UserType, userID: ch.UserID, userRelation: ch.UserRelation,
	})
}

// pendingScope describes what a query reads, so that pending changes that
// cannot affect its answer can be told apart.
type pendingScope struct {
	// sub is the query's subject, or nil for a query about every subject of
	// userType.
	sub      *subject
	userType string
	// sets holds the materialized sets whose members the query asks for, and
	// the sets whose tuples it reads.
	sets *scopeSets
}

// scopeSets holds sets of two kinds, each by key or, for sets of any object,
// by type and relation.
type scopeSets struct {
	leaves, reads       map[string]bool
	anyLeaves, anyReads map[[2]string]bool
}

func newScopeSets() *scopeSets {
	return &scopeSets{
		leaves: map[string]bool{}, reads: map[string]bool{},
		anyLeaves: map[[2]string]bool{}, anyReads: map[[2]string]bool{},
	}
}

func (s *scopeSets) add(byKey map[string]bool, byRelation map[[2]string]bool, obj object, rel string) {
	if obj.id == "*" {
		byRelation[[2]string{obj.typ, rel}] = true
	} else {
		byKey[setKey(obj.typ, obj.id, rel)] = true
	}
}

func (s *scopeSets) isLeaf(key string) bool {
	typ, _, rel := splitSetKey(key)
	return s.leaves[key] || s.anyLeaves[[2]string{typ, rel}]
}

func (s *scopeSets) isRead(typ, id, rel string) bool {
	return s.reads[setKey(typ, id, rel)] || s.anyReads[[2]string{typ, rel}]
}

// addScope adds the sets that a check of rel on obj may ask the view about or
// read tuples of. An object ID "*" stands for every object of the type.
func (m *compiledModel) addScope(s *scopeSets, obj object, rel string, visiting map[string]bool) {
	key := setKey(obj.typ, obj.id, rel)
	if visiting[key] {
		return
	}
	visiting[key] = true
	r := m.relation(obj.typ, rel)
	if r.materialized {
		s.add(s.leaves, s.anyLeaves, obj, rel)
		return
	}
	walkUserset(r.rewrite, func(u *Userset) {
		switch {
		case u.This != nil:
			s.add(s.reads, s.anyReads, obj, rel)
			for _, ref := range r.direct {
				if ref.Relation != "" {
					m.addScope(s, object{typ: ref.Type, id: "*"}, ref.Relation, visiting)
				}
			}
		case u.ComputedUserset != nil:
			m.addScope(s, obj, u.ComputedUserset.Relation, visiting)
		case u.TupleToUserset != nil:
			ts := u.TupleToUserset.Tupleset.Relation
			s.add(s.reads, s.anyReads, obj, ts)
			for _, ref := range m.relation(obj.typ, ts).direct {
				if m.relation(ref.Type, u.TupleToUserset.ComputedUserset.Relation) != nil {
					m.addScope(s, object{typ: ref.Type, id: "*"}, u.TupleToUserset.ComputedUserset.Relation, visiting)
				}
			}
		}
	})
}

// relevantSources reports whether any pending change that prepare read can
// affect the answer of a query with the scope, and when every such change is
// a member or an edge that the argument above finds, the sets they touch
// nearest to the leaves: the set of each such member and the parent of each
// such edge. Only leaves that these sets reach in the applied view, or that
// their computed closures contain, can have other members in the latest
// state. Sources are nil when a change counts as relevant for another reason.
func (ev *evaluator) relevantSources(scope *pendingScope) (bool, []string, error) {
	if ev.pendingOverflow {
		return true, nil, nil
	}
	if len(ev.pending) == 0 {
		return false, nil, nil
	}
	if scope == nil {
		return true, nil, nil
	}
	var members []string
	var edges []viewEdge
	for _, ch := range ev.pending {
		if scope.sets.isRead(ch.ObjectType, ch.ObjectID, ch.Relation) || ch.conditional(ev.m) {
			return true, nil, nil
		}
		mr, es := ch.effects(ev.m)
		if mr != nil && scope.matchesMember(mr) {
			members = append(members, mr.SetKey)
		}
		edges = append(edges, es...)
	}
	if len(members) == 0 && len(edges) == 0 {
		return false, nil, nil
	}
	r, err := ev.readReach(scope, members, edges)
	if err != nil {
		return false, nil, err
	}
	var sources []string
	for _, k := range members {
		if r.reachesLeaf(k) {
			sources = append(sources, k)
		}
	}
	reached := r.reachedChildren(scope, members, edges)
	for _, e := range edges {
		if (scope.sub == nil || reached[e.child]) && r.reachesLeaf(e.parent) {
			sources = append(sources, e.parent)
		}
	}
	return len(sources) > 0, sources, nil
}

// affectedObjects returns the IDs of the objects of typ whose rel set the
// sources that relevantSources returned reach in the applied view, or contain
// through computed usersets. The view's answer is that of the latest state
// for every other object of typ.
func (ev *evaluator) affectedObjects(sources []string, typ, rel string) ([]string, error) {
	descs := map[string]bool{}
	ids := map[string]bool{}
	for _, s := range sources {
		for _, d := range ev.m.computedClosure(s) {
			descs[d] = true
			if t, id, r := splitSetKey(d); t == typ && r == rel {
				ids[id] = true
			}
		}
	}
	var above []string
	err := ev.run(func(db dbtx) (err error) {
		above, err = queryColumn[string](ev.ctx, db, `SELECT DISTINCT anc_id FROM gebra_view_reach
WHERE store_id = ? AND gen = ? AND desc_key IN ? AND anc_type = ? AND anc_rel = ?`,
			ev.storeID, ev.gen, mapKeys(descs), typ, rel)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "read affected objects")
	}
	for _, id := range above {
		ids[id] = true
	}
	return mapKeys(ids), nil
}

// matchesMember reports whether a member row can put the scope's subject in
// its set. Member rows never put a userset in a set.
func (s *pendingScope) matchesMember(mr *viewMemberRow) bool {
	if s.sub == nil {
		return mr.UserType == s.userType
	}
	return s.sub.rel == "" && mr.UserType == s.sub.typ && (mr.UserID == s.sub.id || mr.UserID == "*")
}

// reachInfo is what the applied view says about the sets that decide whether
// pending changes are relevant.
type reachInfo struct {
	m      *compiledModel
	leaves *scopeSets
	// closures holds the computed closure of every source set.
	closures map[string][]string
	// above holds the ancestors that reach rows give a set, among the
	// children of touched edges and the leaves, and anyAbove the types and
	// relations of its ancestors among leaves of any object.
	above    map[string]map[string]bool
	anyAbove map[string]map[[2]string]bool
	// subjectIn holds the children of touched edges that the applied view
	// puts a plain subject in.
	subjectIn map[string]bool
}

// reaches reports whether a path in the applied view leads from set a, which
// readReach read as a source, to set b.
func (r *reachInfo) reaches(a, b string) bool {
	for _, d := range r.closures[a] {
		if d == b || r.above[d][b] {
			return true
		}
	}
	return false
}

// reachesLeaf reports whether a path in the applied view leads from set a,
// which readReach read as a source, to a leaf of the scope.
func (r *reachInfo) reachesLeaf(a string) bool {
	for _, d := range r.closures[a] {
		if r.leaves.isLeaf(d) {
			return true
		}
		for b := range r.above[d] {
			if r.leaves.isLeaf(b) {
				return true
			}
		}
		for p := range r.anyAbove[d] {
			if r.leaves.anyLeaves[p] {
				return true
			}
		}
	}
	return false
}

// reachedChildren returns the children of touched edges that the scope's
// subject reaches in the applied view plus every touched row.
func (r *reachInfo) reachedChildren(scope *pendingScope, members []string, edges []viewEdge) map[string]bool {
	reached := map[string]bool{}
	if scope.sub == nil {
		return reached
	}
	sources := members
	if scope.sub.rel != "" {
		sources = []string{setKey(scope.sub.typ, scope.sub.id, scope.sub.rel)}
	}
	var frontier []string
	visit := func(c string) {
		if !reached[c] {
			reached[c] = true
			frontier = append(frontier, c)
		}
	}
	for _, e := range edges {
		if r.subjectIn[e.child] {
			visit(e.child)
		}
		for _, s := range sources {
			if r.reaches(s, e.child) {
				visit(e.child)
			}
		}
	}
	for len(frontier) > 0 {
		c := frontier[0]
		frontier = frontier[1:]
		for _, e := range edges {
			if e.child != c {
				continue
			}
			for _, next := range edges {
				if r.reaches(e.parent, next.child) {
					visit(next.child)
				}
			}
		}
	}
	return reached
}

// readReach reads, in one statement, the reach rows of the evaluator's view
// generation from the sources, i.e., the sets of touched members, the parents
// of touched edges, and a userset subject, to the children of touched edges
// and the leaves, and the children of touched edges that the applied view puts
// a plain subject in.
func (ev *evaluator) readReach(scope *pendingScope, members []string, edges []viewEdge) (*reachInfo, error) {
	r := &reachInfo{
		m: ev.m, leaves: scope.sets, closures: map[string][]string{},
		above: map[string]map[string]bool{}, anyAbove: map[string]map[[2]string]bool{}, subjectIn: map[string]bool{},
	}
	sources := append([]string(nil), members...)
	var children []string
	for _, e := range edges {
		sources = append(sources, e.parent)
		children = append(children, e.child)
	}
	if scope.sub != nil && scope.sub.rel != "" {
		sources = append(sources, setKey(scope.sub.typ, scope.sub.id, scope.sub.rel))
	}
	descs := map[string]bool{}
	for _, s := range sources {
		if _, ok := r.closures[s]; ok {
			continue
		}
		r.closures[s] = ev.m.computedClosure(s)
		for _, d := range r.closures[s] {
			descs[d] = true
		}
	}
	targets := append([]string(nil), children...)
	for k := range scope.sets.leaves {
		targets = append(targets, k)
	}

	// Each part filters the columns of ancestors with separate lists, which
	// select more rows than asked for but keep the statement simple to plan.
	// The rows are filtered here.
	var parts []string
	var args []any
	descKeys := mapKeys(descs)
	if len(targets) > 0 {
		types, ids, rels := splitKeys(targets)
		parts = append(parts, `SELECT 'r' AS kind, desc_key, anc_type, anc_id, anc_rel FROM gebra_view_reach
WHERE store_id = ? AND gen = ? AND desc_key IN ? AND anc_type IN ? AND anc_id IN ? AND anc_rel IN ?`)
		args = append(args, ev.storeID, ev.gen, descKeys, types, ids, rels)
	}
	if len(scope.sets.anyLeaves) > 0 {
		var types, rels []string
		for p := range scope.sets.anyLeaves {
			types = append(types, p[0])
			rels = append(rels, p[1])
		}
		parts = append(parts, `SELECT DISTINCT 'w' AS kind, desc_key, anc_type, '' AS anc_id, anc_rel FROM gebra_view_reach
WHERE store_id = ? AND gen = ? AND desc_key IN ? AND anc_type IN ? AND anc_rel IN ?`)
		args = append(args, ev.storeID, ev.gen, descKeys, types, rels)
	}
	if sub := scope.sub; sub != nil && sub.rel == "" && len(children) > 0 {
		types, ids, rels := splitKeys(children)
		// As in memberQuery, the wildcard has parts with a literal '*'.
		userIDs := []string{"'*'"}
		if sub.id != "*" {
			userIDs = append(userIDs, "?")
		}
		for _, userID := range userIDs {
			parts = append(parts, `SELECT 'm' AS kind, set_key AS desc_key, '' AS anc_type, '' AS anc_id, '' AS anc_rel FROM gebra_view_member
WHERE store_id = ? AND gen = ? AND set_key IN ? AND user_type = ? AND user_id = `+userID, `SELECT 'n' AS kind, '' AS desc_key, r.anc_type, r.anc_id, r.anc_rel FROM gebra_view_member m
JOIN gebra_view_reach r ON r.store_id = m.store_id AND r.gen = m.gen AND r.desc_key = m.set_key
WHERE m.store_id = ? AND m.gen = ? AND m.user_type = ? AND m.user_id = `+userID+` AND r.anc_type IN ? AND r.anc_id IN ? AND r.anc_rel IN ?`)
			if userID == "?" {
				args = append(args, ev.storeID, ev.gen, children, sub.typ, sub.id, ev.storeID, ev.gen, sub.typ, sub.id, types, ids, rels)
			} else {
				args = append(args, ev.storeID, ev.gen, children, sub.typ, ev.storeID, ev.gen, sub.typ, types, ids, rels)
			}
		}
	}
	if len(parts) == 0 {
		return r, nil
	}
	type reachRead struct {
		Kind, DescKey, AncType, AncID, AncRel string
	}
	var rows []reachRead
	err := ev.run(func(db dbtx) (err error) {
		rows, err = queryStructs[reachRead](ev.ctx, db, strings.Join(parts, "\nUNION ALL\n"), args...)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "read view reach")
	}
	isChild := map[string]bool{}
	for _, c := range children {
		isChild[c] = true
	}
	for _, row := range rows {
		anc := setKey(row.AncType, row.AncID, row.AncRel)
		switch row.Kind {
		case "r":
			if r.above[row.DescKey] == nil {
				r.above[row.DescKey] = map[string]bool{}
			}
			r.above[row.DescKey][anc] = true
		case "w":
			if r.anyAbove[row.DescKey] == nil {
				r.anyAbove[row.DescKey] = map[[2]string]bool{}
			}
			r.anyAbove[row.DescKey][[2]string{row.AncType, row.AncRel}] = true
		case "m":
			r.subjectIn[row.DescKey] = true
		case "n":
			if isChild[anc] {
				r.subjectIn[anc] = true
			}
		}
	}
	return r, nil
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// splitKeys returns the distinct types, IDs and relations of the set keys.
func splitKeys(sets []string) (types, ids, rels []string) {
	seen := [3]map[string]bool{{}, {}, {}}
	add := func(i int, s string, out *[]string) {
		if !seen[i][s] {
			seen[i][s] = true
			*out = append(*out, s)
		}
	}
	for _, k := range sets {
		typ, id, rel := splitSetKey(k)
		add(0, typ, &types)
		add(1, id, &ids)
		add(2, rel, &rels)
	}
	return types, ids, rels
}
