// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogits/gogs/utils/log"
	git "github.com/libgit2/git2go"
)

type Repo struct {
	Id        int64
	OwnerId   int64 `xorm:"unique(s)"`
	ForkId    int64
	LowerName string `xorm:"unique(s) index not null"`
	Name      string `xorm:"index not null"`
	NumWatchs int
	NumStars  int
	NumForks  int
	Created   time.Time `xorm:"created"`
	Updated   time.Time `xorm:"updated"`
}

type Star struct {
	Id      int64
	RepoId  int64
	UserId  int64
	Created time.Time `xorm:"created"`
}

// check if repository is exist
func IsRepositoryExist(user *User, reposName string) (bool, error) {
	repo := Repo{OwnerId: user.Id}
	has, err := orm.Where("lower_name = ?", strings.ToLower(reposName)).Get(&repo)
	if err != nil {
		return has, err
	}
	s, err := os.Stat(RepoPath(user.Name, reposName))
	if err != nil {
		return false, nil
	}
	return s.IsDir(), nil
}

//
// create a repository for a user or orgnaziation
//
func CreateRepository(user *User, reposName string) (*Repo, error) {
	f := RepoPath(user.Name, reposName)
	_, err := git.InitRepository(f, true)
	if err != nil {
		return nil, err
	}

	repo := Repo{OwnerId: user.Id, Name: reposName, LowerName: strings.ToLower(reposName)}
	session := orm.NewSession()
	defer session.Close()
	session.Begin()
	_, err = session.Insert(&repo)
	if err != nil {
		err2 := os.RemoveAll(f)
		if err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, reposName)
		}
		session.Rollback()
		return nil, err
	}
	access := Access{UserName: user.Name,
		RepoName: repo.Name,
		Mode:     AU_WRITABLE,
	}
	_, err = session.Insert(&access)
	if err != nil {
		err2 := os.RemoveAll(f)
		if err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, reposName)
		}
		session.Rollback()
		return nil, err
	}
	_, err = session.Exec("update user set num_repos = num_repos + 1 where id = ?", user.Id)
	if err != nil {
		err2 := os.RemoveAll(f)
		if err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, reposName)
		}
		session.Rollback()
		return nil, err
	}
	err = session.Commit()
	if err != nil {
		err2 := os.RemoveAll(f)
		if err2 != nil {
			log.Error("delete repo directory %s/%s failed", user.Name, reposName)
		}
		session.Rollback()
		return nil, err
	}
	return &repo, nil
}

// GetRepositories returns the list of repositories of given user.
func GetRepositories(user *User) ([]Repo, error) {
	repos := make([]Repo, 0)
	err := orm.Find(&repos, &Repo{OwnerId: user.Id})
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
	if _, err = session.Delete(&Repo{OwnerId: user.Id, Name: reposName}); err != nil {
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
