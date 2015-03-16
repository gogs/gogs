// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"fmt"
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
	_TPL_UPDATE_HOOK = "#!/usr/bin/env %s\n%s update $1 $2 $3 --config='%s'\n"
)

var (
	ErrRepoAlreadyExist  = errors.New("Repository already exist")
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

	// Check if server has user.email and user.name set correctly and set if they're not.
	for configKey, defaultValue := range map[string]string{"user.name": "Gogs", "user.email": "gogitservice@gmail.com"} {
		if stdout, stderr, err := process.Exec("NewRepoContext(get setting)", "git", "config", "--get", configKey); err != nil || strings.TrimSpace(stdout) == "" {
			// ExitError indicates this config is not set
			if _, ok := err.(*exec.ExitError); ok || strings.TrimSpace(stdout) == "" {
				if _, stderr, gerr := process.Exec("NewRepoContext(set "+configKey+")", "git", "config", "--global", configKey, defaultValue); gerr != nil {
					log.Fatal(4, "Fail to set git %s(%s): %s", configKey, gerr, stderr)
				}
				log.Info("Git config %s set to %s", configKey, defaultValue)
			} else {
				log.Fatal(4, "Fail to get git %s(%s): %s", configKey, err, stderr)
			}
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

	IsMirror bool
	*Mirror  `xorm:"-"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false"`
	ForkId   int64
	ForkRepo *Repository `xorm:"-"`

	Created time.Time `xorm:"CREATED"`
	Updated time.Time `xorm:"UPDATED"`
}

func (repo *Repository) getOwner(e Engine) (err error) {
	if repo.Owner == nil {
		repo.Owner, err = getUserById(e, repo.OwnerId)
	}
	return err
}

func (repo *Repository) GetOwner() (err error) {
	return repo.getOwner(x)
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

func (repo *Repository) HasAccess(u *User) bool {
	has, _ := HasAccess(u, repo, ACCESS_MODE_READ)
	return has
}

func (repo *Repository) IsOwnedBy(u *User) bool {
	return repo.OwnerId == u.Id
}

// DescriptionHtml does special handles to description and return HTML string.
func (repo *Repository) DescriptionHtml() template.HTML {
	sanitize := func(s string) string {
		return fmt.Sprintf(`<a href="%[1]s" target="_blank">%[1]s</a>`, s)
	}
	return template.HTML(DescPattern.ReplaceAllStringFunc(base.Sanitizer.Sanitize(repo.Description), sanitize))
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(u *User, repoName string) bool {
	has, _ := x.Get(&Repository{
		OwnerId:   u.Id,
		LowerName: strings.ToLower(repoName),
	})
	return has && com.IsDir(RepoPath(u.Name, repoName))
}

// CloneLink represents different types of clone URLs of repository.
type CloneLink struct {
	SSH   string
	HTTPS string
	Git   string
}

// CloneLink returns clone URLs of repository.
func (repo *Repository) CloneLink() (cl CloneLink, err error) {
	if err = repo.GetOwner(); err != nil {
		return cl, err
	}
	if setting.SSHPort != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", setting.RunUser, setting.Domain, setting.SSHPort, repo.Owner.LowerName, repo.LowerName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", setting.RunUser, setting.Domain, repo.Owner.LowerName, repo.LowerName)
	}
	cl.HTTPS = fmt.Sprintf("%s%s/%s.git", setting.AppUrl, repo.Owner.LowerName, repo.LowerName)
	return cl, nil
}

var (
	illegalEquals  = []string{"debug", "raw", "install", "api", "avatar", "user", "org", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin", "new"}
	illegalSuffixs = []string{".git", ".keys"}
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

	// FIXME: this command could for both migrate and mirror
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("MigrateRepository: %s", repoPath),
		"git", "clone", "--mirror", "--bare", url, repoPath)
	if err != nil {
		return repo, fmt.Errorf("git clone --mirror --bare: %v", stderr)
	} else if err = createUpdateHook(repoPath); err != nil {
		return repo, fmt.Errorf("create update hook: %v", err)
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

func createUpdateHook(repoPath string) error {
	return ioutil.WriteFile(path.Join(repoPath, "hooks/update"),
		[]byte(fmt.Sprintf(_TPL_UPDATE_HOOK, setting.ScriptType, "\""+appPath+"\"", setting.CustomConf)), 0777)
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(e Engine, f string, u *User, repo *Repository, initReadme bool, repoLang, license string) error {
	repoPath := RepoPath(u.Name, repo.Name)

	// Create bare new repository.
	if err := extractGitBareZip(repoPath); err != nil {
		return err
	}

	if err := createUpdateHook(repoPath); err != nil {
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
		// Re-fetch the repository from database before updating it (else it would
		// override changes that were done earlier with sql)
		if repo, err = getRepositoryById(e, repo.Id); err != nil {
			return err
		}
		repo.IsBare = true
		repo.DefaultBranch = "master"
		return updateRepository(e, repo)
	}

	// Apply changes and commit.
	return initRepoCommit(tmpDir, u.NewGitSig())
}

// CreateRepository creates a repository for given user or organization.
func CreateRepository(u *User, name, desc, lang, license string, isPrivate, isMirror, initReadme bool) (_ *Repository, err error) {
	if !IsLegalName(name) {
		return nil, ErrRepoNameIllegal
	}

	if IsRepositoryExist(u, name) {
		return nil, ErrRepoAlreadyExist
	}

	repo := &Repository{
		OwnerId:     u.Id,
		Owner:       u,
		Name:        name,
		LowerName:   strings.ToLower(name),
		Description: desc,
		IsPrivate:   isPrivate,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if _, err = sess.Insert(repo); err != nil {
		return nil, err
	} else if _, err = sess.Exec("UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?", u.Id); err != nil {
		return nil, err
	}

	// TODO fix code for mirrors?

	// Give access to all members in owner team.
	if u.IsOrganization() {
		t, err := u.getOwnerTeam(sess)
		if err != nil {
			return nil, fmt.Errorf("getOwnerTeam: %v", err)
		} else if err = t.addRepository(sess, repo); err != nil {
			return nil, fmt.Errorf("addRepository: %v", err)
		}
	} else {
		// Organization called this in addRepository method.
		if err = repo.recalculateAccesses(sess); err != nil {
			return nil, fmt.Errorf("recalculateAccesses: %v", err)
		}
	}

	if err = watchRepo(sess, u.Id, repo.Id, true); err != nil {
		return nil, fmt.Errorf("watchRepo: %v", err)
	} else if err = newRepoAction(sess, u, repo); err != nil {
		return nil, fmt.Errorf("newRepoAction: %v", err)
	}

	// No need for init mirror.
	if !isMirror {
		repoPath := RepoPath(u.Name, repo.Name)
		if err = initRepository(sess, repoPath, u, repo, initReadme, lang, license); err != nil {
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
	}

	return repo, sess.Commit()
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
func TransferOwnership(u *User, newOwnerName string, repo *Repository) error {
	newOwner, err := GetUserByName(newOwnerName)
	if err != nil {
		return fmt.Errorf("get new owner '%s': %v", newOwnerName, err)
	}

	// Check if new owner has repository with same name.
	if IsRepositoryExist(newOwner, repo.Name) {
		return ErrRepoAlreadyExist
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return fmt.Errorf("sess.Begin: %v", err)
	}

	owner := repo.Owner

	// Note: we have to set value here to make sure recalculate accesses is based on
	//	new owner.
	repo.OwnerId = newOwner.Id
	repo.Owner = newOwner

	// Update repository.
	if _, err := sess.Id(repo.Id).Update(repo); err != nil {
		return fmt.Errorf("update owner: %v", err)
	}

	// Remove redundant collaborators.
	collaborators, err := repo.GetCollaborators()
	if err != nil {
		return fmt.Errorf("GetCollaborators: %v", err)
	}

	// Dummy object.
	collaboration := &Collaboration{RepoID: repo.Id}
	for _, c := range collaborators {
		collaboration.UserID = c.Id
		if c.Id == newOwner.Id || newOwner.IsOrgMember(c.Id) {
			if _, err = sess.Delete(collaboration); err != nil {
				return fmt.Errorf("remove collaborator '%d': %v", c.Id, err)
			}
		}
	}

	// Remove old team-repository relations.
	if owner.IsOrganization() {
		if err = owner.getTeams(sess); err != nil {
			return fmt.Errorf("getTeams: %v", err)
		}
		for _, t := range owner.Teams {
			if !t.hasRepository(sess, repo.Id) {
				continue
			}

			t.NumRepos--
			if _, err := sess.Id(t.ID).AllCols().Update(t); err != nil {
				return fmt.Errorf("decrease team repository count '%d': %v", t.ID, err)
			}
		}

		if err = owner.removeOrgRepo(sess, repo.Id); err != nil {
			return fmt.Errorf("removeOrgRepo: %v", err)
		}
	}

	if newOwner.IsOrganization() {
		t, err := newOwner.GetOwnerTeam()
		if err != nil {
			return fmt.Errorf("GetOwnerTeam: %v", err)
		} else if err = t.addRepository(sess, repo); err != nil {
			return fmt.Errorf("add to owner team: %v", err)
		}
	} else {
		// Organization called this in addRepository method.
		if err = repo.recalculateAccesses(sess); err != nil {
			return fmt.Errorf("recalculateAccesses: %v", err)
		}
	}

	// Update repository count.
	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos+1 WHERE id=?", newOwner.Id); err != nil {
		return fmt.Errorf("increase new owner repository count: %v", err)
	} else if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", owner.Id); err != nil {
		return fmt.Errorf("decrease old owner repository count: %v", err)
	}

	if err = watchRepo(sess, newOwner.Id, repo.Id, true); err != nil {
		return fmt.Errorf("watchRepo: %v", err)
	} else if err = transferRepoAction(sess, u, owner, newOwner, repo); err != nil {
		return fmt.Errorf("transferRepoAction: %v", err)
	}

	// Change repository directory name.
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
		return fmt.Errorf("rename directory: %v", err)
	}

	return sess.Commit()
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(userName, oldRepoName, newRepoName string) (err error) {
	userName = strings.ToLower(userName)
	oldRepoName = strings.ToLower(oldRepoName)
	newRepoName = strings.ToLower(newRepoName)
	if !IsLegalName(newRepoName) {
		return ErrRepoNameIllegal
	}

	// Change repository directory name.
	return os.Rename(RepoPath(userName, oldRepoName), RepoPath(userName, newRepoName))
}

func updateRepository(e Engine, repo *Repository) error {
	repo.LowerName = strings.ToLower(repo.Name)

	if len(repo.Description) > 255 {
		repo.Description = repo.Description[:255]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}
	_, err := e.Id(repo.Id).AllCols().Update(repo)
	return err
}

func UpdateRepository(repo *Repository) error {
	return updateRepository(x, repo)
}

// DeleteRepository deletes a repository for a user or organization.
func DeleteRepository(uid, repoID int64, userName string) error {
	repo := &Repository{Id: repoID, OwnerId: uid}
	has, err := x.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist{repoID, uid, ""}
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
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if org.IsOrganization() {
		for _, t := range org.Teams {
			if !t.hasRepository(sess, repoID) {
				continue
			} else if err = t.removeRepository(sess, repo, false); err != nil {
				return err
			}
		}
	}

	if _, err = sess.Delete(&Repository{Id: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Access{RepoID: repo.Id}); err != nil {
		return err
	} else if _, err = sess.Delete(&Action{RepoId: repo.Id}); err != nil {
		return err
	} else if _, err = sess.Delete(&Watch{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Mirror{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&IssueUser{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Milestone{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Release{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Collaboration{RepoID: repoID}); err != nil {
		return err
	}

	// Delete comments.
	issues := make([]*Issue, 0, 25)
	if err = sess.Where("repo_id=?", repoID).Find(&issues); err != nil {
		return err
	}
	for i := range issues {
		if _, err = sess.Delete(&Comment{IssueId: issues[i].Id}); err != nil {
			return err
		}
	}

	if _, err = sess.Delete(&Issue{RepoId: repoID}); err != nil {
		return err
	}

	if repo.IsFork {
		if _, err = sess.Exec("UPDATE `repository` SET num_forks=num_forks-1 WHERE id=?", repo.ForkId); err != nil {
			return err
		}
	}

	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", uid); err != nil {
		return err
	}

	// Remove repository files.
	if err = os.RemoveAll(RepoPath(userName, repo.Name)); err != nil {
		desc := fmt.Sprintf("delete repository files(%s/%s): %v", userName, repo.Name, err)
		log.Warn(desc)
		if err = CreateRepositoryNotice(desc); err != nil {
			log.Error(4, "add notice: %v", err)
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
		return nil, ErrRepoNotExist{0, uid, repoName}
	}
	return repo, err
}

func getRepositoryById(e Engine, id int64) (*Repository, error) {
	repo := new(Repository)
	has, err := e.Id(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist{id, 0, ""}
	}
	return repo, nil
}

// GetRepositoryById returns the repository by given id if exists.
func GetRepositoryById(id int64) (*Repository, error) {
	return getRepositoryById(x, id)
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

type SearchOption struct {
	Keyword string
	Uid     int64
	Limit   int
	Private bool
}

// SearchRepositoryByName returns given number of repositories whose name contains keyword.
func SearchRepositoryByName(opt SearchOption) (repos []*Repository, err error) {
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
	if !opt.Private {
		sess.And("is_private=false")
	}
	sess.And("lower_name like ?", "%"+opt.Keyword+"%").Find(&repos)
	return repos, err
}

// DeleteRepositoryArchives deletes all repositories' archives.
func DeleteRepositoryArchives() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}
			return os.RemoveAll(filepath.Join(RepoPath(repo.Owner.Name, repo.Name), "archives"))
		})
}

// RewriteRepositoryUpdateHook rewrites all repositories' update hook.
func RewriteRepositoryUpdateHook() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}
			return createUpdateHook(RepoPath(repo.Owner.Name, repo.Name))
		})
}

var (
	// Prevent duplicate tasks.
	isMirrorUpdating = false
	isGitFscking     = false
)

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if isMirrorUpdating {
		return
	}
	isMirrorUpdating = true
	defer func() { isMirrorUpdating = false }()

	mirrors := make([]*Mirror, 0, 10)

	if err := x.Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.NextUpdate.After(time.Now()) {
			return nil
		}

		repoPath := filepath.Join(setting.RepoRootPath, m.RepoName+".git")
		if _, stderr, err := process.ExecDir(10*time.Minute,
			repoPath, fmt.Sprintf("MirrorUpdate: %s", repoPath),
			"git", "remote", "update"); err != nil {
			desc := fmt.Sprintf("Fail to update mirror repository(%s): %s", repoPath, stderr)
			log.Error(4, desc)
			if err = CreateRepositoryNotice(desc); err != nil {
				log.Error(4, "Fail to add notice: %v", err)
			}
			return nil
		}

		m.NextUpdate = time.Now().Add(time.Duration(m.Interval) * time.Hour)
		mirrors = append(mirrors, m)
		return nil
	}); err != nil {
		log.Error(4, "MirrorUpdate: %v", err)
	}

	for i := range mirrors {
		if err := UpdateMirror(mirrors[i]); err != nil {
			log.Error(4, "UpdateMirror", fmt.Sprintf("%s: %v", mirrors[i].RepoName, err))
		}
	}
}

// GitFsck calls 'git fsck' to check repository health.
func GitFsck() {
	if isGitFscking {
		return
	}
	isGitFscking = true
	defer func() { isGitFscking = false }()

	args := append([]string{"fsck"}, setting.Git.Fsck.Args...)
	if err := x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}

			repoPath := RepoPath(repo.Owner.Name, repo.Name)
			_, _, err := process.ExecDir(-1, repoPath, "Repository health check", "git", args...)
			if err != nil {
				desc := fmt.Sprintf("Fail to health check repository(%s)", repoPath)
				log.Warn(desc)
				if err = CreateRepositoryNotice(desc); err != nil {
					log.Error(4, "Fail to add notice: %v", err)
				}
			}
			return nil
		}); err != nil {
		log.Error(4, "repo.Fsck: %v", err)
	}
}

func GitGcRepos() error {
	args := append([]string{"gc"}, setting.Git.GcArgs...)
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}
			_, stderr, err := process.ExecDir(-1, RepoPath(repo.Owner.Name, repo.Name), "Repository garbage collection", "git", args...)
			if err != nil {
				return fmt.Errorf("%v: %v", err, stderr)
			}
			return nil
		})
}

// _________        .__  .__        ___.                        __  .__
// \_   ___ \  ____ |  | |  | _____ \_ |__   ________________ _/  |_|__| ____   ____
// /    \  \/ /  _ \|  | |  | \__  \ | __ \ /  _ \_  __ \__  \\   __\  |/  _ \ /    \
// \     \___(  <_> )  |_|  |__/ __ \| \_\ (  <_> )  | \// __ \|  | |  (  <_> )   |  \
//  \______  /\____/|____/____(____  /___  /\____/|__|  (____  /__| |__|\____/|___|  /
//         \/                      \/    \/                  \/                    \/

// A Collaboration is a relation between an individual and a repository
type Collaboration struct {
	ID      int64     `xorm:"pk autoincr"`
	RepoID  int64     `xorm:"UNIQUE(s) INDEX NOT NULL"`
	UserID  int64     `xorm:"UNIQUE(s) INDEX NOT NULL"`
	Created time.Time `xorm:"CREATED"`
}

// Add collaborator and accompanying access
func (repo *Repository) AddCollaborator(u *User) error {
	collaboration := &Collaboration{
		RepoID: repo.Id,
		UserID: u.Id,
	}

	has, err := x.Get(collaboration)
	if err != nil {
		return err
	} else if has {
		return nil
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.InsertOne(collaboration); err != nil {
		return err
	} else if err = repo.recalculateAccesses(sess); err != nil {
		return err
	}

	return sess.Commit()
}

func (repo *Repository) getCollaborators(e Engine) ([]*User, error) {
	collaborations := make([]*Collaboration, 0)
	if err := e.Find(&collaborations, &Collaboration{RepoID: repo.Id}); err != nil {
		return nil, err
	}

	users := make([]*User, len(collaborations))
	for i, c := range collaborations {
		user, err := getUserById(e, c.UserID)
		if err != nil {
			return nil, err
		}
		users[i] = user
	}
	return users, nil
}

// GetCollaborators returns the collaborators for a repository
func (repo *Repository) GetCollaborators() ([]*User, error) {
	return repo.getCollaborators(x)
}

// Delete collaborator and accompanying access
func (repo *Repository) DeleteCollaborator(u *User) (err error) {
	collaboration := &Collaboration{
		RepoID: repo.Id,
		UserID: u.Id,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if has, err := sess.Delete(collaboration); err != nil || has == 0 {
		return err
	} else if err = repo.recalculateAccesses(sess); err != nil {
		return err
	}

	return sess.Commit()
}

//  __      __         __         .__
// /  \    /  \_____ _/  |_  ____ |  |__
// \   \/\/   /\__  \\   __\/ ___\|  |  \
//  \        /  / __ \|  | \  \___|   Y  \
//   \__/\  /  (____  /__|  \___  >___|  /
//        \/        \/          \/     \/

// Watch is connection request for receiving repository notification.
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

func watchRepo(e Engine, uid, repoId int64, watch bool) (err error) {
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
	return watchRepo(x, uid, repoId, watch)
}

func getWatchers(e Engine, rid int64) ([]*Watch, error) {
	watches := make([]*Watch, 0, 10)
	err := e.Find(&watches, &Watch{RepoId: rid})
	return watches, err
}

// GetWatchers returns all watchers of given repository.
func GetWatchers(rid int64) ([]*Watch, error) {
	return getWatchers(x, rid)
}

func notifyWatchers(e Engine, act *Action) error {
	// Add feeds for user self and all watchers.
	watches, err := getWatchers(e, act.RepoId)
	if err != nil {
		return fmt.Errorf("get watchers: %v", err)
	}

	// Add feed for actioner.
	act.UserId = act.ActUserId
	if _, err = e.InsertOne(act); err != nil {
		return fmt.Errorf("insert new actioner: %v", err)
	}

	for i := range watches {
		if act.ActUserId == watches[i].UserId {
			continue
		}

		act.Id = 0
		act.UserId = watches[i].UserId
		if _, err = e.InsertOne(act); err != nil {
			return fmt.Errorf("insert new action: %v", err)
		}
	}
	return nil
}

// NotifyWatchers creates batch of actions for every watcher.
func NotifyWatchers(act *Action) error {
	return notifyWatchers(x, act)
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

func ForkRepository(u *User, oldRepo *Repository, name, desc string) (_ *Repository, err error) {
	if IsRepositoryExist(u, name) {
		return nil, ErrRepoAlreadyExist
	}

	// In case the old repository is a fork.
	if oldRepo.IsFork {
		oldRepo, err = GetRepositoryById(oldRepo.ForkId)
		if err != nil {
			return nil, err
		}
	}

	repo := &Repository{
		OwnerId:     u.Id,
		Owner:       u,
		Name:        name,
		LowerName:   strings.ToLower(name),
		Description: desc,
		IsPrivate:   oldRepo.IsPrivate,
		IsFork:      true,
		ForkId:      oldRepo.Id,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if _, err = sess.Insert(repo); err != nil {
		return nil, err
	}

	if err = repo.recalculateAccesses(sess); err != nil {
		return nil, err
	} else if _, err = sess.Exec("UPDATE `user` SET num_repos = num_repos + 1 WHERE id = ?", u.Id); err != nil {
		return nil, err
	}

	if u.IsOrganization() {
		// Update owner team info and count.
		t, err := u.getOwnerTeam(sess)
		if err != nil {
			return nil, fmt.Errorf("getOwnerTeam: %v", err)
		} else if err = t.addRepository(sess, repo); err != nil {
			return nil, fmt.Errorf("addRepository: %v", err)
		}
	} else {
		if err = watchRepo(sess, u.Id, repo.Id, true); err != nil {
			return nil, fmt.Errorf("watchRepo: %v", err)
		}
	}

	if err = newRepoAction(sess, u, repo); err != nil {
		return nil, fmt.Errorf("newRepoAction: %v", err)
	}

	if _, err = sess.Exec("UPDATE `repository` SET num_forks=num_forks+1 WHERE id=?", oldRepo.Id); err != nil {
		return nil, err
	}

	oldRepoPath, err := oldRepo.RepoPath()
	if err != nil {
		return nil, fmt.Errorf("get old repository path: %v", err)
	}

	repoPath := RepoPath(u.Name, repo.Name)
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("ForkRepository(git clone): %s/%s", u.Name, repo.Name),
		"git", "clone", "--bare", oldRepoPath, repoPath)
	if err != nil {
		return nil, fmt.Errorf("git clone: %v", stderr)
	}

	_, stderr, err = process.ExecDir(-1,
		repoPath, fmt.Sprintf("ForkRepository(git update-server-info): %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, fmt.Errorf("git update-server-info: %v", err)
	}

	if err = createUpdateHook(repoPath); err != nil {
		return nil, fmt.Errorf("createUpdateHook: %v", err)
	}

	return repo, sess.Commit()
}
