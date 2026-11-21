package gebra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Builders of hand-written scenario queries. An empty expect leaves the
// answer to the differential comparison with OpenFGA.

func qCheck(user, rel, obj, expect string) *diffQuery {
	q := &diffQuery{Kind: kindCheck, User: user, Relation: rel, Object: obj}
	if expect != "" {
		q.Expect = ptr(expect)
	}
	return q
}

func qObjects(typ, rel, user, expect string) *diffQuery {
	q := &diffQuery{Kind: kindListObjects, Type: typ, Relation: rel, User: user}
	if expect != "-" {
		q.Expect = ptr(expect)
	}
	return q
}

func qStreamed(typ, rel, user, expect string) *diffQuery {
	q := qObjects(typ, rel, user, expect)
	q.Kind = kindStreamedObject
	return q
}

func qUsers(obj, rel, filter, expect string) *diffQuery {
	ft, fr, _ := strings.Cut(filter, "#")
	q := &diffQuery{Kind: kindListUsers, Object: obj, Relation: rel, Filter: UserFilter{Type: ft, Relation: fr}}
	if expect != "-" {
		q.Expect = ptr(expect)
	}
	return q
}

func qExpand(obj, rel string) *diffQuery {
	return &diffQuery{Kind: kindExpand, Object: obj, Relation: rel}
}

func (q *diffQuery) with(contextual []TupleKey, ctx map[string]any) *diffQuery {
	q.Contextual = contextual
	q.Context = ctx
	return q
}

func (q *diffQuery) known(reason string) *diffQuery {
	q.Known = reason
	return q
}

func ctk(user, rel, obj, cond string, ctx map[string]any) TupleKey {
	k := tk(user, rel, obj)
	if cond != "" {
		k.Condition = &RelationshipCondition{Name: cond, Context: ctx}
	}
	return k
}

// coverageScenarios are the hand-written scenarios of the coverage matrix.
// Each name starts with the feature it covers.
func coverageScenarios() []*diffScenario {
	conditionModel := `
model
  schema 1.1
type user
type doc
  relations
    define v_int: [user with c_int]
    define v_uint: [user with c_uint]
    define v_double: [user with c_double]
    define v_bool: [user with c_bool]
    define v_string: [user with c_string]
    define v_duration: [user with c_duration]
    define v_timestamp: [user with c_timestamp]
    define v_list: [user with c_list]
    define v_map: [user with c_map]
    define v_ip: [user with c_ip]
    define v_prec: [user with c_prec]
    define plain: [user]
    define u_missing: v_int or plain
    define i_missing: v_int and plain
    define x_missing: plain but not v_int
    define x_missing_base: v_int but not plain
condition c_int(x: int) {
  x > 10
}
condition c_uint(x: uint) {
  x > 10u
}
condition c_double(x: double) {
  x > 1.5
}
condition c_bool(x: bool) {
  x
}
condition c_string(x: string) {
  x == "ok"
}
condition c_duration(x: duration) {
  x > duration("1h")
}
condition c_timestamp(x: timestamp, now: timestamp) {
  now < x
}
condition c_list(xs: list<string>, x: string) {
  x in xs
}
condition c_map(m: map<int>, k: string) {
  k in m && m[k] > 1
}
condition c_ip(ip: ipaddress, cidr: string) {
  ip.in_cidr(cidr)
}
condition c_prec(x: int) {
  x == 1
}
`
	condTuples := []TupleKey{
		ctk("user:a", "v_int", "doc:1", "c_int", nil),
		ctk("user:a", "v_uint", "doc:1", "c_uint", nil),
		ctk("user:a", "v_double", "doc:1", "c_double", nil),
		ctk("user:a", "v_bool", "doc:1", "c_bool", nil),
		ctk("user:a", "v_string", "doc:1", "c_string", nil),
		ctk("user:a", "v_duration", "doc:1", "c_duration", nil),
		ctk("user:a", "v_timestamp", "doc:1", "c_timestamp", map[string]any{"x": "2026-06-01T00:00:00Z"}),
		ctk("user:a", "v_list", "doc:1", "c_list", map[string]any{"xs": []any{"red", "green"}}),
		ctk("user:a", "v_map", "doc:1", "c_map", nil),
		ctk("user:a", "v_ip", "doc:1", "c_ip", map[string]any{"cidr": "10.0.0.0/8"}),
		ctk("user:a", "v_prec", "doc:1", "c_prec", map[string]any{"x": 1}),
		tk("user:a", "plain", "doc:1"),
	}
	cc := func(rel string, ctx map[string]any, expect string) *diffQuery {
		return qCheck("user:a", rel, "doc:1", expect).with(nil, ctx)
	}

	return []*diffScenario{
		{
			Name:  "zanzibar/this",
			Model: "model\n  schema 1.1\ntype user\ntype doc\n  relations\n    define viewer: [user]\n",
			Tuples: []TupleKey{
				tk("user:anne", "viewer", "doc:1"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true"),
				qCheck("user:bob", "viewer", "doc:1", "false"),
				qCheck("user:anne", "viewer", "doc:2", "false"),
				qObjects("doc", "viewer", "user:anne", "doc:1"),
				qUsers("doc:1", "viewer", "user", "user:anne"),
				qExpand("doc:1", "viewer"),
			},
		},
		{
			Name: "zanzibar/computed-userset",
			Model: `model
  schema 1.1
type user
type doc
  relations
    define owner: [user]
    define editor: [user] or owner
    define viewer: [user] or editor
`,
			Tuples: []TupleKey{tk("user:anne", "owner", "doc:1"), tk("user:bob", "editor", "doc:1")},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true"),
				qCheck("user:bob", "viewer", "doc:1", "true"),
				qCheck("user:bob", "owner", "doc:1", "false"),
				qUsers("doc:1", "viewer", "user", "user:anne,user:bob"),
				qObjects("doc", "viewer", "user:anne", "doc:1"),
				qExpand("doc:1", "viewer"),
			},
		},
		{
			Name: "zanzibar/tuple-to-userset",
			Model: `model
  schema 1.1
type user
type folder
  relations
    define parent: [folder]
    define viewer: [user] or viewer from parent
type doc
  relations
    define parent: [folder]
    define viewer: [user] or viewer from parent
`,
			Tuples: []TupleKey{
				tk("user:anne", "viewer", "folder:root"),
				tk("folder:root", "parent", "folder:a"),
				tk("folder:a", "parent", "folder:b"),
				tk("folder:b", "parent", "doc:1"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true"),
				qCheck("user:anne", "viewer", "folder:b", "true"),
				qCheck("user:bob", "viewer", "doc:1", "false"),
				qObjects("doc", "viewer", "user:anne", "doc:1"),
				qObjects("folder", "viewer", "user:anne", "folder:a,folder:b,folder:root"),
				qUsers("doc:1", "viewer", "user", "user:anne"),
				qExpand("doc:1", "viewer"),
			},
		},
		{
			Name: "zanzibar/union-intersection-exclusion",
			Model: `model
  schema 1.1
type user
type doc
  relations
    define a: [user]
    define b: [user]
    define c: [user]
    define union: a or b or c
    define inter: a and b and c
    define excl: a but not b
    define nested: (a or b) and (c but not a)
`,
			Tuples: []TupleKey{
				tk("user:1", "a", "doc:1"), tk("user:1", "b", "doc:1"), tk("user:1", "c", "doc:1"),
				tk("user:2", "a", "doc:1"),
				tk("user:3", "b", "doc:1"), tk("user:3", "c", "doc:1"),
			},
			Queries: []*diffQuery{
				qCheck("user:1", "union", "doc:1", "true"),
				qCheck("user:3", "union", "doc:1", "true"),
				qCheck("user:4", "union", "doc:1", "false"),
				qCheck("user:1", "inter", "doc:1", "true"),
				qCheck("user:2", "inter", "doc:1", "false"),
				qCheck("user:1", "excl", "doc:1", "false"),
				qCheck("user:2", "excl", "doc:1", "true"),
				qCheck("user:3", "excl", "doc:1", "false"),
				qCheck("user:3", "nested", "doc:1", "true"),
				qCheck("user:1", "nested", "doc:1", "false"),
				qUsers("doc:1", "inter", "user", "user:1"),
				qUsers("doc:1", "excl", "user", "user:2"),
				qUsers("doc:1", "nested", "user", "user:3"),
				qObjects("doc", "excl", "user:2", "doc:1"),
				qObjects("doc", "nested", "user:3", "doc:1"),
				qExpand("doc:1", "nested"),
				qExpand("doc:1", "excl"),
			},
		},
		{
			Name: "zanzibar/userset-subjects-nested-cyclic-groups",
			Model: `model
  schema 1.1
type user
type group
  relations
    define member: [user, group#member]
type doc
  relations
    define viewer: [user, group#member]
`,
			Tuples: []TupleKey{
				tk("user:anne", "member", "group:g1"),
				tk("group:g1#member", "member", "group:g2"),
				tk("group:g2#member", "member", "group:g3"),
				tk("group:g3#member", "member", "group:g1"),
				tk("user:bob", "member", "group:g3"),
				tk("group:g2#member", "viewer", "doc:1"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "member", "group:g3", "true"),
				qCheck("user:bob", "member", "group:g1", "true"),
				qCheck("user:carl", "member", "group:g1", "false"),
				qCheck("user:bob", "viewer", "doc:1", "true"),
				qCheck("group:g1#member", "viewer", "doc:1", "true"),
				qCheck("group:g3#member", "member", "group:g2", "true"),
				qCheck("group:g1#member", "member", "group:g1", "true"),
				qObjects("group", "member", "user:anne", "group:g1,group:g2,group:g3"),
				qObjects("doc", "viewer", "group:g1#member", "doc:1"),
				qUsers("group:g1", "member", "user", "user:anne,user:bob"),
				qUsers("doc:1", "viewer", "group#member", "-"),
				qUsers("group:g1", "member", "group#member", "-"),
				qExpand("group:g1", "member"),
			},
		},
		{
			Name: "openfga/type-restrictions-and-userset-restrictions",
			Model: `model
  schema 1.1
type user
type team
  relations
    define member: [user]
    define admin: [user]
type doc
  relations
    define viewer: [user, team#member]
    define editor: [team#admin]
`,
			Tuples: []TupleKey{
				tk("user:anne", "member", "team:t"),
				tk("user:bob", "admin", "team:t"),
				tk("team:t#member", "viewer", "doc:1"),
				tk("team:t#admin", "editor", "doc:1"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true"),
				qCheck("user:bob", "viewer", "doc:1", "false"),
				qCheck("user:bob", "editor", "doc:1", "true"),
				qCheck("team:t#member", "viewer", "doc:1", "true"),
				qCheck("team:t#admin", "viewer", "doc:1", "false"),
				qCheck("user:anne", "viewer", "doc:1", "true").with([]TupleKey{tk("team:t#member", "viewer", "doc:1")}, nil),
				qCheck("user:anne", "viewer", "doc:1", "error").with([]TupleKey{tk("team:t#admin", "viewer", "doc:1")}, nil),
				qCheck("user:anne", "nope", "doc:1", "error"),
				qCheck("nope:anne", "viewer", "doc:1", "error"),
				qCheck("user:anne", "viewer", "nope:1", "error"),
				qUsers("doc:1", "viewer", "team#member", "team:t#member"),
				qUsers("doc:1", "editor", "user", "user:bob"),
			},
		},
		{
			Name: "openfga/wildcards",
			Model: `model
  schema 1.1
type user
type employee
type group
  relations
    define member: [user, user:*]
type doc
  relations
    define viewer: [user, user:*, group#member]
    define blocked: [user, user:*]
    define can_view: viewer but not blocked
    define public: [user:*, employee:*]
`,
			Tuples: []TupleKey{
				tk("user:*", "viewer", "doc:1"),
				tk("user:anne", "blocked", "doc:1"),
				tk("user:anne", "viewer", "doc:2"),
				tk("user:*", "blocked", "doc:2"),
				tk("user:*", "member", "group:everyone"),
				tk("group:everyone#member", "viewer", "doc:3"),
				tk("user:bob", "viewer", "doc:1"),
				tk("user:*", "public", "doc:4"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true"),
				qCheck("user:anne", "can_view", "doc:1", "false"),
				qCheck("user:carl", "can_view", "doc:1", "true"),
				qCheck("user:*", "viewer", "doc:1", "true"),
				qCheck("user:*", "can_view", "doc:1", "true"),
				qCheck("user:*", "viewer", "doc:2", "false"),
				qCheck("user:anne", "can_view", "doc:2", "false"),
				qCheck("user:zed", "viewer", "doc:3", "true"),
				qCheck("user:*", "viewer", "doc:3", "true"),
				qCheck("employee:e", "public", "doc:4", "false"),
				qCheck("user:x", "public", "doc:4", "true"),
				qObjects("doc", "viewer", "user:zed", "doc:1,doc:3"),
				qObjects("doc", "can_view", "user:anne", "doc:3"),
				qObjects("doc", "can_view", "user:zed", "doc:1,doc:3"),
				qObjects("doc", "viewer", "user:*", "doc:1,doc:3"),
				qUsers("doc:1", "viewer", "user", "user:*,user:bob"),
				qUsers("doc:1", "can_view", "user", "-"),
				qUsers("doc:2", "can_view", "user", ""),
				qUsers("doc:3", "viewer", "user", "user:*"),
				qUsers("doc:4", "public", "employee", ""),
				qExpand("doc:1", "can_view"),
			},
		},
		{
			Name:   "openfga/conditions-cel-types",
			Model:  conditionModel,
			Tuples: condTuples,
			Queries: []*diffQuery{
				cc("v_int", map[string]any{"x": 11}, "true"),
				cc("v_int", map[string]any{"x": 10}, "false"),
				cc("v_int", map[string]any{"x": 11.5}, "error"),
				cc("v_int", map[string]any{"x": "11"}, ""),
				cc("v_uint", map[string]any{"x": 11}, "true"),
				cc("v_uint", map[string]any{"x": -1}, "error"),
				cc("v_double", map[string]any{"x": 1.6}, "true"),
				cc("v_double", map[string]any{"x": 1}, "false"),
				cc("v_bool", map[string]any{"x": true}, "true"),
				cc("v_bool", map[string]any{"x": false}, "false"),
				cc("v_bool", map[string]any{"x": "true"}, "true").known("Gebra accepts \"true\" and \"false\" strings for bool parameters, OpenFGA rejects them"),
				cc("v_bool", map[string]any{"x": 1}, "error"),
				cc("v_string", map[string]any{"x": "ok"}, "true"),
				cc("v_string", map[string]any{"x": "no"}, "false"),
				cc("v_string", map[string]any{"x": 5}, "error"),
				cc("v_duration", map[string]any{"x": "2h"}, "true"),
				cc("v_duration", map[string]any{"x": "30m"}, "false"),
				cc("v_duration", map[string]any{"x": "1h30m"}, "true"),
				cc("v_duration", map[string]any{"x": "soon"}, "error"),
				cc("v_timestamp", map[string]any{"now": "2026-01-01T00:00:00Z"}, "true"),
				cc("v_timestamp", map[string]any{"now": "2026-07-01T00:00:00Z"}, "false"),
				cc("v_timestamp", map[string]any{"now": "2026-01-01T00:00:00.123+02:00"}, "true"),
				cc("v_timestamp", map[string]any{"now": "yesterday"}, "error"),
				cc("v_list", map[string]any{"x": "red"}, "true"),
				cc("v_list", map[string]any{"x": "blue"}, "false"),
				cc("v_list", map[string]any{"x": "red", "xs": []any{"blue"}}, "true"),
				cc("v_map", map[string]any{"m": map[string]any{"a": 2}, "k": "a"}, "true"),
				cc("v_map", map[string]any{"m": map[string]any{"a": 1}, "k": "a"}, "false"),
				cc("v_map", map[string]any{"m": map[string]any{"a": 2}, "k": "b"}, "false"),
				cc("v_ip", map[string]any{"ip": "10.1.2.3"}, "true"),
				cc("v_ip", map[string]any{"ip": "192.168.0.1"}, "false"),
				cc("v_ip", map[string]any{"ip": "not-an-ip"}, "error"),
				cc("v_ip", map[string]any{"ip": "10.1.2.3", "cidr": "192.168.0.0/16"}, "true"),
				cc("v_prec", map[string]any{"x": 2}, "true"),
				cc("v_int", nil, "error"),
				cc("v_int", map[string]any{"y": 1}, "error"),
				cc("u_missing", nil, "true"),
				cc("i_missing", nil, "error"),
				cc("x_missing", nil, "error"),
				cc("x_missing_base", nil, "false"),
				qObjects("doc", "v_int", "user:a", "doc:1").with(nil, map[string]any{"x": 11}),
				qObjects("doc", "v_int", "user:a", "").with(nil, map[string]any{"x": 1}),
				qObjects("doc", "v_int", "user:a", "error"),
				qObjects("doc", "u_missing", "user:a", "doc:1").known("OpenFGA ListObjects fails on the missing parameter although the unconditional branch grants, and its Check returns true"),
				qUsers("doc:1", "v_int", "user", "user:a").with(nil, map[string]any{"x": 11}),
				qUsers("doc:1", "v_int", "user", "").with(nil, map[string]any{"x": 1}),
				qUsers("doc:1", "v_int", "user", "error"),
				qUsers("doc:1", "v_ip", "user", "user:a").with(nil, map[string]any{"ip": "10.0.0.1"}),
				qExpand("doc:1", "v_int"),
			},
		},
		{
			Name: "openfga/conditions-on-ttu-and-usersets",
			Model: `model
  schema 1.1
type user
type group
  relations
    define member: [user, user with flag]
type folder
  relations
    define viewer: [user, group#member with flag]
type doc
  relations
    define parent: [folder, folder with flag]
    define viewer: [user] or viewer from parent
condition flag(on: bool) {
  on
}
`,
			Tuples: []TupleKey{
				ctk("folder:f", "parent", "doc:1", "flag", nil),
				tk("folder:g", "parent", "doc:2"),
				tk("user:anne", "viewer", "folder:f"),
				ctk("group:x#member", "viewer", "folder:g", "flag", map[string]any{"on": true}),
				ctk("user:bob", "member", "group:x", "flag", nil),
				tk("user:carl", "member", "group:x"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true").with(nil, map[string]any{"on": true}),
				qCheck("user:anne", "viewer", "doc:1", "false").with(nil, map[string]any{"on": false}),
				qCheck("user:anne", "viewer", "doc:1", "error"),
				qCheck("user:carl", "viewer", "doc:2", "true"),
				qCheck("user:carl", "viewer", "doc:1", "false").known("Gebra decides false because folder:f grants carl nothing whatever the condition on the parent tuple says, OpenFGA reports the missing parameter"),
				qCheck("user:bob", "viewer", "doc:2", "true").with(nil, map[string]any{"on": true}),
				qCheck("user:bob", "viewer", "doc:2", "false").with(nil, map[string]any{"on": false}),
				qCheck("user:bob", "viewer", "doc:2", "error"),
				qObjects("doc", "viewer", "user:anne", "doc:1").with(nil, map[string]any{"on": true}),
				qObjects("doc", "viewer", "user:carl", "doc:2"),
				qUsers("doc:2", "viewer", "user", "user:bob,user:carl").with(nil, map[string]any{"on": true}),
				qUsers("doc:2", "viewer", "user", "user:carl").with(nil, map[string]any{"on": false}),
			},
		},
		{
			Name: "openfga/contextual-tuples",
			Model: `model
  schema 1.1
type user
type group
  relations
    define member: [user]
type doc
  relations
    define viewer: [user, user with flag, group#member]
    define blocked: [user]
    define can_view: viewer but not blocked
condition flag(on: bool) {
  on
}
`,
			Tuples: []TupleKey{
				tk("group:eng#member", "viewer", "doc:1"),
				ctk("user:dan", "viewer", "doc:2", "flag", map[string]any{"on": false}),
				tk("user:erin", "viewer", "doc:3"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "viewer", "doc:1", "true").with([]TupleKey{tk("user:anne", "member", "group:eng")}, nil),
				qCheck("user:anne", "viewer", "doc:1", "false"),
				qCheck("user:anne", "can_view", "doc:1", "false").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:anne", "blocked", "doc:1")}, nil),
				qCheck("user:anne", "viewer", "doc:9", "true").with([]TupleKey{ctk("user:anne", "viewer", "doc:9", "flag", nil)}, map[string]any{"on": true}),
				qCheck("user:anne", "viewer", "doc:9", "false").with([]TupleKey{ctk("user:anne", "viewer", "doc:9", "flag", map[string]any{"on": false})}, map[string]any{"on": true}),
				// A contextual tuple with the key of a stored conditional tuple.
				qCheck("user:dan", "viewer", "doc:2", "").with([]TupleKey{tk("user:dan", "viewer", "doc:2")}, nil),
				qCheck("user:erin", "viewer", "doc:3", "").with([]TupleKey{ctk("user:erin", "viewer", "doc:3", "flag", map[string]any{"on": false})}, nil),
				qCheck("user:anne", "viewer", "doc:1", "error").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:anne", "member", "group:eng")}, nil).known("Gebra rejects duplicate contextual tuples, OpenFGA accepts them"),
				qCheck("user:anne", "viewer", "doc:1", "error").with([]TupleKey{tk("user:anne", "viewer", "group:eng")}, nil),
				qObjects("doc", "viewer", "user:anne", "doc:1,doc:7").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:anne", "viewer", "doc:7")}, nil),
				qObjects("doc", "can_view", "user:anne", "doc:7").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:anne", "viewer", "doc:7"), tk("user:anne", "blocked", "doc:1")}, nil),
				qStreamed("doc", "viewer", "user:anne", "doc:1,doc:7").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:anne", "viewer", "doc:7")}, nil),
				qUsers("doc:1", "viewer", "user", "user:anne,user:bob").with([]TupleKey{tk("user:anne", "member", "group:eng"), tk("user:bob", "member", "group:eng")}, nil),
				qUsers("doc:3", "viewer", "user", "").with([]TupleKey{ctk("user:erin", "viewer", "doc:3", "flag", map[string]any{"on": false})}, nil).known("OpenFGA ListUsers keeps the stored tuple that a contextual tuple with the same key replaces, so it lists erin although its Check says false"),
				qExpand("doc:1", "viewer").with([]TupleKey{tk("user:anne", "viewer", "doc:1")}, nil),
			},
		},
		{
			Name: "openfga/list-users-filters",
			Model: `model
  schema 1.1
type user
type group
  relations
    define member: [user, group#member]
    define owner: [user]
type doc
  relations
    define parent: [doc]
    define viewer: [user, group#member, group#owner] or viewer from parent
`,
			Tuples: []TupleKey{
				tk("user:anne", "member", "group:a"),
				tk("group:a#member", "member", "group:b"),
				tk("user:bob", "owner", "group:a"),
				tk("group:b#member", "viewer", "doc:1"),
				tk("group:a#owner", "viewer", "doc:1"),
				tk("doc:1", "parent", "doc:2"),
				tk("user:carl", "viewer", "doc:2"),
			},
			Queries: []*diffQuery{
				qUsers("doc:1", "viewer", "user", "user:anne,user:bob"),
				qUsers("doc:2", "viewer", "user", "user:anne,user:bob,user:carl"),
				qUsers("doc:1", "viewer", "group#member", "-"),
				qUsers("doc:1", "viewer", "group#owner", "group:a#owner"),
				qUsers("doc:2", "viewer", "doc", "-"),
				qUsers("doc:2", "viewer", "group", "-"),
				qUsers("group:b", "member", "group#member", "-"),
				qObjects("doc", "viewer", "user:anne", "doc:1,doc:2"),
				qObjects("doc", "viewer", "group:a#member", "doc:1,doc:2"),
				qObjects("doc", "viewer", "group:a#owner", "doc:1,doc:2"),
				qStreamed("doc", "viewer", "user:bob", "doc:1,doc:2"),
			},
		},
		{
			Name: "openfga/exclusion-and-intersection-in-cycles",
			Model: `model
  schema 1.1
type user
type group
  relations
    define banned: [user]
    define member: [user, group#allowed]
    define allowed: member but not banned
    define both: [user, group#both] and member
`,
			Tuples: []TupleKey{
				tk("user:anne", "member", "group:a"),
				tk("group:a#allowed", "member", "group:b"),
				tk("group:b#allowed", "member", "group:a"),
				tk("user:bob", "member", "group:b"),
				tk("user:bob", "banned", "group:a"),
				tk("user:anne", "both", "group:a"),
				tk("group:a#both", "both", "group:b"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "allowed", "group:b", ""),
				qCheck("user:bob", "allowed", "group:a", ""),
				qCheck("user:bob", "member", "group:a", ""),
				qCheck("user:anne", "both", "group:b", ""),
				qCheck("user:carl", "allowed", "group:a", "false"),
				qObjects("group", "allowed", "user:anne", "-"),
				qObjects("group", "allowed", "user:bob", "-"),
				qUsers("group:a", "allowed", "user", "-"),
				qUsers("group:b", "both", "user", "-"),
			},
		},
		{
			Name: "openfga/ttu-to-multiple-types",
			Model: `model
  schema 1.1
type user
type org
  relations
    define admin: [user]
type team
  relations
    define admin: [user]
    define lead: [user]
type repo
  relations
    define owner: [org, team]
    define admin: admin from owner
    define lead: lead from owner
`,
			Tuples: []TupleKey{
				tk("user:anne", "admin", "org:o"),
				tk("user:bob", "admin", "team:t"),
				tk("user:carl", "lead", "team:t"),
				tk("org:o", "owner", "repo:1"),
				tk("team:t", "owner", "repo:2"),
			},
			Queries: []*diffQuery{
				qCheck("user:anne", "admin", "repo:1", "true"),
				qCheck("user:bob", "admin", "repo:2", "true"),
				qCheck("user:carl", "lead", "repo:2", "true"),
				qCheck("user:carl", "lead", "repo:1", "false"),
				qObjects("repo", "admin", "user:bob", "repo:2"),
				qUsers("repo:2", "admin", "user", "user:bob"),
				qExpand("repo:1", "admin"),
			},
		},
		{
			Name: "openfga/list-users-wildcard-algebra",
			Note: "OpenFGA lists a user named in one operand of an intersection when a wildcard satisfies the other, and lists the wildcard when an exclusion removes named users from it.",
			Model: `model
  schema 1.1
type user
type doc
  relations
    define a: [user, user:*]
    define b: [user, user:*]
    define i: a and b
    define x: a but not b
    define u: a or b
`,
			Tuples: []TupleKey{
				tk("user:*", "a", "doc:1"), tk("user:anne", "b", "doc:1"),
				tk("user:bob", "a", "doc:2"), tk("user:*", "b", "doc:2"),
				tk("user:*", "a", "doc:3"), tk("user:anne", "a", "doc:3"), tk("user:anne", "b", "doc:3"),
				tk("user:*", "a", "doc:4"), tk("user:*", "b", "doc:4"),
			},
			Queries: []*diffQuery{
				qUsers("doc:1", "i", "user", "user:anne"),
				qUsers("doc:1", "x", "user", "user:*"),
				qUsers("doc:1", "u", "user", "user:*,user:anne"),
				qUsers("doc:2", "i", "user", "user:bob"),
				qUsers("doc:2", "x", "user", ""),
				qUsers("doc:2", "u", "user", "user:*,user:bob"),
				qUsers("doc:3", "i", "user", "user:anne"),
				qUsers("doc:3", "x", "user", "user:*"),
				qUsers("doc:4", "i", "user", "user:*"),
				qUsers("doc:4", "x", "user", ""),
				qUsers("doc:1", "i", "user", "user:anne").with([]TupleKey{tk("user:carl", "a", "doc:9")}, nil),
				qUsers("doc:3", "x", "user", "user:*").with([]TupleKey{tk("user:carl", "a", "doc:9")}, nil),
				qCheck("user:*", "x", "doc:3", "true"),
				qObjects("doc", "i", "user:*", "doc:4"),
				qObjects("doc", "x", "user:*", "doc:1,doc:3"),
				qObjects("doc", "i", "user:zed", "doc:4"),
				qObjects("doc", "x", "user:zed", "doc:1,doc:3"),
			},
		},
		{
			Name: "zanzibar/expand-trees",
			Model: `model
  schema 1.1
type user
type group
  relations
    define member: [user, group#member]
type folder
  relations
    define viewer: [user, group#member]
type doc
  relations
    define parent: [folder]
    define owner: [user]
    define blocked: [user]
    define editor: [user] or owner
    define viewer: ([user, user:*] or editor or viewer from parent) but not blocked
    define both: editor and viewer
`,
			Tuples: []TupleKey{
				tk("user:anne", "owner", "doc:1"),
				tk("user:bob", "editor", "doc:1"),
				tk("user:*", "viewer", "doc:1"),
				tk("folder:f", "parent", "doc:1"),
				tk("folder:g", "parent", "doc:1"),
				tk("group:eng#member", "viewer", "folder:f"),
				tk("user:carl", "blocked", "doc:1"),
			},
			Queries: []*diffQuery{
				qExpand("doc:1", "viewer"),
				qExpand("doc:1", "editor"),
				qExpand("doc:1", "both"),
				qExpand("folder:f", "viewer"),
				qExpand("doc:2", "viewer"),
			},
		},
	}
}

// TestCoverageMatrix runs the hand-written scenarios on Gebra in every mode,
// and on OpenFGA when GEBRA_OPENFGA_URL is set, and checks every answer
// against the expected one or, when none is given, against OpenFGA.
func TestCoverageMatrix(t *testing.T) {
	ctx := context.Background()
	targets := diffTargets(t)
	report := map[string][]string{}
	for _, sc := range coverageScenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			res, err := runDiff(ctx, sc, targets)
			require.NoError(t, err)
			for name, err := range res.setupErrs {
				t.Errorf("%s rejected the scenario: %v", name, err)
			}
			for _, m := range res.mismatches {
				q, known := "setup", ""
				if m.Query >= 0 {
					q, known = sc.Queries[m.Query].String(), sc.Queries[m.Query].Known
				}
				line := fmt.Sprintf("%s: %s: got %s, want %s", m.Target, q, m.Got, m.Want)
				if known != "" {
					t.Logf("known difference: %s (%s)", line, known)
					continue
				}
				report[sc.Name] = append(report[sc.Name], line)
				t.Error(line)
			}
			// Record OpenFGA's answers to queries without an expectation.
			if outs, ok := res.outcomes["openfga"]; ok {
				for i, q := range sc.Queries {
					if q.Expect == nil && q.Kind != kindExpand {
						t.Logf("openfga: %s = %s", q, outs[i])
					}
				}
			}
		})
	}
	if out := os.Getenv("GEBRA_MATRIX_OUT"); out != "" {
		b, err := json.MarshalIndent(report, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(out, b, 0o644))
	}
	assert.NotEmpty(t, targets)
}
