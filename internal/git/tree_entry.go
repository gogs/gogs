package git

import (
	"context"
	"fmt"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// EntryMode is the unix file mode of a tree entry.
type EntryMode int

// There are only a few file modes in Git. They look like unix file modes, but
// they can only be one of these.
const (
	EntryTree    EntryMode = 0040000
	EntryBlob    EntryMode = 0100644
	EntryExec    EntryMode = 0100755
	EntrySymlink EntryMode = 0120000
	EntryCommit  EntryMode = 0160000
)

type TreeEntry struct {
	mode EntryMode
	typ  ObjectType
	id   *SHA1
	name string

	parent *Tree

	size    int64
	sizeMu  sync.Mutex
	sizeSet bool
}

// Mode returns the entry mode if the tree entry.
func (e *TreeEntry) Mode() EntryMode {
	return e.mode
}

// IsTree returns tree if the entry itself is another tree (i.e. a directory).
func (e *TreeEntry) IsTree() bool {
	return e.mode == EntryTree
}

// IsBlob returns true if the entry is a blob.
func (e *TreeEntry) IsBlob() bool {
	return e.mode == EntryBlob
}

// IsExec returns tree if the entry is an executable.
func (e *TreeEntry) IsExec() bool {
	return e.mode == EntryExec
}

// IsSymlink returns true if the entry is a symbolic link.
func (e *TreeEntry) IsSymlink() bool {
	return e.mode == EntrySymlink
}

// IsCommit returns true if the entry is a commit (i.e. a submodule).
func (e *TreeEntry) IsCommit() bool {
	return e.mode == EntryCommit
}

// Type returns the object type of the entry.
func (e *TreeEntry) Type() ObjectType {
	return e.typ
}

// ID returns the SHA-1 hash of the entry.
func (e *TreeEntry) ID() *SHA1 {
	return e.id
}

// Name returns name of the entry.
func (e *TreeEntry) Name() string {
	return e.name
}

// Size returns the size of the entry. It runs a git command to determine the
// size on first call. Successful results are cached; failed attempts are not
// cached, allowing retries with a fresh context.
func (e *TreeEntry) Size(ctx context.Context) int64 {
	e.sizeMu.Lock()
	defer e.sizeMu.Unlock()

	if e.sizeSet || e.IsTree() {
		return e.size
	}

	stdout, err := exec(ctx, e.parent.repo.path, []string{"cat-file", "-s", e.id.String()}, nil)
	if err != nil {
		return 0
	}
	e.size, _ = strconv.ParseInt(strings.TrimSpace(string(stdout)), 10, 64)
	e.sizeSet = true
	return e.size
}

// Blob returns a blob object from the entry.
func (e *TreeEntry) Blob() *Blob {
	return &Blob{
		TreeEntry: e,
	}
}

// Entries is a sortable list of tree entries.
type Entries []*TreeEntry

var sorters = []func(t1, t2 *TreeEntry) bool{
	func(t1, t2 *TreeEntry) bool {
		return (t1.IsTree() || t1.IsCommit()) && !t2.IsTree() && !t2.IsCommit()
	},
	func(t1, t2 *TreeEntry) bool {
		return t1.name < t2.name
	},
}

func (es Entries) Len() int      { return len(es) }
func (es Entries) Swap(i, j int) { es[i], es[j] = es[j], es[i] }
func (es Entries) Less(i, j int) bool {
	t1, t2 := es[i], es[j]
	var k int
	for k = 0; k < len(sorters)-1; k++ {
		sorter := sorters[k]
		switch {
		case sorter(t1, t2):
			return true
		case sorter(t2, t1):
			return false
		}
	}
	return sorters[k](t1, t2)
}

func (es Entries) Sort() {
	sort.Sort(es)
}

// EntryCommitInfo contains a tree entry with its commit information.
type EntryCommitInfo struct {
	Entry     *TreeEntry
	Index     int
	Commit    *Commit
	Submodule *Submodule
}

// CommitsInfoOptions contains optional arguments for getting commits
// information.
type CommitsInfoOptions struct {
	// The relative path of the repository.
	Path string
	// The maximum number of goroutines to be used for getting commits information.
	// When not set (i.e. <=0), runtime.GOMAXPROCS is used to determine the value.
	MaxConcurrency int
}

var defaultConcurrency = runtime.GOMAXPROCS(0)

// CommitsInfo returns a list of commit information for these tree entries in
// the state of given commit and subpath. It takes advantages of concurrency to
// speed up the process. The returned list has the same number of items as tree
// entries, so the caller can access them via slice indices.
func (es Entries) CommitsInfo(ctx context.Context, commit *Commit, opts ...CommitsInfoOptions) ([]*EntryCommitInfo, error) {
	if len(es) == 0 {
		return []*EntryCommitInfo{}, nil
	}

	var opt CommitsInfoOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.MaxConcurrency <= 0 {
		opt.MaxConcurrency = defaultConcurrency
	}

	// Length of bucket determines how many goroutines (subprocesses) can run at the same time.
	bucket := make(chan struct{}, opt.MaxConcurrency)
	results := make(chan *EntryCommitInfo, len(es))
	errs := make(chan error, 1)

	var errored int64
	hasErrored := func() bool {
		return atomic.LoadInt64(&errored) != 0
	}
	// Only count for the first error, discard the rest
	setError := func(err error) {
		if !atomic.CompareAndSwapInt64(&errored, 0, 1) {
			return
		}
		errs <- err
	}

	var wg sync.WaitGroup
	wg.Add(len(es))
	go func() {
		for i, e := range es {
			// Shrink down the counter and exit when there is an error
			if hasErrored() {
				wg.Add(i - len(es))
				return
			}

			// Block until there is an empty slot to control the maximum concurrency
			bucket <- struct{}{}

			go func(e *TreeEntry, i int) {
				defer func() {
					wg.Done()
					<-bucket
				}()

				// Avoid expensive operations if has errored
				if hasErrored() {
					return
				}

				info := &EntryCommitInfo{
					Entry: e,
					Index: i,
				}
				epath := path.Join(opt.Path, e.Name())

				var err error
				info.Commit, err = commit.CommitByPath(ctx, CommitByRevisionOptions{
					Path: epath,
				})
				if err != nil {
					setError(fmt.Errorf("get commit by path %q: %v", epath, err))
					return
				}

				// Get extra information for submodules
				if e.IsCommit() {
					// Be tolerant to implicit submodules
					info.Submodule, err = commit.Submodule(ctx, epath)
					if err != nil {
						info.Submodule = &Submodule{Name: epath}
					}
				}

				results <- info
			}(e, i)
		}
	}()

	wg.Wait()
	if hasErrored() {
		return nil, <-errs
	}

	close(results)

	commitsInfo := make([]*EntryCommitInfo, len(es))
	for info := range results {
		commitsInfo[info.Index] = info
	}
	return commitsInfo, nil
}
