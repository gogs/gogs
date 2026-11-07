package git

import "context"

// CatFileBlobOptions contains optional arguments for verifying the objects.
//
// Docs: https://git-scm.com/docs/git-cat-file#Documentation/git-cat-file.txt
type CatFileBlobOptions struct {
	CommandOptions
}

// CatFileBlob returns the blob corresponding to the given revision of the repository.
func (r *Repository) CatFileBlob(ctx context.Context, rev string, opts ...CatFileBlobOptions) (*Blob, error) {
	var opt CatFileBlobOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Type conversions work because all three option types share the same
	// underlying structure (CommandOptions only).
	rev, err := r.RevParse(ctx, rev, RevParseOptions(opt)) //nolint
	if err != nil {
		return nil, err
	}

	typ, err := r.CatFileType(ctx, rev, CatFileTypeOptions(opt))
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
