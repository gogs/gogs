// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Unknwon/com"
	git "github.com/libgit2/git2go"

	"github.com/gogits/gogs/modules/base"
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
	LanguageIgns, Licenses []string
)

var (
	ErrRepoAlreadyExist = errors.New("Repository already exist")
	ErrRepoNotExist     = errors.New("Repository does not exist")
)

func init() {
	LanguageIgns = strings.Split(base.Cfg.MustValue("repository", "LANG_IGNS"), "|")
	Licenses = strings.Split(base.Cfg.MustValue("repository", "LICENSES"), "|")
}

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
func CreateRepository(user *User, repoName, desc, repoLang, license string, private bool, initReadme bool) (*Repository, error) {
	isExist, err := IsRepositoryExist(user, repoName)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrRepoAlreadyExist
	}

	repo := &Repository{
		OwnerId:     user.Id,
		Name:        repoName,
		LowerName:   strings.ToLower(repoName),
		Description: desc,
		Private:     private,
	}

	f := RepoPath(user.Name, repoName)
	if err = initRepository(f, user, repo, initReadme, repoLang, license); err != nil {
		return nil, err
	}
	session := orm.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Insert(repo); err != nil {
		if err2 := os.RemoveAll(f); err2 != nil {
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed", user.Name, repoName))
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
		session.Rollback()
		if err2 := os.RemoveAll(f); err2 != nil {
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed", user.Name, repoName))
		}
		return nil, err
	}

	if _, err = session.Exec("update user set num_repos = num_repos + 1 where id = ?", user.Id); err != nil {
		session.Rollback()
		if err2 := os.RemoveAll(f); err2 != nil {
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed", user.Name, repoName))
		}
		return nil, err
	}

	if err = session.Commit(); err != nil {
		session.Rollback()
		if err2 := os.RemoveAll(f); err2 != nil {
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed", user.Name, repoName))
		}
		return nil, err
	}

	return repo, NewRepoAction(user, repo)
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(f string, user *User, repo *Repository, initReadme bool, repoLang, license string) error {
	fileName := map[string]string{}

	if initReadme {
		fileName["readme"] = "README.md"
	}
	if repoLang != "" {
		fileName["gitign"] = ".gitignore"
	}
	if license != "" {
		fileName["license"] = "LICENSE"
	}

	workdir := filepath.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()))
	os.MkdirAll(workdir, os.ModePerm)

	sig := user.NewGitSig()

	// README
	if initReadme {
		defaultReadme := repo.Name + "\n" + strings.Repeat("=",
			utf8.RuneCountInString(repo.Name)) + "\n\n" + repo.Description
		if err := ioutil.WriteFile(filepath.Join(workdir, fileName["readme"]),
			[]byte(defaultReadme), 0644); err != nil {
			return err
		}
	}

	if repoLang != "" {
		// .gitignore
		filePath := "conf/gitignore/" + repoLang
		if com.IsFile(filePath) {
			if _, err := com.Copy(filePath,
				filepath.Join(workdir, fileName["gitign"])); err != nil {
				return err
			}
		}
	}

	if license != "" {
		// LICENSE
		filePath := "conf/license/" + license
		if com.IsFile(filePath) {
			if _, err := com.Copy(filePath,
				filepath.Join(workdir, fileName["license"])); err != nil {
				return err
			}
		}
	}

	rp, err := git.InitRepository(f, true)
	if err != nil {
		return err
	}
	rp.SetWorkdir(workdir, false)

	idx, err := rp.Index()
	if err != nil {
		return err
	}

	for _, name := range fileName {
		if err = idx.AddByPath(name); err != nil {
			return err
		}
	}

	treeId, err := idx.WriteTree()
	if err != nil {
		return err
	}

	message := "Init commit"
	tree, err := rp.LookupTree(treeId)
	if err != nil {
		return err
	}

	if _, err = rp.CreateCommit("HEAD", sig, sig, message, tree); err != nil {
		return err
	}

	return nil
}

func GetRepositoryByName(user *User, repoName string) (*Repository, error) {
	repo := &Repository{
		OwnerId:   user.Id,
		LowerName: strings.ToLower(repoName),
	}
	has, err := orm.Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist
	}
	return repo, err
}

func GetRepositoryById(id int64) (repo *Repository, err error) {
	has, err := orm.Id(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist
	}
	return repo, err
}

// GetRepositories returns the list of repositories of given user.
func GetRepositories(user *User) ([]Repository, error) {
	repos := make([]Repository, 0, 10)
	err := orm.Find(&repos, &Repository{OwnerId: user.Id})
	return repos, err
}

func GetRepositoryCount(user *User) (int64, error) {
	return orm.Count(&Repository{OwnerId: user.Id})
}

const (
	RFile = iota + 1
	RDir
)

type RepoFile struct {
	Type    int
	Name    string
	Message string
	Created time.Time
}

func (f *RepoFile) IsFile() bool {
	return f.Type == git.FilemodeBlob || f.Type == git.FilemodeBlobExecutable
}

func (f *RepoFile) IsDir() bool {
	return f.Type == git.FilemodeTree
}

func GetReposFiles(userName, reposName, treeName, rpath string) ([]*RepoFile, error) {
	f := RepoPath(userName, reposName)
	repo, err := git.OpenRepository(f)
	if err != nil {
		return nil, err
	}

	obj, err := repo.RevparseSingle("HEAD")
	if err != nil {
		return nil, err
	}
	lastCommit := obj.(*git.Commit)
	var repofiles []*RepoFile
	tree, err := lastCommit.Tree()
	if err != nil {
		return nil, err
	}
	var i uint64 = 0
	for ; i < tree.EntryCount(); i++ {
		entry := tree.EntryByIndex(i)

		repofiles = append(repofiles, &RepoFile{
			entry.Filemode,
			entry.Name,
			lastCommit.Message(),
			lastCommit.Committer().When,
		})
	}

	return repofiles, nil
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
