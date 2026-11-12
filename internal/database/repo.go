package database

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
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
	"gorm.io/gorm"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	embedConf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/semverx"
	"gogs.io/gogs/internal/strx"
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

func LoadRepoConfig(logger *logx.Logger) {
	// Load .gitignore and license files and readme templates.
	types := []string{"gitignore", "license", "readme", "label"}
	typeFiles := make([][]string, 4)
	for i, t := range types {
		files, err := embedConf.FileNames(t)
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to get embedded config files", "type", t, "error", err)
		}

		customPath := filepath.Join(conf.CustomDir(), "conf", t)
		if osx.IsDir(customPath) {
			entries, err := os.ReadDir(customPath)
			if err != nil {
				logger.FatalContext(context.Background(), "Failed to get custom config files", "type", t, "error", err)
			}

			for _, entry := range entries {
				f := entry.Name()
				if !strx.ContainsFold(files, f) {
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

func NewRepoContext(logger *logx.Logger) {
	zip.Verbose = false

	// Check Git installation and version.
	var err error
	conf.Git.Version, err = git.BinVersion(context.TODO())
	if err != nil {
		logger.FatalContext(context.Background(), "Failed to get Git version", "error", err)
	}

	logger.Debug("Git version", "version", conf.Git.Version)
	if semverx.Compare(conf.Git.Version, "<", "2.43.1") {
		logger.FatalContext(context.Background(), "Gogs requires Git version 2.43.1 or later", "version", conf.Git.Version)
	}

	// Git requires setting user.name and user.email in order to commit changes.
	for configKey, defaultValue := range map[string]string{"user.name": "Gogs", "user.email": "gogs@fake.local"} {
		value, err := git.ConfigGet(context.TODO(), configKey)
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to get Git config", "configKey", configKey, "error", err)
		} else if value != "" {
			continue
		}

		if err = git.SetGlobalConfig(context.TODO(), configKey, defaultValue); err != nil {
			logger.FatalContext(context.Background(), "Failed to set Git config", "configKey", configKey, "error", err)
		}
		logger.Info("Git config set", "configKey", configKey, "configValue", defaultValue)
	}

	// Set git some configurations.
	if err = git.SetGlobalConfig(context.TODO(), "core.quotepath", "false"); err != nil {
		logger.FatalContext(context.Background(), "Failed to set Git config", "configKey", "core.quotepath", "error", err)
	}

	RemoveAllWithNotice(logger, "Clean up repository temporary data", filepath.Join(conf.Server.AppDataPath, "tmp"))
	RemoveAllWithNotice(logger, "Clean up LFS temporary data", conf.LFS.ObjectsTempPath)
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
	// Stored as "is_bare" and encoded as "IsBare" to stay compatible with
	// existing databases and backup archives.
	IsEmpty bool `xorm:"is_bare" gorm:"column:is_bare" json:"IsBare"`

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
	PullsAllowMerge       bool              `xorm:"NOT NULL DEFAULT true" gorm:"not null;default:TRUE"`
	PullsAllowRebase      bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	PullsAllowSquash      bool              `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	// PullsDeleteBranch is whether head branches in the repository are
	// deleted after their pull requests are merged.
	PullsDeleteBranch bool `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	// PullsMergeMessage and PullsSquashMessage are the presets of the
	// messages merge commits and squashing start with, e.g.,
	// MergeMessageTitle.
	PullsMergeMessage  string `xorm:"NOT NULL DEFAULT ''" gorm:"not null;default:''"`
	PullsSquashMessage string `xorm:"NOT NULL DEFAULT ''" gorm:"not null;default:''"`

	IsFork   bool `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	ForkID   int64
	BaseRepo *Repository `xorm:"-" gorm:"-" json:"-"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64
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
	if r.Owner != nil {
		return r.Owner
	}
	return r.mustOwner(Handle.db)
}

func (r *Repository) FullName() string {
	return r.MustOwner().Name + "/" + r.Name
}

// Deprecated: Use repox.HTMLURL instead.
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
	if !osx.Exist(r.CustomAvatarPath()) {
		return defaultImgURL
	}
	return fmt.Sprintf("%s/%s/%d", conf.Server.Subpath, RepoAvatarURLPrefix, r.ID)
}

// AvatarLink returns repository avatar absolute link.
func (r *Repository) AvatarLink() string {
	link := r.RelAvatarLink()
	if len(link) > 2 && link[0] == '/' && link[1] != '/' {
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
func (r *Repository) DeleteAvatar(logger *logx.Logger) error {
	logger.Debug("Deleting repository avatar", "repoID", r.ID, "avatarPath", r.CustomAvatarPath())
	if err := os.Remove(r.CustomAvatarPath()); err != nil {
		return err
	}

	r.UseCustomAvatar = false
	return UpdateRepository(logger, r, false)
}

// This method assumes following fields have been assigned with valid values:
// Required - BaseRepo (if fork)
// Arguments that are allowed to be nil: permission
//
// Deprecated: Use APIFormat instead.
func (r *Repository) APIFormatLegacy(logger *logx.Logger, permission *apiv1types.RepositoryPermission, user ...*User) *apiv1types.Repository {
	cloneLink := r.CloneLink()
	apiRepo := &apiv1types.Repository{
		ID:            r.ID,
		Owner:         r.Owner.APIFormat(),
		Name:          r.Name,
		FullName:      r.FullName(),
		Description:   r.Description,
		Private:       r.IsPrivate,
		Fork:          r.IsFork,
		Empty:         r.IsEmpty,
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
				context.TODO(), logger,
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
		apiRepo.Parent = r.BaseRepo.APIFormatLegacy(logger, p)
	}
	return apiRepo
}

func (r *Repository) getOwner(tx *gorm.DB) (err error) {
	if r.Owner != nil {
		return nil
	}

	r.Owner, err = newUsersStore(tx).GetByID(context.TODO(), r.OwnerID)
	return err
}

func (r *Repository) GetOwner() error {
	if r.Owner != nil {
		return nil
	}
	return r.getOwner(Handle.db)
}

func (r *Repository) mustOwner(tx *gorm.DB) *User {
	if err := r.getOwner(tx); err != nil {
		return &User{
			Name:     "error",
			FullName: err.Error(),
		}
	}

	return r.Owner
}

func (r *Repository) UpdateSize(ctx context.Context) error {
	countObject, err := git.RepoCountObjects(ctx, r.RepoPath())
	if err != nil {
		return errors.Newf("count repository objects: %v", err)
	}

	r.Size = countObject.Size + countObject.SizePack
	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err = Handle.db.Model(r).Select("size").UpdateColumns(r).Error; err != nil {
		return errors.Newf("update size: %v", err)
	}
	return nil
}

// ComposeMetas composes a map of metas for rendering commit URL and external issue tracker URL.
func (r *Repository) ComposeMetas() map[string]string {
	if r.ExternalMetas != nil {
		return r.ExternalMetas
	}

	r.ExternalMetas = map[string]string{
		"repoLink": r.Link(),
	}

	// Markup links hex strings of 64 characters to commits only in SHA-256
	// repositories, because elsewhere they are checksums and the like.
	objectFormat, err := git.RepoObjectFormat(context.Background(), r.RepoPath())
	if err != nil {
		handleLogger().Error("Failed to get object format of repository", "repo", r.FullName(), "error", err)
	} else if objectFormat == git.ObjectFormatSHA256 {
		r.ExternalMetas["objectFormat"] = string(objectFormat)
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

// DeleteWiki removes the repository wiki.
func (r *Repository) DeleteWiki(logger *logx.Logger) {
	RemoveAllWithNotice(logger, "Delete repository wiki", r.WikiPath())
}

// getUsersWithAccesMode returns users that have at least given access mode to the repository.
func (r *Repository) getUsersWithAccesMode(tx *gorm.DB, mode AccessMode) (_ []*User, err error) {
	if err = r.getOwner(tx); err != nil {
		return nil, err
	}

	accesses := make([]*Access, 0, 10)
	if err = tx.Where("repo_id = ? AND mode >= ?", r.ID, mode).Find(&accesses).Error; err != nil {
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

		if err = tx.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
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
func (r *Repository) getAssignees(tx *gorm.DB) (_ []*User, err error) {
	return r.getUsersWithAccesMode(tx, AccessModeRead)
}

// GetAssignees returns all users that have read access and can be assigned to issues
// of the repository,
func (r *Repository) GetAssignees() (_ []*User, err error) {
	return r.getAssignees(Handle.db)
}

// GetAssigneeByID returns the user that has write access of repository by given ID.
func (r *Repository) GetAssigneeByID(logger *logx.Logger, userID int64) (*User, error) {
	ctx := context.TODO()
	if !Handle.Permissions().Authorize(
		ctx, logger,
		userID,
		r.ID,
		AccessModeRead,
		AccessModeOptions{
			OwnerID: r.OwnerID,
			Private: r.IsPrivate,
		},
	) {
		return nil, ErrUserNotExist{args: errx.Args{"userID": userID}}
	}
	return Handle.Users().GetByID(ctx, userID)
}

// GetWriters returns all users that have write access to the repository.
func (r *Repository) GetWriters() (_ []*User, err error) {
	return r.getUsersWithAccesMode(Handle.db, AccessModeWrite)
}

func (r *Repository) repoPath(tx *gorm.DB) string {
	return RepoPath(r.mustOwner(tx).Name, r.Name)
}

// Deprecated: Use repox.RepositoryPath instead.
func (r *Repository) RepoPath() string {
	return RepoPath(r.MustOwner().Name, r.Name)
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

// Deprecated: Use repox.ComparePath instead.
func (r *Repository) ComposeCompareURL(oldCommitID, newCommitID string) string {
	return fmt.Sprintf("%s/%s/compare/%s...%s", r.MustOwner().Name, r.Name, oldCommitID, newCommitID)
}

func (r *Repository) HasAccess(logger *logx.Logger, userID int64) bool {
	return Handle.Permissions().Authorize(context.TODO(), logger, userID, r.ID, AccessModeRead,
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
	return !r.IsEmpty
}

// CanEnablePulls returns true if repository meets the requirements of accepting pulls.
func (r *Repository) CanEnablePulls() bool {
	return !r.IsMirror && !r.IsEmpty
}

// AllowPulls returns true if repository meets the requirements of accepting pulls and has them enabled.
func (r *Repository) AllowsPulls() bool {
	return r.CanEnablePulls() && r.EnablePulls
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

func isRepositoryExist(tx *gorm.DB, u *User, repoName string) (bool, error) {
	var count int64
	err := tx.Model(new(Repository)).Where("owner_id = ? AND lower_name = ?", u.ID, strings.ToLower(repoName)).Count(&count).Error
	return count > 0 && osx.IsDir(RepoPath(u.Name, repoName)), err
}

// IsRepositoryExist returns true if the repository with given name under user has already existed.
func IsRepositoryExist(u *User, repoName string) (bool, error) {
	return isRepositoryExist(Handle.db, u, repoName)
}

// Deprecated: Use repox.NewCloneLink instead.
func (r *Repository) cloneLink(isWiki bool) *repox.CloneLink {
	repoName := r.Name
	if isWiki {
		repoName += ".wiki"
	}

	r.Owner = r.MustOwner()
	cl := new(repox.CloneLink)
	if conf.SSH.Port != 22 {
		cl.SSH = fmt.Sprintf("ssh://%s@%s:%d/%s/%s.git", conf.App.RunUser, conf.SSH.Domain, conf.SSH.Port, r.Owner.Name, repoName)
	} else {
		cl.SSH = fmt.Sprintf("%s@%s:%s/%s.git", conf.App.RunUser, conf.SSH.Domain, r.Owner.Name, repoName)
	}
	cl.HTTPS = repox.HTTPSCloneURL(r.Owner.Name, repoName)
	return cl
}

// CloneLink returns clone URLs of repository.
//
// Deprecated: Use repox.NewCloneLink instead.
func (r *Repository) CloneLink() (cl *repox.CloneLink) {
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
		if isMirrorURLAccessible(time.Minute, wikiURL) {
			return wikiURL
		}
	}
	return ""
}

// MigrateRepository migrates a existing repository from other project hosting.
func MigrateRepository(ctx context.Context, logger *logx.Logger, doer, owner *User, opts MigrateRepoOptions) (*Repository, error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	repo, err := CreateRepository(ctx, logger, doer, owner, CreateRepoOptionsLegacy{
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
		t, err := Handle.Teams().GetOwnerTeam(context.TODO(), owner.ID)
		if err != nil {
			return nil, err
		}
		repo.NumWatches = t.NumMembers
	} else {
		repo.NumWatches = 1
	}

	migrateTimeout := time.Duration(conf.Git.Timeout.Migrate) * time.Second

	mirrorClone := func(url, dst string) error {
		ctx, cancel := git.WithTimeout(ctx, migrateTimeout)
		defer cancel()
		return git.Clone(ctx, url, dst, git.CloneOptions{
			Mirror: true,
			Quiet:  true,
			Envs:   mirrorGitEnv(),
		})
	}

	RemoveAllWithNotice(logger, "Repository path erase before creation", repoPath)
	if err := mirrorClone(opts.RemoteAddr, repoPath); err != nil {
		return repo, errors.Newf("clone: %v", err)
	}

	wikiRemotePath := wikiRemoteURL(opts.RemoteAddr)
	if len(wikiRemotePath) > 0 {
		RemoveAllWithNotice(logger, "Repository wiki path erase before creation", wikiPath)
		if err := mirrorClone(wikiRemotePath, wikiPath); err != nil {
			logger.Error("Failed to clone wiki", "error", err)
			RemoveAllWithNotice(logger, "Delete repository wiki for initialization failure", wikiPath)
		}
	}

	gitRepo, err := git.Open(repoPath)
	if err != nil {
		return repo, errors.Newf("open repository: %v", err)
	}

	// Set the HEAD branch as default branch, which is also available for empty
	// repositories.
	refspec, err := gitRepo.SymbolicRef(ctx)
	if err != nil {
		return repo, errors.Newf("get HEAD branch: %v", err)
	}
	repo.DefaultBranch = git.RefShortName(refspec)

	hasBranches, err := gitRepo.HasBranches(ctx)
	if err != nil {
		return repo, errors.Wrap(err, "check empty")
	}
	repo.IsEmpty = !hasBranches

	// HEAD may point to a branch that does not exist while other branches do, in
	// which case any existing branch becomes the default branch.
	if !repo.IsEmpty && !gitRepo.HasBranch(ctx, repo.DefaultBranch) {
		branches, err := gitRepo.Branches(ctx)
		if err != nil {
			return repo, errors.Wrap(err, "list branches")
		}
		repo.DefaultBranch = branches[0]
		_, err = gitRepo.SymbolicRef(ctx, git.SymbolicRefOptions{Ref: git.RefsHeads + repo.DefaultBranch})
		if err != nil {
			return repo, errors.Wrap(err, "set HEAD branch")
		}
	}

	if !repo.IsEmpty {
		if err = repo.UpdateSize(ctx); err != nil {
			logger.Error("Failed to update repository size", "repoID", repo.ID, "error", err)
		}
	}

	if opts.IsMirror {
		if err = Handle.db.Create(&Mirror{
			RepoID:      repo.ID,
			Interval:    conf.Mirror.DefaultInterval,
			EnablePrune: true,
			NextSync:    time.Now().Add(time.Duration(conf.Mirror.DefaultInterval) * time.Hour),
		}).Error; err != nil {
			return repo, errors.Newf("InsertOne: %v", err)
		}

		repo.IsMirror = true
		return repo, UpdateRepository(logger, repo, false)
	}

	return CleanUpMigrateInfo(logger, repo)
}

// setDefaultBranchIfMissing replaces the saved default branch when it does not
// exist, e.g., when the first push to an empty repository uses a different
// branch name than the one set at creation, or when no default branch was saved.
// See resolveMissingDefaultBranch for which branch replaces it.
func (r *Repository) setDefaultBranchIfMissing(ctx context.Context, candidate string) error {
	branch, err := resolveMissingDefaultBranch(ctx, r.RepoPath(), r.DefaultBranch, candidate)
	if err != nil || branch == "" || branch == r.DefaultBranch {
		return err
	}

	r.DefaultBranch = branch
	if _, err = x.Id(r.ID).Cols("default_branch").Update(r); err != nil {
		return errors.Wrap(err, "update default branch")
	}
	return nil
}

// resolveMissingDefaultBranch returns the branch that should replace the saved
// default branch of the repository in given path, or an empty string when the
// saved one exists or no replacement exists. HEAD is the source of truth, so the
// branch it points to is used when it exists. Otherwise, HEAD is pointed to the
// candidate branch if that exists.
func resolveMissingDefaultBranch(ctx context.Context, repoPath, savedBranch, candidate string) (string, error) {
	if savedBranch != "" && git.RepoHasBranch(ctx, repoPath, savedBranch) {
		return "", nil
	}

	head, err := git.RepoSymbolicRef(ctx, repoPath)
	if err != nil {
		return "", errors.Wrap(err, "get HEAD")
	}
	if headBranch := git.RefShortName(head); git.RepoHasBranch(ctx, repoPath, headBranch) {
		return headBranch, nil
	}

	if candidate == "" || !git.RepoHasBranch(ctx, repoPath, candidate) {
		return "", nil
	}
	_, err = git.RepoSymbolicRef(ctx, repoPath, git.SymbolicRefOptions{Ref: git.RefsHeads + candidate})
	if err != nil {
		return "", errors.Wrap(err, "set HEAD")
	}
	return candidate, nil
}

// cleanUpMigrateGitConfig removes mirror info which prevents "push --all".
// This also removes possible user credentials.
func cleanUpMigrateGitConfig(configPath string) error {
	cfg, err := ini.Load(configPath)
	if err != nil {
		return errors.Newf("open config file: %v", err)
	}
	for sub, sec := range cfg.Subsections("remote") {
		if sub == "origin" {
			cfg.DeleteSection(sec.Name())
		}
	}
	if err = cfg.SaveTo(configPath, ini.Indent("\t")); err != nil {
		return errors.Newf("save config file: %v", err)
	}
	return nil
}

// The delegate hooks pass on to "gogs hook". A hook process spends most of its
// time starting up, when Go would otherwise collect garbage, and exits before it
// uses much memory, so the garbage collector is turned off for it.
var hooksTpls = map[git.HookName]string{
	"pre-receive":  "#!/usr/bin/env %s\nGOGC=off \"%s\" hook --config='%s' pre-receive\n",
	"update":       "#!/usr/bin/env %s\nGOGC=off \"%s\" hook --config='%s' update $1 $2 $3\n",
	"post-receive": "#!/usr/bin/env %s\nGOGC=off \"%s\" hook --config='%s' post-receive\n",
}

func writeDelegateHook(repoPath string, name git.HookName) error {
	hookPath := filepath.Join(repoPath, "hooks", string(name))
	err := os.WriteFile(hookPath,
		fmt.Appendf(nil, hooksTpls[name], conf.Repository.ScriptType, conf.AppPath(), conf.CustomConf),
		os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "write delegate hook %q", hookPath)
	}
	return nil
}

// syncDelegateHooks installs the delegate hooks that the repository in given
// path needs, and removes the others, because Git starts a process for every
// hook, and for the update hook, once for every reference a push updates. A
// hook is needed when the repository has a custom hook of the same name. The
// pre-receive hook is also needed when hasProtectedBranch reports that the
// repository has a protected branch, because it enforces the protection.
//
// Saving the protection of a branch, maybe in another process, installs the
// pre-receive hook before it commits and syncs after it. So that a sync running
// at the same time never leaves a protected branch without the hook, the
// protection is checked again after the hook was removed, and the hook is put
// back when a branch has been protected in the meantime.
func syncDelegateHooks(repoPath string, hasProtectedBranch func() (bool, error)) error {
	for _, name := range git.ServerSideHooks {
		needed := osx.IsFile(filepath.Join(repoPath, "custom_hooks", string(name)))
		if !needed && name == git.HookPreReceive {
			protected, err := hasProtectedBranch()
			if err != nil {
				return err
			}
			needed = protected
		}
		if needed {
			if err := writeDelegateHook(repoPath, name); err != nil {
				return err
			}
			continue
		}

		hookPath := filepath.Join(repoPath, "hooks", string(name))
		if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
			return errors.Wrapf(err, "remove delegate hook %q", hookPath)
		}
		if name != git.HookPreReceive {
			continue
		}
		protected, err := hasProtectedBranch()
		if err != nil {
			return err
		}
		if protected {
			if err = writeDelegateHook(repoPath, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncDelegateHooks installs the delegate hooks that the repository and its
// wiki need, and removes the others. It must be called whenever their custom
// hooks or the protection of the branches of the repository change.
func (r *Repository) SyncDelegateHooks() error {
	return syncRepoDelegateHooks(Handle.db, r)
}

func syncRepoDelegateHooks(db *gorm.DB, r *Repository) error {
	hasProtectedBranch := func() (bool, error) {
		var count int64
		err := db.Model(new(ProtectBranch)).Where("repo_id = ? AND protected = ?", r.ID, true).Count(&count).Error
		if err != nil {
			return false, errors.Wrap(err, "check protected branches")
		}
		return count > 0, nil
	}
	if err := syncDelegateHooks(r.RepoPath(), hasProtectedBranch); err != nil {
		return err
	}
	if r.HasWiki() {
		return syncDelegateHooks(r.WikiPath(), func() (bool, error) { return false, nil })
	}
	return nil
}

// Finish migrating repository and/or wiki with things that don't need to be done for mirrors.
func CleanUpMigrateInfo(logger *logx.Logger, repo *Repository) (*Repository, error) {
	if err := cleanUpMigrateGitConfig(repo.GitConfigPath()); err != nil {
		return repo, errors.Newf("cleanUpMigrateGitConfig: %v", err)
	}
	if repo.HasWiki() {
		if err := cleanUpMigrateGitConfig(path.Join(repo.WikiPath(), "config")); err != nil {
			return repo, errors.Newf("cleanUpMigrateGitConfig.(wiki): %v", err)
		}
	}

	return repo, UpdateRepository(logger, repo, false)
}

// initRepoCommit commits the files to the default branch of the empty
// repository at repoPath.
func initRepoCommit(ctx context.Context, repoPath string, files map[string][]byte, author *git.Signature) error {
	_, err := commitAndPush(ctx, repoPath, nil, conf.Repository.DefaultBranch, "Initial commit", author, author, git.PushOptions{}, func(tmpRepo *git.Repository) error {
		indexFiles := make([]git.IndexFile, 0, len(files))
		for name, data := range files {
			indexFiles = append(indexFiles, git.IndexFile{Path: name, Content: bytes.NewReader(data)})
		}
		return errors.Wrap(tmpRepo.AddToIndex(ctx, indexFiles...), "add files")
	})
	return err
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
	// The hash algorithm of the repository, or empty to use the default of Git on
	// the server.
	ObjectFormat git.ObjectFormat
}

func getRepoInitFile(tp, name string) ([]byte, error) {
	var names []string
	switch tp {
	case "gitignore":
		names = Gitignores
	case "license":
		names = Licenses
	case "readme":
		names = Readmes
	case "label":
		names = LabelTemplates
	}
	// 🚨 SECURITY: Only accept names of known templates, so that the name can
	// never resolve to a file outside of the template directory.
	if !strx.ContainsFold(names, name) {
		return nil, errors.Newf("unknown %s template %q", tp, name)
	}

	relPath := path.Join(tp, name)

	// Use custom file when available.
	customPath := filepath.Join(conf.CustomDir(), "conf", relPath)
	if osx.IsFile(customPath) {
		return os.ReadFile(customPath)
	}
	return embedConf.Files.ReadFile(relPath)
}

// prepareRepoCommit returns the files of the initial commit by their names.
func prepareRepoCommit(repo *Repository, opts CreateRepoOptionsLegacy) (map[string][]byte, error) {
	files := make(map[string][]byte, 3)

	// README
	data, err := getRepoInitFile("readme", opts.Readme)
	if err != nil {
		return nil, errors.Newf("getRepoInitFile[%s]: %v", opts.Readme, err)
	}

	cloneLink := repo.CloneLink()
	match := map[string]string{
		"Name":           repo.Name,
		"Description":    repo.Description,
		"CloneURL.SSH":   cloneLink.SSH,
		"CloneURL.HTTPS": cloneLink.HTTPS,
	}
	files["README.md"] = []byte(com.Expand(string(data), match))

	// .gitignore
	if len(opts.Gitignores) > 0 {
		var buf bytes.Buffer
		names := strings.SplitSeq(opts.Gitignores, ",")
		for name := range names {
			data, err = getRepoInitFile("gitignore", name)
			if err != nil {
				return nil, errors.Newf("getRepoInitFile[%s]: %v", name, err)
			}
			buf.WriteString("# ---> " + name + "\n")
			buf.Write(data)
			buf.WriteString("\n")
		}

		if buf.Len() > 0 {
			files[".gitignore"] = buf.Bytes()
		}
	}

	// LICENSE
	if len(opts.License) > 0 {
		data, err = getRepoInitFile("license", opts.License)
		if err != nil {
			return nil, errors.Newf("getRepoInitFile[%s]: %v", opts.License, err)
		}
		files["LICENSE"] = data
	}

	return files, nil
}

// initRepository performs initial commit with chosen setup files on behave of doer.
func initRepository(ctx context.Context, logger *logx.Logger, tx *gorm.DB, repoPath string, doer *User, repo *Repository, opts CreateRepoOptionsLegacy) (err error) {
	// Init bare new repository.
	if err = git.Init(ctx, repoPath, git.InitOptions{Bare: true, ObjectFormat: opts.ObjectFormat}); err != nil {
		return errors.Newf("init repository: %v", err)
	}

	// Set default branch
	_, err = git.RepoSymbolicRef(
		ctx,
		repoPath,
		git.SymbolicRefOptions{
			Name: "HEAD",
			Ref:  git.RefsHeads + conf.Repository.DefaultBranch,
		},
	)
	if err != nil {
		return errors.Wrap(err, "set default branch")
	}

	// Initialize repository according to user's choice.
	if opts.AutoInit {
		files, err := prepareRepoCommit(repo, opts)
		if err != nil {
			return errors.Newf("prepareRepoCommit: %v", err)
		}

		err = initRepoCommit(
			ctx,
			repoPath,
			files,
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
	saved, err := getRepositoryByID(tx, repo.ID)
	if err != nil {
		return errors.Newf("getRepositoryByID: %v", err)
	}

	if !opts.AutoInit {
		saved.IsEmpty = true
	}

	saved.DefaultBranch = conf.Repository.DefaultBranch
	if err = updateRepository(logger, tx, saved, false); err != nil {
		return errors.Newf("updateRepository: %v", err)
	}

	// The caller returns its copy to API clients, which must match what was saved.
	*repo = *saved
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

func createRepository(tx *gorm.DB, doer, owner *User, repo *Repository) (err error) {
	if err = isRepoNameAllowed(repo.Name); err != nil {
		return err
	}

	has, err := isRepositoryExist(tx, owner, repo.Name)
	if err != nil {
		return errors.Newf("IsRepositoryExist: %v", err)
	} else if has {
		return ErrRepoAlreadyExist{args: errx.Args{"ownerID": owner.ID, "name": repo.Name}}
	}

	// GORM replaces a false value with the column default (true) on create, so
	// restore the feature switches the caller chose, e.g., all disabled for forks.
	enableWiki, enableIssues, enablePulls := repo.EnableWiki, repo.EnableIssues, repo.EnablePulls
	if err = tx.Create(repo).Error; err != nil {
		return err
	}
	repo.EnableWiki, repo.EnableIssues, repo.EnablePulls = enableWiki, enableIssues, enablePulls
	err = tx.Model(repo).UpdateColumns(map[string]any{
		"enable_wiki":   enableWiki,
		"enable_issues": enableIssues,
		"enable_pulls":  enablePulls,
	}).Error
	if err != nil {
		return errors.Wrap(err, "restore feature switches")
	}

	err = tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos + 1 WHERE id = ?", "user"), owner.ID).Error
	if err != nil {
		return errors.Wrap(err, "increase owned repository count")
	}

	// Give access to all members in owner team.
	if owner.IsOrganization() {
		teams := newTeamsStore(tx)
		t, err := teams.GetOwnerTeam(context.TODO(), owner.ID)
		if err != nil {
			return errors.Newf("getOwnerTeam: %v", err)
		} else if err = teams.addRepo(context.TODO(), t, repo); err != nil {
			return errors.Newf("addRepository: %v", err)
		}
	} else {
		// Organization automatically called this in addRepository method.
		if err = newPermissionsStore(tx).recalculateAccesses(context.TODO(), repo); err != nil {
			return errors.Newf("recalculateAccesses: %v", err)
		}
	}

	if err = newReposStore(tx).watch(context.TODO(), owner.ID, repo.ID); err != nil {
		return errors.Newf("watchRepo: %v", err)
	}

	if err = newActionsStore(tx).NewRepo(context.TODO(), doer, owner, repo); err != nil {
		return errors.Newf("newRepoAction: %v", err)
	}

	return newReposStore(tx).LoadAttributes(context.TODO(), repo)
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
func CreateRepository(ctx context.Context, logger *logx.Logger, doer, owner *User, opts CreateRepoOptionsLegacy) (_ *Repository, err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	repoPath := RepoPath(owner.Name, opts.Name)
	if osx.Exist(repoPath) {
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
		// xorm inserts false for a zero bool instead of the column default.
		PullsAllowMerge: true,
	}

	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := createRepository(tx, doer, owner, repo); err != nil {
			return err
		}

		// No need for init mirror.
		if opts.IsMirror {
			return nil
		}
		if err := initRepository(ctx, logger, tx, repoPath, doer, repo, opts); err != nil {
			RemoveAllWithNotice(logger, "Delete repository for initialization failure", repoPath)
			return errors.Newf("initRepository: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Remember visibility preference
	err = Handle.Users().Update(ctx, owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	return repo, nil
}

func countRepositories(logger *logx.Logger, userID int64, private bool) int64 {
	query := Handle.db.Model(new(Repository)).Where("id > 0")
	if userID > 0 {
		query = query.Where("owner_id = ?", userID)
	}
	if !private {
		query = query.Where("is_private = ?", false)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		logger.Error("Failed to count repositories", "error", err)
	}
	return count
}

// CountRepositories returns number of repositories.
// Argument private only takes effect when it is false,
// set it true to count all repositories.
func CountRepositories(logger *logx.Logger, private bool) int64 {
	return countRepositories(logger, -1, private)
}

// CountUserRepositories returns number of repositories user owns.
// Argument private only takes effect when it is false,
// set it true to count all repositories.
func CountUserRepositories(logger *logx.Logger, userID int64, private bool) int64 {
	return countRepositories(logger, userID, private)
}

func Repositories(page, pageSize int) (_ []*Repository, err error) {
	repos := make([]*Repository, 0, pageSize)
	return repos, Handle.db.Order("id ASC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&repos).Error
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

	var validIDs []int64
	if err := Handle.db.Model(new(Repository)).
		Where("enable_issues = ? AND enable_external_tracker = ? AND id IN ?", true, false, repoIDs).
		Pluck("id", &validIDs).Error; err != nil {
		return nil, errors.Newf("filter valid repositories %v: %v", repoIDs, err)
	}

	if len(validIDs) == 0 {
		return nil, nil
	}
	return validIDs, nil
}

// RepoPath returns repository path by given user and repository name.
//
// Deprecated: Use repox.RepositoryPath instead.
func RepoPath(userName, repoName string) string {
	return repox.RepositoryPath(userName, repoName)
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
		return ErrRepoAlreadyExist{args: errx.Args{"ownerName": newOwnerName, "name": repo.Name}}
	}

	owner := repo.Owner
	return Handle.db.Transaction(func(tx *gorm.DB) error {
		return transferOwnership(tx, doer, owner, newOwner, repo)
	})
}

func transferOwnership(tx *gorm.DB, doer, owner, newOwner *User, repo *Repository) error {

	// Note: we have to set value here to make sure recalculate accesses is based on
	// new owner.
	repo.OwnerID = newOwner.ID
	repo.Owner = newOwner

	// Update repository.
	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err := tx.Model(repo).UpdateColumns(repo).Error; err != nil {
		return errors.Newf("update owner: %v", err)
	}

	// Remove redundant collaborators.
	collaborators, err := newCollaborationsStore(tx).List(context.TODO(), repo.ID)
	if err != nil {
		return errors.Newf("getCollaborators: %v", err)
	}

	for _, c := range collaborators {
		if c.ID == newOwner.ID || (newOwner.IsOrganization() && newOrganizationsStore(tx).IsMember(context.TODO(), newOwner.ID, c.ID)) {
			err = tx.Where(&Collaboration{RepoID: repo.ID, UserID: c.ID}).Delete(new(Collaboration)).Error
			if err != nil {
				return errors.Newf("remove collaborator '%d': %v", c.ID, err)
			}
		}
	}

	// Remove old team-repository relations.
	if owner.IsOrganization() {
		teams := newTeamsStore(tx)
		orgTeams, err := teams.ListByOrg(context.TODO(), owner.ID)
		if err != nil {
			return errors.Newf("getTeams: %v", err)
		}
		for _, t := range orgTeams {
			if !teams.HasRepo(context.TODO(), owner.ID, t.ID, repo.ID) {
				continue
			}

			t.NumRepos--
			if err := tx.Model(t).Select("*").Updates(t).Error; err != nil {
				return errors.Newf("decrease team repository count '%d': %v", t.ID, err)
			}
		}

		if err = newOrganizationsStore(tx).removeRepo(context.TODO(), owner.ID, repo.ID); err != nil {
			return errors.Newf("removeOrgRepo: %v", err)
		}
	}

	if newOwner.IsOrganization() {
		teams := newTeamsStore(tx)
		t, err := teams.GetOwnerTeam(context.TODO(), newOwner.ID)
		if err != nil {
			return errors.Newf("getOwnerTeam: %v", err)
		} else if err = teams.addRepo(context.TODO(), t, repo); err != nil {
			return errors.Newf("add to owner team: %v", err)
		}
	} else {
		// Organization called this in addRepository method.
		if err = newPermissionsStore(tx).recalculateAccesses(context.TODO(), repo); err != nil {
			return errors.Newf("recalculateAccesses: %v", err)
		}
	}

	// Update repository count.
	if err = tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos + 1 WHERE id = ?", "user"), newOwner.ID).Error; err != nil {
		return errors.Newf("increase new owner repository count: %v", err)
	} else if err = tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos - 1 WHERE id = ?", "user"), owner.ID).Error; err != nil {
		return errors.Newf("decrease old owner repository count: %v", err)
	}

	// Remove watch for organization.
	if owner.IsOrganization() {
		if err = newReposStore(tx).unwatch(context.TODO(), owner.ID, repo.ID); err != nil {
			return errors.Wrap(err, "unwatch repository for the organization owner")
		}
	}

	if err = newReposStore(tx).watch(context.TODO(), newOwner.ID, repo.ID); err != nil {
		return errors.Newf("watchRepo: %v", err)
	}

	if err = newActionsStore(tx).TransferRepo(context.TODO(), doer, owner, newOwner, repo); err != nil {
		return errors.Newf("transferRepoAction: %v", err)
	}

	// Rename remote repository to new path.
	if err = os.MkdirAll(repox.UserPath(newOwner.Name), os.ModePerm); err != nil {
		return err
	}
	if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
		return errors.Newf("rename repository directory: %v", err)
	}

	// Rename remote wiki repository to new path.
	wikiPath := WikiPath(owner.Name, repo.Name)
	if osx.Exist(wikiPath) {
		if err = os.Rename(wikiPath, WikiPath(newOwner.Name, repo.Name)); err != nil {
			return errors.Newf("rename repository wiki: %v", err)
		}
	}
	return nil
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
		return ErrRepoAlreadyExist{args: errx.Args{"ownerID": u.ID, "name": newRepoName}}
	}

	repo, err := Handle.Repositories().GetByName(context.TODO(), u.ID, oldRepoName)
	if err != nil {
		return errors.Wrap(err, "get repository")
	}
	repo.Owner = u

	// Change repository directory name
	if err = os.Rename(repo.RepoPath(), RepoPath(u.Name, newRepoName)); err != nil {
		return errors.Newf("rename repository directory: %v", err)
	}

	wikiPath := repo.WikiPath()
	if osx.Exist(wikiPath) {
		if err = os.Rename(wikiPath, WikiPath(u.Name, newRepoName)); err != nil {
			return errors.Newf("rename repository wiki: %v", err)
		}
	}
	return nil
}

func getRepositoriesByForkID(tx *gorm.DB, forkID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, tx.Where("fork_id = ?", forkID).Find(&repos).Error
}

// GetRepositoriesByForkID returns all repositories with given fork ID.
func GetRepositoriesByForkID(forkID int64) ([]*Repository, error) {
	return getRepositoriesByForkID(Handle.db, forkID)
}

func getNonMirrorRepositories(tx *gorm.DB) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, tx.Where("is_mirror = ?", false).Find(&repos).Error
}

// GetRepositoriesMirror returns only mirror repositories with user.
func GetNonMirrorRepositories() ([]*Repository, error) {
	return getNonMirrorRepositories(Handle.db)
}

func updateRepository(logger *logx.Logger, tx *gorm.DB, repo *Repository, visibilityChanged bool) (err error) {
	repo.LowerName = strings.ToLower(repo.Name)

	if len(repo.Description) > 512 {
		repo.Description = repo.Description[:512]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err = tx.Model(repo).Select("*").UpdateColumns(repo).Error; err != nil {
		return errors.Newf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(tx); err != nil {
			return errors.Newf("getOwner: %v", err)
		}
		if repo.Owner.IsOrganization() {
			// Organization repository need to recalculate access table when visibility is changed
			if err = newPermissionsStore(tx).recalculateTeamAccesses(context.TODO(), repo, 0); err != nil {
				return errors.Newf("recalculateTeamAccesses: %v", err)
			}
		}

		// Create/Remove git-daemon-export-ok for git-daemon
		daemonExportFile := path.Join(repo.RepoPath(), "git-daemon-export-ok")
		if repo.IsPrivate && osx.Exist(daemonExportFile) {
			if err = os.Remove(daemonExportFile); err != nil {
				logger.Error("Failed to remove Git daemon export file", "path", daemonExportFile, "error", err)
			}
		} else if !repo.IsPrivate && !osx.Exist(daemonExportFile) {
			if f, err := os.Create(daemonExportFile); err != nil {
				logger.Error("Failed to create Git daemon export file", "path", daemonExportFile, "error", err)
			} else {
				f.Close()
			}
		}

		forkRepos, err := getRepositoriesByForkID(tx, repo.ID)
		if err != nil {
			return errors.Newf("getRepositoriesByForkID: %v", err)
		}
		for i := range forkRepos {
			forkRepos[i].IsPrivate = repo.IsPrivate
			forkRepos[i].IsUnlisted = repo.IsUnlisted
			if err = updateRepository(logger, tx, forkRepos[i], true); err != nil {
				return errors.Newf("updateRepository[%d]: %v", forkRepos[i].ID, err)
			}
		}

		// Change visibility of generated actions
		err = tx.Model(new(Action)).Where("repo_id = ?", repo.ID).UpdateColumn("is_private", repo.IsPrivate || repo.IsUnlisted).Error
		if err != nil {
			return errors.Newf("change action visibility of repository: %v", err)
		}
	}

	return nil
}

func UpdateRepository(logger *logx.Logger, repo *Repository, visibilityChanged bool) (err error) {
	return Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := updateRepository(logger, tx, repo, visibilityChanged); err != nil {
			return errors.Newf("updateRepository: %v", err)
		}
		return nil
	})
}

// DeleteRepository deletes a repository for a user or organization.
func DeleteRepository(logger *logx.Logger, ownerID, repoID int64) error {
	repo := new(Repository)
	err := Handle.db.Where(&Repository{ID: repoID, OwnerID: ownerID}).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRepoNotExist{args: map[string]any{"ownerID": ownerID, "repoID": repoID}}
		}
		return err
	}

	// In case is a organization.
	org, err := Handle.Users().GetByID(context.TODO(), ownerID)
	if err != nil {
		return err
	}
	var teams []*Team
	if org.IsOrganization() {
		if teams, err = Handle.Teams().ListByOrg(context.TODO(), org.ID); err != nil {
			return err
		}
	}

	attachmentPaths := make([]string, 0, 25)
	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		if org.IsOrganization() {
			teamsStore := newTeamsStore(tx)
			for _, t := range teams {
				if !teamsStore.HasRepo(context.TODO(), org.ID, t.ID, repoID) {
					continue
				} else if err := teamsStore.removeRepo(context.TODO(), t, repo, false); err != nil {
					return err
				}
			}
		}

		if err := deleteBeans(tx,
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
			&Webhook{RepoID: repoID},
			&HookTask{RepoID: repoID},
			&LFSObject{RepoID: repoID},
		); err != nil {
			return errors.Newf("deleteBeans: %v", err)
		}

		// Delete comments and attachments.
		issues := make([]*Issue, 0, 25)
		if err := tx.Where("repo_id = ?", repoID).Find(&issues).Error; err != nil {
			return err
		}
		for i := range issues {
			if err := tx.Where("issue_id = ?", issues[i].ID).Delete(new(Comment)).Error; err != nil {
				return err
			}

			attachments := make([]*Attachment, 0, 5)
			if err := tx.Where("issue_id = ?", issues[i].ID).Find(&attachments).Error; err != nil {
				return err
			}
			for j := range attachments {
				attachmentPaths = append(attachmentPaths, attachments[j].LocalPath())
			}

			if err := tx.Where("issue_id = ?", issues[i].ID).Delete(new(Attachment)).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("repo_id = ?", repoID).Delete(new(Issue)).Error; err != nil {
			return err
		}

		if repo.IsFork {
			if err := tx.Exec("UPDATE repository SET num_forks = num_forks - 1 WHERE id = ?", repo.ForkID).Error; err != nil {
				return errors.Newf("decrease fork count: %v", err)
			}
		}

		return tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos - 1 WHERE id = ?", "user"), ownerID).Error
	})
	if err != nil {
		return err
	}

	// Remove repository files.
	repoPath := repo.RepoPath()
	RemoveAllWithNotice(logger, "Delete repository files", repoPath)

	repo.DeleteWiki(logger)

	// Remove attachment files.
	for i := range attachmentPaths {
		RemoveAllWithNotice(logger, "Delete attachment", attachmentPaths[i])
	}

	if repo.NumForks > 0 {
		if err = Handle.db.Exec("UPDATE repository SET fork_id = 0, is_fork = ? WHERE fork_id = ?", false, repo.ID).Error; err != nil {
			logger.Error("Failed to reset fork status of forks", "repoID", repo.ID, "error", err)
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

	repos := Handle.Repositories()
	repo, err := repos.GetByName(context.TODO(), user.ID, repoName)
	if err != nil {
		return nil, err
	}
	return repo, repos.LoadAttributes(context.TODO(), repo)
}

// getRepositoryByID returns the repository with given ID and its attributes.
//
// TODO: Remove once its callers are moved into stores.
func getRepositoryByID(tx *gorm.DB, id int64) (*Repository, error) {
	repos := newReposStore(tx)
	repo, err := repos.GetByID(context.TODO(), id)
	if err != nil {
		return nil, err
	}
	return repo, repos.LoadAttributes(context.TODO(), repo)
}

type UserRepoOptions struct {
	UserID   int64
	Private  bool
	Page     int
	PageSize int
}

// GetUserRepositories returns a list of repositories of given user.
func GetUserRepositories(opts *UserRepoOptions) ([]*Repository, error) {
	query := Handle.db.Where("owner_id = ?", opts.UserID).Order("updated_unix DESC")
	if !opts.Private {
		query = query.Where("is_private = ? AND is_unlisted = ?", false, false)
	}

	if opts.Page <= 0 {
		opts.Page = 1
	}
	// NOTE: A non-positive page size means no limit, e.g., callers pass the
	// cached number of repositories which can be zero.
	if opts.PageSize > 0 {
		query = query.Limit(opts.PageSize).Offset((opts.Page - 1) * opts.PageSize)
	}

	repos := make([]*Repository, 0, opts.PageSize)
	return repos, query.Find(&repos).Error
}

// GetUserRepositories returns a list of mirror repositories of given user.
func GetUserMirrorRepositories(userID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, Handle.db.Where("owner_id = ? AND is_mirror = ?", userID, true).Find(&repos).Error
}

// GetRecentUpdatedRepositories returns the list of repositories that are recently updated.
func GetRecentUpdatedRepositories(page, pageSize int) (repos []*Repository, err error) {
	return repos, Handle.db.Where("is_private = ?", false).
		Order("updated_unix DESC").
		Limit(pageSize).Offset((page - 1) * pageSize).
		Find(&repos).Error
}

// GetUserAndCollaborativeRepositories returns list of repositories the user owns and collaborates.
func GetUserAndCollaborativeRepositories(userID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	if err := Handle.db.Table("repository AS repo").
		Select("repo.*").
		Joins("INNER JOIN collaboration ON collaboration.repo_id = repo.id").
		Where("collaboration.user_id = ?", userID).
		Find(&repos).Error; err != nil {
		return nil, errors.Newf("select collaborative repositories: %v", err)
	}

	ownRepos := make([]*Repository, 0, 10)
	if err := Handle.db.Where("owner_id = ?", userID).Find(&ownRepos).Error; err != nil {
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
	query := Handle.db.Table("repository AS repo")
	// Attempt to find repositories that opts.UserID has access to,
	// this does not include other people's private repositories even if opts.UserID is an admin.
	if !opts.Private && opts.UserID > 0 {
		query = query.Joins("LEFT JOIN access ON access.repo_id = repo.id").
			Where("(repo.owner_id = ? OR access.user_id = ? OR (repo.is_private = ? AND repo.is_unlisted = ?) OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?)))", opts.UserID, opts.UserID, false, false, true, true, true)
	} else {
		// Only return public repositories if opts.Private is not set
		if !opts.Private {
			query = query.Where("((repo.is_private = ? AND repo.is_unlisted = ?) OR (repo.is_private = ? AND (repo.allow_public_wiki = ? OR repo.allow_public_issues = ?)))", false, false, true, true, true)
		}
	}
	if len(opts.Keyword) > 0 {
		query = query.Where("(repo.lower_name LIKE ? OR repo.description LIKE ?)", "%"+strings.ToLower(opts.Keyword)+"%", "%"+strings.ToLower(opts.Keyword)+"%")
	}
	if opts.OwnerID > 0 {
		query = query.Where("repo.owner_id = ?", opts.OwnerID)
	}

	// We need all fields (repo.*) in final list but only ID (repo.id) is good enough for counting.
	if err = query.Session(&gorm.Session{}).Distinct("repo.id").Count(&count).Error; err != nil {
		return nil, 0, errors.Newf("Count: %v", err)
	}

	if len(opts.OrderBy) > 0 {
		query = query.Order("repo." + opts.OrderBy)
	}
	return repos, count, query.Distinct("repo.*").
		Limit(opts.PageSize).Offset((opts.Page - 1) * opts.PageSize).
		Find(&repos).Error
}

// iterateRepositories calls fn for every repository in batches, and stops at the
// first error.
func iterateRepositories(fn func(repo *Repository) error) error {
	var repos []*Repository
	return Handle.db.Where("id > 0").FindInBatches(&repos, 100, func(*gorm.DB, int) error {
		for _, repo := range repos {
			if err := fn(repo); err != nil {
				return err
			}
		}
		return nil
	}).Error
}

func DeleteOldRepositoryArchives(logger *logx.Logger) {
	if taskStatusTable.IsRunning(taskNameCleanOldArchives) {
		return
	}
	taskStatusTable.Start(taskNameCleanOldArchives)
	defer taskStatusTable.Stop(taskNameCleanOldArchives)

	logger.Debug("Deleting old repository archives")

	formats := []string{"zip", "targz"}
	oldestTime := time.Now().Add(-conf.Cron.RepoArchiveCleanup.OlderThan)
	if err := iterateRepositories(func(repo *Repository) error {
		basePath := filepath.Join(repo.RepoPath(), "archives")
		for _, format := range formats {
			dirPath := filepath.Join(basePath, format)
			if !osx.IsDir(dirPath) {
				continue
			}

			dir, err := os.Open(dirPath)
			if err != nil {
				logger.Error("Failed to open directory", "dirPath", dirPath, "error", err)
				continue
			}

			fis, err := dir.Readdir(0)
			dir.Close()
			if err != nil {
				logger.Error("Failed to read directory", "dirPath", dirPath, "error", err)
				continue
			}

			for _, fi := range fis {
				if fi.IsDir() || fi.ModTime().After(oldestTime) {
					continue
				}

				archivePath := filepath.Join(dirPath, fi.Name())
				if err = os.Remove(archivePath); err != nil {
					const fmtStr = "Failed to health delete archive %q: %v"
					logger.Warn("Failed to delete old repository archive", "archivePath", archivePath, "error", err)
					if err = Handle.Notices().Create(
						context.TODO(),
						NoticeTypeRepository,
						fmt.Sprintf(fmtStr, archivePath, err),
					); err != nil {
						logger.Error("Failed to create repository notice", "error", err)
					}
				}
			}
		}

		return nil
	}); err != nil {
		logger.Error("Failed to delete old repository archives", "error", err)
	}
}

// DeleteRepositoryArchives deletes all repositories' archives.
func DeleteRepositoryArchives() error {
	if taskStatusTable.IsRunning(taskNameCleanOldArchives) {
		return nil
	}
	taskStatusTable.Start(taskNameCleanOldArchives)
	defer taskStatusTable.Stop(taskNameCleanOldArchives)

	return iterateRepositories(func(repo *Repository) error {
		return os.RemoveAll(filepath.Join(repo.RepoPath(), "archives"))
	})
}

func gatherMissingRepoRecords() ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	if err := iterateRepositories(func(repo *Repository) error {
		if !osx.IsDir(repo.RepoPath()) {
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
func DeleteMissingRepositories(logger *logx.Logger) error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return errors.Newf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		logger.Debug("Deleting missing repository", "ownerID", repo.OwnerID, "repoID", repo.ID)
		if err := DeleteRepository(logger, repo.OwnerID, repo.ID); err != nil {
			if err2 := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, fmt.Sprintf("DeleteRepository [%d]: %v", repo.ID, err)); err2 != nil {
				return errors.Newf("CreateRepositoryNotice: %v", err)
			}
		}
	}
	return nil
}

// ReinitMissingRepositories reinitializes all repository records that lost Git files.
func ReinitMissingRepositories(logger *logx.Logger) error {
	repos, err := gatherMissingRepoRecords()
	if err != nil {
		return errors.Newf("gatherMissingRepoRecords: %v", err)
	}

	if len(repos) == 0 {
		return nil
	}

	for _, repo := range repos {
		logger.Debug("Reinitializing missing repository", "ownerID", repo.OwnerID, "repoID", repo.ID)
		if err := git.Init(context.TODO(), repo.RepoPath(), git.InitOptions{Bare: true}); err != nil {
			if err2 := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, fmt.Sprintf("init repository [repo_id: %d]: %v", repo.ID, err)); err2 != nil {
				return errors.Newf("create repository notice: %v", err)
			}
		}
	}
	return nil
}

// SyncRepositoryHooks rewrites the delegate hooks of all repositories to make
// sure the binary and custom conf path are up-to-date, and that every repository
// has only the delegate hooks it needs.
func SyncRepositoryHooks() error {
	return iterateRepositories(func(repo *Repository) error {
		return repo.SyncDelegateHooks()
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
func GitFsck(logger *logx.Logger) {
	if taskStatusTable.IsRunning(taskNameGitFSCK) {
		return
	}
	taskStatusTable.Start(taskNameGitFSCK)
	defer taskStatusTable.Stop(taskNameGitFSCK)

	logger.Debug("Running health checks on repositories")

	if err := iterateRepositories(func(repo *Repository) error {
		repoPath := repo.RepoPath()
		ctx, cancel := git.WithTimeout(context.TODO(), conf.Cron.RepoHealthCheck.Timeout)
		err := git.RepoFsck(ctx, repoPath, git.FsckOptions{
			Args: conf.Cron.RepoHealthCheck.Args,
		})
		cancel()
		if err != nil {
			const fmtStr = "Failed to perform health check on repository %q: %v"
			logger.Warn("Failed to perform health check on repository", "repoPath", repoPath, "error", err)
			if err = Handle.Notices().Create(
				context.TODO(),
				NoticeTypeRepository,
				fmt.Sprintf(fmtStr, repoPath, err),
			); err != nil {
				logger.Error("Failed to create repository notice", "error", err)
			}
		}
		return nil
	}); err != nil {
		logger.Error("Failed to run health checks on repositories", "error", err)
	}
}

func GitGcRepos() error {
	return iterateRepositories(func(repo *Repository) error {
		if err := repo.GetOwner(); err != nil {
			return err
		}
		ctx, cancel := git.WithTimeout(context.TODO(), time.Duration(conf.Git.Timeout.GC)*time.Second)
		defer cancel()
		return git.RepoGC(ctx, RepoPath(repo.Owner.Name, repo.Name), git.GCOptions{Args: conf.Git.GCArgs})
	})
}

type repoChecker struct {
	querySQL, correctSQL string
	desc                 string
}

func repoStatsCheck(logger *logx.Logger, checker *repoChecker) {
	var ids []int64
	if err := Handle.db.Raw(checker.querySQL).Scan(&ids).Error; err != nil {
		logger.Error("Failed to find inconsistent stats", "check", checker.desc, "error", err)
		return
	}
	for _, id := range ids {
		logger.Debug("Correcting inconsistent stats", "check", checker.desc, "id", id)
		if err := Handle.db.Exec(checker.correctSQL, id, id).Error; err != nil {
			logger.Error("Failed to correct inconsistent stats", "check", checker.desc, "id", id, "error", err)
		}
	}
}

func CheckRepoStats(logger *logx.Logger) {
	if taskStatusTable.IsRunning(taskNameCheckRepoStats) {
		return
	}
	taskStatusTable.Start(taskNameCheckRepoStats)
	defer taskStatusTable.Stop(taskNameCheckRepoStats)

	logger.Debug("Checking repository stats")

	checkers := []*repoChecker{
		// Repository.NumWatches
		{
			"SELECT repo.id FROM repository repo WHERE repo.num_watches != (SELECT COUNT(*) FROM watch WHERE repo_id = repo.id)",
			"UPDATE repository SET num_watches = (SELECT COUNT(*) FROM watch WHERE repo_id = ?) WHERE id = ?",
			"repository count 'num_watches'",
		},
		// Repository.NumStars
		{
			"SELECT repo.id FROM repository repo WHERE repo.num_stars != (SELECT COUNT(*) FROM star WHERE repo_id = repo.id)",
			"UPDATE repository SET num_stars = (SELECT COUNT(*) FROM star WHERE repo_id = ?) WHERE id = ?",
			"repository count 'num_stars'",
		},
		// Label.NumIssues
		{
			"SELECT label.id FROM label WHERE label.num_issues != (SELECT COUNT(*) FROM issue_label WHERE label_id = label.id)",
			"UPDATE label SET num_issues = (SELECT COUNT(*) FROM issue_label WHERE label_id = ?) WHERE id = ?",
			"label count 'num_issues'",
		},
		// User.NumRepos
		{
			dbx.Quote("SELECT %[1]s.id FROM %[1]s WHERE %[1]s.num_repos != (SELECT COUNT(*) FROM repository WHERE owner_id = %[1]s.id)", "user"),
			dbx.Quote("UPDATE %s SET num_repos = (SELECT COUNT(*) FROM repository WHERE owner_id = ?) WHERE id = ?", "user"),
			"user count 'num_repos'",
		},
		// Issue.NumComments
		{
			"SELECT issue.id FROM issue WHERE issue.num_comments != (SELECT COUNT(*) FROM comment WHERE issue_id = issue.id AND type = 0)",
			"UPDATE issue SET num_comments = (SELECT COUNT(*) FROM comment WHERE issue_id = ? AND type = 0) WHERE id = ?",
			"issue count 'num_comments'",
		},
	}
	for i := range checkers {
		repoStatsCheck(logger, checkers[i])
	}

	// ***** START: Repository.NumClosedIssues *****
	desc := "repository count 'num_closed_issues'"
	var ids []int64
	err := Handle.db.Raw("SELECT repo.id FROM repository repo WHERE repo.num_closed_issues != (SELECT COUNT(*) FROM issue WHERE repo_id = repo.id AND is_closed = ? AND is_pull = ?)", true, false).Scan(&ids).Error
	if err != nil {
		logger.Error("Failed to find inconsistent stats", "check", desc, "error", err)
	} else {
		for _, id := range ids {
			logger.Debug("Correcting inconsistent stats", "check", desc, "id", id)
			err = Handle.db.Exec("UPDATE repository SET num_closed_issues = (SELECT COUNT(*) FROM issue WHERE repo_id = ? AND is_closed = ? AND is_pull = ?) WHERE id = ?", id, true, false, id).Error
			if err != nil {
				logger.Error("Failed to correct inconsistent stats", "check", desc, "id", id, "error", err)
			}
		}
	}
	// ***** END: Repository.NumClosedIssues *****

	// FIXME: use checker when stop supporting old fork repo format.
	// ***** START: Repository.NumForks *****
	ids = nil
	err = Handle.db.Raw("SELECT repo.id FROM repository repo WHERE repo.num_forks != (SELECT COUNT(*) FROM repository WHERE fork_id = repo.id)").Scan(&ids).Error
	if err != nil {
		logger.Error("Failed to find inconsistent stats", "check", "repository count 'num_forks'", "error", err)
	} else {
		for _, id := range ids {
			logger.Debug("Correcting inconsistent stats", "check", "repository count 'num_forks'", "id", id)

			repo, err := getRepositoryByID(Handle.db, id)
			if err != nil {
				logger.Error("Failed to get repository", "repoID", id, "error", err)
				continue
			}

			var numForks int64
			if err = Handle.db.Model(new(Repository)).Where("fork_id = ?", repo.ID).Count(&numForks).Error; err != nil {
				logger.Error("Failed to count forks", "repoID", repo.ID, "error", err)
				continue
			}
			repo.NumForks = int(numForks)

			if err = UpdateRepository(logger, repo, false); err != nil {
				logger.Error("Failed to update repository", "repoID", id, "error", err)
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
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"uniqueIndex:UQE_watch_watch"`
	RepoID int64 `gorm:"uniqueIndex:UQE_watch_watch"`
}

//   _________ __
//  /   _____//  |______ _______
//  \_____  \\   __\__  \\_  __ \
//  /        \|  |  / __ \|  | \/
// /_______  /|__| (____  /__|
//         \/           \/

type Star struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"column:uid;uniqueIndex:UQE_star_s"`
	RepoID int64 `gorm:"uniqueIndex:UQE_star_s"`
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
	err := Handle.db.Where("owner_id = ? AND fork_id = ?", ownerID, repoID).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return repo, true, newReposStore(Handle.db).LoadAttributes(context.TODO(), repo)
}

// ForkRepository creates a fork of target repository under another user domain.
func ForkRepository(ctx context.Context, logger *logx.Logger, doer, owner *User, baseRepo *Repository, name, desc string) (_ *Repository, err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

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
		// xorm inserts false for a zero bool instead of the column default.
		PullsAllowMerge: true,
	}

	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := createRepository(tx, doer, owner, repo); err != nil {
			return err
		} else if err = tx.Exec("UPDATE repository SET num_forks = num_forks + 1 WHERE id = ?", baseRepo.ID).Error; err != nil {
			return err
		}

		repoPath := repo.repoPath(tx)
		RemoveAllWithNotice(logger, "Repository path erase before creation", repoPath)

		cloneCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := git.Clone(cloneCtx, baseRepo.RepoPath(), repoPath, git.CloneOptions{Bare: true}); err != nil {
			RemoveAllWithNotice(logger, "Delete fork repository for clone failure", repoPath)
			return errors.Newf("git clone: %v", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Remember visibility preference
	err = Handle.Users().Update(ctx, owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	if err = repo.UpdateSize(ctx); err != nil {
		logger.Error("Failed to update repository size", "repoID", repo.ID, "error", err)
	}
	if err = Handle.Webhooks().Prepare(ctx, logger, baseRepo, HookEventTypeFork, &apiv1types.WebhookForkPayload{
		Forkee: repo.APIFormatLegacy(logger, nil),
		Repo:   baseRepo.APIFormatLegacy(logger, nil),
		Sender: doer.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks", "repoID", baseRepo.ID, "error", err)
	}
	return repo, nil
}

func (r *Repository) GetForks() ([]*Repository, error) {
	forks := make([]*Repository, 0, r.NumForks)
	if err := Handle.db.Where("fork_id = ?", r.ID).Find(&forks).Error; err != nil {
		return nil, err
	}

	for _, fork := range forks {
		fork.BaseRepo = r
	}
	return forks, nil
}

// TODO: Remove once the repository statistics are moved into a store.
func parseCountResult(results []map[string][]byte) int64 {
	if len(results) == 0 {
		return 0
	}
	for _, result := range results[0] {
		count, _ := strconv.ParseInt(string(result), 10, 64)
		return count
	}
	return 0
}

// deleteBeans deletes all given beans, beans should contain delete conditions.
func deleteBeans(tx *gorm.DB, beans ...any) (err error) {
	for i := range beans {
		if err = tx.Where(beans[i]).Delete(beans[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
