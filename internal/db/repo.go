// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nfnt/resize"
	"github.com/unknwon/cae/zip"
	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/semverutil"
	"gogs.io/gogs/internal/sync"
)

// REPO_AVATAR_URL_PREFIX is used to identify a URL is to access repository avatar.
const REPO_AVATAR_URL_PREFIX = "repo-avatars"

var repoWorkingPool = sync.NewExclusivePool()

var (
	Gitignores, Licenses, Readmes, LabelTemplates []string

	// Maximum items per page in forks, watchers and stars of a repo
	ItemsPerPage = 40
)

func LoadRepoConfig() {
	// Load .gitignore and license files and readme templates.
	types := []string{"gitignore", "license", "readme", "label"}
	typeFiles := make([][]string, 4)
	for i, t := range types {
		files, err := conf.AssetDir("conf/" + t)
		if err != nil {
			log.Fatal("Failed to get %s files: %v", t, err)
		}
		customPath := filepath.Join(conf.CustomDir(), "conf", t)
		if com.IsDir(customPath) {
			customFiles, err := com.StatDir(customPath)
			if err != nil {
				log.Fatal("Failed to get custom %s files: %v", t, err)
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
	LabelTemplates = typeFiles[3]
	sort.Strings(Gitignores)
	sort.Strings(Licenses)
	sort.Strings(Readmes)
	sort.Strings(LabelTemplates)

	// Filter out invalid names and promote preferred licenses.
	sortedLicenses := make([]string, 0, len(Licenses))
	for _, name := range conf.Repository.PreferredLicenses {
		if com.IsSliceContainsStr(Licenses, name) {
			sortedLicenses = append(sortedLicenses, name)
		}
	}
	for _, name := range Licenses {
		if !com.IsSliceContainsStr(conf.Repository.PreferredLicenses, name) {
			sortedLicenses = append(sortedLicenses, name)
		}
	}
	Licenses = sortedLicenses
}

func NewRepoContext() {
	zip.Verbose = false

	// Check Git installation.
	if _, err := exec.LookPath("git"); err != nil {
		log.Fatal("Failed to test 'git' command: %v (forgotten install?)", err)
	}

	// Check Git version.
	var err error
	conf.Git.Version, err = git.BinVersion()
	if err != nil {
		log.Fatal("Failed to get Git version: %v", err)
	}

	log.Trace("Git version: %s", conf.Git.Version)
	if semverutil.Compare(conf.Git.Version, "<", "1.8.3") {
		log.Fatal("Gogs requires Git version greater or equal to 1.8.3")
	}

	// Git requires setting user.name and user.email in order to commit changes.
	for configKey, defaultValue := range map[string]string{"user.name": "Gogs", "user.email": "gogs@fake.local"} {
		if stdout, stderr, err := process.Exec("NewRepoContext(get setting)", "git", "config", "--get", configKey); err != nil || strings.TrimSpace(stdout) == "" {
			// ExitError indicates this config is not set
			if _, ok := err.(*exec.ExitError); ok || strings.TrimSpace(stdout) == "" {
				if _, stderr, gerr := process.Exec("NewRepoContext(set "+configKey+")", "git", "config", "--global", configKey, defaultValue); gerr != nil {
					log.Fatal("Failed to set git %s(%s): %s", configKey, gerr, stderr)
				}
				log.Info("Git config %s set to %s", configKey, defaultValue)
			} else {
				log.Fatal("Failed to get git %s(%s): %s", configKey, err, stderr)
			}
		}
	}

	// Set git some configurations.
	if _, stderr, err := process.Exec("NewRepoContext(git config --global core.quotepath false)",
		"git", "config", "--global", "core.quotepath", "false"); err != nil {
		log.Fatal("Failed to execute 'git config --global core.quotepath false': %v - %s", err, stderr)
	}

	RemoveAllWithNotice("Clean up repository temporary data", filepath.Join(conf.Server.AppDataPath, "tmp"))
}

// Repository contains information of a repository.
type Repository struct {
	ID              int64
	OwnerID         int64  `xorm:"UNIQUE(s)" gorm:"UNIQUE_INDEX:s"`
	Owner           *User  `xorm:"-" gorm:"-" json:"-"`
	LowerName       string `xorm:"UNIQUE(s) INDEX NOT NULL" gorm:"UNIQUE_INDEX:s"`
	Name            string `xorm:"INDEX NOT NULL" gorm:"NOT NULL"`
	Description     string `xorm:"VARCHAR(512)" gorm:"TYPE:VARCHAR(512)"`
	Website         string
	DefaultBranch   string
	Size            int64 `xorm:"NOT NULL DEFAULT 0" gorm:"NOT NULL;DEFAULT:0"`
	UseCustomAvatar bool

	// Counters
	NumWatches          int
	NumStars            int
	NumForks            int
	NumIssues           int
	NumClosedIssues     int
	NumOpenIssues       int `xorm:"-" gorm:"-" json:"-"`
	NumPulls            int
	NumClosedPulls      int
	NumOpenPulls        int `xorm:"-" gorm:"-" json:"-"`
	NumMilestones       int `xorm:"NOT NULL DEFAULT 0" gorm:"NOT NULL;DEFAULT:0"`
	NumClosedMilestones int `xorm:"NOT NULL DEFAULT 0" gorm:"NOT NULL;DEFAULT:0"`
	NumOpenMilestones   int `xorm:"-" gorm:"-" json:"-"`
	NumTags             int `xorm:"-" gorm:"-" json:"-"`

	IsPrivate  bool
	IsUnlisted bool
	IsBare     bool

	IsMirror bool
	*Mirror  `xorm:"-" gorm:"-" json:"-"`

	// Advanced settings
	EnableWiki            bool `xorm:"NOT NULL DEFAULT true" gorm:"NOT NULL;DEFAULT:TRUE"`
	AllowPublicWiki       bool
	EnableExternalWiki    bool
	ExternalWikiURL       string
	EnableIssues          bool `xorm:"NOT NULL DEFAULT true" gorm:"NOT NULL;DEFAULT:TRUE"`
	AllowPublicIssues     bool
	EnableExternalTracker bool
	ExternalTrackerURL    string
	ExternalTrackerFormat string
	ExternalTrackerStyle  string
	ExternalMetas         map[string]string `xorm:"-" gorm:"-" json:"-"`
	EnablePulls           bool              `xorm:"NOT NULL DEFAULT true" gorm:"NOT NULL;DEFAULT:TRUE"`
	PullsIgnoreWhitespace bool              `xorm:"NOT NULL DEFAULT false" gorm:"NOT NULL;DEFAULT:FALSE"`
	PullsAllowRebase      bool              `xorm:"NOT NULL DEFAULT false" gorm:"NOT NULL;DEFAULT:FALSE"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false" gorm:"NOT NULL;DEFAULT:FALSE"`
	ForkID   int64
	BaseRepo *Repository `xorm:"-" gorm:"-" json:"-"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64
}

func (repo *Repository) BeforeInsert() {
	repo.CreatedUnix = time.Now().Unix()
	repo.UpdatedUnix = repo.CreatedUnix
}

func (repo *Repository) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "default_branch":
		// FIXME: use db migration to solve all at once.
		if len(repo.DefaultBranch) == 0 {
			repo.DefaultBranch = "master"
		}
	case "num_closed_issues":
		repo.NumOpenIssues = repo.NumIssues - repo.NumClosedIssues
	case "num_closed_pulls":
		repo.NumOpenPulls = repo.NumPulls - repo.NumClosedPulls
	case "num_closed_milestones":
		repo.NumOpenMilestones = repo.NumMilestones - repo.NumClosedMilestones
	case "external_tracker_style":
		if len(repo.ExternalTrackerStyle) == 0 {
			repo.ExternalTrackerStyle = markup.ISSUE_NAME_STYLE_NUMERIC
		}
	case "created_unix":
		repo.Created = time.Unix(repo.CreatedUnix, 0).Local()
	case "updated_unix":
		repo.Updated = time.Unix(repo.UpdatedUnix, 0)
	}
}

func (repo *Repository) loadAttributes(e Engine) (err error) {
	if repo.Owner == nil {
		repo.Owner, err = getUserByID(e, repo.OwnerID)
		if err != nil {
			return fmt.Errorf("getUserByID [%d]: %v", repo.OwnerID, err)
		}
	}

	if repo.IsFork && repo.BaseRepo == nil {
		repo.BaseRepo, err = getRepositoryByID(e, repo.ForkID)
		if err != nil {
			if IsErrRepoNotExist(err) {
				repo.IsFork = false
				repo.ForkID = 0
			} else {
				return fmt.Errorf("get fork repository by ID: %v", err)
			}
		}
	}

	return nil
}

func (repo *Repository) LoadAttributes() error {
	return repo.loadAttributes(x)
}

// IsPartialPublic returns true if repository is public or allow public access to wiki or issues.
func (repo *Repository) IsPartialPublic() bool {
	return !repo.IsPrivate || repo.AllowPublicWiki || repo.AllowPublicIssues
}

func (repo *Repository) CanGuestViewWiki() bool {
	return repo.EnableWiki && !repo.EnableExternalWiki && repo.AllowPublicWiki
}

func (repo *Repository) CanGuestViewIssues() bool {
	return repo.EnableIssues && !repo.EnableExternalTracker && repo.AllowPublicIssues
}

// MustOwner always returns a valid *User object to avoid conceptually impossible error handling.
// It creates a fake object that contains error deftail when error occurs.
func (repo *Repository) MustOwner() *User {
	return repo.mustOwner(x)
}

func (repo *Repository) FullName() string {
	return repo.MustOwner().Name + "/" + repo.Name
}

func (repo *Repository) HTMLURL() string {
	return conf.Server.ExternalURL + repo.FullName()
}

// CustomAvatarPath returns repository custom avatar file path.
func (repo *Repository) CustomAvatarPath() string {
	return filepath.Join(conf.Picture.RepositoryAvatarUploadPath, com.ToStr(repo.ID))
}

// RelAvatarLink returns relative avatar link to the site domain,
// which includes app sub-url as prefix.
// Since Gravatar support not needed here - just check for image path.
func (repo *Repository) RelAvatarLink() string {
	defaultImgUrl := ""
	if !com.IsExist(repo.CustomAvatarPath()) {
		return defaultImgUrl
	}
	return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, REPO_AVATAR_URL_PREFIX, repo.ID)
}

// AvatarLink returns repository avatar absolute link.
func (repo *Repository) AvatarLink() string {
	link := repo.RelAvatarLink()
	if link[0] == '/' && link[1] != '/' {
		return conf.Server.ExternalURL + strings.TrimPrefix(link, conf.Server.Subpath)[1:]
	}
	return link
}

// UploadAvatar saves custom avatar for repository.
// FIXME: split uploads to different subdirs in case we have massive number of repositories.
func (repo *Repository) UploadAvatar(data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image: %v", err)
	}

	_ = os.MkdirAll(conf.Picture.RepositoryAvatarUploadPath, os.ModePerm)
	fw, err := os.Create(repo.CustomAvatarPath())
	if err != nil {
		return fmt.Errorf("create custom avatar directory: %v", err)
	}
	defer fw.Close()

	m := resize.Resize(avatar.AVATAR_SIZE, avatar.AVATAR_SIZE, img, resize.NearestNeighbor)
	if err = png.Encode(fw, m); err != nil {
		return fmt.Errorf("encode image: %v", err)
	}

	return nil
}

// DeleteAvatar deletes the repository custom avatar.
func (repo *Repository) DeleteAvatar() error {
	log.Trace("DeleteAvatar [%d]: %s", repo.ID, repo.CustomAvatarPath())
	if err := os.Remove(repo.CustomAvatarPath()); err != nil {
		return err
	}

	repo.UseCustomAvatar = false
	return UpdateRepository(repo, false)
}

// This method assumes following fields have been assigned with valid values:
// Required - BaseRepo (if fork)
// Arguments that are allowed to be nil: permission
func (repo *Repository) APIFormat(permission *api.Permission, user ...*User) *api.Repository {
	cloneLink := repo.CloneLink()
	apiRepo := &api.Repository{
		ID:            repo.ID,
		Owner:         repo.Owner.APIFormat(),
		Name:          repo.Name,
		FullName:      repo.FullName(),
		Description:   repo.Description,
		Private:       repo.IsPrivate,
		Fork:          repo.IsFork,
		Empty:         repo.IsBare,
		Mirror:        repo.IsMirror,
		Size:          repo.Size,
		HTMLURL:       repo.HTMLURL(),
		SSHURL:        cloneLink.SSH,
		CloneURL:      cloneLink.HTTPS,
		Website:       repo.Website,
		Stars:         repo.NumStars,
		Forks:         repo.NumForks,
		Watchers:      repo.NumWatches,
		OpenIssues:    repo.NumOpenIssues,
		DefaultBranch: repo.DefaultBranch,
		Created:       repo.Created,
		Updated:       repo.Updated,
		Permissions:   permission,
		// Reserved for go-gogs-client change
		//		AvatarUrl:     repo.AvatarLink(),
	}
	if repo.IsFork {
		p := &api.Permission{Pull: true}
		if len(user) != 0 {
			p.Admin = user[0].IsAdminOfRepo(repo)
			p.Push = user[0].IsWriterOfRepo(repo)
		}
		apiRepo.Parent = repo.BaseRepo.APIFormat(p)
	}
	return apiRepo
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

func (repo *Repository) UpdateSize() error {
	countObject, err := git.RepoCountObjects(repo.RepoPath())
	if err != nil {
		return fmt.Errorf("count repository objects: %v", err)
	}

	repo.Size = countObject.Size + countObject.SizePack
	if _, err = x.Id(repo.ID).Cols("size").Update(repo); err != nil {
		return fmt.Errorf("update size: %v", err)
	}
	return nil
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
		switch repo.ExternalTrackerStyle {
		case markup.ISSUE_NAME_STYLE_ALPHANUMERIC:
			repo.ExternalMetas["style"] = markup.ISSUE_NAME_STYLE_ALPHANUMERIC
		default:
			repo.ExternalMetas["style"] = markup.ISSUE_NAME_STYLE_NUMERIC
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

// getUsersWithAccesMode returns users that have at least given access mode to the repository.
func (repo *Repository) getUsersWithAccesMode(e Engine, mode AccessMode) (_ []*User, err error) {
	if err = repo.getOwner(e); err != nil {
		return nil, err
	}

	accesses := make([]*Access, 0, 10)
	if err = e.Where("repo_id = ? AND mode >= ?", repo.ID, mode).Find(&accesses); err != nil {
		return nil, err
	}

	// Leave a seat for owner itself to append later, but if owner is an organization
	// and just waste 1 unit is cheaper than re-allocate memory once.
	users := make([]*User, 0, len(accesses)+1)
	if len(accesses) > 0 {
		userIDs := make([]int64, len(accesses))
		for i := 0; i < len(accesses); i++ {
			userIDs[i] = accesses[i].UserID
		}

		if err = e.In("id", userIDs).Find(&users); err != nil {
			return nil, err
		}
	}
	if !repo.Owner.IsOrganization() {
		users = append(users, repo.Owner)
	}

	return users, nil
}

// getAssignees returns a list of users who can be assigned to issues in this repository.
func (repo *Repository) getAssignees(e Engine) (_ []*User, err error) {
	return repo.getUsersWithAccesMode(e, AccessModeRead)
}

// GetAssignees returns all users that have read access and can be assigned to issues
// of the repository,
func (repo *Repository) GetAssignees() (_ []*User, err error) {
	return repo.getAssignees(x)
}

// GetAssigneeByID returns the user that has write access of repository by given ID.
func (repo *Repository) GetAssigneeByID(userID int64) (*User, error) {
	return GetAssigneeByID(repo, userID)
}

// GetWriters returns all users that have write access to the repository.
func (repo *Repository) GetWriters() (_ []*User, err error) {
	return repo.getUsersWithAccesMode(x, AccessModeWrite)
}

// GetMilestoneByID returns the milestone belongs to repository by given ID.
func (repo *Repository) GetMilestoneByID(milestoneID int64) (*Milestone, error) {
	return GetMilestoneByRepoID(repo.ID, milestoneID)
}

// IssueStats returns number of open and closed repository issues by given filter mode.
func (repo *Repository) IssueStats(userID int64, filterMode FilterMode, isPull bool) (int64, int64) {
	return GetRepoIssueStats(repo.ID, userID, filterMode, isPull)
}

func (repo *Repository) GetMirror() (err error) {
	repo.Mirror, err = GetMirrorByRepoID(repo.ID)
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

func (repo *Repository) RelLink() string {
	return "/" + repo.FullName()
}

func (repo *Repository) Link() string {
	return conf.Server.Subpath + "/" + repo.FullName()
}

func (repo *Repository) ComposeCompareURL(oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", repo.MustOwner().Name, repo.Name, oldCommitID, newCommitID)
}

func (repo *Repository) HasAccess(userID int64) bool {
	has, _ := HasAccess(userID, repo, AccessModeRead)
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
	return !repo.IsMirror && !repo.IsBare
}

// AllowPulls returns true if repository meets the requirements of accepting pulls and has them enabled.
func (repo *Repository) AllowsPulls() bool {
	return repo.CanEnablePulls() && repo.EnablePulls
}

func (repo *Repository) IsBranchRequirePullRequest(name string) bool {
	return IsBranchOfRepoRequirePullRequest(repo.ID, name)
}

// CanEnableEditor returns true if repository meets the requirements of web editor.
func (repo *Repository) CanEnableEditor() bool {
	return !repo.IsMirror
}

// FIXME: should have a mutex to prevent producing same index for two issues that are created
// closely enough.
func (repo *Repository) NextIssueIndex() int64 {
	return int64(repo.NumIssues+repo.NumPulls) + 1
}

func (repo *Repository) LocalCopyPath() string {
	return filepath.Join(conf.Server.AppDataPath, "tmp", "local-repo", com.ToStr(repo.ID))
}

// UpdateLocalCopy fetches latest changes of given branch from repoPath to localPath.
// It creates a new clone if local copy does not exist, but does not checks out to a
// specific branch if the local copy belongs to a wiki.
// For existing local copy, it checks out to target branch by default, and safe to
// assume subsequent operations are against target branch when caller has confidence
// about no race condition.
func UpdateLocalCopyBranch(repoPath, localPath, branch string, isWiki bool) (err error) {
	if !osutil.IsExist(localPath) {
		// Checkout to a specific branch fails when wiki is an empty repository.
		if isWiki {
			branch = ""
		}
		if err = git.Clone(repoPath, localPath, git.CloneOptions{
			Branch:  branch,
			Timeout: time.Duration(conf.Git.Timeout.Clone) * time.Second,
		}); err != nil {
			return fmt.Errorf("git clone [branch: %s]: %v", branch, err)
		}
		return nil
	}

	gitRepo, err := git.Open(localPath)
	if err != nil {
		return fmt.Errorf("open repository: %v", err)
	}

	if err = gitRepo.Fetch(git.FetchOptions{
		Prune: true,
	}); err != nil {
		return fmt.Errorf("fetch: %v", err)
	}

	if err = gitRepo.Checkout(branch); err != nil {
		return fmt.Errorf("checkout [branch: %s]: %v", branch, err)
	}

	// Reset to align with remote in case of force push.
	rev := "origin/" + branch
	if err = gitRepo.Reset(rev, git.ResetOptions{
		Hard: true,
	}); err != nil {
		return fmt.Errorf("reset [revision: %s]: %v", rev, err)
	}
	return nil
}

// UpdateLocalCopyBranch makes sure local copy of repository in given branch is up-to-date.
func (repo *Repository) UpdateLocalCopyBranch(branch string) error {
	return UpdateLocalCopyBranch(repo.RepoPath(), repo.LocalCopyPath(), branch, false)
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

	if err = os.MkdirAll(filepath.Dir(patchPath), os.ModePerm); err != nil {
		return err
	}
	if err = ioutil.WriteFile(patchPath, patch, 0644); err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	return nil
}

func isRepositoryExist(e Engine, u *User, repoName string) (bool, error) {
	has, err := e.Get(&Repository{
		OwnerID:   u.ID,
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

// ComposeHTTPSCloneURL returns HTTPS clone URL based on given owner and repository name.
func ComposeHTTPSCloneURL(owner, repo string) string {
	return fmt.Sprintf("%s%s/%s.git", conf.Server.ExternalURL, owner, repo)
}

func (repo *Repository) cloneLink(isWiki bool) *CloneLink {
	repoName := repo.Name
	if isWiki {
		repoName += ".wiki"
	}

	repo.Owner = repo.MustOwner()
	cl := new(CloneLink)
	if conf.SSH.Port != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", conf.App.RunUser, conf.SSH.Domain, conf.SSH.Port, repo.Owner.Name, repoName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", conf.App.RunUser, conf.SSH.Domain, repo.Owner.Name, repoName)
	}
	cl.HTTPS = ComposeHTTPSCloneURL(repo.Owner.Name, repoName)
	return cl
}

// CloneLink returns clone URLs of repository.
func (repo *Repository) CloneLink() (cl *CloneLink) {
	return repo.cloneLink(false)
}

type MigrateRepoOptions struct {
	Name        string
	Description string
	IsPrivate   bool
	IsMirror    bool
	RemoteAddr  string
}

/*
	GitHub, GitLab, Gogs: *.wiki.git
	BitBucket: *.git/wiki
*/
var commonWikiURLSuffixes = []string{".wiki.git", ".git/wiki"}

// wikiRemoteURL returns accessible repository URL for wiki if exists.
// Otherwise, it returns an empty string.
func wikiRemoteURL(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")
	for _, suffix := range commonWikiURLSuffixes {
		wikiURL := remote + suffix
		if git.IsURLAccessible(time.Minute, wikiURL) {
			return wikiURL
		}
	}
	return ""
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(doer, owner *User, opts MigrateRepoOptions) (*Repository, error) {
	repo, err := CreateRepository(doer, owner, CreateRepoOptions{
		Name:        opts.Name,
		Description: opts.Description,
		IsPrivate:   opts.IsPrivate,
		IsMirror:    opts.IsMirror,
	})
	if err != nil {
		return nil, err
	}

	repoPath := RepoPath(owner.Name, opts.Name)
	wikiPath := WikiPath(owner.Name, opts.Name)

	if owner.IsOrganization() {
		t, err := owner.GetOwnerTeam()
		if err != nil {
			return nil, err
		}
		repo.NumWatches = t.NumMembers
	} else {
		repo.NumWatches = 1
	}

	migrateTimeout := time.Duration(conf.Git.Timeout.Migrate) * time.Second

	RemoveAllWithNotice("Repository path erase before creation", repoPath)
	if err = git.Clone(opts.RemoteAddr, repoPath, git.CloneOptions{
		Mirror:  true,
		Quiet:   true,
		Timeout: migrateTimeout,
	}); err != nil {
		return repo, fmt.Errorf("clone: %v", err)
	}

	wikiRemotePath := wikiRemoteURL(opts.RemoteAddr)
	if len(wikiRemotePath) > 0 {
		RemoveAllWithNotice("Repository wiki path erase before creation", wikiPath)
		if err = git.Clone(wikiRemotePath, wikiPath, git.CloneOptions{
			Mirror:  true,
			Quiet:   true,
			Timeout: migrateTimeout,
		}); err != nil {
			log.Error("Failed to clone wiki: %v", err)
			RemoveAllWithNotice("Delete repository wiki for initialization failure", wikiPath)
		}
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
		gitRepo, err := git.Open(repoPath)
		if err != nil {
			return repo, fmt.Errorf("open repository: %v", err)
		}
		refspec, err := gitRepo.SymbolicRef()
		if err != nil {
			return repo, fmt.Errorf("get HEAD branch: %v", err)
		}
		repo.DefaultBranch = git.RefShortName(refspec)

		if err = repo.UpdateSize(); err != nil {
			log.Error("UpdateSize [repo_id: %d]: %v", repo.ID, err)
		}
	}

	if opts.IsMirror {
		if _, err = x.InsertOne(&Mirror{
			RepoID:      repo.ID,
			Interval:    conf.Mirror.DefaultInterval,
			EnablePrune: true,
			NextSync:    time.Now().Add(time.Duration(conf.Mirror.DefaultInterval) * time.Hour),
		}); err != nil {
			return repo, fmt.Errorf("InsertOne: %v", err)
		}

		repo.IsMirror = true
		return repo, UpdateRepository(repo, false)
	}

	return CleanUpMigrateInfo(repo)
}

// cleanUpMigrateGitConfig removes mirror info which prevents "push --all".
// This also removes possible user credentials.
func cleanUpMigrateGitConfig(configPath string) error {
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("open config file: %v", err)
	}
	cfg.DeleteSection("remote \"origin\"")
	if err = cfg.SaveToIndent(configPath, "\t"); err != nil {
		return fmt.Errorf("save config file: %v", err)
	}
	return nil
}

var hooksTpls = map[git.HookName]string{
	"pre-receive":  "#!/usr/bin/env %s\n\"%s\" hook --config='%s' pre-receive\n",
	"update":       "#!/usr/bin/env %s\n\"%s\" hook --config='%s' update $1 $2 $3\n",
	"post-receive": "#!/usr/bin/env %s\n\"%s\" hook --config='%s' post-receive\n",
}

func createDelegateHooks(repoPath string) (err error) {
	for _, name := range git.ServerSideHooks {
		hookPath := filepath.Join(repoPath, "hooks", string(name))
		if err = ioutil.WriteFile(hookPath,
			[]byte(fmt.Sprintf(hooksTpls[name], conf.Repository.ScriptType, conf.AppPath(), conf.CustomConf)),
			os.ModePerm); err != nil {
			return fmt.Errorf("create delegate hook '%s': %v", hookPath, err)
		}
	}
	return nil
}

// Finish migrating repository and/or wiki with things that don't need to be done for mirrors.
func CleanUpMigrateInfo(repo *Repository) (*Repository, error) {
	repoPath := repo.RepoPath()
	if err := createDelegateHooks(repoPath); err != nil {
		return repo, fmt.Errorf("createDelegateHooks: %v", err)
	}
	if repo.HasWiki() {
		if err := createDelegateHooks(repo.WikiPath()); err != nil {
			return repo, fmt.Errorf("createDelegateHooks.(wiki): %v", err)
		}
	}

	if err := cleanUpMigrateGitConfig(repo.GitConfigPath()); err != nil {
		return repo, fmt.Errorf("cleanUpMigrateGitConfig: %v", err)
	}
	if repo.HasWiki() {
		if err := cleanUpMigrateGitConfig(path.Join(repo.WikiPath(), "config")); err != nil {
			return repo, fmt.Errorf("cleanUpMigrateGitConfig.(wiki): %v", err)
		}
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
		"-m", "Initial commit"); err != nil {
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
	relPath := path.Join("conf", tp, strings.TrimLeft(path.Clean("/"+name), "/"))

	// Use custom file when available.
	customPath := filepath.Join(conf.CustomDir(), relPath)
	if osutil.IsFile(customPath) {
		return ioutil.ReadFile(customPath)
	}
	return conf.Asset(relPath)
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

// initRepository performs initial commit with chosen setup files on behave of doer.
func initRepository(e Engine, repoPath string, doer *User, repo *Repository, opts CreateRepoOptions) (err error) {
	// Somehow the directory could exist.
	if com.IsExist(repoPath) {
		return fmt.Errorf("initRepository: path already exists: %s", repoPath)
	}

	// Init bare new repository.
	if err = git.Init(repoPath, git.InitOptions{Bare: true}); err != nil {
		return fmt.Errorf("init repository: %v", err)
	} else if err = createDelegateHooks(repoPath); err != nil {
		return fmt.Errorf("createDelegateHooks: %v", err)
	}

	tmpDir := filepath.Join(os.TempDir(), "gogs-"+repo.Name+"-"+com.ToStr(time.Now().Nanosecond()))

	// Initialize repository according to user's choice.
	if opts.AutoInit {
		if err = os.MkdirAll(tmpDir, os.ModePerm); err != nil {
			return err
		}
		defer RemoveAllWithNotice("Delete repository for auto-initialization", tmpDir)

		if err = prepareRepoCommit(repo, tmpDir, repoPath, opts); err != nil {
			return fmt.Errorf("prepareRepoCommit: %v", err)
		}

		// Apply changes and commit.
		if err = initRepoCommit(tmpDir, doer.NewGitSig()); err != nil {
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

var (
	reservedRepoNames    = []string{".", ".."}
	reservedRepoPatterns = []string{"*.git", "*.wiki"}
)

// isRepoNameAllowed return an error if given name is a reserved name or pattern for repositories.
func isRepoNameAllowed(name string) error {
	return isNameAllowed(reservedRepoNames, reservedRepoPatterns, name)
}

func createRepository(e *xorm.Session, doer, owner *User, repo *Repository) (err error) {
	if err = isRepoNameAllowed(repo.Name); err != nil {
		return err
	}

	has, err := isRepositoryExist(e, owner, repo.Name)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerID": owner.ID, "name": repo.Name}}
	}

	if _, err = e.Insert(repo); err != nil {
		return err
	}

	owner.NumRepos++
	// Remember visibility preference.
	owner.LastRepoVisibility = repo.IsPrivate
	if err = updateUser(e, owner); err != nil {
		return fmt.Errorf("updateUser: %v", err)
	}

	// Give access to all members in owner team.
	if owner.IsOrganization() {
		t, err := owner.getOwnerTeam(e)
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

	if err = watchRepo(e, owner.ID, repo.ID, true); err != nil {
		return fmt.Errorf("watchRepo: %v", err)
	} else if err = newRepoAction(e, doer, owner, repo); err != nil {
		return fmt.Errorf("newRepoAction: %v", err)
	}

	return repo.loadAttributes(e)
}

type ErrReachLimitOfRepo struct {
	Limit int
}

func IsErrReachLimitOfRepo(err error) bool {
	_, ok := err.(ErrReachLimitOfRepo)
	return ok
}

func (err ErrReachLimitOfRepo) Error() string {
	return fmt.Sprintf("user has reached maximum limit of repositories [limit: %d]", err.Limit)
}

// CreateRepository creates a repository for given user or organization.
func CreateRepository(doer, owner *User, opts CreateRepoOptions) (_ *Repository, err error) {
	if !owner.CanCreateRepo() {
		return nil, ErrReachLimitOfRepo{Limit: owner.RepoCreationNum()}
	}

	repo := &Repository{
		OwnerID:      owner.ID,
		Owner:        owner,
		Name:         opts.Name,
		LowerName:    strings.ToLower(opts.Name),
		Description:  opts.Description,
		IsPrivate:    opts.IsPrivate,
		EnableWiki:   true,
		EnableIssues: true,
		EnablePulls:  true,
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if err = createRepository(sess, doer, owner, repo); err != nil {
		return nil, err
	}

	// No need for init mirror.
	if !opts.IsMirror {
		repoPath := RepoPath(owner.Name, repo.Name)
		if err = initRepository(sess, repoPath, doer, repo, opts); err != nil {
			RemoveAllWithNotice("Delete repository for initialization failure", repoPath)
			return nil, fmt.Errorf("initRepository: %v", err)
		}

		_, stderr, err := process.ExecDir(-1,
			repoPath, fmt.Sprintf("CreateRepository 'git update-server-info': %s", repoPath),
			"git", "update-server-info")
		if err != nil {
			return nil, fmt.Errorf("CreateRepository 'git update-server-info': %s", stderr)
		}
	}

	return repo, sess.Commit()
}

func countRepositories(userID int64, private bool) int64 {
	sess := x.Where("id > 0")

	if userID > 0 {
		sess.And("owner_id = ?", userID)
	}
	if !private {
		sess.And("is_private=?", false)
	}

	count, err := sess.Count(new(Repository))
	if err != nil {
		log.Error("countRepositories: %v", err)
	}
	return count
}

// CountRepositories returns number of repositories.
// Argument private only takes effect when it is false,
// set it true to count all repositories.
func CountRepositories(private bool) int64 {
	return countRepositories(-1, private)
}

// CountUserRepositories returns number of repositories user owns.
// Argument private only takes effect when it is false,
// set it true to count all repositories.
func CountUserRepositories(userID int64, private bool) int64 {
	return countRepositories(userID, private)
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

// FilterRepositoryWithIssues selects repositories that are using interal issue tracker
// and has disabled external tracker from given set.
// It returns nil if result set is empty.
func FilterRepositoryWithIssues(repoIDs []int64) ([]int64, error) {
	if len(repoIDs) == 0 {
		return nil, nil
	}

	repos := make([]*Repository, 0, len(repoIDs))
	if err := x.Where("enable_issues=?", true).
		And("enable_external_tracker=?", false).
		In("id", repoIDs).
		Cols("id").
		Find(&repos); err != nil {
		return nil, fmt.Errorf("filter valid repositories %v: %v", repoIDs, err)
	}

	if len(repos) == 0 {
		return nil, nil
	}

	repoIDs = make([]int64, len(repos))
	for i := range repos {
		repoIDs[i] = repos[i].ID
	}
	return repoIDs, nil
}

// RepoPath returns repository path by given user and repository name.
func RepoPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), strings.ToLower(repoName)+".git")
}

// TransferOwnership transfers all corresponding setting from old user to new one.
func TransferOwnership(doer *User, newOwnerName string, repo *Repository) error {
	newOwner, err := GetUserByName(newOwnerName)
	if err != nil {
		return fmt.Errorf("get new owner '%s': %v", newOwnerName, err)
	}

	// Check if new owner has repository with same name.
	has, err := IsRepositoryExist(newOwner, repo.Name)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerName": newOwnerName, "name": repo.Name}}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return fmt.Errorf("sess.Begin: %v", err)
	}

	owner := repo.Owner

	// Note: we have to set value here to make sure recalculate accesses is based on
	// new owner.
	repo.OwnerID = newOwner.ID
	repo.Owner = newOwner

	// Update repository.
	if _, err := sess.ID(repo.ID).Update(repo); err != nil {
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
		collaboration.UserID = c.ID
		if c.ID == newOwner.ID || newOwner.IsOrgMember(c.ID) {
			if _, err = sess.Delete(collaboration); err != nil {
				return fmt.Errorf("remove collaborator '%d': %v", c.ID, err)
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
			if _, err := sess.ID(t.ID).AllCols().Update(t); err != nil {
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
	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos+1 WHERE id=?", newOwner.ID); err != nil {
		return fmt.Errorf("increase new owner repository count: %v", err)
	} else if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", owner.ID); err != nil {
		return fmt.Errorf("decrease old owner repository count: %v", err)
	}

	if err = watchRepo(sess, newOwner.ID, repo.ID, true); err != nil {
		return fmt.Errorf("watchRepo: %v", err)
	} else if err = transferRepoAction(sess, doer, owner, repo); err != nil {
		return fmt.Errorf("transferRepoAction: %v", err)
	}

	// Rename remote repository to new path and delete local copy.
	if err = os.MkdirAll(UserPath(newOwner.Name), os.ModePerm); err != nil {
		return err
	}
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
		return fmt.Errorf("rename repository directory: %v", err)
	}

	deleteRepoLocalCopy(repo)

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

func deleteRepoLocalCopy(repo *Repository) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))
	RemoveAllWithNotice("Delete repository local copy", repo.LocalCopyPath())
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(u *User, oldRepoName, newRepoName string) (err error) {
	oldRepoName = strings.ToLower(oldRepoName)
	newRepoName = strings.ToLower(newRepoName)
	if err = isRepoNameAllowed(newRepoName); err != nil {
		return err
	}

	has, err := IsRepositoryExist(u, newRepoName)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerID": u.ID, "name": newRepoName}}
	}

	repo, err := GetRepositoryByName(u.ID, oldRepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName: %v", err)
	}

	// Change repository directory name
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

	deleteRepoLocalCopy(repo)
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

func getNonMirrorRepositories(e Engine) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, e.Where("is_mirror = ?", false).Find(&repos)
}

// GetRepositoriesMirror returns only mirror repositories with user.
func GetNonMirrorRepositories() ([]*Repository, error) {
	return getNonMirrorRepositories(x)
}

func updateRepository(e Engine, repo *Repository, visibilityChanged bool) (err error) {
	repo.LowerName = strings.ToLower(repo.Name)

	if len(repo.Description) > 512 {
		repo.Description = repo.Description[:512]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	if _, err = e.ID(repo.ID).AllCols().Update(repo); err != nil {
		return fmt.Errorf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(e); err != nil {
			return fmt.Errorf("getOwner: %v", err)
		}
		if repo.Owner.IsOrganization() {
			// Organization repository need to recalculate access table when visivility is changed
			if err = repo.recalculateTeamAccesses(e, 0); err != nil {
				return fmt.Errorf("recalculateTeamAccesses: %v", err)
			}
		}

		// Create/Remove git-daemon-export-ok for git-daemon
		daemonExportFile := path.Join(repo.RepoPath(), "git-daemon-export-ok")
		if repo.IsPrivate && com.IsExist(daemonExportFile) {
			if err = os.Remove(daemonExportFile); err != nil {
				log.Error("Failed to remove %s: %v", daemonExportFile, err)
			}
		} else if !repo.IsPrivate && !com.IsExist(daemonExportFile) {
			if f, err := os.Create(daemonExportFile); err != nil {
				log.Error("Failed to create %s: %v", daemonExportFile, err)
			} else {
				f.Close()
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

		// Change visibility of generated actions
		if _, err = e.Where("repo_id = ?", repo.ID).Cols("is_private").Update(&Action{IsPrivate: repo.IsPrivate}); err != nil {
			return fmt.Errorf("change action visibility of repository: %v", err)
		}
	}

	return nil
}

func UpdateRepository(repo *Repository, visibilityChanged bool) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = updateRepository(x, repo, visibilityChanged); err != nil {
		return fmt.Errorf("updateRepository: %v", err)
	}

	return sess.Commit()
}

// DeleteRepository deletes a repository for a user or organization.
func DeleteRepository(ownerID, repoID int64) error {
	repo := &Repository{ID: repoID, OwnerID: ownerID}
	has, err := x.Get(repo)
	if err != nil {
		return err
	} else if !has {
		return ErrRepoNotExist{args: map[string]interface{}{"ownerID": ownerID, "repoID": repoID}}
	}

	// In case is a organization.
	org, err := GetUserByID(ownerID)
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
		&ProtectBranch{RepoID: repoID},
		&ProtectBranchWhitelist{RepoID: repoID},
		&Webhook{RepoID: repoID},
		&HookTask{RepoID: repoID},
		&LFSObject{RepoID: repoID},
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

	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", ownerID); err != nil {
		return err
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	// Remove repository files.
	repoPath := repo.RepoPath()
	RemoveAllWithNotice("Delete repository files", repoPath)

	repo.DeleteWiki()

	// Remove attachment files.
	for i := range attachmentPaths {
		RemoveAllWithNotice("Delete attachment", attachmentPaths[i])
	}

	if repo.NumForks > 0 {
		if _, err = x.Exec("UPDATE `repository` SET fork_id=0,is_fork=? WHERE fork_id=?", false, repo.ID); err != nil {
			log.Error("reset 'fork_id' and 'is_fork': %v", err)
		}
	}

	return nil
}

// GetRepositoryByRef returns a Repository specified by a GFM reference.
// See https://help.github.com/articles/writing-on-github#references for more information on the syntax.
func GetRepositoryByRef(ref string) (*Repository, error) {
	n := strings.IndexByte(ref, byte('/'))
	if n < 2 {
		return nil, errors.InvalidRepoReference{Ref: ref}
	}

	userName, repoName := ref[:n], ref[n+1:]
	user, err := GetUserByName(userName)
	if err != nil {
		return nil, err
	}

	return GetRepositoryByName(user.ID, repoName)
}

// GetRepositoryByName returns the repository by given name under user if exists.
// Deprecated: Use Repos.GetByName instead.
func GetRepositoryByName(ownerID int64, name string) (*Repository, error) {
	repo := &Repository{
		OwnerID:   ownerID,
		LowerName: strings.ToLower(name),
	}
	has, err := x.Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist{args: map[string]interface{}{"ownerID": ownerID, "name": name}}
	}
	return repo, repo.LoadAttributes()
}

func getRepositoryByID(e Engine, id int64) (*Repository, error) {
	repo := new(Repository)
	has, err := e.ID(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist{args: map[string]interface{}{"repoID": id}}
	}
	return repo, repo.loadAttributes(e)
}

// GetRepositoryByID returns the repository by given id if exists.
func GetRepositoryByID(id int64) (*Repository, error) {
	return getRepositoryByID(x, id)
}

type UserRepoOptions struct {
	UserID   int64
	Private  bool
	Page     int
	PageSize int
}

// GetUserRepositories returns a list of repositories of given user.
func GetUserRepositories(opts *UserRepoOptions) ([]*Repository, error) {
	sess := x.Where("owner_id=?", opts.UserID).Desc("updated_unix")
	if !opts.Private {
		sess.And("is_private=?", false)
	}

	if opts.Page <= 0 {
		opts.Page = 1
	}
	sess.Limit(opts.PageSize, (opts.Page-1)*opts.PageSize)

	repos := make([]*Repository, 0, opts.PageSize)
	return repos, sess.Find(&repos)
}

// GetUserRepositories returns a list of mirror repositories of given user.
func GetUserMirrorRepositories(userID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, x.Where("owner_id = ?", userID).And("is_mirror = ?", true).Find(&repos)
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories(page, pageSize int) (repos []*Repository, err error) {
	return repos, x.Limit(pageSize, (page-1)*pageSize).
		Where("is_private=?", false).Limit(pageSize).Desc("updated_unix").Find(&repos)
}

// GetUserAndCollaborativeRepositories returns list of repositories the user owns and collaborates.
func GetUserAndCollaborativeRepositories(userID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	if err := x.Alias("repo").
		Join("INNER", "collaboration", "collaboration.repo_id = repo.id").
		Where("collaboration.user_id = ?", userID).
		Find(&repos); err != nil {
		return nil, fmt.Errorf("select collaborative repositories: %v", err)
	}

	ownRepos := make([]*Repository, 0, 10)
	if err := x.Where("owner_id = ?", userID).Find(&ownRepos); err != nil {
		return nil, fmt.Errorf("select own repositories: %v", err)
	}

	return append(repos, ownRepos...), nil
}

func getRepositoryCount(e Engine, u *User) (int64, error) {
	return x.Count(&Repository{OwnerID: u.ID})
}

// GetRepositoryCount returns the total number of repositories of user.
func GetRepositoryCount(u *User) (int64, error) {
	return getRepositoryCount(x, u)
}

type SearchRepoOptions struct {
	Keyword  string
	OwnerID  int64
	UserID   int64 // When set results will contain all public/private repositories user has access to
	OrderBy  string
	Private  bool // Include private repositories in results
	Page     int
	PageSize int // Can be smaller than or equal to setting.ExplorePagingNum
}

// SearchRepositoryByName takes keyword and part of repository name to search,
// it returns results in given range and number of total results.
func SearchRepositoryByName(opts *SearchRepoOptions) (repos []*Repository, count int64, err error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}

	repos = make([]*Repository, 0, opts.PageSize)
	sess := x.Alias("repo")
	// Attempt to find repositories that opts.UserID has access to,
	// this does not include other people's private repositories even if opts.UserID is an admin.
	if !opts.Private && opts.UserID > 0 {
		sess.Join("LEFT", "access", "access.repo_id = repo.id").
			Where("repo.owner_id = ? OR access.user_id = ? OR repo.is_private = ? OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?))", opts.UserID, opts.UserID, false, true, true, true)
	} else {
		// Only return public repositories if opts.Private is not set
		if !opts.Private {
			sess.And("repo.is_private = ? OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?))", false, true, true, true)
		}
	}
	if len(opts.Keyword) > 0 {
		sess.And("repo.lower_name LIKE ? OR repo.description LIKE ?", "%"+strings.ToLower(opts.Keyword)+"%", "%"+strings.ToLower(opts.Keyword)+"%")
	}
	if opts.OwnerID > 0 {
		sess.And("repo.owner_id = ?", opts.OwnerID)
	}

	// We need all fields (repo.*) in final list but only ID (repo.id) is good enough for counting.
	count, err = sess.Clone().Distinct("repo.id").Count(new(Repository))
	if err != nil {
		return nil, 0, fmt.Errorf("Count: %v", err)
	}

	if len(opts.OrderBy) > 0 {
		sess.OrderBy("repo." + opts.OrderBy)
	}
	return repos, count, sess.Distinct("repo.*").Limit(opts.PageSize, (opts.Page-1)*opts.PageSize).Find(&repos)
}

func DeleteOldRepositoryArchives() {
	if taskStatusTable.IsRunning(_CLEAN_OLD_ARCHIVES) {
		return
	}
	taskStatusTable.Start(_CLEAN_OLD_ARCHIVES)
	defer taskStatusTable.Stop(_CLEAN_OLD_ARCHIVES)

	log.Trace("Doing: DeleteOldRepositoryArchives")

	formats := []string{"zip", "targz"}
	oldestTime := time.Now().Add(-conf.Cron.RepoArchiveCleanup.OlderThan)
	if err := x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			basePath := filepath.Join(repo.RepoPath(), "archives")
			for _, format := range formats {
				dirPath := filepath.Join(basePath, format)
				if !com.IsDir(dirPath) {
					continue
				}

				dir, err := os.Open(dirPath)
				if err != nil {
					log.Error("Failed to open directory '%s': %v", dirPath, err)
					continue
				}

				fis, err := dir.Readdir(0)
				dir.Close()
				if err != nil {
					log.Error("Failed to read directory '%s': %v", dirPath, err)
					continue
				}

				for _, fi := range fis {
					if fi.IsDir() || fi.ModTime().After(oldestTime) {
						continue
					}

					archivePath := filepath.Join(dirPath, fi.Name())
					if err = os.Remove(archivePath); err != nil {
						desc := fmt.Sprintf("Failed to health delete archive '%s': %v", archivePath, err)
						log.Warn(desc)
						if err = CreateRepositoryNotice(desc); err != nil {
							log.Error("CreateRepositoryNotice: %v", err)
						}
					}
				}
			}

			return nil
		}); err != nil {
		log.Error("DeleteOldRepositoryArchives: %v", err)
	}
}

// DeleteRepositoryArchives deletes all repositories' archives.
func DeleteRepositoryArchives() error {
	if taskStatusTable.IsRunning(_CLEAN_OLD_ARCHIVES) {
		return nil
	}
	taskStatusTable.Start(_CLEAN_OLD_ARCHIVES)
	defer taskStatusTable.Stop(_CLEAN_OLD_ARCHIVES)

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
		if err := git.Init(repo.RepoPath(), git.InitOptions{Bare: true}); err != nil {
			if err2 := CreateRepositoryNotice(fmt.Sprintf("init repository [repo_id: %d]: %v", repo.ID, err)); err2 != nil {
				return fmt.Errorf("create repository notice: %v", err)
			}
		}
	}
	return nil
}

// SyncRepositoryHooks rewrites all repositories' pre-receive, update and post-receive hooks
// to make sure the binary and custom conf path are up-to-date.
func SyncRepositoryHooks() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := createDelegateHooks(repo.RepoPath()); err != nil {
				return err
			}

			if repo.HasWiki() {
				return createDelegateHooks(repo.WikiPath())
			}
			return nil
		})
}

// Prevent duplicate running tasks.
var taskStatusTable = sync.NewStatusTable()

const (
	_MIRROR_UPDATE      = "mirror_update"
	_GIT_FSCK           = "git_fsck"
	_CHECK_REPO_STATS   = "check_repos_stats"
	_CLEAN_OLD_ARCHIVES = "clean_old_archives"
)

// GitFsck calls 'git fsck' to check repository health.
func GitFsck() {
	if taskStatusTable.IsRunning(_GIT_FSCK) {
		return
	}
	taskStatusTable.Start(_GIT_FSCK)
	defer taskStatusTable.Stop(_GIT_FSCK)

	log.Trace("Doing: GitFsck")

	if err := x.Where("id>0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			repoPath := repo.RepoPath()
			err := git.RepoFsck(repoPath, git.FsckOptions{
				Args:    conf.Cron.RepoHealthCheck.Args,
				Timeout: conf.Cron.RepoHealthCheck.Timeout,
			})
			if err != nil {
				desc := fmt.Sprintf("Failed to perform health check on repository '%s': %v", repoPath, err)
				log.Warn(desc)
				if err = CreateRepositoryNotice(desc); err != nil {
					log.Error("CreateRepositoryNotice: %v", err)
				}
			}
			return nil
		}); err != nil {
		log.Error("GitFsck: %v", err)
	}
}

func GitGcRepos() error {
	args := append([]string{"gc"}, conf.Git.GCArgs...)
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}
			_, stderr, err := process.ExecDir(
				time.Duration(conf.Git.Timeout.GC)*time.Second,
				RepoPath(repo.Owner.Name, repo.Name), "Repository garbage collection",
				"git", args...)
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
		log.Error("Select %s: %v", checker.desc, err)
		return
	}
	for _, result := range results {
		id := com.StrTo(result["id"]).MustInt64()
		log.Trace("Updating %s: %d", checker.desc, id)
		_, err = x.Exec(checker.correctSQL, id, id)
		if err != nil {
			log.Error("Update %s[%d]: %v", checker.desc, id, err)
		}
	}
}

func CheckRepoStats() {
	if taskStatusTable.IsRunning(_CHECK_REPO_STATS) {
		return
	}
	taskStatusTable.Start(_CHECK_REPO_STATS)
	defer taskStatusTable.Stop(_CHECK_REPO_STATS)

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

	// ***** START: Repository.NumClosedIssues *****
	desc := "repository count 'num_closed_issues'"
	results, err := x.Query("SELECT repo.id FROM `repository` repo WHERE repo.num_closed_issues!=(SELECT COUNT(*) FROM `issue` WHERE repo_id=repo.id AND is_closed=? AND is_pull=?)", true, false)
	if err != nil {
		log.Error("Select %s: %v", desc, err)
	} else {
		for _, result := range results {
			id := com.StrTo(result["id"]).MustInt64()
			log.Trace("Updating %s: %d", desc, id)
			_, err = x.Exec("UPDATE `repository` SET num_closed_issues=(SELECT COUNT(*) FROM `issue` WHERE repo_id=? AND is_closed=? AND is_pull=?) WHERE id=?", id, true, false, id)
			if err != nil {
				log.Error("Update %s[%d]: %v", desc, id, err)
			}
		}
	}
	// ***** END: Repository.NumClosedIssues *****

	// FIXME: use checker when stop supporting old fork repo format.
	// ***** START: Repository.NumForks *****
	results, err = x.Query("SELECT repo.id FROM `repository` repo WHERE repo.num_forks!=(SELECT COUNT(*) FROM `repository` WHERE fork_id=repo.id)")
	if err != nil {
		log.Error("Select repository count 'num_forks': %v", err)
	} else {
		for _, result := range results {
			id := com.StrTo(result["id"]).MustInt64()
			log.Trace("Updating repository count 'num_forks': %d", id)

			repo, err := GetRepositoryByID(id)
			if err != nil {
				log.Error("GetRepositoryByID[%d]: %v", id, err)
				continue
			}

			rawResult, err := x.Query("SELECT COUNT(*) FROM `repository` WHERE fork_id=?", repo.ID)
			if err != nil {
				log.Error("Select count of forks[%d]: %v", repo.ID, err)
				continue
			}
			repo.NumForks = int(parseCountResult(rawResult))

			if err = UpdateRepository(repo, false); err != nil {
				log.Error("UpdateRepository[%d]: %v", id, err)
				continue
			}
		}
	}
	// ***** END: Repository.NumForks *****
}

type RepositoryList []*Repository

func (repos RepositoryList) loadAttributes(e Engine) error {
	if len(repos) == 0 {
		return nil
	}

	// Load owners
	userSet := make(map[int64]*User)
	for i := range repos {
		userSet[repos[i].OwnerID] = nil
	}
	userIDs := make([]int64, 0, len(userSet))
	for userID := range userSet {
		userIDs = append(userIDs, userID)
	}
	users := make([]*User, 0, len(userIDs))
	if err := e.Where("id > 0").In("id", userIDs).Find(&users); err != nil {
		return fmt.Errorf("find users: %v", err)
	}
	for i := range users {
		userSet[users[i].ID] = users[i]
	}
	for i := range repos {
		repos[i].Owner = userSet[repos[i].OwnerID]
	}

	// Load base repositories
	repoSet := make(map[int64]*Repository)
	for i := range repos {
		if repos[i].IsFork {
			repoSet[repos[i].ForkID] = nil
		}
	}
	baseIDs := make([]int64, 0, len(repoSet))
	for baseID := range repoSet {
		baseIDs = append(baseIDs, baseID)
	}
	baseRepos := make([]*Repository, 0, len(baseIDs))
	if err := e.Where("id > 0").In("id", baseIDs).Find(&baseRepos); err != nil {
		return fmt.Errorf("find base repositories: %v", err)
	}
	for i := range baseRepos {
		repoSet[baseRepos[i].ID] = baseRepos[i]
	}
	for i := range repos {
		if repos[i].IsFork {
			repos[i].BaseRepo = repoSet[repos[i].ForkID]
		}
	}

	return nil
}

func (repos RepositoryList) LoadAttributes() error {
	return repos.loadAttributes(x)
}

type MirrorRepositoryList []*Repository

func (repos MirrorRepositoryList) loadAttributes(e Engine) error {
	if len(repos) == 0 {
		return nil
	}

	// Load mirrors.
	repoIDs := make([]int64, 0, len(repos))
	for i := range repos {
		if !repos[i].IsMirror {
			continue
		}

		repoIDs = append(repoIDs, repos[i].ID)
	}
	mirrors := make([]*Mirror, 0, len(repoIDs))
	if err := e.Where("id > 0").In("repo_id", repoIDs).Find(&mirrors); err != nil {
		return fmt.Errorf("find mirrors: %v", err)
	}

	set := make(map[int64]*Mirror)
	for i := range mirrors {
		set[mirrors[i].RepoID] = mirrors[i]
	}
	for i := range repos {
		repos[i].Mirror = set[repos[i].ID]
	}
	return nil
}

func (repos MirrorRepositoryList) LoadAttributes() error {
	return repos.loadAttributes(x)
}

//  __      __         __         .__
// /  \    /  \_____ _/  |_  ____ |  |__
// \   \/\/   /\__  \\   __\/ ___\|  |  \
//  \        /  / __ \|  | \  \___|   Y  \
//   \__/\  /  (____  /__|  \___  >___|  /
//        \/        \/          \/     \/

// Watch is connection request for receiving repository notification.
type Watch struct {
	ID     int64
	UserID int64 `xorm:"UNIQUE(watch)"`
	RepoID int64 `xorm:"UNIQUE(watch)"`
}

func isWatching(e Engine, userID, repoID int64) bool {
	has, _ := e.Get(&Watch{0, userID, repoID})
	return has
}

// IsWatching checks if user has watched given repository.
func IsWatching(userID, repoID int64) bool {
	return isWatching(x, userID, repoID)
}

func watchRepo(e Engine, userID, repoID int64, watch bool) (err error) {
	if watch {
		if isWatching(e, userID, repoID) {
			return nil
		}
		if _, err = e.Insert(&Watch{RepoID: repoID, UserID: userID}); err != nil {
			return err
		}
		_, err = e.Exec("UPDATE `repository` SET num_watches = num_watches + 1 WHERE id = ?", repoID)
	} else {
		if !isWatching(e, userID, repoID) {
			return nil
		}
		if _, err = e.Delete(&Watch{0, userID, repoID}); err != nil {
			return err
		}
		_, err = e.Exec("UPDATE `repository` SET num_watches = num_watches - 1 WHERE id = ?", repoID)
	}
	return err
}

// Watch or unwatch repository.
func WatchRepo(userID, repoID int64, watch bool) (err error) {
	return watchRepo(x, userID, repoID, watch)
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
	if conf.UsePostgreSQL {
		sess = sess.Join("LEFT", "watch", `"user".id=watch.user_id`)
	} else {
		sess = sess.Join("LEFT", "watch", "user.id=watch.user_id")
	}
	return users, sess.Find(&users)
}

func notifyWatchers(e Engine, act *Action) error {
	// Add feeds for user self and all watchers.
	watchers, err := getWatchers(e, act.RepoID)
	if err != nil {
		return fmt.Errorf("getWatchers: %v", err)
	}

	// Reset ID to reuse Action object
	act.ID = 0

	// Add feed for actioner.
	act.UserID = act.ActUserID
	if _, err = e.Insert(act); err != nil {
		return fmt.Errorf("insert new action: %v", err)
	}

	for i := range watchers {
		if act.ActUserID == watchers[i].UserID {
			continue
		}

		act.ID = 0
		act.UserID = watchers[i].UserID
		if _, err = e.Insert(act); err != nil {
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
	ID     int64
	UID    int64 `xorm:"UNIQUE(s)"`
	RepoID int64 `xorm:"UNIQUE(s)"`
}

// Star or unstar repository.
func StarRepo(userID, repoID int64, star bool) (err error) {
	if star {
		if IsStaring(userID, repoID) {
			return nil
		}
		if _, err = x.Insert(&Star{UID: userID, RepoID: repoID}); err != nil {
			return err
		} else if _, err = x.Exec("UPDATE `repository` SET num_stars = num_stars + 1 WHERE id = ?", repoID); err != nil {
			return err
		}
		_, err = x.Exec("UPDATE `user` SET num_stars = num_stars + 1 WHERE id = ?", userID)
	} else {
		if !IsStaring(userID, repoID) {
			return nil
		}
		if _, err = x.Delete(&Star{0, userID, repoID}); err != nil {
			return err
		} else if _, err = x.Exec("UPDATE `repository` SET num_stars = num_stars - 1 WHERE id = ?", repoID); err != nil {
			return err
		}
		_, err = x.Exec("UPDATE `user` SET num_stars = num_stars - 1 WHERE id = ?", userID)
	}
	return err
}

// IsStaring checks if user has starred given repository.
func IsStaring(userID, repoID int64) bool {
	has, _ := x.Get(&Star{0, userID, repoID})
	return has
}

func (repo *Repository) GetStargazers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("star.repo_id=?", repo.ID)
	if conf.UsePostgreSQL {
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

// HasForkedRepo checks if given user has already forked a repository.
// When user has already forked, it returns true along with the repository.
func HasForkedRepo(ownerID, repoID int64) (*Repository, bool, error) {
	repo := new(Repository)
	has, err := x.Where("owner_id = ? AND fork_id = ?", ownerID, repoID).Get(repo)
	if err != nil {
		return nil, false, err
	} else if !has {
		return nil, false, nil
	}
	return repo, true, repo.LoadAttributes()
}

// ForkRepository creates a fork of target repository under another user domain.
func ForkRepository(doer, owner *User, baseRepo *Repository, name, desc string) (_ *Repository, err error) {
	if !owner.CanCreateRepo() {
		return nil, ErrReachLimitOfRepo{Limit: owner.RepoCreationNum()}
	}

	repo := &Repository{
		OwnerID:       owner.ID,
		Owner:         owner,
		Name:          name,
		LowerName:     strings.ToLower(name),
		Description:   desc,
		DefaultBranch: baseRepo.DefaultBranch,
		IsPrivate:     baseRepo.IsPrivate,
		IsFork:        true,
		ForkID:        baseRepo.ID,
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	if err = createRepository(sess, doer, owner, repo); err != nil {
		return nil, err
	} else if _, err = sess.Exec("UPDATE `repository` SET num_forks=num_forks+1 WHERE id=?", baseRepo.ID); err != nil {
		return nil, err
	}

	repoPath := repo.repoPath(sess)
	RemoveAllWithNotice("Repository path erase before creation", repoPath)

	_, stderr, err := process.ExecTimeout(10*time.Minute,
		fmt.Sprintf("ForkRepository 'git clone': %s/%s", owner.Name, repo.Name),
		"git", "clone", "--bare", baseRepo.RepoPath(), repoPath)
	if err != nil {
		return nil, fmt.Errorf("git clone: %v - %s", err, stderr)
	}

	_, stderr, err = process.ExecDir(-1,
		repoPath, fmt.Sprintf("ForkRepository 'git update-server-info': %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, fmt.Errorf("git update-server-info: %v - %s", err, stderr)
	}

	if err = createDelegateHooks(repoPath); err != nil {
		return nil, fmt.Errorf("createDelegateHooks: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return nil, fmt.Errorf("Commit: %v", err)
	}

	if err = repo.UpdateSize(); err != nil {
		log.Error("UpdateSize [repo_id: %d]: %v", repo.ID, err)
	}
	if err = PrepareWebhooks(baseRepo, HOOK_EVENT_FORK, &api.ForkPayload{
		Forkee: repo.APIFormat(nil),
		Repo:   baseRepo.APIFormat(nil),
		Sender: doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks [repo_id: %d]: %v", baseRepo.ID, err)
	}
	return repo, nil
}

func (repo *Repository) GetForks() ([]*Repository, error) {
	forks := make([]*Repository, 0, repo.NumForks)
	if err := x.Find(&forks, &Repository{ForkID: repo.ID}); err != nil {
		return nil, err
	}

	for _, fork := range forks {
		fork.BaseRepo = repo
	}
	return forks, nil
}

// __________                             .__
// \______   \____________    ____   ____ |  |__
//  |    |  _/\_  __ \__  \  /    \_/ ___\|  |  \
//  |    |   \ |  | \// __ \|   |  \  \___|   Y  \
//  |______  / |__|  (____  /___|  /\___  >___|  /
//         \/             \/     \/     \/     \/
//

func (repo *Repository) CreateNewBranch(oldBranch, newBranch string) (err error) {
	repoWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer repoWorkingPool.CheckOut(com.ToStr(repo.ID))

	localPath := repo.LocalCopyPath()

	if err = discardLocalRepoBranchChanges(localPath, oldBranch); err != nil {
		return fmt.Errorf("discard changes in local copy [path: %s, branch: %s]: %v", localPath, oldBranch, err)
	} else if err = repo.UpdateLocalCopyBranch(oldBranch); err != nil {
		return fmt.Errorf("update branch for local copy [path: %s, branch: %s]: %v", localPath, oldBranch, err)
	}

	if err = repo.CheckoutNewBranch(oldBranch, newBranch); err != nil {
		return fmt.Errorf("create new branch [base: %s, new: %s]: %v", oldBranch, newBranch, err)
	}

	if err = git.RepoPush(localPath, "origin", newBranch); err != nil {
		return fmt.Errorf("push [branch: %s]: %v", newBranch, err)
	}

	return nil
}
