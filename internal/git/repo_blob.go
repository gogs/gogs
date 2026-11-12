package git

import "context"

// CatFileBlob returns the blob corresponding to the given revision of the repository.
func (r *Repository) CatFileBlob(ctx context.Context, rev string) (*Blob, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.CatFileBlob")
	defer span.End()

	rev, err := r.RevParse(ctx, rev)
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
