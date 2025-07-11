// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	gouuid "github.com/satori/go.uuid"
	"github.com/unknwon/com"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptoutil"
	dberrors "gogs.io/gogs/internal/database/errors"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/pathutil"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/tool"
)

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
		EnvAuthUserID + "=" + com.ToStr(opts.AuthUser.ID),
		EnvAuthUserName + "=" + opts.AuthUser.Name,
		EnvAuthUserEmail + "=" + opts.AuthUser.Email,
		EnvRepoOwnerName + "=" + opts.OwnerName,
		EnvRepoOwnerSaltMd5 + "=" + cryptoutil.MD5(opts.OwnerSalt),
		EnvRepoID + "=" + com.ToStr(opts.RepoID),
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

// discardLocalRepoBranchChanges discards local commits/changes of
// given branch to make sure it is even to remote branch.
func discardLocalRepoBranchChanges(localPath, branch string) error {
	if !com.IsExist(localPath) {
		return nil
	}

	// No need to check if nothing in the repository.
	if !git.RepoHasBranch(localPath, branch) {
		return nil
	}

	rev := "origin/" + branch
	if err := git.Reset(localPath, rev, git.ResetOptions{Hard: true}); err != nil {
		return fmt.Errorf("reset [revision: %s]: %v", rev, err)
	}
	return nil
}

func (r *Repository) DiscardLocalRepoBranchChanges(branch string) error {
	return discardLocalRepoBranchChanges(r.LocalCopyPath(), branch)
}

// CheckoutNewBranch checks out to a new branch from the a branch name.
func (r *Repository) CheckoutNewBranch(oldBranch, newBranch string) error {
	if err := git.Checkout(r.LocalCopyPath(), newBranch, git.CheckoutOptions{
		BaseBranch: oldBranch,
		Timeout:    time.Duration(conf.Git.Timeout.Pull) * time.Second,
	}); err != nil {
		return fmt.Errorf("checkout [base: %s, new: %s]: %v", oldBranch, newBranch, err)
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
func (r *Repository) UpdateRepoFile(doer *User, opts UpdateRepoFileOptions) (err error) {
	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(opts.NewTreeName) {
		return errors.Errorf("bad tree path %q", opts.NewTreeName)
	}

	repoWorkingPool.CheckIn(com.ToStr(r.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(r.ID))

	if err = r.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local r branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = r.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	repoPath := r.RepoPath()
	localPath := r.LocalCopyPath()

	if opts.OldBranch != opts.NewBranch {
		// Directly return error if new branch already exists in the server
		if git.RepoHasBranch(repoPath, opts.NewBranch) {
			return dberrors.BranchAlreadyExists{Name: opts.NewBranch}
		}

		// Otherwise, delete branch from local copy in case out of sync
		if git.RepoHasBranch(localPath, opts.NewBranch) {
			if err = git.DeleteBranch(localPath, opts.NewBranch, git.DeleteBranchOptions{
				Force: true,
			}); err != nil {
				return fmt.Errorf("delete branch %q: %v", opts.NewBranch, err)
			}
		}

		if err := r.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("checkout new branch[%s] from old branch[%s]: %v", opts.NewBranch, opts.OldBranch, err)
		}
	}

	oldFilePath := path.Join(localPath, opts.OldTreeName)
	filePath := path.Join(localPath, opts.NewTreeName)
	if err = os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	// If it's meant to be a new file, make sure it doesn't exist.
	if opts.IsNewFile {
		// 🚨 SECURITY: Prevent updating files in surprising place, check if the file is
		// a symlink.
		if osutil.IsSymlink(filePath) {
			return fmt.Errorf("cannot update symbolic link: %s", opts.NewTreeName)
		}
		if osutil.IsExist(filePath) {
			return ErrRepoFileAlreadyExist{filePath}
		}
	}

	// Ignore move step if it's a new file under a directory.
	// Otherwise, move the file when name changed.
	if osutil.IsFile(oldFilePath) && opts.OldTreeName != opts.NewTreeName {
		// 🚨 SECURITY: Prevent updating files in surprising place, check if the file is
		// a symlink.
		if osutil.IsSymlink(oldFilePath) {
			return fmt.Errorf("cannot move symbolic link: %s", opts.OldTreeName)
		}

		if err = git.Move(localPath, opts.OldTreeName, opts.NewTreeName); err != nil {
			return fmt.Errorf("git mv %q %q: %v", opts.OldTreeName, opts.NewTreeName, err)
		}
	}

	if err = os.WriteFile(filePath, []byte(opts.Content), 0600); err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	}

	err = git.CreateCommit(
		localPath,
		&git.Signature{
			Name:  doer.DisplayName(),
			Email: doer.Email,
			When:  time.Now(),
		},
		opts.Message,
	)
	if err != nil {
		return fmt.Errorf("commit changes on %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: r.MustOwner().Name,
					OwnerSalt: r.MustOwner().Salt,
					RepoID:    r.ID,
					RepoName:  r.Name,
					RepoPath:  r.RepoPath(),
				}),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.NewBranch, err)
	}
	return nil
}

// GetDiffPreview produces and returns diff result of a file which is not yet committed.
func (r *Repository) GetDiffPreview(branch, treePath, content string) (diff *gitutil.Diff, err error) {
	// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
	if isRepositoryGitPath(treePath) {
		return nil, errors.Errorf("bad tree path %q", treePath)
	}

	repoWorkingPool.CheckIn(com.ToStr(r.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(r.ID))

	if err = r.DiscardLocalRepoBranchChanges(branch); err != nil {
		return nil, fmt.Errorf("discard local r branch[%s] changes: %v", branch, err)
	} else if err = r.UpdateLocalCopyBranch(branch); err != nil {
		return nil, fmt.Errorf("update local copy branch[%s]: %v", branch, err)
	}

	localPath := r.LocalCopyPath()
	filePath := path.Join(localPath, treePath)

	// 🚨 SECURITY: Prevent updating files in surprising place, check if the target is
	// a symlink.
	if osutil.IsSymlink(filePath) {
		return nil, fmt.Errorf("cannot get diff preview for symbolic link: %s", treePath)
	}
	if err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return nil, err
	} else if err = os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return nil, fmt.Errorf("write file: %v", err)
	}

	// 🚨 SECURITY: Prevent including unintended options in the path to the Git command.
	cmd := exec.Command("git", "diff", "--end-of-options", treePath)
	cmd.Dir = localPath
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdout pipe: %v", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %v", err)
	}

	pid := process.Add(fmt.Sprintf("GetDiffPreview [repo_path: %s]", r.RepoPath()), cmd)
	defer process.Remove(pid)

	diff, err = gitutil.ParseDiff(stdout, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %v", err)
	}

	if err = cmd.Wait(); err != nil {
		return nil, fmt.Errorf("wait: %v", err)
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

	repoWorkingPool.CheckIn(com.ToStr(r.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(r.ID))

	if err = r.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local r branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = r.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	if opts.OldBranch != opts.NewBranch {
		if err := r.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("checkout new branch[%s] from old branch[%s]: %v", opts.NewBranch, opts.OldBranch, err)
		}
	}

	localPath := r.LocalCopyPath()
	filePath := path.Join(localPath, opts.TreePath)

	// 🚨 SECURITY: Prevent updating files in surprising place, check if the file is
	// a symlink.
	if osutil.IsSymlink(filePath) {
		return fmt.Errorf("cannot delete symbolic link: %s", opts.TreePath)
	}

	if err = os.Remove(filePath); err != nil {
		return fmt.Errorf("remove file %q: %v", opts.TreePath, err)
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	}

	err = git.CreateCommit(
		localPath,
		&git.Signature{
			Name:  doer.DisplayName(),
			Email: doer.Email,
			When:  time.Now(),
		},
		opts.Message,
	)
	if err != nil {
		return fmt.Errorf("commit changes to %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: r.MustOwner().Name,
					OwnerSalt: r.MustOwner().Salt,
					RepoID:    r.ID,
					RepoName:  r.Name,
					RepoPath:  r.RepoPath(),
				}),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.NewBranch, err)
	}
	return nil
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
	if tool.IsMaliciousPath(name) {
		return nil, fmt.Errorf("malicious path detected: %s", name)
	}

	upload := &Upload{
		UUID: gouuid.NewV4().String(),
		Name: name,
	}

	localPath := upload.LocalPath()
	if err = os.MkdirAll(path.Dir(localPath), os.ModePerm); err != nil {
		return nil, fmt.Errorf("mkdir all: %v", err)
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, fmt.Errorf("create: %v", err)
	}
	defer func() { _ = fw.Close() }()

	if _, err = fw.Write(buf); err != nil {
		return nil, fmt.Errorf("write: %v", err)
	} else if _, err = io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("copy: %v", err)
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
	for i := 0; i < len(uploads); i++ {
		ids[i] = uploads[i].ID
	}
	if _, err = sess.In("id", ids).Delete(new(Upload)); err != nil {
		return fmt.Errorf("delete uploads: %v", err)
	}

	for _, upload := range uploads {
		localPath := upload.LocalPath()
		if !osutil.IsFile(localPath) {
			continue
		}

		if err := os.Remove(localPath); err != nil {
			return fmt.Errorf("remove upload: %v", err)
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
		return fmt.Errorf("get upload by UUID[%s]: %v", uuid, err)
	}

	if err := DeleteUpload(upload); err != nil {
		return fmt.Errorf("delete upload: %v", err)
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
// TODO(unknwon): Move to repoutil during refactoring for this file.
func isRepositoryGitPath(path string) bool {
	path = strings.ToLower(path)
	return strings.HasSuffix(path, ".git") ||
		strings.Contains(path, ".git/") ||
		strings.Contains(path, `.git\`) ||
		// Windows treats ".git." the same as ".git"
		strings.HasSuffix(path, ".git.") ||
		strings.Contains(path, ".git./") ||
		strings.Contains(path, `.git.\`)
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
		return fmt.Errorf("get uploads by UUIDs[%v]: %v", opts.Files, err)
	}

	repoWorkingPool.CheckIn(com.ToStr(r.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(r.ID))

	if err = r.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local r branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = r.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	if opts.OldBranch != opts.NewBranch {
		if err = r.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("checkout new branch[%s] from old branch[%s]: %v", opts.NewBranch, opts.OldBranch, err)
		}
	}

	localPath := r.LocalCopyPath()
	dirPath := path.Join(localPath, opts.TreePath)
	if err = os.MkdirAll(dirPath, os.ModePerm); err != nil {
		return err
	}

	// Copy uploaded files into repository
	for _, upload := range uploads {
		tmpPath := upload.LocalPath()
		if !osutil.IsFile(tmpPath) {
			continue
		}

		// 🚨 SECURITY: Prevent path traversal.
		upload.Name = pathutil.Clean(upload.Name)

		// 🚨 SECURITY: Prevent uploading files into the ".git" directory.
		if isRepositoryGitPath(upload.Name) {
			continue
		}

		targetPath := path.Join(dirPath, upload.Name)

		// 🚨 SECURITY: Prevent updating files in surprising place, check if the target
		// is a symlink.
		if osutil.IsSymlink(targetPath) {
			return fmt.Errorf("cannot overwrite symbolic link: %s", upload.Name)
		}

		if err = com.Copy(tmpPath, targetPath); err != nil {
			return fmt.Errorf("copy: %v", err)
		}
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	}

	err = git.CreateCommit(
		localPath,
		&git.Signature{
			Name:  doer.DisplayName(),
			Email: doer.Email,
			When:  time.Now(),
		},
		opts.Message,
	)
	if err != nil {
		return fmt.Errorf("commit changes on %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: r.MustOwner().Name,
					OwnerSalt: r.MustOwner().Salt,
					RepoID:    r.ID,
					RepoName:  r.Name,
					RepoPath:  r.RepoPath(),
				}),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.NewBranch, err)
	}

	return DeleteUploads(uploads...)
}
