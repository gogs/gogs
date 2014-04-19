// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
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
	ErrRepoFileNotLoaded = errors.New("repo file not loaded")
	ErrMirrorNotExist    = errors.New("Mirror does not exist")
)

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
}

// Repository represents a git repository.
type Repository struct {
	Id              int64
	OwnerId         int64 `xorm:"unique(s)"`
	Owner           *User `xorm:"-"`
	ForkId          int64
	LowerName       string `xorm:"unique(s) index not null"`
	Name            string `xorm:"index not null"`
	Description     string
	Website         string
	NumWatches      int
	NumStars        int
	NumForks        int
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int `xorm:"-"`
	NumTags         int `xorm:"-"`
	IsPrivate       bool
	IsMirror        bool
	IsBare          bool
	IsGoget         bool
	DefaultBranch   string
	Created         time.Time `xorm:"created"`
	Updated         time.Time `xorm:"updated"`
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(user *User, repoName string) (bool, error) {
	repo := Repository{OwnerId: user.Id}
	has, err := orm.Where("lower_name = ?", strings.ToLower(repoName)).Get(&repo)
	if err != nil {
		return has, err
	} else if !has {
		return false, nil
	}

	return com.IsDir(RepoPath(user.Name, repoName)), nil
}

var (
	illegalEquals  = []string{"raw", "install", "api", "avatar", "user", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin"}
	illegalSuffixs = []string{".git"}
)

// IsLegalName returns false if name contains illegal characters.
func IsLegalName(repoName string) bool {
	repoName = strings.ToLower(repoName)
	for _, char := range illegalEquals {
		if repoName == char {
			return false
		}
	}
	for _, char := range illegalSuffixs {
		if strings.HasSuffix(repoName, char) {
			return false
		}
	}
	return true
}

// Mirror represents a mirror information of repository.
type Mirror struct {
	Id         int64
	RepoId     int64
	RepoName   string    // <user name>/<repo name>
	Interval   int       // Hour.
	Updated    time.Time `xorm:"UPDATED"`
	NextUpdate time.Time
}

func GetMirror(repoId int64) (*Mirror, error) {
	m := &Mirror{RepoId: repoId}
	has, err := orm.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMirrorNotExist
	}
	return m, nil
}

func UpdateMirror(m *Mirror) error {
	_, err := orm.Id(m.Id).Update(m)
	return err
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if err := orm.Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.NextUpdate.After(time.Now()) {
			return nil
		}

		repoPath := filepath.Join(base.RepoRootPath, m.RepoName+".git")
		_, stderr, err := com.ExecCmdDir(repoPath, "git", "remote", "update")
		if err != nil {
			return err
		} else if strings.Contains(stderr, "fatal:") {
			return errors.New(stderr)
		} else if err = git.UnpackRefs(repoPath); err != nil {
			return err
		}

		m.NextUpdate = time.Now().Add(time.Duration(m.Interval) * time.Hour)
		return UpdateMirror(m)
	}); err != nil {
		log.Error("repo.MirrorUpdate: %v", err)
	}
}

// MirrorRepository creates a mirror repository from source.
func MirrorRepository(repoId int64, userName, repoName, repoPath, url string) error {
	_, stderr, err := com.ExecCmd("git", "clone", "--mirror", url, repoPath)
	if err != nil {
		return err
	} else if strings.Contains(stderr, "fatal:") {
		return errors.New(stderr)
	}

	if _, err = orm.InsertOne(&Mirror{
		RepoId:     repoId,
		RepoName:   strings.ToLower(userName + "/" + repoName),
		Interval:   24,
		NextUpdate: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return err
	}

	return git.UnpackRefs(repoPath)
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(user *User, name, desc string, private, mirror bool, url string) (*Repository, error) {
	repo, err := CreateRepository(user, name, desc, "", "", private, mirror, false)
	if err != nil {
		return nil, err
	}

	// Clone to temprory path and do the init commit.
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()))
	os.MkdirAll(tmpDir, os.ModePerm)

	repoPath := RepoPath(user.Name, name)

	repo.IsBare = false
	if mirror {
		if err = MirrorRepository(repo.Id, user.Name, repo.Name, repoPath, url); err != nil {
			return repo, err
		}
		repo.IsMirror = true
		return repo, UpdateRepository(repo)
	}

	// Clone from local repository.
	_, stderr, err := com.ExecCmd("git", "clone", repoPath, tmpDir)
	if err != nil {
		return repo, err
	} else if strings.Contains(stderr, "fatal:") {
		return repo, errors.New("git clone: " + stderr)
	}

	// Pull data from source.
	_, stderr, err = com.ExecCmdDir(tmpDir, "git", "pull", url)
	if err != nil {
		return repo, err
	} else if strings.Contains(stderr, "fatal:") {
		return repo, errors.New("git pull: " + stderr)
	}

	// Push data to local repository.
	if _, stderr, err = com.ExecCmdDir(tmpDir, "git", "push", "origin", "master"); err != nil {
		return repo, err
	} else if strings.Contains(stderr, "fatal:") {
		return repo, errors.New("git push: " + stderr)
	}

	return repo, UpdateRepository(repo)
}

// CreateRepository creates a repository for given user or orgnaziation.
func CreateRepository(user *User, name, desc, lang, license string, private, mirror, initReadme bool) (*Repository, error) {
	if !IsLegalName(name) {
		return nil, ErrRepoNameIllegal
	}

	isExist, err := IsRepositoryExist(user, name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrRepoAlreadyExist
	}

	repo := &Repository{
		OwnerId:       user.Id,
		Name:          name,
		LowerName:     strings.ToLower(name),
		Description:   desc,
		IsPrivate:     private,
		IsBare:        lang == "" && license == "" && !initReadme,
		DefaultBranch: "master",
	}
	repoPath := RepoPath(user.Name, repo.Name)

	sess := orm.NewSession()
	defer sess.Close()
	sess.Begin()

	if _, err = sess.Insert(repo); err != nil {
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(repo): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(1): %v", user.Name, repo.Name, err2))
		}
		sess.Rollback()
		return nil, err
	}

	mode := AU_WRITABLE
	if mirror {
		mode = AU_READABLE
	}
	access := Access{
		UserName: user.LowerName,
		RepoName: strings.ToLower(path.Join(user.Name, repo.Name)),
		Mode:     mode,
	}
	if _, err = sess.Insert(&access); err != nil {
		sess.Rollback()
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(access): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(2): %v", user.Name, repo.Name, err2))
		}
		return nil, err
	}

	rawSql := "UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, user.Id); err != nil {
		sess.Rollback()
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(repo count): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(3): %v", user.Name, repo.Name, err2))
		}
		return nil, err
	}

	if err = sess.Commit(); err != nil {
		sess.Rollback()
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(commit): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(3): %v", user.Name, repo.Name, err2))
		}
		return nil, err
	}

	if !repo.IsPrivate {
		if err = NewRepoAction(user, repo); err != nil {
			log.Error("repo.CreateRepository(NewRepoAction): %v", err)
		}
	}

	if err = WatchRepo(user.Id, repo.Id, true); err != nil {
		log.Error("repo.CreateRepository(WatchRepo): %v", err)
	}

	// No need for init for mirror.
	if mirror {
		return repo, nil
	}

	if err = initRepository(repoPath, user, repo, initReadme, lang, license); err != nil {
		return nil, err
	}

	c := exec.Command("git", "update-server-info")
	c.Dir = repoPath
	if err = c.Run(); err != nil {
		log.Error("repo.CreateRepository(exec update-server-info): %v", err)
	}

	return repo, nil
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
func initRepoCommit(tmpPath string, sig *git.Signature) (err error) {
	var stderr string
	if _, stderr, err = com.ExecCmdDir(tmpPath, "git", "add", "--all"); err != nil {
		return err
	} else if strings.Contains(stderr, "fatal:") {
		return errors.New("git add: " + stderr)
	}

	if _, stderr, err = com.ExecCmdDir(tmpPath, "git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return err
	} else if strings.Contains(stderr, "fatal:") {
		return errors.New("git commit: " + stderr)
	}

	if _, stderr, err = com.ExecCmdDir(tmpPath, "git", "push", "origin", "master"); err != nil {
		return err
	} else if strings.Contains(stderr, "fatal:") {
		return errors.New("git push: " + stderr)
	}
	return nil
}

func createHookUpdate(hookPath, content string) error {
	pu, err := os.OpenFile(hookPath, os.O_CREATE|os.O_WRONLY, 0777)
	if err != nil {
		return err
	}
	defer pu.Close()

	_, err = pu.WriteString(content)
	return err
}

// SetRepoEnvs sets environment variables for command update.
func SetRepoEnvs(userId int64, userName, repoName string) {
	os.Setenv("userId", base.ToStr(userId))
	os.Setenv("userName", userName)
	os.Setenv("repoName", repoName)
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(f string, user *User, repo *Repository, initReadme bool, repoLang, license string) error {
	repoPath := RepoPath(user.Name, repo.Name)

	// Create bare new repository.
	if err := extractGitBareZip(repoPath); err != nil {
		return err
	}

	// hook/post-update
	if err := createHookUpdate(filepath.Join(repoPath, "hooks", "update"),
		fmt.Sprintf("#!/usr/bin/env %s\n%s update $1 $2 $3\n", base.ScriptType,
			strings.Replace(appPath, "\\", "/", -1))); err != nil {
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

	_, stderr, err := com.ExecCmd("git", "clone", repoPath, tmpDir)
	if err != nil {
		return err
	} else if strings.Contains(stderr, "fatal:") {
		return errors.New("git clone: " + stderr)
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
			if err := com.Copy(filePath,
				filepath.Join(tmpDir, fileName["gitign"])); err != nil {
				return err
			}
		}
	}

	// LICENSE
	if license != "" {
		filePath := "conf/license/" + license
		if com.IsFile(filePath) {
			if err := com.Copy(filePath,
				filepath.Join(tmpDir, fileName["license"])); err != nil {
				return err
			}
		}
	}

	if len(fileName) == 0 {
		return nil
	}

	SetRepoEnvs(user.Id, user.Name, repo.Name)

	// Apply changes and commit.
	return initRepoCommit(tmpDir, user.NewGitSig())
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

// RepoPath returns repository path by given user and repository name.
func RepoPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), strings.ToLower(repoName)+".git")
}

// TransferOwnership transfers all corresponding setting from old user to new one.
func TransferOwnership(user *User, newOwner string, repo *Repository) (err error) {
	newUser, err := GetUserByName(newOwner)
	if err != nil {
		return err
	}

	// Update accesses.
	accesses := make([]Access, 0, 10)
	if err = orm.Find(&accesses, &Access{RepoName: user.LowerName + "/" + repo.LowerName}); err != nil {
		return err
	}

	sess := orm.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].RepoName = newUser.LowerName + "/" + repo.LowerName
		if accesses[i].UserName == user.LowerName {
			accesses[i].UserName = newUser.LowerName
		}
		if err = UpdateAccessWithSession(sess, &accesses[i]); err != nil {
			return err
		}
	}

	// Update repository.
	repo.OwnerId = newUser.Id
	if _, err := sess.Id(repo.Id).Update(repo); err != nil {
		sess.Rollback()
		return err
	}

	// Update user repository number.
	rawSql := "UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, newUser.Id); err != nil {
		sess.Rollback()
		return err
	}
	rawSql = "UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, user.Id); err != nil {
		sess.Rollback()
		return err
	}

	// Add watch of new owner to repository.
	if !IsWatching(newUser.Id, repo.Id) {
		if err = WatchRepo(newUser.Id, repo.Id, true); err != nil {
			sess.Rollback()
			return err
		}
	}

	if err = TransferRepoAction(user, newUser, repo); err != nil {
		sess.Rollback()
		return err
	}

	// Change repository directory name.
	if err = os.Rename(RepoPath(user.Name, repo.Name), RepoPath(newUser.Name, repo.Name)); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(userName, oldRepoName, newRepoName string) (err error) {
	// Update accesses.
	accesses := make([]Access, 0, 10)
	if err = orm.Find(&accesses, &Access{RepoName: strings.ToLower(userName + "/" + oldRepoName)}); err != nil {
		return err
	}

	sess := orm.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].RepoName = userName + "/" + newRepoName
		if err = UpdateAccessWithSession(sess, &accesses[i]); err != nil {
			return err
		}
	}

	// Change repository directory name.
	if err = os.Rename(RepoPath(userName, oldRepoName), RepoPath(userName, newRepoName)); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

func UpdateRepository(repo *Repository) error {
	repo.LowerName = strings.ToLower(repo.Name)

	if len(repo.Description) > 255 {
		repo.Description = repo.Description[:255]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}
	_, err := orm.Id(repo.Id).AllCols().Update(repo)
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

	sess := orm.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}
	if _, err = sess.Delete(&Repository{Id: repoId}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err := sess.Delete(&Access{RepoName: strings.ToLower(path.Join(userName, repo.Name))}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err := sess.Delete(&Action{RepoId: repo.Id}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&Watch{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&Mirror{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, userId); err != nil {
		sess.Rollback()
		return err
	}
	if err = sess.Commit(); err != nil {
		sess.Rollback()
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
func GetRepositoryById(id int64) (*Repository, error) {
	repo := &Repository{}
	has, err := orm.Id(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist
	}
	return repo, err
}

// GetRepositories returns the list of repositories of given user.
func GetRepositories(user *User, private bool) ([]Repository, error) {
	repos := make([]Repository, 0, 10)
	sess := orm.Desc("updated")
	if !private {
		sess.Where("is_private=?", false)
	}

	err := sess.Find(&repos, &Repository{OwnerId: user.Id})
	return repos, err
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories() (repos []*Repository, err error) {
	err = orm.Where("is_private=?", false).Limit(5).Desc("updated").Find(&repos)
	return repos, err
}

// GetRepositoryCount returns the total number of repositories of user.
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

// NotifyWatchers creates batch of actions for every watcher.
func NotifyWatchers(act *Action) error {
	// Add feeds for user self and all watchers.
	watches, err := GetWatches(act.RepoId)
	if err != nil {
		return errors.New("repo.NotifyWatchers(get watches): " + err.Error())
	}

	// Add feed for actioner.
	act.UserId = act.ActUserId
	if _, err = orm.InsertOne(act); err != nil {
		return errors.New("repo.NotifyWatchers(create action): " + err.Error())
	}

	for i := range watches {
		if act.ActUserId == watches[i].UserId {
			continue
		}

		act.Id = 0
		act.UserId = watches[i].UserId
		if _, err = orm.InsertOne(act); err != nil {
			return errors.New("repo.NotifyWatchers(create action): " + err.Error())
		}
	}
	return nil
}

// IsWatching checks if user has watched given repository.
func IsWatching(userId, repoId int64) bool {
	has, _ := orm.Get(&Watch{0, repoId, userId})
	return has
}

func ForkRepository(reposName string, userId int64) {

}
