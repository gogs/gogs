package git

import (
	"context"
	"io"
	"slices"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"
)

// DiffOptions contains optional arguments for parsing diff.
//
// Docs: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt---full-index
type DiffOptions struct {
	// The commit ID to use for computing diff between a range of commits (base,
	// revision]. When not set, only computes diff for a single commit at revision.
	Base string
}

// Diff returns a parsed diff object between given commits of the repository.
func (r *Repository) Diff(ctx context.Context, logger *logx.Logger, rev string, maxFiles, maxFileLines, maxLineChars int, opts ...DiffOptions) (_ *Diff, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.Diff", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.revision", rev), attribute.Int64("git.max_files", int64(maxFiles))))
	defer func() { endSpan(span, err) }()

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
	return r.parseDiff(ctx, logger, args, nil, maxFiles, maxFileLines, maxLineChars)
}

func (r *Repository) parseDiff(ctx context.Context, logger *logx.Logger, args, envs []string, maxFiles, maxFileLines, maxLineChars int) (*Diff, error) {
	stdout, w := io.Pipe()
	done := make(chan streamParseDiffResult)
	go streamParseDiff(logger, stdout, done, maxFiles, maxFileLines, maxLineChars)

	err := pipe(ctx, r.path, args, envs, w)
	_ = w.Close() // Close writer to exit parsing goroutine.
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
	// Base is the revision to diff against instead of the first parent. It is
	// only supported by RawDiffNormal.
	Base string
}

// RawDiff dumps diff of repository in given revision directly to given
// io.Writer.
func (r *Repository) RawDiff(ctx context.Context, rev string, diffType RawDiffFormat, w io.Writer, opts ...RawDiffOptions) (err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.RawDiff", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.revision", rev), attribute.String("git.diff.format", string(diffType))))
	defer func() { endSpan(span, err) }()

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
		if opt.Base != "" {
			args = []string{"diff", "--full-index", "-M", opt.Base, "--end-of-options", rev}
		} else if commit.ParentsCount() == 0 {
			args = []string{"show", "--full-index", "--end-of-options", rev}
		} else {
			c, err := commit.Parent(ctx, 0)
			if err != nil {
				return err
			}
			args = []string{"diff", "--full-index", "-M", c.ID.String(), "--end-of-options", rev}
		}
	case RawDiffPatch:
		if opt.Base != "" {
			return errors.New("base is only supported by RawDiffNormal")
		}
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
func (r *Repository) DiffBinary(ctx context.Context, base, head string) (_ []byte, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.DiffBinary", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.base", base), attribute.String("git.head", head)))
	defer func() { endSpan(span, err) }()

	args := []string{"diff", "--full-index", "--binary", "--end-of-options", base, head}
	return exec(ctx, r.path, args, nil)
}
