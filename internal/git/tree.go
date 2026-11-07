package git

import (
	"context"
	"strings"
	"sync"
)

// Tree represents a flat directory listing in Git.
type Tree struct {
	id     *SHA1
	parent *Tree

	repo *Repository

	entries    Entries
	entriesMu  sync.Mutex
	entriesSet bool
}

// Subtree returns a subtree by given subpath of the tree.
func (t *Tree) Subtree(ctx context.Context, subpath string, opts ...LsTreeOptions) (*Tree, error) {
	if len(subpath) == 0 {
		return t, nil
	}

	paths := strings.Split(subpath, "/")
	var (
		err error
		g   = t
		p   = t
		e   *TreeEntry
	)
	for _, name := range paths {
		e, err = p.TreeEntry(ctx, name, opts...)
		if err != nil {
			return nil, err
		}

		g = &Tree{
			id:     e.id,
			parent: p,
			repo:   t.repo,
		}
		p = g
	}
	return g, nil
}

// Entries returns all entries of the tree. Successful results are cached;
// failed attempts are not cached, allowing retries with a fresh context.
func (t *Tree) Entries(ctx context.Context, opts ...LsTreeOptions) (Entries, error) {
	t.entriesMu.Lock()
	defer t.entriesMu.Unlock()

	if t.entriesSet {
		return t.entries, nil
	}

	tt, err := t.repo.LsTree(ctx, t.id.String(), opts...)
	if err != nil {
		return nil, err
	}
	t.entries = tt.entries
	t.entriesSet = true
	return t.entries, nil
}
