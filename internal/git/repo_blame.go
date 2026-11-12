package git

import (
	"bytes"
	"context"
)

// BlameOptions contains optional arguments for blaming a file.
// Docs: https://git-scm.com/docs/git-blame
type BlameOptions struct {
	CommandOptions
}

// Blame returns blame results of the file with the given revision of the
// repository.
func (r *Repository) Blame(ctx context.Context, rev, file string, opts ...BlameOptions) (*Blame, error) {
	var opt BlameOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"blame", "-l", "-s", rev, "--", file}
	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(stdout, []byte{'\n'})
	blame := &Blame{
		lines: make([]*Commit, 0, len(lines)),
	}
	for _, line := range lines {
		if len(line) < 40 {
			break
		}
		id := line[:40]

		// Earliest commit is indicated by a leading "^"
		if id[0] == '^' {
			id = id[1:]
		}
		commit, err := r.CatFileCommit(ctx, string(id)) //nolint
		if err != nil {
			return nil, err
		}
		blame.lines = append(blame.lines, commit)
	}
	return blame, nil
}
