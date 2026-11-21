package gebra

import (
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// viewEdge says that set child is contained in set parent.
type viewEdge struct {
	child, parent string
}

// viewEffects returns what a tuple contributes to the view: a direct member
// when the tuple puts a subject in a materialized set, and the edges when it
// puts a set inside one or more materialized sets.
func (m *compiledModel) viewEffects(t *tuple) (member *viewMemberRow, edges []viewEdge) {
	if !m.allows(t) {
		return nil, nil
	}
	r := m.relation(t.objectType, t.relation)
	set := setKey(t.objectType, t.objectID, t.relation)
	if r.materialized {
		if t.userRelation == "" {
			member = &viewMemberRow{
				SetKey: set, SetType: t.objectType, SetID: t.objectID, SetRel: t.relation,
				UserType: t.userType, UserID: t.userID,
			}
		} else {
			edges = append(edges, viewEdge{child: setKey(t.userType, t.userID, t.userRelation), parent: set})
		}
	}
	// A tuple on a tupleset relation links its object to the user object's
	// relations that tuple to usersets read.
	if t.userRelation == "" && t.userID != "*" {
		for _, e := range m.ttuEdges[t.objectType][t.relation] {
			if m.relation(t.userType, e.target) == nil {
				continue
			}
			edges = append(edges, viewEdge{
				child:  setKey(t.userType, t.userID, e.target),
				parent: setKey(t.objectType, t.objectID, e.parent),
			})
		}
	}
	return member, edges
}

// implicitParents returns the sets that contain the given set because their
// relation includes its relation as a computed userset on the same object.
func (m *compiledModel) implicitParents(key string) []string {
	typ, id, rel := splitSetKey(key)
	parents := m.computedParents[typ][rel]
	out := make([]string, len(parents))
	for i, p := range parents {
		out[i] = setKey(typ, id, p)
	}
	return out
}

// bfsUp returns every set that contains start, directly or indirectly,
// excluding start itself. explicitParents returns the parents that edges give
// a batch of sets.
func bfsUp(m *compiledModel, start string, explicitParents func(children []string) ([]string, error)) (map[string]struct{}, error) {
	seen := map[string]struct{}{}
	frontier := []string{start}
	for len(frontier) > 0 {
		parents, err := explicitParents(frontier)
		if err != nil {
			return nil, err
		}
		for _, c := range frontier {
			parents = append(parents, m.implicitParents(c)...)
		}
		frontier = frontier[:0:0]
		for _, p := range parents {
			if _, ok := seen[p]; ok || p == start {
				continue
			}
			seen[p] = struct{}{}
			frontier = append(frontier, p)
		}
	}
	return seen, nil
}

func reachRow(storeID string, gen int64, anc, desc string) viewReachRow {
	at, aid, arel := splitSetKey(anc)
	dt, did, drel := splitSetKey(desc)
	return viewReachRow{
		StoreID: storeID, Gen: gen, DescKey: desc, DescType: dt, DescID: did, DescRel: drel,
		AncType: at, AncID: aid, AncRel: arel,
	}
}

const sqlBatchSize = 500

// sqlRowBatchSize bounds lists of row values, e.g., (a, b) IN ((?, ?), ...).
// Postgres plans such a list as an OR of conditions in time that grows with
// the square of its length: 500 rows of four columns take about 100 ms to
// plan.
const sqlRowBatchSize = 100

func chunks[T any](s []T) [][]T { return chunksOf(s, sqlBatchSize) }

func rowChunks[T any](s []T) [][]T { return chunksOf(s, sqlRowBatchSize) }

func chunksOf[T any](s []T, size int) [][]T {
	var out [][]T
	for len(s) > size {
		out = append(out, s[:size])
		s = s[size:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

// viewMaintainer applies tuple changes to a generation of the view inside an
// applier transaction. The store's applier is the only writer of its view, so
// the maintainer takes no locks.
type viewMaintainer struct {
	ctx     context.Context
	tx      dbtx
	storeID string
	gen     int64
	m       *compiledModel
	// noReach makes the maintainer change only member and edge rows, for a
	// generation whose build has not reached its reach rows yet.
	noReach bool
	// strict makes inserting a member row that exists fail, for a build that
	// must not cover a tuple twice.
	strict bool
	// covered, when set, returns which of the sets have reach rows in a
	// generation whose build covered only some sets so far. The maintainer
	// keeps the reach rows of covered sets and leaves the others to the build.
	covered func(sets []string) (map[string]bool, error)

	memberAdds, memberDels []viewMemberRow
	memberSets             map[string]struct{}
	edgeDelta              map[viewEdge]int64

	// wasAnchored and isAnchored hold whether the sets whose anchor can
	// change were anchored before and after the batch. Every other set that
	// has reach rows is anchored both times.
	wasAnchored, isAnchored map[string]bool
	// parents caches the parents that edges give the sets that walkUp
	// visited, once the batch changed the edges.
	parents map[string][]string
	// reachInserts are reach rows to insert when the batch ends.
	reachInserts []viewReachRow

	// added and removed count the rows inserted into and deleted from each
	// view table.
	added, removed viewCounts
}

// net returns how many rows each view table gained, negative when it lost
// rows.
func (v *viewMaintainer) net() viewCounts {
	return viewCounts{
		member: v.added.member - v.removed.member,
		edge:   v.added.edge - v.removed.edge,
		reach:  v.added.reach - v.removed.reach,
	}
}

// viewCounts holds a number per view table: rows held, or rows gained.
type viewCounts struct {
	member, edge, reach int64
}

func newViewMaintainer(ctx context.Context, tx dbtx, storeID string, gen int64, m *compiledModel) *viewMaintainer {
	return &viewMaintainer{
		ctx: ctx, tx: tx, storeID: storeID, gen: gen, m: m,
		memberSets: map[string]struct{}{}, edgeDelta: map[viewEdge]int64{},
		parents: map[string][]string{},
	}
}

// record records what the tuples that a batch removes and adds change in the
// view.
func (v *viewMaintainer) record(removed, added []*tuple) {
	collect := func(ts []*tuple, delta int64) {
		for _, t := range ts {
			member, edges := v.m.viewEffects(t)
			if member != nil {
				member.StoreID, member.Gen = v.storeID, v.gen
				if delta > 0 {
					v.memberAdds = append(v.memberAdds, *member)
				} else {
					v.memberDels = append(v.memberDels, *member)
				}
				v.memberSets[member.SetKey] = struct{}{}
			}
			for _, e := range edges {
				v.edgeDelta[e] += delta
			}
		}
	}
	collect(removed, -1)
	collect(added, 1)
}

// apply updates the view for the changes that record recorded, with a
// fixed number of statements per step for the whole batch.
func (v *viewMaintainer) apply() error {
	if len(v.memberSets) == 0 && len(v.edgeDelta) == 0 {
		return nil
	}
	// Sets that gain members or appear as an edge child may gain their anchor,
	// and sets that lose them may lose it.
	candidates := maps.Clone(v.memberSets)
	for e, d := range v.edgeDelta {
		if d != 0 {
			candidates[e.child] = struct{}{}
		}
	}
	sets := slices.Collect(maps.Keys(candidates))
	var wasAnchored map[string]bool
	var err error
	if !v.noReach {
		if wasAnchored, err = v.anchored(sets); err != nil {
			return err
		}
	}
	appeared, disappeared, err := v.applyEdges(v.edgeDelta)
	if err != nil {
		return err
	}
	// Deletes go first: an applier batch can remove a tuple and add it again.
	for _, batch := range rowChunks(v.memberDels) {
		keys := make([][]any, len(batch))
		for i, m := range batch {
			keys[i] = []any{m.SetKey, m.UserType, m.UserID}
		}
		n, err := v.tx.exec(v.ctx, "DELETE FROM gebra_view_member WHERE store_id = ? AND gen = ? AND (set_key, user_type, user_id) IN ?", v.storeID, v.gen, keys)
		if err != nil {
			return errors.Wrap(err, "delete view members")
		}
		v.removed.member += n
	}
	if len(v.memberAdds) > 0 {
		values := make([][]any, len(v.memberAdds))
		for i, m := range v.memberAdds {
			values[i] = []any{m.StoreID, m.Gen, m.SetKey, m.UserType, m.UserID, m.SetType, m.SetRel, m.SetID}
		}
		n, err := v.tx.insertRows(v.ctx, "gebra_view_member", []string{"store_id", "gen", "set_key", "user_type", "user_id", "set_type", "set_rel", "set_id"}, values, onConflict{ignore: !v.strict})
		if err != nil {
			return errors.Wrap(err, "insert view members")
		}
		v.added.member += n
	}
	if v.noReach {
		return nil
	}

	// Without deletes no set loses its anchor, so the new anchors follow from
	// the old ones without another read.
	var isAnchored map[string]bool
	if len(v.memberDels) > 0 || len(disappeared) > 0 {
		if isAnchored, err = v.anchored(sets); err != nil {
			return err
		}
	} else {
		isAnchored = maps.Clone(wasAnchored)
		for _, m := range v.memberAdds {
			isAnchored[m.SetKey] = true
		}
		for _, e := range appeared {
			isAnchored[e.child] = true
		}
	}
	var gained, lost []string
	for _, s := range sets {
		switch {
		case !wasAnchored[s] && isAnchored[s]:
			gained = append(gained, s)
		case wasAnchored[s] && !isAnchored[s]:
			lost = append(lost, s)
		}
	}

	v.wasAnchored, v.isAnchored = wasAnchored, isAnchored
	if len(disappeared) == 0 && len(lost) == 0 {
		err = v.extend(appeared, gained)
	} else {
		err = v.rederive(slices.Concat(appeared, disappeared), slices.Concat(gained, lost))
	}
	if err != nil {
		return err
	}
	return v.flushReach()
}

// extend adds the reach rows that new edges and new anchors give when
// nothing was removed. A new path from a set d to a set a goes through a new
// edge c → p, and taking the first such edge on the path, d reaches c through
// old edges and p reaches a through any edges. So the new rows are the old
// descendants of c, and c itself, crossed with p and everything above p.
func (v *viewMaintainer) extend(appeared []viewEdge, gained []string) error {
	children := make([]string, len(appeared))
	tops := make([]string, len(appeared))
	for i, e := range appeared {
		children[i] = e.child
		tops[i] = e.parent
	}
	// Reach rows are unchanged so far, so they hold the old descendants.
	below, err := v.descendants(children)
	if err != nil {
		return err
	}
	parents, err := v.walkUp(tops)
	if err != nil {
		return err
	}
	explicitParents := func(children []string) ([]string, error) {
		var out []string
		for _, c := range children {
			out = append(out, parents[c]...)
		}
		return out, nil
	}

	// Sets that gained their anchor have no reach rows yet, and recompute
	// derives all of them.
	skip := map[string]struct{}{}
	for _, s := range gained {
		skip[s] = struct{}{}
	}
	var covered map[string]bool
	if v.covered != nil {
		var ds []string
		for _, e := range appeared {
			ds = append(append(ds, below[e.child]...), e.child)
		}
		if covered, err = v.covered(ds); err != nil {
			return err
		}
	}
	rows := map[[2]string]struct{}{}
	for _, e := range appeared {
		above, _ := bfsUp(v.m, e.parent, explicitParents)
		above[e.parent] = struct{}{}
		for _, d := range append(below[e.child], e.child) {
			if _, ok := skip[d]; ok || covered != nil && !covered[d] {
				continue
			}
			for a := range above {
				if a != d {
					rows[[2]string{a, d}] = struct{}{}
				}
			}
		}
	}
	inserts := make([]viewReachRow, 0, len(rows))
	for r := range rows {
		inserts = append(inserts, reachRow(v.storeID, v.gen, r[0], r[1]))
	}
	v.insertReach(inserts)
	if len(gained) > 0 {
		return v.recompute(gained)
	}
	return nil
}

// rederive recomputes the reach rows of every set whose anchor changed or
// that sits at or below a changed edge. Reach rows are unchanged so far, so
// they still list every set below each changed edge.
func (v *viewMaintainer) rederive(changed []viewEdge, anchorChanged []string) error {
	affected := map[string]struct{}{}
	for _, s := range anchorChanged {
		affected[s] = struct{}{}
	}
	children := make([]string, len(changed))
	for i, e := range changed {
		affected[e.child] = struct{}{}
		children[i] = e.child
	}
	below, err := v.descendants(children)
	if err != nil {
		return err
	}
	for _, ds := range below {
		for _, d := range ds {
			affected[d] = struct{}{}
		}
	}
	if len(affected) == 0 {
		return nil
	}
	return v.recompute(slices.Collect(maps.Keys(affected)))
}

// applyEdges adds the reference count deltas to the edges and returns the
// edges that appeared and the ones that disappeared.
func (v *viewMaintainer) applyEdges(delta map[viewEdge]int64) (appeared, disappeared []viewEdge, err error) {
	pairs := make([]viewEdge, 0, len(delta))
	for e, d := range delta {
		if d != 0 {
			pairs = append(pairs, e)
		}
	}
	refs := map[viewEdge]int64{}
	for _, batch := range rowChunks(pairs) {
		keys := make([][]any, len(batch))
		for i, e := range batch {
			keys[i] = []any{e.child, e.parent}
		}
		rows, err := queryStructs[viewEdgeRow](v.ctx, v.tx, "SELECT child_key, parent_key, refs FROM gebra_view_edge WHERE store_id = ? AND gen = ? AND (child_key, parent_key) IN ?", v.storeID, v.gen, keys)
		if err != nil {
			return nil, nil, errors.Wrap(err, "read view edges")
		}
		for _, r := range rows {
			refs[viewEdge{child: r.ChildKey, parent: r.ParentKey}] = r.Refs
		}
	}

	var creates []viewEdgeRow
	var deletes [][]any
	var updates []viewEdge
	for _, e := range pairs {
		before := refs[e]
		after := before + delta[e]
		switch {
		case after < 0:
			return nil, nil, errors.Newf("view edge %s in %s would have %d references", e.child, e.parent, after)
		case before == 0:
			appeared = append(appeared, e)
			creates = append(creates, viewEdgeRow{StoreID: v.storeID, Gen: v.gen, ChildKey: e.child, ParentKey: e.parent, Refs: after})
		case after == 0:
			disappeared = append(disappeared, e)
			deletes = append(deletes, []any{e.child, e.parent})
		default:
			updates = append(updates, e)
		}
	}
	for _, batch := range rowChunks(updates) {
		// The counts are literals, which every database types as integers,
		// where it would type parameters of a CASE as text.
		var q strings.Builder
		q.WriteString("UPDATE gebra_view_edge SET refs = CASE")
		args := make([]any, 0, 4*len(batch)+1)
		keys := make([][]any, len(batch))
		for i, e := range batch {
			q.WriteString(" WHEN child_key = ? AND parent_key = ? THEN " + strconv.FormatInt(refs[e]+delta[e], 10))
			args = append(args, e.child, e.parent)
			keys[i] = []any{e.child, e.parent}
		}
		q.WriteString(" ELSE refs END WHERE store_id = ? AND gen = ? AND (child_key, parent_key) IN ?")
		args = append(args, v.storeID, v.gen, keys)
		if _, err := v.tx.exec(v.ctx, q.String(), args...); err != nil {
			return nil, nil, errors.Wrap(err, "update view edges")
		}
	}
	if len(creates) > 0 {
		values := make([][]any, len(creates))
		for i, e := range creates {
			values[i] = []any{e.StoreID, e.Gen, e.ChildKey, e.ParentKey, e.Refs}
		}
		n, err := v.tx.insertRows(v.ctx, "gebra_view_edge", []string{"store_id", "gen", "child_key", "parent_key", "refs"}, values, onConflict{})
		if err != nil {
			return nil, nil, errors.Wrap(err, "insert view edges")
		}
		v.added.edge += n
	}
	for _, batch := range rowChunks(deletes) {
		n, err := v.tx.exec(v.ctx, "DELETE FROM gebra_view_edge WHERE store_id = ? AND gen = ? AND (child_key, parent_key) IN ?", v.storeID, v.gen, batch)
		if err != nil {
			return nil, nil, errors.Wrap(err, "delete view edges")
		}
		v.removed.edge += n
	}
	return appeared, disappeared, nil
}

// anchored reports which of the sets have direct members or are the child of
// an edge. The view keeps reach rows only for anchored sets.
func (v *viewMaintainer) anchored(sets []string) (map[string]bool, error) {
	out := make(map[string]bool, len(sets))
	for _, batch := range chunks(sets) {
		// EXISTS stops at the first row, which matters for sets with many
		// members. CASE makes the result an integer on every database.
		cols := make([]string, len(batch))
		args := make([]any, 0, 6*len(batch))
		for i, s := range batch {
			cols[i] = `CASE WHEN EXISTS (SELECT 1 FROM gebra_view_member WHERE store_id = ? AND gen = ? AND set_key = ?)
				OR EXISTS (SELECT 1 FROM gebra_view_edge WHERE store_id = ? AND gen = ? AND child_key = ?) THEN 1 ELSE 0 END`
			args = append(args, v.storeID, v.gen, s, v.storeID, v.gen, s)
		}
		flags := make([]int64, len(batch))
		dest := make([]any, len(batch))
		for i := range flags {
			dest[i] = &flags[i]
		}
		if _, err := v.tx.queryRow(v.ctx, "SELECT "+strings.Join(cols, ", "), args, dest...); err != nil {
			return nil, errors.Wrap(err, "read view anchors")
		}
		for i, s := range batch {
			out[s] = flags[i] == 1
		}
	}
	return out, nil
}

// descendants returns the anchored sets that each of the sets contains.
func (v *viewMaintainer) descendants(sets []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, batch := range rowChunks(sets) {
		keys := make([][]any, len(batch))
		for i, s := range batch {
			typ, id, rel := splitSetKey(s)
			keys[i] = []any{typ, id, rel}
		}
		rows, err := queryStructs[viewReachRow](v.ctx, v.tx, "SELECT desc_key, anc_type, anc_id, anc_rel FROM gebra_view_reach WHERE store_id = ? AND gen = ? AND (anc_type, anc_id, anc_rel) IN ?", v.storeID, v.gen, keys)
		if err != nil {
			return nil, errors.Wrap(err, "read view descendants")
		}
		for _, r := range rows {
			anc := setKey(r.AncType, r.AncID, r.AncRel)
			out[anc] = append(out[anc], r.DescKey)
		}
	}
	return out, nil
}

// insertReach adds reach rows to insert when the batch ends. The batch's
// reads of reach rows never read the sets whose rows wait there.
func (v *viewMaintainer) insertReach(rows []viewReachRow) {
	v.reachInserts = append(v.reachInserts, rows...)
}

// insertReach inserts reach rows and returns how many it inserted.
func insertReach(ctx context.Context, tx dbtx, rows []viewReachRow, conflict onConflict) (int64, error) {
	values := make([][]any, len(rows))
	for i, r := range rows {
		values[i] = []any{r.StoreID, r.Gen, r.DescKey, r.AncType, r.AncRel, r.AncID, r.DescType, r.DescRel, r.DescID}
	}
	return tx.insertRows(ctx, "gebra_view_reach", []string{"store_id", "gen", "desc_key", "anc_type", "anc_rel", "anc_id", "desc_type", "desc_rel", "desc_id"}, values, conflict)
}

func (v *viewMaintainer) flushReach() error {
	if len(v.reachInserts) == 0 {
		return nil
	}
	// A new edge can connect sets that another path already connects.
	n, err := insertReach(v.ctx, v.tx, v.reachInserts, onConflict{ignore: true})
	if err != nil {
		return errors.Wrap(err, "insert view reach")
	}
	v.added.reach += n
	v.reachInserts = nil
	return nil
}

// walkUp reads the edges up from the sets, level by level, and returns the
// parents that edges give each set it visits. It runs after the batch changed
// the edges, so it reads the edges of each set once per batch.
func (v *viewMaintainer) walkUp(sets []string) (map[string][]string, error) {
	parents := v.parents
	seen := map[string]struct{}{}
	for _, s := range sets {
		seen[s] = struct{}{}
	}
	for frontier := sets; len(frontier) > 0; {
		var next []string
		visit := func(p string) {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				next = append(next, p)
			}
		}
		var unread []string
		for _, c := range frontier {
			if ps, ok := parents[c]; ok {
				for _, p := range ps {
					visit(p)
				}
			} else {
				unread = append(unread, c)
				parents[c] = nil
			}
		}
		for _, batch := range chunks(unread) {
			edges, err := queryStructs[viewEdgeRow](v.ctx, v.tx, "SELECT child_key, parent_key FROM gebra_view_edge WHERE store_id = ? AND gen = ? AND child_key IN ?", v.storeID, v.gen, batch)
			if err != nil {
				return nil, errors.Wrap(err, "read view edges")
			}
			for _, e := range edges {
				parents[e.ChildKey] = append(parents[e.ChildKey], e.ParentKey)
				visit(e.ParentKey)
			}
		}
		for _, c := range frontier {
			for _, p := range v.m.implicitParents(c) {
				visit(p)
			}
		}
		frontier = next
	}
	return parents, nil
}

// recompute replaces the reach rows of the sets with the ones the current
// edges give. Deriving the closure again, rather than counting paths, handles
// cycles.
func (v *viewMaintainer) recompute(sets []string) error {
	if v.covered != nil {
		covered, err := v.covered(sets)
		if err != nil {
			return err
		}
		sets = slices.DeleteFunc(slices.Clone(sets), func(s string) bool { return !covered[s] })
	}
	if len(sets) == 0 {
		return nil
	}
	parents, err := v.walkUp(sets)
	if err != nil {
		return err
	}

	// Only sets that were anchored have reach rows.
	var anchoredBefore []string
	for _, s := range sets {
		if was, ok := v.wasAnchored[s]; !ok || was {
			anchoredBefore = append(anchoredBefore, s)
		}
	}
	have := map[string]map[string]struct{}{}
	for _, batch := range chunks(anchoredBefore) {
		rows, err := queryStructs[viewReachRow](v.ctx, v.tx, "SELECT desc_key, anc_type, anc_id, anc_rel FROM gebra_view_reach WHERE store_id = ? AND gen = ? AND desc_key IN ?", v.storeID, v.gen, batch)
		if err != nil {
			return errors.Wrap(err, "read view reach")
		}
		for _, r := range rows {
			if have[r.DescKey] == nil {
				have[r.DescKey] = map[string]struct{}{}
			}
			have[r.DescKey][setKey(r.AncType, r.AncID, r.AncRel)] = struct{}{}
		}
	}

	explicitParents := func(children []string) ([]string, error) {
		var out []string
		for _, c := range children {
			out = append(out, parents[c]...)
		}
		return out, nil
	}
	var inserts []viewReachRow
	var deletes [][]any
	for _, d := range sets {
		want := map[string]struct{}{}
		if is, ok := v.isAnchored[d]; !ok || is {
			want, _ = bfsUp(v.m, d, explicitParents)
		}
		for a := range want {
			if _, ok := have[d][a]; !ok {
				inserts = append(inserts, reachRow(v.storeID, v.gen, a, d))
			}
		}
		for a := range have[d] {
			if _, ok := want[a]; !ok {
				typ, id, rel := splitSetKey(a)
				deletes = append(deletes, []any{d, typ, id, rel})
			}
		}
	}
	for _, batch := range rowChunks(deletes) {
		n, err := v.tx.exec(v.ctx, "DELETE FROM gebra_view_reach WHERE store_id = ? AND gen = ? AND (desc_key, anc_type, anc_id, anc_rel) IN ?", v.storeID, v.gen, batch)
		if err != nil {
			return errors.Wrap(err, "delete view reach")
		}
		v.removed.reach += n
	}
	v.insertReach(inserts)
	return nil
}
