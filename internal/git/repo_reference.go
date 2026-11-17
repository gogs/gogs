package git

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	RefsHeads = "refs/heads/"
	RefsTags  = "refs/tags/"
)

// RefShortName returns short name of heads or tags. Other references will
// return original string.
func RefShortName(ref string) string {
	if strings.HasPrefix(ref, RefsHeads) {
		return ref[len(RefsHeads):]
	} else if strings.HasPrefix(ref, RefsTags) {
		return ref[len(RefsTags):]
	}

	return ref
}

// Reference contains information of a Git reference.
type Reference struct {
	ID      string
	Refspec string
}

var ErrReferenceNotExist = errors.New("reference does not exist")

// showRefVerify returns the commit ID of given reference (e.g.,
// "refs/heads/master") if it exists in the repository.
func (r *Repository) showRefVerify(ctx context.Context, ref string) (string, error) {
	args := []string{"show-ref", "--verify", "--end-of-options", ref}
	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not a valid ref") {
			return "", ErrReferenceNotExist
		}
		return "", err
	}
	return strings.Split(string(stdout), " ")[0], nil
}

// BranchCommitID returns the commit ID of given branch if it exists in the
// repository. The branch must be given in short name, e.g., "master".
func (r *Repository) BranchCommitID(ctx context.Context, branch string) (_ string, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.BranchCommitID", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.branch", branch)))
	defer func() { endSpan(span, err) }()

	return r.showRefVerify(ctx, RefsHeads+branch)
}

// RepoBranchCommitID is like Repository.BranchCommitID for the repository in given path.
func RepoBranchCommitID(ctx context.Context, repoPath, branch string) (string, error) {
	r, err := Open(repoPath)
	if err != nil {
		return "", err
	}
	return r.BranchCommitID(ctx, branch)
}

// TagCommitID returns the commit ID of given tag if it exists in the
// repository. The tag must be given in short name, e.g., "v1.0.0".
func (r *Repository) TagCommitID(ctx context.Context, tag string) (_ string, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.TagCommitID", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.tag", tag)))
	defer func() { endSpan(span, err) }()

	return r.showRefVerify(ctx, RefsTags+tag)
}

// hasReference returns true if given reference exists in the repository. The
// reference must be given in full refspec, e.g., "refs/heads/master".
func (r *Repository) hasReference(ctx context.Context, ref string) bool {
	_, err := r.showRefVerify(ctx, ref)
	return err == nil
}

// HasBranch returns true if given branch exists in the repository. The branch
// must be given in short name, e.g., "master".
func (r *Repository) HasBranch(ctx context.Context, branch string) bool {
	ctx, span := tracer.Start(ctx, "git.Repository.HasBranch", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.branch", branch)))
	defer span.End()

	return r.hasReference(ctx, RefsHeads+branch)
}

// RepoHasBranch returns true if the branch exists in the repository in given path.
func RepoHasBranch(ctx context.Context, repoPath, branch string) bool {
	r, err := Open(repoPath)
	if err != nil {
		return false
	}
	return r.HasBranch(ctx, branch)
}

// HasBranches returns true if the repository has at least one branch, even when
// HEAD points to a branch that does not exist.
func (r *Repository) HasBranches(ctx context.Context) (_ bool, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.HasBranches", trace.WithAttributes(attribute.String("git.dir", r.path)))
	defer func() { endSpan(span, err) }()

	stdout, err := exec(ctx, r.path, []string{"for-each-ref", "--count=1", "--format=%(refname)", "--end-of-options", RefsHeads}, nil)
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(stdout)) > 0, nil
}

// RepoHasBranches is like Repository.HasBranches for the repository in given
// path.
func RepoHasBranches(ctx context.Context, repoPath string) (bool, error) {
	r, err := Open(repoPath)
	if err != nil {
		return false, err
	}
	return r.HasBranches(ctx)
}

// HasTag returns true if given tag exists in the repository. The tag must be
// given in short name, e.g., "v1.0.0".
func (r *Repository) HasTag(ctx context.Context, tag string) bool {
	ctx, span := tracer.Start(ctx, "git.Repository.HasTag", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.tag", tag)))
	defer span.End()

	return r.hasReference(ctx, RefsTags+tag)
}

// SymbolicRefOptions contains optional arguments for get and set symbolic ref.
type SymbolicRefOptions struct {
	// The name of the symbolic ref. When not set, default ref "HEAD" is used.
	Name string
	// The name of the reference, e.g., "refs/heads/master". When set, it will be
	// used to update the symbolic ref.
	Ref string
}

// SymbolicRef returns the reference name (e.g., "refs/heads/master") pointed by
// the symbolic ref. It returns an empty string and nil error when doing set
// operation.
func (r *Repository) SymbolicRef(ctx context.Context, opts ...SymbolicRefOptions) (_ string, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.SymbolicRef", trace.WithAttributes(attribute.String("git.dir", r.path)))
	defer func() { endSpan(span, err) }()

	var opt SymbolicRefOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"symbolic-ref"}
	if opt.Name == "" {
		opt.Name = "HEAD"
	}
	args = append(args, "--end-of-options", opt.Name)
	if opt.Ref != "" {
		args = append(args, opt.Ref)
	}

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// RepoSymbolicRef is like Repository.SymbolicRef for the repository in given path.
func RepoSymbolicRef(ctx context.Context, repoPath string, opts ...SymbolicRefOptions) (string, error) {
	r, err := Open(repoPath)
	if err != nil {
		return "", err
	}
	return r.SymbolicRef(ctx, opts...)
}

// showRefOptions contains optional arguments for listing references.
//
// Docs: https://git-scm.com/docs/git-show-ref
type showRefOptions struct {
	// Indicates whether to include heads.
	Heads bool
	// Indicates whether to include tags.
	Tags bool
	// The list of patterns to filter results.
	Patterns []string
}

// showRef returns a list of references in the repository.
func (r *Repository) showRef(ctx context.Context, opts ...showRefOptions) ([]*Reference, error) {
	var opt showRefOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"show-ref"}
	if opt.Heads {
		args = append(args, "--heads")
	}
	if opt.Tags {
		args = append(args, "--tags")
	}
	args = append(args, "--")
	if len(opt.Patterns) > 0 {
		args = append(args, opt.Patterns...)
	}

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(stdout), "\n")
	refs := make([]*Reference, 0, len(lines))
	for i := range lines {
		fields := strings.Fields(lines[i])
		if len(fields) != 2 {
			continue
		}
		refs = append(refs, &Reference{
			ID:      fields[0],
			Refspec: fields[1],
		})
	}
	return refs, nil
}

// Branches returns a list of all branches in the repository.
func (r *Repository) Branches(ctx context.Context) (_ []string, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.Branches", trace.WithAttributes(attribute.String("git.dir", r.path)))
	defer func() { endSpan(span, err) }()

	heads, err := r.showRef(ctx, showRefOptions{Heads: true})
	if err != nil {
		return nil, err
	}

	branches := make([]string, len(heads))
	for i := range heads {
		branches[i] = strings.TrimPrefix(heads[i].Refspec, RefsHeads)
	}
	return branches, nil
}

// RepoBranches is like Repository.Branches for the repository in given path.
func RepoBranches(ctx context.Context, repoPath string) ([]string, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.Branches(ctx)
}

// Branch is a branch with the latest commit on it.
type Branch struct {
	// The short name, e.g., "master".
	Name     string
	CommitID string
	// The committer of the latest commit.
	Committer *Signature
}

// BranchesByCommitterDate returns all branches in the repository, the most
// recently committed first, with a single Git command.
func (r *Repository) BranchesByCommitterDate(ctx context.Context) (_ []*Branch, err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.BranchesByCommitterDate", trace.WithAttributes(attribute.String("git.dir", r.path)))
	defer func() { endSpan(span, err) }()

	// Branch names and identities cannot contain NUL or newline characters.
	const format = "--format=%(refname)%00%(objectname)%00%(committerdate:iso-strict)%00%(committername)%00%(committeremail)"
	stdout, err := exec(ctx, r.path, []string{"for-each-ref", "--sort=-committerdate", format, "--end-of-options", RefsHeads}, nil)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSuffix(string(stdout), "\n"), "\n")
	branches := make([]*Branch, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\x00")
		if len(fields) != 5 {
			continue
		}
		// A branch that points to an object other than a commit has no
		// committer date.
		var when time.Time
		if fields[2] != "" {
			when, err = time.Parse(time.RFC3339, fields[2])
			if err != nil {
				return nil, errors.Wrapf(err, "parse committer date of %q", fields[0])
			}
		}
		branches = append(branches, &Branch{
			Name:     strings.TrimPrefix(fields[0], RefsHeads),
			CommitID: fields[1],
			Committer: &Signature{
				Name:  fields[3],
				Email: strings.TrimSuffix(strings.TrimPrefix(fields[4], "<"), ">"),
				When:  when,
			},
		})
	}
	return branches, nil
}

// CreateBranch creates the branch at the revision. It fails when the branch
// exists.
func (r *Repository) CreateBranch(ctx context.Context, name, rev string) (err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.CreateBranch", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.name", name), attribute.String("git.revision", rev)))
	defer func() { endSpan(span, err) }()

	_, err = exec(ctx, r.path, []string{"branch", "--no-track", "--end-of-options", name, rev}, nil)
	return err
}

// DeleteBranchOptions contains optional arguments for deleting a branch.
//
// Docs: https://git-scm.com/docs/git-branch
type DeleteBranchOptions struct {
	// Indicates whether to force delete the branch.
	Force bool
}

// DeleteBranch deletes the branch from the repository.
func (r *Repository) DeleteBranch(ctx context.Context, name string, opts ...DeleteBranchOptions) (err error) {
	ctx, span := tracer.Start(ctx, "git.Repository.DeleteBranch", trace.WithAttributes(attribute.String("git.dir", r.path), attribute.String("git.name", name)))
	defer func() { endSpan(span, err) }()

	var opt DeleteBranchOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"branch"}
	if opt.Force {
		args = append(args, "-D")
	} else {
		args = append(args, "-d")
	}
	args = append(args, "--end-of-options", name)
	_, err = exec(ctx, r.path, args, nil)
	return err
}
