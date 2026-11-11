package git

import "context"

// Tag contains information of a Git tag.
type Tag struct {
	typ      ObjectType
	id       *SHA1
	commitID *SHA1 // The ID of the underlying commit
	message  string

	repo *Repository
}

// Type returns the type of the tag.
func (t *Tag) Type() ObjectType {
	return t.typ
}

// ID returns the SHA-1 hash of the tag.
func (t *Tag) ID() *SHA1 {
	return t.id
}

// CommitID returns the commit ID of the tag.
func (t *Tag) CommitID() *SHA1 {
	return t.commitID
}

// Message returns the message of the tag.
func (t *Tag) Message() string {
	return t.message
}

// Commit returns the underlying commit of the tag.
func (t *Tag) Commit(ctx context.Context) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Tag.Commit")
	defer span.End()

	return t.repo.CatFileCommit(ctx, t.commitID.String())
}
