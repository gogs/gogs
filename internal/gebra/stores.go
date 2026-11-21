package gebra

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Store is an isolated set of authorization models and tuples.
type Store struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *storeRow) store() *Store {
	return &Store{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// CreateStore creates a store with the given name.
func (e *Engine) CreateStore(ctx context.Context, name string) (*Store, error) {
	if name == "" || len(name) > 256 {
		return nil, invalidf("store name must be 1 to 256 characters")
	}
	now := time.Now().UTC()
	row := &storeRow{ID: newID(), Name: name, CreatedAt: now, UpdatedAt: now}
	err := e.writeTx(ctx, func(tx dbtx) error {
		_, err := tx.exec(ctx, "INSERT INTO gebra_store (id, name, model_id, revision, created_at, updated_at) VALUES (?, ?, '', 0, ?, ?)",
			row.ID, row.Name, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, "create store")
		}
		return resetViewState(ctx, tx, row.ID)
	})
	if err != nil {
		return nil, err
	}
	return row.store(), nil
}

// GetStore returns the store with the given ID.
func (e *Engine) GetStore(ctx context.Context, id string) (*Store, error) {
	row, err := e.storeRow(ctx, e.db.handle(), id)
	if err != nil {
		return nil, err
	}
	return row.store(), nil
}

func (e *Engine) storeRow(ctx context.Context, db dbtx, id string) (*storeRow, error) {
	rows, err := queryStructs[storeRow](ctx, db, "SELECT * FROM gebra_store WHERE id = ? AND deleted_at IS NULL", id)
	if err != nil {
		return nil, errors.Wrap(err, "get store")
	}
	if len(rows) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "store %q", id)
	}
	return &rows[0], nil
}

// ListStores returns stores in creation order. A zero pageSize returns all of
// them. When name is not empty, only stores with exactly that name are listed.
func (e *Engine) ListStores(ctx context.Context, name string, pageSize int, continuationToken string) ([]*Store, string, error) {
	q := "SELECT * FROM gebra_store WHERE deleted_at IS NULL"
	var args []any
	if name != "" {
		q += " AND name = ?"
		args = append(args, name)
	}
	if continuationToken != "" {
		last, err := decodeToken(continuationToken)
		if err != nil {
			return nil, "", err
		}
		q += " AND id > ?"
		args = append(args, last)
	}
	q += " ORDER BY id"
	if pageSize > 0 {
		q += " LIMIT ?"
		args = append(args, pageSize+1)
	}
	rows, err := queryStructs[storeRow](ctx, e.db.handle(), q, args...)
	if err != nil {
		return nil, "", errors.Wrap(err, "list stores")
	}
	var next string
	if pageSize > 0 && len(rows) > pageSize {
		rows = rows[:pageSize]
		next = encodeToken(rows[pageSize-1].ID)
	}
	stores := make([]*Store, len(rows))
	for i, r := range rows {
		stores[i] = r.store()
	}
	return stores, next, nil
}

// DeleteStore marks the store as deleted. Its tuples and models stay in the
// database, and engines delete its pending writes, view, and change log in the
// background.
func (e *Engine) DeleteStore(ctx context.Context, id string) error {
	now := time.Now().UTC()
	n, err := e.db.handle().exec(ctx, "UPDATE gebra_store SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL", now, now, id)
	if err != nil {
		return errors.Wrap(err, "delete store")
	}
	if n == 0 {
		return errors.Wrapf(ErrNotFound, "store %q", id)
	}
	return nil
}

// WriteAuthorizationModel validates and stores a new model, which becomes the
// store's latest model at once: later writes validate tuples against it, and
// ReadAuthorizationModel returns it. Queries that name no model keep using the
// live model, the one of the store's serving view, while the store's applier
// builds a view for the new model next to it. The new model goes live once
// that view is complete, for every query at once, and ViewStatus reports the
// build's progress. Until then, queries that name the new model evaluate the
// graph. A newer model cancels the build of an older one. A store's first
// model has no view to wait for, and queries evaluate the graph with it until
// its view is complete.
func (e *Engine) WriteAuthorizationModel(ctx context.Context, storeID string, model *AuthorizationModel) (_ string, err error) {
	ctx, span := e.tel.tracer.Start(ctx, "gebra.write_authorization_model",
		trace.WithAttributes(attribute.String("gebra.store.id", storeID)))
	defer func() {
		var attrs []attribute.KeyValue
		(&op{span: span}).finish(err, &attrs, nil)
	}()
	src := *model
	src.ID = newID()
	compiled, err := compileModel(&src, e.opts.ConditionCostLimit)
	if err != nil {
		return "", err
	}
	def, err := json.Marshal(&src)
	if err != nil {
		return "", errors.Wrap(err, "marshal model")
	}

	err = e.writeTx(ctx, func(tx dbtx) error {
		now := time.Now().UTC()
		// Model writers of a store wait for each other on its row. Tuple
		// writers read the row without locking it.
		n, err := tx.exec(ctx, "UPDATE gebra_store SET model_id = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND deleted_at IS NULL", src.ID, now, storeID)
		if err != nil {
			return errors.Wrap(err, "update store")
		}
		if n == 0 {
			return errors.Wrapf(ErrNotFound, "store %q", storeID)
		}
		_, err = tx.exec(ctx, "INSERT INTO gebra_model (id, store_id, definition, created_at) VALUES (?, ?, ?, ?)", src.ID, storeID, string(def), now)
		if err != nil {
			return errors.Wrap(err, "create model")
		}
		_, err = tx.exec(ctx, "INSERT INTO gebra_inbox (store_id, kind, changes, tuples, created_at) VALUES (?, ?, '', 0, ?)", storeID, inboxModel, now)
		return errors.Wrap(err, "record model write")
	})
	if err != nil {
		return "", err
	}
	e.models.Store(src.ID, compiled)
	e.kick(storeID)
	e.logger.InfoContext(ctx, "Wrote authorization model", "storeID", storeID, "modelID", src.ID)
	return src.ID, nil
}

// model returns the compiled model with the given ID in the store.
func (e *Engine) model(ctx context.Context, db dbtx, storeID, id string) (*compiledModel, error) {
	if id == "" {
		return nil, errors.Wrapf(ErrNotFound, "store %q has no authorization model", storeID)
	}
	if m, ok := e.models.Load(id); ok {
		return m.(*compiledModel), nil
	}
	defs, err := queryColumn[string](ctx, db, "SELECT definition FROM gebra_model WHERE id = ? AND store_id = ?", id, storeID)
	if err != nil {
		return nil, errors.Wrap(err, "get model")
	}
	if len(defs) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "authorization model %q", id)
	}
	var src AuthorizationModel
	if err := json.Unmarshal([]byte(defs[0]), &src); err != nil {
		return nil, errors.Wrap(err, "unmarshal model")
	}
	m, err := compileModel(&src, e.opts.ConditionCostLimit)
	if err != nil {
		return nil, errors.Wrap(err, "compile stored model")
	}
	e.models.Store(id, m)
	return m, nil
}

// ReadAuthorizationModel returns a model of the store. An empty id returns the
// latest model, which writes validate tuples against, even while the live
// model that queries use by default is an older one.
func (e *Engine) ReadAuthorizationModel(ctx context.Context, storeID, id string) (*AuthorizationModel, error) {
	db := e.db.handle()
	st, err := e.storeRow(ctx, db, storeID)
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = st.ModelID
	}
	m, err := e.model(ctx, db, storeID, id)
	if err != nil {
		return nil, err
	}
	return m.source, nil
}

// ReadAuthorizationModels returns the store's models, newest first.
func (e *Engine) ReadAuthorizationModels(ctx context.Context, storeID string, pageSize int, continuationToken string) ([]*AuthorizationModel, string, error) {
	db := e.db.handle()
	if _, err := e.storeRow(ctx, db, storeID); err != nil {
		return nil, "", err
	}
	q := "SELECT id FROM gebra_model WHERE store_id = ?"
	args := []any{storeID}
	if continuationToken != "" {
		last, err := decodeToken(continuationToken)
		if err != nil {
			return nil, "", err
		}
		q += " AND id < ?"
		args = append(args, last)
	}
	q += " ORDER BY id DESC"
	if pageSize > 0 {
		q += " LIMIT ?"
		args = append(args, pageSize+1)
	}
	ids, err := queryColumn[string](ctx, db, q, args...)
	if err != nil {
		return nil, "", errors.Wrap(err, "list models")
	}
	var next string
	if pageSize > 0 && len(ids) > pageSize {
		ids = ids[:pageSize]
		next = encodeToken(ids[pageSize-1])
	}
	models := make([]*AuthorizationModel, 0, len(ids))
	for _, id := range ids {
		m, err := e.model(ctx, db, storeID, id)
		if err != nil {
			return nil, "", err
		}
		models = append(models, m.source)
	}
	return models, next, nil
}

// Assertion is a check that a model is expected to pass or fail, stored for
// testing models.
type Assertion struct {
	TupleKey         CheckTupleKey  `json:"tuple_key"`
	Expectation      bool           `json:"expectation"`
	ContextualTuples []TupleKey     `json:"contextual_tuples,omitempty"`
	Context          map[string]any `json:"context,omitempty"`
}

// CheckTupleKey is the user, relation, and object of a check.
type CheckTupleKey struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// WriteAssertions replaces the assertions of a model.
func (e *Engine) WriteAssertions(ctx context.Context, storeID, modelID string, assertions []Assertion) error {
	db := e.db.handle()
	if _, err := e.storeRow(ctx, db, storeID); err != nil {
		return err
	}
	m, err := e.model(ctx, db, storeID, modelID)
	if err != nil {
		return err
	}
	for _, a := range assertions {
		if _, err := parseCheck(m, a.TupleKey); err != nil {
			return err
		}
		if _, err := parseContextualTuples(m, a.ContextualTuples); err != nil {
			return err
		}
	}
	def, err := json.Marshal(assertions)
	if err != nil {
		return errors.Wrap(err, "marshal assertions")
	}
	_, err = db.insertRows(ctx, "gebra_assertion", []string{"store_id", "model_id", "definition"}, [][]any{{storeID, modelID, string(def)}},
		onConflict{key: []string{"store_id", "model_id"}, update: []string{"definition"}})
	return errors.Wrap(err, "write assertions")
}

// ReadAssertions returns the assertions of a model.
func (e *Engine) ReadAssertions(ctx context.Context, storeID, modelID string) ([]Assertion, error) {
	db := e.db.handle()
	if _, err := e.model(ctx, db, storeID, modelID); err != nil {
		return nil, err
	}
	defs, err := queryColumn[string](ctx, db, "SELECT definition FROM gebra_assertion WHERE store_id = ? AND model_id = ?", storeID, modelID)
	if err != nil {
		return nil, errors.Wrap(err, "get assertions")
	}
	out := []Assertion{}
	if len(defs) == 0 {
		return out, nil
	}
	if err := json.Unmarshal([]byte(defs[0]), &out); err != nil {
		return nil, errors.Wrap(err, "unmarshal assertions")
	}
	return out, nil
}

func encodeToken(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func decodeToken(token string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", invalidf("invalid continuation token")
	}
	return string(b), nil
}

func decodeIntToken(token string) (int64, error) {
	s, err := decodeToken(token)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, invalidf("invalid continuation token")
	}
	return n, nil
}
