// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/Unknwon/com"

	git "github.com/gogits/git-module"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/process"
	"github.com/gogits/gogs/modules/setting"
)

// ___________    .___.__  __    ___________.__.__
// \_   _____/  __| _/|__|/  |_  \_   _____/|__|  |   ____
//  |    __)_  / __ | |  \   __\  |    __)  |  |  | _/ __ \
//  |        \/ /_/ | |  ||  |    |     \   |  |  |_\  ___/
// /_______  /\____ | |__||__|    \___  /   |__|____/\___  >
//         \/      \/                 \/                 \/

// discardLocalRepoBranchChanges discards local commits of given branch
// to make sure it is even to remote branch when local copy exists.
func discardLocalRepoBranchChanges(localPath, branch string) error {
	if !com.IsExist(localPath) {
		return nil
	}
	// No need to check if nothing in the repository.
	if !git.IsBranchExist(localPath, branch) {
		return nil
	}
	if err := git.ResetHEAD(localPath, true, "origin/"+branch); err != nil {
		return fmt.Errorf("ResetHEAD: %v", err)
	}
	return nil
}

func (repo *Repository) DiscardLocalRepoBranchChanges(branch string) error {
	return discardLocalRepoBranchChanges(repo.LocalCopyPath(), branch)
}

func checkoutNewBranch(repoPath, localPath, oldBranch, newBranch string) error {
	if !com.IsExist(localPath) {
		if err := UpdateLocalCopyBranch(repoPath, localPath, oldBranch); err != nil {
			return err
		}
	}
	if err := git.Checkout(localPath, git.CheckoutOptions{
		Branch:    newBranch,
		OldBranch: oldBranch,
		Timeout:   time.Duration(setting.Git.Timeout.Pull) * time.Second,
	}); err != nil {
		return fmt.Errorf("Checkout: %v", err)
	}
	return nil
}

// CheckoutNewBranch checks out a new branch from the given branch name.
func (repo *Repository) CheckoutNewBranch(oldBranch, newBranch string) error {
	return checkoutNewBranch(repo.RepoPath(), repo.LocalCopyPath(), oldBranch, newBranch)
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

// updateRepoFile adds new file to repository.
func (repo *Repository) UpdateRepoFile(doer *User, opts *UpdateRepoFileOptions) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(opts.OldBranch); err != nil {
		return fmt.Errorf("DiscardLocalRepoBranchChanges [branch: %s]: %v", opts.OldBranch, err)
	} else if err = repo.UpdateLocalCopyBranch(opts.OldBranch); err != nil {
		return fmt.Errorf("UpdateLocalCopyBranch [branch: %s]: %v", opts.OldBranch, err)
	}

	if opts.OldBranch != opts.NewBranch {
		if err := repo.CheckoutNewBranch(opts.OldBranch, opts.NewBranch); err != nil {
			return fmt.Errorf("CheckoutNewBranch [old_branch: %s, new_branch: %s]: %v", opts.OldBranch, opts.NewBranch, err)
		}
	}

	localPath := repo.LocalCopyPath()
	filePath := path.Join(localPath, opts.NewTreeName)

	if len(opts.Message) == 0 {
		if opts.IsNewFile {
			opts.Message = "Add '" + opts.NewTreeName + "'"
		} else {
			opts.Message = "Update '" + opts.NewTreeName + "'"
		}
	}

	os.MkdirAll(path.Dir(filePath), os.ModePerm)

	// If new file, make sure it doesn't exist; if old file, move if file name change.
	if opts.IsNewFile {
		if com.IsExist(filePath) {
			return ErrRepoFileAlreadyExist{filePath}
		}
	} else if len(opts.OldTreeName) > 0 && len(opts.NewTreeName) > 0 && opts.NewTreeName != opts.OldTreeName {
		if err = git.MoveFile(localPath, opts.OldTreeName, opts.NewTreeName); err != nil {
			return fmt.Errorf("MoveFile [old_tree_name: %s, new_tree_name: %s]: %v", opts.OldTreeName, opts.NewTreeName, err)
		}
	}

	if err = ioutil.WriteFile(filePath, []byte(opts.Content), 0666); err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("AddChanges: %v", err)
	} else if err = git.CommitChanges(localPath, opts.Message, doer.NewGitSig()); err != nil {
		return fmt.Errorf("CommitChanges: %v", err)
	} else if err = git.Push(localPath, "origin", opts.NewBranch); err != nil {
		return fmt.Errorf("Push: %v", err)
	}

	gitRepo, err := git.OpenRepository(repo.RepoPath())
	if err != nil {
		log.Error(4, "OpenRepository: %v", err)
		return nil
	}
	commit, err := gitRepo.GetBranchCommit(opts.NewBranch)
	if err != nil {
		log.Error(4, "GetBranchCommit [branch: %s]: %v", opts.NewBranch, err)
		return nil
	}

	pushCommits := &PushCommits{
		Len:     1,
		Commits: []*PushCommit{CommitToPushCommit(commit)},
	}
	oldCommitID := opts.LastCommitID
	if opts.NewBranch != opts.OldBranch {
		oldCommitID = "0000000000000000000000000000000000000000" // New Branch so we use all 0s
	}
	if err := CommitRepoAction(doer.ID, repo.MustOwner().ID, doer.Name, doer.Email,
		repo.ID, repo.MustOwner().Name, repo.Name, git.BRANCH_PREFIX+opts.NewBranch,
		pushCommits, oldCommitID, commit.ID.String()); err != nil {
		log.Error(4, "CommitRepoAction: %v", err)
		return nil
	}
	go HookQueue.Add(repo.ID)

	return nil
}

func (repo *Repository) GetDiffPreview(branch, treeName, content string) (diff *Diff, err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(branch); err != nil {
		return nil, fmt.Errorf("discardLocalRepoChanges: %s - %v", branch, err)
	} else if err = repo.UpdateLocalCopyBranch(branch); err != nil {
		return nil, fmt.Errorf("UpdateLocalCopyBranch: %s - %v", branch, err)
	}

	localPath := repo.LocalCopyPath()
	filePath := path.Join(localPath, treeName)

	os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
	if err = ioutil.WriteFile(filePath, []byte(content), 0666); err != nil {
		return nil, fmt.Errorf("WriteFile: %v", err)
	}

	cmd := exec.Command("git", "diff", treeName)
	cmd.Dir = localPath
	cmd.Stderr = os.Stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("StdoutPipe: %v", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("Start: %v", err)
	}

	pid := process.Add(fmt.Sprintf("GetDiffRange [repo_path: %s]", repo.RepoPath()), cmd)
	defer process.Remove(pid)

	diff, err = ParsePatch(setting.Git.MaxGitDiffLines, setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles, stdout)
	if err != nil {
		return nil, fmt.Errorf("ParsePatch: %v", err)
	}

	if err = cmd.Wait(); err != nil {
		return nil, fmt.Errorf("Wait: %v", err)
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

func (repo *Repository) DeleteRepoFile(doer *User, oldCommitID, branch, treeName, message string) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	localPath := repo.LocalCopyPath()
	if err = discardLocalRepoBranchChanges(localPath, branch); err != nil {
		return fmt.Errorf("discardLocalRepoChanges: %v", err)
	} else if err = repo.UpdateLocalCopyBranch(branch); err != nil {
		return fmt.Errorf("UpdateLocalCopyBranch: %v", err)
	}

	filePath := path.Join(localPath, treeName)
	os.Remove(filePath)

	if len(message) == 0 {
		message = "Delete file '" + treeName + "'"
	}

	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("AddChanges: %v", err)
	} else if err = git.CommitChanges(localPath, message, doer.NewGitSig()); err != nil {
		return fmt.Errorf("CommitChanges: %v", err)
	} else if err = git.Push(localPath, "origin", branch); err != nil {
		return fmt.Errorf("Push: %v", err)
	}

	gitRepo, err := git.OpenRepository(repo.RepoPath())
	if err != nil {
		log.Error(4, "OpenRepository: %v", err)
		return nil
	}
	commit, err := gitRepo.GetBranchCommit(branch)
	if err != nil {
		log.Error(4, "GetBranchCommit [branch: %s]: %v", branch, err)
		return nil
	}

	pushCommits := &PushCommits{
		Len:     1,
		Commits: []*PushCommit{CommitToPushCommit(commit)},
	}
	if err := CommitRepoAction(doer.ID, repo.MustOwner().ID, doer.Name, doer.Email,
		repo.ID, repo.MustOwner().Name, repo.Name, git.BRANCH_PREFIX+branch,
		pushCommits, oldCommitID, commit.ID.String()); err != nil {
		log.Error(4, "CommitRepoAction: %v", err)
		return nil
	}
	go HookQueue.Add(repo.ID)

	return nil
}
