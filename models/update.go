package models

import (
	"container/list"
	"os/exec"
	"strings"

	"github.com/gogits/git"
	"github.com/gogits/gogs/modules/base"
	qlog "github.com/qiniu/log"
)

func Update(refName, oldCommitId, newCommitId, userName, repoName string, userId int64) {
	isNew := strings.HasPrefix(oldCommitId, "0000000")
	if isNew &&
		strings.HasPrefix(newCommitId, "0000000") {
		qlog.Fatal("old rev and new rev both 000000")
	}

	f := RepoPath(userName, repoName)

	gitUpdate := exec.Command("git", "update-server-info")
	gitUpdate.Dir = f
	gitUpdate.Run()

	repo, err := git.OpenRepository(f)
	if err != nil {
		qlog.Fatalf("runUpdate.Open repoId: %v", err)
	}

	newOid, err := git.NewOidFromString(newCommitId)
	if err != nil {
		qlog.Fatalf("runUpdate.Ref repoId:%v err: %v", newCommitId, err)
	}

	newCommit, err := repo.LookupCommit(newOid)
	if err != nil {
		qlog.Fatalf("runUpdate.Ref repoId: %v", err)
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = repo.CommitsBefore(newCommit.Id())
		if err != nil {
			qlog.Fatalf("Find CommitsBefore erro:", err)
		}
	} else {
		oldOid, err := git.NewOidFromString(oldCommitId)
		if err != nil {
			qlog.Fatalf("runUpdate.Ref repoId: %v", err)
		}

		oldCommit, err := repo.LookupCommit(oldOid)
		if err != nil {
			qlog.Fatalf("runUpdate.Ref repoId: %v", err)
		}
		l = repo.CommitsBetween(newCommit, oldCommit)
	}

	if err != nil {
		qlog.Fatalf("runUpdate.Commit repoId: %v", err)
	}

	repos, err := GetRepositoryByName(userId, repoName)
	if err != nil {
		qlog.Fatalf("runUpdate.GetRepositoryByName userId: %v", err)
	}

	commits := make([]*base.PushCommit, 0)
	var maxCommits = 3
	var actEmail string
	for e := l.Front(); e != nil; e = e.Next() {
		commit := e.Value.(*git.Commit)
		if actEmail == "" {
			actEmail = commit.Committer.Email
		}
		commits = append(commits,
			&base.PushCommit{commit.Id().String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name})
		if len(commits) >= maxCommits {
			break
		}
	}

	//commits = append(commits, []string{lastCommit.Id().String(), lastCommit.Message()})
	if err = CommitRepoAction(userId, userName, actEmail,
		repos.Id, repoName, git.BranchName(refName), &base.PushCommits{l.Len(), commits}); err != nil {
		qlog.Fatalf("runUpdate.models.CommitRepoAction: %v", err)
	}
}
