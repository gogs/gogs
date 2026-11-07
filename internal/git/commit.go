package git

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Commit contains information of a Git commit.
type Commit struct {
	// The SHA-1 hash of the commit.
	ID *SHA1
	//  The author of the commit.
	Author *Signature
	// The committer of the commit.
	Committer *Signature
	// The full commit message.
	Message string

	parents []*SHA1
	*Tree

	submodules    Submodules
	submodulesMu  sync.Mutex
	submodulesSet bool
}

// Summary returns first line of commit message.
func (c *Commit) Summary() string {
	return strings.Split(c.Message, "\n")[0]
}

// ParentsCount returns number of parents of the commit. It returns 0 if this is
// the root commit, otherwise returns 1, 2, etc.
func (c *Commit) ParentsCount() int {
	return len(c.parents)
}

// ParentID returns the SHA-1 hash of the n-th parent (0-based) of this commit.
// It returns an ErrParentNotExist if no such parent exists.
func (c *Commit) ParentID(n int) (*SHA1, error) {
	if n >= len(c.parents) {
		return nil, ErrParentNotExist
	}
	return c.parents[n], nil
}

// Parent returns the n-th parent commit (0-based) of this commit. It returns
// ErrRevisionNotExist if no such parent exists.
func (c *Commit) Parent(ctx context.Context, n int, opts ...CatFileCommitOptions) (*Commit, error) {
	id, err := c.ParentID(n)
	if err != nil {
		return nil, err
	}

	return c.repo.CatFileCommit(ctx, id.String(), opts...)
}

// CommitByPath returns the commit of the path in the state of this commit.
func (c *Commit) CommitByPath(ctx context.Context, opts ...CommitByRevisionOptions) (*Commit, error) {
	return c.repo.CommitByRevision(ctx, c.ID.String(), opts...)
}

// CommitsByPage returns a paginated list of commits in the state of this
// commit. The returned list is in reverse chronological order.
func (c *Commit) CommitsByPage(ctx context.Context, page, size int, opts ...CommitsByPageOptions) ([]*Commit, error) {
	return c.repo.CommitsByPage(ctx, c.ID.String(), page, size, opts...)
}

// SearchCommits searches commit message with given pattern. The returned list
// is in reverse chronological order.
func (c *Commit) SearchCommits(ctx context.Context, pattern string, opts ...SearchCommitsOptions) ([]*Commit, error) {
	return c.repo.SearchCommits(ctx, c.ID.String(), pattern, opts...)
}

// ShowNameStatus returns name status of the commit.
func (c *Commit) ShowNameStatus(ctx context.Context, opts ...ShowNameStatusOptions) (*NameStatus, error) {
	return c.repo.ShowNameStatus(ctx, c.ID.String(), opts...)
}

// CommitsCount returns number of total commits up to this commit.
func (c *Commit) CommitsCount(ctx context.Context, opts ...RevListCountOptions) (int64, error) {
	return c.repo.RevListCount(ctx, []string{c.ID.String()}, opts...)
}

// FilesChangedAfter returns a list of files changed after given commit ID.
func (c *Commit) FilesChangedAfter(ctx context.Context, after string, opts ...DiffNameOnlyOptions) ([]string, error) {
	return c.repo.DiffNameOnly(ctx, after, c.ID.String(), opts...)
}

// CommitsAfter returns a list of commits after given commit ID up to this
// commit. The returned list is in reverse chronological order.
func (c *Commit) CommitsAfter(ctx context.Context, after string, opts ...RevListOptions) ([]*Commit, error) {
	return c.repo.RevList(ctx, []string{after + "..." + c.ID.String()}, opts...)
}

// Ancestors returns a list of ancestors of this commit in reverse chronological
// order.
func (c *Commit) Ancestors(ctx context.Context, opts ...LogOptions) ([]*Commit, error) {
	if c.ParentsCount() == 0 {
		return []*Commit{}, nil
	}

	var opt LogOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	opt.Skip++

	return c.repo.Log(ctx, c.ID.String(), opt)
}

type limitWriter struct {
	W io.Writer
	N int64
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.N <= 0 {
		return len(p), nil
	}

	limit := int64(len(p))
	if limit > w.N {
		limit = w.N
	}
	n, err := w.W.Write(p[:limit])
	w.N -= int64(n)

	// Prevent "short write" error
	return len(p), err
}

func (c *Commit) isImageFile(ctx context.Context, blob *Blob, err error) (bool, error) {
	if err != nil {
		if err == ErrNotBlob {
			return false, nil
		}
		return false, err
	}

	buf := new(bytes.Buffer)
	buf.Grow(512)
	stdout := &limitWriter{
		W: buf,
		N: int64(buf.Cap()),
	}

	err = blob.Pipe(ctx, stdout)
	if err != nil {
		return false, err
	}

	return strings.Contains(http.DetectContentType(buf.Bytes()), "image/"), nil
}

// IsImageFile returns true if the blob of the commit is an image by subpath.
func (c *Commit) IsImageFile(ctx context.Context, subpath string) (bool, error) {
	blob, err := c.Blob(ctx, subpath)
	return c.isImageFile(ctx, blob, err)
}

// IsImageFileByIndex returns true if the blob of the commit is an image by
// index.
func (c *Commit) IsImageFileByIndex(ctx context.Context, index string) (bool, error) {
	blob, err := c.BlobByIndex(ctx, index)
	return c.isImageFile(ctx, blob, err)
}
