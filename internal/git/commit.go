package git

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
)

// Commit contains information of a Git commit.
type Commit struct {
	// The ID of the commit.
	ID Oid
	//  The author of the commit.
	Author *Signature
	// The committer of the commit.
	Committer *Signature
	// The full commit message.
	Message string

	parents []Oid
	*Tree

	submodules    *sync.Map
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

// ParentID returns the ID of the n-th parent (0-based) of this commit.
// It returns an ErrParentNotExist if no such parent exists.
func (c *Commit) ParentID(n int) (Oid, error) {
	if n >= len(c.parents) {
		return nil, ErrParentNotExist
	}
	return c.parents[n], nil
}

// Parent returns the n-th parent commit (0-based) of this commit. It returns
// ErrRevisionNotExist if no such parent exists.
func (c *Commit) Parent(ctx context.Context, n int) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.Parent")
	defer span.End()

	id, err := c.ParentID(n)
	if err != nil {
		return nil, err
	}

	return c.repo.CatFileCommit(ctx, id.String())
}

// CommitByPath returns the commit of the path in the state of this commit.
func (c *Commit) CommitByPath(ctx context.Context, opts ...CommitByRevisionOption) (*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.CommitByPath")
	defer span.End()

	return c.repo.commitByRevision(ctx, c.ID.String(), opts...)
}

// CommitsByPage returns a paginated list of commits in the state of this
// commit. The returned list is in reverse chronological order.
func (c *Commit) CommitsByPage(ctx context.Context, page, size int, opts ...CommitsByPageOption) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.CommitsByPage")
	defer span.End()

	return c.repo.CommitsByPage(ctx, c.ID.String(), page, size, opts...)
}

// SearchCommits searches commit message with given pattern. The returned list
// is in reverse chronological order.
func (c *Commit) SearchCommits(ctx context.Context, pattern string) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.SearchCommits")
	defer span.End()

	return c.repo.SearchCommits(ctx, c.ID.String(), pattern)
}

// ShowNameStatus returns name status of the commit.
func (c *Commit) ShowNameStatus(ctx context.Context) (*NameStatus, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.ShowNameStatus")
	defer span.End()

	return c.repo.ShowNameStatus(ctx, c.ID.String())
}

// CommitsCount returns number of total commits up to this commit.
func (c *Commit) CommitsCount(ctx context.Context) (int64, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.CommitsCount")
	defer span.End()

	return c.repo.RevListCount(ctx, []string{c.ID.String()})
}

// FilesChangedAfter returns a list of files changed after given commit ID.
func (c *Commit) FilesChangedAfter(ctx context.Context, after string) ([]string, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.FilesChangedAfter")
	defer span.End()

	return c.repo.DiffNameOnly(ctx, after, c.ID.String())
}

// CommitsAfter returns a list of commits after given commit ID up to this
// commit. The returned list is in reverse chronological order.
func (c *Commit) CommitsAfter(ctx context.Context, after string) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.CommitsAfter")
	defer span.End()

	return c.repo.RevList(ctx, []string{after + "..." + c.ID.String()})
}

// Ancestors returns a list of ancestors of this commit in reverse chronological
// order.
func (c *Commit) Ancestors(ctx context.Context, opts ...LogOption) ([]*Commit, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.Ancestors")
	defer span.End()

	if c.ParentsCount() == 0 {
		return []*Commit{}, nil
	}

	// Skip the commit itself on top of whatever the caller asks to skip.
	skipSelf := func(opt *logOptions) { opt.Skip++ }
	return c.repo.Log(ctx, c.ID.String(), append(slices.Clip(opts), skipSelf)...)
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
	ctx, span := tracer.Start(ctx, "git.Commit.IsImageFile")
	defer span.End()

	blob, err := c.Blob(ctx, subpath)
	return c.isImageFile(ctx, blob, err)
}

// IsImageFileByIndex returns true if the blob of the commit is an image by
// index.
func (c *Commit) IsImageFileByIndex(ctx context.Context, index string) (bool, error) {
	ctx, span := tracer.Start(ctx, "git.Commit.IsImageFileByIndex")
	defer span.End()

	blob, err := c.blobByIndex(ctx, index)
	return c.isImageFile(ctx, blob, err)
}
