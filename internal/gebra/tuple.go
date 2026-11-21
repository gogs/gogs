package gebra

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// TupleKey is a relationship tuple in OpenFGA's string form, e.g., user
// "group:eng#member", relation "viewer", object "document:roadmap".
type TupleKey struct {
	User      string                 `json:"user"`
	Relation  string                 `json:"relation"`
	Object    string                 `json:"object"`
	Condition *RelationshipCondition `json:"condition,omitempty"`
}

// RelationshipCondition names the condition of a conditional tuple and the
// parameter values stored with it.
type RelationshipCondition struct {
	Name    string         `json:"name"`
	Context map[string]any `json:"context,omitempty"`
}

// Tuple is a stored relationship tuple.
type Tuple struct {
	Key       TupleKey  `json:"key"`
	Timestamp time.Time `json:"timestamp"`
}

type object struct {
	typ, id string
}

func (o object) String() string { return o.typ + ":" + o.id }

// subject is the user side of a tuple or query: an object (rel is empty), a
// wildcard (id is "*"), or a userset (rel is set).
type subject struct {
	typ, id, rel string
}

func (s subject) String() string {
	if s.rel == "" {
		return s.typ + ":" + s.id
	}
	return s.typ + ":" + s.id + "#" + s.rel
}

func validID(s string) bool {
	return s != "" && len(s) <= 191 && !strings.ContainsAny(s, "#* \t\n\r")
}

func parseObject(s string) (object, error) {
	typ, id, ok := strings.Cut(s, ":")
	if !ok || !validName(typ) || !validID(id) {
		return object{}, invalidf("invalid object %q, want type:id", s)
	}
	return object{typ: typ, id: id}, nil
}

func parseSubject(s string) (subject, error) {
	typ, rest, ok := strings.Cut(s, ":")
	if !ok || !validName(typ) {
		return subject{}, invalidf("invalid user %q, want type:id, type:*, or type:id#relation", s)
	}
	id, rel, hasRel := strings.Cut(rest, "#")
	if hasRel && !validName(rel) {
		return subject{}, invalidf("invalid user %q: bad relation", s)
	}
	if id != "*" && !validID(id) || id == "*" && hasRel {
		return subject{}, invalidf("invalid user %q: bad ID", s)
	}
	return subject{typ: typ, id: id, rel: rel}, nil
}

// tuple is a parsed relationship tuple.
type tuple struct {
	objectType, objectID, relation string
	userType, userID, userRelation string
	conditionName                  string
	conditionContext               map[string]any
}

func parseTupleKey(k TupleKey) (*tuple, error) {
	obj, err := parseObject(k.Object)
	if err != nil {
		return nil, err
	}
	sub, err := parseSubject(k.User)
	if err != nil {
		return nil, err
	}
	if !validName(k.Relation) {
		return nil, invalidf("invalid relation %q", k.Relation)
	}
	t := &tuple{
		objectType: obj.typ, objectID: obj.id, relation: k.Relation,
		userType: sub.typ, userID: sub.id, userRelation: sub.rel,
	}
	if k.Condition != nil {
		if !validName(k.Condition.Name) {
			return nil, invalidf("invalid condition name %q", k.Condition.Name)
		}
		t.conditionName = k.Condition.Name
		t.conditionContext = k.Condition.Context
	}
	return t, nil
}

func (t *tuple) object() object { return object{typ: t.objectType, id: t.objectID} }

func (t *tuple) user() subject {
	return subject{typ: t.userType, id: t.userID, rel: t.userRelation}
}

func (t *tuple) userString() string { return t.user().String() }

// keyString identifies the tuple without its condition.
func (t *tuple) keyString() string {
	return t.objectType + ":" + t.objectID + "#" + t.relation + "@" + t.userString()
}

func (t *tuple) key() TupleKey {
	k := TupleKey{User: t.userString(), Relation: t.relation, Object: t.object().String()}
	if t.conditionName != "" {
		k.Condition = &RelationshipCondition{Name: t.conditionName, Context: t.conditionContext}
	}
	return k
}

func (t *tuple) contextJSON() (string, error) {
	if len(t.conditionContext) == 0 {
		return "", nil
	}
	b, err := json.Marshal(t.conditionContext)
	if err != nil {
		return "", errors.Wrap(err, "marshal condition context")
	}
	return string(b), nil
}

func (r *tupleRow) tuple() (*tuple, error) {
	t := &tuple{
		objectType: r.ObjectType, objectID: r.ObjectID, relation: r.Relation,
		userType: r.UserType, userID: r.UserID, userRelation: r.UserRelation,
		conditionName: r.ConditionName,
	}
	if r.ConditionContext != "" {
		if err := json.Unmarshal([]byte(r.ConditionContext), &t.conditionContext); err != nil {
			return nil, errors.Wrap(err, "unmarshal condition context")
		}
	}
	return t, nil
}

// setKey is the view node of a set, "type:id#relation".
func setKey(typ, id, rel string) string { return typ + ":" + id + "#" + rel }

func splitSetKey(k string) (typ, id, rel string) {
	typ, rest, _ := strings.Cut(k, ":")
	i := strings.LastIndexByte(rest, '#')
	return typ, rest[:i], rest[i+1:]
}
