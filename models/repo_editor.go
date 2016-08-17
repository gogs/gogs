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

// discardLocalRepoBranchChanges discards local commits/changes of
// given branch to make sure it is even to remote branch.
func discardLocalRepoBranchChanges(localPath, branch string) error {
	if !com.IsExist(localPath) {
		return nil
	}
	// No need to check if nothing in the repository.
	if !git.IsBranchExist(localPath, branch) {
		return nil
	}

	refName := "origin/" + branch
	if err := git.ResetHEAD(localPath, true, refName); err != nil {
		return fmt.Errorf("git reset --hard %s: %v", refName, err)
	}
	return nil
}

func (repo *Repository) DiscardLocalRepoBranchChanges(branch string) error {
	return discardLocalRepoBranchChanges(repo.LocalCopyPath(), branch)
}

// checkoutNewBranch checks out to a new branch from the a branch name.
func checkoutNewBranch(repoPath, localPath, oldBranch, newBranch string) error {
	if err := git.Checkout(localPath, git.CheckoutOptions{
		Timeout:   time.Duration(setting.Git.Timeout.Pull) * time.Second,
		Branch:    newBranch,
		OldBranch: oldBranch,
	}); err != nil {
		return fmt.Errorf("git checkout -b %s %s: %v", newBranch, oldBranch, err)
	}
	return nil
}

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

// UpdateRepoFile adds or updates a file in repository.
func (repo *Repository) UpdateRepoFile(doer *User, opts UpdateRepoFileOptions) (err error) {
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

	if len(opts.Message) == 0 {
		if opts.IsNewFile {
			opts.Message = "Add '" + opts.NewTreeName + "'"
		} else {
			opts.Message = "Update '" + opts.NewTreeName + "'"
		}
	}

	localPath := repo.LocalCopyPath()
	filePath := path.Join(localPath, opts.NewTreeName)
	os.MkdirAll(path.Dir(filePath), os.ModePerm)

	// If it's meant to be a new file, make sure it doesn't exist.
	if opts.IsNewFile {
		if com.IsExist(filePath) {
			return ErrRepoFileAlreadyExist{filePath}
		}
	}

	// If update a file, move if file name change.
	if len(opts.OldTreeName) > 0 && len(opts.NewTreeName) > 0 && opts.OldTreeName != opts.NewTreeName {
		if err = git.MoveFile(localPath, opts.OldTreeName, opts.NewTreeName); err != nil {
			return fmt.Errorf("git mv %s %s: %v", opts.OldTreeName, opts.NewTreeName, err)
		}
	}

	if err = ioutil.WriteFile(filePath, []byte(opts.Content), 0666); err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	}

	signaure := doer.NewGitSig()
	if err = git.CommitChanges(localPath, opts.Message, signaure); err != nil {
		return fmt.Errorf("git commit -m %s --author='%s <%s>': %v", opts.Message, signaure.Name, signaure.Email, err)
	} else if err = git.Push(localPath, "origin", opts.NewBranch); err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.NewBranch, err)
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

	// Simulate push event.
	pushCommits := &PushCommits{
		Len:     1,
		Commits: []*PushCommit{CommitToPushCommit(commit)},
	}
	oldCommitID := opts.LastCommitID
	if opts.NewBranch != opts.OldBranch {
		oldCommitID = git.EMPTY_SHA
	}
	if err := CommitRepoAction(CommitRepoActionOptions{
		PusherName:  doer.Name,
		RepoOwnerID: repo.MustOwner().ID,
		RepoName:    repo.Name,
		RefFullName: git.BRANCH_PREFIX + opts.NewBranch,
		OldCommitID: oldCommitID,
		NewCommitID: commit.ID.String(),
		Commits:     pushCommits,
	}); err != nil {
		log.Error(4, "CommitRepoAction: %v", err)
		return nil
	}

	return nil
}

// GetDiffPreview produces and returns diff result of a file which is not yet committed.
func (repo *Repository) GetDiffPreview(branch, treeName, content string) (diff *Diff, err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.DiscardLocalRepoBranchChanges(branch); err != nil {
		return nil, fmt.Errorf("DiscardLocalRepoBranchChanges [branch: %s]: %v", branch, err)
	} else if err = repo.UpdateLocalCopyBranch(branch); err != nil {
		return nil, fmt.Errorf("UpdateLocalCopyBranch [branch: %s]: %v", branch, err)
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

	pid := process.Add(fmt.Sprintf("GetDiffPreview [repo_path: %s]", repo.RepoPath()), cmd)
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

type DeleteRepoFileOptions struct {
	LastCommitID string
	Branch       string
	TreePath     string
	Message      string
}

func (repo *Repository) DeleteRepoFile(doer *User, opts DeleteRepoFileOptions) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	localPath := repo.LocalCopyPath()
	if err = discardLocalRepoBranchChanges(localPath, opts.Branch); err != nil {
		return fmt.Errorf("discardLocalRepoBranchChanges [branch: %s]: %v", opts.Branch, err)
	} else if err = repo.UpdateLocalCopyBranch(opts.Branch); err != nil {
		return fmt.Errorf("UpdateLocalCopyBranch [branch: %s]: %v", opts.Branch, err)
	}

	if err = os.Remove(path.Join(localPath, opts.TreePath)); err != nil {
		return fmt.Errorf("Remove: %v", err)
	}

	if len(opts.Message) == 0 {
		opts.Message = "Delete file '" + opts.TreePath + "'"
	}

	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("git add --all: %v", err)
	}

	signaure := doer.NewGitSig()
	if err = git.CommitChanges(localPath, opts.Message, signaure); err != nil {
		return fmt.Errorf("git commit -m %s --author='%s <%s>': %v", opts.Message, signaure.Name, signaure.Email, err)
	} else if err = git.Push(localPath, "origin", opts.Branch); err != nil {
		return fmt.Errorf("git push origin %s: %v", opts.Branch, err)
	}

	gitRepo, err := git.OpenRepository(repo.RepoPath())
	if err != nil {
		log.Error(4, "OpenRepository: %v", err)
		return nil
	}
	commit, err := gitRepo.GetBranchCommit(opts.Branch)
	if err != nil {
		log.Error(4, "GetBranchCommit [branch: %s]: %v", opts.Branch, err)
		return nil
	}

	// Simulate push event.
	pushCommits := &PushCommits{
		Len:     1,
		Commits: []*PushCommit{CommitToPushCommit(commit)},
	}
	if err := CommitRepoAction(CommitRepoActionOptions{
		PusherName:  doer.Name,
		RepoOwnerID: repo.MustOwner().ID,
		RepoName:    repo.Name,
		RefFullName: git.BRANCH_PREFIX + opts.Branch,
		OldCommitID: opts.LastCommitID,
		NewCommitID: commit.ID.String(),
		Commits:     pushCommits,
	}); err != nil {
		log.Error(4, "CommitRepoAction: %v", err)
		return nil
	}

	return nil
}
