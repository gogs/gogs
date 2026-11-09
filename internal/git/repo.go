package git

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Repository contains information of a Git repository.
type Repository struct {
	path string

	cachedCommits sync.Map
	cachedTags    sync.Map
	cachedTrees   sync.Map
}

// Path returns the path of the repository.
func (r *Repository) Path() string {
	return r.path
}

const logFormatHashOnly = `format:%H`

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
	// The hash algorithm of the repository, e.g., "sha1", or empty to use the
	// default.
	ObjectFormat string
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
	if opt.ObjectFormat != "" {
		args = append(args, "--object-format="+opt.ObjectFormat)
	}
	args = append(args, "--end-of-options")
	_, err = exec(ctx, path, args, nil)
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

	return &Repository{path: repoPath}, nil
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
	// The additional environment variables to run the command with.
	Envs []string
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

	args = append(args, "--end-of-options", url, dst)
	_, err = exec(ctx, "", args, opt.Envs)
	return err
}

// PushOptions contains optional arguments for pushing repository changes.
//
// Docs: https://git-scm.com/docs/git-push
type PushOptions struct {
	// The additional environment variables to run the command with.
	Envs []string
}

// Push pushes local changes to given remote and branch for the repository.
func (r *Repository) Push(ctx context.Context, remote, branch string, opts ...PushOptions) error {
	var opt PushOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"push"}
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

// ApplyCheckOptions contains optional arguments for checking a patch.
//
// Docs: https://git-scm.com/docs/git-apply
type ApplyCheckOptions struct {
	// Indicates whether to ignore whitespace differences in context lines.
	IgnoreWhitespace bool
}

// ApplyCheck returns an error if the patch file in given path does not apply
// cleanly to the index of the repository.
func (r *Repository) ApplyCheck(ctx context.Context, patchPath string, opts ...ApplyCheckOptions) error {
	var opt ApplyCheckOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"apply", "--check", "--cached"}
	if opt.IgnoreWhitespace {
		args = append(args, "--ignore-whitespace")
	}
	args = append(args, "--end-of-options", patchPath)
	_, err := exec(ctx, r.path, args, nil)
	return err
}

// NameStatus contains name status of a commit.
type NameStatus struct {
	Added    []string
	Removed  []string
	Modified []string
}

// ShowNameStatus returns name status of given revision of the repository.
func (r *Repository) ShowNameStatus(ctx context.Context, rev string) (*NameStatus, error) {
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

	err := pipe(ctx, r.path, args, nil, w)
	_ = w.Close() // Close writer to exit parsing goroutine
	if err != nil {
		return nil, err
	}

	<-done
	return fileStatus, nil
}

// RepoShowNameStatus is like Repository.ShowNameStatus for the repository in given path.
func RepoShowNameStatus(ctx context.Context, repoPath, rev string) (*NameStatus, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.ShowNameStatus(ctx, rev)
}

// RevParse returns full length (40) commit ID by given revision in the
// repository.
func (r *Repository) RevParse(ctx context.Context, rev string) (string, error) {
	// A revision that starts with a dash is never a valid object and would
	// instead be interpreted by "git rev-parse" as a command-line option,
	// allowing argument injection. The "--end-of-options" separator does not
	// help here because "git rev-parse" does not honor it for revisions, so
	// reject such input outright.
	if strings.HasPrefix(rev, "-") {
		return "", ErrRevisionNotExist
	}

	args := []string{"rev-parse", rev}

	commitID, err := exec(ctx, r.path, args, nil)
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

// CountObjects returns disk usage report of the repository.
func (r *Repository) CountObjects(ctx context.Context) (*CountObject, error) {
	args := []string{"count-objects", "-v", "--end-of-options"}

	stdout, err := exec(ctx, r.path, args, nil)
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
func RepoCountObjects(ctx context.Context, repoPath string) (*CountObject, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.CountObjects(ctx)
}

// FsckOptions contains optional arguments for verifying the objects.
//
// Docs: https://git-scm.com/docs/git-fsck
type FsckOptions struct {
	// Additional arguments to be passed to "git fsck", e.g., "--strict".
	Args []string
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
	_, err := exec(ctx, r.path, args, nil)
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
	_, err := exec(ctx, r.path, args, nil)
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
