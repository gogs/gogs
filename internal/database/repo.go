package database

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/cae/zip"
	"github.com/unknwon/com"
	"golang.org/x/image/draw"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"

	embedConf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/repoutil"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/semverutil"
	"gogs.io/gogs/internal/strutil"
	"gogs.io/gogs/internal/sync"
)

// RepoAvatarURLPrefix is used to identify a URL is to access repository avatar.
const RepoAvatarURLPrefix = "repo-avatars"

// InvalidRepoReference represents an error when repository reference is invalid.
type InvalidRepoReference struct {
	Ref string
}

// IsInvalidRepoReference returns true if the error is InvalidRepoReference.
func IsInvalidRepoReference(err error) bool {
	_, ok := err.(InvalidRepoReference)
	return ok
}

func (err InvalidRepoReference) Error() string {
	return fmt.Sprintf("invalid repository reference [ref: %s]", err.Ref)
}

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
		files, err := embedConf.FileNames(t)
		if err != nil {
			log.Fatal("Failed to get %q files: %v", t, err)
		}

		customPath := filepath.Join(conf.CustomDir(), "conf", t)
		if osutil.IsDir(customPath) {
			entries, err := os.ReadDir(customPath)
			if err != nil {
				log.Fatal("Failed to get custom %s files: %v", t, err)
			}

			for _, entry := range entries {
				f := entry.Name()
				if !strutil.ContainsFold(files, f) {
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
		if slices.Contains(Licenses, name) {
			sortedLicenses = append(sortedLicenses, name)
		}
	}
	for _, name := range Licenses {
		if !slices.Contains(conf.Repository.PreferredLicenses, name) {
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
	RemoveAllWithNotice("Clean up LFS temporary data", conf.LFS.ObjectsTempPath)
}

// Repository contains information of a repository.
type Repository struct {
	ID              int64  `gorm:"primaryKey"`
	OwnerID         int64  `xorm:"UNIQUE(s)" gorm:"uniqueIndex:repo_owner_name_unique"`
	Owner           *User  `xorm:"-" gorm:"-" json:"-"`
	LowerName       string `xorm:"UNIQUE(s) INDEX NOT NULL" gorm:"uniqueIndex:repo_owner_name_unique;index;not null"`
	Name            string `xorm:"INDEX NOT NULL" gorm:"index;not null"`
	Description     string `xorm:"VARCHAR(512)" gorm:"type:VARCHAR(512)"`
	Website         string
	DefaultBranch   string
	Size            int64 `xorm:"NOT NULL DEFAULT 0" gorm:"not null;default:0"`
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
	NumMilestones       int `xorm:"NOT NULL DEFAULT 0" gorm:"not null;default:0"`
	NumClosedMilestones int `xorm:"NOT NULL DEFAULT 0" gorm:"not null;default:0"`
	NumOpenMilestones   int `xorm:"-" gorm:"-" json:"-"`
	NumTags             int `xorm:"-" gorm:"-" json:"-"`

	IsPrivate bool
	// TODO: When migrate to GORM, make sure to do a loose migration with `HasColumn` and `AddColumn`,
	// see docs in https://gorm.io/docs/migration.html.
	IsUnlisted bool `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	IsBare     bool

	IsMirror bool
	*Mirror  `xorm:"-" gorm:"-" json:"-"`

	// Advanced settings
	EnableWiki            bool `xorm:"NOT NULL DEFAULT true" gorm:"not null;default:TRUE"`
	AllowPublicWiki       bool
	EnableExternalWiki    bool
	ExternalWikiURL       string
	EnableIssues          bool `xorm:"NOT NULL DEFAULT true" gorm:"not null;default:TRUE"`
	AllowPublicIssues     bool
	EnableExternalTracker bool
	ExternalTrackerURL    string
	ExternalTrackerFormat string
	ExternalTrackerStyle  string
	ExternalMetas         map[string]string `xorm:"-" gorm:"-" json:"-"`
	EnablePulls           bool              `xorm:"NOT NULL DEFAULT true" gorm:"not null;default:TRUE"`
	PullsIgnoreWhitespace bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	PullsAllowRebase      bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	ForkID   int64
	BaseRepo *Repository `xorm:"-" gorm:"-" json:"-"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64
}

func (r *Repository) BeforeInsert() {
	r.CreatedUnix = time.Now().Unix()
	r.UpdatedUnix = r.CreatedUnix
}

func (r *Repository) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "default_branch":
		// FIXME: use db migration to solve all at once.
		if r.DefaultBranch == "" {
			r.DefaultBranch = conf.Repository.DefaultBranch
		}
	case "num_closed_issues":
		r.NumOpenIssues = r.NumIssues - r.NumClosedIssues
	case "num_closed_pulls":
		r.NumOpenPulls = r.NumPulls - r.NumClosedPulls
	case "num_closed_milestones":
		r.NumOpenMilestones = r.NumMilestones - r.NumClosedMilestones
	case "external_tracker_style":
		if r.ExternalTrackerStyle == "" {
			r.ExternalTrackerStyle = markup.IssueNameStyleNumeric
		}
	case "created_unix":
		r.Created = time.Unix(r.CreatedUnix, 0).Local()
	case "updated_unix":
		r.Updated = time.Unix(r.UpdatedUnix, 0)
	}
}

func (r *Repository) loadAttributes(e Engine) (err error) {
	if r.Owner == nil {
		r.Owner, err = getUserByID(e, r.OwnerID)
		if err != nil {
			return errors.Newf("getUserByID [%d]: %v", r.OwnerID, err)
		}
	}

	if r.IsFork && r.BaseRepo == nil {
		r.BaseRepo, err = getRepositoryByID(e, r.ForkID)
		if err != nil {
			if IsErrRepoNotExist(err) {
				r.IsFork = false
				r.ForkID = 0
			} else {
				return errors.Newf("get fork repository by ID: %v", err)
			}
		}
	}

	return nil
}

func (r *Repository) LoadAttributes() error {
	return r.loadAttributes(x)
}

// IsPartialPublic returns true if repository is public or allow public access to wiki or issues.
func (r *Repository) IsPartialPublic() bool {
	return !r.IsPrivate || r.AllowPublicWiki || r.AllowPublicIssues
}

func (r *Repository) CanGuestViewWiki() bool {
	return r.EnableWiki && !r.EnableExternalWiki && r.AllowPublicWiki
}

func (r *Repository) CanGuestViewIssues() bool {
	return r.EnableIssues && !r.EnableExternalTracker && r.AllowPublicIssues
}

// MustOwner always returns a valid *User object to avoid conceptually impossible error handling.
// It creates a fake object that contains error details when error occurs.
func (r *Repository) MustOwner() *User {
	return r.mustOwner(x)
}

func (r *Repository) FullName() string {
	return r.MustOwner().Name + "/" + r.Name
}

// Deprecated: Use repoutil.HTMLURL instead.
func (r *Repository) HTMLURL() string {
	return conf.Server.ExternalURL + r.FullName()
}

// CustomAvatarPath returns repository custom avatar file path.
func (r *Repository) CustomAvatarPath() string {
	return filepath.Join(conf.Picture.RepositoryAvatarUploadPath, strconv.FormatInt(r.ID, 10))
}

// RelAvatarLink returns relative avatar link to the site domain,
// which includes app sub-url as prefix.
// Since Gravatar support not needed here - just check for image path.
func (r *Repository) RelAvatarLink() string {
	defaultImgURL := ""
	if !osutil.Exist(r.CustomAvatarPath()) {
		return defaultImgURL
	}
	return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, RepoAvatarURLPrefix, r.ID)
}

// AvatarLink returns repository avatar absolute link.
func (r *Repository) AvatarLink() string {
	link := r.RelAvatarLink()
	if link[0] == '/' && link[1] != '/' {
		return conf.Server.ExternalURL + strings.TrimPrefix(link, conf.Server.Subpath)[1:]
	}
	return link
}

// UploadAvatar saves custom avatar for repository.
// FIXME: split uploads to different subdirs in case we have massive number of repositories.
func (r *Repository) UploadAvatar(data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.Newf("decode image: %v", err)
	}

	_ = os.MkdirAll(conf.Picture.RepositoryAvatarUploadPath, os.ModePerm)
	fw, err := os.Create(r.CustomAvatarPath())
	if err != nil {
		return errors.Newf("create custom avatar directory: %v", err)
	}
	defer fw.Close()

	dst := image.NewRGBA(image.Rect(0, 0, avatar.DefaultSize, avatar.DefaultSize))
	draw.NearestNeighbor.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	if err = png.Encode(fw, dst); err != nil {
		return errors.Newf("encode image: %v", err)
	}

	return nil
}

// DeleteAvatar deletes the repository custom avatar.
func (r *Repository) DeleteAvatar() error {
	log.Trace("DeleteAvatar [%d]: %s", r.ID, r.CustomAvatarPath())
	if err := os.Remove(r.CustomAvatarPath()); err != nil {
		return err
	}

	r.UseCustomAvatar = false
	return UpdateRepository(r, false)
}

// This method assumes following fields have been assigned with valid values:
// Required - BaseRepo (if fork)
// Arguments that are allowed to be nil: permission
//
// Deprecated: Use APIFormat instead.
func (r *Repository) APIFormatLegacy(permission *apiv1types.RepositoryPermission, user ...*User) *apiv1types.Repository {
	cloneLink := r.CloneLink()
	apiRepo := &apiv1types.Repository{
		ID:            r.ID,
		Owner:         r.Owner.APIFormat(),
		Name:          r.Name,
		FullName:      r.FullName(),
		Description:   r.Description,
		Private:       r.IsPrivate,
		Fork:          r.IsFork,
		Empty:         r.IsBare,
		Mirror:        r.IsMirror,
		Size:          r.Size,
		HTMLURL:       r.HTMLURL(),
		SSHURL:        cloneLink.SSH,
		CloneURL:      cloneLink.HTTPS,
		Website:       r.Website,
		Stars:         r.NumStars,
		Forks:         r.NumForks,
		Watchers:      r.NumWatches,
		OpenIssues:    r.NumOpenIssues,
		DefaultBranch: r.DefaultBranch,
		Created:       r.Created,
		Updated:       r.Updated,
		Permissions:   permission,
		// Reserved for go-gogs-client change
		//		AvatarUrl:     r.AvatarLink(),
	}
	if r.IsFork {
		p := &apiv1types.RepositoryPermission{Pull: true}
		if len(user) != 0 {
			accessMode := Handle.Permissions().AccessMode(
				context.TODO(),
				user[0].ID,
				r.ID,
				AccessModeOptions{
					OwnerID: r.OwnerID,
					Private: r.IsPrivate,
				},
			)
			p.Admin = accessMode >= AccessModeAdmin
			p.Push = accessMode >= AccessModeWrite
		}
		apiRepo.Parent = r.BaseRepo.APIFormatLegacy(p)
	}
	return apiRepo
}

func (r *Repository) getOwner(e Engine) (err error) {
	if r.Owner != nil {
		return nil
	}

	r.Owner, err = getUserByID(e, r.OwnerID)
	return err
}

func (r *Repository) GetOwner() error {
	return r.getOwner(x)
}

func (r *Repository) mustOwner(e Engine) *User {
	if err := r.getOwner(e); err != nil {
		return &User{
			Name:     "error",
			FullName: err.Error(),
		}
	}

	return r.Owner
}

func (r *Repository) UpdateSize() error {
	countObject, err := git.CountObjects(r.RepoPath())
	if err != nil {
		return errors.Newf("count repository objects: %v", err)
	}

	r.Size = countObject.Size + countObject.SizePack
	if _, err = x.Id(r.ID).Cols("size").Update(r); err != nil {
		return errors.Newf("update size: %v", err)
	}
	return nil
}

// ComposeMetas composes a map of metas for rendering SHA1 URL and external issue tracker URL.
func (r *Repository) ComposeMetas() map[string]string {
	if r.ExternalMetas != nil {
		return r.ExternalMetas
	}

	r.ExternalMetas = map[string]string{
		"repoLink": r.Link(),
	}

	if r.EnableExternalTracker {
		r.ExternalMetas["user"] = r.MustOwner().Name
		r.ExternalMetas["repo"] = r.Name
		r.ExternalMetas["format"] = r.ExternalTrackerFormat

		switch r.ExternalTrackerStyle {
		case markup.IssueNameStyleAlphanumeric:
			r.ExternalMetas["style"] = markup.IssueNameStyleAlphanumeric
		default:
			r.ExternalMetas["style"] = markup.IssueNameStyleNumeric
		}
	}

	return r.ExternalMetas
}

// DeleteWiki removes the actual and local copy of repository wiki.
func (r *Repository) DeleteWiki() {
	wikiPaths := []string{r.WikiPath(), r.LocalWikiPath()}
	for _, wikiPath := range wikiPaths {
		RemoveAllWithNotice("Delete repository wiki", wikiPath)
	}
}

// getUsersWithAccesMode returns users that have at least given access mode to the repository.
func (r *Repository) getUsersWithAccesMode(e Engine, mode AccessMode) (_ []*User, err error) {
	if err = r.getOwner(e); err != nil {
		return nil, err
	}

	accesses := make([]*Access, 0, 10)
	if err = e.Where("repo_id = ? AND mode >= ?", r.ID, mode).Find(&accesses); err != nil {
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

		// TODO(unknwon): Rely on AfterFind hook to sanitize user full name.
		for _, u := range users {
			u.FullName = markup.Sanitize(u.FullName)
		}
	}
	if !r.Owner.IsOrganization() {
		users = append(users, r.Owner)
	}

	return users, nil
}

// getAssignees returns a list of users who can be assigned to issues in this repository.
func (r *Repository) getAssignees(e Engine) (_ []*User, err error) {
	return r.getUsersWithAccesMode(e, AccessModeRead)
}

// GetAssignees returns all users that have read access and can be assigned to issues
// of the repository,
func (r *Repository) GetAssignees() (_ []*User, err error) {
	return r.getAssignees(x)
}

// GetAssigneeByID returns the user that has write access of repository by given ID.
func (r *Repository) GetAssigneeByID(userID int64) (*User, error) {
	ctx := context.TODO()
	if !Handle.Permissions().Authorize(
		ctx,
		userID,
		r.ID,
		AccessModeRead,
		AccessModeOptions{
			OwnerID: r.OwnerID,
			Private: r.IsPrivate,
		},
	) {
		return nil, ErrUserNotExist{args: errutil.Args{"userID": userID}}
	}
	return Handle.Users().GetByID(ctx, userID)
}

// GetWriters returns all users that have write access to the repository.
func (r *Repository) GetWriters() (_ []*User, err error) {
	return r.getUsersWithAccesMode(x, AccessModeWrite)
}

// GetMilestoneByID returns the milestone belongs to repository by given ID.
func (r *Repository) GetMilestoneByID(milestoneID int64) (*Milestone, error) {
	return GetMilestoneByRepoID(r.ID, milestoneID)
}

// IssueStats returns number of open and closed repository issues by given filter mode.
func (r *Repository) IssueStats(userID int64, filterMode FilterMode, isPull bool) (int64, int64) {
	return GetRepoIssueStats(r.ID, userID, filterMode, isPull)
}

func (r *Repository) GetMirror() (err error) {
	r.Mirror, err = GetMirrorByRepoID(r.ID)
	return err
}

func (r *Repository) repoPath(e Engine) string {
	return RepoPath(r.mustOwner(e).Name, r.Name)
}

// Deprecated: Use repoutil.RepositoryPath instead.
func (r *Repository) RepoPath() string {
	return r.repoPath(x)
}

func (r *Repository) GitConfigPath() string {
	return filepath.Join(r.RepoPath(), "config")
}

func (r *Repository) RelLink() string {
	return "/" + r.FullName()
}

func (r *Repository) Link() string {
	return conf.Server.Subpath + "/" + r.FullName()
}

// Deprecated: Use repoutil.ComparePath instead.
func (r *Repository) ComposeCompareURL(oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", r.MustOwner().Name, r.Name, oldCommitID, newCommitID)
}

func (r *Repository) HasAccess(userID int64) bool {
	return Handle.Permissions().Authorize(context.TODO(), userID, r.ID, AccessModeRead,
		AccessModeOptions{
			OwnerID: r.OwnerID,
			Private: r.IsPrivate,
		},
	)
}

func (r *Repository) IsOwnedBy(userID int64) bool {
	return r.OwnerID == userID
}

// CanBeForked returns true if repository meets the requirements of being forked.
func (r *Repository) CanBeForked() bool {
	return !r.IsBare
}

// CanEnablePulls returns true if repository meets the requirements of accepting pulls.
func (r *Repository) CanEnablePulls() bool {
	return !r.IsMirror && !r.IsBare
}

// AllowPulls returns true if repository meets the requirements of accepting pulls and has them enabled.
func (r *Repository) AllowsPulls() bool {
	return r.CanEnablePulls() && r.EnablePulls
}

func (r *Repository) IsBranchRequirePullRequest(name string) bool {
	return IsBranchOfRepoRequirePullRequest(r.ID, name)
}

// CanEnableEditor returns true if repository meets the requirements of web editor.
func (r *Repository) CanEnableEditor() bool {
	return !r.IsMirror
}

// FIXME: should have a mutex to prevent producing same index for two issues that are created
// closely enough.
func (r *Repository) NextIssueIndex() int64 {
	return int64(r.NumIssues+r.NumPulls) + 1
}

func (r *Repository) LocalCopyPath() string {
	return filepath.Join(conf.Server.AppDataPath, "tmp", "local-r", strconv.FormatInt(r.ID, 10))
}

// UpdateLocalCopy fetches latest changes of given branch from repoPath to localPath.
// It creates a new clone if local copy does not exist, but does not checks out to a
// specific branch if the local copy belongs to a wiki.
// For existing local copy, it checks out to target branch by default, and safe to
// assume subsequent operations are against target branch when caller has confidence
// about no race condition.
func UpdateLocalCopyBranch(repoPath, localPath, branch string, isWiki bool) (err error) {
	if !osutil.Exist(localPath) {
		// Checkout to a specific branch fails when wiki is an empty repository.
		if isWiki {
			branch = ""
		}
		if err = git.Clone(repoPath, localPath, git.CloneOptions{
			Branch:  branch,
			Timeout: time.Duration(conf.Git.Timeout.Clone) * time.Second,
		}); err != nil {
			return errors.Newf("git clone [branch: %s]: %v", branch, err)
		}
		return nil
	}

	gitRepo, err := git.Open(localPath)
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}

	if err = gitRepo.Fetch(git.FetchOptions{
		Prune: true,
	}); err != nil {
		return errors.Newf("fetch: %v", err)
	}

	if err = gitRepo.Checkout(branch); err != nil {
		return errors.Newf("checkout [branch: %s]: %v", branch, err)
	}

	// Reset to align with remote in case of force push.
	rev := "origin/" + branch
	if err = gitRepo.Reset(rev, git.ResetOptions{
		Hard: true,
	}); err != nil {
		return errors.Newf("reset [revision: %s]: %v", rev, err)
	}
	return nil
}

// UpdateLocalCopyBranch makes sure local copy of repository in given branch is up-to-date.
func (r *Repository) UpdateLocalCopyBranch(branch string) error {
	return UpdateLocalCopyBranch(r.RepoPath(), r.LocalCopyPath(), branch, false)
}

// PatchPath returns corresponding patch file path of repository by given issue ID.
func (r *Repository) PatchPath(index int64) (string, error) {
	if err := r.GetOwner(); err != nil {
		return "", err
	}

	return filepath.Join(RepoPath(r.Owner.Name, r.Name), "pulls", strconv.FormatInt(index, 10)+".patch"), nil
}

// SavePatch saves patch data to corresponding location by given issue ID.
func (r *Repository) SavePatch(index int64, patch []byte) error {
	patchPath, err := r.PatchPath(index)
	if err != nil {
		return errors.Newf("PatchPath: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(patchPath), os.ModePerm); err != nil {
		return err
	}
	if err = os.WriteFile(patchPath, patch, 0o644); err != nil {
		return errors.Newf("WriteFile: %v", err)
	}

	return nil
}

func isRepositoryExist(e Engine, u *User, repoName string) (bool, error) {
	has, err := e.Get(&Repository{
		OwnerID:   u.ID,
		LowerName: strings.ToLower(repoName),
	})
	return has && osutil.IsDir(RepoPath(u.Name, repoName)), err
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(u *User, repoName string) (bool, error) {
	return isRepositoryExist(x, u, repoName)
}

// Deprecated: Use repoutil.NewCloneLink instead.
func (r *Repository) cloneLink(isWiki bool) *repoutil.CloneLink {
	repoName := r.Name
	if isWiki {
		repoName += ".wiki"
	}

	r.Owner = r.MustOwner()
	cl := new(repoutil.CloneLink)
	if conf.SSH.Port != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", conf.App.RunUser, conf.SSH.Domain, conf.SSH.Port, r.Owner.Name, repoName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", conf.App.RunUser, conf.SSH.Domain, r.Owner.Name, repoName)
	}
	cl.HTTPS = repoutil.HTTPSCloneURL(r.Owner.Name, repoName)
	return cl
}

// CloneLink returns clone URLs of repository.
//
// Deprecated: Use repoutil.NewCloneLink instead.
func (r *Repository) CloneLink() (cl *repoutil.CloneLink) {
	return r.cloneLink(false)
}

type MigrateRepoOptions struct {
	Name        string
	Description string
	IsPrivate   bool
	IsUnlisted  bool
	IsMirror    bool
	RemoteAddr  string
}

/*
- GitHub, GitLab, Gogs: *.wiki.git
- BitBucket: *.git/wiki
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
	repo, err := CreateRepository(doer, owner, CreateRepoOptionsLegacy{
		Name:        opts.Name,
		Description: opts.Description,
		IsPrivate:   opts.IsPrivate,
		IsUnlisted:  opts.IsUnlisted,
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
		return repo, errors.Newf("clone: %v", err)
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
	cmd := exec.Command("git", "log", "-1")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "fatal: bad default revision 'HEAD'") {
			repo.IsBare = true
		} else {
			return repo, errors.Newf("check bare: %v - %s", err, output)
		}
	}

	if !repo.IsBare {
		// Try to get HEAD branch and set it as default branch.
		gitRepo, err := git.Open(repoPath)
		if err != nil {
			return repo, errors.Newf("open repository: %v", err)
		}
		refspec, err := gitRepo.SymbolicRef()
		if err != nil {
			return repo, errors.Newf("get HEAD branch: %v", err)
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
			return repo, errors.Newf("InsertOne: %v", err)
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
		return errors.Newf("open config file: %v", err)
	}
	cfg.DeleteSection("remote \"origin\"")
	if err = cfg.SaveToIndent(configPath, "\t"); err != nil {
		return errors.Newf("save config file: %v", err)
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
		if err = os.WriteFile(hookPath,
			fmt.Appendf(nil, hooksTpls[name], conf.Repository.ScriptType, conf.AppPath(), conf.CustomConf),
			os.ModePerm); err != nil {
			return errors.Newf("create delegate hook '%s': %v", hookPath, err)
		}
	}
	return nil
}

// Finish migrating repository and/or wiki with things that don't need to be done for mirrors.
func CleanUpMigrateInfo(repo *Repository) (*Repository, error) {
	repoPath := repo.RepoPath()
	if err := createDelegateHooks(repoPath); err != nil {
		return repo, errors.Newf("createDelegateHooks: %v", err)
	}
	if repo.HasWiki() {
		if err := createDelegateHooks(repo.WikiPath()); err != nil {
			return repo, errors.Newf("createDelegateHooks.(wiki): %v", err)
		}
	}

	if err := cleanUpMigrateGitConfig(repo.GitConfigPath()); err != nil {
		return repo, errors.Newf("cleanUpMigrateGitConfig: %v", err)
	}
	if repo.HasWiki() {
		if err := cleanUpMigrateGitConfig(path.Join(repo.WikiPath(), "config")); err != nil {
			return repo, errors.Newf("cleanUpMigrateGitConfig.(wiki): %v", err)
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
		return errors.Newf("git add: %s", stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit (git commit): %s", tmpPath),
		"git", "commit", fmt.Sprintf("--author='%s <%s>'", sig.Name, sig.Email),
		"-m", "Initial commit"); err != nil {
		return errors.Newf("git commit: %s", stderr)
	}

	if _, stderr, err = process.ExecDir(-1,
		tmpPath, fmt.Sprintf("initRepoCommit (git push): %s", tmpPath),
		"git", "push"); err != nil {
		return errors.Newf("git push: %s", stderr)
	}
	return nil
}

type CreateRepoOptionsLegacy struct {
	Name        string
	Description string
	Gitignores  string
	License     string
	Readme      string
	IsPrivate   bool
	IsUnlisted  bool
	IsMirror    bool
	AutoInit    bool
}

func getRepoInitFile(tp, name string) ([]byte, error) {
	relPath := path.Join(tp, strings.TrimLeft(path.Clean("/"+name), "/"))

	// Use custom file when available.
	customPath := filepath.Join(conf.CustomDir(), "conf", relPath)
	if osutil.IsFile(customPath) {
		return os.ReadFile(customPath)
	}
	return embedConf.Files.ReadFile(relPath)
}

func prepareRepoCommit(repo *Repository, tmpDir, repoPath string, opts CreateRepoOptionsLegacy) error {
	// Clone to temporary path and do the init commit.
	err := git.Clone(repoPath, tmpDir, git.CloneOptions{})
	if err != nil {
		return errors.Wrap(err, "clone")
	}

	// README
	data, err := getRepoInitFile("readme", opts.Readme)
	if err != nil {
		return errors.Newf("getRepoInitFile[%s]: %v", opts.Readme, err)
	}

	cloneLink := repo.CloneLink()
	match := map[string]string{
		"Name":           repo.Name,
		"Description":    repo.Description,
		"CloneURL.SSH":   cloneLink.SSH,
		"CloneURL.HTTPS": cloneLink.HTTPS,
	}
	if err = os.WriteFile(filepath.Join(tmpDir, "README.md"),
		[]byte(com.Expand(string(data), match)), 0o644); err != nil {
		return errors.Newf("write README.md: %v", err)
	}

	// .gitignore
	if len(opts.Gitignores) > 0 {
		var buf bytes.Buffer
		names := strings.SplitSeq(opts.Gitignores, ",")
		for name := range names {
			data, err = getRepoInitFile("gitignore", name)
			if err != nil {
				return errors.Newf("getRepoInitFile[%s]: %v", name, err)
			}
			buf.WriteString("# ---> " + name + "\n")
			buf.Write(data)
			buf.WriteString("\n")
		}

		if buf.Len() > 0 {
			if err = os.WriteFile(filepath.Join(tmpDir, ".gitignore"), buf.Bytes(), 0o644); err != nil {
				return errors.Newf("write .gitignore: %v", err)
			}
		}
	}

	// LICENSE
	if len(opts.License) > 0 {
		data, err = getRepoInitFile("license", opts.License)
		if err != nil {
			return errors.Newf("getRepoInitFile[%s]: %v", opts.License, err)
		}

		if err = os.WriteFile(filepath.Join(tmpDir, "LICENSE"), data, 0o644); err != nil {
			return errors.Newf("write LICENSE: %v", err)
		}
	}

	return nil
}

// initRepository performs initial commit with chosen setup files on behave of doer.
func initRepository(e Engine, repoPath string, doer *User, repo *Repository, opts CreateRepoOptionsLegacy) (err error) {
	// Init bare new repository.
	if err = git.Init(repoPath, git.InitOptions{Bare: true}); err != nil {
		return errors.Newf("init repository: %v", err)
	} else if err = createDelegateHooks(repoPath); err != nil {
		return errors.Newf("createDelegateHooks: %v", err)
	}

	// Set default branch
	_, err = git.SymbolicRef(
		repoPath,
		git.SymbolicRefOptions{
			Name: "HEAD",
			Ref:  git.RefsHeads + conf.Repository.DefaultBranch,
		},
	)
	if err != nil {
		return errors.Wrap(err, "set default branch")
	}

	tmpDir := filepath.Join(os.TempDir(), "gogs-"+repo.Name+"-"+strconv.Itoa(time.Now().Nanosecond()))

	// Initialize repository according to user's choice.
	if opts.AutoInit {
		if err = os.MkdirAll(tmpDir, os.ModePerm); err != nil {
			return err
		}
		defer RemoveAllWithNotice("Delete repository for auto-initialization", tmpDir)

		if err = prepareRepoCommit(repo, tmpDir, repoPath, opts); err != nil {
			return errors.Newf("prepareRepoCommit: %v", err)
		}

		// Apply changes and commit.
		err = initRepoCommit(
			tmpDir,
			&git.Signature{
				Name:  doer.DisplayName(),
				Email: doer.Email,
				When:  time.Now(),
			},
		)
		if err != nil {
			return errors.Newf("initRepoCommit: %v", err)
		}
	}

	// Re-fetch the repository from database before updating it (else it would
	// override changes that were done earlier with sql)
	if repo, err = getRepositoryByID(e, repo.ID); err != nil {
		return errors.Newf("getRepositoryByID: %v", err)
	}

	if !opts.AutoInit {
		repo.IsBare = true
	}

	repo.DefaultBranch = conf.Repository.DefaultBranch
	if err = updateRepository(e, repo, false); err != nil {
		return errors.Newf("updateRepository: %v", err)
	}

	return nil
}

var (
	reservedRepoNames = map[string]struct{}{
		".":  {},
		"..": {},
	}
	reservedRepoPatterns = []string{
		"*.git",
		"*.wiki",
	}
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
		return errors.Newf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerID": owner.ID, "name": repo.Name}}
	}

	if _, err = e.Insert(repo); err != nil {
		return err
	}

	_, err = e.Exec(dbutil.Quote("UPDATE %s SET num_repos = num_repos + 1 WHERE id = ?", "user"), owner.ID)
	if err != nil {
		return errors.Wrap(err, "increase owned repository count")
	}

	// Give access to all members in owner team.
	if owner.IsOrganization() {
		t, err := owner.getOwnerTeam(e)
		if err != nil {
			return errors.Newf("getOwnerTeam: %v", err)
		} else if err = t.addRepository(e, repo); err != nil {
			return errors.Newf("addRepository: %v", err)
		}
	} else {
		// Organization automatically called this in addRepository method.
		if err = repo.recalculateAccesses(e); err != nil {
			return errors.Newf("recalculateAccesses: %v", err)
		}
	}

	if err = watchRepo(e, owner.ID, repo.ID, true); err != nil {
		return errors.Newf("watchRepo: %v", err)
	}

	// FIXME: This is identical to Actions.NewRepo but we are not yet able to wrap
	// transaction with different ORM objects, should delete this once migrated to
	// GORM for this part of logic.
	newRepoAction := func(e Engine, doer *User, repo *Repository) (err error) {
		opType := ActionCreateRepo
		if repo.IsFork {
			opType = ActionForkRepo
		}

		return notifyWatchers(e, &Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       opType,
			RepoID:       repo.ID,
			RepoUserName: repo.Owner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
			CreatedUnix:  time.Now().Unix(),
		})
	}
	if err = newRepoAction(e, doer, repo); err != nil {
		return errors.Newf("newRepoAction: %v", err)
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
func CreateRepository(doer, owner *User, opts CreateRepoOptionsLegacy) (_ *Repository, err error) {
	repoPath := RepoPath(owner.Name, opts.Name)
	if osutil.Exist(repoPath) {
		return nil, errors.Errorf("repository directory already exists: %s", repoPath)
	}
	if !owner.canCreateRepo() {
		return nil, ErrReachLimitOfRepo{Limit: owner.maxNumRepos()}
	}

	repo := &Repository{
		OwnerID:      owner.ID,
		Owner:        owner,
		Name:         opts.Name,
		LowerName:    strings.ToLower(opts.Name),
		Description:  opts.Description,
		IsPrivate:    opts.IsPrivate,
		IsUnlisted:   opts.IsUnlisted,
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
		if err = initRepository(sess, repoPath, doer, repo, opts); err != nil {
			RemoveAllWithNotice("Delete repository for initialization failure", repoPath)
			return nil, errors.Newf("initRepository: %v", err)
		}

		_, stderr, err := process.ExecDir(-1,
			repoPath, fmt.Sprintf("CreateRepository 'git update-server-info': %s", repoPath),
			"git", "update-server-info")
		if err != nil {
			return nil, errors.Newf("CreateRepository 'git update-server-info': %s", stderr)
		}
	}
	if err = sess.Commit(); err != nil {
		return nil, err
	}

	// Remember visibility preference
	err = Handle.Users().Update(context.TODO(), owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	return repo, nil
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
		return nil, errors.Newf("Repositories: %v", err)
	}

	for i := range repos {
		if err = repos[i].GetOwner(); err != nil {
			return nil, err
		}
	}

	return repos, nil
}

// FilterRepositoryWithIssues selects repositories that are using internal issue tracker
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
		return nil, errors.Newf("filter valid repositories %v: %v", repoIDs, err)
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
//
// Deprecated: Use repoutil.RepositoryPath instead.
func RepoPath(userName, repoName string) string {
	return filepath.Join(repoutil.UserPath(userName), strings.ToLower(repoName)+".git")
}

// TransferOwnership transfers all corresponding setting from old user to new one.
func TransferOwnership(doer *User, newOwnerName string, repo *Repository) error {
	newOwner, err := Handle.Users().GetByUsername(context.TODO(), newOwnerName)
	if err != nil {
		return errors.Newf("get new owner '%s': %v", newOwnerName, err)
	}

	// Check if new owner has repository with same name.
	has, err := IsRepositoryExist(newOwner, repo.Name)
	if err != nil {
		return errors.Newf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerName": newOwnerName, "name": repo.Name}}
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return errors.Newf("sess.Begin: %v", err)
	}

	owner := repo.Owner

	// Note: we have to set value here to make sure recalculate accesses is based on
	// new owner.
	repo.OwnerID = newOwner.ID
	repo.Owner = newOwner

	// Update repository.
	if _, err := sess.ID(repo.ID).Update(repo); err != nil {
		return errors.Newf("update owner: %v", err)
	}

	// Remove redundant collaborators.
	collaborators, err := repo.getCollaborators(sess)
	if err != nil {
		return errors.Newf("getCollaborators: %v", err)
	}

	// Dummy object.
	collaboration := &Collaboration{RepoID: repo.ID}
	for _, c := range collaborators {
		collaboration.UserID = c.ID
		if c.ID == newOwner.ID || newOwner.IsOrgMember(c.ID) {
			if _, err = sess.Delete(collaboration); err != nil {
				return errors.Newf("remove collaborator '%d': %v", c.ID, err)
			}
		}
	}

	// Remove old team-repository relations.
	if owner.IsOrganization() {
		if err = owner.getTeams(sess); err != nil {
			return errors.Newf("getTeams: %v", err)
		}
		for _, t := range owner.Teams {
			if !t.hasRepository(sess, repo.ID) {
				continue
			}

			t.NumRepos--
			if _, err := sess.ID(t.ID).AllCols().Update(t); err != nil {
				return errors.Newf("decrease team repository count '%d': %v", t.ID, err)
			}
		}

		if err = owner.removeOrgRepo(sess, repo.ID); err != nil {
			return errors.Newf("removeOrgRepo: %v", err)
		}
	}

	if newOwner.IsOrganization() {
		t, err := newOwner.getOwnerTeam(sess)
		if err != nil {
			return errors.Newf("getOwnerTeam: %v", err)
		} else if err = t.addRepository(sess, repo); err != nil {
			return errors.Newf("add to owner team: %v", err)
		}
	} else {
		// Organization called this in addRepository method.
		if err = repo.recalculateAccesses(sess); err != nil {
			return errors.Newf("recalculateAccesses: %v", err)
		}
	}

	// Update repository count.
	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos+1 WHERE id=?", newOwner.ID); err != nil {
		return errors.Newf("increase new owner repository count: %v", err)
	} else if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", owner.ID); err != nil {
		return errors.Newf("decrease old owner repository count: %v", err)
	}

	// Remove watch for organization.
	if owner.IsOrganization() {
		if err = watchRepo(sess, owner.ID, repo.ID, false); err != nil {
			return errors.Wrap(err, "unwatch repository for the organization owner")
		}
	}

	if err = watchRepo(sess, newOwner.ID, repo.ID, true); err != nil {
		return errors.Newf("watchRepo: %v", err)
	}

	// FIXME: This is identical to Actions.TransferRepo but we are not yet able to
	// wrap transaction with different ORM objects, should delete this once migrated
	// to GORM for this part of logic.
	transferRepoAction := func(e Engine, doer, oldOwner *User, repo *Repository) error {
		return notifyWatchers(e, &Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       ActionTransferRepo,
			RepoID:       repo.ID,
			RepoUserName: repo.Owner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
			Content:      path.Join(oldOwner.Name, repo.Name),
			CreatedUnix:  time.Now().Unix(),
		})
	}
	if err = transferRepoAction(sess, doer, owner, repo); err != nil {
		return errors.Newf("transferRepoAction: %v", err)
	}

	// Rename remote repository to new path and delete local copy.
	if err = os.MkdirAll(repoutil.UserPath(newOwner.Name), os.ModePerm); err != nil {
		return err
	}
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
		return errors.Newf("rename repository directory: %v", err)
	}

	deleteRepoLocalCopy(repo.ID)

	// Rename remote wiki repository to new path and delete local copy.
	wikiPath := WikiPath(owner.Name, repo.Name)
	if osutil.Exist(wikiPath) {
		RemoveAllWithNotice("Delete repository wiki local copy", repo.LocalWikiPath())
		if err = os.Rename(wikiPath, WikiPath(newOwner.Name, repo.Name)); err != nil {
			return errors.Newf("rename repository wiki: %v", err)
		}
	}

	return sess.Commit()
}

func deleteRepoLocalCopy(repoID int64) {
	repoWorkingPool.CheckIn(strconv.FormatInt(repoID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(repoID, 10))
	RemoveAllWithNotice(fmt.Sprintf("Delete repository %d local copy", repoID), repoutil.RepositoryLocalPath(repoID))
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
		return errors.Newf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errutil.Args{"ownerID": u.ID, "name": newRepoName}}
	}

	repo, err := GetRepositoryByName(u.ID, oldRepoName)
	if err != nil {
		return errors.Newf("GetRepositoryByName: %v", err)
	}

	// Change repository directory name
	if err = os.Rename(repo.RepoPath(), RepoPath(u.Name, newRepoName)); err != nil {
		return errors.Newf("rename repository directory: %v", err)
	}

	wikiPath := repo.WikiPath()
	if osutil.Exist(wikiPath) {
		if err = os.Rename(wikiPath, WikiPath(u.Name, newRepoName)); err != nil {
			return errors.Newf("rename repository wiki: %v", err)
		}
		RemoveAllWithNotice("Delete repository wiki local copy", repo.LocalWikiPath())
	}

	deleteRepoLocalCopy(repo.ID)
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
		return errors.Newf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(e); err != nil {
			return errors.Newf("getOwner: %v", err)
		}
		if repo.Owner.IsOrganization() {
			// Organization repository need to recalculate access table when visibility is changed
			if err = repo.recalculateTeamAccesses(e, 0); err != nil {
				return errors.Newf("recalculateTeamAccesses: %v", err)
			}
		}

		// Create/Remove git-daemon-export-ok for git-daemon
		daemonExportFile := path.Join(repo.RepoPath(), "git-daemon-export-ok")
		if repo.IsPrivate && osutil.Exist(daemonExportFile) {
			if err = os.Remove(daemonExportFile); err != nil {
				log.Error("Failed to remove %s: %v", daemonExportFile, err)
			}
		} else if !repo.IsPrivate && !osutil.Exist(daemonExportFile) {
			if f, err := os.Create(daemonExportFile); err != nil {
				log.Error("Failed to create %s: %v", daemonExportFile, err)
			} else {
				f.Close()
			}
		}

		forkRepos, err := getRepositoriesByForkID(e, repo.ID)
		if err != nil {
			return errors.Newf("getRepositoriesByForkID: %v", err)
		}
		for i := range forkRepos {
			forkRepos[i].IsPrivate = repo.IsPrivate
			forkRepos[i].IsUnlisted = repo.IsUnlisted
			if err = updateRepository(e, forkRepos[i], true); err != nil {
				return errors.Newf("updateRepository[%d]: %v", forkRepos[i].ID, err)
			}
		}

		// Change visibility of generated actions
		if _, err = e.Where("repo_id = ?", repo.ID).Cols("is_private").Update(&Action{IsPrivate: repo.IsPrivate || repo.IsUnlisted}); err != nil {
			return errors.Newf("change action visibility of repository: %v", err)
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
		return errors.Newf("updateRepository: %v", err)
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
		return ErrRepoNotExist{args: map[string]any{"ownerID": ownerID, "repoID": repoID}}
	}

	// In case is a organization.
	org, err := Handle.Users().GetByID(context.TODO(), ownerID)
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
		return errors.Newf("deleteBeans: %v", err)
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
			return errors.Newf("decrease fork count: %v", err)
		}
	}

	if _, err = sess.Exec("UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", ownerID); err != nil {
		return err
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
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
		return nil, InvalidRepoReference{Ref: ref}
	}

	userName, repoName := ref[:n], ref[n+1:]
	user, err := Handle.Users().GetByUsername(context.TODO(), userName)
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
		return nil, ErrRepoNotExist{args: map[string]any{"ownerID": ownerID, "name": name}}
	}
	return repo, repo.LoadAttributes()
}

func getRepositoryByID(e Engine, id int64) (*Repository, error) {
	repo := new(Repository)
	has, err := e.ID(id).Get(repo)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrRepoNotExist{args: map[string]any{"repoID": id}}
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
		sess.And("is_unlisted=?", false)
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
		return nil, errors.Newf("select collaborative repositories: %v", err)
	}

	ownRepos := make([]*Repository, 0, 10)
	if err := x.Where("owner_id = ?", userID).Find(&ownRepos); err != nil {
		return nil, errors.Newf("select own repositories: %v", err)
	}

	return append(repos, ownRepos...), nil
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
			Where("repo.owner_id = ? OR access.user_id = ? OR (repo.is_private = ? AND repo.is_unlisted = ?) OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?))", opts.UserID, opts.UserID, false, false, true, true, true)
	} else {
		// Only return public repositories if opts.Private is not set
		if !opts.Private {
			sess.And("(repo.is_private = ? AND repo.is_unlisted = ?) OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?))", false, false, true, true, true)
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
		return nil, 0, errors.Newf("Count: %v", err)
	}

	if len(opts.OrderBy) > 0 {
		sess.OrderBy("repo." + opts.OrderBy)
	}
	return repos, count, sess.Distinct("repo.*").Limit(opts.PageSize, (opts.Page-1)*opts.PageSize).Find(&repos)
}

func DeleteOldRepositoryArchives() {
	if taskStatusTable.IsRunning(taskNameCleanOldArchives) {
		return
	}
	taskStatusTable.Start(taskNameCleanOldArchives)
	defer taskStatusTable.Stop(taskNameCleanOldArchives)

	log.Trace("Doing: DeleteOldRepositoryArchives")

	formats := []string{"zip", "targz"}
	oldestTime := time.Now().Add(-conf.Cron.RepoArchiveCleanup.OlderThan)
	if err := x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean any) error {
			repo := bean.(*Repository)
			basePath := filepath.Join(repo.RepoPath(), "archives")
			for _, format := range formats {
				dirPath := filepath.Join(basePath, format)
				if !osutil.IsDir(dirPath) {
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
						const fmtStr = "Failed to health delete archive %q: %v"
						log.Warn(fmtStr, archivePath, err)
						if err = Handle.Notices().Create(
							context.TODO(),
							NoticeTypeRepository,
							fmt.Sprintf(fmtStr, archivePath, err),
						); err != nil {
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
	if taskStatusTable.IsRunning(taskNameCleanOldArchives) {
		return nil
	}
	taskStatusTable.Start(taskNameCleanOldArchives)
	defer taskStatusTable.Stop(taskNameCleanOldArchives)

	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean any) error {
			repo := bean.(*Repository)
			return os.RemoveAll(filepath.Join(repo.RepoPath(), "archives"))
		})
}

func gatherMissingRepoRecords() ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	if err := x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean any) error {
			repo := bean.(*Repository)
			if !osutil.IsDir(repo.RepoPath()) {
				repos = append(repos, repo)
			}
			return nil
		}); err != nil {
		if err2 := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, fmt.Sprintf("gatherMissingRepoRecords: %v", err)); err2 != nil {
			return nil, errors.Newf("CreateRepositoryNotice: %v", err)
		}
	}
	return repos, nil
}

// DeleteMissingRepositories deletes all repository records that lost Git files.
func DeleteMissingRepositories() error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return errors.Newf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		log.Trace("Deleting %d/%d...", repo.OwnerID, repo.ID)
		if err := DeleteRepository(repo.OwnerID, repo.ID); err != nil {
			if err2 := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, fmt.Sprintf("DeleteRepository [%d]: %v", repo.ID, err)); err2 != nil {
				return errors.Newf("CreateRepositoryNotice: %v", err)
			}
		}
	}
	return nil
}

// ReinitMissingRepositories reinitializes all repository records that lost Git files.
func ReinitMissingRepositories() error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return errors.Newf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		log.Trace("Initializing %d/%d...", repo.OwnerID, repo.ID)
		if err := git.Init(repo.RepoPath(), git.InitOptions{Bare: true}); err != nil {
			if err2 := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, fmt.Sprintf("init repository [repo_id: %d]: %v", repo.ID, err)); err2 != nil {
				return errors.Newf("create repository notice: %v", err)
			}
		}
	}
	return nil
}

// SyncRepositoryHooks rewrites all repositories' pre-receive, update and post-receive hooks
// to make sure the binary and custom conf path are up-to-date.
func SyncRepositoryHooks() error {
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean any) error {
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
	taskNameMirrorUpdate     = "mirror_update"
	taskNameGitFSCK          = "git_fsck"
	taskNameCheckRepoStats   = "check_repos_stats"
	taskNameCleanOldArchives = "clean_old_archives"
)

// GitFsck calls 'git fsck' to check repository health.
func GitFsck() {
	if taskStatusTable.IsRunning(taskNameGitFSCK) {
		return
	}
	taskStatusTable.Start(taskNameGitFSCK)
	defer taskStatusTable.Stop(taskNameGitFSCK)

	log.Trace("Doing: GitFsck")

	if err := x.Where("id>0").Iterate(new(Repository),
		func(idx int, bean any) error {
			repo := bean.(*Repository)
			repoPath := repo.RepoPath()
			err := git.Fsck(repoPath, git.FsckOptions{
				CommandOptions: git.CommandOptions{
					Args: conf.Cron.RepoHealthCheck.Args,
				},
				Timeout: conf.Cron.RepoHealthCheck.Timeout,
			})
			if err != nil {
				const fmtStr = "Failed to perform health check on repository %q: %v"
				log.Warn(fmtStr, repoPath, err)
				if err = Handle.Notices().Create(
					context.TODO(),
					NoticeTypeRepository,
					fmt.Sprintf(fmtStr, repoPath, err),
				); err != nil {
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
		func(idx int, bean any) error {
			repo := bean.(*Repository)
			if err := repo.GetOwner(); err != nil {
				return err
			}
			_, stderr, err := process.ExecDir(
				time.Duration(conf.Git.Timeout.GC)*time.Second,
				RepoPath(repo.Owner.Name, repo.Name), "Repository garbage collection",
				"git", args...)
			if err != nil {
				return errors.Newf("%v: %v", err, stderr)
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
		id, _ := strconv.ParseInt(string(result["id"]), 10, 64)
		log.Trace("Updating %s: %d", checker.desc, id)
		_, err = x.Exec(checker.correctSQL, id, id)
		if err != nil {
			log.Error("Update %s[%d]: %v", checker.desc, id, err)
		}
	}
}

func CheckRepoStats() {
	if taskStatusTable.IsRunning(taskNameCheckRepoStats) {
		return
	}
	taskStatusTable.Start(taskNameCheckRepoStats)
	defer taskStatusTable.Stop(taskNameCheckRepoStats)

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
			id, _ := strconv.ParseInt(string(result["id"]), 10, 64)
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
			id, _ := strconv.ParseInt(string(result["id"]), 10, 64)
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
		return errors.Newf("find users: %v", err)
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
		return errors.Newf("find base repositories: %v", err)
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
		return errors.Newf("find mirrors: %v", err)
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
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `xorm:"UNIQUE(watch)" gorm:"uniqueIndex:watch_user_repo_unique;not null"`
	RepoID int64 `xorm:"UNIQUE(watch)" gorm:"uniqueIndex:watch_user_repo_unique;not null"`
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
//
// Deprecated: Use Watches.Watch instead.
func WatchRepo(userID, repoID int64, watch bool) (err error) {
	return watchRepo(x, userID, repoID, watch)
}

// Deprecated: Use Repos.ListByRepo instead.
func getWatchers(e Engine, repoID int64) ([]*Watch, error) {
	watches := make([]*Watch, 0, 10)
	return watches, e.Find(&watches, &Watch{RepoID: repoID})
}

// GetWatchers returns all watchers of given repository.
//
// Deprecated: Use Repos.ListByRepo instead.
func GetWatchers(repoID int64) ([]*Watch, error) {
	return getWatchers(x, repoID)
}

// Repository.GetWatchers returns range of users watching given repository.
func (r *Repository) GetWatchers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("watch.repo_id=?", r.ID)
	if conf.UsePostgreSQL {
		sess = sess.Join("LEFT", "watch", `"user".id=watch.user_id`)
	} else {
		sess = sess.Join("LEFT", "watch", "user.id=watch.user_id")
	}
	return users, sess.Find(&users)
}

// Deprecated: Use Actions.notifyWatchers instead.
func notifyWatchers(e Engine, act *Action) error {
	if act.CreatedUnix <= 0 {
		act.CreatedUnix = time.Now().Unix()
	}

	// Add feeds for user self and all watchers.
	watchers, err := getWatchers(e, act.RepoID)
	if err != nil {
		return errors.Newf("getWatchers: %v", err)
	}

	// Reset ID to reuse Action object
	act.ID = 0

	// Add feed for actioner.
	act.UserID = act.ActUserID
	if _, err = e.Insert(act); err != nil {
		return errors.Newf("insert new action: %v", err)
	}

	for i := range watchers {
		if act.ActUserID == watchers[i].UserID {
			continue
		}

		act.ID = 0
		act.UserID = watchers[i].UserID
		if _, err = e.Insert(act); err != nil {
			return errors.Newf("insert new action: %v", err)
		}
	}
	return nil
}

// NotifyWatchers creates batch of actions for every watcher.
//
// Deprecated: Use Actions.notifyWatchers instead.
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
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `xorm:"uid UNIQUE(s)" gorm:"column:uid;uniqueIndex:star_user_repo_unique;not null"`
	RepoID int64 `xorm:"UNIQUE(s)" gorm:"uniqueIndex:star_user_repo_unique;not null"`
}

// Star or unstar repository.
//
// Deprecated: Use Stars.Star instead.
func StarRepo(userID, repoID int64, star bool) (err error) {
	if star {
		if IsStaring(userID, repoID) {
			return nil
		}
		if _, err = x.Insert(&Star{UserID: userID, RepoID: repoID}); err != nil {
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

func (r *Repository) GetStargazers(page int) ([]*User, error) {
	users := make([]*User, 0, ItemsPerPage)
	sess := x.Limit(ItemsPerPage, (page-1)*ItemsPerPage).Where("star.repo_id=?", r.ID)
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
	if !owner.canCreateRepo() {
		return nil, ErrReachLimitOfRepo{Limit: owner.maxNumRepos()}
	}

	repo := &Repository{
		OwnerID:       owner.ID,
		Owner:         owner,
		Name:          name,
		LowerName:     strings.ToLower(name),
		Description:   desc,
		DefaultBranch: baseRepo.DefaultBranch,
		IsPrivate:     baseRepo.IsPrivate,
		IsUnlisted:    baseRepo.IsUnlisted,
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
		return nil, errors.Newf("git clone: %v - %s", err, stderr)
	}

	_, stderr, err = process.ExecDir(-1,
		repoPath, fmt.Sprintf("ForkRepository 'git update-server-info': %s", repoPath),
		"git", "update-server-info")
	if err != nil {
		return nil, errors.Newf("git update-server-info: %v - %s", err, stderr)
	}

	if err = createDelegateHooks(repoPath); err != nil {
		return nil, errors.Newf("createDelegateHooks: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return nil, errors.Newf("commit: %v", err)
	}

	// Remember visibility preference
	err = Handle.Users().Update(context.TODO(), owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	if err = repo.UpdateSize(); err != nil {
		log.Error("UpdateSize [repo_id: %d]: %v", repo.ID, err)
	}
	if err = PrepareWebhooks(baseRepo, HookEventTypeFork, &apiv1types.WebhookForkPayload{
		Forkee: repo.APIFormatLegacy(nil),
		Repo:   baseRepo.APIFormatLegacy(nil),
		Sender: doer.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks [repo_id: %d]: %v", baseRepo.ID, err)
	}
	return repo, nil
}

func (r *Repository) GetForks() ([]*Repository, error) {
	forks := make([]*Repository, 0, r.NumForks)
	if err := x.Find(&forks, &Repository{ForkID: r.ID}); err != nil {
		return nil, err
	}

	for _, fork := range forks {
		fork.BaseRepo = r
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

func (r *Repository) CreateNewBranch(oldBranch, newBranch string) (err error) {
	repoWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer repoWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	localPath := r.LocalCopyPath()

	if err = discardLocalRepoBranchChanges(localPath, oldBranch); err != nil {
		return errors.Newf("discard changes in local copy [path: %s, branch: %s]: %v", localPath, oldBranch, err)
	} else if err = r.UpdateLocalCopyBranch(oldBranch); err != nil {
		return errors.Newf("update branch for local copy [path: %s, branch: %s]: %v", localPath, oldBranch, err)
	}

	if err = r.CheckoutNewBranch(oldBranch, newBranch); err != nil {
		return errors.Newf("create new branch [base: %s, new: %s]: %v", oldBranch, newBranch, err)
	}

	if err = git.Push(localPath, "origin", newBranch); err != nil {
		return errors.Newf("push [branch: %s]: %v", newBranch, err)
	}

	return nil
}

// Deprecated: Use Perms.SetRepoPerms instead.
func (r *Repository) refreshAccesses(e Engine, accessMap map[int64]AccessMode) (err error) {
	newAccesses := make([]Access, 0, len(accessMap))
	for userID, mode := range accessMap {
		newAccesses = append(newAccesses, Access{
			UserID: userID,
			RepoID: r.ID,
			Mode:   mode,
		})
	}

	// Delete old accesses and insert new ones for repository.
	if _, err = e.Delete(&Access{RepoID: r.ID}); err != nil {
		return errors.Newf("delete old accesses: %v", err)
	} else if _, err = e.Insert(newAccesses); err != nil {
		return errors.Newf("insert new accesses: %v", err)
	}
	return nil
}

// refreshCollaboratorAccesses retrieves repository collaborations with their access modes.
func (r *Repository) refreshCollaboratorAccesses(e Engine, accessMap map[int64]AccessMode) error {
	collaborations, err := r.getCollaborations(e)
	if err != nil {
		return errors.Newf("getCollaborations: %v", err)
	}
	for _, c := range collaborations {
		accessMap[c.UserID] = c.Mode
	}
	return nil
}

// recalculateTeamAccesses recalculates new accesses for teams of an organization
// except the team whose ID is given. It is used to assign a team ID when
// remove repository from that team.
func (r *Repository) recalculateTeamAccesses(e Engine, ignTeamID int64) (err error) {
	accessMap := make(map[int64]AccessMode, 20)

	if err = r.getOwner(e); err != nil {
		return err
	} else if !r.Owner.IsOrganization() {
		return errors.Newf("owner is not an organization: %d", r.OwnerID)
	}

	if err = r.refreshCollaboratorAccesses(e, accessMap); err != nil {
		return errors.Newf("refreshCollaboratorAccesses: %v", err)
	}

	if err = r.Owner.getTeams(e); err != nil {
		return err
	}

	maxAccessMode := func(modes ...AccessMode) AccessMode {
		max := AccessModeNone
		for _, mode := range modes {
			if mode > max {
				max = mode
			}
		}
		return max
	}

	for _, t := range r.Owner.Teams {
		if t.ID == ignTeamID {
			continue
		}

		// Owner team gets owner access, and skip for teams that do not
		// have relations with repository.
		if t.IsOwnerTeam() {
			t.Authorize = AccessModeOwner
		} else if !t.hasRepository(e, r.ID) {
			continue
		}

		if err = t.getMembers(e); err != nil {
			return errors.Newf("getMembers '%d': %v", t.ID, err)
		}
		for _, m := range t.Members {
			accessMap[m.ID] = maxAccessMode(accessMap[m.ID], t.Authorize)
		}
	}

	return r.refreshAccesses(e, accessMap)
}

func (r *Repository) recalculateAccesses(e Engine) error {
	if r.Owner.IsOrganization() {
		return r.recalculateTeamAccesses(e, 0)
	}

	accessMap := make(map[int64]AccessMode, 10)
	if err := r.refreshCollaboratorAccesses(e, accessMap); err != nil {
		return errors.Newf("refreshCollaboratorAccesses: %v", err)
	}
	return r.refreshAccesses(e, accessMap)
}

// RecalculateAccesses recalculates all accesses for repository.
func (r *Repository) RecalculateAccesses() error {
	return r.recalculateAccesses(x)
}
