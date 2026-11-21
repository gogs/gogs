package gebra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// modelGen generates random authorization models, tuples valid under them,
// and queries.
type modelGen struct {
	rng *rand.Rand
	m   *AuthorizationModel
	// types are the object types, "user" excluded.
	types []string
	rels  map[string][]string
	// objects are the object IDs of every type.
	objects []string
}

const (
	condFlag = "cflag"
	condNum  = "cnum"
)

func fuzzConditions() map[string]*Condition {
	return map[string]*Condition{
		condFlag: {Name: condFlag, Expression: "on", Parameters: map[string]*ConditionParamTypeRef{
			"on": {TypeName: "TYPE_NAME_BOOL"},
		}},
		condNum: {Name: condNum, Expression: "x < limit", Parameters: map[string]*ConditionParamTypeRef{
			"x": {TypeName: "TYPE_NAME_INT"}, "limit": {TypeName: "TYPE_NAME_INT"},
		}},
	}
}

// newModelGen generates a model. Shapes include deep rewrites, cycles through
// tuple to usersets and usersets, wildcards, and conditions.
func newModelGen(rng *rand.Rand) *modelGen {
	g := &modelGen{rng: rng, rels: map[string][]string{}, objects: []string{"a", "b", "c"}}
	n := 2 + rng.IntN(3)
	for i := range n {
		typ := fmt.Sprintf("t%d", i)
		g.types = append(g.types, typ)
		k := 2 + rng.IntN(3)
		for j := range k {
			g.rels[typ] = append(g.rels[typ], fmt.Sprintf("r%d", j))
		}
	}
	// Tupleset relations link objects to objects of the same or other types.
	tuplesets := map[string][]*RelationReference{}
	for _, typ := range g.types {
		if rng.IntN(10) < 7 {
			var refs []*RelationReference
			for _, other := range g.types {
				if rng.IntN(2) == 0 || other == typ && len(refs) == 0 {
					ref := &RelationReference{Type: other}
					if rng.IntN(8) == 0 {
						ref.Condition = condFlag
					}
					refs = append(refs, ref)
				}
			}
			tuplesets[typ] = refs
		}
	}

	g.m = &AuthorizationModel{SchemaVersion: "1.1", Conditions: fuzzConditions()}
	g.m.TypeDefinitions = append(g.m.TypeDefinitions, &TypeDefinition{Type: "user"})
	for _, typ := range g.types {
		td := &TypeDefinition{Type: typ, Relations: map[string]*Userset{}, Metadata: &Metadata{Relations: map[string]*RelationMetadata{}}}
		if refs, ok := tuplesets[typ]; ok {
			td.Relations["p"] = &Userset{This: &struct{}{}}
			td.Metadata.Relations["p"] = &RelationMetadata{DirectlyRelatedUserTypes: refs}
		}
		for _, rel := range g.rels[typ] {
			usedThis := false
			var direct []*RelationReference
			var gen func(depth int) *Userset
			gen = func(depth int) *Userset {
				leaf := depth >= 3 || rng.IntN(10) < 4+2*depth
				if !leaf {
					a, b := gen(depth+1), gen(depth+1)
					// A child that has no leaf left to use collapses into
					// its sibling.
					if a == nil || b == nil {
						if a == nil {
							return b
						}
						return a
					}
					switch rng.IntN(5) {
					case 0, 1:
						return &Userset{Union: &Usersets{Child: []*Userset{a, b}}}
					case 2:
						return &Userset{Intersection: &Usersets{Child: []*Userset{a, b}}}
					default:
						return &Userset{Difference: &Difference{Base: a, Subtract: b}}
					}
				}
				// Computed usersets point to earlier relations only, since
				// cycles of computed usersets alone are invalid. Cycles come
				// from tuple to usersets and userset restrictions instead.
				earlier := slices.DeleteFunc(slices.Clone(g.rels[typ]), func(r string) bool { return r >= rel })
				if _, ok := tuplesets[typ]; ok && rng.IntN(4) == 0 {
					earlier = append(earlier, "p")
				}
				var kinds []int
				if !usedThis {
					kinds = append(kinds, 0)
				}
				if len(earlier) > 0 {
					kinds = append(kinds, 1)
				}
				if _, ok := tuplesets[typ]; ok {
					kinds = append(kinds, 2)
				}
				if len(kinds) == 0 {
					return nil
				}
				switch kinds[rng.IntN(len(kinds))] {
				case 0:
					usedThis = true
					direct = g.restrictions()
					return &Userset{This: &struct{}{}}
				case 1:
					return &Userset{ComputedUserset: &ObjectRelation{Relation: earlier[rng.IntN(len(earlier))]}}
				default:
					refs := tuplesets[typ]
					target := refs[rng.IntN(len(refs))].Type
					return &Userset{TupleToUserset: &TupleToUserset{
						Tupleset:        ObjectRelation{Relation: "p"},
						ComputedUserset: ObjectRelation{Relation: g.rels[target][rng.IntN(len(g.rels[target]))]},
					}}
				}
			}
			u := gen(0)
			if !usedThis && rng.IntN(3) > 0 {
				// Most relations allow direct tuples, so that data reaches them.
				direct = g.restrictions()
				u = &Userset{Union: &Usersets{Child: []*Userset{{This: &struct{}{}}, u}}}
			}
			td.Relations[rel] = u
			if len(direct) > 0 {
				td.Metadata.Relations[rel] = &RelationMetadata{DirectlyRelatedUserTypes: direct}
			}
		}
		if len(td.Metadata.Relations) == 0 {
			td.Metadata = nil
		}
		g.m.TypeDefinitions = append(g.m.TypeDefinitions, td)
	}
	return g
}

// restrictions returns random type restrictions of a direct relation.
func (g *modelGen) restrictions() []*RelationReference {
	rng := g.rng
	seen := map[string]bool{}
	var out []*RelationReference
	add := func(ref *RelationReference) {
		k := fmt.Sprintf("%s#%s*%v@%s", ref.Type, ref.Relation, ref.Wildcard != nil, ref.Condition)
		if !seen[k] {
			seen[k] = true
			out = append(out, ref)
		}
	}
	add(&RelationReference{Type: "user"})
	for range rng.IntN(4) {
		switch rng.IntN(6) {
		case 0:
			add(&RelationReference{Type: "user", Wildcard: &struct{}{}})
		case 1:
			add(&RelationReference{Type: "user", Condition: []string{condFlag, condNum}[rng.IntN(2)]})
		case 2:
			add(&RelationReference{Type: g.types[rng.IntN(len(g.types))]})
		case 3:
			add(&RelationReference{Type: g.types[rng.IntN(len(g.types))], Wildcard: &struct{}{}})
		default:
			typ := g.types[rng.IntN(len(g.types))]
			ref := &RelationReference{Type: typ, Relation: g.rels[typ][rng.IntN(len(g.rels[typ]))]}
			if rng.IntN(6) == 0 {
				ref.Condition = condFlag
			}
			add(ref)
		}
	}
	return out
}

func (g *modelGen) user() string {
	return fmt.Sprintf("user:u%d", g.rng.IntN(4))
}

func (g *modelGen) object(typ string) string {
	return typ + ":" + g.objects[g.rng.IntN(len(g.objects))]
}

func (g *modelGen) conditionContext(name string) map[string]any {
	rng := g.rng
	ctx := map[string]any{}
	switch name {
	case condFlag:
		if rng.IntN(2) == 0 {
			ctx["on"] = rng.IntN(2) == 0
		}
	case condNum:
		if rng.IntN(2) == 0 {
			ctx["limit"] = rng.IntN(4)
		}
		if rng.IntN(3) == 0 {
			ctx["x"] = rng.IntN(4)
		}
	}
	if len(ctx) == 0 {
		return nil
	}
	return ctx
}

func (g *modelGen) requestContext() map[string]any {
	if g.rng.IntN(10) < 3 {
		return nil
	}
	return map[string]any{"on": g.rng.IntN(2) == 0, "x": g.rng.IntN(4), "limit": g.rng.IntN(4)}
}

// tupleFor returns a random tuple that the model allows on the relation, or
// false when the relation takes no direct tuples.
func (g *modelGen) tupleFor(typ, rel string) (TupleKey, bool) {
	var refs []*RelationReference
	for _, td := range g.m.TypeDefinitions {
		if td.Type == typ && td.Metadata != nil && td.Metadata.Relations[rel] != nil {
			refs = td.Metadata.Relations[rel].DirectlyRelatedUserTypes
		}
	}
	if len(refs) == 0 {
		return TupleKey{}, false
	}
	ref := refs[g.rng.IntN(len(refs))]
	k := TupleKey{Relation: rel, Object: g.object(typ)}
	switch {
	case ref.Wildcard != nil:
		k.User = ref.Type + ":*"
	case ref.Type == "user":
		k.User = g.user()
	case ref.Relation != "":
		k.User = g.object(ref.Type) + "#" + ref.Relation
	default:
		k.User = g.object(ref.Type)
	}
	if k.User == k.Object+"#"+rel {
		// OpenFGA and Gebra reject implicit tuples.
		return TupleKey{}, false
	}
	if ref.Condition != "" {
		k.Condition = &RelationshipCondition{Name: ref.Condition, Context: g.conditionContext(ref.Condition)}
	}
	return k, true
}

// randomTuple returns a random allowed tuple of any relation.
func (g *modelGen) randomTuple() TupleKey {
	for {
		typ := g.types[g.rng.IntN(len(g.types))]
		rels := append(slices.Clone(g.rels[typ]), "p")
		if k, ok := g.tupleFor(typ, rels[g.rng.IntN(len(rels))]); ok {
			return k
		}
	}
}

func (g *modelGen) tuples(n int) []TupleKey {
	seen := map[string]bool{}
	var out []TupleKey
	for range n {
		k := g.randomTuple()
		key := k.Object + "#" + k.Relation + "@" + k.User
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, k)
	}
	return out
}

func (g *modelGen) subject() string {
	rng := g.rng
	switch rng.IntN(10) {
	case 0:
		return "user:*"
	case 1, 2:
		typ := g.types[rng.IntN(len(g.types))]
		return g.object(typ) + "#" + g.rels[typ][rng.IntN(len(g.rels[typ]))]
	case 3:
		return g.object(g.types[rng.IntN(len(g.types))])
	}
	return g.user()
}

func (g *modelGen) anyRelation(typ string) string {
	rels := g.rels[typ]
	return rels[g.rng.IntN(len(rels))]
}

// queries returns random queries, some with contextual tuples and request
// context.
func (g *modelGen) queries(n int) []*diffQuery {
	rng := g.rng
	var out []*diffQuery
	for range n {
		typ := g.types[rng.IntN(len(g.types))]
		rel := g.anyRelation(typ)
		var q *diffQuery
		switch r := rng.IntN(20); {
		case r < 9:
			q = &diffQuery{Kind: kindCheck, User: g.subject(), Relation: rel, Object: g.object(typ)}
		case r < 13:
			user := g.subject()
			if rng.IntN(3) > 0 {
				user = g.user()
			}
			q = &diffQuery{Kind: kindListObjects, Type: typ, Relation: rel, User: user}
			if rng.IntN(4) == 0 {
				q.Kind = kindStreamedObject
			}
		case r < 18:
			q = &diffQuery{Kind: kindListUsers, Object: g.object(typ), Relation: rel, Filter: UserFilter{Type: "user"}}
			if rng.IntN(3) == 0 {
				ft := g.types[rng.IntN(len(g.types))]
				q.Filter.Type = ft
				if rng.IntN(2) == 0 {
					q.Filter.Relation = g.anyRelation(ft)
				}
			}
		default:
			q = &diffQuery{Kind: kindExpand, Object: g.object(typ), Relation: rel}
		}
		if rng.IntN(4) == 0 {
			seen := map[string]bool{}
			for range 1 + rng.IntN(2) {
				k := g.randomTuple()
				key := k.Object + "#" + k.Relation + "@" + k.User
				if !seen[key] {
					seen[key] = true
					q.Contextual = append(q.Contextual, k)
				}
			}
		}
		if q.Kind != kindExpand {
			q.Context = g.requestContext()
		}
		out = append(out, q)
	}
	return out
}

func modelJSON(m *AuthorizationModel) string {
	b, _ := json.Marshal(openFGAModel(m))
	return string(b)
}

// fuzzStats counts what the fuzzer ran and what it found.
type fuzzStats struct {
	models, rejectedModels, tuples, queries int
	classes                                 map[string]int
	examples                                map[string]string
}

// TestDifferentialFuzz generates random models, tuples, and queries and
// compares Gebra in every mode with OpenFGA. It runs when GEBRA_FUZZ is set
// to the number of models. Failing cases are shrunk and saved to
// GEBRA_FUZZ_OUT, by default testdata/fuzz.
func TestDifferentialFuzz(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("GEBRA_FUZZ"))
	if n <= 0 {
		t.Skip("set GEBRA_FUZZ to the number of random models to run")
	}
	seed, _ := strconv.ParseUint(os.Getenv("GEBRA_FUZZ_SEED"), 10, 64)
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	outDir := os.Getenv("GEBRA_FUZZ_OUT")
	if outDir == "" {
		outDir = filepath.Join("testdata", "fuzz")
	}
	t.Logf("seed %d", seed)
	ctx := context.Background()
	targets := diffTargets(t)
	stats := fuzzStats{classes: map[string]int{}, examples: map[string]string{}}
	saved := map[string]int{}

	for i := range n {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		g := newModelGen(rng)
		sc := &diffScenario{
			Name:    fmt.Sprintf("seed-%d-%d", seed, i),
			Model:   modelJSON(g.m),
			Tuples:  g.tuples(10 + rng.IntN(40)),
			Queries: g.queries(30),
		}
		res, err := runDiff(ctx, sc, targets)
		require.NoError(t, err, "model %d", i)
		stats.models++
		if len(res.outcomes) == 0 {
			stats.rejectedModels++
			continue
		}
		stats.tuples += len(sc.Tuples)
		stats.queries += len(sc.Queries)
		for _, mm := range res.mismatches {
			stats.classes[mm.Target+" "+mm.Class]++
			key := mm.Class
			if saved[key] >= 3 {
				continue
			}
			saved[key]++
			t.Logf("model %d: %s %s, shrinking", i, mm.Target, mm.Class)
			min := shrinkScenario(ctx, t, sc, mm, targets)
			path := saveScenario(t, outDir, mm.Class, min)
			stats.examples[mm.Target+" "+mm.Class] = path
		}
		if i%50 == 49 {
			t.Logf("%d models, %d mismatch classes", i+1, len(stats.classes))
		}
	}
	t.Logf("models %d (rejected %d), tuples %d, queries %d", stats.models, stats.rejectedModels, stats.tuples, stats.queries)
	var classes []string
	for c := range stats.classes {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		t.Logf("mismatch %s: %d (example %s)", c, stats.classes[c], stats.examples[c])
	}
	if out := os.Getenv("GEBRA_FUZZ_STATS"); out != "" {
		b, _ := json.MarshalIndent(map[string]any{
			"seed": seed, "models": stats.models, "rejected_models": stats.rejectedModels,
			"tuples": stats.tuples, "queries": stats.queries, "classes": stats.classes, "examples": stats.examples,
		}, "", "  ")
		require.NoError(t, os.WriteFile(out, b, 0o644))
	}
}

// reproduces reports whether the scenario still shows a mismatch of the
// class on the target.
func reproduces(ctx context.Context, sc *diffScenario, target, class string, targets []diffTarget) bool {
	res, err := runDiff(ctx, sc, targets)
	if err != nil {
		return false
	}
	for _, m := range res.mismatches {
		if m.Target == target && m.Class == class {
			return true
		}
	}
	return false
}

// shrinkScenario reduces the scenario to a single query, the fewest tuples,
// and a simpler model that still show the mismatch.
func shrinkScenario(ctx context.Context, t *testing.T, sc *diffScenario, mm diffMismatch, targets []diffTarget) *diffScenario {
	t.Helper()
	cur := *sc
	if mm.Query >= 0 {
		cur.Queries = []*diffQuery{sc.Queries[mm.Query]}
	}
	try := func(cand *diffScenario) bool {
		if reproduces(ctx, cand, mm.Target, mm.Class, targets) {
			cur = *cand
			return true
		}
		return false
	}
	if !try(&cur) {
		// Flaky on its own, keep the full scenario.
		return sc
	}
	for round := 0; round < 20; round++ {
		changed := false
		for i := 0; i < len(cur.Tuples); i++ {
			cand := cur
			cand.Tuples = slices.Delete(slices.Clone(cur.Tuples), i, i+1)
			if try(&cand) {
				changed = true
				i--
			}
		}
		if len(cur.Queries) == 1 {
			q := *cur.Queries[0]
			for i := 0; i < len(q.Contextual); i++ {
				cq := q
				cq.Contextual = slices.Delete(slices.Clone(q.Contextual), i, i+1)
				cand := cur
				cand.Queries = []*diffQuery{&cq}
				if try(&cand) {
					q = cq
					changed = true
					i--
				}
			}
			if q.Context != nil {
				cq := q
				cq.Context = nil
				cand := cur
				cand.Queries = []*diffQuery{&cq}
				if try(&cand) {
					changed = true
				}
			}
		}
		for _, m := range modelSimplifications(cur.Model) {
			if m == cur.Model {
				continue
			}
			cand := cur
			cand.Model = m
			if try(&cand) {
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	return &cur
}

// modelSimplifications returns models that each make one rewrite node of the
// model simpler: an operator replaced by one of its operands, a relation
// replaced by a direct relation of users, or a type restriction dropped.
func modelSimplifications(src string) []string {
	var base AuthorizationModel
	if json.Unmarshal([]byte(src), &base) != nil {
		return nil
	}
	var out []string
	emit := func(edit func(m *AuthorizationModel) bool) {
		var m AuthorizationModel
		_ = json.Unmarshal([]byte(src), &m)
		if !edit(&m) {
			return
		}
		if _, err := compileModel(&m, DefaultConditionCostLimit); err != nil {
			return
		}
		if j := modelJSON(&m); j != modelJSON(&base) {
			out = append(out, j)
		}
	}
	for ti, td := range base.TypeDefinitions {
		for rel := range td.Relations {
			// Number the operator nodes of the rewrite.
			count := 0
			walkUserset(td.Relations[rel], func(u *Userset) {
				if u.Union != nil || u.Intersection != nil || u.Difference != nil {
					count++
				}
			})
			for node := range count {
				for child := range 3 {
					emit(func(m *AuthorizationModel) bool {
						seen := -1
						var replace func(u *Userset) *Userset
						replace = func(u *Userset) *Userset {
							if u.Union != nil || u.Intersection != nil || u.Difference != nil {
								seen++
								if seen == node {
									var kids []*Userset
									switch {
									case u.Union != nil:
										kids = u.Union.Child
									case u.Intersection != nil:
										kids = u.Intersection.Child
									default:
										kids = []*Userset{u.Difference.Base, u.Difference.Subtract}
									}
									if child >= len(kids) {
										return u
									}
									return kids[child]
								}
							}
							for _, us := range []*Usersets{u.Union, u.Intersection} {
								if us == nil {
									continue
								}
								for i, c := range us.Child {
									if r := replace(c); r != nil {
										us.Child[i] = r
									}
								}
							}
							if u.Difference != nil {
								if r := replace(u.Difference.Base); r != nil {
									u.Difference.Base = r
								}
								if r := replace(u.Difference.Subtract); r != nil {
									u.Difference.Subtract = r
								}
							}
							return nil
						}
						r := replace(m.TypeDefinitions[ti].Relations[rel])
						if seen < node {
							return false
						}
						if r != nil {
							m.TypeDefinitions[ti].Relations[rel] = r
						}
						fixDirect(m.TypeDefinitions[ti], rel)
						return true
					})
				}
			}
			emit(func(m *AuthorizationModel) bool {
				t := m.TypeDefinitions[ti]
				if t.Relations[rel].This != nil {
					return false
				}
				t.Relations[rel] = &Userset{This: &struct{}{}}
				if t.Metadata == nil {
					t.Metadata = &Metadata{}
				}
				if t.Metadata.Relations == nil {
					t.Metadata.Relations = map[string]*RelationMetadata{}
				}
				t.Metadata.Relations[rel] = &RelationMetadata{DirectlyRelatedUserTypes: []*RelationReference{{Type: "user"}}}
				return true
			})
			if td.Metadata != nil && td.Metadata.Relations[rel] != nil {
				for i := range td.Metadata.Relations[rel].DirectlyRelatedUserTypes {
					emit(func(m *AuthorizationModel) bool {
						refs := m.TypeDefinitions[ti].Metadata.Relations[rel]
						if len(refs.DirectlyRelatedUserTypes) < 2 {
							return false
						}
						refs.DirectlyRelatedUserTypes = slices.Delete(refs.DirectlyRelatedUserTypes, i, i+1)
						return true
					})
				}
			}
		}
	}
	return out
}

// fixDirect drops the type restrictions of a relation whose rewrite no
// longer allows direct tuples.
func fixDirect(td *TypeDefinition, rel string) {
	has := false
	walkUserset(td.Relations[rel], func(u *Userset) {
		if u.This != nil {
			has = true
		}
	})
	if !has && td.Metadata != nil {
		delete(td.Metadata.Relations, rel)
	}
}

// saveScenario writes the scenario as a regression file and returns its
// path.
func saveScenario(t *testing.T, dir, class string, sc *diffScenario) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b, err := json.MarshalIndent(sc, "", "  ")
	require.NoError(t, err)
	sum := sha256.Sum256(b)
	name := strings.NewReplacer(":", "-", "+", "-").Replace(class) + "-" + hex.EncodeToString(sum[:4]) + ".json"
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, b, 0o644))
	return path
}

// TestFuzzRegressions replays the saved scenarios of testdata/fuzz, or of
// GEBRA_REPLAY, on every target. Queries marked known are logged, and every
// other mismatch fails.
func TestFuzzRegressions(t *testing.T) {
	dir := os.Getenv("GEBRA_REPLAY")
	if dir == "" {
		dir = filepath.Join("testdata", "fuzz")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	if len(files) == 0 {
		t.Skip("no saved scenarios")
	}
	ctx := context.Background()
	targets := diffTargets(t)
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			require.NoError(t, err)
			var sc diffScenario
			require.NoError(t, json.Unmarshal(b, &sc))
			res, err := runDiff(ctx, &sc, targets)
			require.NoError(t, err)
			for name, outs := range res.outcomes {
				for i, o := range outs {
					t.Logf("%s: %s = %s", name, sc.Queries[i], o)
				}
			}
			for name, err := range res.setupErrs {
				t.Logf("%s setup: %v", name, err)
			}
			for _, m := range res.mismatches {
				known := ""
				if m.Query >= 0 {
					known = sc.Queries[m.Query].Known
				} else {
					known = sc.Note
				}
				if known != "" {
					t.Logf("known difference %s: %s (%s)", m.Target, m.Class, known)
					continue
				}
				t.Errorf("%s: %s: got %s, want %s", m.Target, m.Class, m.Got, m.Want)
			}
		})
	}
}

// TestFuzzTriage assigns a root cause to every saved mismatch of the
// directory GEBRA_TRIAGE by asking OpenFGA follow-up questions: whether it
// answers the same query the same way again, and whether its Check agrees
// with its own list results on the items where it differs from Gebra.
func TestFuzzTriage(t *testing.T) {
	dir := os.Getenv("GEBRA_TRIAGE")
	if dir == "" || openFGAURL() == "" {
		t.Skip("set GEBRA_TRIAGE and GEBRA_OPENFGA_URL")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	ctx := context.Background()
	targets := diffTargets(t)
	fga := targets[0].(*fgaTarget)
	verdicts := map[string]int{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		var sc diffScenario
		require.NoError(t, json.Unmarshal(b, &sc))
		res, err := runDiff(ctx, &sc, targets)
		require.NoError(t, err)
		verdict := triage(ctx, fga, &sc, res)
		verdicts[verdict]++
		t.Logf("%s: %s", filepath.Base(f), verdict)
	}
	var keys []string
	for k := range verdicts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("verdict %q: %d", k, verdicts[k])
	}
}

func triage(ctx context.Context, fga *fgaTarget, sc *diffScenario, res *diffResult) string {
	var mm *diffMismatch
	for i := range res.mismatches {
		if res.mismatches[i].Target == "gebra-view" || res.mismatches[i].Query < 0 {
			mm = &res.mismatches[i]
			break
		}
	}
	if mm == nil {
		return "no longer reproduces"
	}
	if mm.Query < 0 {
		return "setup: " + mm.Class
	}
	q := sc.Queries[mm.Query]
	gebra := res.outcomes["gebra-view"][mm.Query]
	for range 5 {
		if again := fga.run(ctx, q); !again.same(mm.Want) {
			return "OpenFGA answers the same query differently on repeated calls"
		}
	}
	m, _ := scenarioModel(sc)
	cm, _ := compileModel(m, DefaultConditionCostLimit)
	tags := ""
	if cm != nil && negationCycle(cm) {
		tags = " (model has a cycle through an exclusion)"
	}
	if gebra.Err != "" || mm.Want.Err != "" {
		if strings.Contains(gebra.Err+mm.Want.Err, "missing") {
			return "missing condition parameter: error reported by one side only" + tags
		}
		return "error on one side: " + gebra.Err + mm.Want.Err
	}
	if q.Kind == kindListObjects || q.Kind == kindStreamedObject || q.Kind == kindListUsers {
		g := strings.Split(gebra.Value, ",")
		w := strings.Split(mm.Want.Value, ",")
		var diff []string
		for _, x := range g {
			if x != "" && !slices.Contains(w, x) {
				diff = append(diff, x)
			}
		}
		for _, x := range w {
			if x != "" && !slices.Contains(g, x) {
				diff = append(diff, x)
			}
		}
		agree, disagree := 0, 0
		for _, item := range diff {
			cq := &diffQuery{Kind: kindCheck, Relation: q.Relation, Contextual: q.Contextual, Context: q.Context}
			if q.Kind == kindListUsers {
				if strings.HasSuffix(item, ":*") && q.Filter.Relation != "" {
					return "OpenFGA ListUsers returns a wildcard for a userset filter, which its Check denies" + tags
				}
				cq.User, cq.Object = item, q.Object
			} else {
				cq.User, cq.Object = q.User, item
			}
			out := fga.run(ctx, cq)
			inGebra := slices.Contains(g, item)
			if out.Err == "" && out.Value == fmt.Sprint(inGebra) {
				agree++
			} else {
				disagree++
			}
		}
		if disagree == 0 {
			return "OpenFGA list contradicts its own Check, which agrees with Gebra" + tags
		}
		return fmt.Sprintf("OpenFGA Check agrees with its list on %d of %d items%s", disagree, agree+disagree, tags)
	}
	if q.Kind == kindCheck {
		return "check differs" + tags
	}
	return "other " + mm.Class + tags
}
