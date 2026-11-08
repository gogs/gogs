package git

import (
	"context"
	"io"
	"slices"

	"github.com/cockroachdb/errors"
)

// DiffOptions contains optional arguments for parsing diff.
//
// Docs: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt---full-index
type DiffOptions struct {
	// The commit ID to used for computing diff between a range of commits (base,
	// revision]. When not set, only computes diff for a single commit at revision.
	Base string
}

// Diff returns a parsed diff object between given commits of the repository.
func (r *Repository) Diff(ctx context.Context, rev string, maxFiles, maxFileLines, maxLineChars int, opts ...DiffOptions) (*Diff, error) {
	var opt DiffOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	commit, err := r.CatFileCommit(ctx, rev)
	if err != nil {
		return nil, err
	}

	var args []string
	if opt.Base == "" {
		// First commit of repository
		if commit.ParentsCount() == 0 {
			args = []string{"show", "--full-index", "--end-of-options", rev}
		} else {
			c, err := commit.Parent(ctx, 0)
			if err != nil {
				return nil, err
			}
			args = []string{"diff", "--full-index", "-M", c.ID.String(), "--end-of-options", rev}
		}
	} else {
		args = []string{"diff", "--full-index", "-M", opt.Base, "--end-of-options", rev}
	}
	return r.parseDiff(ctx, args, nil, maxFiles, maxFileLines, maxLineChars)
}

// DiffWorktree returns a parsed diff of unstaged changes to given path in the
// working tree of the repository.
func (r *Repository) DiffWorktree(ctx context.Context, path string, maxFiles, maxFileLines, maxLineChars int) (*Diff, error) {
	args := []string{"diff", "--", path}
	return r.parseDiff(ctx, args, nil, maxFiles, maxFileLines, maxLineChars)
}

func (r *Repository) parseDiff(ctx context.Context, args, envs []string, maxFiles, maxFileLines, maxLineChars int) (*Diff, error) {
	stdout, w := io.Pipe()
	done := make(chan SteamParseDiffResult)
	go StreamParseDiff(stdout, done, maxFiles, maxFileLines, maxLineChars)

	err := pipe(ctx, r.path, args, envs, w)
	_ = w.Close() // Close writer to exit parsing goroutine
	if err != nil {
		return nil, err
	}

	result := <-done
	return result.Diff, result.Err
}

// RawDiffFormat is the format of a raw diff.
type RawDiffFormat string

const (
	RawDiffNormal RawDiffFormat = "diff"
	RawDiffPatch  RawDiffFormat = "patch"
)

// RawDiffOptions contains optional arguments for dumping a raw diff.
//
// Docs: https://git-scm.com/docs/git-format-patch
type RawDiffOptions struct {
	// Additional arguments to be passed right after the subcommand, e.g., "-w".
	Args []string
}

// RawDiff dumps diff of repository in given revision directly to given
// io.Writer.
func (r *Repository) RawDiff(ctx context.Context, rev string, diffType RawDiffFormat, w io.Writer, opts ...RawDiffOptions) error {
	var opt RawDiffOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	commit, err := r.CatFileCommit(ctx, rev) //nolint
	if err != nil {
		return err
	}

	var args []string
	switch diffType {
	case RawDiffNormal:
		if commit.ParentsCount() == 0 {
			args = []string{"show", "--full-index", "--end-of-options", rev}
		} else {
			c, err := commit.Parent(ctx, 0)
			if err != nil {
				return err
			}
			args = []string{"diff", "--full-index", "-M", c.ID.String(), "--end-of-options", rev}
		}
	case RawDiffPatch:
		if commit.ParentsCount() == 0 {
			args = []string{"format-patch", "--full-index", "--no-signoff", "--no-signature", "--stdout", "--root", "--end-of-options", rev}
		} else {
			c, err := commit.Parent(ctx, 0)
			if err != nil {
				return err
			}
			args = []string{"format-patch", "--full-index", "--no-signoff", "--no-signature", "--stdout", "--end-of-options", rev + "..." + c.ID.String()}
		}
	default:
		return errors.Newf("invalid diffType: %s", diffType)
	}
	args = slices.Concat(args[:1], opt.Args, args[1:])

	if err = pipe(ctx, r.path, args, nil, w); err != nil {
		return err
	}
	return nil
}

// DiffBinary returns binary patch between base and head revisions that could be
// used for git-apply.
func (r *Repository) DiffBinary(ctx context.Context, base, head string) ([]byte, error) {
	args := []string{"diff", "--full-index", "--binary", "--end-of-options", base, head}
	return exec(ctx, r.path, args, nil)
}
