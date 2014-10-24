// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
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
	ErrInvalidReference  = errors.New("Invalid reference specified")
)

var (
	Gitignores, Licenses []string
)

var (
	DescPattern = regexp.MustCompile(`https?://\S+`)
)

func LoadRepoConfig() {
	// Load .gitignore and license files.
	types := []string{"gitignore", "license"}
	typeFiles := make([][]string, 2)
	for i, t := range types {
		files, err := com.StatDir(path.Join("conf", t))
		if err != nil {
			log.Fatal(4, "Fail to get %s files: %v", t, err)
		}
		customPath := path.Join(setting.CustomPath, "conf", t)
		if com.IsDir(customPath) {
			customFiles, err := com.StatDir(customPath)
			if err != nil {
				log.Fatal(4, "Fail to get custom %s files: %v", t, err)
			}

			for _, f := range customFiles {
				if !com.IsSliceContainsStr(files, f) {
					files = append(files, f)
				}
			}
		}
		typeFiles[i] = files
	}

	Gitignores = typeFiles[0]
	Licenses = typeFiles[1]
	sort.Strings(Gitignores)
	sort.Strings(Licenses)
}

func NewRepoContext() {
	zip.Verbose = false

	// Check Git installation.
	if _, err := exec.LookPath("git"); err != nil {
		log.Fatal(4, "Fail to test 'git' command: %v (forgotten install?)", err)
	}

	// Check Git version.
	ver, err := git.GetVersion()
	if err != nil {
		log.Fatal(4, "Fail to get Git version: %v", err)
	}

	reqVer, err := git.ParseVersion("1.7.1")
	if err != nil {
		log.Fatal(4, "Fail to parse required Git version: %v", err)
	}
	if ver.LessThan(reqVer) {
		log.Fatal(4, "Gogs requires Git version greater or equal to 1.7.1")
	}

	// Check if server has basic git setting and set if not.
	if stdout, stderr, err := process.Exec("NewRepoContext(get setting)", "git", "config", "--get", "user.name"); err != nil || strings.TrimSpace(stdout) == "" {
		// ExitError indicates user.name is not set
		if _, ok := err.(*exec.ExitError); ok || strings.TrimSpace(stdout) == "" {
			stndrdUserName := "Gogs"
			stndrdUserEmail := "gogitservice@gmail.com"
			if _, stderr, gerr := process.Exec("NewRepoContext(set name)", "git", "config", "--global", "user.name", stndrdUserName); gerr != nil {
				log.Fatal(4, "Fail to set git user.name(%s): %s", gerr, stderr)
			}
			if _, stderr, gerr := process.Exec("NewRepoContext(set email)", "git", "config", "--global", "user.email", stndrdUserEmail); gerr != nil {
				log.Fatal(4, "Fail to set git user.email(%s): %s", gerr, stderr)
			}
			log.Info("Git user.name and user.email set to %s <%s>", stndrdUserName, stndrdUserEmail)
		} else {
			log.Fatal(4, "Fail to get git user.name(%s): %s", err, stderr)
		}
	}

	// Set git some configurations.
	if _, stderr, err := process.Exec("NewRepoContext(git config --global core.quotepath false)",
		"git", "config", "--global", "core.quotepath", "false"); err != nil {
		log.Fatal(4, "Fail to execute 'git config --global core.quotepath false': %s", stderr)
	}

}

// Repository represents a git repository.
type Repository struct {
	Id            int64
	OwnerId       int64  `xorm:"UNIQUE(s)"`
	Owner         *User  `xorm:"-"`
	LowerName     string `xorm:"UNIQUE(s) INDEX NOT NULL"`
	Name          string `xorm:"INDEX NOT NULL"`
	Description   string
	Website       string
	DefaultBranch string

	NumWatches          int
	NumStars            int
	NumForks            int
	NumIssues           int
	NumClosedIssues     int
	NumOpenIssues       int `xorm:"-"`
	NumPulls            int
	NumClosedPulls      int
	NumOpenPulls        int `xorm:"-"`
	NumMilestones       int `xorm:"NOT NULL DEFAULT 0"`
	NumClosedMilestones int `xorm:"NOT NULL DEFAULT 0"`
	NumOpenMilestones   int `xorm:"-"`
	NumTags             int `xorm:"-"`

	IsPrivate bool
	IsBare    bool
	IsGoget   bool

	IsMirror bool
	*Mirror  `xorm:"-"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false"`
	ForkId   int64
	ForkRepo *Repository `xorm:"-"`

	Created time.Time `xorm:"CREATED"`
	Updated time.Time `xorm:"UPDATED"`
}

func (repo *Repository) GetOwner() (err error) {
	if repo.Owner == nil {
		repo.Owner, err = GetUserById(repo.OwnerId)
	}
	return err
}

func (repo *Repository) GetMirror() (err error) {
	repo.Mirror, err = GetMirror(repo.Id)
	return err
}

func (repo *Repository) GetForkRepo() (err error) {
	if !repo.IsFork {
		return nil
	}

	repo.ForkRepo, err = GetRepositoryById(repo.ForkId)
	return err
}

func (repo *Repository) RepoPath() (string, error) {
	if err := repo.GetOwner(); err != nil {
		return "", err
	}
	return RepoPath(repo.Owner.Name, repo.Name), nil
}

func (repo *Repository) RepoLink() (string, error) {
	if err := repo.GetOwner(); err != nil {
		return "", err
	}
	return setting.AppSubUrl + "/" + repo.Owner.Name + "/" + repo.Name, nil
}

func (repo *Repository) IsOwnedBy(u *User) bool {
	return repo.OwnerId == u.Id
}

func (repo *Repository) HasAccess(uname string) bool {
	if err := repo.GetOwner(); err != nil {
		return false
	}
	has, _ := HasAccess(uname, path.Join(repo.Owner.Name, repo.Name), READABLE)
	return has
}

// DescriptionHtml does special handles to description and return HTML string.
func (repo *Repository) DescriptionHtml() template.HTML {
	sanitize := func(s string) string {
		// TODO(nuss-justin): Improve sanitization. Strip all tags?
		ss := html.EscapeString(s)
		return fmt.Sprintf(`<a href="%s" target="_blank">%s</a>`, ss, ss)
	}
	return template.HTML(DescPattern.ReplaceAllStringFunc(base.XSSString(repo.Description), sanitize))
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

// MirrorRepository creates a mirror repository from source.
func MirrorRepository(repoId int64, userName, repoName, repoPath, url string) error {
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("MirrorRepository: %s/%s", userName, repoName),
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
	return nil
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if err := x.Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.NextUpdate.After(time.Now()) {
			return nil
		}

		repoPath := filepath.Join(setting.RepoRootPath, m.RepoName+".git")
		if _, stderr, err := process.ExecDir(10*time.Minute,
			repoPath, fmt.Sprintf("MirrorUpdate: %s", repoPath),
			"git", "remote", "update"); err != nil {
			return errors.New("git remote update: " + stderr)
		}

		m.NextUpdate = time.Now().Add(time.Duration(m.Interval) * time.Hour)
		return UpdateMirror(m)
	}); err != nil {
		log.Error(4, "repo.MirrorUpdate: %v", err)
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

	if u.IsOrganization() {
		t, err := u.GetOwnerTeam()
		if err != nil {
			return nil, err
		}
		repo.NumWatches = t.NumMembers
	} else {
		repo.NumWatches = 1
	}

	repo.IsBare = false
	if mirror {
		if err = MirrorRepository(repo.Id, u.Name, repo.Name, repoPath, url); err != nil {
			return repo, err
		}
		repo.IsMirror = true
		return repo, UpdateRepository(repo)
	} else {
		os.RemoveAll(repoPath)
	}

	// this command could for both migrate and mirror
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("MigrateRepository: %s", repoPath),
		"git", "clone", "--mirror", "--bare", url, repoPath)
	if err != nil {
		return repo, errors.New("git clone: " + stderr)
	}
	return repo, UpdateRepository(repo)
}

// extractGitBareZip extracts git-bare.zip to repository path.
func extractGitBareZip(repoPath string) error {
	z, err := zip.Open(path.Join(setting.ConfRootPath, "content/git-bare.zip"))
	if err != nil {
		return err
	}
	defer z.Close()

	return z.ExtractTo(repoPath)
}

// initRepoCommit temporarily changes with work directory.
func initRepoCommit(tmpPath string, sig *git.Signature) (err error) {
	var stderr string
	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit(git add): %s", tmpPath),
		"git", "add", "--all"); err != nil {
		return errors.New("git add: " + stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit(git commit): %s", tmpPath),
		"git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Init commit"); err != nil {
		return errors.New("git commit: " + stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
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

// InitRepository initializes README and .gitignore if needed.
func initRepository(f string, u *User, repo *Repository, initReadme bool, repoLang, license string) error {
	repoPath := RepoPath(u.Name, repo.Name)

	// Create bare new repository.
	if err := extractGitBareZip(repoPath); err != nil {
		return err
	}

	// hook/post-update
	if err := createHookUpdate(filepath.Join(repoPath, "hooks", "update"),
		fmt.Sprintf(TPL_UPDATE_HOOK, setting.ScriptType, "\""+appPath+"\"")); err != nil {
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
	tmpDir := filepath.Join(os.TempDir(), com.ToStr(time.Now().Nanosecond()))
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
	filePath := "conf/gitignore/" + repoLang
	if com.IsFile(filePath) {
		targetPath := path.Join(tmpDir, fileName["gitign"])
		if com.IsFile(filePath) {
			if err = com.Copy(filePath, targetPath); err != nil {
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
	} else {
		delete(fileName, "gitign")
	}

	// LICENSE
	filePath = "conf/license/" + license
	if com.IsFile(filePath) {
		targetPath := path.Join(tmpDir, fileName["license"])
		if com.IsFile(filePath) {
			if err = com.Copy(filePath, targetPath); err != nil {
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
	} else {
		delete(fileName, "license")
	}

	if len(fileName) == 0 {
		repo.IsBare = true
		repo.DefaultBranch = "master"
		return UpdateRepository(repo)
	}

	// Apply changes and commit.
	return initRepoCommit(tmpDir, u.NewGitSig())
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
		RepoName: path.Join(u.LowerName, repo.LowerName),
		Mode:     mode,
	}
	// Give access to all members in owner team.
	if u.IsOrganization() {
		t, err = u.GetOwnerTeam()
		if err != nil {
			sess.Rollback()
			return nil, err
		}
		if err = t.GetMembers(); err != nil {
			sess.Rollback()
			return nil, err
		}
		for _, u := range t.Members {
			access.Id = 0
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

	if _, err = sess.Exec(
		"UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?", u.Id); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Update owner team info and count.
	if u.IsOrganization() {
		t.RepoIds += "$" + com.ToStr(repo.Id) + "|"
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
		t, err := u.GetOwnerTeam()
		if err != nil {
			log.Error(4, "GetOwnerTeam: %v", err)
		} else {
			if err = t.GetMembers(); err != nil {
				log.Error(4, "GetMembers: %v", err)
			} else {
				for _, u := range t.Members {
					if err = WatchRepo(u.Id, repo.Id, true); err != nil {
						log.Error(4, "WatchRepo2: %v", err)
					}
				}
			}
		}
	} else {
		if err = WatchRepo(u.Id, repo.Id, true); err != nil {
			log.Error(4, "WatchRepo3: %v", err)
		}
	}

	if err = NewRepoAction(u, repo); err != nil {
		log.Error(4, "NewRepoAction: %v", err)
	}

	// No need for init mirror.
	if mirror {
		return repo, nil
	}

	repoPath := RepoPath(u.Name, repo.Name)
	if err = initRepository(repoPath, u, repo, initReadme, lang, license); err != nil {
		if err2 := os.RemoveAll(repoPath); err2 != nil {
			log.Error(4, "initRepository: %v", err)
			return nil, fmt.Errorf(
				"delete repo directory %s/%s failed(2): %v", u.Name, repo.Name, err2)
		}
		return nil, fmt.Errorf("initRepository: %v", err)
	}

	_, stderr, err := process.ExecDir(-1,
		repoPath, fmt.Sprintf("CreateRepository(git update-server-info): %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, errors.New("CreateRepository(git update-server-info): " + stderr)
	}

	return repo, nil
}

// CountRepositories returns number of repositories.
func CountRepositories() int64 {
	count, _ := x.Count(new(Repository))
	return count
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
func TransferOwnership(u *User, newOwner string, repo *Repository) error {
	newUser, err := GetUserByName(newOwner)
	if err != nil {
		return fmt.Errorf("fail to get new owner(%s): %v", newOwner, err)
	}

	// Check if new owner has repository with same name.
	has, err := IsRepositoryExist(newUser, repo.Name)
	if err != nil {
		return err
	} else if has {
		return ErrRepoAlreadyExist
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	owner := repo.Owner
	oldRepoLink := path.Join(owner.LowerName, repo.LowerName)
	// Delete all access first if current owner is an organization.
	if owner.IsOrganization() {
		if _, err = sess.Where("repo_name=?", oldRepoLink).Delete(new(Access)); err != nil {
			sess.Rollback()
			return fmt.Errorf("fail to delete current accesses: %v", err)
		}
	} else {
		// Delete current owner access.
		if _, err = sess.Where("repo_name=?", oldRepoLink).And("user_name=?", owner.LowerName).
			Delete(new(Access)); err != nil {
			sess.Rollback()
			return fmt.Errorf("fail to delete access(owner): %v", err)
		}
		// In case new owner has access.
		if _, err = sess.Where("repo_name=?", oldRepoLink).And("user_name=?", newUser.LowerName).
			Delete(new(Access)); err != nil {
			sess.Rollback()
			return fmt.Errorf("fail to delete access(new user): %v", err)
		}
	}

	// Change accesses to new repository path.
	if _, err = sess.Where("repo_name=?", oldRepoLink).
		Update(&Access{RepoName: path.Join(newUser.LowerName, repo.LowerName)}); err != nil {
		sess.Rollback()
		return fmt.Errorf("fail to update access(change reponame): %v", err)
	}

	// Update repository.
	repo.OwnerId = newUser.Id
	if _, err := sess.Id(repo.Id).Update(repo); err != nil {
		sess.Rollback()
		return err
	}

	// Update user repository number.
	if _, err = sess.Exec("UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?", newUser.Id); err != nil {
		sess.Rollback()
		return err
	}

	if _, err = sess.Exec("UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?", owner.Id); err != nil {
		sess.Rollback()
		return err
	}

	mode := WRITABLE
	if repo.IsMirror {
		mode = READABLE
	}
	// New owner is organization.
	if newUser.IsOrganization() {
		access := &Access{
			RepoName: path.Join(newUser.LowerName, repo.LowerName),
			Mode:     mode,
		}

		// Give access to all members in owner team.
		t, err := newUser.GetOwnerTeam()
		if err != nil {
			sess.Rollback()
			return err
		}
		if err = t.GetMembers(); err != nil {
			sess.Rollback()
			return err
		}
		for _, u := range t.Members {
			access.Id = 0
			access.UserName = u.LowerName
			if _, err = sess.Insert(access); err != nil {
				sess.Rollback()
				return err
			}
		}

		// Update owner team info and count.
		t.RepoIds += "$" + com.ToStr(repo.Id) + "|"
		t.NumRepos++
		if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
			sess.Rollback()
			return err
		}
	} else {
		access := &Access{
			RepoName: path.Join(newUser.LowerName, repo.LowerName),
			UserName: newUser.LowerName,
			Mode:     mode,
		}
		if _, err = sess.Insert(access); err != nil {
			sess.Rollback()
			return fmt.Errorf("fail to insert access: %v", err)
		}
	}

	// Change repository directory name.
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newUser.Name, repo.Name)); err != nil {
		sess.Rollback()
		return err
	}

	if err = sess.Commit(); err != nil {
		return err
	}

	if err = WatchRepo(newUser.Id, repo.Id, true); err != nil {
		log.Error(4, "WatchRepo", err)
	}

	if err = TransferRepoAction(u, newUser, repo); err != nil {
		return err
	}

	return nil
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(userName, oldRepoName, newRepoName string) (err error) {
	if !IsLegalName(newRepoName) {
		return ErrRepoNameIllegal
	}

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
func DeleteRepository(uid, repoId int64, userName string) error {
	repo := &Repository{Id: repoId, OwnerId: uid}
	has, err := x.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist
	}

	// In case is a organization.
	org, err := GetUserById(uid)
	if err != nil {
		return err
	}
	if org.IsOrganization() {
		if err = org.GetTeams(); err != nil {
			return err
		}
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

	// Delete all access.
	if _, err := sess.Delete(&Access{RepoName: strings.ToLower(path.Join(userName, repo.Name))}); err != nil {
		sess.Rollback()
		return err
	}
	if org.IsOrganization() {
		idStr := "$" + com.ToStr(repoId) + "|"
		for _, t := range org.Teams {
			if !strings.Contains(t.RepoIds, idStr) {
				continue
			}
			t.NumRepos--
			t.RepoIds = strings.Replace(t.RepoIds, idStr, "", 1)
			if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
				sess.Rollback()
				return err
			}
		}
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

	if repo.IsFork {
		if _, err = sess.Exec("UPDATE `repository` SET num_forks = num_forks - 1 WHERE id = ?", repo.ForkId); err != nil {
			sess.Rollback()
			return err
		}
	}

	if _, err = sess.Exec("UPDATE `user` SET num_repos = num_repos - 1 WHERE id = ?", uid); err != nil {
		sess.Rollback()
		return err
	}

	// Remove repository files.
	if err = os.RemoveAll(RepoPath(userName, repo.Name)); err != nil {
		desc := fmt.Sprintf("Fail to delete repository files(%s/%s): %v", userName, repo.Name, err)
		log.Warn(desc)
		if err = CreateRepositoryNotice(desc); err != nil {
			log.Error(4, "Fail to add notice: %v", err)
		}
	}
	return sess.Commit()
}

// GetRepositoryByRef returns a Repository specified by a GFM reference.
// See https://help.github.com/articles/writing-on-github#references for more information on the syntax.
func GetRepositoryByRef(ref string) (*Repository, error) {
	n := strings.IndexByte(ref, byte('/'))

	if n < 2 {
		return nil, ErrInvalidReference
	}

	userName, repoName := ref[:n], ref[n+1:]

	user, err := GetUserByName(userName)

	if err != nil {
		return nil, err
	}

	return GetRepositoryByName(user.Id, repoName)
}

// GetRepositoryByName returns the repository by given name under user if exists.
func GetRepositoryByName(uid int64, repoName string) (*Repository, error) {
	repo := &Repository{
		OwnerId:   uid,
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
func GetRecentUpdatedRepositories(num int) (repos []*Repository, err error) {
	err = x.Where("is_private=?", false).Limit(num).Desc("updated").Find(&repos)
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

type SearchOption struct {
	Keyword string
	Uid     int64
	Limit   int
}

// SearchRepositoryByName returns given number of repositories whose name contains keyword.
func SearchRepositoryByName(opt SearchOption) (repos []*Repository, err error) {
	// Prevent SQL inject.
	opt.Keyword = strings.TrimSpace(opt.Keyword)
	if len(opt.Keyword) == 0 {
		return repos, nil
	}

	opt.Keyword = strings.Split(opt.Keyword, " ")[0]
	if len(opt.Keyword) == 0 {
		return repos, nil
	}
	opt.Keyword = strings.ToLower(opt.Keyword)

	repos = make([]*Repository, 0, opt.Limit)

	// Append conditions.
	sess := x.Limit(opt.Limit)
	if opt.Uid > 0 {
		sess.Where("owner_id=?", opt.Uid)
	}
	sess.And("lower_name like '%" + opt.Keyword + "%'").Find(&repos)
	return repos, err
}

//  __      __         __         .__
// /  \    /  \_____ _/  |_  ____ |  |__
// \   \/\/   /\__  \\   __\/ ___\|  |  \
//  \        /  / __ \|  | \  \___|   Y  \
//   \__/\  /  (____  /__|  \___  >___|  /
//        \/        \/          \/     \/

// Watch is connection request for receiving repository notifycation.
type Watch struct {
	Id     int64
	UserId int64 `xorm:"UNIQUE(watch)"`
	RepoId int64 `xorm:"UNIQUE(watch)"`
}

// IsWatching checks if user has watched given repository.
func IsWatching(uid, repoId int64) bool {
	has, _ := x.Get(&Watch{0, uid, repoId})
	return has
}

func watchRepoWithEngine(e Engine, uid, repoId int64, watch bool) (err error) {
	if watch {
		if IsWatching(uid, repoId) {
			return nil
		}
		if _, err = e.Insert(&Watch{RepoId: repoId, UserId: uid}); err != nil {
			return err
		}
		_, err = e.Exec("UPDATE `repository` SET num_watches = num_watches + 1 WHERE id = ?", repoId)
	} else {
		if !IsWatching(uid, repoId) {
			return nil
		}
		if _, err = e.Delete(&Watch{0, uid, repoId}); err != nil {
			return err
		}
		_, err = e.Exec("UPDATE `repository` SET num_watches = num_watches - 1 WHERE id = ?", repoId)
	}
	return err
}

// Watch or unwatch repository.
func WatchRepo(uid, repoId int64, watch bool) (err error) {
	return watchRepoWithEngine(x, uid, repoId, watch)
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

//   _________ __
//  /   _____//  |______ _______
//  \_____  \\   __\__  \\_  __ \
//  /        \|  |  / __ \|  | \/
// /_______  /|__| (____  /__|
//         \/           \/

type Star struct {
	Id     int64
	Uid    int64 `xorm:"UNIQUE(s)"`
	RepoId int64 `xorm:"UNIQUE(s)"`
}

// Star or unstar repository.
func StarRepo(uid, repoId int64, star bool) (err error) {
	if star {
		if IsStaring(uid, repoId) {
			return nil
		}
		if _, err = x.Insert(&Star{Uid: uid, RepoId: repoId}); err != nil {
			return err
		} else if _, err = x.Exec("UPDATE `repository` SET num_stars = num_stars + 1 WHERE id = ?", repoId); err != nil {
			return err
		}
		_, err = x.Exec("UPDATE `user` SET num_stars = num_stars + 1 WHERE id = ?", uid)
	} else {
		if !IsStaring(uid, repoId) {
			return nil
		}
		if _, err = x.Delete(&Star{0, uid, repoId}); err != nil {
			return err
		} else if _, err = x.Exec("UPDATE `repository` SET num_stars = num_stars - 1 WHERE id = ?", repoId); err != nil {
			return err
		}
		_, err = x.Exec("UPDATE `user` SET num_stars = num_stars - 1 WHERE id = ?", uid)
	}
	return err
}

// IsStaring checks if user has starred given repository.
func IsStaring(uid, repoId int64) bool {
	has, _ := x.Get(&Star{0, uid, repoId})
	return has
}

// ___________           __
// \_   _____/__________|  | __
//  |    __)/  _ \_  __ \  |/ /
//  |     \(  <_> )  | \/    <
//  \___  / \____/|__|  |__|_ \
//      \/                   \/

func ForkRepository(u *User, oldRepo *Repository) (*Repository, error) {
	isExist, err := IsRepositoryExist(u, oldRepo.Name)
	if err != nil {
		return nil, err
	} else if isExist {
		return nil, ErrRepoAlreadyExist
	}

	// In case the old repository is a fork.
	if oldRepo.IsFork {
		oldRepo, err = GetRepositoryById(oldRepo.ForkId)
		if err != nil {
			return nil, err
		}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	repo := &Repository{
		OwnerId:     u.Id,
		Owner:       u,
		Name:        oldRepo.Name,
		LowerName:   oldRepo.LowerName,
		Description: oldRepo.Description,
		IsPrivate:   oldRepo.IsPrivate,
		IsFork:      true,
		ForkId:      oldRepo.Id,
	}

	if _, err = sess.Insert(repo); err != nil {
		sess.Rollback()
		return nil, err
	}

	var t *Team // Owner team.

	mode := WRITABLE

	access := &Access{
		UserName: u.LowerName,
		RepoName: path.Join(u.LowerName, repo.LowerName),
		Mode:     mode,
	}
	// Give access to all members in owner team.
	if u.IsOrganization() {
		t, err = u.GetOwnerTeam()
		if err != nil {
			sess.Rollback()
			return nil, err
		}
		if err = t.GetMembers(); err != nil {
			sess.Rollback()
			return nil, err
		}
		for _, u := range t.Members {
			access.Id = 0
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

	if _, err = sess.Exec(
		"UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?", u.Id); err != nil {
		sess.Rollback()
		return nil, err
	}

	// Update owner team info and count.
	if u.IsOrganization() {
		t.RepoIds += "$" + com.ToStr(repo.Id) + "|"
		t.NumRepos++
		if _, err = sess.Id(t.Id).AllCols().Update(t); err != nil {
			sess.Rollback()
			return nil, err
		}
	}

	if u.IsOrganization() {
		t, err := u.GetOwnerTeam()
		if err != nil {
			log.Error(4, "GetOwnerTeam: %v", err)
		} else {
			if err = t.GetMembers(); err != nil {
				log.Error(4, "GetMembers: %v", err)
			} else {
				for _, u := range t.Members {
					if err = watchRepoWithEngine(sess, u.Id, repo.Id, true); err != nil {
						log.Error(4, "WatchRepo2: %v", err)
					}
				}
			}
		}
	} else {
		if err = watchRepoWithEngine(sess, u.Id, repo.Id, true); err != nil {
			log.Error(4, "WatchRepo3: %v", err)
		}
	}

	if err = NewRepoAction(u, repo); err != nil {
		log.Error(4, "NewRepoAction: %v", err)
	}

	if _, err = sess.Exec(
		"UPDATE `repository` SET num_forks = num_forks + 1 WHERE id = ?", oldRepo.Id); err != nil {
		sess.Rollback()
		return nil, err
	}

	oldRepoPath, err := oldRepo.RepoPath()
	if err != nil {
		sess.Rollback()
		return nil, fmt.Errorf("fail to get repo path(%s): %v", oldRepo.Name, err)
	}

	if err = sess.Commit(); err != nil {
		return nil, err
	}

	repoPath := RepoPath(u.Name, repo.Name)
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("ForkRepository(git clone): %s/%s", u.Name, repo.Name),
		"git", "clone", "--bare", oldRepoPath, repoPath)
	if err != nil {
		return nil, errors.New("ForkRepository(git clone): " + stderr)
	}

	_, stderr, err = process.ExecDir(-1,
		repoPath, fmt.Sprintf("ForkRepository(git update-server-info): %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, errors.New("ForkRepository(git update-server-info): " + stderr)
	}

	return repo, nil
}
