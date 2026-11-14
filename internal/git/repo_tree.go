package git

import (
	"bytes"
	"context"

	"github.com/cockroachdb/errors"
)

// unescapeChars reverses escaped characters in quoted output from Git.
func unescapeChars(in []byte) []byte {
	if !bytes.ContainsRune(in, '\\') {
		return in
	}

	out := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		if in[i] == '\\' && i+1 < len(in) {
			switch in[i+1] {
			case '\\':
				out = append(out, '\\')
				i++
			case '"':
				out = append(out, '"')
				i++
			case 't':
				out = append(out, '\t')
				i++
			case 'n':
				out = append(out, '\n')
				i++
			default:
				out = append(out, in[i])
			}
		} else {
			out = append(out, in[i])
		}
	}
	return out
}

// parseTree parses tree information from the (uncompressed) raw data of the
// tree object, as printed by "git ls-tree -z".
func parseTree(t *Tree, data []byte) ([]*TreeEntry, error) {
	entries := make([]*TreeEntry, 0, 10)
	l := len(data)
	pos := 0
	for pos < l {
		entry := new(TreeEntry)
		entry.parent = t
		step := 6
		switch string(data[pos : pos+step]) {
		case "100644", "100664":
			entry.mode = EntryBlob
			entry.typ = ObjectBlob
		case "100755":
			entry.mode = EntryExec
			entry.typ = ObjectBlob
		case "120000":
			entry.mode = EntrySymlink
			entry.typ = ObjectBlob
		case "160000":
			entry.mode = EntryCommit
			entry.typ = ObjectCommit

			step = 8
		case "040000":
			entry.mode = EntryTree
			entry.typ = ObjectTree
		default:
			return nil, errors.Newf("unknown type: %v", string(data[pos:pos+step]))
		}
		pos += step + 6 // Skip string type of entry type.

		step = bytes.IndexByte(data[pos:], '\t')
		id, err := newIDFromString(string(data[pos : pos+step]))
		if err != nil {
			return nil, err
		}
		entry.id = id
		pos += step + 1 // Skip the tab.

		step = bytes.IndexByte(data[pos:], 0)
		entry.name = string(data[pos : pos+step])

		pos += step + 1
		entries = append(entries, entry)
	}
	return entries, nil
}

// LsTree returns the tree object in the repository by given tree ID.
//
// Docs: https://git-scm.com/docs/git-ls-tree
func (r *Repository) LsTree(ctx context.Context, treeID string) (*Tree, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.LsTree")
	defer span.End()

	cache, ok := r.cachedTrees.Load(treeID)
	if ok {
		logf("Cached tree hit: %s", treeID)
		return cache.(*Tree), nil
	}

	var err error
	treeID, err = r.RevParse(ctx, treeID) //nolint
	if err != nil {
		return nil, err
	}
	t := &Tree{
		id:   MustIDFromString(treeID),
		repo: r,
	}

	// Use NUL-terminated output so that file names are printed verbatim rather
	// than quoted.
	args := []string{"ls-tree", "-z", "--end-of-options", treeID}

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return nil, err
	}

	t.entries, err = parseTree(t, stdout)
	if err != nil {
		return nil, err
	}

	r.cachedTrees.Store(treeID, t)
	return t, nil
}

// RepoLsTree is like Repository.LsTree for the repository in given path.
func RepoLsTree(ctx context.Context, repoPath, treeID string) (*Tree, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.LsTree(ctx, treeID)
}
