// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
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

var (
	ErrRepoAlreadyExist  = errors.New("Repository already exist")
	ErrRepoNotExist      = errors.New("Repository does not exist")
	ErrRepoFileNotExist  = errors.New("Target Repo file does not exist")
	ErrRepoNameIllegal   = errors.New("Repository name contains illegal characters")
	ErrRepoFileNotLoaded = fmt.Errorf("repo file not loaded")
)

var gitInitLocker = sync.Mutex{}

var (
	LanguageIgns, Licenses []string
)

func LoadRepoConfig() {
	LanguageIgns = strings.Split(base.Cfg.MustValue("repository", "LANG_IGNS"), "|")
	Licenses = strings.Split(base.Cfg.MustValue("repository", "LICENSES"), "|")
}

func NewRepoContext() {
	zip.Verbose = false

	// Check if server has basic git setting.
	stdout, _, err := com.ExecCmd("git", "config", "--get", "user.name")
	if err != nil {
		fmt.Printf("repo.init(fail to get git user.name): %v", err)
		os.Exit(2)
	} else if len(stdout) == 0 {
		if _, _, err = com.ExecCmd("git", "config", "--global", "user.email", "gogitservice@gmail.com"); err != nil {
			fmt.Printf("repo.init(fail to set git user.email): %v", err)
			os.Exit(2)
		} else if _, _, err = com.ExecCmd("git", "config", "--global", "user.name", "Gogs"); err != nil {
			fmt.Printf("repo.init(fail to set git user.name): %v", err)
			os.Exit(2)
		}
	}

	// Initialize illegal patterns.
	for i := range illegalPatterns[1:] {
		pattern := ""
		for j := range illegalPatterns[i+1] {
			pattern += "[" + string(illegalPatterns[i+1][j]-32) + string(illegalPatterns[i+1][j]) + "]"
		}
		illegalPatterns[i+1] = pattern
	}
}

// Repository represents a git repository.
type Repository struct {
	Id          int64
	OwnerId     int64 `xorm:"unique(s)"`
	ForkId      int64
	LowerName   string `xorm:"unique(s) index not null"`
	Name        string `xorm:"index not null"`
	Description string
	Website     string
	NumWatches  int
	NumStars    int
	NumForks    int
	IsPrivate   bool
	IsBare      bool
	Created     time.Time `xorm:"created"`
	Updated     time.Time `xorm:"updated"`
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

var (
	// Define as all lower case!!
	illegalPatterns = []string{"[.][Gg][Ii][Tt]", "user", "help", "stars", "issues", "pulls", "commits", "admin", "repo", "template", "admin"}
)

// IsLegalName returns false if name contains illegal characters.
func IsLegalName(repoName string) bool {
	for _, pattern := range illegalPatterns {
		has, _ := regexp.MatchString(pattern, repoName)
		if has {
			return false
		}
	}
	return true
}

// CreateRepository creates a repository for given user or orgnaziation.
func CreateRepository(user *User, repoName, desc, repoLang, license string, private bool, initReadme bool) (*Repository, error) {
	if !IsLegalName(repoName) {
		return nil, ErrRepoNameIllegal
	}

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
		IsPrivate:   private,
		IsBare:      repoLang == "" && license == "" && !initReadme,
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

	rawSql := "UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?"
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

	c := exec.Command("git", "update-server-info")
	c.Dir = repoPath
	err = c.Run()
	if err != nil {
		log.Error("repo.CreateRepository(exec update-server-info): %v", err)
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

	var stderr string
	if _, stderr, err = com.ExecCmd("git", "add", "--all"); err != nil {
		return err
	}
	log.Info("stderr(1): %s", stderr)
	if _, stderr, err = com.ExecCmd("git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return err
	}
	log.Info("stderr(2): %s", stderr)
	if _, stderr, err = com.ExecCmd("git", "push", "origin", "master"); err != nil {
		return err
	}
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

	// hook/post-update
	pu, err := os.OpenFile(filepath.Join(repoPath, "hooks", "post-update"), os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	defer pu.Close()
	// TODO: Windows .bat
	if _, err = pu.WriteString(fmt.Sprintf("#!/usr/bin/env bash\n%s update\n", appPath)); err != nil {
		return err
	}

	// hook/post-update
	pu2, err := os.OpenFile(filepath.Join(repoPath, "hooks", "post-receive"), os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	defer pu2.Close()
	// TODO: Windows .bat
	if _, err = pu2.WriteString("#!/usr/bin/env bash\ngit update-server-info\n"); err != nil {
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

	if len(fileName) == 0 {
		return nil
	}

	// Apply changes and commit.
	if err := initRepoCommit(tmpDir, user.NewGitSig()); err != nil {
		return err
	}
	return nil
}

// UserRepo reporesents a repository with user name.
type UserRepo struct {
	*Repository
	UserName string
}

// GetRepos returns given number of repository objects with offset.
func GetRepos(num, offset int) ([]UserRepo, error) {
	repos := make([]Repository, 0, num)
	if err := orm.Limit(num, offset).Asc("id").Find(&repos); err != nil {
		return nil, err
	}

	urepos := make([]UserRepo, len(repos))
	for i := range repos {
		urepos[i].Repository = &repos[i]
		u := new(User)
		has, err := orm.Id(urepos[i].Repository.OwnerId).Get(u)
		if err != nil {
			return nil, err
		} else if !has {
			return nil, ErrUserNotExist
		}
		urepos[i].UserName = u.Name
	}

	return urepos, nil
}

func RepoPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), repoName+".git")
}

func UpdateRepository(repo *Repository) error {
	if len(repo.Description) > 255 {
		repo.Description = repo.Description[:255]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	_, err := orm.Id(repo.Id).UseBool().Cols("description", "website").Update(repo)
	return err
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
	rawSql := "UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?"
	if _, err = session.Exec(rawSql, userId); err != nil {
		session.Rollback()
		return err
	}
	if _, err = session.Delete(&Watch{RepoId: repoId}); err != nil {
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

// GetRepositoryByName returns the repository by given name under user if exists.
func GetRepositoryByName(userId int64, repoName string) (*Repository, error) {
	repo := &Repository{
		OwnerId:   userId,
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

// Watch is connection request for receiving repository notifycation.
type Watch struct {
	Id     int64
	RepoId int64 `xorm:"UNIQUE(watch)"`
	UserId int64 `xorm:"UNIQUE(watch)"`
}

// Watch or unwatch repository.
func WatchRepo(userId, repoId int64, watch bool) (err error) {
	if watch {
		if _, err = orm.Insert(&Watch{RepoId: repoId, UserId: userId}); err != nil {
			return err
		}

		rawSql := "UPDATE `repository` SET num_watches = num_watches + 1 WHERE id = ?"
		_, err = orm.Exec(rawSql, repoId)
	} else {
		if _, err = orm.Delete(&Watch{0, repoId, userId}); err != nil {
			return err
		}
		rawSql := "UPDATE `repository` SET num_watches = num_watches - 1 WHERE id = ?"
		_, err = orm.Exec(rawSql, repoId)
	}
	return err
}

// GetWatches returns all watches of given repository.
func GetWatches(repoId int64) ([]Watch, error) {
	watches := make([]Watch, 0, 10)
	err := orm.Find(&watches, &Watch{RepoId: repoId})
	return watches, err
}

// IsWatching checks if user has watched given repository.
func IsWatching(userId, repoId int64) bool {
	has, _ := orm.Get(&Watch{0, repoId, userId})
	return has
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

// RepoFile represents a file object in git repository.
type RepoFile struct {
	*git.TreeEntry
	Path   string
	Size   int64
	Repo   *git.Repository
	Commit *git.Commit
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

func GetTargetFile(userName, reposName, branchName, commitId, rpath string) (*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(branchName, commitId)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(path.Clean(rpath), "/")

	var entry *git.TreeEntry
	tree := commit.Tree
	for i, part := range parts {
		if i == len(parts)-1 {
			entry = tree.EntryByName(part)
			if entry == nil {
				return nil, ErrRepoFileNotExist
			}
		} else {
			tree, err = repo.SubTree(tree, part)
			if err != nil {
				return nil, err
			}
		}
	}

	size, err := repo.ObjectSize(entry.Id)
	if err != nil {
		return nil, err
	}

	repoFile := &RepoFile{
		entry,
		rpath,
		size,
		repo,
		commit,
	}

	return repoFile, nil
}

// GetReposFiles returns a list of file object in given directory of repository.
func GetReposFiles(userName, reposName, branchName, commitId, rpath string) ([]*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(branchName, commitId)
	if err != nil {
		return nil, err
	}

	var repodirs []*RepoFile
	var repofiles []*RepoFile
	commit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			// TODO: size get method shoule be improved
			size, err := repo.ObjectSize(entry.Id)
			if err != nil {
				return 0
			}

			var cm = commit
			var i int
			for {
				i = i + 1
				//fmt.Println(".....", i, cm.Id(), cm.ParentCount())
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
					var lastSameCm *git.Commit
					//fmt.Println(".....", cm.ParentCount())
					for i := 0; i < cm.ParentCount(); i++ {
						//fmt.Println("parent", i, cm.Parent(i).Id())
						p := cm.Parent(i)
						pt, _ := repo.SubTree(p.Tree, dirname)
						var pEntry *git.TreeEntry
						if pt != nil {
							pEntry = pt.EntryByName(entry.Name)
						}

						//fmt.Println("pEntry", pEntry)

						if pEntry == nil {
							emptyCnt = emptyCnt + 1
							if emptyCnt+sameIdcnt == cm.ParentCount() {
								if lastSameCm == nil {
									goto loop
								} else {
									cm = lastSameCm
									break
								}
							}
						} else {
							//fmt.Println(i, "pEntry", pEntry.Id, "entry", entry.Id)
							if !pEntry.Id.Equal(entry.Id) {
								goto loop
							} else {
								lastSameCm = cm.Parent(i)
								sameIdcnt = sameIdcnt + 1
								if emptyCnt+sameIdcnt == cm.ParentCount() {
									// TODO: now follow the first parent commit?
									cm = lastSameCm
									//fmt.Println("sameId...")
									break
								}
							}
						}
					}
				}
			}

		loop:

			rp := &RepoFile{
				entry,
				path.Join(dirname, entry.Name),
				size,
				repo,
				cm,
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

func GetCommit(userName, repoName, branchname, commitid string) (*git.Commit, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	return repo.GetCommit(branchname, commitid)
}

// GetCommits returns all commits of given branch of repository.
func GetCommits(userName, reposName, branchname string) (*list.List, error) {
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
