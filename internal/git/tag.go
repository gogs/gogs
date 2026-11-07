package git

import "context"

// Tag contains information of a Git tag.
type Tag struct {
	typ      ObjectType
	id       *SHA1
	commitID *SHA1 // The ID of the underlying commit
	refspec  string
	tagger   *Signature
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

// Refspec returns the refspec of the tag.
func (t *Tag) Refspec() string {
	return t.refspec
}

// Tagger returns the tagger of the tag.
func (t *Tag) Tagger() *Signature {
	return t.tagger
}

// Message returns the message of the tag.
func (t *Tag) Message() string {
	return t.message
}

// Commit returns the underlying commit of the tag.
func (t *Tag) Commit(ctx context.Context, opts ...CatFileCommitOptions) (*Commit, error) {
	return t.repo.CatFileCommit(ctx, t.commitID.String(), opts...)
}
