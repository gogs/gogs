package database

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/pathx"
)

// BranchAlreadyExists represents an error when branch already exists.
type BranchAlreadyExists struct {
	Name string
}

// IsBranchAlreadyExists returns true if the error is BranchAlreadyExists.
func IsBranchAlreadyExists(err error) bool {
	_, ok := err.(BranchAlreadyExists)
	return ok
}

func (err BranchAlreadyExists) Error() string {
	return fmt.Sprintf("branch already exists [name: %s]", err.Name)
}

const (
	EnvAuthUserID          = "GOGS_AUTH_USER_ID"
	EnvAuthUserName        = "GOGS_AUTH_USER_NAME"
	EnvAuthUserEmail       = "GOGS_AUTH_USER_EMAIL"
	EnvRepoOwnerName       = "GOGS_REPO_OWNER_NAME"
	EnvRepoOwnerSaltMd5    = "GOGS_REPO_OWNER_SALT_MD5"
	EnvRepoID              = "GOGS_REPO_ID"
	EnvRepoName            = "GOGS_REPO_NAME"
	EnvRepoCustomHooksPath = "GOGS_REPO_CUSTOM_HOOKS_PATH"
)

type ComposeHookEnvsOptions struct {
	AuthUser  *User
	OwnerName string
	OwnerSalt string
	RepoID    int64
	RepoName  string
	RepoPath  string
}

func ComposeHookEnvs(opts ComposeHookEnvsOptions) []string {
	envs := []string{
		"SSH_ORIGINAL_COMMAND=1",
		EnvAuthUserID + "=" + strconv.FormatInt(opts.AuthUser.ID, 10),
		EnvAuthUserName + "=" + opts.AuthUser.Name,
		EnvAuthUserEmail + "=" + opts.AuthUser.Email,
		EnvRepoOwnerName + "=" + opts.OwnerName,
		EnvRepoOwnerSaltMd5 + "=" + cryptox.MD5(opts.OwnerSalt),
		EnvRepoID + "=" + strconv.FormatInt(opts.RepoID, 10),
		EnvRepoName + "=" + opts.RepoName,
		EnvRepoCustomHooksPath + "=" + filepath.Join(opts.RepoPath, "custom_hooks"),
	}
	return envs
}

// ___________    .___.__  __    ___________.__.__
// \_   _____/  __| _/|__|/  |_  \_   _____/|__|  |   ____
//  |    __)_  / __ | |  \   __\  |    __)  |  |  | _/ __ \
//  |        \/ /_/ | |  ||  |    |     \   |  |  |_\  ___/
// /_______  /\____ | |__||__|    \___  /   |__|____/\___  >
//         \/      \/                 \/                 \/

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
	sourceRepo, err := git.Open(repoPaths[0])
	if err != nil {
		return nil, nil, errors.Wrap(err, "open repository")
	}
	objectFormat, err := sourceRepo.ObjectFormat(ctx)
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
// repoPath with the environment variables. When the changes leave the tree of
// the parent as it was, it pushes the parent as is. The committer may be nil to
// use the one from Git configuration.
func commitAndPush(ctx context.Context, repoPath string, parent *git.Commit, branch, message string, author, committer *git.Signature, envs []string, change func(tmpRepo *git.Repository) error) error {
	tmpRepo, cleanup, err := newTemporaryRepository(ctx, repoPath)
	if err != nil {
		return errors.Wrap(err, "create temporary repository")
	}
	defer cleanup()

	var parents []string
	if parent != nil {
		parents = []string{parent.ID.String()}
		if err = tmpRepo.ReadTree(ctx, parents[0]); err != nil {
			return errors.Wrap(err, "read tree")
		}
	}
	if err = change(tmpRepo); err != nil {
		return err
	}
	tree, err := tmpRepo.WriteTree(ctx)
	if err != nil {
		return errors.Wrap(err, "write tree")
	}

	unchanged := false
	if parent != nil {
		parentTree, err := tmpRepo.RevParse(ctx, parents[0]+"^{tree}")
		if err != nil {
			return errors.Wrap(err, "get tree of parent")
		}
		unchanged = tree == parentTree
	}

	var commitID string
	if unchanged {
		commitID = parents[0]
	} else {
		message = git.CleanupMessage(message)
		if message == "" {
			return errors.New("empty commit message")
		}
		commitID, err = tmpRepo.CommitTree(ctx, tree, message, git.CommitTreeOptions{
			Parents:   parents,
			Author:    author,
			Committer: committer,
		})
		if err != nil {
			return errors.Wrap(err, "commit tree")
		}
	}

	err = tmpRepo.Push(ctx, repoPath, commitID+":"+git.RefsHeads+branch, git.PushOptions{Envs: envs})
	if err != nil {
		return errors.Newf("git push origin %s: %v", branch, err)
	}
	return nil
}

// commitToBranch creates a commit on top of the old branch with the changes
// that change makes to the index of a temporary repository, and pushes it to
// the new branch as the doer, which runs the hooks of the repository.
func (r *Repository) commitToBranch(ctx context.Context, doer *User, oldBranch, newBranch, message string, change func(tmpRepo *git.Repository, parent *git.Commit) error) error {
	gitRepo, err := git.Open(r.RepoPath())
	if err != nil {
		return errors.Wrap(err, "open repository")
	}
	parent, err := gitRepo.BranchCommit(ctx, oldBranch)
	if err != nil {
		return errors.Wrapf(err, "get commit of branch %q", oldBranch)
	}
	if newBranch != oldBranch && !gitRepo.IsValidBranchName(ctx, newBranch) {
		return errors.Newf("invalid branch name %q", newBranch)
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
	return commitAndPush(ctx, r.RepoPath(), parent, newBranch, message, sig, sig, envs, func(tmpRepo *git.Repository) error {
		return change(tmpRepo, parent)
	})
}

// treeEntry returns the entry at the path in the tree of the commit, or nil if
// there is none.
func treeEntry(ctx context.Context, commit *git.Commit, path string) (*git.TreeEntry, error) {
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
func (r *Repository) UpdateRepoFile(doer *User, opts UpdateRepoFileOptions) error {
	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(opts.NewTreeName) {
		return errors.Errorf("bad tree path %q", opts.NewTreeName)
	}

	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	ctx := context.TODO()
	if opts.OldBranch != opts.NewBranch {
		gitRepo, err := git.Open(r.RepoPath())
		if err != nil {
			return errors.Newf("open repository: %v", err)
		}
		// Directly return error if new branch already exists in the server
		if gitRepo.HasBranch(ctx, opts.NewBranch) {
			return BranchAlreadyExists{Name: opts.NewBranch}
		}
	}

	return r.commitToBranch(ctx, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
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

		blobID, err := tmpRepo.HashObject(ctx, strings.NewReader(opts.Content), git.HashObjectOptions{
			Path:       opts.NewTreeName,
			AttrSource: parent.ID.String(),
		})
		if err != nil {
			return errors.Wrap(err, "write file")
		}
		if err = tmpRepo.AddToIndex(ctx, git.IndexEntry{Mode: mode, ID: blobID, Path: opts.NewTreeName}); err != nil {
			return errors.Wrapf(err, "add %q", opts.NewTreeName)
		}
		return nil
	})
}

// GetDiffPreview produces and returns diff result of a file which is not yet committed.
func (r *Repository) GetDiffPreview(branch, treePath, content string) (*git.Diff, error) {
	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(treePath) {
		return nil, errors.Errorf("bad tree path %q", treePath)
	}

	ctx := context.TODO()
	gitRepo, err := git.Open(r.RepoPath())
	if err != nil {
		return nil, errors.Wrap(err, "open repository")
	}
	commit, err := gitRepo.BranchCommit(ctx, branch)
	if err != nil {
		return nil, errors.Wrapf(err, "get commit of branch %q", branch)
	}
	if err = checkNoSymlinkInPaths(ctx, commit, treePath); err != nil {
		return nil, err
	}

	mode := git.EntryBlob
	entry, err := treeEntry(ctx, commit, treePath)
	if err != nil {
		return nil, err
	} else if entry != nil {
		mode = entry.Mode()
	}

	tmpRepo, cleanup, err := newTemporaryRepository(ctx, r.RepoPath())
	if err != nil {
		return nil, errors.Wrap(err, "create temporary repository")
	}
	defer cleanup()

	if err = tmpRepo.ReadTree(ctx, commit.ID.String()); err != nil {
		return nil, errors.Wrap(err, "read tree")
	}
	blobID, err := tmpRepo.HashObject(ctx, strings.NewReader(content), git.HashObjectOptions{
		Path:       treePath,
		AttrSource: commit.ID.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "write file")
	}
	if err = tmpRepo.AddToIndex(ctx, git.IndexEntry{Mode: mode, ID: blobID, Path: treePath}); err != nil {
		return nil, errors.Wrapf(err, "add %q", treePath)
	}

	diff, err := tmpRepo.DiffCached(ctx, commit.ID.String(), treePath, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars)
	if err != nil {
		return nil, errors.Newf("get diff: %v", err)
	}
	return diff, nil
}

// ________         .__          __           ___________.__.__
// \______ \   ____ |  |   _____/  |_  ____   \_   _____/|__|  |   ____
//  |    |  \_/ __ \|  | _/ __ \   __\/ __ \   |    __)  |  |  | _/ __ \
//  |    `   \  ___/|  |_\  ___/|  | \  ___/   |     \   |  |  |_\  ___/
// /_______  /\___  >____/\___  >__|  \___  >  \___  /   |__|____/\___  >
//         \/     \/          \/          \/       \/                 \/
//

type DeleteRepoFileOptions struct {
	LastCommitID string
	OldBranch    string
	NewBranch    string
	TreePath     string
	Message      string
}

func (r *Repository) DeleteRepoFile(doer *User, opts DeleteRepoFileOptions) (err error) {
	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(opts.TreePath) {
		return errors.Errorf("bad tree path %q", opts.TreePath)
	}

	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	ctx := context.TODO()
	return r.commitToBranch(ctx, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
		if err := checkNoSymlinkInPaths(ctx, parent, opts.TreePath); err != nil {
			return err
		}
		if err := tmpRepo.RemoveFromIndex(ctx, opts.TreePath); err != nil {
			return errors.Newf("remove file %q: %v", opts.TreePath, err)
		}
		return nil
	})
}

//  ____ ___        .__                    .___ ___________.___.__
// |    |   \______ |  |   _________     __| _/ \_   _____/|   |  |   ____   ______
// |    |   /\____ \|  |  /  _ \__  \   / __ |   |    __)  |   |  | _/ __ \ /  ___/
// |    |  / |  |_> >  |_(  <_> ) __ \_/ /_/ |   |     \   |   |  |_\  ___/ \___ \
// |______/  |   __/|____/\____(____  /\____ |   \___  /   |___|____/\___  >____  >
//           |__|                   \/      \/       \/                  \/     \/
//

// Upload represent a uploaded file to a repo to be deleted when moved
type Upload struct {
	ID   int64
	UUID string `xorm:"uuid UNIQUE"`
	Name string
}

// UploadLocalPath returns where uploads is stored in local file system based on given UUID.
func UploadLocalPath(uuid string) string {
	return path.Join(conf.Repository.Upload.TempPath, uuid[0:1], uuid[1:2], uuid)
}

// LocalPath returns where uploads are temporarily stored in local file system.
func (upload *Upload) LocalPath() string {
	return UploadLocalPath(upload.UUID)
}

// NewUpload creates a new upload object.
func NewUpload(name string, buf []byte, file multipart.File) (_ *Upload, err error) {
	// 🚨 SECURITY: Prevent path traversal.
	name = pathx.Clean(name)
	if name == "" {
		return nil, errors.New("empty file name")
	}

	upload := &Upload{
		UUID: uuid.New().String(),
		Name: name,
	}

	localPath := upload.LocalPath()
	if err = os.MkdirAll(path.Dir(localPath), os.ModePerm); err != nil {
		return nil, errors.Newf("mkdir all: %v", err)
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, errors.Newf("create: %v", err)
	}
	defer func() { _ = fw.Close() }()

	if _, err = fw.Write(buf); err != nil {
		return nil, errors.Newf("write: %v", err)
	} else if _, err = io.Copy(fw, file); err != nil {
		return nil, errors.Newf("copy: %v", err)
	}

	if _, err := x.Insert(upload); err != nil {
		return nil, err
	}

	return upload, nil
}

func GetUploadByUUID(uuid string) (*Upload, error) {
	upload := &Upload{UUID: uuid}
	has, err := x.Get(upload)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUploadNotExist{0, uuid}
	}
	return upload, nil
}

func GetUploadsByUUIDs(uuids []string) ([]*Upload, error) {
	if len(uuids) == 0 {
		return []*Upload{}, nil
	}

	// Silently drop invalid uuids.
	uploads := make([]*Upload, 0, len(uuids))
	return uploads, x.In("uuid", uuids).Find(&uploads)
}

func DeleteUploads(uploads ...*Upload) (err error) {
	if len(uploads) == 0 {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	ids := make([]int64, len(uploads))
	for i := range uploads {
		ids[i] = uploads[i].ID
	}
	if _, err = sess.In("id", ids).Delete(new(Upload)); err != nil {
		return errors.Newf("delete uploads: %v", err)
	}

	for _, upload := range uploads {
		localPath := upload.LocalPath()
		if !osx.IsFile(localPath) {
			continue
		}

		if err := os.Remove(localPath); err != nil {
			return errors.Newf("remove upload: %v", err)
		}
	}

	return sess.Commit()
}

func DeleteUpload(u *Upload) error {
	return DeleteUploads(u)
}

func DeleteUploadByUUID(uuid string) error {
	upload, err := GetUploadByUUID(uuid)
	if err != nil {
		if IsErrUploadNotExist(err) {
			return nil
		}
		return errors.Newf("get upload by UUID[%s]: %v", uuid, err)
	}

	if err := DeleteUpload(upload); err != nil {
		return errors.Newf("delete upload: %v", err)
	}

	return nil
}

type UploadRepoFileOptions struct {
	LastCommitID string
	OldBranch    string
	NewBranch    string
	TreePath     string
	Message      string
	Files        []string // In UUID format
}

// isRepositoryGitPath returns true if given path is or resides inside ".git"
// path of the repository.
//
// TODO(unknwon): Move to repox during refactoring for this file.
func isRepositoryGitPath(path string) bool {
	for _, component := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if isDotGit(component) {
			return true
		}
	}
	return false
}

// isDotGit returns true if given path component may refer to ".git" on some file
// system, following is_ntfs_dotgit of Git. Besides case-insensitive matches,
// NTFS resolves the 8.3 short name "git~1", trailing dots and spaces, and
// alternate data streams (e.g., ".git::$INDEX_ALLOCATION") to ".git".
func isDotGit(component string) bool {
	component = strings.ToLower(component)
	var rest string
	switch {
	case strings.HasPrefix(component, ".git"):
		rest = component[len(".git"):]
	case strings.HasPrefix(component, "git~1"):
		rest = component[len("git~1"):]
	default:
		return false
	}
	rest, _, _ = strings.Cut(rest, ":")
	return strings.TrimRight(rest, ". ") == ""
}

func (r *Repository) UploadRepoFiles(doer *User, opts UploadRepoFileOptions) error {
	if len(opts.Files) == 0 {
		return nil
	}

	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(opts.TreePath) {
		return errors.Errorf("bad tree path %q", opts.TreePath)
	}

	uploads, err := GetUploadsByUUIDs(opts.Files)
	if err != nil {
		return errors.Newf("get uploads by UUIDs[%v]: %v", opts.Files, err)
	}

	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	ctx := context.TODO()
	err = r.commitToBranch(ctx, doer, opts.OldBranch, opts.NewBranch, opts.Message, func(tmpRepo *git.Repository, parent *git.Commit) error {
		type file struct{ localPath, treePath string }
		var attributes, others []file
		for _, upload := range uploads {
			tmpPath := upload.LocalPath()
			if !osx.IsFile(tmpPath) {
				continue
			}

			// 🚨 SECURITY: Prevent path traversal.
			upload.Name = pathx.Clean(upload.Name)

			// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
			if isRepositoryGitPath(upload.Name) {
				continue
			}

			targetPath := path.Join(opts.TreePath, upload.Name)
			if err := checkNoSymlinkInPaths(ctx, parent, targetPath); err != nil {
				return err
			}
			if path.Base(targetPath) == ".gitattributes" {
				attributes = append(attributes, file{tmpPath, targetPath})
			} else {
				others = append(others, file{tmpPath, targetPath})
			}
		}

		add := func(files []file, attrSource string) error {
			entries := make([]git.IndexEntry, 0, len(files))
			for _, f := range files {
				blobID, err := hashFile(ctx, tmpRepo, f.localPath, f.treePath, attrSource)
				if err != nil {
					return errors.Wrapf(err, "write file %q", f.treePath)
				}
				entries = append(entries, git.IndexEntry{Mode: git.EntryBlob, ID: blobID, Path: f.treePath})
			}
			if len(entries) == 0 {
				return nil
			}
			return errors.Wrap(tmpRepo.AddToIndex(ctx, entries...), "add files")
		}

		// Uploaded ".gitattributes" files apply to the other uploaded files, as they
		// did when the files were added together in a checkout.
		attrSource := parent.ID.String()
		if len(attributes) > 0 {
			if err := add(attributes, attrSource); err != nil {
				return err
			}
			tree, err := tmpRepo.WriteTree(ctx)
			if err != nil {
				return errors.Wrap(err, "write tree")
			}
			attrSource = tree
		}
		return add(others, attrSource)
	})
	if err != nil {
		return err
	}

	return DeleteUploads(uploads...)
}

// hashFile writes the content of the file at the local path as a blob object
// for the tree path to the repository.
func hashFile(ctx context.Context, repo *git.Repository, localPath, treePath, attrSource string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", errors.Wrap(err, "open")
	}
	defer func() { _ = f.Close() }()
	return repo.HashObject(ctx, f, git.HashObjectOptions{Path: treePath, AttrSource: attrSource})
}
