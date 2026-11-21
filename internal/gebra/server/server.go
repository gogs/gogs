// Package server exposes a Gebra engine over HTTP with OpenFGA's REST API.
// List endpoints also accept page_size, continuation_token, and sort, and
// POST /stores/{store_id}/attributes sets the object attributes they sort by.
// Writes return a revision, which queries accept as min_revision next to
// OpenFGA's consistency, and accept "wait": true to return only once the
// store's applier applied them. GET /stores/{store_id}/view-status reports
// how far the materialized view lags behind writes, and, while a new model's
// view is being built, the build's progress as "migration".
//
// A new model is live, i.e., used by queries without authorization_model_id,
// once its view is built. Until then such queries use the previous model, and
// queries that name the new model evaluate the graph. Reading the latest
// authorization model returns the newest model written, as in OpenFGA, and
// writes validate tuples against it.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/gebra"
)

// maxBodyBytes caps request bodies. It is generous because Gebra has no limit
// on tuples per write or checks per batch.
const maxBodyBytes = 64 << 20

type Server struct {
	logger *logx.Logger
	engine *gebra.Engine
}

// New returns the HTTP handler for the engine.
func New(logger *logx.Logger, engine *gebra.Engine) http.Handler {
	s := &Server{logger: logger, engine: engine}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "SERVING"})
	})
	mux.HandleFunc("POST /stores", s.createStore)
	mux.HandleFunc("GET /stores", s.listStores)
	mux.HandleFunc("GET /stores/{store_id}", s.getStore)
	mux.HandleFunc("DELETE /stores/{store_id}", s.deleteStore)
	mux.HandleFunc("POST /stores/{store_id}/authorization-models", s.writeModel)
	mux.HandleFunc("GET /stores/{store_id}/authorization-models", s.readModels)
	mux.HandleFunc("GET /stores/{store_id}/authorization-models/{model_id}", s.readModel)
	mux.HandleFunc("PUT /stores/{store_id}/assertions/{model_id}", s.writeAssertions)
	mux.HandleFunc("GET /stores/{store_id}/assertions/{model_id}", s.readAssertions)
	mux.HandleFunc("POST /stores/{store_id}/write", s.write)
	mux.HandleFunc("POST /stores/{store_id}/read", s.read)
	mux.HandleFunc("GET /stores/{store_id}/changes", s.readChanges)
	mux.HandleFunc("POST /stores/{store_id}/attributes", s.writeAttributes)
	mux.HandleFunc("POST /stores/{store_id}/check", s.check)
	mux.HandleFunc("POST /stores/{store_id}/batch-check", s.batchCheck)
	mux.HandleFunc("POST /stores/{store_id}/expand", s.expand)
	mux.HandleFunc("POST /stores/{store_id}/list-objects", s.listObjects)
	mux.HandleFunc("POST /stores/{store_id}/streamed-list-objects", s.streamedListObjects)
	mux.HandleFunc("POST /stores/{store_id}/list-users", s.listUsers)
	mux.HandleFunc("GET /stores/{store_id}/view-status", s.viewStatus)
	return mux
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeClientError writes the response for errors caused by the request and
// reports whether it did. Callers handle every other error as a 5xx.
func writeClientError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, gebra.ErrChangesExpired):
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "changes_expired", Message: err.Error()})
	case errors.Is(err, gebra.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "validation_error", Message: err.Error()})
	case errors.Is(err, gebra.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Code: "not_found", Message: err.Error()})
	default:
		return false
	}
	return true
}

func writeInternalError(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, errorBody{Code: "internal_error", Message: "Internal server error."})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "validation_error", Message: "Invalid request body: " + err.Error()})
		return false
	}
	return true
}

func pageSize(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("page_size")
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "validation_error", Message: "page_size must be a non-negative integer."})
		return 0, false
	}
	return n, true
}

func (s *Server) createStore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	st, err := s.engine.CreateStore(r.Context(), req.Name)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to create store", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) listStores(w http.ResponseWriter, r *http.Request) {
	size, ok := pageSize(w, r)
	if !ok {
		return
	}
	stores, next, err := s.engine.ListStores(r.Context(), r.URL.Query().Get("name"), size, r.URL.Query().Get("continuation_token"))
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to list stores", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": stores, "continuation_token": next})
}

func (s *Server) getStore(w http.ResponseWriter, r *http.Request) {
	st, err := s.engine.GetStore(r.Context(), r.PathValue("store_id"))
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to get store", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) deleteStore(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.DeleteStore(r.Context(), r.PathValue("store_id")); err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to delete store", "error", err)
			writeInternalError(w)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeModel(w http.ResponseWriter, r *http.Request) {
	var model gebra.AuthorizationModel
	if !decode(w, r, &model) {
		return
	}
	id, err := s.engine.WriteAuthorizationModel(r.Context(), r.PathValue("store_id"), &model)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to write authorization model", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"authorization_model_id": id})
}

func (s *Server) readModels(w http.ResponseWriter, r *http.Request) {
	size, ok := pageSize(w, r)
	if !ok {
		return
	}
	models, next, err := s.engine.ReadAuthorizationModels(r.Context(), r.PathValue("store_id"), size, r.URL.Query().Get("continuation_token"))
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read authorization models", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authorization_models": models, "continuation_token": next})
}

func (s *Server) readModel(w http.ResponseWriter, r *http.Request) {
	model, err := s.engine.ReadAuthorizationModel(r.Context(), r.PathValue("store_id"), r.PathValue("model_id"))
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read authorization model", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authorization_model": model})
}

func (s *Server) writeAssertions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Assertions []gebra.Assertion `json:"assertions"`
	}
	if !decode(w, r, &req) {
		return
	}
	err := s.engine.WriteAssertions(r.Context(), r.PathValue("store_id"), r.PathValue("model_id"), req.Assertions)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to write assertions", "error", err)
			writeInternalError(w)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) readAssertions(w http.ResponseWriter, r *http.Request) {
	modelID := r.PathValue("model_id")
	assertions, err := s.engine.ReadAssertions(r.Context(), r.PathValue("store_id"), modelID)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read assertions", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authorization_model_id": modelID, "assertions": assertions})
}

type tupleKeys struct {
	TupleKeys []gebra.TupleKey `json:"tuple_keys"`
}

// consistencyBody holds the consistency fields that every query accepts.
type consistencyBody struct {
	Consistency string `json:"consistency"`
	MinRevision string `json:"min_revision"`
}

func (b consistencyBody) consistency() gebra.Consistency {
	return gebra.Consistency{Preference: b.Consistency, MinRevision: b.MinRevision}
}

func (s *Server) write(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Writes *struct {
			TupleKeys   []gebra.TupleKey `json:"tuple_keys"`
			OnDuplicate string           `json:"on_duplicate"`
		} `json:"writes"`
		Deletes *struct {
			TupleKeys []gebra.TupleKey `json:"tuple_keys"`
			OnMissing string           `json:"on_missing"`
		} `json:"deletes"`
		AuthorizationModelID string `json:"authorization_model_id"`
		// Wait is a Gebra extension.
		Wait bool `json:"wait"`
	}
	if !decode(w, r, &req) {
		return
	}
	wr := &gebra.WriteRequest{AuthorizationModelID: req.AuthorizationModelID, Wait: req.Wait}
	if req.Writes != nil {
		wr.Writes, wr.OnDuplicate = req.Writes.TupleKeys, req.Writes.OnDuplicate
	}
	if req.Deletes != nil {
		wr.Deletes, wr.OnMissing = req.Deletes.TupleKeys, req.Deletes.OnMissing
	}
	revision, err := s.engine.Write(r.Context(), r.PathValue("store_id"), wr)
	if errors.Is(err, gebra.ErrNotApplied) {
		// The write is durable, so the response carries its revision, which
		// tells a client not to retry it.
		s.logger.ErrorContext(r.Context(), "Failed to wait for the applier", "error", err)
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{
			"code": "write_not_applied", "message": err.Error(), "revision": revision,
		})
		return
	}
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to write tuples", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"revision": revision})
}

func (s *Server) read(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TupleKey *struct {
			User     string `json:"user"`
			Relation string `json:"relation"`
			Object   string `json:"object"`
		} `json:"tuple_key"`
		PageSize          int    `json:"page_size"`
		ContinuationToken string `json:"continuation_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	rr := &gebra.ReadRequest{PageSize: req.PageSize, ContinuationToken: req.ContinuationToken}
	if req.TupleKey != nil {
		rr.User, rr.Relation, rr.Object = req.TupleKey.User, req.TupleKey.Relation, req.TupleKey.Object
	}
	tuples, next, err := s.engine.Read(r.Context(), r.PathValue("store_id"), rr)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read tuples", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tuples": tuples, "continuation_token": next})
}

func (s *Server) readChanges(w http.ResponseWriter, r *http.Request) {
	size, ok := pageSize(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	req := &gebra.ReadChangesRequest{Type: q.Get("type"), PageSize: size, ContinuationToken: q.Get("continuation_token")}
	if v := q.Get("start_time"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody{Code: "validation_error", Message: "start_time must be an RFC 3339 timestamp."})
			return
		}
		req.StartTime = t
	}
	changes, next, err := s.engine.ReadChanges(r.Context(), r.PathValue("store_id"), req)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read changes", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "continuation_token": next})
}

func (s *Server) writeAttributes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Objects []gebra.ObjectAttributes `json:"objects"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.engine.WriteAttributes(r.Context(), r.PathValue("store_id"), req.Objects); err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to write attributes", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TupleKey             gebra.CheckTupleKey `json:"tuple_key"`
		ContextualTuples     *tupleKeys          `json:"contextual_tuples"`
		Context              map[string]any      `json:"context"`
		AuthorizationModelID string              `json:"authorization_model_id"`
		consistencyBody
	}
	if !decode(w, r, &req) {
		return
	}
	cr := &gebra.CheckRequest{
		TupleKey: req.TupleKey, Context: req.Context, AuthorizationModelID: req.AuthorizationModelID,
		Consistency: req.consistency(),
	}
	if req.ContextualTuples != nil {
		cr.ContextualTuples = req.ContextualTuples.TupleKeys
	}
	allowed, err := s.engine.Check(r.Context(), r.PathValue("store_id"), cr)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to check", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowed": allowed, "resolution": ""})
}

func (s *Server) batchCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Checks []struct {
			TupleKey         gebra.CheckTupleKey `json:"tuple_key"`
			ContextualTuples *tupleKeys          `json:"contextual_tuples"`
			Context          map[string]any      `json:"context"`
			CorrelationID    string              `json:"correlation_id"`
		} `json:"checks"`
		AuthorizationModelID string `json:"authorization_model_id"`
		consistencyBody
	}
	if !decode(w, r, &req) {
		return
	}
	items := make([]gebra.BatchCheckItem, len(req.Checks))
	for i, c := range req.Checks {
		items[i] = gebra.BatchCheckItem{TupleKey: c.TupleKey, Context: c.Context, CorrelationID: c.CorrelationID}
		if c.ContextualTuples != nil {
			items[i].ContextualTuples = c.ContextualTuples.TupleKeys
		}
	}
	results, err := s.engine.BatchCheck(r.Context(), r.PathValue("store_id"), req.AuthorizationModelID, req.consistency(), items)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to batch check", "error", err)
			writeInternalError(w)
		}
		return
	}

	type itemError struct {
		InputError    string `json:"input_error,omitempty"`
		InternalError string `json:"internal_error,omitempty"`
		Message       string `json:"message"`
	}
	type itemResult struct {
		Allowed bool       `json:"allowed"`
		Error   *itemError `json:"error,omitempty"`
	}
	out := make(map[string]itemResult, len(results))
	for id, res := range results {
		ir := itemResult{Allowed: res.Allowed}
		switch {
		case res.Err == nil:
		case errors.Is(res.Err, gebra.ErrInvalidArgument) || errors.Is(res.Err, gebra.ErrNotFound):
			ir.Error = &itemError{InputError: "validation_error", Message: res.Err.Error()}
		default:
			s.logger.ErrorContext(r.Context(), "Failed to check in batch", "correlationID", id, "error", res.Err)
			ir.Error = &itemError{InternalError: "internal_error", Message: "Internal server error."}
		}
		out[id] = ir
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": out})
}

func (s *Server) expand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TupleKey struct {
			Relation string `json:"relation"`
			Object   string `json:"object"`
		} `json:"tuple_key"`
		ContextualTuples     *tupleKeys `json:"contextual_tuples"`
		AuthorizationModelID string     `json:"authorization_model_id"`
		consistencyBody
	}
	if !decode(w, r, &req) {
		return
	}
	er := &gebra.ExpandRequest{
		Object: req.TupleKey.Object, Relation: req.TupleKey.Relation,
		AuthorizationModelID: req.AuthorizationModelID, Consistency: req.consistency(),
	}
	if req.ContextualTuples != nil {
		er.ContextualTuples = req.ContextualTuples.TupleKeys
	}
	root, err := s.engine.Expand(r.Context(), r.PathValue("store_id"), er)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to expand", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": map[string]any{"root": root}})
}

type sortBody struct {
	Attribute  string `json:"attribute"`
	Descending bool   `json:"descending"`
}

type listObjectsBody struct {
	Type                 string         `json:"type"`
	Relation             string         `json:"relation"`
	User                 string         `json:"user"`
	ContextualTuples     *tupleKeys     `json:"contextual_tuples"`
	Context              map[string]any `json:"context"`
	AuthorizationModelID string         `json:"authorization_model_id"`
	Sort                 *sortBody      `json:"sort"`
	PageSize             int            `json:"page_size"`
	ContinuationToken    string         `json:"continuation_token"`
	consistencyBody
}

func (b *listObjectsBody) request() *gebra.ListObjectsRequest {
	req := &gebra.ListObjectsRequest{
		Type: b.Type, Relation: b.Relation, User: b.User, Context: b.Context,
		AuthorizationModelID: b.AuthorizationModelID, Consistency: b.consistency(),
		PageSize: b.PageSize, ContinuationToken: b.ContinuationToken,
	}
	if b.ContextualTuples != nil {
		req.ContextualTuples = b.ContextualTuples.TupleKeys
	}
	if b.Sort != nil {
		req.Sort = gebra.Sort{Attribute: b.Sort.Attribute, Descending: b.Sort.Descending}
	}
	return req
}

func (s *Server) listObjects(w http.ResponseWriter, r *http.Request) {
	var body listObjectsBody
	if !decode(w, r, &body) {
		return
	}
	objects, next, err := s.engine.ListObjects(r.Context(), r.PathValue("store_id"), body.request())
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to list objects", "error", err)
			writeInternalError(w)
		}
		return
	}
	if objects == nil {
		objects = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"objects": objects, "continuation_token": next})
}

// streamedListObjects writes one JSON object per line as results are found,
// like OpenFGA's streaming endpoint. Errors after the first result are sent
// in-band because the status code is already written.
func (s *Server) streamedListObjects(w http.ResponseWriter, r *http.Request) {
	var body listObjectsBody
	if !decode(w, r, &body) {
		return
	}
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	started := false
	err := s.engine.StreamListObjects(r.Context(), r.PathValue("store_id"), body.request(), func(object string) error {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		if err := enc.Encode(map[string]any{"result": map[string]string{"object": object}}); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	})
	if err != nil && r.Context().Err() == nil {
		if started {
			s.logger.ErrorContext(r.Context(), "Failed to stream list objects", "error", err)
			_ = enc.Encode(map[string]any{"error": errorBody{Code: "internal_error", Message: "Internal server error."}})
			return
		}
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to stream list objects", "error", err)
			writeInternalError(w)
		}
		return
	}
	if !started {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Object struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"object"`
		Relation             string             `json:"relation"`
		UserFilters          []gebra.UserFilter `json:"user_filters"`
		ContextualTuples     []gebra.TupleKey   `json:"contextual_tuples"`
		Context              map[string]any     `json:"context"`
		AuthorizationModelID string             `json:"authorization_model_id"`
		Sort                 *sortBody          `json:"sort"`
		PageSize             int                `json:"page_size"`
		ContinuationToken    string             `json:"continuation_token"`
		consistencyBody
	}
	if !decode(w, r, &body) {
		return
	}
	req := &gebra.ListUsersRequest{
		Object: body.Object.Type + ":" + body.Object.ID, Relation: body.Relation, UserFilters: body.UserFilters,
		ContextualTuples: body.ContextualTuples, Context: body.Context, AuthorizationModelID: body.AuthorizationModelID,
		Consistency: body.consistency(), PageSize: body.PageSize, ContinuationToken: body.ContinuationToken,
	}
	if body.Sort != nil {
		req.Sort = gebra.Sort{Attribute: body.Sort.Attribute, Descending: body.Sort.Descending}
	}
	users, next, err := s.engine.ListUsers(r.Context(), r.PathValue("store_id"), req)
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to list users", "error", err)
			writeInternalError(w)
		}
		return
	}
	if users == nil {
		users = []gebra.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "continuation_token": next})
}

func (s *Server) viewStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.engine.ViewStatus(r.Context(), r.PathValue("store_id"))
	if err != nil {
		if !writeClientError(w, err) {
			s.logger.ErrorContext(r.Context(), "Failed to read view status", "error", err)
			writeInternalError(w)
		}
		return
	}
	writeJSON(w, http.StatusOK, status)
}
