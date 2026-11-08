package git

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// The functions in this file build commits through the index of the
// repository, so they also work in a bare repository, which has no working
// tree to check files out to.

// ReadTree replaces the index of the repository with the tree of given
// tree-ish.
func (r *Repository) ReadTree(ctx context.Context, treeish string) error {
	_, err := exec(ctx, r.path, []string{"read-tree", "--end-of-options", treeish}, nil)
	return err
}

// HashObjectOptions contains optional arguments for writing a blob object.
//
// Docs: https://git-scm.com/docs/git-hash-object
type HashObjectOptions struct {
	// The path the content is stored at, which decides the filters and
	// end-of-line conversions applied to it.
	Path string
	// The tree-ish to read ".gitattributes" files from, e.g., the commit the
	// content is added on top of.
	AttrSource string
}

// HashObject writes the content as a blob object to the repository and
// returns its ID.
func (r *Repository) HashObject(ctx context.Context, content io.Reader, opts ...HashObjectOptions) (string, error) {
	var opt HashObjectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	var args []string
	if opt.AttrSource != "" {
		args = append(args, "--attr-source="+opt.AttrSource)
	}
	args = append(args, "hash-object", "-w", "--stdin")
	if opt.Path != "" {
		args = append(args, "--path="+opt.Path)
	}
	stdout, err := execInput(ctx, r.path, args, nil, content)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// IndexEntry is an entry of the index.
type IndexEntry struct {
	Mode EntryMode
	ID   string
	Path string
}

// AddToIndex adds the entries to the index of the repository, replacing the
// ones at the same paths. It fails when a path is invalid, e.g., inside ".git",
// or when a path or its parent directory already exists in the index as the
// other kind of a file and a directory.
func (r *Repository) AddToIndex(ctx context.Context, entries ...IndexEntry) error {
	args := []string{"update-index", "--add"}
	for _, e := range entries {
		args = append(args, "--cacheinfo", strconv.FormatInt(int64(e.Mode), 8), e.ID, e.Path)
	}
	_, err := exec(ctx, r.path, args, nil)
	return err
}

// RemoveFromIndex removes the files at given paths from the index of the
// repository. It fails when a path does not exist or is a directory.
func (r *Repository) RemoveFromIndex(ctx context.Context, paths ...string) error {
	args := []string{"--literal-pathspecs", "rm", "--cached", "--quiet", "--"}
	args = append(args, paths...)
	_, err := exec(ctx, r.path, args, nil)
	return err
}

// WriteTree writes the index of the repository as a tree object and returns
// its ID.
func (r *Repository) WriteTree(ctx context.Context) (string, error) {
	stdout, err := exec(ctx, r.path, []string{"write-tree"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// CommitTreeOptions contains optional arguments for creating a commit object.
//
// Docs: https://git-scm.com/docs/git-commit-tree
type CommitTreeOptions struct {
	// The parents of the commit.
	Parents []string
	// The author of the commit, or nil to use the one from Git configuration. The
	// current time is used when its time is zero.
	Author *Signature
	// The committer of the commit, or nil to use the one from Git configuration.
	// The current time is used when its time is zero.
	Committer *Signature
}

// CommitTree creates a commit object of the tree with given message as is, and
// returns its ID.
func (r *Repository) CommitTree(ctx context.Context, tree, message string, opts ...CommitTreeOptions) (string, error) {
	var opt CommitTreeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"commit-tree", "-F", "-"}
	for _, parent := range opt.Parents {
		args = append(args, "-p", parent)
	}
	args = append(args, "--end-of-options", tree)

	var envs []string
	envs = append(envs, signatureEnvs("AUTHOR", opt.Author)...)
	envs = append(envs, signatureEnvs("COMMITTER", opt.Committer)...)

	stdout, err := execInput(ctx, r.path, args, envs, strings.NewReader(message))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// signatureEnvs returns the environment variables that make Git use the
// signature as the given role, i.e., "AUTHOR" or "COMMITTER".
func signatureEnvs(role string, sig *Signature) []string {
	if sig == nil {
		return nil
	}
	envs := []string{
		"GIT_" + role + "_NAME=" + sig.Name,
		"GIT_" + role + "_EMAIL=" + sig.Email,
	}
	if !sig.When.IsZero() {
		envs = append(envs, fmt.Sprintf("GIT_%s_DATE=@%d %s", role, sig.When.Unix(), sig.When.Format("-0700")))
	}
	return envs
}

// CleanupMessage cleans up the commit message the same way "git commit" does
// for a message given on the command line: it removes trailing whitespace of
// every line, leading and trailing empty lines, and collapses consecutive
// empty lines into one.
func CleanupMessage(message string) string {
	var b strings.Builder
	empties := 0
	for line := range strings.Lines(message) {
		line = strings.TrimRight(line, " \t\n\r")
		if line == "" {
			empties++
			continue
		}
		if b.Len() > 0 && empties > 0 {
			b.WriteByte('\n')
		}
		empties = 0
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// MergeTreeOptions contains optional arguments for merging trees.
//
// Docs: https://git-scm.com/docs/git-merge-tree
type MergeTreeOptions struct {
	// The commit to use as the merge base instead of finding one.
	MergeBase string
}

// MergeTree merges the two commits the same way "git merge" does but only
// writes the tree of the result, whose ID it returns. It returns
// ErrMergeConflict when the merge has conflicts.
func (r *Repository) MergeTree(ctx context.Context, ours, theirs string, opts ...MergeTreeOptions) (string, error) {
	var opt MergeTreeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"merge-tree", "--write-tree", "--no-messages", "--name-only"}
	if opt.MergeBase != "" {
		args = append(args, "--merge-base="+opt.MergeBase)
	}
	args = append(args, "--end-of-options", ours, theirs)
	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", ErrMergeConflict
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// IsAncestor returns true if the commit is an ancestor of the descendant, or
// the same commit.
func (r *Repository) IsAncestor(ctx context.Context, commit, descendant string) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", "--end-of-options", commit, descendant}
	_, err := exec(ctx, r.path, args, nil)
	if err != nil {
		if isExitStatus(err, 1) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// emptyTreeID is the ID of the tree with no entries, which every repository has.
const emptyTreeID = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Rebase reapplies the commits of the branch that are not in the upstream on
// top of the upstream, the same way "git rebase <upstream> <branch>" does, and
// returns the ID of the resulting commit. Like "git rebase", it keeps commits
// that are already based on the upstream, drops merge commits and commits whose
// changes are already in the upstream, and takes the committer from Git
// configuration. It returns ErrMergeConflict when a commit does not apply.
func (r *Repository) Rebase(ctx context.Context, upstream, branch string) (string, error) {
	head, err := r.RevParse(ctx, upstream)
	if err != nil {
		return "", errors.Wrap(err, "get upstream commit")
	}

	args := []string{"rev-list", "--reverse", "--topo-order", "--right-only", "--cherry-pick", "--no-merges", "--end-of-options", head + "..." + branch, "--"}
	stdout, err := exec(ctx, r.path, args, nil)
	if err != nil {
		return "", errors.Wrap(err, "list commits")
	}

	var emptyCommit string
	for _, id := range strings.Fields(string(stdout)) {
		c, err := r.rawCommit(ctx, id)
		if err != nil {
			return "", errors.Wrapf(err, "read commit %s", id)
		}

		if c.parent == head {
			head = id
			continue
		}

		// A root commit adds everything it has, i.e., it is a change from an empty
		// tree.
		mergeBase := c.parent
		if mergeBase == "" {
			if emptyCommit == "" {
				emptyCommit, err = r.CommitTree(ctx, emptyTreeID, "")
				if err != nil {
					return "", errors.Wrap(err, "create empty commit")
				}
			}
			mergeBase = emptyCommit
		}

		tree, err := r.MergeTree(ctx, head, id, MergeTreeOptions{MergeBase: mergeBase})
		if err != nil {
			return "", errors.Wrapf(err, "apply commit %s", id)
		}

		// Drop the commit when its changes are already there, unless it had no
		// changes to begin with.
		headTree, err := r.RevParse(ctx, head+"^{tree}")
		if err != nil {
			return "", errors.Wrap(err, "get tree")
		}
		if tree == headTree {
			parentTree := emptyTreeID
			if c.parent != "" {
				parentTree, err = r.RevParse(ctx, c.parent+"^{tree}")
				if err != nil {
					return "", errors.Wrap(err, "get tree")
				}
			}
			if c.tree != parentTree {
				continue
			}
		}

		head, err = r.CommitTree(ctx, tree, skipBlankLines(c.message), CommitTreeOptions{
			Parents: []string{head},
			Author:  c.author,
		})
		if err != nil {
			return "", errors.Wrapf(err, "commit %s", id)
		}
	}
	return head, nil
}

// skipBlankLines returns the message without its leading lines that are empty
// or only have whitespace, which "git rebase" leaves out.
func skipBlankLines(message string) string {
	for message != "" {
		line, rest, _ := strings.Cut(message, "\n")
		if strings.TrimRight(line, " \t\r") != "" {
			break
		}
		message = rest
	}
	return message
}

// rawCommitInfo is what Rebase needs to know about a commit, with the author's
// time in the time zone it was recorded in.
type rawCommitInfo struct {
	tree    string
	parent  string // The first parent.
	author  *Signature
	message string
}

func (r *Repository) rawCommit(ctx context.Context, id string) (*rawCommitInfo, error) {
	stdout, err := exec(ctx, r.path, []string{"cat-file", "commit", id}, nil)
	if err != nil {
		return nil, err
	}

	header, message, _ := strings.Cut(string(stdout), "\n\n")
	c := &rawCommitInfo{message: message}
	for line := range strings.SplitSeq(header, "\n") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "tree":
			c.tree = value
		case "parent":
			if c.parent == "" {
				c.parent = value
			}
		case "author":
			c.author, err = parseRawSignature(value)
			if err != nil {
				return nil, errors.Wrap(err, "parse author")
			}
		}
	}
	return c, nil
}

// parseRawSignature parses a signature in the "Name <email> 1700000000 +0100"
// format of commit objects, keeping its time zone.
func parseRawSignature(s string) (*Signature, error) {
	emailEnd := strings.LastIndexByte(s, '>')
	emailStart := strings.LastIndexByte(s[:max(emailEnd, 0)], '<')
	if emailStart < 0 || emailEnd < 0 {
		return nil, errors.Errorf("malformed signature %q", s)
	}

	fields := strings.Fields(s[emailEnd+1:])
	if len(fields) != 2 || len(fields[1]) != 5 {
		return nil, errors.Errorf("malformed signature time %q", s)
	}
	seconds, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "parse time")
	}
	zone, err := time.Parse("-0700", fields[1])
	if err != nil {
		return nil, errors.Wrap(err, "parse time zone")
	}

	return &Signature{
		Name:  strings.TrimSuffix(s[:emailStart], " "),
		Email: s[emailStart+1 : emailEnd],
		When:  time.Unix(seconds, 0).In(zone.Location()),
	}, nil
}
