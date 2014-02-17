// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"os"
	"path/filepath"
	"strings"
	"time"

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

// check if repository is exist
func IsRepositoryExist(user *User, reposName string) (bool, error) {
	repo := Repo{OwnerId: user.Id}
	// TODO: get repository by nocase name
	return orm.Where("lower_name = ?", strings.ToLower(reposName)).Get(&repo)
}

//
// create a repository for a user or orgnaziation
//
func CreateRepository(user *User, reposName string) (*Repo, error) {
	p := filepath.Join(repoRootPath, user.Name)
	os.MkdirAll(p, os.ModePerm)
	f := filepath.Join(p, reposName+".git")
	_, err := git.InitRepository(f, false)
	if err != nil {
		return nil, err
	}

	repo := Repo{OwnerId: user.Id, Name: reposName}
	session := orm.NewSession()
	defer session.Close()
	session.Begin()
	_, err = session.Insert(&repo)
	if err != nil {
		os.RemoveAll(f)
		session.Rollback()
		return nil, err
	}
	_, err = session.Exec("update user set num_repos = num_repos + 1 where id = ?", user.Id)
	if err != nil {
		os.RemoveAll(f)
		session.Rollback()
		return nil, err
	}
	err = session.Commit()
	if err != nil {
		os.RemoveAll(f)
		session.Rollback()
		return nil, err
	}
	return &repo, nil
}

// list one user's repository
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
	if err = os.RemoveAll(filepath.Join(repoRootPath, user.Name, reposName+".git")); err != nil {
		// TODO: log and delete manully
		return err
	}
	return nil
}
