// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/libgit2/git2go"

	"github.com/gogits/gogs/modules/log"
)

type Repository struct {
	Id          int64
	OwnerId     int64 `xorm:"unique(s)"`
	ForkId      int64
	LowerName   string `xorm:"unique(s) index not null"`
	Name        string `xorm:"index not null"`
	Description string
	Private     bool
	NumWatchs   int
	NumStars    int
	NumForks    int
	Created     time.Time `xorm:"created"`
	Updated     time.Time `xorm:"updated"`
}

type Star struct {
	Id      int64
	RepoId  int64
	UserId  int64
	Created time.Time `xorm:"created"`
}

var (
	ErrRepoAlreadyExist = errors.New("Repository already exist")
)

// check if repository is exist
func IsRepositoryExist(user *User, repoName string) (bool, error) {
	repo := Repository{OwnerId: user.Id}
	has, err := orm.Where("lower_name = ?", strings.ToLower(repoName)).Get(&repo)
	if err != nil {
		return has, err
	}
	s, err := os.Stat(RepoPath(user.Name, repoName))
	if err != nil {
		return false, nil
	}
	return s.IsDir(), nil
}

// CreateRepository creates a repository for given user or orgnaziation.
func CreateRepository(user *User, repoName, desc string, private bool) (*Repository, error) {
	isExist, err := IsRepositoryExist(user, repoName)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrRepoAlreadyExist
	}

	f := RepoPath(user.Name, repoName)
	if _, err = git.InitRepository(f, true); err != nil {
		return nil, err
	}

	repo := &Repository{
		OwnerId:     user.Id,
		Name:        repoName,
		LowerName:   strings.ToLower(repoName),
		Description: desc,
		Private:     private,
	}
	session := orm.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Insert(repo); err != nil {
		if err2 := os.RemoveAll(f); err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, repoName)
		}
		session.Rollback()
		return nil, err
	}

	// TODO: RemoveAll may fail due to not root access.
	access := Access{
		UserName: user.Name,
		RepoName: repo.Name,
		Mode:     AU_WRITABLE,
	}
	if _, err = session.Insert(&access); err != nil {
		if err2 := os.RemoveAll(f); err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, repoName)
		}
		session.Rollback()
		return nil, err
	}

	if _, err = session.Exec("update user set num_repos = num_repos + 1 where id = ?", user.Id); err != nil {
		if err2 := os.RemoveAll(f); err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, repoName)
		}
		session.Rollback()
		return nil, err
	}

	if err = session.Commit(); err != nil {
		if err2 := os.RemoveAll(f); err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, repoName)
		}
		session.Rollback()
		return nil, err
	}
	return repo, nil
}

// GetRepositories returns the list of repositories of given user.
func GetRepositories(user *User) ([]Repository, error) {
	repos := make([]Repository, 0, 10)
	err := orm.Find(&repos, &Repository{OwnerId: user.Id})
	return repos, err
}

func StarReposiory(user *User, repoName string) error {
	return nil
}

func UnStarRepository() {

}

func WatchRepository() {

}

func UnWatchRepository() {

}

func ForkRepository(reposName string, userId int64) {

}

func RepoPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), repoName+".git")
}

// DeleteRepository deletes a repository for a user or orgnaztion.
func DeleteRepository(user *User, reposName string) (err error) {
	session := orm.NewSession()
	if _, err = session.Delete(&Repository{OwnerId: user.Id, Name: reposName}); err != nil {
		session.Rollback()
		return err
	}
	if _, err = session.Exec("update user set num_repos = num_repos - 1 where id = ?", user.Id); err != nil {
		session.Rollback()
		return err
	}
	if err = session.Commit(); err != nil {
		session.Rollback()
		return err
	}
	if err = os.RemoveAll(RepoPath(user.Name, reposName)); err != nil {
		// TODO: log and delete manully
		log.Error("delete repo %s/%s failed", user.Name, reposName)
		return err
	}
	return nil
}
