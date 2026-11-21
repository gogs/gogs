package gebra

import (
	"fmt"
	"slices"
	"strings"
)

// checkCompiler turns a check whose rewrite only reads the view into one SQL
// condition, so that the check runs as a single statement, which needs no
// snapshot. The leaves of such a rewrite are materialized relations. Computed
// usersets on the object are inlined, and unions, intersections and
// exclusions become OR, AND and AND NOT. A tuple to userset of a relation
// that is not materialized reads its tupleset from the view's member rows.
// Rewrites that read tuples, i.e., direct relationships of relations that are
// not materialized and tuple to usersets that reach such relations, do not
// compile.
type checkCompiler struct {
	m       *compiledModel
	dialect string
	storeID string
	// gen is the generation of the view that the condition reads.
	gen int64
	sub subject
	// closure holds the sets that a userset subject is in through computed
	// usersets alone, itself included.
	closure []string
	// tuplesets allows tuple to usersets of relations that are not
	// materialized. Their tupleset rows in the view are the tuples as of the
	// writes that the view includes, which only matches what the evaluator
	// reads when it reads those tuples.
	tuplesets bool

	sql  strings.Builder
	args []any

	// leaves holds the materialized sets the condition asks the view about,
	// and tsReads the tupleset sets whose member rows it reads, with the
	// type and computed relation of the objects they list.
	leaves   []object
	leafRels []string
	tsReads  []tuplesetRead
}

// tuplesetRead is a tupleset set whose member rows a compiled condition
// reads, and the relation it asks about on the objects of a type they list.
type tuplesetRead struct {
	obj    object
	rel    string
	typ    string
	target string
}

// placeholders returns n comma-separated placeholders in parentheses.
func placeholders(n int) string {
	return "(" + strings.TrimSuffix(strings.Repeat("?, ", n), ", ") + ")"
}

// appendStrings appends the strings to args.
func appendStrings(args []any, ss []string) []any {
	for _, s := range ss {
		args = append(args, s)
	}
	return args
}

func newCheckCompiler(m *compiledModel, dialect, storeID string, gen int64, sub subject, tuplesets bool) *checkCompiler {
	c := &checkCompiler{m: m, dialect: dialect, storeID: storeID, gen: gen, sub: sub, tuplesets: tuplesets}
	if sub.rel != "" {
		c.closure = m.computedClosure(setKey(sub.typ, sub.id, sub.rel))
	}
	return c
}

// computedClosure returns the set and every set that contains it because
// materialized relations of the same object include its relation as a
// computed userset, directly or indirectly.
func (m *compiledModel) computedClosure(key string) []string {
	out := []string{key}
	for i := 0; i < len(out); i++ {
		for _, p := range m.implicitParents(out[i]) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// relation writes the condition that the subject has rel with obj, or
// returns false when the check does not compile.
func (c *checkCompiler) relation(obj object, rel string) bool {
	if c.sub.rel == rel && c.sub.typ == obj.typ && c.sub.id == obj.id {
		c.sql.WriteString("1 = 1")
		return true
	}
	r := c.m.relation(obj.typ, rel)
	if r.materialized {
		c.leaf(obj, rel)
		return true
	}
	return c.rewrite(obj, r.rewrite)
}

func (c *checkCompiler) rewrite(obj object, u *Userset) bool {
	switch {
	case u.ComputedUserset != nil:
		return c.relation(obj, u.ComputedUserset.Relation)
	case u.TupleToUserset != nil:
		return c.tupleToUserset(obj, u.TupleToUserset)
	case u.Union != nil:
		return c.join(obj, u.Union.Child, " OR ")
	case u.Intersection != nil:
		return c.join(obj, u.Intersection.Child, " AND ")
	case u.Difference != nil:
		c.sql.WriteString("(")
		if !c.rewrite(obj, u.Difference.Base) {
			return false
		}
		c.sql.WriteString(" AND NOT (")
		if !c.rewrite(obj, u.Difference.Subtract) {
			return false
		}
		c.sql.WriteString("))")
		return true
	}
	return false
}

func (c *checkCompiler) join(obj object, children []*Userset, op string) bool {
	c.sql.WriteString("(")
	for i, child := range children {
		if i > 0 {
			c.sql.WriteString(op)
		}
		if !c.rewrite(obj, child) {
			return false
		}
	}
	c.sql.WriteString(")")
	return true
}

// leaf writes the condition that the view has the subject in the
// materialized set obj#rel.
func (c *checkCompiler) leaf(obj object, rel string) {
	target := setKey(obj.typ, obj.id, rel)
	c.leaves, c.leafRels = append(c.leaves, obj), append(c.leafRels, rel)
	if c.sub.rel != "" {
		if slices.Contains(c.closure[1:], target) {
			c.sql.WriteString("1 = 1")
			return
		}
		// Reach rows list every set above an anchored set. A set that is not
		// anchored is in no edge, so it reaches others only through computed
		// usersets, up to sets of its closure that are anchored.
		c.sql.WriteString(`EXISTS (SELECT 1 FROM gebra_view_reach
WHERE store_id = ? AND gen = ? AND desc_key IN ` + placeholders(len(c.closure)) + ` AND anc_type = ? AND anc_id = ? AND anc_rel = ?)`)
		c.args = append(c.args, c.storeID, c.gen)
		c.args = appendStrings(c.args, c.closure)
		c.args = append(c.args, obj.typ, obj.id, rel)
		return
	}
	query, args := memberQuery(c.storeID, c.gen, obj, rel, c.sub)
	c.sql.WriteString("EXISTS (" + query + ")")
	c.args = append(c.args, args...)
}

// memberQuery returns a query that yields a row when generation gen of the
// view has the plain subject in the materialized set obj#rel: as a direct
// member, through its type's wildcard, or as a member of a set that reaches
// it.
func memberQuery(storeID string, gen int64, obj object, rel string, sub subject) (string, []any) {
	ids := []string{sub.id}
	if sub.id != "*" {
		ids = append(ids, "*")
	}
	// The wildcard has its own part with a literal '*' so that the generic
	// plan of the prepared statement knows how many sets grant it and starts
	// from the reach rows of the object. With user_id IN (?, ?) that plan reads
	// every wildcard member row of the store on each check.
	const viaReach = `
UNION ALL
SELECT 1 FROM gebra_view_member m
JOIN gebra_view_reach r ON r.store_id = m.store_id AND r.gen = m.gen AND r.desc_key = m.set_key
WHERE m.store_id = ? AND m.gen = ? AND m.user_type = ? AND m.user_id = %s AND r.anc_type = ? AND r.anc_id = ? AND r.anc_rel = ?`
	query := `SELECT 1 FROM gebra_view_member
WHERE store_id = ? AND gen = ? AND set_key = ? AND user_type = ? AND user_id IN ` + placeholders(len(ids)) + fmt.Sprintf(viaReach, "'*'")
	args := appendStrings([]any{storeID, gen, setKey(obj.typ, obj.id, rel), sub.typ}, ids)
	args = append(args, storeID, gen, sub.typ, obj.typ, obj.id, rel)
	if sub.id != "*" {
		query += fmt.Sprintf(viaReach, "?")
		args = append(args, storeID, gen, sub.typ, sub.id, obj.typ, obj.id, rel)
	}
	return query, args
}

// tupleToUserset writes the condition that an object that the view lists in
// the tupleset of obj has the subject in the computed relation. Each tupleset
// row p names such an object by its type and ID, which the view's member and
// reach rows of the computed relation carry in columns of their own.
func (c *checkCompiler) tupleToUserset(obj object, ttu *TupleToUserset) bool {
	tupleset := c.m.relation(obj.typ, ttu.Tupleset.Relation)
	if !c.tuplesets || !tupleset.materialized {
		return false
	}
	target := ttu.ComputedUserset.Relation
	var types []string
	for _, ref := range tupleset.direct {
		r := c.m.relation(ref.Type, target)
		if r == nil || slices.Contains(types, ref.Type) {
			continue
		}
		if !r.materialized {
			return false
		}
		types = append(types, ref.Type)
	}
	if len(types) == 0 {
		c.sql.WriteString("1 = 0")
		return true
	}
	parents := setKey(obj.typ, obj.id, ttu.Tupleset.Relation)
	for _, typ := range types {
		c.tsReads = append(c.tsReads, tuplesetRead{obj: obj, rel: ttu.Tupleset.Relation, typ: typ, target: target})
	}
	c.sql.WriteString("(")
	for i, typ := range types {
		if i > 0 {
			c.sql.WriteString(" OR ")
		}
		if c.sub.rel != "" {
			c.tuplesetUserset(parents, typ, target)
		} else {
			c.tuplesetPlain(parents, typ, target)
		}
	}
	c.sql.WriteString(")")
	return true
}

func (c *checkCompiler) tuplesetPlain(parents, typ, target string) {
	// The member rows of the computed relation are found by set key, which
	// their primary key starts with.
	key := "p.user_type || ':' || p.user_id || '#' || ?"
	if c.dialect == "mysql" {
		key = "CONCAT(p.user_type, ':', p.user_id, '#', ?)"
	}
	direct := `SELECT 1 FROM gebra_view_member p
JOIN gebra_view_member m ON m.store_id = p.store_id AND m.gen = p.gen AND m.set_key = ` + key + `
WHERE p.store_id = ? AND p.gen = ? AND p.set_key = ? AND p.user_type = ? AND m.user_type = ? AND m.user_id = %s`
	const viaReach = `SELECT 1 FROM gebra_view_member p
JOIN gebra_view_reach r ON r.store_id = p.store_id AND r.gen = p.gen AND r.anc_type = p.user_type AND r.anc_id = p.user_id AND r.anc_rel = ?
JOIN gebra_view_member m ON m.store_id = r.store_id AND m.gen = r.gen AND m.set_key = r.desc_key
WHERE p.store_id = ? AND p.gen = ? AND p.set_key = ? AND p.user_type = ? AND m.user_type = ? AND m.user_id = %s`
	// As in memberQuery, the wildcard has parts with a literal '*'.
	parts := []string{fmt.Sprintf(direct, "'*'"), fmt.Sprintf(viaReach, "'*'")}
	args := []any{target, c.storeID, c.gen, parents, typ, c.sub.typ, target, c.storeID, c.gen, parents, typ, c.sub.typ}
	if c.sub.id != "*" {
		parts = append(parts, fmt.Sprintf(direct, "?"), fmt.Sprintf(viaReach, "?"))
		args = append(args, target, c.storeID, c.gen, parents, typ, c.sub.typ, c.sub.id, target, c.storeID, c.gen, parents, typ, c.sub.typ, c.sub.id)
	}
	c.sql.WriteString("EXISTS (" + strings.Join(parts, "\nUNION ALL\n") + ")")
	c.args = append(c.args, args...)
}

func (c *checkCompiler) tuplesetUserset(parents, typ, target string) {
	// The subject is the set typ:id#target itself, or a set of its closure.
	if typ == c.sub.typ && slices.ContainsFunc(c.closure, func(k string) bool {
		_, _, rel := splitSetKey(k)
		return rel == target
	}) {
		c.sql.WriteString(`EXISTS (SELECT 1 FROM gebra_view_member WHERE store_id = ? AND gen = ? AND set_key = ? AND user_type = ? AND user_id = ?) OR `)
		c.args = append(c.args, c.storeID, c.gen, parents, typ, c.sub.id)
	}
	c.sql.WriteString(`EXISTS (SELECT 1 FROM gebra_view_member p
JOIN gebra_view_reach r ON r.store_id = p.store_id AND r.gen = p.gen AND r.anc_type = p.user_type AND r.anc_id = p.user_id AND r.anc_rel = ?
WHERE p.store_id = ? AND p.gen = ? AND p.set_key = ? AND p.user_type = ? AND r.desc_key IN ` + placeholders(len(c.closure)) + `)`)
	c.args = append(c.args, target, c.storeID, c.gen, parents, typ)
	c.args = appendStrings(c.args, c.closure)
}
