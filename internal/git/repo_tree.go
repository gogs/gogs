package git

import (
	"bytes"
	"context"
	"fmt"
)

// UnescapeChars reverses escaped characters in quoted output from Git.
func UnescapeChars(in []byte) []byte {
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
// tree object. The lineTerminator specifies the character used to separate
// entries ('\n' for normal output, '\x00' for verbatim output).
func parseTree(t *Tree, data []byte, lineTerminator byte) ([]*TreeEntry, error) {
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
			return nil, fmt.Errorf("unknown type: %v", string(data[pos:pos+step]))
		}
		pos += step + 6 // Skip string type of entry type.

		step = 40
		id, err := NewIDFromString(string(data[pos : pos+step]))
		if err != nil {
			return nil, err
		}
		entry.id = id
		pos += step + 1 // Skip half of SHA1.

		step = bytes.IndexByte(data[pos:], lineTerminator)
		if data[pos] == '"' {
			entry.name = string(UnescapeChars(data[pos+1 : pos+step-1]))
		} else {
			entry.name = string(data[pos : pos+step])
		}

		pos += step + 1
		entries = append(entries, entry)
	}
	return entries, nil
}

// LsTreeOptions contains optional arguments for listing trees.
//
// Docs: https://git-scm.com/docs/git-ls-tree
type LsTreeOptions struct {
	// Verbatim outputs filenames unquoted using the -z flag. This avoids issues
	// with special characters in filenames that would otherwise be quoted by Git.
	Verbatim bool
	CommandOptions
}

// LsTree returns the tree object in the repository by given tree ID.
func (r *Repository) LsTree(ctx context.Context, treeID string, opts ...LsTreeOptions) (*Tree, error) {
	var opt LsTreeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cache, ok := r.cachedTrees.Get(treeID)
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

	args := []string{"ls-tree"}
	if opt.Verbatim {
		args = append(args, "-z")
	}
	args = append(args, "--end-of-options", treeID)

	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}

	lineTerminator := byte('\n')
	if opt.Verbatim {
		lineTerminator = 0
	}
	t.entries, err = parseTree(t, stdout, lineTerminator)
	if err != nil {
		return nil, err
	}

	r.cachedTrees.Set(treeID, t)
	return t, nil
}
