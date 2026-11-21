package gebra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

// Query kinds of a differential scenario.
const (
	kindCheck          = "check"
	kindListObjects    = "list_objects"
	kindStreamedObject = "streamed_list_objects"
	kindListUsers      = "list_users"
	kindExpand         = "expand"
)

// diffQuery is one query of a differential scenario.
type diffQuery struct {
	Kind       string         `json:"kind"`
	User       string         `json:"user,omitempty"`
	Relation   string         `json:"relation"`
	Object     string         `json:"object,omitempty"`
	Type       string         `json:"type,omitempty"`
	Filter     UserFilter     `json:"filter,omitzero"`
	Contextual []TupleKey     `json:"contextual_tuples,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
	// Expect is the known answer of a hand-written query: "true", "false",
	// "error", or a comma-separated sorted list.
	Expect *string `json:"expect,omitempty"`
	// Known explains a difference between Gebra and OpenFGA on this query
	// that is understood, which tests report without failing.
	Known string `json:"known,omitempty"`
}

func (q *diffQuery) String() string {
	var s string
	switch q.Kind {
	case kindCheck:
		s = fmt.Sprintf("check(%s, %s, %s)", q.User, q.Relation, q.Object)
	case kindListObjects, kindStreamedObject:
		s = fmt.Sprintf("%s(%s, %s, %s)", q.Kind, q.Type, q.Relation, q.User)
	case kindListUsers:
		f := q.Filter.Type
		if q.Filter.Relation != "" {
			f += "#" + q.Filter.Relation
		}
		s = fmt.Sprintf("list_users(%s, %s, %s)", q.Object, q.Relation, f)
	case kindExpand:
		s = fmt.Sprintf("expand(%s, %s)", q.Object, q.Relation)
	}
	if len(q.Contextual) > 0 {
		b, _ := json.Marshal(q.Contextual)
		s += " contextual=" + string(b)
	}
	if q.Context != nil {
		b, _ := json.Marshal(q.Context)
		s += " context=" + string(b)
	}
	return s
}

// diffScenario is a model, the tuples to store, and the queries to compare.
type diffScenario struct {
	Name    string       `json:"name"`
	Note    string       `json:"note,omitempty"`
	Model   string       `json:"model"`
	Tuples  []TupleKey   `json:"tuples"`
	Queries []*diffQuery `json:"queries"`
}

// diffOutcome is the normalized answer of a target to a query.
type diffOutcome struct {
	Err   string `json:"err,omitempty"`
	Value string `json:"value"`
}

func (o diffOutcome) String() string {
	if o.Err != "" {
		return "error: " + o.Err
	}
	return o.Value
}

func (o diffOutcome) same(p diffOutcome) bool {
	if (o.Err != "") != (p.Err != "") {
		return false
	}
	return o.Err != "" || o.Value == p.Value
}

// matches reports whether the outcome is the expected answer.
func (o diffOutcome) matches(expect string) bool {
	if expect == "error" {
		return o.Err != ""
	}
	return o.Err == "" && o.Value == expect
}

func errOutcome(err error) diffOutcome {
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return diffOutcome{Err: msg}
}

// diffTarget is an implementation that answers scenario queries.
type diffTarget interface {
	name() string
	// setup stores the model and the tuples. It returns errModel or
	// errTuples wrapped around the reason when the target rejects them.
	setup(ctx context.Context, sc *diffScenario, m *AuthorizationModel) error
	run(ctx context.Context, q *diffQuery) diffOutcome
}

var (
	errModel  = errors.New("model rejected")
	errTuples = errors.New("tuples rejected")
)

// gebraTarget answers with Gebra in one of its modes: "view" uses a view that
// the test applies writes to, "graph" evaluates the graph by querying an older
// copy of the latest model, and "applier" uses a view that the store's
// applier maintains, waiting for it on every write.
type gebraTarget struct {
	mode    string
	e       *Engine
	storeID string
	// modelID is the model that queries name, which is an older copy of the
	// latest model in graph mode, and empty otherwise.
	modelID string
}

func newGebraTargets(t *testing.T) []diffTarget {
	t.Helper()
	ctx := context.Background()
	open := func(name string, opts Options) *Engine {
		db, dialect := newTestDB(t, testSuite(t.Name()+"-"+name))
		e, err := New(ctx, logx.NewNoopLogger(), db, dialect, opts)
		require.NoError(t, err)
		t.Cleanup(e.Close)
		return e
	}
	manual := open("manual", Options{manualApply: true})
	return []diffTarget{
		&gebraTarget{mode: "view", e: manual},
		&gebraTarget{mode: "graph", e: manual},
		&gebraTarget{mode: "applier", e: open("applier", Options{})},
	}
}

func (g *gebraTarget) name() string { return "gebra-" + g.mode }

func (g *gebraTarget) setup(ctx context.Context, sc *diffScenario, m *AuthorizationModel) error {
	st, err := g.e.CreateStore(ctx, "diff")
	if err != nil {
		return err
	}
	g.storeID, g.modelID = st.ID, ""
	first, err := g.e.WriteAuthorizationModel(ctx, st.ID, m)
	if err != nil {
		return errors.Mark(err, errModel)
	}
	if g.mode == "graph" {
		g.modelID = first
		if _, err := g.e.WriteAuthorizationModel(ctx, st.ID, m); err != nil {
			return err
		}
	}
	for ts := sc.Tuples; len(ts) > 0; {
		n := min(len(ts), 1000)
		if _, err := g.e.Write(ctx, st.ID, &WriteRequest{Writes: ts[:n], Wait: g.mode == "applier"}); err != nil {
			return errors.Mark(err, errTuples)
		}
		ts = ts[n:]
	}
	if g.e.opts.manualApply {
		for {
			more, err := g.e.applyBatch(ctx, st.ID)
			if err != nil {
				return err
			}
			if !more {
				break
			}
		}
	}
	return nil
}

func (g *gebraTarget) run(ctx context.Context, q *diffQuery) diffOutcome {
	return runGebraQuery(ctx, g.e, g.storeID, g.modelID, Consistency{}, q)
}

// runGebraQuery answers a query with the model and consistency.
func runGebraQuery(ctx context.Context, e *Engine, storeID, modelID string, c Consistency, q *diffQuery) diffOutcome {
	switch q.Kind {
	case kindCheck:
		ok, err := e.Check(ctx, storeID, &CheckRequest{
			TupleKey:         CheckTupleKey{User: q.User, Relation: q.Relation, Object: q.Object},
			ContextualTuples: q.Contextual, Context: q.Context, AuthorizationModelID: modelID, Consistency: c,
		})
		if err != nil {
			return errOutcome(err)
		}
		return diffOutcome{Value: fmt.Sprint(ok)}
	case kindListObjects:
		objs, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{
			Type: q.Type, Relation: q.Relation, User: q.User,
			ContextualTuples: q.Contextual, Context: q.Context, AuthorizationModelID: modelID, Consistency: c,
		})
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(objs)
	case kindStreamedObject:
		var objs []string
		err := e.StreamListObjects(ctx, storeID, &ListObjectsRequest{
			Type: q.Type, Relation: q.Relation, User: q.User,
			ContextualTuples: q.Contextual, Context: q.Context, AuthorizationModelID: modelID, Consistency: c,
		}, func(o string) error {
			objs = append(objs, o)
			return nil
		})
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(objs)
	case kindListUsers:
		users, _, err := e.ListUsers(ctx, storeID, &ListUsersRequest{
			Object: q.Object, Relation: q.Relation, UserFilters: []UserFilter{q.Filter},
			ContextualTuples: q.Contextual, Context: q.Context, AuthorizationModelID: modelID, Consistency: c,
		})
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(userStrings(users))
	case kindExpand:
		tree, err := e.Expand(ctx, storeID, &ExpandRequest{
			Object: q.Object, Relation: q.Relation, ContextualTuples: q.Contextual, AuthorizationModelID: modelID, Consistency: c,
		})
		if err != nil {
			return errOutcome(err)
		}
		return expandOutcome(tree)
	}
	return diffOutcome{Err: "unknown query kind " + q.Kind}
}

// fgaTarget answers with a server that speaks OpenFGA's HTTP API: OpenFGA
// itself, or the Gebra binary.
type fgaTarget struct {
	label   string
	c       *fgaClient
	storeID string
	modelID string
	model   *compiledModel
}

func (f *fgaTarget) name() string { return f.label }

func (f *fgaTarget) setup(ctx context.Context, sc *diffScenario, m *AuthorizationModel) error {
	var err error
	if f.storeID, err = f.c.createStore(ctx, "diff"); err != nil {
		return err
	}
	if f.modelID, err = f.c.writeModel(ctx, f.storeID, m); err != nil {
		return errors.Mark(err, errModel)
	}
	f.model, _ = compileModel(m, DefaultConditionCostLimit)
	if err := f.c.writeAll(ctx, f.storeID, sc.Tuples); err != nil {
		return errors.Mark(err, errTuples)
	}
	return nil
}

func (f *fgaTarget) run(ctx context.Context, q *diffQuery) diffOutcome {
	switch q.Kind {
	case kindCheck:
		ok, err := f.c.check(ctx, f.storeID, f.modelID, q)
		if err != nil {
			return errOutcome(err)
		}
		return diffOutcome{Value: fmt.Sprint(ok)}
	case kindListObjects:
		objs, err := f.c.listObjects(ctx, f.storeID, q)
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(objs)
	case kindStreamedObject:
		objs, err := f.c.streamedListObjects(ctx, f.storeID, q)
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(objs)
	case kindListUsers:
		users, err := f.c.listUsers(ctx, f.storeID, q)
		if err != nil {
			return errOutcome(err)
		}
		return listOutcome(users)
	case kindExpand:
		tree, err := f.c.expand(ctx, f.storeID, q)
		if err != nil {
			return errOutcome(err)
		}
		if f.model != nil {
			dropUndefinedComputed(f.model, tree)
		}
		return expandOutcome(tree)
	}
	return diffOutcome{Err: "unknown query kind " + q.Kind}
}

func listOutcome(items []string) diffOutcome {
	items = slices.Clone(items)
	slices.Sort(items)
	return diffOutcome{Value: strings.Join(items, ",")}
}

// dropUndefinedComputed removes the usersets that OpenFGA lists in tuple to
// userset leaves for objects whose type does not define the relation. Gebra
// omits them, and they grant nothing.
func dropUndefinedComputed(m *compiledModel, n *ExpandNode) {
	if n == nil {
		return
	}
	if n.Leaf != nil && n.Leaf.TupleToUserset != nil {
		n.Leaf.TupleToUserset.Computed = slices.DeleteFunc(n.Leaf.TupleToUserset.Computed, func(c ExpandComputed) bool {
			typ, _, rel := splitSetKey(c.Userset)
			return m.relation(typ, rel) == nil
		})
	}
	for _, ns := range []*ExpandNodes{n.Union, n.Intersection} {
		if ns != nil {
			for _, c := range ns.Nodes {
				dropUndefinedComputed(m, c)
			}
		}
	}
	if n.Difference != nil {
		dropUndefinedComputed(m, n.Difference.Base)
		dropUndefinedComputed(m, n.Difference.Subtract)
	}
}

// expandOutcome renders an expand tree with its user lists sorted, since
// neither implementation promises an order for them.
func expandOutcome(n *ExpandNode) diffOutcome {
	var norm func(n *ExpandNode)
	norm = func(n *ExpandNode) {
		if n == nil {
			return
		}
		if n.Leaf != nil {
			if n.Leaf.Users != nil {
				slices.Sort(n.Leaf.Users.Users)
			}
			if n.Leaf.TupleToUserset != nil {
				slices.SortFunc(n.Leaf.TupleToUserset.Computed, func(a, b ExpandComputed) int {
					return strings.Compare(a.Userset, b.Userset)
				})
			}
		}
		for _, ns := range []*ExpandNodes{n.Union, n.Intersection} {
			if ns != nil {
				for _, c := range ns.Nodes {
					norm(c)
				}
			}
		}
		if n.Difference != nil {
			norm(n.Difference.Base)
			norm(n.Difference.Subtract)
		}
	}
	norm(n)
	b, _ := json.Marshal(n)
	return diffOutcome{Value: string(b)}
}

// diffMismatch is a query that a target answered differently from the
// reference target.
type diffMismatch struct {
	Query  int         `json:"query"`
	Target string      `json:"target"`
	Got    diffOutcome `json:"got"`
	Want   diffOutcome `json:"want"`
	// Class names the kind of difference, e.g., "check:false-vs-true".
	Class string `json:"class"`
}

// diffResult is the outcome of running a scenario on every target.
type diffResult struct {
	// setupErrs holds the setup error of each target that failed setup.
	setupErrs  map[string]error
	outcomes   map[string][]diffOutcome
	mismatches []diffMismatch
}

// runDiff runs the scenario on every target and compares the answers with the
// first target that accepted the model, and with Expect when set.
func runDiff(ctx context.Context, sc *diffScenario, targets []diffTarget) (*diffResult, error) {
	m, err := scenarioModel(sc)
	if err != nil {
		return nil, err
	}
	res := &diffResult{setupErrs: map[string]error{}, outcomes: map[string][]diffOutcome{}}
	var ref diffTarget
	var ready []diffTarget
	for _, tg := range targets {
		if err := tg.setup(ctx, sc, m); err != nil {
			if !errors.Is(err, errModel) && !errors.Is(err, errTuples) {
				return nil, errors.Wrapf(err, "set up %s", tg.name())
			}
			res.setupErrs[tg.name()] = err
			continue
		}
		ready = append(ready, tg)
		if ref == nil {
			ref = tg
		}
	}
	if len(res.setupErrs) > 0 && len(ready) > 0 {
		for name, err := range res.setupErrs {
			class := "setup:tuples"
			if errors.Is(err, errModel) {
				class = "setup:model"
			}
			res.mismatches = append(res.mismatches, diffMismatch{
				Query: -1, Target: name, Got: errOutcome(err), Want: diffOutcome{Value: "accepted by " + ready[0].name()}, Class: class,
			})
		}
	}
	for _, tg := range ready {
		outs := make([]diffOutcome, len(sc.Queries))
		for i, q := range sc.Queries {
			outs[i] = tg.run(ctx, q)
		}
		res.outcomes[tg.name()] = outs
	}
	negCycle := false
	if cm, err := compileModel(m, DefaultConditionCostLimit); err == nil {
		negCycle = negationCycle(cm)
	}
	classify := func(q *diffQuery, got, want diffOutcome) string {
		c := classify(q, got, want)
		if negCycle {
			c += "+negation-cycle"
		}
		return c
	}
	for i, q := range sc.Queries {
		for _, tg := range ready {
			got := res.outcomes[tg.name()][i]
			if q.Expect != nil {
				if !got.matches(*q.Expect) {
					res.mismatches = append(res.mismatches, diffMismatch{
						Query: i, Target: tg.name(), Got: got, Want: diffOutcome{Value: *q.Expect}, Class: classify(q, got, diffOutcome{Value: *q.Expect}),
					})
				}
				continue
			}
			if tg == ref {
				continue
			}
			want := res.outcomes[ref.name()][i]
			if !got.same(want) {
				res.mismatches = append(res.mismatches, diffMismatch{
					Query: i, Target: tg.name(), Got: got, Want: want, Class: classify(q, got, want),
				})
			}
		}
	}
	return res, nil
}

// scenarioModel parses the scenario's model, which is either DSL or JSON.
func scenarioModel(sc *diffScenario) (*AuthorizationModel, error) {
	if strings.HasPrefix(strings.TrimSpace(sc.Model), "{") {
		var m AuthorizationModel
		if err := json.Unmarshal([]byte(sc.Model), &m); err != nil {
			return nil, errors.Wrap(err, "unmarshal model")
		}
		return &m, nil
	}
	m, err := ParseDSL(sc.Model)
	return m, errors.Wrap(err, "parse model")
}

// classify names the kind of a difference, so that mismatches with the same
// cause group together.
func classify(q *diffQuery, got, want diffOutcome) string {
	c := classifyBase(q, got, want)
	if strings.Contains(got.Err+want.Err, "missing") {
		c += "+missing-param"
	}
	if q.Kind == kindListUsers && q.Filter.Relation != "" && strings.HasSuffix(c, ":missing") && strings.HasSuffix(want.Value, ":*") {
		c += "+wildcard-for-userset-filter"
	}
	return c
}

// negationCycle reports whether a relation of the model depends on itself
// through the subtracted side of an exclusion, where evaluation order and
// cycle handling decide answers.
func negationCycle(m *compiledModel) bool {
	type node struct{ typ, rel string }
	type edge struct {
		to  node
		neg bool
	}
	graph := map[node][]edge{}
	for _, t := range m.types {
		for _, r := range t.relations {
			from := node{t.name, r.name}
			var walk func(u *Userset, neg bool)
			walk = func(u *Userset, neg bool) {
				switch {
				case u.This != nil:
					for _, ref := range r.direct {
						if ref.Relation != "" {
							graph[from] = append(graph[from], edge{node{ref.Type, ref.Relation}, neg})
						}
					}
				case u.ComputedUserset != nil:
					graph[from] = append(graph[from], edge{node{t.name, u.ComputedUserset.Relation}, neg})
				case u.TupleToUserset != nil:
					for _, ref := range m.relation(t.name, u.TupleToUserset.Tupleset.Relation).direct {
						if m.relation(ref.Type, u.TupleToUserset.ComputedUserset.Relation) != nil {
							graph[from] = append(graph[from], edge{node{ref.Type, u.TupleToUserset.ComputedUserset.Relation}, neg})
						}
					}
				case u.Union != nil:
					for _, c := range u.Union.Child {
						walk(c, neg)
					}
				case u.Intersection != nil:
					for _, c := range u.Intersection.Child {
						walk(c, neg)
					}
				case u.Difference != nil:
					walk(u.Difference.Base, neg)
					walk(u.Difference.Subtract, true)
				}
			}
			walk(r.rewrite, false)
		}
	}
	reach := func(from, to node) bool {
		seen := map[node]bool{from: true}
		stack := []node{from}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == to {
				return true
			}
			for _, e := range graph[n] {
				if !seen[e.to] {
					seen[e.to] = true
					stack = append(stack, e.to)
				}
			}
		}
		return false
	}
	for from, es := range graph {
		for _, e := range es {
			if e.neg && reach(e.to, from) {
				return true
			}
		}
	}
	return false
}

func classifyBase(q *diffQuery, got, want diffOutcome) string {
	kind := q.Kind
	if kind == kindStreamedObject {
		kind = kindListObjects
	}
	extra := ""
	if len(q.Contextual) > 0 {
		extra = "+contextual"
	}
	switch {
	case got.Err != "" && want.Err == "":
		return kind + extra + ":error-vs-answer"
	case got.Err == "" && want.Err != "":
		return kind + extra + ":answer-vs-error"
	case q.Kind == kindCheck:
		return kind + extra + ":" + got.Value + "-vs-" + want.Value
	case q.Kind == kindExpand:
		return kind + extra + ":tree"
	}
	g := strings.Split(got.Value, ",")
	w := strings.Split(want.Value, ",")
	missing, extraItems := false, false
	for _, x := range w {
		if x != "" && !slices.Contains(g, x) {
			missing = true
		}
	}
	for _, x := range g {
		if x != "" && !slices.Contains(w, x) {
			extraItems = true
		}
	}
	switch {
	case missing && extraItems:
		return kind + extra + ":different"
	case missing:
		return kind + extra + ":missing"
	default:
		return kind + extra + ":extra"
	}
}

// diffTargets returns Gebra in every mode, preceded by OpenFGA when
// GEBRA_OPENFGA_URL is set, so that OpenFGA is the reference.
func diffTargets(t *testing.T) []diffTarget {
	t.Helper()
	targets := newGebraTargets(t)
	if u := strings.TrimRight(os.Getenv("GEBRA_HTTP_URL"), "/"); u != "" {
		targets = append(targets, &fgaTarget{label: "gebra-http", c: &fgaClient{base: u, http: &http.Client{Timeout: 10 * time.Minute}}})
	}
	if openFGAURL() != "" {
		targets = append([]diffTarget{&fgaTarget{label: "openfga", c: newFGAClient(t)}}, targets...)
	}
	return targets
}

func ptr[T any](v T) *T { return &v }
