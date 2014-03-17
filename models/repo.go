// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
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

// Repository represents a git repository.
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

// IsRepositoryExist returns true if the repository with given name under user has already existed.
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

	repoPath := RepoPath(user.Name, repoName)
	if err = initRepository(repoPath, user, repo, initReadme, repoLang, license); err != nil {
		return nil, err
	}
	session := orm.NewSession()
	defer session.Close()
	session.Begin()

	if _, err = session.Insert(repo); err != nil {
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(repo): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(1): %v", user.Name, repoName, err2))
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
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(access): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(2): %v", user.Name, repoName, err2))
		}
		return nil, err
	}

	rawSql := "UPDATE user SET num_repos = num_repos + 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_repos = num_repos + 1 WHERE id = ?"
	}
	if _, err = session.Exec(rawSql, user.Id); err != nil {
		session.Rollback()
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(repo count): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(3): %v", user.Name, repoName, err2))
		}
		return nil, err
	}

	if err = session.Commit(); err != nil {
		session.Rollback()
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(commit): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(3): %v", user.Name, repoName, err2))
		}
		return nil, err
	}

	return repo, NewRepoAction(user, repo)
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

	var stdout, stderr string
	if stdout, stderr, err = com.ExecCmd("git", "add", "--all"); err != nil {
		return err
	}
	log.Info("stdout(1): %s", stdout)
	log.Info("stderr(1): %s", stderr)
	if stdout, stderr, err = com.ExecCmd("git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return err
	}
	log.Info("stdout(2): %s", stdout)
	log.Info("stderr(2): %s", stderr)
	if stdout, stderr, err = com.ExecCmd("git", "push", "origin", "master"); err != nil {
		return err
	}
	log.Info("stdout(3): %s", stdout)
	log.Info("stderr(3): %s", stderr)
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

// GetRepositoryByName returns the repository by given name under user if exists.
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

// GetRepositoryById returns the repository by given id if exists.
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
	rawSql := "UPDATE user SET num_repos = num_repos - 1 WHERE id = ?"
	if base.Cfg.MustValue("database", "DB_TYPE") == "postgres" {
		rawSql = "UPDATE \"user\" SET num_repos = num_repos - 1 WHERE id = ?"
	}
	if _, err = session.Exec(rawSql, userId); err != nil {
		session.Rollback()
		return err
	}
	if err = session.Commit(); err != nil {
		session.Rollback()
		return err
	}
	if err = os.RemoveAll(RepoPath(userName, repo.Name)); err != nil {
		// TODO: log and delete manully
		log.Error("delete repo %s/%s failed: %v", userName, repo.Name, err)
		return err
	}
	return nil
}

// Commit represents a git commit.
type Commit struct {
	Author  string
	Email   string
	Date    time.Time
	SHA     string
	Message string
}

var (
	ErrRepoFileNotLoaded = fmt.Errorf("repo file not loaded")
)

// RepoFile represents a file object in git repository.
type RepoFile struct {
	*git.TreeEntry
	Path       string
	Message    string
	Created    time.Time
	Size       int64
	Repo       *git.Repository
	LastCommit string
}

// LookupBlob returns the content of an object.
func (file *RepoFile) LookupBlob() (*git.Blob, error) {
	if file.Repo == nil {
		return nil, ErrRepoFileNotLoaded
	}

	return file.Repo.LookupBlob(file.Id)
}

// GetBranches returns all branches of given repository.
func GetBranches(userName, reposName string) ([]string, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	refs, err := repo.AllReferences()
	if err != nil {
		return nil, err
	}

	brs := make([]string, len(refs))
	for i, ref := range refs {
		brs[i] = ref.Name
	}
	return brs, nil
}

// GetReposFiles returns a list of file object in given directory of repository.
func GetReposFiles(userName, reposName, branchName, rpath string) ([]*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	ref, err := repo.LookupReference("refs/heads/" + branchName)
	if err != nil {
		return nil, err
	}

	lastCommit, err := repo.LookupCommit(ref.Oid)
	if err != nil {
		return nil, err
	}

	var repodirs []*RepoFile
	var repofiles []*RepoFile
	lastCommit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			size, err := repo.ObjectSize(entry.Id)
			if err != nil {
				return 0
			}

			var cm = lastCommit

			for {
				if cm.ParentCount() == 0 {
					break
				} else if cm.ParentCount() == 1 {
					pt, _ := repo.SubTree(cm.Parent(0).Tree, dirname)
					if pt == nil {
						break
					}
					pEntry := pt.EntryByName(entry.Name)
					if pEntry == nil || !pEntry.Id.Equal(entry.Id) {
						break
					} else {
						cm = cm.Parent(0)
					}
				} else {
					var emptyCnt = 0
					var sameIdcnt = 0
					for i := 0; i < cm.ParentCount(); i++ {
						p := cm.Parent(i)
						pt, _ := repo.SubTree(p.Tree, dirname)
						var pEntry *git.TreeEntry
						if pt != nil {
							pEntry = pt.EntryByName(entry.Name)
						}

						if pEntry == nil {
							if emptyCnt == cm.ParentCount()-1 {
								goto loop
							} else {
								emptyCnt = emptyCnt + 1
								continue
							}
						} else {
							if !pEntry.Id.Equal(entry.Id) {
								goto loop
							} else {
								if sameIdcnt == cm.ParentCount()-1 {
									// TODO: now follow the first parent commit?
									cm = cm.Parent(0)
									break
								}
								sameIdcnt = sameIdcnt + 1
							}
						}
					}
				}
			}

		loop:

			rp := &RepoFile{
				entry,
				path.Join(dirname, entry.Name),
				cm.Message(),
				cm.Committer.When,
				size,
				repo,
				cm.Id().String(),
			}

			if entry.IsFile() {
				repofiles = append(repofiles, rp)
			} else if entry.IsDir() {
				repodirs = append(repodirs, rp)
			}
		}
		return 0
	})

	return append(repodirs, repofiles...), nil
}

// GetLastestCommit returns the latest commit of given repository.
func GetLastestCommit(userName, repoName string) (*Commit, error) {
	stdout, _, err := com.ExecCmd("git", "--git-dir="+RepoPath(userName, repoName), "log", "-1")
	if err != nil {
		return nil, err
	}

	commit := new(Commit)
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) == 0 {
			continue
		}
		switch {
		case line[0] == 'c':
			commit.SHA = line[7:]
		case line[0] == 'A':
			infos := strings.SplitN(line, " ", 3)
			commit.Author = infos[1]
			commit.Email = infos[2][1 : len(infos[2])-1]
		case line[0] == 'D':
			commit.Date, err = time.Parse("Mon Jan 02 15:04:05 2006 -0700", line[8:])
			if err != nil {
				return nil, err
			}
		case line[:4] == "    ":
			commit.Message = line[4:]
		}
	}
	return commit, nil
}

// GetCommits returns all commits of given branch of repository.
func GetCommits(userName, reposName, branchname string) ([]*git.Commit, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}
	r, err := repo.LookupReference(fmt.Sprintf("refs/heads/%s", branchname))
	if err != nil {
		return nil, err
	}
	return r.AllCommits()
}
