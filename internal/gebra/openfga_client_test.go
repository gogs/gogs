package gebra

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
)

// fgaClient talks to an OpenFGA server through its HTTP API only. It is the
// black-box oracle of the differential tests.
type fgaClient struct {
	base string
	http *http.Client
}

// openFGAURL returns the OpenFGA server that differential tests compare with,
// or an empty string when none is configured.
func openFGAURL() string {
	return strings.TrimRight(os.Getenv("GEBRA_OPENFGA_URL"), "/")
}

func newFGAClient(t testing.TB) *fgaClient {
	t.Helper()
	u := openFGAURL()
	if u == "" {
		t.Skip("set GEBRA_OPENFGA_URL to an OpenFGA server to compare with it")
	}
	return &fgaClient{base: u, http: &http.Client{Timeout: 10 * time.Minute}}
}

// fgaError is an error response of OpenFGA.
type fgaError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *fgaError) Error() string {
	return fmt.Sprintf("openfga %d %s: %s", e.Status, e.Code, e.Message)
}

func (c *fgaClient) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return errors.Wrap(err, "marshal request")
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return errors.Wrap(err, "new request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.Wrap(err, "send request")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read response")
	}
	if resp.StatusCode >= 300 {
		e := &fgaError{Status: resp.StatusCode}
		if json.Unmarshal(data, e) != nil {
			e.Message = string(data)
		}
		return e
	}
	if out == nil {
		return nil
	}
	return errors.Wrap(json.Unmarshal(data, out), "unmarshal response")
}

func (c *fgaClient) createStore(ctx context.Context, name string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/stores", map[string]any{"name": name}, &out)
	return out.ID, err
}

// openFGAModel returns the model as OpenFGA accepts it: without Gebra's
// attribute extension.
func openFGAModel(m *AuthorizationModel) *AuthorizationModel {
	out := *m
	out.ID = ""
	out.TypeDefinitions = make([]*TypeDefinition, len(m.TypeDefinitions))
	for i, td := range m.TypeDefinitions {
		cp := *td
		if td.Metadata != nil {
			md := *td.Metadata
			md.Attributes = nil
			if md.Relations == nil {
				cp.Metadata = nil
			} else {
				cp.Metadata = &md
			}
		}
		out.TypeDefinitions[i] = &cp
	}
	return &out
}

func (c *fgaClient) writeModel(ctx context.Context, storeID string, m *AuthorizationModel) (string, error) {
	var out struct {
		ID string `json:"authorization_model_id"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/authorization-models", openFGAModel(m), &out)
	return out.ID, err
}

// fgaTupleKeys converts tuples to OpenFGA's JSON shape, which needs a
// condition object only when the tuple has one.
func fgaTupleKeys(ks []TupleKey) []map[string]any {
	out := make([]map[string]any, len(ks))
	for i, k := range ks {
		m := map[string]any{"user": k.User, "relation": k.Relation, "object": k.Object}
		if k.Condition != nil {
			cond := map[string]any{"name": k.Condition.Name}
			if len(k.Condition.Context) > 0 {
				cond["context"] = k.Condition.Context
			}
			m["condition"] = cond
		}
		out[i] = m
	}
	return out
}

func (c *fgaClient) write(ctx context.Context, storeID string, req *WriteRequest) error {
	_, err := c.writeRevision(ctx, storeID, req)
	return err
}

// writeRevision writes and returns the revision that Gebra's server returns,
// which OpenFGA does not.
func (c *fgaClient) writeRevision(ctx context.Context, storeID string, req *WriteRequest) (string, error) {
	body := map[string]any{}
	if req.AuthorizationModelID != "" {
		body["authorization_model_id"] = req.AuthorizationModelID
	}
	if len(req.Writes) > 0 {
		w := map[string]any{"tuple_keys": fgaTupleKeys(req.Writes)}
		if req.OnDuplicate != "" {
			w["on_duplicate"] = req.OnDuplicate
		}
		body["writes"] = w
	}
	if len(req.Deletes) > 0 {
		keys := make([]map[string]any, len(req.Deletes))
		for i, k := range req.Deletes {
			keys[i] = map[string]any{"user": k.User, "relation": k.Relation, "object": k.Object}
		}
		d := map[string]any{"tuple_keys": keys}
		if req.OnMissing != "" {
			d["on_missing"] = req.OnMissing
		}
		body["deletes"] = d
	}
	var out struct {
		Revision string `json:"revision"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/write", body, &out)
	return out.Revision, err
}

// writeAll writes tuples in batches that OpenFGA accepts.
func (c *fgaClient) writeAll(ctx context.Context, storeID string, tuples []TupleKey) error {
	for len(tuples) > 0 {
		n := min(len(tuples), 1000)
		if err := c.write(ctx, storeID, &WriteRequest{Writes: tuples[:n]}); err != nil {
			return err
		}
		tuples = tuples[n:]
	}
	return nil
}

func contextualBody(ks []TupleKey) map[string]any {
	return map[string]any{"tuple_keys": fgaTupleKeys(ks)}
}

func (c *fgaClient) check(ctx context.Context, storeID, modelID string, q *diffQuery) (bool, error) {
	body := map[string]any{
		"tuple_key": map[string]any{"user": q.User, "relation": q.Relation, "object": q.Object},
	}
	if modelID != "" {
		body["authorization_model_id"] = modelID
	}
	if len(q.Contextual) > 0 {
		body["contextual_tuples"] = contextualBody(q.Contextual)
	}
	if q.Context != nil {
		body["context"] = q.Context
	}
	var out struct {
		Allowed bool `json:"allowed"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/check", body, &out)
	return out.Allowed, err
}

type fgaBatchResult struct {
	Allowed bool            `json:"allowed"`
	Error   json.RawMessage `json:"error"`
}

func (c *fgaClient) batchCheck(ctx context.Context, storeID string, items []BatchCheckItem) (map[string]fgaBatchResult, error) {
	checks := make([]map[string]any, len(items))
	for i, it := range items {
		ch := map[string]any{
			"tuple_key":      map[string]any{"user": it.TupleKey.User, "relation": it.TupleKey.Relation, "object": it.TupleKey.Object},
			"correlation_id": it.CorrelationID,
		}
		if len(it.ContextualTuples) > 0 {
			ch["contextual_tuples"] = contextualBody(it.ContextualTuples)
		}
		if it.Context != nil {
			ch["context"] = it.Context
		}
		checks[i] = ch
	}
	var out struct {
		Result map[string]fgaBatchResult `json:"result"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/batch-check", map[string]any{"checks": checks}, &out)
	return out.Result, err
}

func (c *fgaClient) listObjects(ctx context.Context, storeID string, q *diffQuery) ([]string, error) {
	body := map[string]any{"type": q.Type, "relation": q.Relation, "user": q.User}
	if len(q.Contextual) > 0 {
		body["contextual_tuples"] = contextualBody(q.Contextual)
	}
	if q.Context != nil {
		body["context"] = q.Context
	}
	var out struct {
		Objects []string `json:"objects"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/list-objects", body, &out)
	return out.Objects, err
}

// streamedListObjects reads OpenFGA's newline-delimited stream of results.
func (c *fgaClient) streamedListObjects(ctx context.Context, storeID string, q *diffQuery) ([]string, error) {
	body := map[string]any{"type": q.Type, "relation": q.Relation, "user": q.User}
	if len(q.Contextual) > 0 {
		body["contextual_tuples"] = contextualBody(q.Contextual)
	}
	if q.Context != nil {
		body["context"] = q.Context
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/stores/"+storeID+"/streamed-list-objects", bytes.NewReader(b))
	if err != nil {
		return nil, errors.Wrap(err, "new request")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "send request")
	}
	defer func() { _ = resp.Body.Close() }()
	var out []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var line struct {
			Result *struct {
				Object string `json:"object"`
			} `json:"result"`
			Error *fgaError `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			return nil, errors.Wrapf(err, "unmarshal stream line %q", sc.Text())
		}
		if line.Error != nil {
			return out, line.Error
		}
		if line.Result != nil {
			out = append(out, line.Result.Object)
		}
	}
	return out, errors.Wrap(sc.Err(), "read stream")
}

func (c *fgaClient) listUsers(ctx context.Context, storeID string, q *diffQuery) ([]string, error) {
	typ, id, _ := strings.Cut(q.Object, ":")
	filter := map[string]any{"type": q.Filter.Type}
	if q.Filter.Relation != "" {
		filter["relation"] = q.Filter.Relation
	}
	body := map[string]any{
		"object":       map[string]any{"type": typ, "id": id},
		"relation":     q.Relation,
		"user_filters": []any{filter},
	}
	if len(q.Contextual) > 0 {
		body["contextual_tuples"] = fgaTupleKeys(q.Contextual)
	}
	if q.Context != nil {
		body["context"] = q.Context
	}
	var out struct {
		Users []User `json:"users"`
	}
	if err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/list-users", body, &out); err != nil {
		return nil, err
	}
	return userStrings(out.Users), nil
}

func (c *fgaClient) expand(ctx context.Context, storeID string, q *diffQuery) (*ExpandNode, error) {
	body := map[string]any{"tuple_key": map[string]any{"relation": q.Relation, "object": q.Object}}
	if len(q.Contextual) > 0 {
		body["contextual_tuples"] = contextualBody(q.Contextual)
	}
	var out struct {
		Tree struct {
			Root *ExpandNode `json:"root"`
		} `json:"tree"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/expand", body, &out)
	return out.Tree.Root, err
}

type fgaTuple struct {
	Key       TupleKey  `json:"key"`
	Timestamp time.Time `json:"timestamp"`
}

func (c *fgaClient) read(ctx context.Context, storeID string, filter TupleKey, pageSize int, token string) ([]fgaTuple, string, error) {
	body := map[string]any{"page_size": pageSize}
	if filter != (TupleKey{}) {
		body["tuple_key"] = map[string]any{"user": filter.User, "relation": filter.Relation, "object": filter.Object}
	}
	if token != "" {
		body["continuation_token"] = token
	}
	var out struct {
		Tuples []fgaTuple `json:"tuples"`
		Token  string     `json:"continuation_token"`
	}
	err := c.do(ctx, http.MethodPost, "/stores/"+storeID+"/read", body, &out)
	return out.Tuples, out.Token, err
}

func (c *fgaClient) readChanges(ctx context.Context, storeID, typ string, pageSize int, token string) ([]TupleChange, string, error) {
	v := url.Values{}
	if typ != "" {
		v.Set("type", typ)
	}
	v.Set("page_size", fmt.Sprint(pageSize))
	if token != "" {
		v.Set("continuation_token", token)
	}
	var out struct {
		Changes []TupleChange `json:"changes"`
		Token   string        `json:"continuation_token"`
	}
	err := c.do(ctx, http.MethodGet, "/stores/"+storeID+"/changes?"+v.Encode(), nil, &out)
	return out.Changes, out.Token, err
}

func (c *fgaClient) writeAssertions(ctx context.Context, storeID, modelID string, as []Assertion) error {
	list := make([]map[string]any, len(as))
	for i, a := range as {
		m := map[string]any{
			"tuple_key":   map[string]any{"user": a.TupleKey.User, "relation": a.TupleKey.Relation, "object": a.TupleKey.Object},
			"expectation": a.Expectation,
		}
		if len(a.ContextualTuples) > 0 {
			m["contextual_tuples"] = fgaTupleKeys(a.ContextualTuples)
		}
		if a.Context != nil {
			m["context"] = a.Context
		}
		list[i] = m
	}
	return c.do(ctx, http.MethodPut, "/stores/"+storeID+"/assertions/"+modelID, map[string]any{"assertions": list}, nil)
}

func (c *fgaClient) readAssertions(ctx context.Context, storeID, modelID string) ([]Assertion, error) {
	var out struct {
		Assertions []Assertion `json:"assertions"`
	}
	err := c.do(ctx, http.MethodGet, "/stores/"+storeID+"/assertions/"+modelID, nil, &out)
	return out.Assertions, err
}

// userStrings renders ListUsers results as user strings: "type:id",
// "type:id#relation", or "type:*".
func userStrings(users []User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		switch {
		case u.Object != nil:
			out = append(out, u.Object.Type+":"+u.Object.ID)
		case u.Userset != nil:
			out = append(out, u.Userset.Type+":"+u.Userset.ID+"#"+u.Userset.Relation)
		case u.Wildcard != nil:
			out = append(out, u.Wildcard.Type+":*")
		}
	}
	return out
}
