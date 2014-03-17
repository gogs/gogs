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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"

	"github.com/gogits/git"

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
	gitInitLocker          = sync.Mutex{}
	LanguageIgns, Licenses []string
)

var (
	ErrRepoAlreadyExist = errors.New("Repository already exist")
	ErrRepoNotExist     = errors.New("Repository does not exist")
)

func init() {
	LanguageIgns = strings.Split(base.Cfg.MustValue("repository", "LANG_IGNS"), "|")
	Licenses = strings.Split(base.Cfg.MustValue("repository", "LICENSES"), "|")

	zip.Verbose = false
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
		return false, nil // Error simply means does not exist, but we don't want to show up.
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
	return nil, nil
}

// extractGitBareZip extracts git-bare.zip to repository path.
func extractGitBareZip(repoPath string) error {
	z, err := zip.Open("conf/content/git-bare.zip")
	if err != nil {
		fmt.Println("shi?")
		return err
	}
	defer z.Close()

	return z.ExtractTo(repoPath)
}

// initRepoCommit temporarily changes with work directory.
func initRepoCommit(tmpPath string, sig *git.Signature) error {
	gitInitLocker.Lock()
	defer gitInitLocker.Unlock()

	// Change work directory.
	curPath, err := os.Getwd()
	if err != nil {
		return err
	} else if err = os.Chdir(tmpPath); err != nil {
		return err
	}
	defer os.Chdir(curPath)

	if _, _, err := com.ExecCmd("git", "add", "--all"); err != nil {
		return err
	}
	if _, _, err := com.ExecCmd("git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return err
	}
	if _, _, err := com.ExecCmd("git", "push", "origin", "master"); err != nil {
		return err
	}
	return nil
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(f string, user *User, repo *Repository, initReadme bool, repoLang, license string) error {
	repoPath := RepoPath(user.Name, repo.Name)

	// Create bare new repository.
	if err := extractGitBareZip(repoPath); err != nil {
		return err
	}

	// Initialize repository according to user's choice.
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

	// Clone to temprory path and do the init commit.
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()))
	os.MkdirAll(tmpDir, os.ModePerm)

	if _, _, err := com.ExecCmd("git", "clone", repoPath, tmpDir); err != nil {
		return err
	}

	// README
	if initReadme {
		defaultReadme := repo.Name + "\n" + strings.Repeat("=",
			utf8.RuneCountInString(repo.Name)) + "\n\n" + repo.Description
		if err := ioutil.WriteFile(filepath.Join(tmpDir, fileName["readme"]),
			[]byte(defaultReadme), 0644); err != nil {
			return err
		}
	}

	// .gitignore
	if repoLang != "" {
		filePath := "conf/gitignore/" + repoLang
		if com.IsFile(filePath) {
			if _, err := com.Copy(filePath,
				filepath.Join(tmpDir, fileName["gitign"])); err != nil {
				return err
			}
		}
	}

	// LICENSE
	if license != "" {
		filePath := "conf/license/" + license
		if com.IsFile(filePath) {
			if _, err := com.Copy(filePath,
				filepath.Join(tmpDir, fileName["license"])); err != nil {
				return err
			}
		}
	}

	// Apply changes and commit.
	if err := initRepoCommit(tmpDir, user.NewGitSig()); err != nil {
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
	err := orm.Desc("updated").Find(&repos, &Repository{OwnerId: user.Id})
	return repos, err
}

func GetRepositoryCount(user *User) (int64, error) {
	return orm.Count(&Repository{OwnerId: user.Id})
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
func DeleteRepository(userId, repoId int64, userName string) (err error) {
	repo := &Repository{Id: repoId, OwnerId: userId}
	has, err := orm.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist
	}

	session := orm.NewSession()
	if err = session.Begin(); err != nil {
		return err
	}
	if _, err = session.Delete(&Repository{Id: repoId}); err != nil {
		session.Rollback()
		return err
	}
	if _, err := session.Delete(&Access{UserName: userName, RepoName: repo.Name}); err != nil {
		session.Rollback()
		return err
	}
	if _, err = session.Exec("update user set num_repos = num_repos - 1 where id = ?", userId); err != nil {
		session.Rollback()
		return err
	}
	if err = session.Commit(); err != nil {
		session.Rollback()
		return err
	}
	if err = os.RemoveAll(RepoPath(userName, repo.Name)); err != nil {
		// TODO: log and delete manully
		log.Error("delete repo %s/%s failed", userName, repo.Name)
		return err
	}
	return nil
}
