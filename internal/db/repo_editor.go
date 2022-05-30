// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"io"
	"io/ioutil"
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
	dberrors "gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/pathutil"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/tool"
)

const (
	ENV_AUTH_USER_ID           = "GOGS_AUTH_USER_ID"
	ENV_AUTH_USER_NAME         = "GOGS_AUTH_USER_NAME"
	ENV_AUTH_USER_EMAIL        = "GOGS_AUTH_USER_EMAIL"
	ENV_REPO_OWNER_NAME        = "GOGS_REPO_OWNER_NAME"
	ENV_REPO_OWNER_SALT_MD5    = "GOGS_REPO_OWNER_SALT_MD5"
	ENV_REPO_ID                = "GOGS_REPO_ID"
	ENV_REPO_NAME              = "GOGS_REPO_NAME"
	ENV_REPO_CUSTOM_HOOKS_PATH = "GOGS_REPO_CUSTOM_HOOKS_PATH"
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
		ENV_AUTH_USER_ID + "=" + com.ToStr(opts.AuthUser.ID),
		ENV_AUTH_USER_NAME + "=" + opts.AuthUser.Name,
		ENV_AUTH_USER_EMAIL + "=" + opts.AuthUser.Email,
		ENV_REPO_OWNER_NAME + "=" + opts.OwnerName,
		ENV_REPO_OWNER_SALT_MD5 + "=" + cryptoutil.MD5(opts.OwnerSalt),
		ENV_REPO_ID + "=" + com.ToStr(opts.RepoID),
		ENV_REPO_NAME + "=" + opts.RepoName,
		ENV_REPO_CUSTOM_HOOKS_PATH + "=" + filepath.Join(opts.RepoPath, "custom_hooks"),
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

func (repo *Repository) DiscardLocalRepoBranchChanges(branch string) error {
	return discardLocalRepoBranchChanges(repo.LocalCopyPath(), branch)
}

// CheckoutNewBranch checks out to a new branch from the a branch name.
func (repo *Repository) CheckoutNewBranch(oldBranch, newBranch string) error {
	if err := git.Checkout(repo.LocalCopyPath(), newBranch, git.CheckoutOptions{
		BaseBranch: oldBranch,
		Timeout:    time.Duration(conf.Git.Timeout.Pull) * time.Second,
	}); err != nil {
		return fmt.Errorf("checkout [base: %s, new: %s]: %v", oldBranch, newBranch, err)
	}
	return nil
}

type UpdateRepoFileOptions struct {
	LastCommitID string
	OldBranch    string
	NewBranch    string
	OldTreeName  string
	NewTreeName  string
	Message      string
	Content      string
	IsNewFile    bool
}

// UpdateRepoFile adds or updates a file in repository.
func (repo *Repository) UpdateRepoFile(doer *User, opts UpdateRepoFileOptions) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local repo branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = repo.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	repoPath := repo.RepoPath()
	localPath := repo.LocalCopyPath()

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

		if err := repo.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
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
		if com.IsExist(filePath) {
			return ErrRepoFileAlreadyExist{filePath}
		}
	}

	// Ignore move step if it's a new file under a directory.
	// Otherwise, move the file when name changed.
	if osutil.IsFile(oldFilePath) && opts.OldTreeName != opts.NewTreeName {
		if err = git.Move(localPath, opts.OldTreeName, opts.NewTreeName); err != nil {
			return fmt.Errorf("git mv %q %q: %v", opts.OldTreeName, opts.NewTreeName, err)
		}
	}

	if err = ioutil.WriteFile(filePath, []byte(opts.Content), 0666); err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	} else if err = git.CreateCommit(localPath, doer.NewGitSig(), opts.Message); err != nil {
		return fmt.Errorf("commit changes on %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: repo.MustOwner().Name,
					OwnerSalt: repo.MustOwner().Salt,
					RepoID:    repo.ID,
					RepoName:  repo.Name,
					RepoPath:  repo.RepoPath(),
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
func (repo *Repository) GetDiffPreview(branch, treePath, content string) (diff *gitutil.Diff, err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(branch); err != nil {
		return nil, fmt.Errorf("discard local repo branch[%s] changes: %v", branch, err)
	} else if err = repo.UpdateLocalCopyBranch(branch); err != nil {
		return nil, fmt.Errorf("update local copy branch[%s]: %v", branch, err)
	}

	localPath := repo.LocalCopyPath()
	filePath := path.Join(localPath, treePath)
	if err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return nil, err
	}
	if err = ioutil.WriteFile(filePath, []byte(content), 0666); err != nil {
		return nil, fmt.Errorf("write file: %v", err)
	}

	cmd := exec.Command("git", "diff", treePath)
	cmd.Dir = localPath
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("get stdout pipe: %v", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %v", err)
	}

	pid := process.Add(fmt.Sprintf("GetDiffPreview [repo_path: %s]", repo.RepoPath()), cmd)
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

func (repo *Repository) DeleteRepoFile(doer *User, opts DeleteRepoFileOptions) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local repo branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = repo.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	if opts.OldBranch != opts.NewBranch {
		if err := repo.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("checkout new branch[%s] from old branch[%s]: %v", opts.NewBranch, opts.OldBranch, err)
		}
	}

	localPath := repo.LocalCopyPath()
	if err = os.Remove(path.Join(localPath, opts.TreePath)); err != nil {
		return fmt.Errorf("remove file %q: %v", opts.TreePath, err)
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	} else if err = git.CreateCommit(localPath, doer.NewGitSig(), opts.Message); err != nil {
		return fmt.Errorf("commit changes to %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: repo.MustOwner().Name,
					OwnerSalt: repo.MustOwner().Salt,
					RepoID:    repo.ID,
					RepoName:  repo.Name,
					RepoPath:  repo.RepoPath(),
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
	defer fw.Close()

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

// isRepositoryGitPath returns true if given path is or resides inside ".git" path of the repository.
func isRepositoryGitPath(path string) bool {
	return strings.HasSuffix(path, ".git") ||
		strings.Contains(path, ".git"+string(os.PathSeparator)) ||
		// Windows treats ".git." the same as ".git"
		strings.HasSuffix(path, ".git.") ||
		strings.Contains(path, ".git."+string(os.PathSeparator))
}

func (repo *Repository) UploadRepoFiles(doer *User, opts UploadRepoFileOptions) error {
	if len(opts.Files) == 0 {
		return nil
	}

	// Prevent uploading files into the ".git" directory
	if isRepositoryGitPath(opts.TreePath) {
		return errors.Errorf("bad tree path %q", opts.TreePath)
	}

	uploads, err := GetUploadsByUUIDs(opts.Files)
	if err != nil {
		return fmt.Errorf("get uploads by UUIDs[%v]: %v", opts.Files, err)
	}

	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("discard local repo branch[%s] changes: %v", opts.OldBranch, err)
	} else if err = repo.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("update local copy branch[%s]: %v", opts.OldBranch, err)
	}

	if opts.OldBranch != opts.NewBranch {
		if err = repo.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("checkout new branch[%s] from old branch[%s]: %v", opts.NewBranch, opts.OldBranch, err)
		}
	}

	localPath := repo.LocalCopyPath()
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

		upload.Name = pathutil.Clean(upload.Name)

		// Prevent uploading files into the ".git" directory
		if isRepositoryGitPath(upload.Name) {
			continue
		}

		targetPath := path.Join(dirPath, upload.Name)
		if err = com.Copy(tmpPath, targetPath); err != nil {
			return fmt.Errorf("copy: %v", err)
		}
	}

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	} else if err = git.CreateCommit(localPath, doer.NewGitSig(), opts.Message); err != nil {
		return fmt.Errorf("commit changes on %q: %v", localPath, err)
	}

	err = git.Push(localPath, "origin", opts.NewBranch,
		git.PushOptions{
			CommandOptions: git.CommandOptions{
				Envs: ComposeHookEnvs(ComposeHookEnvsOptions{
					AuthUser:  doer,
					OwnerName: repo.MustOwner().Name,
					OwnerSalt: repo.MustOwner().Salt,
					RepoID:    repo.ID,
					RepoName:  repo.Name,
					RepoPath:  repo.RepoPath(),
				}),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.NewBranch, err)
	}

	return DeleteUploads(uploads...)
}
