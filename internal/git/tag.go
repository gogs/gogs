package git

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Tag contains information of a Git tag.
type Tag struct {
	typ      ObjectType
	id       OID
	commitID OID // The ID of the underlying commit
	message  string

	repo *Repository
}

// Type returns the type of the tag.
func (t *Tag) Type() ObjectType {
	return t.typ
}

// ID returns the ID of the tag.
func (t *Tag) ID() OID {
	return t.id
}

// CommitID returns the commit ID of the tag.
func (t *Tag) CommitID() OID {
	return t.commitID
}

// Message returns the message of the tag.
func (t *Tag) Message() string {
	return t.message
}

// Commit returns the underlying commit of the tag.
func (t *Tag) Commit(ctx context.Context) (_ *Commit, err error) {
	ctx, span := tracer.Start(ctx, "git.Tag.Commit", trace.WithAttributes(attribute.String("git.tag", t.id.String())))
	defer func() { endSpan(span, err) }()

	return t.repo.CatFileCommit(ctx, t.commitID.String())
}
