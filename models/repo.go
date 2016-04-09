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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Unknwon/cae/zip"
	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	"github.com/mcuadros/go-version"
	"gopkg.in/ini.v1"

	git "github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/bindata"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/markdown"
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

	// Maximum items per page in forks, watchers and stars of a repo
	ItemsPerPage = 40
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
	gitVer, err := git.BinVersion()
	if err != nil {
		log.Fatal(4, "Fail to get Git version: %v", err)
	}

	log.Info("Git Version: %s", gitVer)
	if version.Compare("1.7.1", gitVer, ">") {
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

	RemoveAllWithNotice("Clean up repository temporary data", filepath.Join(setting.AppDataPath, "tmp"))
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

	// Advanced settings
	EnableWiki            bool `xorm:"NOT NULL DEFAULT true"`
	EnableExternalWiki    bool
	ExternalWikiURL       string
	EnableIssues          bool `xorm:"NOT NULL DEFAULT true"`
	EnableExternalTracker bool
	ExternalTrackerFormat string
	ExternalMetas         map[string]string `xorm:"-"`
	EnablePulls           bool              `xorm:"NOT NULL DEFAULT true"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false"`
	ForkID   int64
	BaseRepo *Repository `xorm:"-"`

	Created     time.Time `xorm:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-"`
	UpdatedUnix int64
}

func (repo *Repository) BeforeInsert() {
	repo.CreatedUnix = time.Now().UTC().Unix()
	repo.UpdatedUnix = repo.CreatedUnix
}

func (repo *Repository) BeforeUpdate() {
	repo.UpdatedUnix = time.Now().UTC().Unix()
}

func (repo *Repository) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "default_branch":
		// FIXME: use models migration to solve all at once.
		if len(repo.DefaultBranch) == 0 {
			repo.DefaultBranch = "master"
		}
	case "num_closed_issues":
		repo.NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
	case "num_closed_pulls":
		repo.NumOpenPulls = repo.NumPulls - repo.NumClosedPulls
	case "num_closed_milestones":
		repo.NumOpenMilestones = repo.NumMilestones - repo.NumClosedMilestones
	case "created_unix":
		repo.Created = time.Unix(repo.CreatedUnix, 0).Local()
	case "updated_unix":
		repo.Updated = time.Unix(repo.UpdatedUnix, 0)
	}
}

func (repo *Repository) getOwner(e Engine) (err error) {
	if repo.Owner != nil {
		return nil
	}

	repo.Owner, err = getUserByID(e, repo.OwnerID)
	return err
}

func (repo *Repository) GetOwner() error {
	return repo.getOwner(x)
}

func (repo *Repository) mustOwner(e Engine) *User {
	if err := repo.getOwner(e); err != nil {
		return &User{
			Name:     "error",
			FullName: err.Error(),
		}
	}

	return repo.Owner
}

// MustOwner always returns a valid *User object to avoid
// conceptually impossible error handling.
// It creates a fake object that contains error deftail
// when error occurs.
func (repo *Repository) MustOwner() *User {
	return repo.mustOwner(x)
}

// ComposeMetas composes a map of metas for rendering external issue tracker URL.
func (repo *Repository) ComposeMetas() map[string]string {
	if !repo.EnableExternalTracker {
		return nil
	} else if repo.ExternalMetas == nil {
		repo.ExternalMetas = map[string]string{
			"format": repo.ExternalTrackerFormat,
			"user":   repo.MustOwner().Name,
			"repo":   repo.Name,
		}
	}
	return repo.ExternalMetas
}

// DeleteWiki removes the actual and local copy of repository wiki.
func (repo *Repository) DeleteWiki() {
	wikiPaths := []string{repo.WikiPath(), repo.LocalWikiPath()}
	for _, wikiPath := range wikiPaths {
		RemoveAllWithNotice("Delete repository wiki", wikiPath)
	}
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
func (repo *Repository) IssueStats(uid int64, filterMode int, isPull bool) (int64, int64) {
	return GetRepoIssueStats(repo.ID, uid, filterMode, isPull)
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

func (repo *Repository) repoPath(e Engine) string {
	return RepoPath(repo.mustOwner(e).Name, repo.Name)
}

func (repo *Repository) RepoPath() string {
	return repo.repoPath(x)
}

func (repo *Repository) GitConfigPath() string {
	return filepath.Join(repo.RepoPath(), "config")
}

func (repo *Repository) RepoLink() string {
	return setting.AppSubUrl + "/" + repo.MustOwner().Name + "/" + repo.Name
}

func (repo *Repository) RepoRelLink() string {
	return "/" + repo.MustOwner().Name + "/" + repo.Name
}

func (repo *Repository) ComposeCompareURL(oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", repo.MustOwner().Name, repo.Name, oldCommitID, newCommitID)
}

func (repo *Repository) FullRepoLink() string {
	return setting.AppUrl + repo.MustOwner().Name + "/" + repo.Name
}

func (repo *Repository) HasAccess(u *User) bool {
	has, _ := HasAccess(u, repo, ACCESS_MODE_READ)
	return has
}

func (repo *Repository) IsOwnedBy(userID int64) bool {
	return repo.OwnerID == userID
}

// CanBeForked returns true if repository meets the requirements of being forked.
func (repo *Repository) CanBeForked() bool {
	return !repo.IsBare
}

// CanEnablePulls returns true if repository meets the requirements of accepting pulls.
func (repo *Repository) CanEnablePulls() bool {
	return !repo.IsMirror
}

// AllowPulls returns true if repository meets the requirements of accepting pulls and has them enabled.
func (repo *Repository) AllowsPulls() bool {
	return repo.CanEnablePulls() && repo.EnablePulls
}

func (repo *Repository) NextIssueIndex() int64 {
	return int64(repo.NumIssues+repo.NumPulls) + 1
}

var (
	DescPattern = regexp.MustCompile(`https?://\S+`)
)

// DescriptionHtml does special handles to description and return HTML string.
func (repo *Repository) DescriptionHtml() template.HTML {
	sanitize := func(s string) string {
		return fmt.Sprintf(`<a href="%[1]s" target="_blank">%[1]s</a>`, s)
	}
	return template.HTML(DescPattern.ReplaceAllStringFunc(markdown.Sanitizer.Sanitize(repo.Description), sanitize))
}

func (repo *Repository) LocalCopyPath() string {
	return path.Join(setting.AppDataPath, "tmp/local", com.ToStr(repo.ID))
}

func updateLocalCopy(repoPath, localPath string) error {
	if !com.IsExist(localPath) {
		if err := git.Clone(repoPath, localPath, git.CloneRepoOptions{
			Timeout: time.Duration(setting.Git.Timeout.Clone) * time.Second,
		}); err != nil {
			return fmt.Errorf("Clone: %v", err)
		}
	} else {
		if err := git.Pull(localPath, git.PullRemoteOptions{
			All:     true,
			Timeout: time.Duration(setting.Git.Timeout.Pull) * time.Second,
		}); err != nil {
			return fmt.Errorf("Pull: %v", err)
		}
	}
	return nil
}

// UpdateLocalCopy makes sure the local copy of repository is up-to-date.
func (repo *Repository) UpdateLocalCopy() error {
	return updateLocalCopy(repo.RepoPath(), repo.LocalCopyPath())
}

// PatchPath returns corresponding patch file path of repository by given issue ID.
func (repo *Repository) PatchPath(index int64) (string, error) {
	if err := repo.GetOwner(); err != nil {
		return "", err
	}

	return filepath.Join(RepoPath(repo.Owner.Name, repo.Name), "pulls", com.ToStr(index)+".patch"), nil
}

// SavePatch saves patch data to corresponding location by given issue ID.
func (repo *Repository) SavePatch(index int64, patch []byte) error {
	patchPath, err := repo.PatchPath(index)
	if err != nil {
		return fmt.Errorf("PatchPath: %v", err)
	}

	os.MkdirAll(filepath.Dir(patchPath), os.ModePerm)
	if err = ioutil.WriteFile(patchPath, patch, 0644); err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	return nil
}

// ComposePayload composes and returns *api.PayloadRepo corresponding to the repository.
func (repo *Repository) ComposePayload() *api.PayloadRepo {
	cl := repo.CloneLink()
	return &api.PayloadRepo{
		ID:          repo.ID,
		Name:        repo.Name,
		URL:         repo.FullRepoLink(),
		SSHURL:      cl.SSH,
		CloneURL:    cl.HTTPS,
		Description: repo.Description,
		Website:     repo.Website,
		Watchers:    repo.NumWatches,
		Owner: &api.PayloadAuthor{
			Name:     repo.MustOwner().DisplayName(),
			Email:    repo.MustOwner().Email,
			UserName: repo.MustOwner().Name,
		},
		Private:       repo.IsPrivate,
		DefaultBranch: repo.DefaultBranch,
	}
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

func (repo *Repository) cloneLink(isWiki bool) *CloneLink {
	repoName := repo.Name
	if isWiki {
		repoName += ".wiki"
	}

	repo.Owner = repo.MustOwner()
	cl := new(CloneLink)
	if setting.SSH.Port != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", setting.RunUser, setting.SSH.Domain, setting.SSH.Port, repo.Owner.Name, repoName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", setting.RunUser, setting.SSH.Domain, repo.Owner.Name, repoName)
	}
	cl.HTTPS = fmt.Sprintf("%s%s/%s.git", setting.AppUrl, repo.Owner.Name, repoName)
	return cl
}

// CloneLink returns clone URLs of repository.
func (repo *Repository) CloneLink() (cl *CloneLink) {
	return repo.cloneLink(false)
}

var (
	reservedNames    = []string{"debug", "raw", "install", "api", "avatar", "user", "org", "help", "stars", "issues", "pulls", "commits", "repo", "template", "admin", "new"}
	reservedPatterns = []string{"*.git", "*.keys", "*.wiki"}
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
	ID       int64 `xorm:"pk autoincr"`
	RepoID   int64
	Repo     *Repository `xorm:"-"`
	Interval int         // Hour.

	Updated        time.Time `xorm:"-"`
	UpdatedUnix    int64
	NextUpdate     time.Time `xorm:"-"`
	NextUpdateUnix int64

	address string `xorm:"-"`
}

func (m *Mirror) BeforeInsert() {
	m.NextUpdateUnix = m.NextUpdate.UTC().Unix()
}

func (m *Mirror) BeforeUpdate() {
	m.UpdatedUnix = time.Now().UTC().Unix()
	m.NextUpdateUnix = m.NextUpdate.UTC().Unix()
}

func (m *Mirror) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "repo_id":
		m.Repo, err = GetRepositoryByID(m.RepoID)
		if err != nil {
			log.Error(3, "GetRepositoryByID[%d]: %v", m.ID, err)
		}
	case "updated_unix":
		m.Updated = time.Unix(m.UpdatedUnix, 0).Local()
	case "next_updated_unix":
		m.NextUpdate = time.Unix(m.NextUpdateUnix, 0).Local()
	}
}

func (m *Mirror) readAddress() {
	if len(m.address) > 0 {
		return
	}

	cfg, err := ini.Load(m.Repo.GitConfigPath())
	if err != nil {
		log.Error(4, "Load: %v", err)
		return
	}
	m.address = cfg.Section("remote \"origin\"").Key("url").Value()
}

// HandleCloneUserCredentials replaces user credentials from HTTP/HTTPS URL
// with placeholder <credentials>.
// It will fail for any other forms of clone addresses.
func HandleCloneUserCredentials(url string, mosaics bool) string {
	i := strings.Index(url, "@")
	if i == -1 {
		return url
	}
	start := strings.Index(url, "://")
	if start == -1 {
		return url
	}
	if mosaics {
		return url[:start+3] + "<credentials>" + url[i:]
	}
	return url[:start+3] + url[i+1:]
}

// Address returns mirror address from Git repository config without credentials.
func (m *Mirror) Address() string {
	m.readAddress()
	return HandleCloneUserCredentials(m.address, false)
}

// FullAddress returns mirror address from Git repository config.
func (m *Mirror) FullAddress() string {
	m.readAddress()
	return m.address
}

// SaveAddress writes new address to Git repository config.
func (m *Mirror) SaveAddress(addr string) error {
	configPath := m.Repo.GitConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("Load: %v", err)
	}

	cfg.Section("remote \"origin\"").Key("url").SetValue(addr)
	return cfg.SaveToIndent(configPath, "\t")
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

func DeleteMirrorByRepoID(repoID int64) error {
	_, err := x.Delete(&Mirror{RepoID: repoID})
	return err
}

func createUpdateHook(repoPath string) error {
	return git.SetUpdateHook(repoPath,
		fmt.Sprintf(_TPL_UPDATE_HOOK, setting.ScriptType, "\""+setting.AppPath+"\"", setting.CustomConf))
}

type MigrateRepoOptions struct {
	Name        string
	Description string
	IsPrivate   bool
	IsMirror    bool
	RemoteAddr  string
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(u *User, opts MigrateRepoOptions) (*Repository, error) {
	repo, err := CreateRepository(u, CreateRepoOptions{
		Name:        opts.Name,
		Description: opts.Description,
		IsPrivate:   opts.IsPrivate,
		IsMirror:    opts.IsMirror,
	})
	if err != nil {
		return nil, err
	}

	// Clone to temprory path and do the init commit.
	tmpDir := filepath.Join(os.TempDir(), fmt.Sprintf("%d", time.Now().Nanosecond()))
	os.MkdirAll(tmpDir, os.ModePerm)

	repoPath := RepoPath(u.Name, opts.Name)

	if u.IsOrganization() {
		t, err := u.GetOwnerTeam()
		if err != nil {
			return nil, err
		}
		repo.NumWatches = t.NumMembers
	} else {
		repo.NumWatches = 1
	}

	os.RemoveAll(repoPath)
	if err = git.Clone(opts.RemoteAddr, repoPath, git.CloneRepoOptions{
		Mirror:  true,
		Quiet:   true,
		Timeout: time.Duration(setting.Git.Timeout.Migrate) * time.Second,
	}); err != nil {
		return repo, fmt.Errorf("Clone: %v", err)
	}

	// Check if repository is empty.
	_, stderr, err := com.ExecCmdDir(repoPath, "git", "log", "-1")
	if err != nil {
		if strings.Contains(stderr, "fatal: bad default revision 'HEAD'") {
			repo.IsBare = true
		} else {
			return repo, fmt.Errorf("check bare: %v - %s", err, stderr)
		}
	}

	if !repo.IsBare {
		// Try to get HEAD branch and set it as default branch.
		gitRepo, err := git.OpenRepository(repoPath)
		if err != nil {
			return repo, fmt.Errorf("OpenRepository: %v", err)
		}
		headBranch, err := gitRepo.GetHEADBranch()
		if err != nil {
			return repo, fmt.Errorf("GetHEADBranch: %v", err)
		}
		if headBranch != nil {
			repo.DefaultBranch = headBranch.Name
		}
	}

	if opts.IsMirror {
		if _, err = x.InsertOne(&Mirror{
			RepoID:     repo.ID,
			Interval:   24,
			NextUpdate: time.Now().Add(24 * time.Hour),
		}); err != nil {
			return repo, fmt.Errorf("InsertOne: %v", err)
		}

		repo.IsMirror = true
		return repo, UpdateRepository(repo, false)
	}

	return CleanUpMigrateInfo(repo, repoPath)
}

// Finish migrating repository with things that don't need to be done for mirrors.
func CleanUpMigrateInfo(repo *Repository, repoPath string) (*Repository, error) {
	if err := createUpdateHook(repoPath); err != nil {
		return repo, fmt.Errorf("createUpdateHook: %v", err)
	}

	// Clean up mirror info which prevents "push --all".
	// This also removes possible user credentials.
	configPath := repo.GitConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return repo, fmt.Errorf("open config file: %v", err)
	}
	cfg.DeleteSection("remote \"origin\"")
	if err = cfg.SaveToIndent(configPath, "\t"); err != nil {
		return repo, fmt.Errorf("save config file: %v", err)
	}

	return repo, UpdateRepository(repo, false)
}

// initRepoCommit temporarily changes with work directory.
func initRepoCommit(tmpPath string, sig *git.Signature) (err error) {
	var stderr string
	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit (git add): %s", tmpPath),
		"git", "add", "--all"); err != nil {
		return fmt.Errorf("git add: %s", stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit (git commit): %s", tmpPath),
		"git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "initial commit"); err != nil {
		return fmt.Errorf("git commit: %s", stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit (git push): %s", tmpPath),
		"git", "push", "origin", "master"); err != nil {
		return fmt.Errorf("git push: %s", stderr)
	}
	return nil
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

	cloneLink := repo.CloneLink()
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
func initRepository(e Engine, repoPath string, u *User, repo *Repository, opts CreateRepoOptions) (err error) {
	// Somehow the directory could exist.
	if com.IsExist(repoPath) {
		return fmt.Errorf("initRepository: path already exists: %s", repoPath)
	}

	// Init bare new repository.
	if err = git.InitRepository(repoPath, true); err != nil {
		return fmt.Errorf("InitRepository: %v", err)
	} else if err = createUpdateHook(repoPath); err != nil {
		return fmt.Errorf("createUpdateHook: %v", err)
	}

	tmpDir := filepath.Join(os.TempDir(), "gogs-"+repo.Name+"-"+com.ToStr(time.Now().Nanosecond()))

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
	}

	u.NumRepos++
	// Remember visibility preference.
	u.LastRepoVisibility = repo.IsPrivate
	if err = updateUser(e, u); err != nil {
		return fmt.Errorf("updateUser: %v", err)
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
	if !u.CanCreateRepo() {
		return nil, ErrReachLimitOfRepo{u.MaxRepoCreation}
	}

	repo := &Repository{
		OwnerID:      u.Id,
		Owner:        u,
		Name:         opts.Name,
		LowerName:    strings.ToLower(opts.Name),
		Description:  opts.Description,
		IsPrivate:    opts.IsPrivate,
		EnableWiki:   true,
		EnableIssues: true,
		EnablePulls:  true,
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

func countRepositories(showPrivate bool) int64 {
	sess := x.NewSession()

	if !showPrivate {
		sess.Where("is_private=?", false)
	}

	count, err := sess.Count(new(Repository))
	if err != nil {
		log.Error(4, "countRepositories: %v", err)
	}
	return count
}

// CountRepositories returns number of repositories.
func CountRepositories() int64 {
	return countRepositories(true)
}

// CountPublicRepositories returns number of public repositories.
func CountPublicRepositories() int64 {
	return countRepositories(false)
}

func Repositories(page, pageSize int) (_ []*Repository, err error) {
	repos := make([]*Repository, 0, pageSize)
	return repos, x.Limit(pageSize, (page-1)*pageSize).Asc("id").Find(&repos)
}

// RepositoriesWithUsers returns number of repos in given page.
func RepositoriesWithUsers(page, pageSize int) (_ []*Repository, err error) {
	repos, err := Repositories(page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("Repositories: %v", err)
	}

	for i := range repos {
		if err = repos[i].GetOwner(); err != nil {
			return nil, err
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
	collaborators, err := repo.getCollaborators(sess)
	if err != nil {
		return fmt.Errorf("getCollaborators: %v", err)
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
		t, err := newOwner.getOwnerTeam(sess)
		if err != nil {
			return fmt.Errorf("getOwnerTeam: %v", err)
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

	// Rename remote repository to new path and delete local copy.
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
		return fmt.Errorf("rename repository directory: %v", err)
	}
	RemoveAllWithNotice("Delete repository local copy", repo.LocalCopyPath())

	// Rename remote wiki repository to new path and delete local copy.
	wikiPath := WikiPath(owner.Name, repo.Name)
	if com.IsExist(wikiPath) {
		RemoveAllWithNotice("Delete repository wiki local copy", repo.LocalWikiPath())
		if err = os.Rename(wikiPath, WikiPath(newOwner.Name, repo.Name)); err != nil {
			return fmt.Errorf("rename repository wiki: %v", err)
		}
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

	repo, err := GetRepositoryByName(u.Id, oldRepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName: %v", err)
	}

	// Change repository directory name.
	if err = os.Rename(repo.RepoPath(), RepoPath(u.Name, newRepoName)); err != nil {
		return fmt.Errorf("rename repository directory: %v", err)
	}

	wikiPath := repo.WikiPath()
	if com.IsExist(wikiPath) {
		if err = os.Rename(wikiPath, WikiPath(u.Name, newRepoName)); err != nil {
			return fmt.Errorf("rename repository wiki: %v", err)
		}
		RemoveAllWithNotice("Delete repository wiki local copy", repo.LocalWikiPath())
	}

	return nil
}

func getRepositoriesByForkID(e Engine, forkID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, e.Where("fork_id=?", forkID).Find(&repos)
}

// GetRepositoriesByForkID returns all repositories with given fork ID.
func GetRepositoriesByForkID(forkID int64) ([]*Repository, error) {
	return getRepositoriesByForkID(x, forkID)
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
		if repo.Owner.IsOrganization() {
			// Organization repository need to recalculate access table when visivility is changed.
			if err = repo.recalculateTeamAccesses(e, 0); err != nil {
				return fmt.Errorf("recalculateTeamAccesses: %v", err)
			}
		}

		forkRepos, err := getRepositoriesByForkID(e, repo.ID)
		if err != nil {
			return fmt.Errorf("getRepositoriesByForkID: %v", err)
		}
		for i := range forkRepos {
			forkRepos[i].IsPrivate = repo.IsPrivate
			if err = updateRepository(e, forkRepos[i], true); err != nil {
				return fmt.Errorf("updateRepository[%d]: %v", forkRepos[i].ID, err)
			}
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
func DeleteRepository(uid, repoID int64) error {
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

	if err = deleteBeans(sess,
		&Repository{ID: repoID},
		&Access{RepoID: repo.ID},
		&Action{RepoID: repo.ID},
		&Watch{RepoID: repoID},
		&Star{RepoID: repoID},
		&Mirror{RepoID: repoID},
		&IssueUser{RepoID: repoID},
		&Milestone{RepoID: repoID},
		&Release{RepoID: repoID},
		&Collaboration{RepoID: repoID},
		&PullRequest{BaseRepoID: repoID},
	); err != nil {
		return fmt.Errorf("deleteBeans: %v", err)
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
			return fmt.Errorf("decrease fork count: %v", err)
		}
	}

	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", uid); err != nil {
		return err
	}

	// Remove repository files.
	repoPath := repo.repoPath(sess)
	RemoveAllWithNotice("Delete repository files", repoPath)

	repo.DeleteWiki()

	// Remove attachment files.
	for i := range attachmentPaths {
		RemoveAllWithNotice("Delete attachment", attachmentPaths[i])
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	if repo.NumForks > 0 {
		if repo.IsPrivate {
			forkRepos, err := GetRepositoriesByForkID(repo.ID)
			if err != nil {
				return fmt.Errorf("getRepositoriesByForkID: %v", err)
			}
			for i := range forkRepos {
				if err = DeleteRepository(forkRepos[i].OwnerID, forkRepos[i].ID); err != nil {
					log.Error(4, "DeleteRepository [%d]: %v", forkRepos[i].ID, err)
				}
			}
		} else {
			if _, err = x.Exec("UPDATE `repository` SET fork_id=0,is_fork=? WHERE fork_id=?", false, repo.ID); err != nil {
				log.Error(4, "reset 'fork_id' and 'is_fork': %v", err)
			}
		}
	}

	return nil
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
	sess := x.Desc("updated_unix")

	if !private {
		sess.Where("is_private=?", false)
	}

	return repos, sess.Find(&repos, &Repository{OwnerID: uid})
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories(page, pageSize int) (repos []*Repository, err error) {
	return repos, x.Limit(pageSize, (page-1)*pageSize).
		Where("is_private=?", false).Limit(pageSize).Desc("updated_unix").Find(&repos)
}

func getRepositoryCount(e Engine, u *User) (int64, error) {
	return x.Count(&Repository{OwnerID: u.Id})
}

// GetRepositoryCount returns the total number of repositories of user.
func GetRepositoryCount(u *User) (int64, error) {
	return getRepositoryCount(x, u)
}

type SearchRepoOptions struct {
	Keyword  string
	OwnerID  int64
	OrderBy  string
	Private  bool // Include private repositories in results
	Page     int
	PageSize int // Can be smaller than or equal to setting.ExplorePagingNum
}

// SearchRepositoryByName takes keyword and part of repository name to search,
// it returns results in given range and number of total results.
func SearchRepositoryByName(opts *SearchRepoOptions) (repos []*Repository, _ int64, _ error) {
	if len(opts.Keyword) == 0 {
		return repos, 0, nil
	}
	opts.Keyword = strings.ToLower(opts.Keyword)

	if opts.PageSize <= 0 || opts.PageSize > setting.ExplorePagingNum {
		opts.PageSize = setting.ExplorePagingNum
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}

	repos = make([]*Repository, 0, opts.PageSize)

	// Append conditions
	sess := x.Where("LOWER(lower_name) LIKE ?", "%"+opts.Keyword+"%")
	if opts.OwnerID > 0 {
		sess.And("owner_id = ?", opts.OwnerID)
	}
	if !opts.Private {
		sess.And("is_private=?", false)
	}

	var countSess xorm.Session
	countSess = *sess
	count, err := countSess.Count(new(Repository))
	if err != nil {
		return nil, 0, fmt.Errorf("Count: %v", err)
	}

	if len(opts.OrderBy) > 0 {
		sess.OrderBy(opts.OrderBy)
	}
	return repos, count, sess.Limit(opts.PageSize, (opts.Page-1)*opts.PageSize).Find(&repos)
}

// DeleteRepositoryArchives deletes all repositories' archives.
func DeleteRepositoryArchives() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			return os.RemoveAll(filepath.Join(repo.RepoPath(), "archives"))
		})
}

func gatherMissingRepoRecords() ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	if err := x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if !com.IsDir(repo.RepoPath()) {
				repos = append(repos, repo)
			}
			return nil
		}); err != nil {
		if err2 := CreateRepositoryNotice(fmt.Sprintf("gatherMissingRepoRecords: %v", err)); err2 != nil {
			return nil, fmt.Errorf("CreateRepositoryNotice: %v", err)
		}
	}
	return repos, nil
}

// DeleteMissingRepositories deletes all repository records that lost Git files.
func DeleteMissingRepositories() error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return fmt.Errorf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		log.Trace("Deleting %d/%d...", repo.OwnerID, repo.ID)
		if err := DeleteRepository(repo.OwnerID, repo.ID); err != nil {
			if err2 := CreateRepositoryNotice(fmt.Sprintf("DeleteRepository [%d]: %v", repo.ID, err)); err2 != nil {
				return fmt.Errorf("CreateRepositoryNotice: %v", err)
			}
		}
	}
	return nil
}

// ReinitMissingRepositories reinitializes all repository records that lost Git files.
func ReinitMissingRepositories() error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return fmt.Errorf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		log.Trace("Initializing %d/%d...", repo.OwnerID, repo.ID)
		if err := git.InitRepository(repo.RepoPath(), true); err != nil {
			if err2 := CreateRepositoryNotice(fmt.Sprintf("InitRepository [%d]: %v", repo.ID, err)); err2 != nil {
				return fmt.Errorf("CreateRepositoryNotice: %v", err)
			}
		}
	}
	return nil
}

// RewriteRepositoryUpdateHook rewrites all repositories' update hook.
func RewriteRepositoryUpdateHook() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			return createUpdateHook(repo.RepoPath())
		})
}

// statusPool represents a pool of status with true/false.
type statusPool struct {
	lock sync.RWMutex
	pool map[string]bool
}

// Start sets value of given name to true in the pool.
func (p *statusPool) Start(name string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool[name] = true
}

// Stop sets value of given name to false in the pool.
func (p *statusPool) Stop(name string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool[name] = false
}

// IsRunning checks if value of given name is set to true in the pool.
func (p *statusPool) IsRunning(name string) bool {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.pool[name]
}

// Prevent duplicate running tasks.
var taskStatusPool = &statusPool{
	pool: make(map[string]bool),
}

const (
	_MIRROR_UPDATE = "mirror_update"
	_GIT_FSCK      = "git_fsck"
	_CHECK_REPOs   = "check_repos"
)

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if taskStatusPool.IsRunning(_MIRROR_UPDATE) {
		return
	}
	taskStatusPool.Start(_MIRROR_UPDATE)
	defer taskStatusPool.Stop(_MIRROR_UPDATE)

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

		repoPath := m.Repo.RepoPath()
		if _, stderr, err := process.ExecDir(
			time.Duration(setting.Git.Timeout.Mirror)*time.Second,
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
	if taskStatusPool.IsRunning(_GIT_FSCK) {
		return
	}
	taskStatusPool.Start(_GIT_FSCK)
	defer taskStatusPool.Stop(_GIT_FSCK)

	log.Trace("Doing: GitFsck")

	if err := x.Where("id>0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			repoPath := repo.RepoPath()
			if err := git.Fsck(repoPath, setting.Cron.RepoHealthCheck.Timeout, setting.Cron.RepoHealthCheck.Args...); err != nil {
				desc := fmt.Sprintf("Fail to health check repository (%s): %v", repoPath, err)
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

type repoChecker struct {
	querySQL, correctSQL string
	desc                 string
}

func repoStatsCheck(checker *repoChecker) {
	results, err := x.Query(checker.querySQL)
	if err != nil {
		log.Error(4, "Select %s: %v", checker.desc, err)
		return
	}
	for _, result := range results {
		id := com.StrTo(result["id"]).MustInt64()
		log.Trace("Updating %s: %d", checker.desc, id)
		_, err = x.Exec(checker.correctSQL, id, id)
		if err != nil {
			log.Error(4, "Update %s[%d]: %v", checker.desc, id, err)
		}
	}
}

func CheckRepoStats() {
	if taskStatusPool.IsRunning(_CHECK_REPOs) {
		return
	}
	taskStatusPool.Start(_CHECK_REPOs)
	defer taskStatusPool.Stop(_CHECK_REPOs)

	log.Trace("Doing: CheckRepoStats")

	checkers := []*repoChecker{
		// Repository.NumWatches
		{
			"SELECT repo.id FROM `repository` repo WHERE repo.num_watches!=(SELECT COUNT(*) FROM `watch` WHERE repo_id=repo.id)",
			"UPDATE `repository` SET num_watches=(SELECT COUNT(*) FROM `watch` WHERE repo_id=?) WHERE id=?",
			"repository count 'num_watches'",
		},
		// Repository.NumStars
		{
			"SELECT repo.id FROM `repository` repo WHERE repo.num_stars!=(SELECT COUNT(*) FROM `star` WHERE repo_id=repo.id)",
			"UPDATE `repository` SET num_stars=(SELECT COUNT(*) FROM `star` WHERE repo_id=?) WHERE id=?",
			"repository count 'num_stars'",
		},
		// Label.NumIssues
		{
			"SELECT label.id FROM `label` WHERE label.num_issues!=(SELECT COUNT(*) FROM `issue_label` WHERE label_id=label.id)",
			"UPDATE `label` SET num_issues=(SELECT COUNT(*) FROM `issue_label` WHERE label_id=?) WHERE id=?",
			"label count 'num_issues'",
		},
		// User.NumRepos
		{
			"SELECT `user`.id FROM `user` WHERE `user`.num_repos!=(SELECT COUNT(*) FROM `repository` WHERE owner_id=`user`.id)",
			"UPDATE `user` SET num_repos=(SELECT COUNT(*) FROM `repository` WHERE owner_id=?) WHERE id=?",
			"user count 'num_repos'",
		},
		// Issue.NumComments
		{
			"SELECT `issue`.id FROM `issue` WHERE `issue`.num_comments!=(SELECT COUNT(*) FROM `comment` WHERE issue_id=`issue`.id AND type=0)",
			"UPDATE `issue` SET num_comments=(SELECT COUNT(*) FROM `comment` WHERE issue_id=? AND type=0) WHERE id=?",
			"issue count 'num_comments'",
		},
	}
	for i := range checkers {
		repoStatsCheck(checkers[i])
	}

	// FIXME: use checker when v0.9, stop supporting old fork repo format.
	// ***** START: Repository.NumForks *****
	results, err := x.Query("SELECT repo.id FROM `repository` repo WHERE repo.num_forks!=(SELECT COUNT(*) FROM `repository` WHERE fork_id=repo.id)")
	if err != nil {
		log.Error(4, "Select repository count 'num_forks': %v", err)
	} else {
		for _, result := range results {
			id := com.StrTo(result["id"]).MustInt64()
			log.Trace("Updating repository count 'num_forks': %d", id)

			repo, err := GetRepositoryByID(id)
			if err != nil {
				log.Error(4, "GetRepositoryByID[%d]: %v", id, err)
				continue
			}

			rawResult, err := x.Query("SELECT COUNT(*) FROM `repository` WHERE fork_id=?", repo.ID)
			if err != nil {
				log.Error(4, "Select count of forks[%d]: %v", repo.ID, err)
				continue
			}
			repo.NumForks = int(parseCountResult(rawResult))

			if err = UpdateRepository(repo, false); err != nil {
				log.Error(4, "UpdateRepository[%d]: %v", id, err)
				continue
			}
		}
	}
	// ***** END: Repository.NumForks *****
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

func isWatching(e Engine, uid, repoId int64) bool {
	has, _ := e.Get(&Watch{0, uid, repoId})
	return has
}

// IsWatching checks if user has watched given repository.
func IsWatching(uid, repoId int64) bool {
	return isWatching(x, uid, repoId)
}

func watchRepo(e Engine, uid, repoId int64, watch bool) (err error) {
	if watch {
		if isWatching(e, uid, repoId) {
			return nil
		}
		if _, err = e.Insert(&Watch{RepoID: repoId, UserID: uid}); err != nil {
			return err
		}
		_, err = e.Exec("UPDATE `repository` SET num_watches = num_watches + 1 WHERE id = ?", repoId)
	} else {
		if !isWatching(e, uid, repoId) {
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

func getWatchers(e Engine, repoID int64) ([]*Watch, error) {
	watches := make([]*Watch, 0, 10)
	return watches, e.Find(&watches, &Watch{RepoID: repoID})
}

// GetWatchers returns all watchers of given repository.
func GetWatchers(repoID int64) ([]*Watch, error) {
	return getWatchers(x, repoID)
}

// Repository.GetWatchers returns range of users watching given repository.
func (repo *Repository) GetWatchers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("watch.repo_id=?", repo.ID)
	if setting.UsePostgreSQL {
		sess = sess.Join("LEFT", "watch", `"user".id=watch.user_id`)
	} else {
		sess = sess.Join("LEFT", "watch", "user.id=watch.user_id")
	}
	return users, sess.Find(&users)
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
	UID    int64 `xorm:"UNIQUE(s)"`
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

func (repo *Repository) GetStargazers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("star.repo_id=?", repo.ID)
	if setting.UsePostgreSQL {
		sess = sess.Join("LEFT", "star", `"user".id=star.uid`)
	} else {
		sess = sess.Join("LEFT", "star", "user.id=star.uid")
	}
	return users, sess.Find(&users)
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
		OwnerID:       u.Id,
		Owner:         u,
		Name:          name,
		LowerName:     strings.ToLower(name),
		Description:   desc,
		DefaultBranch: oldRepo.DefaultBranch,
		IsPrivate:     oldRepo.IsPrivate,
		IsFork:        true,
		ForkID:        oldRepo.ID,
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

	repoPath := RepoPath(u.Name, repo.Name)
	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("ForkRepository(git clone): %s/%s", u.Name, repo.Name),
		"git", "clone", "--bare", oldRepo.RepoPath(), repoPath)
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

func (repo *Repository) GetForks() ([]*Repository, error) {
	forks := make([]*Repository, 0, repo.NumForks)
	return forks, x.Find(&forks, &Repository{ForkID: repo.ID})
}
