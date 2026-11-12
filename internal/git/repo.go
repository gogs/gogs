package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// Repository contains information of a Git repository.
type Repository struct {
	path string

	cachedCommits *objectCache
	cachedTags    *objectCache
	cachedTrees   *objectCache
}

// Path returns the path of the repository.
func (r *Repository) Path() string {
	return r.path
}

const LogFormatHashOnly = `format:%H`

// parsePrettyFormatLogToList returns a list of commits parsed from given logs
// that are formatted in LogFormatHashOnly.
func (r *Repository) parsePrettyFormatLogToList(ctx context.Context, logs []byte) ([]*Commit, error) {
	if len(logs) == 0 {
		return []*Commit{}, nil
	}

	var err error
	ids := bytes.Split(logs, []byte{'\n'})
	commits := make([]*Commit, len(ids))
	for i, id := range ids {
		commits[i], err = r.CatFileCommit(ctx, string(id))
		if err != nil {
			return nil, err
		}
	}
	return commits, nil
}

// InitOptions contains optional arguments for initializing a repository.
//
// Docs: https://git-scm.com/docs/git-init
type InitOptions struct {
	// Indicates whether the repository should be initialized in bare format.
	Bare bool
	CommandOptions
}

// Init initializes a new Git repository.
func Init(ctx context.Context, path string, opts ...InitOptions) error {
	var opt InitOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		return err
	}

	args := []string{"init"}
	if opt.Bare {
		args = append(args, "--bare")
	}
	args = append(args, "--end-of-options")
	_, err = exec(ctx, path, args, opt.Envs)
	return err
}

// Open opens the repository at the given path. It returns an os.ErrNotExist if
// the path does not exist.
func Open(repoPath string) (*Repository, error) {
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	} else if !isDir(repoPath) {
		return nil, os.ErrNotExist
	}

	return &Repository{
		path:          repoPath,
		cachedCommits: newObjectCache(),
		cachedTags:    newObjectCache(),
		cachedTrees:   newObjectCache(),
	}, nil
}

// CloneOptions contains optional arguments for cloning a repository.
//
// Docs: https://git-scm.com/docs/git-clone
type CloneOptions struct {
	// Indicates whether the repository should be cloned as a mirror.
	Mirror bool
	// Indicates whether the repository should be cloned in bare format.
	Bare bool
	// Indicates whether to suppress the log output.
	Quiet bool
	// The branch to checkout for the working tree when Bare=false.
	Branch string
	// The number of revisions to clone.
	Depth uint64
	CommandOptions
}

// Clone clones the repository from remote URL to the destination.
func Clone(ctx context.Context, url, dst string, opts ...CloneOptions) error {
	var opt CloneOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	err := os.MkdirAll(path.Dir(dst), os.ModePerm)
	if err != nil {
		return err
	}

	args := []string{"clone"}
	if opt.Mirror {
		args = append(args, "--mirror")
	}
	if opt.Bare {
		args = append(args, "--bare")
	}
	if opt.Quiet {
		args = append(args, "--quiet")
	}
	if !opt.Bare && opt.Branch != "" {
		args = append(args, "-b", opt.Branch)
	}
	if opt.Depth > 0 {
		args = append(args, "--depth", strconv.FormatUint(opt.Depth, 10))
	}

	args = append(args, "--end-of-options", url, dst)
	_, err = exec(ctx, "", args, opt.Envs)
	return err
}

// FetchOptions contains optional arguments for fetching repository updates.
//
// Docs: https://git-scm.com/docs/git-fetch
type FetchOptions struct {
	// Indicates whether to prune during fetching.
	Prune bool
	CommandOptions
}

// Fetch fetches updates for the repository.
func (r *Repository) Fetch(ctx context.Context, opts ...FetchOptions) error {
	var opt FetchOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"fetch"}
	if opt.Prune {
		args = append(args, "--prune")
	}
	args = append(args, "--end-of-options")

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// PullOptions contains optional arguments for pulling repository updates.
//
// Docs: https://git-scm.com/docs/git-pull
type PullOptions struct {
	// Indicates whether to rebased during pulling.
	Rebase bool
	// Indicates whether to pull from all remotes.
	All bool
	// The remote to pull updates from when All=false.
	Remote string
	// The branch to pull updates from when All=false and Remote is supplied.
	Branch string
	CommandOptions
}

// Pull pulls updates for the repository.
func (r *Repository) Pull(ctx context.Context, opts ...PullOptions) error {
	var opt PullOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"pull"}
	if opt.Rebase {
		args = append(args, "--rebase")
	}
	if opt.All {
		args = append(args, "--all")
	}
	args = append(args, "--end-of-options")
	if !opt.All && opt.Remote != "" {
		args = append(args, opt.Remote)
		if opt.Branch != "" {
			args = append(args, opt.Branch)
		}
	}

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// PushOptions contains optional arguments for pushing repository changes.
//
// Docs: https://git-scm.com/docs/git-push
type PushOptions struct {
	// Indicates whether to set upstream tracking for the branch.
	SetUpstream bool
	CommandOptions
}

// Push pushes local changes to given remote and branch for the repository.
func (r *Repository) Push(ctx context.Context, remote, branch string, opts ...PushOptions) error {
	var opt PushOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"push"}
	if opt.SetUpstream {
		args = append(args, "-u")
	}
	args = append(args, "--end-of-options", remote, branch)
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RepoPush is like Repository.Push for the repository in given path.
func RepoPush(ctx context.Context, repoPath, remote, branch string, opts ...PushOptions) error {
	r, err := Open(repoPath)
	if err != nil {
		return err
	}
	return r.Push(ctx, remote, branch, opts...)
}

// CheckoutOptions contains optional arguments for checking out to a branch.
//
// Docs: https://git-scm.com/docs/git-checkout
type CheckoutOptions struct {
	// The base branch if checks out to a new branch.
	BaseBranch string
	CommandOptions
}

// Checkout checks out to given branch for the repository.
func (r *Repository) Checkout(ctx context.Context, branch string, opts ...CheckoutOptions) error {
	var opt CheckoutOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"checkout"}
	if opt.BaseBranch != "" {
		args = append(args, "-b", branch, "--end-of-options", opt.BaseBranch)
	} else {
		args = append(args, "--end-of-options", branch)
	}

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RepoCheckout is like Repository.Checkout for the repository in given path.
func RepoCheckout(ctx context.Context, repoPath, branch string, opts ...CheckoutOptions) error {
	r, err := Open(repoPath)
	if err != nil {
		return err
	}
	return r.Checkout(ctx, branch, opts...)
}

// ResetOptions contains optional arguments for resetting a branch.
//
// Docs: https://git-scm.com/docs/git-reset
type ResetOptions struct {
	// Indicates whether to perform a hard reset.
	Hard bool
	CommandOptions
}

// Reset resets working tree to given revision for the repository.
func (r *Repository) Reset(ctx context.Context, rev string, opts ...ResetOptions) error {
	var opt ResetOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"reset"}
	if opt.Hard {
		args = append(args, "--hard")
	}
	args = append(args, "--end-of-options", rev)

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// MoveOptions contains optional arguments for moving a file, a directory, or a
// symlink.
//
// Docs: https://git-scm.com/docs/git-mv
type MoveOptions struct {
	CommandOptions
}

// Move moves a file, a directory, or a symlink file or directory from source to
// destination for the repository.
func (r *Repository) Move(ctx context.Context, src, dst string, opts ...MoveOptions) error {
	var opt MoveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"mv", "--end-of-options", src, dst}
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// AddOptions contains optional arguments for adding local changes.
//
// Docs: https://git-scm.com/docs/git-add
type AddOptions struct {
	// Indicates whether to add all changes to index.
	All bool
	// The specific pathspecs to be added to index.
	Pathspecs []string
	CommandOptions
}

// Add adds local changes to index for the repository.
func (r *Repository) Add(ctx context.Context, opts ...AddOptions) error {
	var opt AddOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"add"}
	if opt.All {
		args = append(args, "--all")
	}
	if len(opt.Pathspecs) > 0 {
		args = append(args, "--")
		args = append(args, opt.Pathspecs...)
	}
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// CommitOptions contains optional arguments to commit changes.
//
// Docs: https://git-scm.com/docs/git-commit
type CommitOptions struct {
	// Author is the author of the changes if that's not the same as committer.
	Author *Signature
	CommandOptions
}

// Commit commits local changes with given author, committer and message for the
// repository. The committer may be nil to use the one from Git configuration.
func (r *Repository) Commit(ctx context.Context, committer *Signature, message string, opts ...CommitOptions) error {
	var opt CommitOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	var envs []string
	if committer != nil {
		envs = committerEnvs(committer)
	}
	envs = append(envs, opt.Envs...)

	if opt.Author == nil {
		opt.Author = committer
	}

	args := []string{"commit"}
	if opt.Author != nil {
		args = append(args, fmt.Sprintf("--author=%s <%s>", opt.Author.Name, opt.Author.Email))
	}
	args = append(args, "-m", message)
	args = append(args, "--end-of-options")

	// Git also exits with status 1 for real failures (e.g., an empty message or a
	// rejecting hook), so check for staged changes up front instead of guessing
	// from the exit status.
	_, err := exec(ctx, r.path, []string{"diff", "--cached", "--quiet"}, opt.Envs)
	if err == nil {
		return nil
	} else if !isExitStatus(err, 1) {
		return errors.Wrap(err, "check staged changes")
	}

	_, err = exec(ctx, r.path, args, envs)
	return err
}

// MergeOptions contains optional arguments for merging.
//
// Docs: https://git-scm.com/docs/git-merge
type MergeOptions struct {
	// Indicates whether to create a merge commit even when the merge resolves as a
	// fast-forward.
	NoFastForward bool
	// Indicates whether to stop before creating the merge commit.
	NoCommit bool
	CommandOptions
}

// Merge merges given revision into the current branch of the repository.
func (r *Repository) Merge(ctx context.Context, rev string, opts ...MergeOptions) error {
	var opt MergeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"merge"}
	if opt.NoFastForward {
		args = append(args, "--no-ff")
	}
	if opt.NoCommit {
		args = append(args, "--no-commit")
	}
	args = append(args, "--end-of-options", rev)
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// Rebase reapplies commits of the branch on top of the upstream for the
// repository, leaving the result checked out as a detached HEAD.
//
// Docs: https://git-scm.com/docs/git-rebase
func (r *Repository) Rebase(ctx context.Context, upstream, branch string) error {
	args := []string{"rebase", "--quiet", "--end-of-options", upstream, branch}
	_, err := exec(ctx, r.path, args, nil)
	return err
}

// ApplyCheckOptions contains optional arguments for checking a patch.
//
// Docs: https://git-scm.com/docs/git-apply
type ApplyCheckOptions struct {
	// Indicates whether to ignore whitespace differences in context lines.
	IgnoreWhitespace bool
	CommandOptions
}

// ApplyCheck returns an error if the patch file in given path does not apply
// cleanly to the working tree of the repository.
func (r *Repository) ApplyCheck(ctx context.Context, patchPath string, opts ...ApplyCheckOptions) error {
	var opt ApplyCheckOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"apply", "--check"}
	if opt.IgnoreWhitespace {
		args = append(args, "--ignore-whitespace")
	}
	args = append(args, "--end-of-options", patchPath)
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// NameStatus contains name status of a commit.
type NameStatus struct {
	Added    []string
	Removed  []string
	Modified []string
}

// ShowNameStatusOptions contains optional arguments for showing name status.
//
// Docs: https://git-scm.com/docs/git-show#Documentation/git-show.txt---name-status
type ShowNameStatusOptions struct {
	CommandOptions
}

// ShowNameStatus returns name status of given revision of the repository.
func (r *Repository) ShowNameStatus(ctx context.Context, rev string, opts ...ShowNameStatusOptions) (*NameStatus, error) {
	var opt ShowNameStatusOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	fileStatus := &NameStatus{}
	stdout, w := io.Pipe()
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}

			switch fields[0][0] {
			case 'A':
				fileStatus.Added = append(fileStatus.Added, fields[1])
			case 'D':
				fileStatus.Removed = append(fileStatus.Removed, fields[1])
			case 'M':
				fileStatus.Modified = append(fileStatus.Modified, fields[1])
			}
		}
		done <- struct{}{}
	}()

	args := []string{"show", "--name-status", "--pretty=format:''", "--end-of-options", rev}

	err := pipe(ctx, r.path, args, opt.Envs, w)
	_ = w.Close() // Close writer to exit parsing goroutine
	if err != nil {
		return nil, err
	}

	<-done
	return fileStatus, nil
}

// RepoShowNameStatus is like Repository.ShowNameStatus for the repository in given path.
func RepoShowNameStatus(ctx context.Context, repoPath, rev string, opts ...ShowNameStatusOptions) (*NameStatus, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.ShowNameStatus(ctx, rev, opts...)
}

// RevParseOptions contains optional arguments for parsing revision.
//
// Docs: https://git-scm.com/docs/git-rev-parse
type RevParseOptions struct {
	CommandOptions
}

// RevParse returns full length (40) commit ID by given revision in the
// repository.
func (r *Repository) RevParse(ctx context.Context, rev string, opts ...RevParseOptions) (string, error) {
	var opt RevParseOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// A revision that starts with a dash is never a valid object and would
	// instead be interpreted by "git rev-parse" as a command-line option,
	// allowing argument injection. The "--end-of-options" separator does not
	// help here because "git rev-parse" does not honor it for revisions, so
	// reject such input outright.
	if strings.HasPrefix(rev, "-") {
		return "", ErrRevisionNotExist
	}

	args := []string{"rev-parse", rev}

	commitID, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		if isExitStatus(err, 128) {
			return "", ErrRevisionNotExist
		}
		return "", err
	}
	return strings.TrimSpace(string(commitID)), nil
}

// CountObject contains disk usage report of a repository.
type CountObject struct {
	Count         int64
	Size          int64
	InPack        int64
	Packs         int64
	SizePack      int64
	PrunePackable int64
	Garbage       int64
	SizeGarbage   int64
}

// CountObjectsOptions contains optional arguments for counting objects.
//
// Docs: https://git-scm.com/docs/git-count-objects
type CountObjectsOptions struct {
	CommandOptions
}

// CountObjects returns disk usage report of the repository.
func (r *Repository) CountObjects(ctx context.Context, opts ...CountObjectsOptions) (*CountObject, error) {
	var opt CountObjectsOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"count-objects", "-v", "--end-of-options"}

	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}

	toInt64 := func(b []byte) int64 {
		i, _ := strconv.ParseInt(string(b), 10, 64)
		return i
	}

	countObject := new(CountObject)
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		switch {
		case bytes.HasPrefix(line, []byte("count: ")):
			countObject.Count = toInt64(line[7:])
		case bytes.HasPrefix(line, []byte("size: ")):
			countObject.Size = toInt64(line[6:]) * 1024
		case bytes.HasPrefix(line, []byte("in-pack: ")):
			countObject.InPack = toInt64(line[9:])
		case bytes.HasPrefix(line, []byte("packs: ")):
			countObject.Packs = toInt64(line[7:])
		case bytes.HasPrefix(line, []byte("size-pack: ")):
			countObject.SizePack = toInt64(line[11:]) * 1024
		case bytes.HasPrefix(line, []byte("prune-packable: ")):
			countObject.PrunePackable = toInt64(line[16:])
		case bytes.HasPrefix(line, []byte("garbage: ")):
			countObject.Garbage = toInt64(line[9:])
		case bytes.HasPrefix(line, []byte("size-garbage: ")):
			countObject.SizeGarbage = toInt64(line[14:]) * 1024
		}
	}

	return countObject, nil
}

// RepoCountObjects is like Repository.CountObjects for the repository in given path.
func RepoCountObjects(ctx context.Context, repoPath string, opts ...CountObjectsOptions) (*CountObject, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.CountObjects(ctx, opts...)
}

// FsckOptions contains optional arguments for verifying the objects.
//
// Docs: https://git-scm.com/docs/git-fsck
type FsckOptions struct {
	// Additional arguments to be passed to "git fsck", e.g., "--strict".
	Args []string
	CommandOptions
}

// Fsck verifies the connectivity and validity of the objects in the database
// for the repository.
func (r *Repository) Fsck(ctx context.Context, opts ...FsckOptions) error {
	var opt FsckOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"fsck"}
	args = append(args, opt.Args...)
	args = append(args, "--end-of-options")
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RepoFsck is like Repository.Fsck for the repository in given path.
func RepoFsck(ctx context.Context, repoPath string, opts ...FsckOptions) error {
	r, err := Open(repoPath)
	if err != nil {
		return err
	}
	return r.Fsck(ctx, opts...)
}

// GCOptions contains optional arguments for cleaning up the repository.
//
// Docs: https://git-scm.com/docs/git-gc
type GCOptions struct {
	// Additional arguments to be passed to "git gc", e.g., "--aggressive".
	Args []string
	CommandOptions
}

// GC cleans up unnecessary files and optimizes the repository.
func (r *Repository) GC(ctx context.Context, opts ...GCOptions) error {
	var opt GCOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"gc"}
	args = append(args, opt.Args...)
	args = append(args, "--end-of-options")
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RepoGC is like Repository.GC for the repository in given path.
func RepoGC(ctx context.Context, repoPath string, opts ...GCOptions) error {
	r, err := Open(repoPath)
	if err != nil {
		return err
	}
	return r.GC(ctx, opts...)
}
