package gebra

import (
	"fmt"
	"slices"
	"strings"
)

// AuthorizationModel is an authorization model in the JSON form that OpenFGA
// accepts, extended with object attributes in type metadata.
type AuthorizationModel struct {
	ID              string                `json:"id,omitempty"`
	SchemaVersion   string                `json:"schema_version"`
	TypeDefinitions []*TypeDefinition     `json:"type_definitions"`
	Conditions      map[string]*Condition `json:"conditions,omitempty"`
}

type TypeDefinition struct {
	Type      string              `json:"type"`
	Relations map[string]*Userset `json:"relations,omitempty"`
	Metadata  *Metadata           `json:"metadata,omitempty"`
}

type Metadata struct {
	Relations map[string]*RelationMetadata `json:"relations,omitempty"`
	// Attributes maps attribute names to their types. Attributes are the keys
	// that list queries sort by. This is a Gebra extension.
	Attributes map[string]AttributeType `json:"attributes,omitempty"`
}

type RelationMetadata struct {
	DirectlyRelatedUserTypes []*RelationReference `json:"directly_related_user_types,omitempty"`
}

// RelationReference is one entry of a relation's type restrictions: a type
// (user), a userset (group#member), or a wildcard (user:*), optionally with a
// condition.
type RelationReference struct {
	Type      string    `json:"type"`
	Relation  string    `json:"relation,omitempty"`
	Wildcard  *struct{} `json:"wildcard,omitempty"`
	Condition string    `json:"condition,omitempty"`
}

// Userset is a node of a userset rewrite. Exactly one field is set.
type Userset struct {
	This            *struct{}       `json:"this,omitempty"`
	ComputedUserset *ObjectRelation `json:"computedUserset,omitempty"`
	TupleToUserset  *TupleToUserset `json:"tupleToUserset,omitempty"`
	Union           *Usersets       `json:"union,omitempty"`
	Intersection    *Usersets       `json:"intersection,omitempty"`
	Difference      *Difference     `json:"difference,omitempty"`
}

type ObjectRelation struct {
	Object   string `json:"object,omitempty"`
	Relation string `json:"relation"`
}

type TupleToUserset struct {
	Tupleset        ObjectRelation `json:"tupleset"`
	ComputedUserset ObjectRelation `json:"computedUserset"`
}

type Usersets struct {
	Child []*Userset `json:"child"`
}

type Difference struct {
	Base     *Userset `json:"base"`
	Subtract *Userset `json:"subtract"`
}

type Condition struct {
	Name       string                            `json:"name"`
	Expression string                            `json:"expression"`
	Parameters map[string]*ConditionParamTypeRef `json:"parameters,omitempty"`
}

type ConditionParamTypeRef struct {
	TypeName     string                   `json:"type_name"`
	GenericTypes []*ConditionParamTypeRef `json:"generic_types,omitempty"`
}

// AttributeType is the type of an object attribute. It decides how values are
// encoded so that they sort correctly.
type AttributeType string

const (
	AttributeString AttributeType = "string"
	AttributeInt    AttributeType = "int"
	AttributeTime   AttributeType = "time"
)

// compiledModel is a validated authorization model with the lookup tables that
// queries and view maintenance need.
type compiledModel struct {
	id         string
	source     *AuthorizationModel
	types      map[string]*typeInfo
	conditions map[string]*compiledCondition

	// computedParents[type][relation] lists the materialized relations of the
	// same type whose union directly includes relation.
	computedParents map[string]map[string][]string
	// ttuEdges[type][tupleset] lists the edges that a tuple on tupleset adds to
	// the view.
	ttuEdges map[string]map[string][]ttuEdge

	// revComputed[type][relation] lists every relation of the same type whose
	// rewrite references relation as a computed userset, materialized or not.
	revComputed map[string]map[string][]string
	// revTTU[type][tupleset] lists every tuple to userset that reads tupleset,
	// materialized or not.
	revTTU map[string]map[string][]ttuEdge
}

type typeInfo struct {
	name       string
	attributes map[string]AttributeType
	relations  map[string]*relationInfo
}

type relationInfo struct {
	typ     string
	name    string
	rewrite *Userset
	direct  []*RelationReference
	// materialized is true when the relation is a union of direct tuples,
	// computed usersets, and tuple to usersets that only reach other
	// materialized relations, with no conditions. The view answers queries on
	// such relations without evaluating the rewrite.
	materialized bool
}

type ttuEdge struct {
	// parent is the materialized relation on the tuple's object.
	parent string
	// target is the relation on the tuple's user.
	target string
}

func (m *compiledModel) relation(typ, rel string) *relationInfo {
	t := m.types[typ]
	if t == nil {
		return nil
	}
	return t.relations[rel]
}

func invalidf(format string, args ...any) error {
	return &invalidArgumentError{msg: fmt.Sprintf(format, args...)}
}

func validName(s string) bool {
	return s != "" && !strings.ContainsAny(s, ":#*@ \t\n\r")
}

// compileModel validates the model and builds its lookup tables.
func compileModel(src *AuthorizationModel, conditionCostLimit uint64) (*compiledModel, error) {
	if src.SchemaVersion != "1.1" {
		return nil, invalidf("unsupported schema version %q, want 1.1", src.SchemaVersion)
	}

	if len(src.TypeDefinitions) == 0 {
		return nil, invalidf("model needs at least one type")
	}

	m := &compiledModel{
		id:              src.ID,
		source:          src,
		types:           make(map[string]*typeInfo, len(src.TypeDefinitions)),
		conditions:      make(map[string]*compiledCondition, len(src.Conditions)),
		computedParents: make(map[string]map[string][]string),
		ttuEdges:        make(map[string]map[string][]ttuEdge),
		revComputed:     make(map[string]map[string][]string),
		revTTU:          make(map[string]map[string][]ttuEdge),
	}

	for name, c := range src.Conditions {
		if c.Name != "" && c.Name != name {
			return nil, invalidf("condition %q has mismatched name %q", name, c.Name)
		}
		cc, err := compileCondition(name, c, conditionCostLimit)
		if err != nil {
			return nil, err
		}
		m.conditions[name] = cc
	}

	for _, td := range src.TypeDefinitions {
		if !validName(td.Type) {
			return nil, invalidf("invalid type name %q", td.Type)
		}
		if m.types[td.Type] != nil {
			return nil, invalidf("duplicate type %q", td.Type)
		}
		t := &typeInfo{
			name:       td.Type,
			attributes: map[string]AttributeType{},
			relations:  make(map[string]*relationInfo, len(td.Relations)),
		}
		if td.Metadata != nil {
			for name, at := range td.Metadata.Attributes {
				if !validName(name) {
					return nil, invalidf("invalid attribute name %q on type %q", name, td.Type)
				}
				switch at {
				case AttributeString, AttributeInt, AttributeTime:
				default:
					return nil, invalidf("attribute %s.%s has unknown type %q", td.Type, name, at)
				}
				t.attributes[name] = at
			}
		}
		for name, rewrite := range td.Relations {
			if !validName(name) {
				return nil, invalidf("invalid relation name %q on type %q", name, td.Type)
			}
			if name == "self" || name == "this" {
				return nil, invalidf("relation name %q on type %q is reserved", name, td.Type)
			}
			if rewrite == nil {
				return nil, invalidf("relation %s#%s has no definition", td.Type, name)
			}
			r := &relationInfo{typ: td.Type, name: name, rewrite: rewrite}
			if td.Metadata != nil && td.Metadata.Relations[name] != nil {
				r.direct = td.Metadata.Relations[name].DirectlyRelatedUserTypes
			}
			t.relations[name] = r
		}
		if td.Metadata != nil {
			for name := range td.Metadata.Relations {
				if t.relations[name] == nil {
					return nil, invalidf("metadata for undefined relation %s#%s", td.Type, name)
				}
			}
		}
		m.types[td.Type] = t
	}

	for _, t := range m.types {
		for _, r := range t.relations {
			if err := m.validateRelation(r); err != nil {
				return nil, err
			}
		}
		if err := checkComputedCycles(t); err != nil {
			return nil, err
		}
	}

	if err := m.checkEntrypoints(); err != nil {
		return nil, err
	}

	m.computeMaterialized()
	m.buildIndexes()
	return m, nil
}

// checkEntrypoints rejects relations that no tuple can ever grant, e.g., a
// relation that is only defined through itself across tuple to usersets or
// usersets, as OpenFGA does. Every operand of an intersection and both sides
// of an exclusion need an entrypoint.
func (m *compiledModel) checkEntrypoints() error {
	entry := map[*relationInfo]bool{}
	var reaches func(r *relationInfo, u *Userset) bool
	reaches = func(r *relationInfo, u *Userset) bool {
		switch {
		case u.This != nil:
			for _, ref := range r.direct {
				if ref.Relation == "" || entry[m.relation(ref.Type, ref.Relation)] {
					return true
				}
			}
		case u.ComputedUserset != nil:
			return entry[m.relation(r.typ, u.ComputedUserset.Relation)]
		case u.TupleToUserset != nil:
			for _, ref := range m.relation(r.typ, u.TupleToUserset.Tupleset.Relation).direct {
				if target := m.relation(ref.Type, u.TupleToUserset.ComputedUserset.Relation); target != nil && entry[target] {
					return true
				}
			}
		case u.Union != nil:
			return slices.ContainsFunc(u.Union.Child, func(c *Userset) bool { return reaches(r, c) })
		case u.Intersection != nil:
			return !slices.ContainsFunc(u.Intersection.Child, func(c *Userset) bool { return !reaches(r, c) })
		case u.Difference != nil:
			return reaches(r, u.Difference.Base) && reaches(r, u.Difference.Subtract)
		}
		return false
	}
	for changed := true; changed; {
		changed = false
		for _, t := range m.types {
			for _, r := range t.relations {
				if !entry[r] && reaches(r, r.rewrite) {
					entry[r] = true
					changed = true
				}
			}
		}
	}
	for _, t := range m.types {
		for _, r := range t.relations {
			if !entry[r] {
				return invalidf("relation %s#%s has no entrypoint: no tuple can grant it", r.typ, r.name)
			}
		}
	}
	return nil
}

func (m *compiledModel) validateRelation(r *relationInfo) error {
	thisCount := 0
	var walk func(u *Userset) error
	walk = func(u *Userset) error {
		set := 0
		if u.This != nil {
			set++
			thisCount++
		}
		if u.ComputedUserset != nil {
			set++
			if u.ComputedUserset.Object != "" {
				return invalidf("relation %s#%s: computed userset object is not supported", r.typ, r.name)
			}
			if m.relation(r.typ, u.ComputedUserset.Relation) == nil {
				return invalidf("relation %s#%s references undefined relation %q", r.typ, r.name, u.ComputedUserset.Relation)
			}
		}
		if u.TupleToUserset != nil {
			set++
			if err := m.validateTupleToUserset(r, u.TupleToUserset); err != nil {
				return err
			}
		}
		for _, us := range []*Usersets{u.Union, u.Intersection} {
			if us == nil {
				continue
			}
			set++
			if len(us.Child) == 0 {
				return invalidf("relation %s#%s has an empty union or intersection", r.typ, r.name)
			}
			for _, c := range us.Child {
				if c == nil {
					return invalidf("relation %s#%s has an empty child", r.typ, r.name)
				}
				if err := walk(c); err != nil {
					return err
				}
			}
		}
		if u.Difference != nil {
			set++
			if u.Difference.Base == nil || u.Difference.Subtract == nil {
				return invalidf("relation %s#%s has an incomplete difference", r.typ, r.name)
			}
			if err := walk(u.Difference.Base); err != nil {
				return err
			}
			if err := walk(u.Difference.Subtract); err != nil {
				return err
			}
		}
		if set != 1 {
			return invalidf("relation %s#%s has a userset with %d operators, want exactly 1", r.typ, r.name, set)
		}
		return nil
	}
	if err := walk(r.rewrite); err != nil {
		return err
	}

	if thisCount > 1 {
		return invalidf("relation %s#%s has more than one direct type restriction", r.typ, r.name)
	}
	if thisCount == 1 && len(r.direct) == 0 {
		return invalidf("relation %s#%s allows direct relationships but has no type restrictions", r.typ, r.name)
	}
	if thisCount == 0 && len(r.direct) > 0 {
		return invalidf("relation %s#%s has type restrictions but does not allow direct relationships", r.typ, r.name)
	}
	for _, ref := range r.direct {
		if m.types[ref.Type] == nil {
			return invalidf("relation %s#%s allows undefined type %q", r.typ, r.name, ref.Type)
		}
		if ref.Relation != "" && ref.Wildcard != nil {
			return invalidf("relation %s#%s has a type restriction that is both a userset and a wildcard", r.typ, r.name)
		}
		if ref.Relation != "" && m.relation(ref.Type, ref.Relation) == nil {
			return invalidf("relation %s#%s allows undefined userset %s#%s", r.typ, r.name, ref.Type, ref.Relation)
		}
		if ref.Condition != "" && m.conditions[ref.Condition] == nil {
			return invalidf("relation %s#%s uses undefined condition %q", r.typ, r.name, ref.Condition)
		}
	}
	return nil
}

func (m *compiledModel) validateTupleToUserset(r *relationInfo, ttu *TupleToUserset) error {
	tupleset := m.relation(r.typ, ttu.Tupleset.Relation)
	if tupleset == nil {
		return invalidf("relation %s#%s references undefined tupleset %q", r.typ, r.name, ttu.Tupleset.Relation)
	}
	if tupleset.rewrite.This == nil {
		return invalidf("relation %s#%s: tupleset %q must only allow direct relationships", r.typ, r.name, tupleset.name)
	}
	found := false
	for _, ref := range tupleset.direct {
		if ref.Relation != "" || ref.Wildcard != nil {
			return invalidf("relation %s#%s: tupleset %q must only allow concrete types", r.typ, r.name, tupleset.name)
		}
		if m.relation(ref.Type, ttu.ComputedUserset.Relation) != nil {
			found = true
		}
	}
	if !found {
		return invalidf("relation %s#%s: no type allowed by tupleset %q defines relation %q", r.typ, r.name, tupleset.name, ttu.ComputedUserset.Relation)
	}
	return nil
}

// checkComputedCycles rejects relations that reference each other through
// computed usersets only, which have no base case.
func checkComputedCycles(t *typeInfo) error {
	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[string]int, len(t.relations))
	var visit func(name string) error
	visit = func(name string) error {
		switch state[name] {
		case visiting:
			return invalidf("relation %s#%s is defined in terms of itself", t.name, name)
		case done:
			return nil
		}
		state[name] = visiting
		var err error
		walkUserset(t.relations[name].rewrite, func(u *Userset) {
			if err == nil && u.ComputedUserset != nil {
				err = visit(u.ComputedUserset.Relation)
			}
		})
		state[name] = done
		return err
	}
	for name := range t.relations {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

// walkUserset calls fn on every node of the rewrite tree.
func walkUserset(u *Userset, fn func(*Userset)) {
	fn(u)
	if u.Union != nil {
		for _, c := range u.Union.Child {
			walkUserset(c, fn)
		}
	}
	if u.Intersection != nil {
		for _, c := range u.Intersection.Child {
			walkUserset(c, fn)
		}
	}
	if u.Difference != nil {
		walkUserset(u.Difference.Base, fn)
		walkUserset(u.Difference.Subtract, fn)
	}
}

func unionOnly(u *Userset) bool {
	ok := true
	walkUserset(u, func(n *Userset) {
		if n.Intersection != nil || n.Difference != nil {
			ok = false
		}
	})
	return ok
}

func (m *compiledModel) computeMaterialized() {
	for _, t := range m.types {
		for _, r := range t.relations {
			r.materialized = unionOnly(r.rewrite)
			for _, ref := range r.direct {
				if ref.Condition != "" {
					r.materialized = false
				}
			}
			walkUserset(r.rewrite, func(u *Userset) {
				if u.TupleToUserset == nil {
					return
				}
				for _, ref := range m.relation(r.typ, u.TupleToUserset.Tupleset.Relation).direct {
					if ref.Condition != "" {
						r.materialized = false
					}
				}
			})
		}
	}

	// A relation stays materialized only while everything it reaches is
	// materialized. Iterate to a fixpoint because references can be cyclic.
	for changed := true; changed; {
		changed = false
		for _, t := range m.types {
			for _, r := range t.relations {
				if r.materialized && !m.reachesOnlyMaterialized(r) {
					r.materialized = false
					changed = true
				}
			}
		}
	}
}

func (m *compiledModel) reachesOnlyMaterialized(r *relationInfo) bool {
	for _, ref := range r.direct {
		if ref.Relation != "" && !m.relation(ref.Type, ref.Relation).materialized {
			return false
		}
	}
	ok := true
	walkUserset(r.rewrite, func(u *Userset) {
		switch {
		case u.ComputedUserset != nil:
			if !m.relation(r.typ, u.ComputedUserset.Relation).materialized {
				ok = false
			}
		case u.TupleToUserset != nil:
			for _, ref := range m.relation(r.typ, u.TupleToUserset.Tupleset.Relation).direct {
				target := m.relation(ref.Type, u.TupleToUserset.ComputedUserset.Relation)
				if target != nil && !target.materialized {
					ok = false
				}
			}
		}
	})
	return ok
}

func (m *compiledModel) buildIndexes() {
	for _, t := range m.types {
		for _, r := range t.relations {
			walkUserset(r.rewrite, func(u *Userset) {
				switch {
				case u.ComputedUserset != nil:
					c := u.ComputedUserset.Relation
					addIndex(m.revComputed, t.name, c, r.name)
					if r.materialized {
						addIndex(m.computedParents, t.name, c, r.name)
					}
				case u.TupleToUserset != nil:
					ts := u.TupleToUserset.Tupleset.Relation
					e := ttuEdge{parent: r.name, target: u.TupleToUserset.ComputedUserset.Relation}
					addIndex(m.revTTU, t.name, ts, e)
					if r.materialized {
						addIndex(m.ttuEdges, t.name, ts, e)
					}
				}
			})
		}
	}
}

func addIndex[V comparable](index map[string]map[string][]V, typ, key string, v V) {
	if index[typ] == nil {
		index[typ] = map[string][]V{}
	}
	if !slices.Contains(index[typ][key], v) {
		index[typ][key] = append(index[typ][key], v)
	}
}

// allows reports whether the tuple matches one of the type restrictions of its
// relation. Stored tuples that no longer match, e.g., after a model change,
// are ignored by queries.
func (m *compiledModel) allows(t *tuple) bool {
	r := m.relation(t.objectType, t.relation)
	if r == nil || m.types[t.userType] == nil {
		return false
	}
	if t.userRelation != "" && m.relation(t.userType, t.userRelation) == nil {
		return false
	}
	wildcard := t.userID == "*"
	for _, ref := range r.direct {
		if ref.Type == t.userType && ref.Relation == t.userRelation && (ref.Wildcard != nil) == wildcard && ref.Condition == t.conditionName {
			return true
		}
	}
	return false
}

// validateTuple reports whether the model allows the tuple to be stored.
func (m *compiledModel) validateTuple(t *tuple) error {
	if m.relation(t.objectType, t.relation) == nil {
		return invalidf("relation %s#%s is not defined", t.objectType, t.relation)
	}
	if err := m.validateSubject(subject{typ: t.userType, id: t.userID, rel: t.userRelation}); err != nil {
		return err
	}
	if t.userRelation == t.relation && t.userType == t.objectType && t.userID == t.objectID {
		return invalidf("tuple %s is implicit: every set contains itself", t.keyString())
	}
	if !m.allows(t) {
		if t.conditionName != "" {
			return invalidf("relation %s#%s does not allow %s with condition %q", t.objectType, t.relation, t.userString(), t.conditionName)
		}
		return invalidf("relation %s#%s does not allow %s", t.objectType, t.relation, t.userString())
	}
	if t.conditionName != "" {
		return m.conditions[t.conditionName].validateContext(t.conditionContext)
	}
	return nil
}

func (m *compiledModel) validateSubject(sub subject) error {
	if m.types[sub.typ] == nil {
		return invalidf("type %q is not defined", sub.typ)
	}
	if sub.rel != "" && m.relation(sub.typ, sub.rel) == nil {
		return invalidf("relation %s#%s is not defined", sub.typ, sub.rel)
	}
	return nil
}
