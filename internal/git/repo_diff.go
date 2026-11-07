package git

import (
	"context"
	"fmt"
	"io"
)

// DiffOptions contains optional arguments for parsing diff.
//
// Docs: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt---full-index
type DiffOptions struct {
	// The commit ID to used for computing diff between a range of commits (base,
	// revision]. When not set, only computes diff for a single commit at revision.
	Base string
	CommandOptions
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

	stdout, w := io.Pipe()
	done := make(chan SteamParseDiffResult)
	go StreamParseDiff(stdout, done, maxFiles, maxFileLines, maxLineChars)

	err = pipe(ctx, r.path, args, opt.Envs, w)
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
	CommandOptions
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
		return fmt.Errorf("invalid diffType: %s", diffType)
	}

	if err = pipe(ctx, r.path, args, opt.Envs, w); err != nil {
		return err
	}
	return nil
}

// DiffBinaryOptions contains optional arguments for producing binary patch.
type DiffBinaryOptions struct {
	CommandOptions
}

// DiffBinary returns binary patch between base and head revisions that could be
// used for git-apply.
func (r *Repository) DiffBinary(ctx context.Context, base, head string, opts ...DiffBinaryOptions) ([]byte, error) {
	var opt DiffBinaryOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"diff", "--full-index", "--binary", "--end-of-options", base, head}
	return exec(ctx, r.path, args, opt.Envs)
}
