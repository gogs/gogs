package database

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

// newTemporaryRepository creates a bare repository in a temporary directory
// that reads objects from the repositories at given paths, so that commits can
// be built on top of theirs through its index without a working tree. The
// returned function removes the temporary repository.
func newTemporaryRepository(ctx context.Context, repoPaths ...string) (_ *git.Repository, _ func(), err error) {
	parent := filepath.Join(conf.Server.AppDataPath, "tmp", "repos")
	if err = os.MkdirAll(parent, os.ModePerm); err != nil {
		return nil, nil, errors.Wrap(err, "create parent directory")
	}
	dir, err := os.MkdirTemp(parent, "*.git")
	if err != nil {
		return nil, nil, errors.Wrap(err, "create temporary directory")
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	// The object format must match for the objects of the repositories to be
	// readable.
	objectFormat, err := git.RepoObjectFormat(ctx, repoPaths[0])
	if err != nil {
		return nil, nil, errors.Wrap(err, "get object format")
	}
	if err = git.Init(ctx, dir, git.InitOptions{Bare: true, ObjectFormat: objectFormat}); err != nil {
		return nil, nil, errors.Wrap(err, "init")
	}

	alternates := make([]string, 0, len(repoPaths))
	for _, repoPath := range repoPaths {
		objectsPath, err := filepath.Abs(filepath.Join(repoPath, "objects"))
		if err != nil {
			return nil, nil, errors.Wrap(err, "get absolute path")
		}
		alternates = append(alternates, objectsPath+"\n")
	}
	err = os.WriteFile(filepath.Join(dir, "objects", "info", "alternates"), []byte(strings.Join(alternates, "")), 0o644)
	if err != nil {
		return nil, nil, errors.Wrap(err, "write alternates")
	}

	repo, err := git.Open(dir)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open")
	}
	return repo, cleanup, nil
}

// commitAndPush creates a commit on top of the parent, or a root commit when
// the parent is nil, with the changes that change makes to the index of a
// temporary repository, and pushes it to the branch of the repository at
// repoPath with the options, and returns the ID of the pushed
// commit. When the changes leave the tree of the parent as it was, it pushes the
// parent as is. The committer may be nil to use the one from Git configuration.
func commitAndPush(ctx context.Context, repoPath string, parent *git.Commit, branch, message string, author, committer *git.Signature, pushOpts git.PushOptions, change func(tmpRepo *git.Repository) error) (string, error) {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)

	tmpRepo, cleanup, err := newTemporaryRepository(ctx, repoPath)
	if err != nil {
		return "", errors.Wrap(err, "create temporary repository")
	}
	defer cleanup()

	var parents []string
	if parent != nil {
		parents = []string{parent.ID.String()}
		if err = tmpRepo.ReadTree(ctx, parents[0]); err != nil {
			return "", errors.Wrap(err, "read tree")
		}
	}
	if err = change(tmpRepo); err != nil {
		return "", err
	}
	tree, err := tmpRepo.WriteTree(ctx)
	if err != nil {
		return "", errors.Wrap(err, "write tree")
	}

	unchanged := false
	if parent != nil {
		parentTree, err := tmpRepo.RevParse(ctx, parents[0]+"^{tree}")
		if err != nil {
			return "", errors.Wrap(err, "get tree of parent")
		}
		unchanged = tree == parentTree
	}

	var commitID string
	if unchanged {
		commitID = parents[0]
	} else {
		message = git.CleanupMessage(message)
		if message == "" {
			return "", errors.New("empty commit message")
		}
		commitID, err = tmpRepo.CommitTree(ctx, tree, message, git.CommitTreeOptions{
			Parents:   parents,
			Author:    author,
			Committer: committer,
		})
		if err != nil {
			return "", errors.Wrap(err, "commit tree")
		}
	}

	err = tmpRepo.Push(ctx, repoPath, commitID+":"+git.RefsHeads+branch, pushOpts)
	if err != nil {
		return "", errors.Newf("git push origin %s: %v", branch, err)
	}
	return commitID, nil
}

// commitToBranch creates a commit on top of the old branch with the changes
// that change makes to the index of a temporary repository, and pushes it to
// the new branch as the doer, which runs the hooks of the repository.
func (r *Repository) commitToBranch(ctx context.Context, logger *logx.Logger, doer *User, oldBranch, newBranch, message string, change func(tmpRepo *git.Repository, parent *git.Commit) error) error {
	gitRepo, err := git.Open(r.RepoPath())
	if err != nil {
		return errors.Wrap(err, "open repository")
	}
	parent, err := gitRepo.BranchCommit(ctx, oldBranch)
	if err != nil {
		// The first commit of an empty repository has no parent.
		hasBranches, hasErr := gitRepo.HasBranches(ctx)
		if hasErr != nil || hasBranches {
			return errors.Wrapf(err, "get commit of branch %q", oldBranch)
		}
		parent = nil
	}

	// The push is rejected when another one updates the branch, or creates the new
	// branch, in the meantime, so that what is recorded for it is what it changed.
	update := git.RefUpdate{RefName: git.RefsHeads + newBranch}
	if newBranch != oldBranch && !gitRepo.IsValidBranchName(ctx, newBranch) {
		return errors.Newf("invalid branch name %q", newBranch)
	}
	if parent != nil && newBranch == oldBranch {
		update.OldID = parent.ID.String()
	} else {
		format, err := gitRepo.ObjectFormat(ctx)
		if err != nil {
			return errors.Wrap(err, "get object format")
		}
		update.OldID = format.EmptyID()
	}

	sig := &git.Signature{Name: doer.DisplayName(), Email: doer.Email}
	envs := ComposeHookEnvs(ComposeHookEnvsOptions{
		AuthUser:  doer,
		OwnerName: r.MustOwner().Name,
		OwnerSalt: r.MustOwner().Salt,
		RepoID:    r.ID,
		RepoName:  r.Name,
		RepoPath:  r.RepoPath(),
	})
	update.NewID, err = commitAndPush(ctx, r.RepoPath(), parent, newBranch, message, sig, sig, git.PushOptions{Envs: envs, ExpectedOldID: update.OldID}, func(tmpRepo *git.Repository) error {
		return change(tmpRepo, parent)
	})
	if err != nil {
		return err
	}
	if update.NewID != update.OldID {
		// Like the push itself, recording it must not stop midway.
		ctx = context.WithoutCancel(ctx)
		PushUpdates(ctx, logger, doer, r.MustOwner().Name, r.Name, []git.RefUpdate{update}, func(branch string) {
			TriggerPushTasks(logger, doer, r.ID, branch)
		})
	}
	return nil
}

// CreateReference creates the reference, e.g., "refs/heads/feature", at the
// commit by pushing it as the doer, which runs the hooks of the repository, and
// records the push. It returns git.ErrReferenceConflict when the reference
// exists or cannot be created next to another one.
func (r *Repository) CreateReference(ctx context.Context, logger *logx.Logger, doer *User, ref, commitID string) error {
	// Stopping midway, e.g., when the client disconnects, would skip the hooks or
	// recording the push.
	ctx = context.WithoutCancel(ctx)

	format, err := git.RepoObjectFormat(ctx, r.RepoPath())
	if err != nil {
		return errors.Wrap(err, "get object format")
	}
	update := git.RefUpdate{OldID: format.EmptyID(), NewID: commitID, RefName: ref}

	tmpRepo, cleanup, err := newTemporaryRepository(ctx, r.RepoPath())
	if err != nil {
		return errors.Wrap(err, "create temporary repository")
	}
	defer cleanup()

	envs := ComposeHookEnvs(ComposeHookEnvsOptions{
		AuthUser:  doer,
		OwnerName: r.MustOwner().Name,
		OwnerSalt: r.MustOwner().Salt,
		RepoID:    r.ID,
		RepoName:  r.Name,
		RepoPath:  r.RepoPath(),
	})
	err = tmpRepo.Push(ctx, r.RepoPath(), commitID+":"+ref, git.PushOptions{Envs: envs, ExpectedOldID: update.OldID})
	if errors.Is(err, git.ErrReferenceConflict) {
		return err
	} else if err != nil {
		// Git before 2.51 rejects a conflict with the same reason as any other
		// failure to update the reference.
		if gitRepo, openErr := git.Open(r.RepoPath()); openErr == nil {
			if taken, _ := gitRepo.IsReferenceTaken(ctx, ref); taken {
				return git.ErrReferenceConflict
			}
		}
		return errors.Wrapf(err, "push %s", ref)
	}
	PushUpdates(ctx, logger, doer, r.MustOwner().Name, r.Name, []git.RefUpdate{update}, func(branch string) {
		TriggerPushTasks(logger, doer, r.ID, branch)
	})
	return nil
}

// treeEntry returns the entry at the path in the tree of the commit, or nil if
// there is none. A nil commit stands for the empty tree of an empty repository.
func treeEntry(ctx context.Context, commit *git.Commit, path string) (*git.TreeEntry, error) {
	if commit == nil {
		return nil, nil
	}
	entry, err := commit.TreeEntry(ctx, path)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "get tree entry %q", path)
	}
	return entry, nil
}

// hasSymlinkInPath returns true if there is any symlink in the path hierarchy
// in the tree of the commit.
func hasSymlinkInPath(ctx context.Context, commit *git.Commit, treePath string) (bool, error) {
	parts := strings.Split(treePath, "/")
	for i := range parts {
		entry, err := treeEntry(ctx, commit, path.Join(parts[:i+1]...))
		if err != nil {
			return false, err
		} else if entry == nil {
			return false, nil
		} else if entry.IsSymlink() {
			return true, nil
		}
	}
	return false, nil
}

// checkNoSymlinkInPaths returns an error if there is any symlink in the path
// hierarchies in the tree of the commit.
func checkNoSymlinkInPaths(ctx context.Context, commit *git.Commit, treePaths ...string) error {
	for _, treePath := range treePaths {
		// 🚨 SECURITY: Prevent touching files in surprising places, reject operations
		// that involve symlinks.
		has, err := hasSymlinkInPath(ctx, commit, treePath)
		if err != nil {
			return err
		} else if has {
			return errors.Newf("cannot update file with symbolic link in path: %s", treePath)
		}
	}
	return nil
}

type UpdateRepoFileOptions struct {
	OldBranch   string
	NewBranch   string
	OldTreeName string
	NewTreeName string
	Message     string
	Content     string
	IsNewFile   bool
}

// UpdateRepoFile adds or updates a file in repository.
func (r *Repository) UpdateRepoFile(ctx context.Context, logger *logx.Logger, doer *User, opts UpdateRepoFileOptions) error {
	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	if opts.OldBranch != opts.NewBranch {
		// Directly return error if new branch already exists in the server.
		if git.RepoHasBranch(ctx, r.RepoPath(), opts.NewBranch) {
			return BranchAlreadyExists{Name: opts.NewBranch}
		}
	}

	return r.commitToBranch(ctx, logger, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
		if err := checkNoSymlinkInPaths(ctx, parent, opts.OldTreeName, opts.NewTreeName); err != nil {
			return err
		}

		newEntry, err := treeEntry(ctx, parent, opts.NewTreeName)
		if err != nil {
			return err
		}
		// Prompt the user if the meant-to-be new file already exists.
		if newEntry != nil && opts.IsNewFile {
			return ErrRepoFileAlreadyExist{opts.NewTreeName}
		}

		mode := git.EntryBlob
		oldEntry, err := treeEntry(ctx, parent, opts.OldTreeName)
		if err != nil {
			return err
		}
		if oldEntry != nil && (oldEntry.IsBlob() || oldEntry.IsExec()) && opts.OldTreeName != opts.NewTreeName {
			if newEntry != nil {
				return ErrRepoFileAlreadyExist{opts.NewTreeName}
			}
			mode = oldEntry.Mode()
			if err := tmpRepo.RemoveFromIndex(ctx, opts.OldTreeName); err != nil {
				return errors.Wrapf(err, "remove %q", opts.OldTreeName)
			}
		} else if newEntry != nil {
			if !newEntry.IsBlob() && !newEntry.IsExec() {
				return errors.Newf("not a file: %s", opts.NewTreeName)
			}
			mode = newEntry.Mode()
		}

		err = tmpRepo.AddToIndex(ctx, git.IndexFile{
			Path:       opts.NewTreeName,
			Content:    strings.NewReader(opts.Content),
			Executable: mode == git.EntryExec,
		})
		return errors.Wrapf(err, "add %q", opts.NewTreeName)
	})
}

type DeleteRepoFileOptions struct {
	LastCommitID string
	OldBranch    string
	NewBranch    string
	TreePath     string
	Message      string
}

func (r *Repository) DeleteRepoFile(ctx context.Context, logger *logx.Logger, doer *User, opts DeleteRepoFileOptions) (err error) {
	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	return r.commitToBranch(ctx, logger, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
		if err := checkNoSymlinkInPaths(ctx, parent, opts.TreePath); err != nil {
			return err
		}
		if err := tmpRepo.RemoveFromIndex(ctx, opts.TreePath); err != nil {
			return errors.Newf("remove file %q: %v", opts.TreePath, err)
		}
		return nil
	})
}

type UploadRepoFileOptions struct {
	LastCommitID string
	OldBranch    string
	NewBranch    string
	TreePath     string
	Message      string
	Files        []string // In UUID format
}

func (r *Repository) UploadRepoFiles(ctx context.Context, logger *logx.Logger, doer *User, opts UploadRepoFileOptions) error {
	if len(opts.Files) == 0 {
		return nil
	}

	uploads, err := newUploadsStore(Handle.db).ListByUUIDs(ctx, opts.Files)
	if err != nil {
		return errors.Newf("get uploads by UUIDs[%v]: %v", opts.Files, err)
	}

	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	err = r.commitToBranch(ctx, logger, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
		var files []git.IndexFile
		for _, upload := range uploads {
			tmpPath := upload.LocalPath()
			if !osx.IsFile(tmpPath) {
				continue
			}

			targetPath := path.Join(opts.TreePath, upload.Name)
			if err := checkNoSymlinkInPaths(ctx, parent, targetPath); err != nil {
				return err
			}
			entry, err := treeEntry(ctx, parent, targetPath)
			if err != nil {
				return err
			} else if entry != nil && !entry.IsBlob() && !entry.IsExec() {
				return errors.Newf("not a file: %s", targetPath)
			}

			f, err := os.Open(tmpPath)
			if err != nil {
				return errors.Wrapf(err, "open %q", upload.Name)
			}
			defer func() { _ = f.Close() }()
			files = append(files, git.IndexFile{Path: targetPath, Content: f})
		}
		if len(files) == 0 {
			return nil
		}
		return errors.Wrap(tmpRepo.AddToIndex(ctx, files...), "add files")
	})
	if err != nil {
		return err
	}

	return newUploadsStore(Handle.db).Delete(ctx, uploads...)
}
