package git

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CatFileBlob returns the blob corresponding to the given revision of the repository.
func (r *Repository) CatFileBlob(ctx context.Context, rev string) (_ *Blob, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.CatFileBlob", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.revision", rev)))
	defer func() { endSpan(span, err) }()

	rev, err = r.RevParse(ctx, rev)
	if err != nil {
		return nil, err
	}

	typ, err := r.catFileType(ctx, rev)
	if err != nil {
		return nil, err
	}

	if typ != ObjectBlob {
		return nil, ErrNotBlob
	}

	return &Blob{
		TreeEntry: &TreeEntry{
			mode: EntryBlob,
			typ:  ObjectBlob,
			id:   MustIDFromString(rev),
			parent: &Tree{
				repo: r,
			},
		},
	}, nil
}

// RepoCatFileBlob is like Repository.CatFileBlob for the repository in given path.
func RepoCatFileBlob(ctx context.Context, repoPath, rev string) (*Blob, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.CatFileBlob(ctx, rev)
}
