package git

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// parseCommit parses commit information from the (uncompressed) raw data of the
// commit object. It assumes "\n\n" separates the header from the rest of the
// message.
func parseCommit(data []byte) (*Commit, error) {
	commit := new(Commit)
	// we now have the contents of the commit object. Let's investigate.
	nextline := 0
loop:
	for {
		eol := bytes.IndexByte(data[nextline:], '\n')
		switch {
		case eol > 0:
			line := data[nextline : nextline+eol]
			spacepos := bytes.IndexByte(line, ' ')
			reftype := line[:spacepos]
			switch string(reftype) {
			case "tree", "object":
				id, err := newIDFromString(string(line[spacepos+1:]))
				if err != nil {
					return nil, err
				}
				commit.Tree = &Tree{id: id}
			case "parent":
				// A commit can have one or more parents
				id, err := newIDFromString(string(line[spacepos+1:]))
				if err != nil {
					return nil, err
				}
				commit.parents = append(commit.parents, id)
			case "author", "tagger":
				sig, err := parseSignature(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Author = sig
			case "committer":
				sig, err := parseSignature(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Committer = sig
			}
			nextline += eol + 1
		case eol == 0:
			commit.Message = string(data[nextline+1:])
			break loop
		default:
			break loop
		}
	}
	return commit, nil
}

// CatFileCommit returns the commit corresponding to the given revision of the
// repository. The revision could be a commit ID or full refspec (e.g.,
// "refs/heads/master").
func (r *Repository) CatFileCommit(ctx context.Context, rev string) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.CatFileCommit")
	defer span.End()

	cache, ok := r.cachedCommits.Load(rev)
	if ok {
		logf("Cached commit hit: %s", rev)
		return cache.(*Commit), nil
	}

	commitID, err := r.RevParse(ctx, rev) //nolint
	if err != nil {
		return nil, err
	}

	args := []string{"cat-file", "commit", commitID}
	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		// "git rev-parse" passes a full-length object ID through without checking
		// that the object exists.
		if isExitStatus(err, 128) {
			return nil, ErrRevisionNotExist
		}
		return nil, err
	}

	c, err := parseCommit(stdout)
	if err != nil {
		return nil, err
	}
	c.repo = r
	c.ID = MustIDFromString(commitID)

	r.cachedCommits.Store(commitID, c)
	return c, nil
}

// RepoCatFileCommit is like Repository.CatFileCommit for the repository in given path.
func RepoCatFileCommit(ctx context.Context, repoPath, rev string) (*Commit, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.CatFileCommit(ctx, rev)
}

// catFileType returns the object type of given revision of the repository.
func (r *Repository) catFileType(ctx context.Context, rev string) (ObjectType, error) {
	args := []string{"cat-file", "-t", rev}
	typ, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return "", err
	}
	typ = bytes.TrimSpace(typ)
	return ObjectType(typ), nil
}

// BranchCommit returns the latest commit of given branch of the repository. The
// branch must be given in short name, e.g., "master".
func (r *Repository) BranchCommit(ctx context.Context, branch string) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.BranchCommit")
	defer span.End()

	return r.CatFileCommit(ctx, RefsHeads+branch)
}

// RepoBranchCommit is like Repository.BranchCommit for the repository in given path.
func RepoBranchCommit(ctx context.Context, repoPath, branch string) (*Commit, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.BranchCommit(ctx, branch)
}

// TagCommit returns the latest commit of given tag of the repository. The tag
// must be given in short name, e.g., "v1.0.0".
func (r *Repository) TagCommit(ctx context.Context, tag string) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.TagCommit")
	defer span.End()

	return r.CatFileCommit(ctx, RefsTags+tag)
}

// RepoTagCommit is like Repository.TagCommit for the repository in given path.
func RepoTagCommit(ctx context.Context, repoPath, tag string) (*Commit, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.TagCommit(ctx, tag)
}

// LogOptions contains optional arguments for listing commits.
//
// Docs: https://git-scm.com/docs/git-log
type LogOptions struct {
	// The maximum number of commits to output.
	MaxCount int
	// The number commits skipped before starting to show the commit output.
	Skip int
	// The regular expression to filter commits by their messages.
	GrepPattern string
	// Indicates whether to ignore letter case when match the regular expression.
	RegexpIgnoreCase bool
	// The relative path of the repository.
	Path string
}

func escapePath(path string) string {
	if len(path) == 0 {
		return path
	}

	// Path starts with ':' must be escaped.
	if path[0] == ':' {
		path = `\` + path
	}
	return path
}

// Log returns a list of commits in the state of given revision of the repository.
// The returned list is in reverse chronological order.
func (r *Repository) Log(ctx context.Context, rev string, opts ...LogOptions) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.Log")
	defer span.End()

	var opt LogOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"log", "--pretty=" + logFormatHashOnly}
	if opt.MaxCount > 0 {
		args = append(args, "--max-count="+strconv.Itoa(opt.MaxCount))
	}
	if opt.Skip > 0 {
		args = append(args, "--skip="+strconv.Itoa(opt.Skip))
	}
	if opt.GrepPattern != "" {
		args = append(args, "--grep="+opt.GrepPattern)
	}
	if opt.RegexpIgnoreCase {
		args = append(args, "--regexp-ignore-case")
	}
	args = append(args, "--end-of-options", rev, "--")
	if opt.Path != "" {
		args = append(args, escapePath(opt.Path))
	}

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return nil, err
	}
	return r.parsePrettyFormatLogToList(ctx, stdout)
}

// RepoLog is like Repository.Log for the repository in given path.
func RepoLog(ctx context.Context, repoPath, rev string, opts ...LogOptions) ([]*Commit, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.Log(ctx, rev, opts...)
}

// CommitByRevisionOptions contains optional arguments for getting a commit.
//
// Docs: https://git-scm.com/docs/git-log
type CommitByRevisionOptions struct {
	// The relative path of the repository.
	Path string
}

// commitByRevision returns a commit by given revision.
func (r *Repository) commitByRevision(ctx context.Context, rev string, opts ...CommitByRevisionOptions) (*Commit, error) {
	var opt CommitByRevisionOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	commits, err := r.Log(ctx, rev, LogOptions{
		MaxCount: 1,
		Path:     opt.Path,
	})
	if err != nil {
		if strings.Contains(err.Error(), "bad revision") {
			return nil, ErrRevisionNotExist
		}
		return nil, err
	} else if len(commits) == 0 {
		return nil, ErrRevisionNotExist
	}
	return commits[0], nil
}

// CommitsByPageOptions contains optional arguments for getting paginated
// commits.
//
// Docs: https://git-scm.com/docs/git-log
type CommitsByPageOptions struct {
	// The relative path of the repository.
	Path string
}

// CommitsByPage returns a paginated list of commits in the state of given
// revision. The pagination starts from the newest to the oldest commit.
func (r *Repository) CommitsByPage(ctx context.Context, rev string, page, size int, opts ...CommitsByPageOptions) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.CommitsByPage")
	defer span.End()

	var opt CommitsByPageOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return r.Log(ctx, rev, LogOptions{
		MaxCount: size,
		Skip:     (page - 1) * size,
		Path:     opt.Path,
	})
}

// SearchCommits searches commit message with given pattern in the state of
// given revision. The returned list is in reverse chronological order.
func (r *Repository) SearchCommits(ctx context.Context, rev, pattern string) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.SearchCommits")
	defer span.End()

	return r.Log(ctx, rev, LogOptions{
		GrepPattern:      pattern,
		RegexpIgnoreCase: true,
	})
}

// DiffNameOnlyOptions contains optional arguments for listing changed files.
//
// Docs: https://git-scm.com/docs/git-diff#Documentation/git-diff.txt---name-only
type DiffNameOnlyOptions struct {
	// Indicates whether two commits should have a merge base.
	NeedsMergeBase bool
}

// DiffNameOnly returns a list of changed files between base and head revisions of the
// repository.
func (r *Repository) DiffNameOnly(ctx context.Context, base, head string, opts ...DiffNameOnlyOptions) ([]string, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.DiffNameOnly")
	defer span.End()

	var opt DiffNameOnlyOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"diff", "--name-only", "--end-of-options"}
	if opt.NeedsMergeBase {
		args = append(args, base+"..."+head)
	} else {
		args = append(args, base, head)
	}
	args = append(args, "--")

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(stdout, []byte("\n"))
	names := make([]string, 0, len(lines)-1)
	for i := range lines {
		if len(lines[i]) == 0 {
			continue
		}

		names = append(names, string(lines[i]))
	}
	return names, nil
}

// RevListCount returns number of total commits up to given refspec of the
// repository.
func (r *Repository) RevListCount(ctx context.Context, refspecs []string) (int64, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.RevListCount")
	defer span.End()

	if len(refspecs) == 0 {
		return 0, errors.New("must have at least one refspec")
	}

	args := []string{"rev-list", "--count", "--end-of-options"}
	args = append(args, refspecs...)
	args = append(args, "--")

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(strings.TrimSpace(string(stdout)), 10, 64)
}

// RepoRevListCount is like Repository.RevListCount for the repository in given path.
func RepoRevListCount(ctx context.Context, repoPath string, refspecs []string) (int64, error) {
	r, err := Open(repoPath)
	if err != nil {
		return 0, err
	}
	return r.RevListCount(ctx, refspecs)
}

// RevList returns a list of commits based on given refspecs in reverse
// chronological order.
func (r *Repository) RevList(ctx context.Context, refspecs []string) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.RevList")
	defer span.End()

	if len(refspecs) == 0 {
		return nil, errors.New("must have at least one refspec")
	}

	args := []string{"rev-list", "--end-of-options"}
	args = append(args, refspecs...)
	args = append(args, "--")

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return nil, err
	}
	return r.parsePrettyFormatLogToList(ctx, bytes.TrimSpace(stdout))
}

// LatestCommitTime returns the time of latest commit of the repository.
func (r *Repository) LatestCommitTime(ctx context.Context) (time.Time, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.LatestCommitTime")
	defer span.End()

	args := []string{"for-each-ref", "--count=1", "--sort=-committerdate", "--format=%(committerdate:iso8601)", "--end-of-options"}

	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(stdout)))
}
