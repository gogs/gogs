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

	newCommit, err := repo.GetCommit(newCommitId)
	if err != nil {
		qlog.Fatalf("runUpdate GetCommit of newCommitId: %v", err)
		return
	}

	var l *list.List
	// if a new branch
	if isNew {
		l, err = newCommit.CommitsBefore()
		if err != nil {
			qlog.Fatalf("Find CommitsBefore erro: %v", err)
		}
	} else {
		l, err = newCommit.CommitsBeforeUntil(oldCommitId)
		if err != nil {
			qlog.Fatalf("Find CommitsBeforeUntil erro: %v", err)
			return
		}
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
			&base.PushCommit{commit.Id.String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name})
		if len(commits) >= maxCommits {
			break
		}
	}

	//commits = append(commits, []string{lastCommit.Id().String(), lastCommit.Message()})
	if err = CommitRepoAction(userId, userName, actEmail,
		repos.Id, repoName, refName, &base.PushCommits{l.Len(), commits}); err != nil {
		qlog.Fatalf("runUpdate.models.CommitRepoAction: %v", err)
	}
}
