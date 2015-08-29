// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
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
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/bindata"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/process"
	"github.com/gogits/gogs/modules/setting"
)

const (
	_TPL_UPDATE_HOOK = "#!/usr/bin/env %s\n%s update $1 $2 $3 --config='%s'\n"
)

var (
	ErrRepoFileNotExist  = errors.New("Repository file does not exist")
	ErrRepoFileNotLoaded = errors.New("Repository file not loaded")
	ErrMirrorNotExist    = errors.New("Mirror does not exist")
	ErrInvalidReference  = errors.New("Invalid reference specified")
	ErrNameEmpty         = errors.New("Name is empty")
)

var (
	Gitignores, Licenses, Readmes []string
)

func LoadRepoConfig() {
	// Load .gitignore and license files and readme templates.
	types := []string{"gitignore", "license", "readme"}
	typeFiles := make([][]string, 3)
	for i, t := range types {
		files, err := bindata.AssetDir("conf/" + t)
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
	Readmes = typeFiles[2]
	sort.Strings(Gitignores)
	sort.Strings(Licenses)
	sort.Strings(Readmes)
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

	// Git requires setting user.name and user.email in order to commit changes.
	for configKey, defaultValue := range map[string]string{"user.name": "Gogs", "user.email": "gogs@fake.local"} {
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
	ID            int64  `xorm:"pk autoincr"`
	OwnerID       int64  `xorm:"UNIQUE(s)"`
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
	ForkID   int64
	BaseRepo *Repository `xorm:"-"`

	Created time.Time `xorm:"CREATED"`
	Updated time.Time `xorm:"UPDATED"`
}

func (repo *Repository) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "updated":
		repo.Updated = regulateTimeZone(repo.Updated)
	}
}

func (repo *Repository) getOwner(e Engine) (err error) {
	if repo.Owner == nil {
		repo.Owner, err = getUserByID(e, repo.OwnerID)
	}
	return err
}

func (repo *Repository) GetOwner() error {
	return repo.getOwner(x)
}

// GetAssignees returns all users that have write access of repository.
func (repo *Repository) GetAssignees() (_ []*User, err error) {
	if err = repo.GetOwner(); err != nil {
		return nil, err
	}

	accesses := make([]*Access, 0, 10)
	if err = x.Where("repo_id=? AND mode>=?", repo.ID, ACCESS_MODE_WRITE).Find(&accesses); err != nil {
		return nil, err
	}

	users := make([]*User, 0, len(accesses)+1) // Just waste 1 unit does not matter.
	if !repo.Owner.IsOrganization() {
		users = append(users, repo.Owner)
	}

	var u *User
	for i := range accesses {
		u, err = GetUserByID(accesses[i].UserID)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// GetAssigneeByID returns the user that has write access of repository by given ID.
func (repo *Repository) GetAssigneeByID(userID int64) (*User, error) {
	return GetAssigneeByID(repo, userID)
}

// GetMilestoneByID returns the milestone belongs to repository by given ID.
func (repo *Repository) GetMilestoneByID(milestoneID int64) (*Milestone, error) {
	return GetRepoMilestoneByID(repo.ID, milestoneID)
}

// IssueStats returns number of open and closed repository issues by given filter mode.
func (repo *Repository) IssueStats(uid int64, filterMode int) (int64, int64) {
	return GetRepoIssueStats(repo.ID, uid, filterMode)
}

func (repo *Repository) GetMirror() (err error) {
	repo.Mirror, err = GetMirror(repo.ID)
	return err
}

func (repo *Repository) GetBaseRepo() (err error) {
	if !repo.IsFork {
		return nil
	}

	repo.BaseRepo, err = GetRepositoryByID(repo.ForkID)
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

func (repo *Repository) IsOwnedBy(userID int64) bool {
	return repo.OwnerID == userID
}

var (
	DescPattern = regexp.MustCompile(`https?://\S+`)
)

// DescriptionHtml does special handles to description and return HTML string.
func (repo *Repository) DescriptionHtml() template.HTML {
	sanitize := func(s string) string {
		return fmt.Sprintf(`<a href="%[1]s" target="_blank">%[1]s</a>`, s)
	}
	return template.HTML(DescPattern.ReplaceAllStringFunc(base.Sanitizer.Sanitize(repo.Description), sanitize))
}

func isRepositoryExist(e Engine, u *User, repoName string) (bool, error) {
	has, err := e.Get(&Repository{
		OwnerID:   u.Id,
		LowerName: strings.ToLower(repoName),
	})
	return has && com.IsDir(RepoPath(u.Name, repoName)), err
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(u *User, repoName string) (bool, error) {
	return isRepositoryExist(x, u, repoName)
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
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", setting.RunUser, setting.SSHDomain, setting.SSHPort, repo.Owner.LowerName, repo.LowerName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", setting.RunUser, setting.SSHDomain, repo.Owner.LowerName, repo.LowerName)
	}
	cl.HTTPS = fmt.Sprintf("%s%s/%s.git", setting.AppUrl, repo.Owner.LowerName, repo.LowerName)
	return cl, nil
}

var (
	reservedNames    = []string{"debug", "raw", "install", "api", "avatar", "user", "org", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin", "new"}
	reservedPatterns = []string{"*.git", "*.keys"}
)

// IsUsableName checks if name is reserved or pattern of name is not allowed.
func IsUsableName(name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if utf8.RuneCountInString(name) == 0 {
		return ErrNameEmpty
	}

	for i := range reservedNames {
		if name == reservedNames[i] {
			return ErrNameReserved{name}
		}
	}

	for _, pat := range reservedPatterns {
		if pat[0] == '*' && strings.HasSuffix(name, pat[1:]) ||
			(pat[len(pat)-1] == '*' && strings.HasPrefix(name, pat[:len(pat)-1])) {
			return ErrNamePatternNotAllowed{pat}
		}
	}

	return nil
}

// Mirror represents a mirror information of repository.
type Mirror struct {
	ID         int64 `xorm:"pk autoincr"`
	RepoID     int64
	Repo       *Repository `xorm:"-"`
	Interval   int         // Hour.
	Updated    time.Time   `xorm:"UPDATED"`
	NextUpdate time.Time
}

func (m *Mirror) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "repo_id":
		m.Repo, err = GetRepositoryByID(m.RepoID)
		if err != nil {
			log.Error(3, "GetRepositoryByID[%d]: %v", m.ID, err)
		}
	}
}

func getMirror(e Engine, repoId int64) (*Mirror, error) {
	m := &Mirror{RepoID: repoId}
	has, err := e.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMirrorNotExist
	}
	return m, nil
}

// GetMirror returns mirror object by given repository ID.
func GetMirror(repoId int64) (*Mirror, error) {
	return getMirror(x, repoId)
}

func updateMirror(e Engine, m *Mirror) error {
	_, err := e.Id(m.ID).Update(m)
	return err
}

func UpdateMirror(m *Mirror) error {
	return updateMirror(x, m)
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
		RepoID:     repoId,
		Interval:   24,
		NextUpdate: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return err
	}
	return nil
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(u *User, name, desc string, private, mirror bool, url string) (*Repository, error) {
	repo, err := CreateRepository(u, CreateRepoOptions{
		Name:        name,
		Description: desc,
		IsPrivate:   private,
		IsMirror:    mirror,
	})
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
		if err = MirrorRepository(repo.ID, u.Name, repo.Name, repoPath, url); err != nil {
			return repo, err
		}
		repo.IsMirror = true
		return repo, UpdateRepository(repo, false)
	} else {
		os.RemoveAll(repoPath)
	}

	// FIXME: this command could for both migrate and mirror
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("MigrateRepository: %s", repoPath),
		"git", "clone", "--mirror", "--bare", "--quiet", url, repoPath)
	if err != nil {
		return repo, fmt.Errorf("git clone --mirror --bare --quiet: %v", stderr)
	} else if err = createUpdateHook(repoPath); err != nil {
		return repo, fmt.Errorf("create update hook: %v", err)
	}

	// Check if repository has master branch, if so set it to default branch.
	gitRepo, err := git.OpenRepository(repoPath)
	if err != nil {
		return repo, fmt.Errorf("open git repository: %v", err)
	}
	if gitRepo.IsBranchExist("master") {
		repo.DefaultBranch = "master"
	}

	return repo, UpdateRepository(repo, false)
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
		"-m", "initial commit"); err != nil {
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
	hookPath := path.Join(repoPath, "hooks/update")
	os.MkdirAll(path.Dir(hookPath), os.ModePerm)
	return ioutil.WriteFile(hookPath,
		[]byte(fmt.Sprintf(_TPL_UPDATE_HOOK, setting.ScriptType, "\""+appPath+"\"", setting.CustomConf)), 0777)
}

type CreateRepoOptions struct {
	Name        string
	Description string
	Gitignores  string
	License     string
	Readme      string
	IsPrivate   bool
	IsMirror    bool
	AutoInit    bool
}

func getRepoInitFile(tp, name string) ([]byte, error) {
	relPath := path.Join("conf", tp, name)

	// Use custom file when available.
	customPath := path.Join(setting.CustomPath, relPath)
	if com.IsFile(customPath) {
		return ioutil.ReadFile(customPath)
	}
	return bindata.Asset(relPath)
}

func prepareRepoCommit(repo *Repository, tmpDir, repoPath string, opts CreateRepoOptions) error {
	// Clone to temprory path and do the init commit.
	_, stderr, err := process.Exec(
		fmt.Sprintf("initRepository(git clone): %s", repoPath), "git", "clone", repoPath, tmpDir)
	if err != nil {
		return fmt.Errorf("git clone: %v - %s", err, stderr)
	}

	// README
	data, err := getRepoInitFile("readme", opts.Readme)
	if err != nil {
		return fmt.Errorf("getRepoInitFile[%s]: %v", opts.Readme, err)
	}

	cloneLink, err := repo.CloneLink()
	if err != nil {
		return fmt.Errorf("CloneLink: %v", err)
	}
	match := map[string]string{
		"Name":           repo.Name,
		"Description":    repo.Description,
		"CloneURL.SSH":   cloneLink.SSH,
		"CloneURL.HTTPS": cloneLink.HTTPS,
	}
	if err = ioutil.WriteFile(filepath.Join(tmpDir, "README.md"),
		[]byte(com.Expand(string(data), match)), 0644); err != nil {
		return fmt.Errorf("write README.md: %v", err)
	}

	// .gitignore
	if len(opts.Gitignores) > 0 {
		var buf bytes.Buffer
		names := strings.Split(opts.Gitignores, ",")
		for _, name := range names {
			data, err = getRepoInitFile("gitignore", name)
			if err != nil {
				return fmt.Errorf("getRepoInitFile[%s]: %v", name, err)
			}
			buf.WriteString("# ---> " + name + "\n")
			buf.Write(data)
			buf.WriteString("\n")
		}

		if buf.Len() > 0 {
			if err = ioutil.WriteFile(filepath.Join(tmpDir, ".gitignore"), buf.Bytes(), 0644); err != nil {
				return fmt.Errorf("write .gitignore: %v", err)
			}
		}
	}

	// LICENSE
	if len(opts.License) > 0 {
		data, err = getRepoInitFile("license", opts.License)
		if err != nil {
			return fmt.Errorf("getRepoInitFile[%s]: %v", opts.License, err)
		}

		if err = ioutil.WriteFile(filepath.Join(tmpDir, "LICENSE"), data, 0644); err != nil {
			return fmt.Errorf("write LICENSE: %v", err)
		}
	}

	return nil
}

// InitRepository initializes README and .gitignore if needed.
func initRepository(e Engine, repoPath string, u *User, repo *Repository, opts CreateRepoOptions) error {
	// Somehow the directory could exist.
	if com.IsExist(repoPath) {
		return fmt.Errorf("initRepository: path already exists: %s", repoPath)
	}

	// Init bare new repository.
	os.MkdirAll(repoPath, os.ModePerm)
	_, stderr, err := process.ExecDir(-1, repoPath,
		fmt.Sprintf("initRepository(git init --bare): %s", repoPath), "git", "init", "--bare")
	if err != nil {
		return fmt.Errorf("git init --bare: %v - %s", err, stderr)
	}

	if err := createUpdateHook(repoPath); err != nil {
		return err
	}

	tmpDir := filepath.Join(os.TempDir(), "gogs", repo.Name, com.ToStr(time.Now().Nanosecond()))

	// Initialize repository according to user's choice.
	if opts.AutoInit {
		os.MkdirAll(tmpDir, os.ModePerm)
		defer os.RemoveAll(tmpDir)

		if err = prepareRepoCommit(repo, tmpDir, repoPath, opts); err != nil {
			return fmt.Errorf("prepareRepoCommit: %v", err)
		}

		// Apply changes and commit.
		if err = initRepoCommit(tmpDir, u.NewGitSig()); err != nil {
			return fmt.Errorf("initRepoCommit: %v", err)
		}
	}

	// Re-fetch the repository from database before updating it (else it would
	// override changes that were done earlier with sql)
	if repo, err = getRepositoryByID(e, repo.ID); err != nil {
		return fmt.Errorf("getRepositoryByID: %v", err)
	}

	if !opts.AutoInit {
		repo.IsBare = true
	}

	repo.DefaultBranch = "master"
	if err = updateRepository(e, repo, false); err != nil {
		return fmt.Errorf("updateRepository: %v", err)
	}

	return nil
}

func createRepository(e *xorm.Session, u *User, repo *Repository) (err error) {
	if err = IsUsableName(repo.Name); err != nil {
		return err
	}

	has, err := isRepositoryExist(e, u, repo.Name)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{u.Name, repo.Name}
	}

	if _, err = e.Insert(repo); err != nil {
		return err
	} else if _, err = e.Exec("UPDATE `user` SET num_repos=num_repos+1 WHERE id=?", u.Id); err != nil {
		return err
	}

	// Give access to all members in owner team.
	if u.IsOrganization() {
		t, err := u.getOwnerTeam(e)
		if err != nil {
			return fmt.Errorf("getOwnerTeam: %v", err)
		} else if err = t.addRepository(e, repo); err != nil {
			return fmt.Errorf("addRepository: %v", err)
		}
	} else {
		// Organization automatically called this in addRepository method.
		if err = repo.recalculateAccesses(e); err != nil {
			return fmt.Errorf("recalculateAccesses: %v", err)
		}
	}

	if err = watchRepo(e, u.Id, repo.ID, true); err != nil {
		return fmt.Errorf("watchRepo: %v", err)
	} else if err = newRepoAction(e, u, repo); err != nil {
		return fmt.Errorf("newRepoAction: %v", err)
	}

	return nil
}

// CreateRepository creates a repository for given user or organization.
func CreateRepository(u *User, opts CreateRepoOptions) (_ *Repository, err error) {
	repo := &Repository{
		OwnerID:     u.Id,
		Owner:       u,
		Name:        opts.Name,
		LowerName:   strings.ToLower(opts.Name),
		Description: opts.Description,
		IsPrivate:   opts.IsPrivate,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if err = createRepository(sess, u, repo); err != nil {
		return nil, err
	}

	// No need for init mirror.
	if !opts.IsMirror {
		repoPath := RepoPath(u.Name, repo.Name)
		if err = initRepository(sess, repoPath, u, repo, opts); err != nil {
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
		repo.Owner = &User{Id: repo.OwnerID}
		has, err := x.Get(repo.Owner)
		if err != nil {
			return nil, err
		} else if !has {
			return nil, ErrUserNotExist{repo.OwnerID, ""}
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
	has, err := IsRepositoryExist(newOwner, repo.Name)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{newOwnerName, repo.Name}
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return fmt.Errorf("sess.Begin: %v", err)
	}

	owner := repo.Owner

	// Note: we have to set value here to make sure recalculate accesses is based on
	//	new owner.
	repo.OwnerID = newOwner.Id
	repo.Owner = newOwner

	// Update repository.
	if _, err := sess.Id(repo.ID).Update(repo); err != nil {
		return fmt.Errorf("update owner: %v", err)
	}

	// Remove redundant collaborators.
	collaborators, err := repo.GetCollaborators()
	if err != nil {
		return fmt.Errorf("GetCollaborators: %v", err)
	}

	// Dummy object.
	collaboration := &Collaboration{RepoID: repo.ID}
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
			if !t.hasRepository(sess, repo.ID) {
				continue
			}

			t.NumRepos--
			if _, err := sess.Id(t.ID).AllCols().Update(t); err != nil {
				return fmt.Errorf("decrease team repository count '%d': %v", t.ID, err)
			}
		}

		if err = owner.removeOrgRepo(sess, repo.ID); err != nil {
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

	if err = watchRepo(sess, newOwner.Id, repo.ID, true); err != nil {
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
func ChangeRepositoryName(u *User, oldRepoName, newRepoName string) (err error) {
	oldRepoName = strings.ToLower(oldRepoName)
	newRepoName = strings.ToLower(newRepoName)
	if err = IsUsableName(newRepoName); err != nil {
		return err
	}

	has, err := IsRepositoryExist(u, newRepoName)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{u.Name, newRepoName}
	}

	// Change repository directory name.
	return os.Rename(RepoPath(u.LowerName, oldRepoName), RepoPath(u.LowerName, newRepoName))
}

func updateRepository(e Engine, repo *Repository, visibilityChanged bool) (err error) {
	repo.LowerName = strings.ToLower(repo.Name)

	if len(repo.Description) > 255 {
		repo.Description = repo.Description[:255]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	if _, err = e.Id(repo.ID).AllCols().Update(repo); err != nil {
		return fmt.Errorf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(e); err != nil {
			return fmt.Errorf("getOwner: %v", err)
		}
		if !repo.Owner.IsOrganization() {
			return nil
		}

		// Organization repository need to recalculate access table when visivility is changed.
		if err = repo.recalculateTeamAccesses(e, 0); err != nil {
			return fmt.Errorf("recalculateTeamAccesses: %v", err)
		}
	}

	return nil
}

func UpdateRepository(repo *Repository, visibilityChanged bool) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = updateRepository(x, repo, visibilityChanged); err != nil {
		return fmt.Errorf("updateRepository: %v", err)
	}

	return sess.Commit()
}

// DeleteRepository deletes a repository for a user or organization.
func DeleteRepository(uid, repoID int64, userName string) error {
	repo := &Repository{ID: repoID, OwnerID: uid}
	has, err := x.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist{repoID, uid, ""}
	}

	// In case is a organization.
	org, err := GetUserByID(uid)
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

	if _, err = sess.Delete(&Repository{ID: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Access{RepoID: repo.ID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Action{RepoID: repo.ID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Watch{RepoID: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Mirror{RepoID: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&IssueUser{RepoID: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Milestone{RepoID: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Release{RepoId: repoID}); err != nil {
		return err
	} else if _, err = sess.Delete(&Collaboration{RepoID: repoID}); err != nil {
		return err
	}

	// Delete comments and attachments.
	issues := make([]*Issue, 0, 25)
	attachmentPaths := make([]string, 0, len(issues))
	if err = sess.Where("repo_id=?", repoID).Find(&issues); err != nil {
		return err
	}
	for i := range issues {
		if _, err = sess.Delete(&Comment{IssueID: issues[i].ID}); err != nil {
			return err
		}

		attachments := make([]*Attachment, 0, 5)
		if err = sess.Where("issue_id=?", issues[i].ID).Find(&attachments); err != nil {
			return err
		}
		for j := range attachments {
			attachmentPaths = append(attachmentPaths, attachments[j].LocalPath())
		}

		if _, err = sess.Delete(&Attachment{IssueID: issues[i].ID}); err != nil {
			return err
		}
	}

	if _, err = sess.Delete(&Issue{RepoID: repoID}); err != nil {
		return err
	}

	if repo.IsFork {
		if _, err = sess.Exec("UPDATE `repository` SET num_forks=num_forks-1 WHERE id=?", repo.ForkID); err != nil {
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

	// Remove attachment files.
	for i := range attachmentPaths {
		if err = os.Remove(attachmentPaths[i]); err != nil {
			log.Warn("delete attachment: %v", err)
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
		OwnerID:   uid,
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

func getRepositoryByID(e Engine, id int64) (*Repository, error) {
	repo := new(Repository)
	has, err := e.Id(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist{id, 0, ""}
	}
	return repo, nil
}

// GetRepositoryByID returns the repository by given id if exists.
func GetRepositoryByID(id int64) (*Repository, error) {
	return getRepositoryByID(x, id)
}

// GetRepositories returns a list of repositories of given user.
func GetRepositories(uid int64, private bool) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	sess := x.Desc("updated")
	if !private {
		sess.Where("is_private=?", false)
	}

	return repos, sess.Find(&repos, &Repository{OwnerID: uid})
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories(num int) (repos []*Repository, err error) {
	err = x.Where("is_private=?", false).Limit(num).Desc("updated").Find(&repos)
	return repos, err
}

// GetRepositoryCount returns the total number of repositories of user.
func GetRepositoryCount(u *User) (int64, error) {
	return x.Count(&Repository{OwnerID: u.Id})
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
	// Prevent duplicate running tasks.
	isMirrorUpdating = false
	isGitFscking     = false
	isCheckingRepos  = false
)

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if isMirrorUpdating {
		return
	}
	isMirrorUpdating = true
	defer func() { isMirrorUpdating = false }()

	log.Trace("Doing: MirrorUpdate")

	mirrors := make([]*Mirror, 0, 10)
	if err := x.Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.NextUpdate.After(time.Now()) {
			return nil
		}

		if m.Repo == nil {
			log.Error(4, "Disconnected mirror repository found: %d", m.ID)
			return nil
		}

		repoPath, err := m.Repo.RepoPath()
		if err != nil {
			return fmt.Errorf("Repo.RepoPath: %v", err)
		}

		if _, stderr, err := process.ExecDir(10*time.Minute,
			repoPath, fmt.Sprintf("MirrorUpdate: %s", repoPath),
			"git", "remote", "update", "--prune"); err != nil {
			desc := fmt.Sprintf("Fail to update mirror repository(%s): %s", repoPath, stderr)
			log.Error(4, desc)
			if err = CreateRepositoryNotice(desc); err != nil {
				log.Error(4, "CreateRepositoryNotice: %v", err)
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
			log.Error(4, "UpdateMirror[%d]: %v", mirrors[i].ID, err)
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

	log.Trace("Doing: GitFsck")

	args := append([]string{"fsck"}, setting.Cron.RepoHealthCheck.Args...)
	if err := x.Where("id>0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			repoPath, err := repo.RepoPath()
			if err != nil {
				return fmt.Errorf("RepoPath: %v", err)
			}

			_, _, err = process.ExecDir(-1, repoPath, "Repository health check", "git", args...)
			if err != nil {
				desc := fmt.Sprintf("Fail to health check repository(%s)", repoPath)
				log.Warn(desc)
				if err = CreateRepositoryNotice(desc); err != nil {
					log.Error(4, "CreateRepositoryNotice: %v", err)
				}
			}
			return nil
		}); err != nil {
		log.Error(4, "GitFsck: %v", err)
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

func CheckRepoStats() {
	if isCheckingRepos {
		return
	}
	isCheckingRepos = true
	defer func() { isCheckingRepos = false }()

	log.Trace("Doing: CheckRepoStats")

	// ***** START: Watch *****
	results, err := x.Query("SELECT repo.id FROM `repository` repo WHERE repo.num_watches!=(SELECT COUNT(*) FROM `watch` WHERE repo_id=repo.id)")
	if err != nil {
		log.Error(4, "Select repository check 'watch': %v", err)
		return
	}
	for _, watch := range results {
		repoID := com.StrTo(watch["id"]).MustInt64()
		log.Trace("Updating repository count 'watch': %d", repoID)
		_, err = x.Exec("UPDATE `repository` SET num_watches=(SELECT COUNT(*) FROM `watch` WHERE repo_id=?) WHERE id=?", repoID, repoID)
		if err != nil {
			log.Error(4, "Update repository check 'watch'[%d]: %v", repoID, err)
		}
	}
	// ***** END: Watch *****

	// ***** START: Star *****
	results, err = x.Query("SELECT repo.id FROM `repository` repo WHERE repo.num_stars!=(SELECT COUNT(*) FROM `star` WHERE repo_id=repo.id)")
	if err != nil {
		log.Error(4, "Select repository check 'star': %v", err)
		return
	}
	for _, star := range results {
		repoID := com.StrTo(star["id"]).MustInt64()
		log.Trace("Updating repository count 'star': %d", repoID)
		_, err = x.Exec("UPDATE `repository` SET num_stars=(SELECT COUNT(*) FROM `star` WHERE repo_id=?) WHERE id=?", repoID, repoID)
		if err != nil {
			log.Error(4, "Update repository check 'star'[%d]: %v", repoID, err)
		}
	}
	// ***** END: Star *****

	// ***** START: Label *****
	results, err = x.Query("SELECT label.id FROM `label` WHERE label.num_issues!=(SELECT COUNT(*) FROM `issue_label` WHERE label_id=label.id)")
	if err != nil {
		log.Error(4, "Select label check 'num_issues': %v", err)
		return
	}
	for _, label := range results {
		labelID := com.StrTo(label["id"]).MustInt64()
		log.Trace("Updating label count 'num_issues': %d", labelID)
		_, err = x.Exec("UPDATE `label` SET num_issues=(SELECT COUNT(*) FROM `issue_label` WHERE label_id=?) WHERE id=?", labelID, labelID)
		if err != nil {
			log.Error(4, "Update label check 'num_issues'[%d]: %v", labelID, err)
		}
	}
	// ***** END: Label *****
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
		RepoID: repo.ID,
		UserID: u.Id,
	}

	has, err := x.Get(collaboration)
	if err != nil {
		return err
	} else if has {
		return nil
	}

	if err = repo.GetOwner(); err != nil {
		return fmt.Errorf("GetOwner: %v", err)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.InsertOne(collaboration); err != nil {
		return err
	}

	if repo.Owner.IsOrganization() {
		err = repo.recalculateTeamAccesses(sess, 0)
	} else {
		err = repo.recalculateAccesses(sess)
	}
	if err != nil {
		return fmt.Errorf("recalculateAccesses 'team=%v': %v", repo.Owner.IsOrganization(), err)
	}

	return sess.Commit()
}

func (repo *Repository) getCollaborators(e Engine) ([]*User, error) {
	collaborations := make([]*Collaboration, 0)
	if err := e.Find(&collaborations, &Collaboration{RepoID: repo.ID}); err != nil {
		return nil, err
	}

	users := make([]*User, len(collaborations))
	for i, c := range collaborations {
		user, err := getUserByID(e, c.UserID)
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
		RepoID: repo.ID,
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
	ID     int64 `xorm:"pk autoincr"`
	UserID int64 `xorm:"UNIQUE(watch)"`
	RepoID int64 `xorm:"UNIQUE(watch)"`
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
		if _, err = e.Insert(&Watch{RepoID: repoId, UserID: uid}); err != nil {
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
		_, err = e.Exec("UPDATE `repository` SET num_watches=num_watches-1 WHERE id=?", repoId)
	}
	return err
}

// Watch or unwatch repository.
func WatchRepo(uid, repoId int64, watch bool) (err error) {
	return watchRepo(x, uid, repoId, watch)
}

func getWatchers(e Engine, rid int64) ([]*Watch, error) {
	watches := make([]*Watch, 0, 10)
	err := e.Find(&watches, &Watch{RepoID: rid})
	return watches, err
}

// GetWatchers returns all watchers of given repository.
func GetWatchers(rid int64) ([]*Watch, error) {
	return getWatchers(x, rid)
}

func notifyWatchers(e Engine, act *Action) error {
	// Add feeds for user self and all watchers.
	watches, err := getWatchers(e, act.RepoID)
	if err != nil {
		return fmt.Errorf("get watchers: %v", err)
	}

	// Add feed for actioner.
	act.UserID = act.ActUserID
	if _, err = e.InsertOne(act); err != nil {
		return fmt.Errorf("insert new actioner: %v", err)
	}

	for i := range watches {
		if act.ActUserID == watches[i].UserID {
			continue
		}

		act.ID = 0
		act.UserID = watches[i].UserID
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
	ID     int64 `xorm:"pk autoincr"`
	UID    int64 `xorm:"uid UNIQUE(s)"`
	RepoID int64 `xorm:"UNIQUE(s)"`
}

// Star or unstar repository.
func StarRepo(uid, repoId int64, star bool) (err error) {
	if star {
		if IsStaring(uid, repoId) {
			return nil
		}
		if _, err = x.Insert(&Star{UID: uid, RepoID: repoId}); err != nil {
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

// HasForkedRepo checks if given user has already forked a repository with given ID.
func HasForkedRepo(ownerID, repoID int64) (*Repository, bool) {
	repo := new(Repository)
	has, _ := x.Where("owner_id=? AND fork_id=?", ownerID, repoID).Get(repo)
	return repo, has
}

func ForkRepository(u *User, oldRepo *Repository, name, desc string) (_ *Repository, err error) {
	repo := &Repository{
		OwnerID:     u.Id,
		Owner:       u,
		Name:        name,
		LowerName:   strings.ToLower(name),
		Description: desc,
		IsPrivate:   oldRepo.IsPrivate,
		IsFork:      true,
		ForkID:      oldRepo.ID,
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if err = createRepository(sess, u, repo); err != nil {
		return nil, err
	}

	if _, err = sess.Exec("UPDATE `repository` SET num_forks=num_forks+1 WHERE id=?", oldRepo.ID); err != nil {
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
