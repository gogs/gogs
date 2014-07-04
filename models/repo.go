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
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/bin"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/process"
	"github.com/gogits/gogs/modules/setting"
)

const (
	TPL_UPDATE_HOOK = "#!/usr/bin/env %s\n%s update $1 $2 $3\n"
)

var (
	ErrRepoAlreadyExist  = errors.New("Repository already exist")
	ErrRepoNotExist      = errors.New("Repository does not exist")
	ErrRepoFileNotExist  = errors.New("Repository file does not exist")
	ErrRepoNameIllegal   = errors.New("Repository name contains illegal characters")
	ErrRepoFileNotLoaded = errors.New("Repository file not loaded")
	ErrMirrorNotExist    = errors.New("Mirror does not exist")
)

var (
	LanguageIgns, Licenses []string
)

// getAssetList returns corresponding asset list in 'conf'.
func getAssetList(prefix string) []string {
	assets := make([]string, 0, 15)
	for _, name := range bin.AssetNames() {
		if strings.HasPrefix(name, prefix) {
			assets = append(assets, strings.TrimPrefix(name, prefix+"/"))
		}
	}
	return assets
}

func LoadRepoConfig() {
	// Load .gitignore and license files.
	types := []string{"gitignore", "license"}
	typeFiles := make([][]string, 2)
	for i, t := range types {
		files := getAssetList(path.Join("conf", t))
		customPath := path.Join(setting.CustomPath, "conf", t)
		if com.IsDir(customPath) {
			customFiles, err := com.StatDir(customPath)
			if err != nil {
				log.Fatal("Fail to get custom %s files: %v", t, err)
			}

			for _, f := range customFiles {
				if !com.IsSliceContainsStr(files, f) {
					files = append(files, f)
				}
			}
		}
		typeFiles[i] = files
	}

	LanguageIgns = typeFiles[0]
	Licenses = typeFiles[1]
	sort.Strings(LanguageIgns)
	sort.Strings(Licenses)
}

func NewRepoContext() {
	zip.Verbose = false

	// Check if server has basic git setting.
	stdout, stderr, err := process.Exec("NewRepoContext(get setting)", "git", "config", "--get", "user.name")
	if strings.Contains(stderr, "fatal:") {
		log.Fatal("repo.NewRepoContext(fail to get git user.name): %s", stderr)
	} else if err != nil || len(strings.TrimSpace(stdout)) == 0 {
		if _, stderr, err = process.Exec("NewRepoContext(set email)", "git", "config", "--global", "user.email", "gogitservice@gmail.com"); err != nil {
			log.Fatal("repo.NewRepoContext(fail to set git user.email): %s", stderr)
		} else if _, stderr, err = process.Exec("NewRepoContext(set name)", "git", "config", "--global", "user.name", "Gogs"); err != nil {
			log.Fatal("repo.NewRepoContext(fail to set git user.name): %s", stderr)
		}
	}

	barePath := path.Join(setting.RepoRootPath, "git-bare.zip")
	if !com.IsExist(barePath) {
		data, err := bin.Asset("conf/content/git-bare.zip")
		if err != nil {
			log.Fatal("Fail to get asset 'git-bare.zip': %v", err)
		} else if err := ioutil.WriteFile(barePath, data, os.ModePerm); err != nil {
			log.Fatal("Fail to write asset 'git-bare.zip': %v", err)
		}
	}
}

// Repository represents a git repository.
type Repository struct {
	Id                  int64
	OwnerId             int64 `xorm:"UNIQUE(s)"`
	Owner               *User `xorm:"-"`
	ForkId              int64
	LowerName           string `xorm:"UNIQUE(s) INDEX NOT NULL"`
	Name                string `xorm:"INDEX NOT NULL"`
	Description         string
	Website             string
	NumWatches          int
	NumStars            int
	NumForks            int
	NumIssues           int
	NumClosedIssues     int
	NumOpenIssues       int `xorm:"-"`
	NumMilestones       int `xorm:"NOT NULL DEFAULT 0"`
	NumClosedMilestones int `xorm:"NOT NULL DEFAULT 0"`
	NumOpenMilestones   int `xorm:"-"`
	NumTags             int `xorm:"-"`
	IsPrivate           bool
	IsMirror            bool
	IsBare              bool
	IsGoget             bool
	DefaultBranch       string
	Created             time.Time `xorm:"CREATED"`
	Updated             time.Time `xorm:"UPDATED"`
}

func (repo *Repository) GetOwner() (err error) {
	repo.Owner, err = GetUserById(repo.OwnerId)
	return err
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(u *User, repoName string) (bool, error) {
	repo := Repository{OwnerId: u.Id}
	has, err := x.Where("lower_name = ?", strings.ToLower(repoName)).Get(&repo)
	if err != nil {
		return has, err
	} else if !has {
		return false, nil
	}

	return com.IsDir(RepoPath(u.Name, repoName)), nil
}

var (
	illegalEquals  = []string{"debug", "raw", "install", "api", "avatar", "user", "org", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin", "new"}
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

// MirrorRepository creates a mirror repository from source.
func MirrorRepository(repoId int64, userName, repoName, repoPath, url string) error {
	// TODO: need timeout.
	_, stderr, err := process.Exec(fmt.Sprintf("MirrorRepository: %s/%s", userName, repoName),
		"git", "clone", "--mirror", url, repoPath)
	if err != nil {
		return errors.New("git clone --mirror: " + stderr)
	}

	if _, err = x.InsertOne(&Mirror{
		RepoId:     repoId,
		RepoName:   strings.ToLower(userName + "/" + repoName),
		Interval:   24,
		NextUpdate: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return err
	}

	return git.UnpackRefs(repoPath)
}

func GetMirror(repoId int64) (*Mirror, error) {
	m := &Mirror{RepoId: repoId}
	has, err := x.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMirrorNotExist
	}
	return m, nil
}

func UpdateMirror(m *Mirror) error {
	_, err := x.Id(m.Id).Update(m)
	return err
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if err := x.Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.NextUpdate.After(time.Now()) {
			return nil
		}

		// TODO: need timeout.
		repoPath := filepath.Join(setting.RepoRootPath, m.RepoName+".git")
		if _, stderr, err := process.ExecDir(
			repoPath, fmt.Sprintf("MirrorUpdate: %s", repoPath),
			"git", "remote", "update"); err != nil {
			return errors.New("git remote update: " + stderr)
		} else if err = git.UnpackRefs(repoPath); err != nil {
			return errors.New("UnpackRefs: " + err.Error())
		}

		m.NextUpdate = time.Now().Add(time.Duration(m.Interval) * time.Hour)
		return UpdateMirror(m)
	}); err != nil {
		log.Error("repo.MirrorUpdate: %v", err)
	}
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(u *User, name, desc string, private, mirror bool, url string) (*Repository, error) {
	repo, err := CreateRepository(u, name, desc, "", "", private, mirror, false)
	if err != nil {
		return nil, err
	}

	// Clone to temprory path and do the init commit.
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()))
	os.MkdirAll(tmpDir, os.ModePerm)

	repoPath := RepoPath(u.Name, name)

	repo.IsBare = false
	if mirror {
		if err = MirrorRepository(repo.Id, u.Name, repo.Name, repoPath, url); err != nil {
			return repo, err
		}
		repo.IsMirror = true
		return repo, UpdateRepository(repo)
	}

	// TODO: need timeout.
	// Clone from local repository.
	_, stderr, err := process.Exec(
		fmt.Sprintf("MigrateRepository(git clone): %s", repoPath),
		"git", "clone", repoPath, tmpDir)
	if err != nil {
		return repo, errors.New("git clone: " + stderr)
	}

	// TODO: need timeout.
	// Pull data from source.
	if _, stderr, err = process.ExecDir(
		tmpDir, fmt.Sprintf("MigrateRepository(git pull): %s", repoPath),
		"git", "pull", url); err != nil {
		return repo, errors.New("git pull: " + stderr)
	}

	// TODO: need timeout.
	// Push data to local repository.
	if _, stderr, err = process.ExecDir(
		tmpDir, fmt.Sprintf("MigrateRepository(git push): %s", repoPath),
		"git", "push", "origin", "master"); err != nil {
		return repo, errors.New("git push: " + stderr)
	}

	return repo, UpdateRepository(repo)
}

// extractGitBareZip extracts git-bare.zip to repository path.
func extractGitBareZip(repoPath string) error {
	z, err := zip.Open(filepath.Join(setting.RepoRootPath, "git-bare.zip"))
	if err != nil {
		return err
	}
	defer z.Close()

	return z.ExtractTo(repoPath)
}

// initRepoCommit temporarily changes with work directory.
func initRepoCommit(tmpPath string, sig *git.Signature) (err error) {
	var stderr string
	if _, stderr, err = process.ExecDir(
		tmpPath, fmt.Sprintf("initRepoCommit(git add): %s", tmpPath),
		"git", "add", "--all"); err != nil {
		return errors.New("git add: " + stderr)
	}

	if _, stderr, err = process.ExecDir(
		tmpPath, fmt.Sprintf("initRepoCommit(git commit): %s", tmpPath),
		"git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return errors.New("git commit: " + stderr)
	}

	if _, stderr, err = process.ExecDir(
		tmpPath, fmt.Sprintf("initRepoCommit(git push): %s", tmpPath),
		"git", "push", "origin", "master"); err != nil {
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
func SetRepoEnvs(userId int64, userName, repoName, repoUserName string) {
	os.Setenv("userId", base.ToStr(userId))
	os.Setenv("userName", userName)
	os.Setenv("repoName", repoName)
	os.Setenv("repoUserName", repoUserName)
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(f string, user *User, repo *Repository, initReadme bool, repoLang, license string) error {
	repoPath := RepoPath(user.Name, repo.Name)

	// Create bare new repository.
	if err := extractGitBareZip(repoPath); err != nil {
		return err
	}

	rp := strings.NewReplacer("\\", "/", " ", "\\ ")
	// hook/post-update
	if err := createHookUpdate(filepath.Join(repoPath, "hooks", "update"),
		fmt.Sprintf(TPL_UPDATE_HOOK, setting.ScriptType,
			rp.Replace(appPath))); err != nil {
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
	tmpDir := filepath.Join(os.TempDir(), base.ToStr(time.Now().Nanosecond()))
	os.MkdirAll(tmpDir, os.ModePerm)

	_, stderr, err := process.Exec(
		fmt.Sprintf("initRepository(git clone): %s", repoPath),
		"git", "clone", repoPath, tmpDir)
	if err != nil {
		return errors.New("initRepository(git clone): " + stderr)
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
		targetPath := path.Join(tmpDir, fileName["gitign"])
		data, err := bin.Asset(filePath)
		if err == nil {
			if err = ioutil.WriteFile(targetPath, data, os.ModePerm); err != nil {
				return err
			}
		} else {
			// Check custom files.
			filePath = path.Join(setting.CustomPath, "conf/gitignore", repoLang)
			if com.IsFile(filePath) {
				if err := com.Copy(filePath, targetPath); err != nil {
					return err
				}
			}
		}
	}

	// LICENSE
	if license != "" {
		filePath := "conf/license/" + license
		targetPath := path.Join(tmpDir, fileName["license"])
		data, err := bin.Asset(filePath)
		if err == nil {
			if err = ioutil.WriteFile(targetPath, data, os.ModePerm); err != nil {
				return err
			}
		} else {
			// Check custom files.
			filePath = path.Join(setting.CustomPath, "conf/license", license)
			if com.IsFile(filePath) {
				if err := com.Copy(filePath, targetPath); err != nil {
					return err
				}
			}
		}
	}

	if len(fileName) == 0 {
		return nil
	}

	SetRepoEnvs(user.Id, user.Name, repo.Name, user.Name)

	// Apply changes and commit.
	return initRepoCommit(tmpDir, user.NewGitSig())
}

// CreateRepository creates a repository for given user or organization.
func CreateRepository(u *User, name, desc, lang, license string, private, mirror, initReadme bool) (*Repository, error) {
	if !IsLegalName(name) {
		return nil, ErrRepoNameIllegal
	}

	isExist, err := IsRepositoryExist(u, name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrRepoAlreadyExist
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	repo := &Repository{
		OwnerId:     u.Id,
		Owner:       u,
		Name:        name,
		LowerName:   strings.ToLower(name),
		Description: desc,
		IsPrivate:   private,
		IsBare:      lang == "" && license == "" && !initReadme,
	}
	if !repo.IsBare {
		repo.DefaultBranch = "master"
	}

	if _, err = sess.Insert(repo); err != nil {
		sess.Rollback()
		return nil, err
	}

	var t *Team // Owner team.

	mode := WRITABLE
	if mirror {
		mode = READABLE
	}
	access := &Access{
		UserName: u.LowerName,
		RepoName: strings.ToLower(path.Join(u.Name, repo.Name)),
		Mode:     mode,
	}
	// Give access to all members in owner team.
	if u.IsOrganization() {
		t, err = u.GetOwnerTeam()
		if err != nil {
			sess.Rollback()
			return nil, err
		}
		us, err := GetTeamMembers(u.Id, t.Id)
		if err != nil {
			sess.Rollback()
			return nil, err
		}
		for _, u := range us {
			access.UserName = u.LowerName
			if _, err = sess.Insert(access); err != nil {
				sess.Rollback()
				return nil, err
			}
		}
	} else {
		if _, err = sess.Insert(access); err != nil {
			sess.Rollback()
			return nil, err
		}
	}

	rawSql := "UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, u.Id); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Update owner team info and count.
	if u.IsOrganization() {
		t.RepoIds += "$" + base.ToStr(repo.Id) + "|"
		t.NumRepos++
		if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
			sess.Rollback()
			return nil, err
		}
	}

	if err = sess.Commit(); err != nil {
		return nil, err
	}

	if u.IsOrganization() {
		ous, err := GetOrgUsersByOrgId(u.Id)
		if err != nil {
			log.Error("repo.CreateRepository(GetOrgUsersByOrgId): %v", err)
		} else {
			for _, ou := range ous {
				if err = WatchRepo(ou.Uid, repo.Id, true); err != nil {
					log.Error("repo.CreateRepository(WatchRepo): %v", err)
				}
			}
		}
	}
	if err = WatchRepo(u.Id, repo.Id, true); err != nil {
		log.Error("repo.CreateRepository(WatchRepo2): %v", err)
	}

	if err = NewRepoAction(u, repo); err != nil {
		log.Error("repo.CreateRepository(NewRepoAction): %v", err)
	}

	// No need for init for mirror.
	if mirror {
		return repo, nil
	}

	repoPath := RepoPath(u.Name, repo.Name)
	if err = initRepository(repoPath, u, repo, initReadme, lang, license); err != nil {
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error("repo.CreateRepository(initRepository): %v", err)
			return nil, errors.New(fmt.Sprintf(
				"delete repo directory %s/%s failed(2): %v", u.Name, repo.Name, err2))
		}
		return nil, err
	}

	_, stderr, err := process.ExecDir(
		repoPath, fmt.Sprintf("CreateRepository(git update-server-info): %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, errors.New("CreateRepository(git update-server-info): " + stderr)
	}

	return repo, nil
}

// GetRepositoriesWithUsers returns given number of repository objects with offset.
// It also auto-gets corresponding users.
func GetRepositoriesWithUsers(num, offset int) ([]*Repository, error) {
	repos := make([]*Repository, 0, num)
	if err := x.Limit(num, offset).Asc("id").Find(&repos); err != nil {
		return nil, err
	}

	for _, repo := range repos {
		repo.Owner = &User{Id: repo.OwnerId}
		has, err := x.Get(repo.Owner)
		if err != nil {
			return nil, err
		} else if !has {
			return nil, ErrUserNotExist
		}
	}

	return repos, nil
}

// RepoPath returns repository path by given user and repository name.
func RepoPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), strings.ToLower(repoName)+".git")
}

// TransferOwnership transfers all corresponding setting from old user to new one.
func TransferOwnership(u *User, newOwner string, repo *Repository) (err error) {
	newUser, err := GetUserByName(newOwner)
	if err != nil {
		return err
	}

	// Update accesses.
	accesses := make([]Access, 0, 10)
	if err = x.Find(&accesses, &Access{RepoName: u.LowerName + "/" + repo.LowerName}); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for i := range accesses {
		accesses[i].RepoName = newUser.LowerName + "/" + repo.LowerName
		if accesses[i].UserName == u.LowerName {
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
	if _, err = sess.Exec(rawSql, u.Id); err != nil {
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

	if err = TransferRepoAction(u, newUser, repo); err != nil {
		sess.Rollback()
		return err
	}

	// Change repository directory name.
	if err = os.Rename(RepoPath(u.Name, repo.Name), RepoPath(newUser.Name, repo.Name)); err != nil {
		sess.Rollback()
		return err
	}

	return sess.Commit()
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(userName, oldRepoName, newRepoName string) (err error) {
	// Update accesses.
	accesses := make([]Access, 0, 10)
	if err = x.Find(&accesses, &Access{RepoName: strings.ToLower(userName + "/" + oldRepoName)}); err != nil {
		return err
	}

	sess := x.NewSession()
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
	_, err := x.Id(repo.Id).AllCols().Update(repo)
	return err
}

// DeleteRepository deletes a repository for a user or orgnaztion.
func DeleteRepository(userId, repoId int64, userName string) error {
	repo := &Repository{Id: repoId, OwnerId: userId}
	has, err := x.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist
	}

	sess := x.NewSession()
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
	if _, err = sess.Delete(&IssueUser{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&Milestone{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}
	if _, err = sess.Delete(&Release{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}

	// Delete comments.
	if err = x.Iterate(&Issue{RepoId: repoId}, func(idx int, bean interface{}) error {
		issue := bean.(*Issue)
		if _, err = sess.Delete(&Comment{IssueId: issue.Id}); err != nil {
			sess.Rollback()
			return err
		}
		return nil
	}); err != nil {
		sess.Rollback()
		return err
	}

	if _, err = sess.Delete(&Issue{RepoId: repoId}); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, userId); err != nil {
		sess.Rollback()
		return err
	}
	if err = os.RemoveAll(RepoPath(userName, repo.Name)); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// GetRepositoryByName returns the repository by given name under user if exists.
func GetRepositoryByName(userId int64, repoName string) (*Repository, error) {
	repo := &Repository{
		OwnerId:   userId,
		LowerName: strings.ToLower(repoName),
	}
	has, err := x.Get(repo)
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
	has, err := x.Id(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist
	}
	return repo, nil
}

// GetRepositories returns a list of repositories of given user.
func GetRepositories(uid int64, private bool) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	sess := x.Desc("updated")
	if !private {
		sess.Where("is_private=?", false)
	}

	err := sess.Find(&repos, &Repository{OwnerId: uid})
	return repos, err
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories() (repos []*Repository, err error) {
	err = x.Where("is_private=?", false).Limit(5).Desc("updated").Find(&repos)
	return repos, err
}

// GetRepositoryCount returns the total number of repositories of user.
func GetRepositoryCount(user *User) (int64, error) {
	return x.Count(&Repository{OwnerId: user.Id})
}

// GetCollaboratorNames returns a list of user name of repository's collaborators.
func GetCollaboratorNames(repoName string) ([]string, error) {
	accesses := make([]*Access, 0, 10)
	if err := x.Find(&accesses, &Access{RepoName: strings.ToLower(repoName)}); err != nil {
		return nil, err
	}

	names := make([]string, len(accesses))
	for i := range accesses {
		names[i] = accesses[i].UserName
	}
	return names, nil
}

// GetCollaborativeRepos returns a list of repositories that user is collaborator.
func GetCollaborativeRepos(uname string) ([]*Repository, error) {
	uname = strings.ToLower(uname)
	accesses := make([]*Access, 0, 10)
	if err := x.Find(&accesses, &Access{UserName: uname}); err != nil {
		return nil, err
	}

	repos := make([]*Repository, 0, 10)
	for _, access := range accesses {
		infos := strings.Split(access.RepoName, "/")
		if infos[0] == uname {
			continue
		}

		u, err := GetUserByName(infos[0])
		if err != nil {
			return nil, err
		}

		repo, err := GetRepositoryByName(u.Id, infos[1])
		if err != nil {
			return nil, err
		}
		repo.Owner = u
		repos = append(repos, repo)
	}
	return repos, nil
}

// GetCollaborators returns a list of users of repository's collaborators.
func GetCollaborators(repoName string) (us []*User, err error) {
	accesses := make([]*Access, 0, 10)
	if err = x.Find(&accesses, &Access{RepoName: strings.ToLower(repoName)}); err != nil {
		return nil, err
	}

	us = make([]*User, len(accesses))
	for i := range accesses {
		us[i], err = GetUserByName(accesses[i].UserName)
		if err != nil {
			return nil, err
		}
	}
	return us, nil
}

// Watch is connection request for receiving repository notifycation.
type Watch struct {
	Id     int64
	UserId int64 `xorm:"UNIQUE(watch)"`
	RepoId int64 `xorm:"UNIQUE(watch)"`
}

// Watch or unwatch repository.
func WatchRepo(uid, rid int64, watch bool) (err error) {
	if watch {
		if _, err = x.Insert(&Watch{RepoId: rid, UserId: uid}); err != nil {
			return err
		}

		rawSql := "UPDATE `repository` SET num_watches = num_watches + 1 WHERE id = ?"
		_, err = x.Exec(rawSql, rid)
	} else {
		if _, err = x.Delete(&Watch{0, uid, rid}); err != nil {
			return err
		}
		rawSql := "UPDATE `repository` SET num_watches = num_watches - 1 WHERE id = ?"
		_, err = x.Exec(rawSql, rid)
	}
	return err
}

// GetWatchers returns all watchers of given repository.
func GetWatchers(rid int64) ([]*Watch, error) {
	watches := make([]*Watch, 0, 10)
	err := x.Find(&watches, &Watch{RepoId: rid})
	return watches, err
}

// NotifyWatchers creates batch of actions for every watcher.
func NotifyWatchers(act *Action) error {
	// Add feeds for user self and all watchers.
	watches, err := GetWatchers(act.RepoId)
	if err != nil {
		return errors.New("repo.NotifyWatchers(get watches): " + err.Error())
	}

	// Add feed for actioner.
	act.UserId = act.ActUserId
	if _, err = x.InsertOne(act); err != nil {
		return errors.New("repo.NotifyWatchers(create action): " + err.Error())
	}

	for i := range watches {
		if act.ActUserId == watches[i].UserId {
			continue
		}

		act.Id = 0
		act.UserId = watches[i].UserId
		if _, err = x.InsertOne(act); err != nil {
			return errors.New("repo.NotifyWatchers(create action): " + err.Error())
		}
	}
	return nil
}

// IsWatching checks if user has watched given repository.
func IsWatching(uid, rid int64) bool {
	has, _ := x.Get(&Watch{0, uid, rid})
	return has
}

func ForkRepository(repoName string, uid int64) {

}
