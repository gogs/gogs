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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiServers returns OpenFGA and the Gebra binary, which the API coverage
// tests talk to over HTTP only. They are skipped unless both are configured.
func apiServers(t *testing.T) (fga, gebra *fgaClient) {
	t.Helper()
	g := strings.TrimRight(os.Getenv("GEBRA_HTTP_URL"), "/")
	if openFGAURL() == "" || g == "" {
		t.Skip("set GEBRA_OPENFGA_URL and GEBRA_HTTP_URL to compare the HTTP APIs")
	}
	return newFGAClient(t), &fgaClient{base: g, http: &http.Client{Timeout: time.Minute}}
}

// statusOf renders the outcome of a call as "ok" or the HTTP status of the
// error, which is what the API coverage compares.
func statusOf(err error) string {
	if err == nil {
		return "ok"
	}
	var fe *fgaError
	if errors.As(err, &fe) {
		return fmt.Sprint(fe.Status)
	}
	return "transport: " + err.Error()
}

// modelCase is a model that OpenFGA either accepts or rejects. JSON is used
// when Gebra's DSL parser cannot express the model.
type modelCase struct {
	name  string
	dsl   string
	json  string
	known string
}

func modelValidationCases() []modelCase {
	head := "model\n  schema 1.1\ntype user\n"
	return []modelCase{
		{name: "valid", dsl: head + "type doc\n  relations\n    define viewer: [user]\n"},
		{name: "undefined type in restriction", dsl: head + "type doc\n  relations\n    define viewer: [nope]\n"},
		{name: "undefined computed relation", dsl: head + "type doc\n  relations\n    define viewer: editor\n"},
		{name: "undefined userset restriction", dsl: head + "type group\n  relations\n    define member: [user]\ntype doc\n  relations\n    define viewer: [group#nope]\n"},
		{name: "undefined tupleset", dsl: head + "type doc\n  relations\n    define viewer: [user] or viewer from nope\n"},
		{name: "tupleset is computed", dsl: head + "type doc\n  relations\n    define owner: [doc]\n    define parent: owner\n    define viewer: [user] or viewer from parent\n"},
		{name: "tupleset allows userset", dsl: head + "type folder\n  relations\n    define viewer: [user]\ntype doc\n  relations\n    define parent: [folder#viewer]\n    define viewer: [user] or viewer from parent\n"},
		{name: "tupleset allows wildcard", dsl: head + "type folder\n  relations\n    define viewer: [user]\ntype doc\n  relations\n    define parent: [folder, folder:*]\n    define viewer: [user] or viewer from parent\n"},
		{name: "tupleset is union with direct", dsl: head + "type folder\n  relations\n    define viewer: [user]\n    define p2: [folder]\ntype doc\n  relations\n    define p2: [folder]\n    define parent: [folder] or p2\n    define viewer: [user] or viewer from parent\n"},
		{name: "ttu target undefined on every type", dsl: head + "type folder\n  relations\n    define viewer: [user]\ntype doc\n  relations\n    define parent: [folder]\n    define viewer: [user] or nope from parent\n"},
		{name: "ttu target defined on some types", dsl: head + "type folder\n  relations\n    define viewer: [user]\ntype org\ntype doc\n  relations\n    define parent: [folder, org]\n    define viewer: [user] or viewer from parent\n"},
		{name: "computed cycle", dsl: head + "type doc\n  relations\n    define a: b\n    define b: a\n"},
		{name: "self reference", dsl: head + "type doc\n  relations\n    define a: a\n"},
		{name: "union cycle with entry", dsl: head + "type doc\n  relations\n    define a: [user] or b\n    define b: a\n"},
		{name: "ttu cycle without entry", dsl: head + "type doc\n  relations\n    define parent: [doc]\n    define viewer: viewer from parent\n"},
		{name: "ttu cycle with entry", dsl: head + "type doc\n  relations\n    define parent: [doc]\n    define viewer: [user] or viewer from parent\n"},
		{name: "intersection cycle without entry", dsl: head + "type doc\n  relations\n    define a: [user] and b\n    define b: [user] and a\n"},
		{name: "intersection with self", dsl: head + "type doc\n  relations\n    define a: [user] and a\n"},
		{name: "exclusion of self", dsl: head + "type doc\n  relations\n    define a: [user] but not a\n"},
		{name: "userset cycle without entry", dsl: head + "type group\n  relations\n    define member: [group#member]\n"},
		{name: "userset cycle with entry", dsl: head + "type group\n  relations\n    define member: [user, group#member]\n"},
		{name: "exclusion base cycles through userset", dsl: head + "type group\n  relations\n    define banned: [user]\n    define member: [user, group#allowed]\n    define allowed: member but not banned\n"},
		{name: "duplicate type", dsl: head + "type doc\ntype doc\n"},
		{name: "undefined condition", dsl: head + "type doc\n  relations\n    define viewer: [user with nope]\n"},
		{name: "unused condition", dsl: head + "type doc\n  relations\n    define viewer: [user]\ncondition c(x: int) {\n  x > 1\n}\n"},
		{name: "condition syntax error", dsl: head + "type doc\n  relations\n    define viewer: [user with c]\ncondition c(x: int) {\n  x >\n}\n"},
		{name: "condition not bool", dsl: head + "type doc\n  relations\n    define viewer: [user with c]\ncondition c(x: int) {\n  x + 1\n}\n"},
		{name: "condition undeclared variable", dsl: head + "type doc\n  relations\n    define viewer: [user with c]\ncondition c(x: int) {\n  y > 1\n}\n"},
		{name: "condition every param type", dsl: head + "type doc\n  relations\n    define viewer: [user with c]\ncondition c(a: int, b: uint, d: double, e: bool, f: string, g: duration, h: timestamp, i: list<int>, j: map<string>, k: ipaddress, l: any) {\n  a > 0\n}\n"},
		{name: "relation named this", dsl: head + "type doc\n  relations\n    define this: [user]\n"},
		{name: "relation named self", dsl: head + "type doc\n  relations\n    define self: [user]\n"},
		{name: "wildcard on a userset", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"group","relations":{"member":{"this":{}}},"metadata":{"relations":{"member":{"directly_related_user_types":[{"type":"user"}]}}}},{"type":"doc","relations":{"viewer":{"this":{}}},"metadata":{"relations":{"viewer":{"directly_related_user_types":[{"type":"group","relation":"member","wildcard":{}}]}}}}]}`},
		{name: "direct without restrictions", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"viewer":{"this":{}}}}]}`},
		{name: "restrictions without direct", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"owner":{"this":{}},"viewer":{"computedUserset":{"relation":"owner"}}},"metadata":{"relations":{"owner":{"directly_related_user_types":[{"type":"user"}]},"viewer":{"directly_related_user_types":[{"type":"user"}]}}}}]}`},
		{name: "schema 1.0", json: `{"schema_version":"1.0","type_definitions":[{"type":"user"}]}`},
		{name: "schema 1.2", known: "Gebra only accepts schema 1.1, OpenFGA also accepts the modular schema 1.2", json: `{"schema_version":"1.2","type_definitions":[{"type":"user"}]}`},
		{name: "no types", json: `{"schema_version":"1.1","type_definitions":[]}`},
		{name: "type name with space", json: `{"schema_version":"1.1","type_definitions":[{"type":"a b"}]}`},
		{name: "type name with colon", json: `{"schema_version":"1.1","type_definitions":[{"type":"a:b"}]}`},
		{name: "empty union", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"viewer":{"union":{"child":[]}}}}]}`},
		{name: "metadata for undefined relation", known: "Gebra rejects metadata for a relation the type does not define, OpenFGA ignores it", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"viewer":{"this":{}}},"metadata":{"relations":{"viewer":{"directly_related_user_types":[{"type":"user"}]},"nope":{"directly_related_user_types":[{"type":"user"}]}}}}]}`},
		{name: "condition name mismatch", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"viewer":{"this":{}}},"metadata":{"relations":{"viewer":{"directly_related_user_types":[{"type":"user","condition":"cc"}]}}}}],"conditions":{"cc":{"name":"dd","expression":"x > 1","parameters":{"x":{"type_name":"TYPE_NAME_INT"}}}}}`},
		{name: "computed userset with object", known: "Gebra rejects the object field of a computed userset, OpenFGA ignores it", json: `{"schema_version":"1.1","type_definitions":[{"type":"user"},{"type":"doc","relations":{"owner":{"this":{}},"viewer":{"computedUserset":{"object":"doc:1","relation":"owner"}}},"metadata":{"relations":{"owner":{"directly_related_user_types":[{"type":"user"}]}}}}]}`},
	}
}

// TestAPIModelValidation writes models that OpenFGA accepts or rejects to
// OpenFGA and the Gebra binary and compares the outcomes.
func TestAPIModelValidation(t *testing.T) {
	ctx := context.Background()
	fga, gb := apiServers(t)
	fgaStore, err := fga.createStore(ctx, "models")
	require.NoError(t, err)
	gbStore, err := gb.createStore(ctx, "models")
	require.NoError(t, err)
	for _, c := range modelValidationCases() {
		t.Run(c.name, func(t *testing.T) {
			var body json.RawMessage
			if c.json != "" {
				body = json.RawMessage(c.json)
			} else {
				m, err := ParseDSL(c.dsl)
				if err != nil {
					t.Logf("Gebra's DSL parser rejects the model: %v", err)
					body = nil
				} else {
					b, err := json.Marshal(openFGAModel(m))
					require.NoError(t, err)
					body = b
				}
			}
			if body == nil {
				// Compare OpenFGA on the JSON form is not possible, so record
				// only that Gebra rejects it.
				return
			}
			fgaErr := fga.do(ctx, http.MethodPost, "/stores/"+fgaStore+"/authorization-models", body, nil)
			gbErr := gb.do(ctx, http.MethodPost, "/stores/"+gbStore+"/authorization-models", body, nil)
			t.Logf("openfga: %s %v", statusOf(fgaErr), fgaErr)
			t.Logf("gebra: %s %v", statusOf(gbErr), gbErr)
			if c.known != "" && statusOf(fgaErr) != statusOf(gbErr) {
				t.Logf("known difference: %s", c.known)
				return
			}
			assert.Equal(t, statusOf(fgaErr), statusOf(gbErr))
		})
	}
}

// apiPair runs the same step on OpenFGA and the Gebra binary, each with its
// own store, and compares the results.
type apiPair struct {
	t                 *testing.T
	ctx               context.Context
	fga, gb           *fgaClient
	fgaStore, gbStore string
	fgaModel, gbModel string
	knownDifferences  map[string]string
}

func newAPIPair(t *testing.T, dsl string) *apiPair {
	t.Helper()
	ctx := context.Background()
	fga, gb := apiServers(t)
	p := &apiPair{t: t, ctx: ctx, fga: fga, gb: gb, knownDifferences: map[string]string{}}
	var err error
	p.fgaStore, err = fga.createStore(ctx, t.Name())
	require.NoError(t, err)
	p.gbStore, err = gb.createStore(ctx, t.Name())
	require.NoError(t, err)
	m, err := ParseDSL(dsl)
	require.NoError(t, err)
	p.fgaModel, err = fga.writeModel(ctx, p.fgaStore, m)
	require.NoError(t, err)
	p.gbModel, err = gb.writeModel(ctx, p.gbStore, m)
	require.NoError(t, err)
	return p
}

// same calls fn on both servers and checks that the results are equal.
// known, when not empty, explains an expected difference, which is logged.
func (p *apiPair) same(name, known string, fn func(c *fgaClient, store, model string) string) {
	p.t.Helper()
	a := fn(p.fga, p.fgaStore, p.fgaModel)
	b := fn(p.gb, p.gbStore, p.gbModel)
	if a == b {
		return
	}
	if known != "" {
		p.t.Logf("known difference in %s: openfga %q, gebra %q (%s)", name, a, b, known)
		return
	}
	p.t.Errorf("%s: openfga %q, gebra %q", name, a, b)
}

func writeStatus(req *WriteRequest) func(c *fgaClient, store, model string) string {
	return func(c *fgaClient, store, _ string) string {
		return statusOf(c.write(context.Background(), store, req))
	}
}

const apiModel = `model
  schema 1.1
type user
type group
  relations
    define member: [user, group#member]
type doc
  relations
    define viewer: [user, user:*, group#member]
    define editor: [user with cc]
    define owner: [user with cc, user]
condition cc(x: int) {
  x > 1
}
`

// TestAPIWrites compares Write validation and the duplicate and missing
// tuple options.
func TestAPIWrites(t *testing.T) {
	p := newAPIPair(t, apiModel)
	w := func(name, known string, req *WriteRequest) { p.same(name, known, writeStatus(req)) }
	cond := func(user, rel, obj string, ctx map[string]any) TupleKey {
		return ctk(user, rel, obj, "cc", ctx)
	}

	w("write", "", &WriteRequest{Writes: []TupleKey{tk("user:anne", "viewer", "doc:1"), cond("user:anne", "editor", "doc:1", map[string]any{"x": 2})}})
	w("duplicate", "", &WriteRequest{Writes: []TupleKey{tk("user:anne", "viewer", "doc:1")}})
	w("duplicate ignored", "", &WriteRequest{Writes: []TupleKey{tk("user:anne", "viewer", "doc:1")}, OnDuplicate: "ignore"})
	w("duplicate with other condition context ignored", "OpenFGA answers 409 Conflict, Gebra 400", &WriteRequest{Writes: []TupleKey{cond("user:anne", "editor", "doc:1", map[string]any{"x": 3})}, OnDuplicate: "ignore"})
	w("duplicate with same condition ignored", "", &WriteRequest{Writes: []TupleKey{cond("user:anne", "editor", "doc:1", map[string]any{"x": 2})}, OnDuplicate: "ignore"})
	w("delete missing", "", &WriteRequest{Deletes: []TupleKey{tk("user:bob", "viewer", "doc:1")}})
	w("delete missing ignored", "", &WriteRequest{Deletes: []TupleKey{tk("user:bob", "viewer", "doc:1")}, OnMissing: "ignore"})
	w("same tuple twice in writes", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:2"), tk("user:x", "viewer", "doc:2")}})
	w("same tuple in writes and deletes", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:2")}, Deletes: []TupleKey{tk("user:x", "viewer", "doc:2")}})
	w("type restriction", "", &WriteRequest{Writes: []TupleKey{tk("group:g", "viewer", "doc:2")}})
	w("userset restriction", "", &WriteRequest{Writes: []TupleKey{tk("group:g#member", "editor", "doc:2")}})
	w("wildcard not allowed", "", &WriteRequest{Writes: []TupleKey{tk("user:*", "editor", "doc:2")}})
	w("wildcard allowed", "", &WriteRequest{Writes: []TupleKey{tk("user:*", "viewer", "doc:2")}})
	w("wildcard userset", "", &WriteRequest{Writes: []TupleKey{tk("group:*#member", "viewer", "doc:2")}})
	w("undefined relation", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "nope", "doc:2")}})
	w("undefined object type", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "nope:2")}})
	w("undefined user type", "", &WriteRequest{Writes: []TupleKey{tk("nope:x", "viewer", "doc:2")}})
	w("empty object id", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:")}})
	w("object without type", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc")}})
	w("missing required condition", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "editor", "doc:2")}})
	w("undefined condition", "", &WriteRequest{Writes: []TupleKey{ctk("user:x", "owner", "doc:2", "nope", nil)}})
	w("condition not allowed on relation", "", &WriteRequest{Writes: []TupleKey{ctk("user:x", "viewer", "doc:2", "cc", nil)}})
	w("condition context unknown parameter", "", &WriteRequest{Writes: []TupleKey{cond("user:x", "owner", "doc:2", map[string]any{"y": 1})}})
	w("condition context wrong type", "", &WriteRequest{Writes: []TupleKey{cond("user:x", "owner", "doc:2", map[string]any{"x": "abc"})}})
	w("condition context partial", "", &WriteRequest{Writes: []TupleKey{cond("user:x", "owner", "doc:3", nil)}})
	w("empty write", "", &WriteRequest{})
	w("invalid on_duplicate", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:4")}, OnDuplicate: "skip"})
	w("delete with wrong type", "", &WriteRequest{Deletes: []TupleKey{tk("group:g", "viewer", "doc:1")}})
	w("long object id", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:"+strings.Repeat("a", 300))}})
	w("object id with space", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:a b")}})
	w("object id with hash", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:a#b")}})
	w("unknown store", "", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:1")}})
	p.same("write to unknown store", "OpenFGA answers 400 for an unknown store or model, Gebra 404", func(c *fgaClient, _, _ string) string {
		return statusOf(c.write(p.ctx, "01JZZZZZZZZZZZZZZZZZZZZZZZ", &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:1")}}))
	})
	p.same("write with unknown model", "OpenFGA answers 400 for an unknown store or model, Gebra 404", func(c *fgaClient, store, _ string) string {
		return statusOf(c.write(p.ctx, store, &WriteRequest{Writes: []TupleKey{tk("user:x", "viewer", "doc:9")}, AuthorizationModelID: "01JZZZZZZZZZZZZZZZZZZZZZZZ"}))
	})
}

// readAll pages through Read with a small page size and returns the tuples
// and the number of pages.
func readAll(ctx context.Context, c *fgaClient, store string, filter TupleKey) (string, int) {
	var keys []string
	token := ""
	pages := 0
	for {
		ts, next, err := c.read(ctx, store, filter, 2, token)
		if err != nil {
			return statusOf(err), pages
		}
		pages++
		for _, t := range ts {
			keys = append(keys, t.Key.Object+"#"+t.Key.Relation+"@"+t.Key.User)
		}
		if next == "" || pages > 100 {
			break
		}
		token = next
	}
	slices.Sort(keys)
	return strings.Join(keys, " "), pages
}

// TestAPIReadAndChanges compares Read filters and pagination, and the order
// and type filter of ReadChanges.
func TestAPIReadAndChanges(t *testing.T) {
	p := newAPIPair(t, apiModel)
	tuples := []TupleKey{
		tk("user:anne", "viewer", "doc:1"), tk("user:bob", "viewer", "doc:1"), tk("user:anne", "viewer", "doc:2"),
		tk("group:eng#member", "viewer", "doc:1"), tk("user:*", "viewer", "doc:3"), tk("user:anne", "member", "group:eng"),
		tk("user:anne", "owner", "doc:1"),
	}
	for _, k := range tuples {
		p.same("write "+k.Object, "", writeStatus(&WriteRequest{Writes: []TupleKey{k}}))
	}
	p.same("delete", "", writeStatus(&WriteRequest{Deletes: []TupleKey{tk("user:bob", "viewer", "doc:1")}}))
	p.same("write again", "", writeStatus(&WriteRequest{Writes: []TupleKey{tk("user:bob", "viewer", "doc:1")}}))

	filters := []TupleKey{
		{},
		{Object: "doc:1"},
		{Object: "doc:1", Relation: "viewer"},
		{Object: "doc:1", Relation: "viewer", User: "user:anne"},
		{Object: "doc:", Relation: "viewer", User: "user:anne"},
		{Object: "doc:", User: "user:anne"},
		{Object: "doc:", User: "user:*"},
		{Object: "doc:", User: "group:eng#member"},
		{Object: "doc:"},
		{User: "user:anne"},
		{Relation: "viewer"},
		{Object: "doc:", Relation: "viewer"},
		{Object: "nope:1"},
	}
	for _, f := range filters {
		known := ""
		if f.Object == "doc:" && f.User == "" || f.Object == "" && (f.User != "" || f.Relation != "") {
			known = "OpenFGA needs an object type and an object ID or user in a read filter, Gebra reads any filter"
		}
		p.same(fmt.Sprintf("read %+v", f), known, func(c *fgaClient, store, _ string) string {
			s, _ := readAll(p.ctx, c, store, f)
			return s
		})
	}

	changes := func(typ string, pageSize int) func(c *fgaClient, store, _ string) string {
		return func(c *fgaClient, store, _ string) string {
			var out []string
			token := ""
			for range 50 {
				cs, next, err := c.readChanges(p.ctx, store, typ, pageSize, token)
				if err != nil {
					return statusOf(err)
				}
				if len(cs) == 0 {
					break
				}
				for _, ch := range cs {
					out = append(out, strings.TrimPrefix(ch.Operation, "TUPLE_OPERATION_")+" "+ch.TupleKey.Object+"#"+ch.TupleKey.Relation+"@"+ch.TupleKey.User)
				}
				token = next
			}
			return strings.Join(out, "; ")
		}
	}
	p.same("changes", "", changes("", 3))
	p.same("changes of type doc", "", changes("doc", 2))
	p.same("changes of type group", "", changes("group", 1))
	p.same("changes of unknown type", "", changes("nope", 1))
	p.same("changes poll after end", "", func(c *fgaClient, store, _ string) string {
		_, token, err := c.readChanges(p.ctx, store, "", 100, "")
		if err != nil {
			return statusOf(err)
		}
		cs, next, err := c.readChanges(p.ctx, store, "", 100, token)
		return fmt.Sprintf("%s %d token=%v", statusOf(err), len(cs), next != "")
	})
}

// TestAPIBatchCheckAndAssertions compares BatchCheck correlation and
// per-check errors, and the assertion round trip.
func TestAPIBatchCheckAndAssertions(t *testing.T) {
	p := newAPIPair(t, apiModel)
	p.same("write", "", writeStatus(&WriteRequest{Writes: []TupleKey{
		tk("user:anne", "viewer", "doc:1"), ctk("user:bob", "editor", "doc:1", "cc", nil),
	}}))
	batch := func(items []BatchCheckItem) func(c *fgaClient, store, _ string) string {
		return func(c *fgaClient, store, _ string) string {
			res, err := c.batchCheck(p.ctx, store, items)
			if err != nil {
				return statusOf(err)
			}
			var out []string
			for id, r := range res {
				s := fmt.Sprintf("%s=%v", id, r.Allowed)
				if len(r.Error) > 0 && string(r.Error) != "null" {
					s = id + "=error"
				}
				out = append(out, s)
			}
			slices.Sort(out)
			return strings.Join(out, " ")
		}
	}
	item := func(id, user, rel, obj string, ctx map[string]any) BatchCheckItem {
		return BatchCheckItem{CorrelationID: id, TupleKey: CheckTupleKey{User: user, Relation: rel, Object: obj}, Context: ctx}
	}
	p.same("batch", "", batch([]BatchCheckItem{
		item("a", "user:anne", "viewer", "doc:1", nil),
		item("b", "user:bob", "viewer", "doc:1", nil),
		item("c", "user:bob", "editor", "doc:1", map[string]any{"x": 5}),
		item("d", "user:bob", "editor", "doc:1", map[string]any{"x": 0}),
		item("e", "user:bob", "editor", "doc:1", nil),
		item("f", "user:bob", "nope", "doc:1", nil),
		{CorrelationID: "g", TupleKey: CheckTupleKey{User: "user:carl", Relation: "viewer", Object: "doc:1"}, ContextualTuples: []TupleKey{tk("user:carl", "viewer", "doc:1")}},
	}))
	p.same("batch duplicate correlation", "", batch([]BatchCheckItem{item("a", "user:anne", "viewer", "doc:1", nil), item("a", "user:bob", "viewer", "doc:1", nil)}))
	p.same("batch empty correlation", "", batch([]BatchCheckItem{item("", "user:anne", "viewer", "doc:1", nil)}))
	p.same("batch empty", "OpenFGA rejects an empty batch, Gebra returns no results", batch(nil))
	var many []BatchCheckItem
	for i := range 500 {
		many = append(many, item(fmt.Sprint(i), fmt.Sprintf("user:u%d", i%7), "viewer", "doc:1", nil))
	}
	p.same("batch of 500", "", func(c *fgaClient, store, model string) string {
		s := batch(many)(c, store, model)
		return fmt.Sprintf("%d results, anne=%v", strings.Count(s, "="), strings.Contains(s, "=true"))
	})

	assertions := []Assertion{
		{TupleKey: CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "doc:1"}, Expectation: true},
		{TupleKey: CheckTupleKey{User: "user:carl", Relation: "viewer", Object: "doc:1"}, Expectation: true, ContextualTuples: []TupleKey{tk("user:carl", "viewer", "doc:1")}},
		{TupleKey: CheckTupleKey{User: "user:bob", Relation: "editor", Object: "doc:1"}, Expectation: false, Context: map[string]any{"x": 0}},
	}
	p.same("write assertions", "", func(c *fgaClient, store, model string) string {
		return statusOf(c.writeAssertions(p.ctx, store, model, assertions))
	})
	p.same("read assertions", "", func(c *fgaClient, store, model string) string {
		as, err := c.readAssertions(p.ctx, store, model)
		if err != nil {
			return statusOf(err)
		}
		b, _ := json.Marshal(as)
		return string(b)
	})
	p.same("invalid assertion", "", func(c *fgaClient, store, model string) string {
		return statusOf(c.writeAssertions(p.ctx, store, model, []Assertion{{TupleKey: CheckTupleKey{User: "user:anne", Relation: "nope", Object: "doc:1"}}}))
	})
	p.same("assertions of unknown model", "OpenFGA answers 400 for an unknown store or model, Gebra 404", func(c *fgaClient, store, _ string) string {
		_, err := c.readAssertions(p.ctx, store, "01JZZZZZZZZZZZZZZZZZZZZZZZ")
		return statusOf(err)
	})

	// Error codes of queries.
	q := func(name, known string, path string, body any) {
		p.same(name, known, func(c *fgaClient, store, _ string) string {
			return statusOf(c.do(p.ctx, http.MethodPost, "/stores/"+store+path, body, nil))
		})
	}
	q("check undefined relation", "", "/check", map[string]any{"tuple_key": map[string]any{"user": "user:anne", "relation": "nope", "object": "doc:1"}})
	q("check invalid user", "", "/check", map[string]any{"tuple_key": map[string]any{"user": "anne", "relation": "viewer", "object": "doc:1"}})
	q("check unknown model", "OpenFGA answers 400 for an unknown store or model, Gebra 404", "/check", map[string]any{"tuple_key": map[string]any{"user": "user:anne", "relation": "viewer", "object": "doc:1"}, "authorization_model_id": "01JZZZZZZZZZZZZZZZZZZZZZZZ"})
	q("check missing context", "", "/check", map[string]any{"tuple_key": map[string]any{"user": "user:bob", "relation": "editor", "object": "doc:1"}})
	q("list objects undefined type", "", "/list-objects", map[string]any{"type": "nope", "relation": "viewer", "user": "user:anne"})
	q("list users two filters", "", "/list-users", map[string]any{"object": map[string]any{"type": "doc", "id": "1"}, "relation": "viewer", "user_filters": []any{map[string]any{"type": "user"}, map[string]any{"type": "group"}}})
	q("list users undefined filter type", "", "/list-users", map[string]any{"object": map[string]any{"type": "doc", "id": "1"}, "relation": "viewer", "user_filters": []any{map[string]any{"type": "nope"}}})
	q("expand undefined relation", "", "/expand", map[string]any{"tuple_key": map[string]any{"relation": "nope", "object": "doc:1"}})
	q("invalid consistency", "OpenFGA ignores an unknown consistency value, Gebra rejects it", "/check", map[string]any{"tuple_key": map[string]any{"user": "user:anne", "relation": "viewer", "object": "doc:1"}, "consistency": "SOMETIMES"})
	q("higher consistency", "", "/check", map[string]any{"tuple_key": map[string]any{"user": "user:anne", "relation": "viewer", "object": "doc:1"}, "consistency": "HIGHER_CONSISTENCY"})
	p.same("check unknown store", "OpenFGA answers 400 for an unknown store or model, Gebra 404", func(c *fgaClient, _, _ string) string {
		return statusOf(c.do(p.ctx, http.MethodPost, "/stores/01JZZZZZZZZZZZZZZZZZZZZZZZ/check", map[string]any{"tuple_key": map[string]any{"user": "user:anne", "relation": "viewer", "object": "doc:1"}}, nil))
	})
}
