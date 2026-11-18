package git

import (
	"context"
	"path"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TreeEntry returns the TreeEntry by given subpath of the tree.
func (t *Tree) TreeEntry(ctx context.Context, subpath string) (_ *TreeEntry, err error) {
	ctx, span := tracer.Start(ctx, "git.Tree.TreeEntry", trace.WithAttributes(attribute.String("git.tree", t.id.String())))
	defer func() { endSpan(span, err) }()

	if len(subpath) == 0 {
		return &TreeEntry{
			id:   t.id,
			typ:  ObjectTree,
			mode: EntryTree,
		}, nil
	}

	subpath = path.Clean(subpath)
	paths := strings.Split(subpath, "/")
	tree := t
	for i, name := range paths {
		// The last path component is the entry itself.
		if i == len(paths)-1 {
			entries, err := tree.Entries(ctx)
			if err != nil {
				return nil, err
			}

			for _, v := range entries {
				if v.name == name {
					return v, nil
				}
			}
		} else {
			tree, err = tree.Subtree(ctx, name)
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, ErrRevisionNotExist
}

// Blob returns the blob object by given subpath of the tree.
func (t *Tree) Blob(ctx context.Context, subpath string) (_ *Blob, err error) {
	ctx, span := tracer.Start(ctx, "git.Tree.Blob", trace.WithAttributes(attribute.String("git.tree", t.id.String())))
	defer func() { endSpan(span, err) }()

	e, err := t.TreeEntry(ctx, subpath)
	if err != nil {
		return nil, err
	}

	if e.IsBlob() || e.IsExec() {
		return e.Blob(), nil
	}

	return nil, ErrNotBlob
}

// blobByIndex returns blob object by given index.
func (t *Tree) blobByIndex(ctx context.Context, index string) (*Blob, error) {
	typ, err := t.repo.catFileType(ctx, index)
	if err != nil {
		return nil, err
	}

	if typ != ObjectBlob {
		return nil, ErrNotBlob
	}

	id, err := t.repo.RevParse(ctx, index)
	if err != nil {
		return nil, err
	}

	return &Blob{
		TreeEntry: &TreeEntry{
			mode:   EntryBlob,
			typ:    ObjectBlob,
			id:     MustIDFromString(id),
			parent: t,
		},
	}, nil
}
