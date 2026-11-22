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
	"gorm.io/gorm/clause"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	embedConf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/semverx"
	"gogs.io/gogs/internal/strx"
	"gogs.io/gogs/internal/sync"
	"gogs.io/gogs/internal/urlx"
)

// RepoAvatarURLPrefix is the path prefix to repository avatars. It is under
// "-", a reserved username, so it never shadows the pages of a user.
const RepoAvatarURLPrefix = "-/repo-avatars"

// InvalidRepoReference represents an error when repository reference is invalid.
type InvalidRepoReference struct {
	Ref string
}

func (err InvalidRepoReference) Error() string {
	return fmt.Sprintf("invalid repository reference [ref: %s]", err.Ref)
}

var repoWorkingPool = sync.NewExclusivePool()

var (
	Gitignores, Licenses, Readmes, LabelTemplates []string

	// Maximum items per page in forks, watchers and stars of a repo.
	ItemsPerPage = 40
)

func LoadRepoConfig(logger *logx.Logger) {
	// Load .gitignore, license, readme and label template files.
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

	// Set some Git configurations.
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

	IsPrivate  bool
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

// IsPartialPublic returns true if repository is public or allows public access to wiki or issues.
func (r *Repository) IsPartialPublic() bool {
	return !r.IsPrivate || r.AllowPublicWiki || r.AllowPublicIssues
}

func (r *Repository) CanGuestViewWiki() bool {
	return r.EnableWiki && !r.EnableExternalWiki && r.AllowPublicWiki
}

func (r *Repository) CanGuestViewIssues() bool {
	return r.EnableIssues && !r.EnableExternalTracker && r.AllowPublicIssues
}

// ExternalTrackerLink returns the URL of the external issue tracker when the
// repository uses one, and an empty string otherwise or when the URL is not an
// HTTP or HTTPS URL.
func (r *Repository) ExternalTrackerLink() string {
	if r.EnableIssues && r.EnableExternalTracker && urlx.IsHTTP(r.ExternalTrackerURL) {
		return r.ExternalTrackerURL
	}
	return ""
}

// ExternalWikiLink returns the URL of the external wiki when the repository
// uses one, and an empty string otherwise or when the URL is not an HTTP or
// HTTPS URL.
func (r *Repository) ExternalWikiLink() string {
	if r.EnableWiki && r.EnableExternalWiki && urlx.IsHTTP(r.ExternalWikiURL) {
		return r.ExternalWikiURL
	}
	return ""
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
// which includes app sub-url as prefix, or an empty string when the
// repository has no custom avatar.
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
	return Handle.Repositories().Update(context.TODO(), logger, r, false)
}

// APIFormatLegacy returns the API format of the repository. It assumes the
// following fields have been assigned with valid values:
// Required - BaseRepo (if fork)
// Arguments that are allowed to be nil: permission
//
// Deprecated: Use APIFormat instead.
func (r *Repository) APIFormatLegacy(logger *logx.Logger, permission *v1types.RepositoryPermission, user ...*User) *v1types.Repository {
	cloneLink := r.CloneLink()
	apiRepo := &v1types.Repository{
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
		p := &v1types.RepositoryPermission{Pull: true}
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

// ComposeMetas composes a map of metas for rendering commit URL and external issue tracker URL.
func (r *Repository) ComposeMetas() map[string]string {
	if r.ExternalMetas != nil {
		return r.ExternalMetas
	}

	r.ExternalMetas = map[string]string{
		"repoLink": r.Link(),
		"repoPath": r.RepoPath(),
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

// AllowsPulls returns true if repository meets the requirements of accepting pulls and has them enabled.
func (r *Repository) AllowsPulls() bool {
	return r.CanEnablePulls() && r.EnablePulls
}

// CanEnableEditor returns true if repository meets the requirements of web editor.
func (r *Repository) CanEnableEditor() bool {
	return !r.IsMirror
}

// PatchPath returns corresponding patch file path of repository by given issue index.
func (r *Repository) PatchPath(index int64) (string, error) {
	if err := r.GetOwner(); err != nil {
		return "", err
	}

	return filepath.Join(RepoPath(r.Owner.Name, r.Name), "pulls", strconv.FormatInt(index, 10)+".patch"), nil
}

// SavePatch saves patch data to corresponding location by given issue index.
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

// ForksMayAllowPublicWiki reports whether a private fork of the repository may
// allow public access to its wiki without being more visible than the
// repository. Explore lists private repositories that allow public access even
// when they are unlisted, so forks of an unlisted public repository may not.
func (r *Repository) ForksMayAllowPublicWiki() bool {
	return (!r.IsPrivate && !r.IsUnlisted) || (r.IsPrivate && r.AllowPublicWiki)
}

// ForksMayAllowPublicIssues is like ForksMayAllowPublicWiki for issues.
func (r *Repository) ForksMayAllowPublicIssues() bool {
	return (!r.IsPrivate && !r.IsUnlisted) || (r.IsPrivate && r.AllowPublicIssues)
}

// limitVisibilityTo makes the fork at most as visible as its base repository,
// i.e., private when the base is private, unlisted when the base is unlisted,
// and without public access to its wiki or issues unless the base allows it.
func (r *Repository) limitVisibilityTo(base *Repository) {
	r.IsPrivate = r.IsPrivate || base.IsPrivate
	r.IsUnlisted = r.IsUnlisted || base.IsUnlisted
	r.AllowPublicWiki = r.AllowPublicWiki && base.ForksMayAllowPublicWiki()
	r.AllowPublicIssues = r.AllowPublicIssues && base.ForksMayAllowPublicIssues()
}

// BeforeCreate implements the GORM create hook.
func (r *Repository) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
	}
	if r.UpdatedUnix == 0 {
		r.UpdatedUnix = r.CreatedUnix
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (r *Repository) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (r *Repository) AfterFind(_ *gorm.DB) error {
	if r.ExternalTrackerStyle == "" {
		r.ExternalTrackerStyle = markup.IssueNameStyleNumeric
	}
	r.NumOpenIssues = r.NumIssues - r.NumClosedIssues
	r.NumOpenPulls = r.NumPulls - r.NumClosedPulls
	r.NumOpenMilestones = r.NumMilestones - r.NumClosedMilestones
	r.Created = time.Unix(r.CreatedUnix, 0).Local()
	r.Updated = time.Unix(r.UpdatedUnix, 0).Local()
	return nil
}

type RepositoryAPIFormatOptions struct {
	Permission *v1types.RepositoryPermission
	Parent     *v1types.Repository
}

// APIFormat returns the API format of a repository.
func (r *Repository) APIFormat(owner *User, opts ...RepositoryAPIFormatOptions) *v1types.Repository {
	var opt RepositoryAPIFormatOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cloneLink := repox.NewCloneLink(owner.Name, r.Name, false)
	return &v1types.Repository{
		ID:            r.ID,
		Owner:         owner.APIFormat(),
		Name:          r.Name,
		FullName:      owner.Name + "/" + r.Name,
		Description:   r.Description,
		Private:       r.IsPrivate,
		Fork:          r.IsFork,
		Parent:        opt.Parent,
		Empty:         r.IsEmpty,
		Mirror:        r.IsMirror,
		Size:          r.Size,
		HTMLURL:       repox.HTMLURL(owner.Name, r.Name),
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
		Permissions:   opt.Permission,
	}
}

// RepositoriesStore is the storage layer for repositories.
type RepositoriesStore struct {
	db *gorm.DB
}

func newReposStore(db *gorm.DB) *RepositoriesStore {
	return &RepositoriesStore{db: db}
}

type ErrRepoAlreadyExist struct {
	args errx.Args
}

func IsErrRepoAlreadyExist(err error) bool {
	_, ok := err.(ErrRepoAlreadyExist)
	return ok
}

func (err ErrRepoAlreadyExist) Error() string {
	return fmt.Sprintf("repository already exists: %v", err.args)
}

type CreateRepoOptions struct {
	Name          string
	Description   string
	DefaultBranch string
	Private       bool
	Mirror        bool
	EnableWiki    bool
	EnableIssues  bool
	EnablePulls   bool
	Fork          bool
	ForkID        int64
}

// create creates a repository record without its Git repository, accesses, or
// action, which tests use to set up repositories. It returns
// ErrNameNotAllowed when the repository name is not allowed, or
// ErrRepoAlreadyExist when a repository with same name already exists for the
// owner.
func (s *RepositoriesStore) create(ctx context.Context, ownerID int64, opts CreateRepoOptions) (*Repository, error) {
	err := isRepoNameAllowed(opts.Name)
	if err != nil {
		return nil, err
	}

	_, err = s.GetByName(ctx, ownerID, opts.Name)
	if err == nil {
		return nil, ErrRepoAlreadyExist{
			args: errx.Args{
				"ownerID": ownerID,
				"name":    opts.Name,
			},
		}
	} else if !IsErrRepoNotExist(err) {
		return nil, err
	}

	repo := &Repository{
		OwnerID:         ownerID,
		LowerName:       strings.ToLower(opts.Name),
		Name:            opts.Name,
		Description:     opts.Description,
		DefaultBranch:   opts.DefaultBranch,
		IsPrivate:       opts.Private,
		IsMirror:        opts.Mirror,
		EnableWiki:      opts.EnableWiki,
		EnableIssues:    opts.EnableIssues,
		EnablePulls:     opts.EnablePulls,
		IsFork:          opts.Fork,
		ForkID:          opts.ForkID,
		PullsAllowMerge: true,
	}
	return repo, s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err = tx.Create(repo).Error
		if err != nil {
			return errors.Wrap(err, "create")
		}

		err = newReposStore(tx).Watch(ctx, ownerID, repo.ID)
		if err != nil {
			return errors.Wrap(err, "watch")
		}
		return nil
	})
}

// GetByCollaboratorID returns a list of repositories that the given
// collaborator has access to. Results are limited to the given limit and sorted
// by the given order (e.g., "updated_unix DESC"). Repositories that are owned
// directly by the given collaborator are not included.
func (s *RepositoriesStore) GetByCollaboratorID(ctx context.Context, collaboratorID int64, limit int, orderBy string) ([]*Repository, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM repository
		JOIN access ON access.repo_id = repository.id AND access.user_id = @collaboratorID
		WHERE access.mode >= @accessModeRead
		ORDER BY @orderBy
		LIMIT @limit
	*/
	var repos []*Repository
	return repos, s.db.WithContext(ctx).
		Joins("JOIN access ON access.repo_id = repository.id AND access.user_id = ?", collaboratorID).
		Where("access.mode >= ?", AccessModeRead).
		Order(orderBy).
		Limit(limit).
		Find(&repos).
		Error
}

// GetByCollaboratorIDWithAccessMode returns a list of repositories and
// corresponding access mode that the given collaborator has access to.
// Repositories that are owned directly by the given collaborator are not
// included.
func (s *RepositoriesStore) GetByCollaboratorIDWithAccessMode(ctx context.Context, collaboratorID int64) (map[*Repository]AccessMode, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT
			repository.*,
			access.mode
		FROM repository
		JOIN access ON access.repo_id = repository.id AND access.user_id = @collaboratorID
		WHERE access.mode >= @accessModeRead
	*/
	var reposWithAccessMode []*struct {
		*Repository
		Mode AccessMode
	}
	err := s.db.WithContext(ctx).
		Select("repository.*", "access.mode").
		Table("repository").
		Joins("JOIN access ON access.repo_id = repository.id AND access.user_id = ?", collaboratorID).
		Where("access.mode >= ?", AccessModeRead).
		Find(&reposWithAccessMode).
		Error
	if err != nil {
		return nil, err
	}

	repos := make(map[*Repository]AccessMode, len(reposWithAccessMode))
	for _, repoWithAccessMode := range reposWithAccessMode {
		repos[repoWithAccessMode.Repository] = repoWithAccessMode.Mode
	}
	return repos, nil
}

var _ errx.NotFound = (*ErrRepoNotExist)(nil)

type ErrRepoNotExist struct {
	args errx.Args
}

func IsErrRepoNotExist(err error) bool {
	_, ok := err.(ErrRepoNotExist)
	return ok
}

func (err ErrRepoNotExist) Error() string {
	return fmt.Sprintf("repository does not exist: %v", err.args)
}

func (ErrRepoNotExist) NotFound() bool {
	return true
}

// GetByID returns the repository with given ID. It returns ErrRepoNotExist when
// not found.
func (s *RepositoriesStore) GetByID(ctx context.Context, id int64) (*Repository, error) {
	repo := new(Repository)
	err := s.db.WithContext(ctx).Where("id = ?", id).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRepoNotExist{errx.Args{"repoID": id}}
		}
		return nil, err
	}
	return repo, nil
}

// LoadAttributes loads the owners of given repositories, and the repositories
// they are forked from with their owners, up to the root of each fork chain. A
// fork whose base repository is gone is marked as not being a fork. Owners and
// base repositories that are already set are kept. It returns ErrUserNotExist
// when an owner is gone.
func (s *RepositoriesStore) LoadAttributes(ctx context.Context, repos ...*Repository) error {
	db := s.db.WithContext(ctx)

	loaded := make(map[int64]*Repository, len(repos))
	for _, repo := range repos {
		loaded[repo.ID] = repo
	}
	for pending := repos; len(pending) > 0; {
		ownerIDs := make([]int64, 0, len(pending))
		for _, repo := range pending {
			if repo.Owner == nil {
				ownerIDs = append(ownerIDs, repo.OwnerID)
			}
		}
		if len(ownerIDs) > 0 {
			var owners []*User
			err := db.Where("id IN ?", ownerIDs).Find(&owners).Error
			if err != nil {
				return errors.Wrap(err, "list owners")
			}
			ownersByID := make(map[int64]*User, len(owners))
			for _, owner := range owners {
				ownersByID[owner.ID] = owner
			}
			for _, repo := range pending {
				if repo.Owner != nil {
					continue
				}
				repo.Owner = ownersByID[repo.OwnerID]
				if repo.Owner == nil {
					return ErrUserNotExist{args: errx.Args{"userID": repo.OwnerID}}
				}
			}
		}

		var baseIDs []int64
		for _, repo := range pending {
			if repo.IsFork && repo.BaseRepo == nil && loaded[repo.ForkID] == nil {
				baseIDs = append(baseIDs, repo.ForkID)
			}
		}
		var bases []*Repository
		if len(baseIDs) > 0 {
			err := db.Where("id IN ?", baseIDs).Find(&bases).Error
			if err != nil {
				return errors.Wrap(err, "list base repositories")
			}
			for _, base := range bases {
				loaded[base.ID] = base
			}
		}
		for _, repo := range pending {
			if !repo.IsFork || repo.BaseRepo != nil {
				continue
			}
			repo.BaseRepo = loaded[repo.ForkID]
			if repo.BaseRepo == nil {
				repo.IsFork = false
				repo.ForkID = 0
			}
		}
		pending = bases
	}
	return nil
}

// GetByName returns the repository with given owner and name. It returns
// ErrRepoNotExist when not found.
func (s *RepositoriesStore) GetByName(ctx context.Context, ownerID int64, name string) (*Repository, error) {
	repo := new(Repository)
	err := s.db.WithContext(ctx).
		Where("owner_id = ? AND lower_name = ?", ownerID, strings.ToLower(name)).
		First(repo).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRepoNotExist{
				args: errx.Args{
					"ownerID": ownerID,
					"name":    name,
				},
			}
		}
		return nil, err
	}
	return repo, nil
}

// addStars adds delta to the numbers of stars of the repository and of the
// user. Adding is atomic, unlike counting the rows of the table, which drifts
// under concurrent changes and deadlocks on MySQL.
func (*RepositoriesStore) addStars(tx *gorm.DB, userID, repoID int64, delta int) error {
	err := tx.Model(&Repository{}).Where("id = ?", repoID).UpdateColumn("num_stars", gorm.Expr("num_stars + ?", delta)).Error
	if err != nil {
		return errors.Wrap(err, `update "repository.num_stars"`)
	}
	err = tx.Model(&User{}).Where("id = ?", userID).UpdateColumn("num_stars", gorm.Expr("num_stars + ?", delta)).Error
	if err != nil {
		return errors.Wrap(err, `update "user.num_stars"`)
	}
	return nil
}

// Star marks the user to star the repository.
//
// Its statements run without a transaction, so each holds its lock only
// briefly and cannot deadlock. A crash in between leaves a count off by one,
// which CheckRepoStats corrects.
func (s *RepositoriesStore) Star(ctx context.Context, userID, repoID int64) error {
	db := s.db.WithContext(ctx)
	star := &Star{
		UserID: userID,
		RepoID: repoID,
	}
	result := db.FirstOrCreate(star, star)
	if result.Error != nil {
		return errors.Wrap(result.Error, "upsert")
	} else if result.RowsAffected <= 0 {
		return nil // Relation already exists
	}

	return s.addStars(db, userID, repoID, 1)
}

// Unstar removes the user's star from the repository. It is a no-op if the user
// has not starred it. Like Star, it runs without a transaction.
func (s *RepositoriesStore) Unstar(ctx context.Context, userID, repoID int64) error {
	db := s.db.WithContext(ctx)
	result := db.Where("uid = ? AND repo_id = ?", userID, repoID).Delete(new(Star))
	if result.Error != nil {
		return errors.Wrap(result.Error, "delete")
	} else if result.RowsAffected <= 0 {
		return nil
	}

	return s.addStars(db, userID, repoID, -1)
}

// IsStarring returns true if the user stars the repository.
func (s *RepositoriesStore) IsStarring(ctx context.Context, userID, repoID int64) bool {
	if userID <= 0 {
		return false
	}

	var count int64
	s.db.WithContext(ctx).Model(new(Star)).Where("uid = ? AND repo_id = ?", userID, repoID).Count(&count)
	return count > 0
}

// listUsersOf returns the page of users linked to the repository through the
// table, whose user ID column is given, oldest link first.
func (s *RepositoriesStore) listUsersOf(ctx context.Context, table, userIDColumn string, repoID int64, page int) ([]*User, error) {
	db := s.db.WithContext(ctx)
	users := make([]*User, 0, ItemsPerPage)
	return users, db.Joins("INNER JOIN "+table+" ON "+table+"."+userIDColumn+" = "+db.Statement.Quote("user")+".id").
		Where(table+".repo_id = ?", repoID).
		Order(table + ".id ASC").
		Limit(ItemsPerPage).Offset((max(page, 1) - 1) * ItemsPerPage).
		Find(&users).Error
}

// ListStargazers returns the users of given page who star the repository, in
// the order they starred it.
func (s *RepositoriesStore) ListStargazers(ctx context.Context, repoID int64, page int) ([]*User, error) {
	return s.listUsersOf(ctx, "star", "uid", repoID, page)
}

// Touch updates the updated time to the current time and removes the empty state
// of the given repository.
func (s *RepositoriesStore) Touch(ctx context.Context, id int64) error {
	return s.db.WithContext(ctx).
		Model(new(Repository)).
		Where("id = ?", id).
		Updates(map[string]any{
			"is_bare":      false,
			"updated_unix": s.db.NowFunc().Unix(),
		}).
		Error
}

// ListWatches returns all watches of the given repository.
func (s *RepositoriesStore) ListWatches(ctx context.Context, repoID int64) ([]*Watch, error) {
	var watches []*Watch
	return watches, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&watches).Error
}

// addWatches adds delta to the number of watchers of the repository. Adding is
// atomic, unlike counting the rows of the table, which drifts under concurrent
// changes and deadlocks on MySQL.
func (*RepositoriesStore) addWatches(tx *gorm.DB, repoID int64, delta int) error {
	return tx.Model(&Repository{}).Where("id = ?", repoID).UpdateColumn("num_watches", gorm.Expr("num_watches + ?", delta)).Error
}

// Watch marks the user to watch the repository. Like Star, it runs without a
// transaction.
func (s *RepositoriesStore) Watch(ctx context.Context, userID, repoID int64) error {
	return s.watch(ctx, userID, repoID)
}

// watch is Watch on the store's database, which may be a transaction.
func (s *RepositoriesStore) watch(ctx context.Context, userID, repoID int64) error {
	db := s.db.WithContext(ctx)
	w := &Watch{
		UserID: userID,
		RepoID: repoID,
	}
	result := db.FirstOrCreate(w, w)
	if result.Error != nil {
		return errors.Wrap(result.Error, "upsert")
	} else if result.RowsAffected <= 0 {
		return nil // Relation already exists
	}

	return s.addWatches(db, repoID, 1)
}

// Unwatch stops the user from watching the repository. It is a no-op if the
// user does not watch it. Like Star, it runs without a transaction.
func (s *RepositoriesStore) Unwatch(ctx context.Context, userID, repoID int64) error {
	return s.unwatch(ctx, userID, repoID)
}

// unwatch is Unwatch on the store's database, which may be a transaction.
func (s *RepositoriesStore) unwatch(ctx context.Context, userID, repoID int64) error {
	db := s.db.WithContext(ctx)
	result := db.Where("user_id = ? AND repo_id = ?", userID, repoID).Delete(new(Watch))
	if result.Error != nil {
		return errors.Wrap(result.Error, "delete")
	} else if result.RowsAffected <= 0 {
		return nil
	}

	return s.addWatches(db, repoID, -1)
}

// IsWatching returns true if the user watches the repository.
func (s *RepositoriesStore) IsWatching(ctx context.Context, userID, repoID int64) bool {
	if userID <= 0 {
		return false
	}

	var count int64
	s.db.WithContext(ctx).Model(new(Watch)).Where("user_id = ? AND repo_id = ?", userID, repoID).Count(&count)
	return count > 0
}

// ListWatchers returns the users of given page who watch the repository, in
// the order they started to watch it.
func (s *RepositoriesStore) ListWatchers(ctx context.Context, repoID int64, page int) ([]*User, error) {
	return s.listUsersOf(ctx, "watch", "user_id", repoID, page)
}

// HasForkedBy returns true if the given repository has been forked by the given user.
func (s *RepositoriesStore) HasForkedBy(ctx context.Context, repoID, userID int64) bool {
	var count int64
	s.db.WithContext(ctx).Model(new(Repository)).Where("owner_id = ? AND fork_id = ?", userID, repoID).Count(&count)
	return count > 0
}

// Create creates a repository of the owner, created by the doer, with its Git
// repository, which is initialized with the files of given options unless it
// is a mirror. It returns ErrReachLimitOfRepo when the owner cannot create
// more repositories.
func (s *RepositoriesStore) Create(ctx context.Context, logger *logx.Logger, doer, owner *User, opts CreateRepoOptionsLegacy) (_ *Repository, err error) {
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

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := newReposStore(tx).insert(ctx, doer, owner, repo); err != nil {
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

	// Remember visibility preference.
	err = newUsersStore(s.db).Update(ctx, owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	return repo, nil
}

// insert saves the repository of the owner, created by the doer, and gives
// access to it, makes the owner watch it, and records the action. It returns
// ErrRepoAlreadyExist when the owner has a repository of the same name.
func (s *RepositoriesStore) insert(ctx context.Context, doer, owner *User, repo *Repository) (err error) {
	if err = isRepoNameAllowed(repo.Name); err != nil {
		return err
	}

	has, err := s.Exists(ctx, owner, repo.Name)
	if err != nil {
		return errors.Wrap(err, "check existence")
	} else if has {
		return ErrRepoAlreadyExist{args: errx.Args{"ownerID": owner.ID, "name": repo.Name}}
	}

	db := s.db.WithContext(ctx)
	// GORM saves and returns the column default (true) instead of a false value
	// on create, so save the feature switches the caller chose, e.g., all
	// disabled for forks, which also sets them back on the repository.
	enableWiki, enableIssues, enablePulls := repo.EnableWiki, repo.EnableIssues, repo.EnablePulls
	if err = db.Create(repo).Error; err != nil {
		return err
	}
	err = db.Model(repo).UpdateColumns(map[string]any{
		"enable_wiki":   enableWiki,
		"enable_issues": enableIssues,
		"enable_pulls":  enablePulls,
	}).Error
	if err != nil {
		return errors.Wrap(err, "restore feature switches")
	}

	err = db.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos + 1 WHERE id = ?", "user"), owner.ID).Error
	if err != nil {
		return errors.Wrap(err, "increase owned repository count")
	}

	// Give access to all members in owner team.
	if owner.IsOrganization() {
		teams := newTeamsStore(s.db)
		t, err := teams.GetOwnerTeam(ctx, owner.ID)
		if err != nil {
			return errors.Wrap(err, "get owner team")
		} else if err = teams.addRepo(ctx, t, repo); err != nil {
			return errors.Wrap(err, "add to owner team")
		}
	} else {
		// Adding the repository to the owner team recalculates the accesses.
		if err = newPermissionsStore(s.db).recalculateAccesses(ctx, repo); err != nil {
			return errors.Wrap(err, "recalculate accesses")
		}
	}

	if err = s.watch(ctx, owner.ID, repo.ID); err != nil {
		return errors.Wrap(err, "watch")
	}

	if err = newActionsStore(s.db).NewRepo(ctx, doer, owner, repo); err != nil {
		return errors.Wrap(err, "create action")
	}

	return s.LoadAttributes(ctx, repo)
}

// Exists returns true if the owner has a repository of given name, ignoring
// case, whose Git repository exists.
func (s *RepositoriesStore) Exists(ctx context.Context, owner *User, name string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(new(Repository)).Where("owner_id = ? AND lower_name = ?", owner.ID, strings.ToLower(name)).Count(&count).Error
	return count > 0 && osx.IsDir(RepoPath(owner.Name, name)), err
}

// Migrate creates a repository of the owner, created by the doer, from the
// Git repository of given remote address, and its wiki when the remote has
// one. The repository keeps pulling from the remote when it is a mirror.
func (s *RepositoriesStore) Migrate(ctx context.Context, logger *logx.Logger, doer, owner *User, opts MigrateRepoOptions) (*Repository, error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	repo, err := s.Create(ctx, logger, doer, owner, CreateRepoOptionsLegacy{
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
		t, err := newTeamsStore(s.db).GetOwnerTeam(ctx, owner.ID)
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
		if err = s.UpdateSize(ctx, repo); err != nil {
			logger.Error("Failed to update repository size", "repoID", repo.ID, "error", err)
		}
	}

	if opts.IsMirror {
		if err = s.db.WithContext(ctx).Create(&Mirror{
			RepoID:      repo.ID,
			Interval:    conf.Mirror.DefaultInterval,
			EnablePrune: true,
			NextSync:    time.Now().Add(time.Duration(conf.Mirror.DefaultInterval) * time.Hour),
		}).Error; err != nil {
			return repo, errors.Wrap(err, "create mirror")
		}

		repo.IsMirror = true
		return repo, s.Update(ctx, logger, repo, false)
	}

	return repo, s.CleanUpMigrateInfo(ctx, logger, repo)
}

// CleanUpMigrateInfo removes the remote of the repository and its wiki, which
// was migrated from the remote, and saves the repository.
func (s *RepositoriesStore) CleanUpMigrateInfo(ctx context.Context, logger *logx.Logger, repo *Repository) error {
	if err := cleanUpMigrateGitConfig(repo.GitConfigPath()); err != nil {
		return errors.Newf("cleanUpMigrateGitConfig: %v", err)
	}
	if repo.HasWiki() {
		if err := cleanUpMigrateGitConfig(path.Join(repo.WikiPath(), "config")); err != nil {
			return errors.Newf("cleanUpMigrateGitConfig.(wiki): %v", err)
		}
	}

	return s.Update(ctx, logger, repo, false)
}

// Fork creates a fork of the base repository with given name and description
// for the owner, created by the doer. It returns ErrReachLimitOfRepo when the
// owner cannot create more repositories.
func (s *RepositoriesStore) Fork(ctx context.Context, logger *logx.Logger, doer, owner *User, baseRepo *Repository, name, desc string) (_ *Repository, err error) {
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
		IsPrivate:     baseRepo.IsPrivate || conf.Repository.ForcePrivate,
		IsUnlisted:    baseRepo.IsUnlisted,
		IsFork:        true,
		ForkID:        baseRepo.ID,
		// xorm inserts false for a zero bool instead of the column default.
		PullsAllowMerge: true,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A base repository is locked before anything else, the order deleting
		// its forks locks it in, so the two cannot deadlock.
		if err := tx.Exec("UPDATE repository SET num_forks = num_forks + 1 WHERE id = ?", baseRepo.ID).Error; err != nil {
			return err
		} else if err = newReposStore(tx).insert(ctx, doer, owner, repo); err != nil {
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

	// Remember visibility preference.
	err = newUsersStore(s.db).Update(ctx, owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	if err = s.UpdateSize(ctx, repo); err != nil {
		logger.Error("Failed to update repository size", "repoID", repo.ID, "error", err)
	}
	if err = newWebhooksStore(s.db).Prepare(ctx, logger, baseRepo, HookEventTypeFork, &v1types.WebhookForkPayload{
		Forkee: repo.APIFormatLegacy(logger, nil),
		Repo:   baseRepo.APIFormatLegacy(logger, nil),
		Sender: doer.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks", "repoID", baseRepo.ID, "error", err)
	}
	return repo, nil
}

// GetForkBy returns the fork of the repository with given ID that the user
// with given ID owns, with its attributes. It returns ErrRepoNotExist when
// not found.
func (s *RepositoriesStore) GetForkBy(ctx context.Context, repoID, userID int64) (*Repository, error) {
	repo := new(Repository)
	err := s.db.WithContext(ctx).Where("owner_id = ? AND fork_id = ?", userID, repoID).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRepoNotExist{args: errx.Args{"ownerID": userID, "forkID": repoID}}
		}
		return nil, err
	}
	return repo, s.LoadAttributes(ctx, repo)
}

// ListForks returns the forks of the repository, in the order they were
// created, with the repository as their base.
func (s *RepositoriesStore) ListForks(ctx context.Context, repo *Repository) ([]*Repository, error) {
	forks := make([]*Repository, 0, repo.NumForks)
	if err := s.db.WithContext(ctx).Where("fork_id = ?", repo.ID).Order("id ASC").Find(&forks).Error; err != nil {
		return nil, err
	}

	for _, fork := range forks {
		fork.BaseRepo = repo
	}
	return forks, nil
}

// Transfer gives the repository to the user or organization of given name,
// on behalf of the doer. It returns ErrRepoAlreadyExist when the new owner
// has a repository of the same name.
func (s *RepositoriesStore) Transfer(ctx context.Context, doer *User, newOwnerName string, repo *Repository) error {
	// Stopping midway, e.g., when the client disconnects, would roll back the
	// database after the repository moved on disk.
	ctx = context.WithoutCancel(ctx)

	newOwner, err := newUsersStore(s.db).GetByUsername(ctx, newOwnerName)
	if err != nil {
		return errors.Newf("get new owner '%s': %v", newOwnerName, err)
	}

	// Check if new owner has repository with same name.
	has, err := s.Exists(ctx, newOwner, repo.Name)
	if err != nil {
		return errors.Wrap(err, "check existence")
	} else if has {
		return ErrRepoAlreadyExist{args: errx.Args{"ownerName": newOwnerName, "name": repo.Name}}
	}

	owner := repo.Owner
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Note: we have to set value here to make sure recalculate accesses is based on
		// new owner.
		repo.OwnerID = newOwner.ID
		repo.Owner = newOwner

		// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
		if err := tx.Model(repo).UpdateColumns(repo).Error; err != nil {
			return errors.Newf("update owner: %v", err)
		}

		// Remove redundant collaborators.
		collaborators, err := newCollaborationsStore(tx).List(ctx, repo.ID)
		if err != nil {
			return errors.Wrap(err, "list collaborators")
		}

		for _, c := range collaborators {
			if c.ID == newOwner.ID || (newOwner.IsOrganization() && newOrganizationsStore(tx).IsMember(ctx, newOwner.ID, c.ID)) {
				err = tx.Where(&Collaboration{RepoID: repo.ID, UserID: c.ID}).Delete(new(Collaboration)).Error
				if err != nil {
					return errors.Newf("remove collaborator '%d': %v", c.ID, err)
				}
			}
		}

		teams := newTeamsStore(tx)
		// Remove old team-repository relations.
		if owner.IsOrganization() {
			orgTeams, err := teams.ListByOrg(ctx, owner.ID)
			if err != nil {
				return errors.Wrap(err, "list teams")
			}
			for _, t := range orgTeams {
				if !teams.HasRepo(ctx, owner.ID, t.ID, repo.ID) {
					continue
				}

				t.NumRepos--
				if err := tx.Model(t).Select("*").Updates(t).Error; err != nil {
					return errors.Newf("decrease team repository count '%d': %v", t.ID, err)
				}
			}

			if err = newOrganizationsStore(tx).removeRepo(ctx, owner.ID, repo.ID); err != nil {
				return errors.Wrap(err, "remove from teams")
			}
		}

		if newOwner.IsOrganization() {
			t, err := teams.GetOwnerTeam(ctx, newOwner.ID)
			if err != nil {
				return errors.Wrap(err, "get owner team")
			} else if err = teams.addRepo(ctx, t, repo); err != nil {
				return errors.Newf("add to owner team: %v", err)
			}
		} else {
			// Adding the repository to the owner team recalculates the accesses.
			if err = newPermissionsStore(tx).recalculateAccesses(ctx, repo); err != nil {
				return errors.Wrap(err, "recalculate accesses")
			}

			// Repositories of users have no push allowlist to manage, and one left
			// enabled would keep out everyone not listed by name, e.g., the new
			// owner.
			if err = tx.Model(new(ProtectBranch)).Where("repo_id = ?", repo.ID).UpdateColumn("enable_whitelist", false).Error; err != nil {
				return errors.Wrap(err, "disable push allowlists")
			}
		}

		// Update repository count.
		if err = tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos + 1 WHERE id = ?", "user"), newOwner.ID).Error; err != nil {
			return errors.Newf("increase new owner repository count: %v", err)
		} else if err = tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos - 1 WHERE id = ?", "user"), owner.ID).Error; err != nil {
			return errors.Newf("decrease old owner repository count: %v", err)
		}

		repos := newReposStore(tx)
		// Remove watch for organization.
		if owner.IsOrganization() {
			if err = repos.unwatch(ctx, owner.ID, repo.ID); err != nil {
				return errors.Wrap(err, "unwatch repository for the organization owner")
			}
		}

		if err = repos.watch(ctx, newOwner.ID, repo.ID); err != nil {
			return errors.Wrap(err, "watch")
		}

		if err = newActionsStore(tx).TransferRepo(ctx, doer, owner, newOwner, repo); err != nil {
			return errors.Wrap(err, "create action")
		}

		// Move repository to new path.
		if err = os.MkdirAll(repox.UserPath(newOwner.Name), os.ModePerm); err != nil {
			return err
		}
		if err = os.Rename(RepoPath(owner.Name, repo.Name), RepoPath(newOwner.Name, repo.Name)); err != nil {
			return errors.Newf("rename repository directory: %v", err)
		}

		// Move wiki repository to new path.
		wikiPath := WikiPath(owner.Name, repo.Name)
		if osx.Exist(wikiPath) {
			if err = os.Rename(wikiPath, WikiPath(newOwner.Name, repo.Name)); err != nil {
				return errors.Newf("rename repository wiki: %v", err)
			}
		}
		return nil
	})
}

// Rename renames the Git repository and wiki of the repository of the owner
// from the old name to the new name. The caller saves the new name. It
// returns ErrRepoAlreadyExist when the owner has a repository of the new
// name.
func (s *RepositoriesStore) Rename(ctx context.Context, owner *User, oldName, newName string) (err error) {
	oldName = strings.ToLower(oldName)
	newName = strings.ToLower(newName)
	if err = isRepoNameAllowed(newName); err != nil {
		return err
	}

	has, err := s.Exists(ctx, owner, newName)
	if err != nil {
		return errors.Wrap(err, "check existence")
	} else if has {
		return ErrRepoAlreadyExist{args: errx.Args{"ownerID": owner.ID, "name": newName}}
	}

	repo, err := s.GetByName(ctx, owner.ID, oldName)
	if err != nil {
		return errors.Wrap(err, "get repository")
	}
	repo.Owner = owner

	// Change repository directory name.
	if err = os.Rename(repo.RepoPath(), RepoPath(owner.Name, newName)); err != nil {
		return errors.Newf("rename repository directory: %v", err)
	}

	wikiPath := repo.WikiPath()
	if osx.Exist(wikiPath) {
		if err = os.Rename(wikiPath, WikiPath(owner.Name, newName)); err != nil {
			return errors.Newf("rename repository wiki: %v", err)
		}
	}
	return nil
}

// Update saves the repository. When its visibility changed, it also changes
// the visibility of its forks and actions, and recalculates the accesses of
// organization repositories. Descriptions longer than 512 bytes and websites
// longer than 255 bytes are cut.
func (s *RepositoriesStore) Update(ctx context.Context, logger *logx.Logger, repo *Repository, visibilityChanged bool) error {
	// Stopping midway, e.g., when the client disconnects, would roll back the
	// database but keep the changed Git daemon export file.
	ctx = context.WithoutCancel(ctx)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := newReposStore(tx).update(ctx, logger, repo, visibilityChanged); err != nil {
			return errors.Newf("updateRepository: %v", err)
		}
		return nil
	})
}

// update is Update without a transaction of its own.
func (s *RepositoriesStore) update(ctx context.Context, logger *logx.Logger, repo *Repository, visibilityChanged bool) (err error) {
	repo.LowerName = strings.ToLower(repo.Name)

	db := s.db.WithContext(ctx)
	visibilityCols := []string{"is_private", "is_unlisted", "allow_public_wiki", "allow_public_issues"}

	// 🚨 SECURITY: A fork must never be more visible than its base repository,
	// whichever caller saves it.
	if repo.IsFork {
		base := new(Repository)
		result := db.Select(visibilityCols).Where("id = ?", repo.ForkID).Limit(1).Find(base)
		if result.Error != nil {
			return errors.Wrap(result.Error, "get base repository")
		} else if result.RowsAffected > 0 {
			repo.limitVisibilityTo(base)
		}
	}

	saved := new(Repository)
	if err = db.Select(visibilityCols).Where("id = ?", repo.ID).Limit(1).Find(saved).Error; err != nil {
		return errors.Wrap(err, "get saved visibility")
	}
	visibilityChanged = visibilityChanged || repo.IsPrivate != saved.IsPrivate || repo.IsUnlisted != saved.IsUnlisted
	publicAccessChanged := repo.AllowPublicWiki != saved.AllowPublicWiki || repo.AllowPublicIssues != saved.AllowPublicIssues

	if len(repo.Description) > 512 {
		repo.Description = repo.Description[:512]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err = db.Model(repo).Select("*").UpdateColumns(repo).Error; err != nil {
		return errors.Newf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(db); err != nil {
			return errors.Newf("getOwner: %v", err)
		}
		if repo.Owner.IsOrganization() {
			// Organization repository needs to recalculate access table when visibility is changed.
			if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, 0); err != nil {
				return errors.Newf("recalculateTeamAccesses: %v", err)
			}
		}

		// Create/Remove git-daemon-export-ok for git-daemon.
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

		// Change visibility of generated actions.
		err = db.Model(new(Action)).Where("repo_id = ?", repo.ID).UpdateColumn("is_private", repo.IsPrivate || repo.IsUnlisted).Error
		if err != nil {
			return errors.Newf("change action visibility of repository: %v", err)
		}
	}

	if visibilityChanged || publicAccessChanged {
		forkRepos := make([]*Repository, 0, 10)
		if err = db.Where("fork_id = ?", repo.ID).Find(&forkRepos).Error; err != nil {
			return errors.Wrap(err, "list forks")
		}
		// Saving a fork limits it to what its base allows.
		for i := range forkRepos {
			// Forks follow their base repository, unless the site forces private
			// repositories and forks keep the visibility they have, as far as their
			// base allows.
			if visibilityChanged && !conf.Repository.ForcePrivate {
				forkRepos[i].IsPrivate = repo.IsPrivate
				forkRepos[i].IsUnlisted = repo.IsUnlisted
			}
			if err = s.update(ctx, logger, forkRepos[i], visibilityChanged); err != nil {
				return errors.Newf("updateRepository[%d]: %v", forkRepos[i].ID, err)
			}
		}
	}

	return nil
}

// Delete deletes the repository with given ID of the owner with given ID,
// with everything in it, and its Git repository, wiki, and attachments. It
// returns ErrRepoNotExist when not found.
func (s *RepositoriesStore) Delete(ctx context.Context, logger *logx.Logger, ownerID, repoID int64) error {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind, and cleanups after a failed creation run after
	// the client may have left.
	ctx = context.WithoutCancel(ctx)

	repo := new(Repository)
	err := s.db.WithContext(ctx).Where("id = ? AND owner_id = ?", repoID, ownerID).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRepoNotExist{args: map[string]any{"ownerID": ownerID, "repoID": repoID}}
		}
		return err
	}

	// In case it is an organization.
	org, err := newUsersStore(s.db).GetByID(ctx, ownerID)
	if err != nil {
		return err
	}
	repo.Owner = org
	var teams []*Team
	if org.IsOrganization() {
		if teams, err = newTeamsStore(s.db).ListByOrg(ctx, org.ID); err != nil {
			return err
		}
	}

	var attachmentPaths []string
	var publicKeyDeleted bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Attachments of releases go with the releases, and are deleted after
		// them, in the order deleting a single release takes their locks, so that
		// the two cannot deadlock. The releases are locked before anything else
		// is read, because on MySQL the first plain read fixes the snapshot, which
		// would hide the attachments an edit of a release adds before the
		// releases are deleted.
		var releaseIDs []int64
		listReleases := tx.Model(new(Release)).Where("repo_id = ?", repoID)
		if !conf.UseSQLite3 {
			listReleases = listReleases.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := listReleases.Pluck("id", &releaseIDs).Error; err != nil {
			return errors.Wrap(err, "list releases")
		}

		// Delete labels and milestones before the repository, the order issue
		// creation locks them in, so the two cannot deadlock.
		if err := deleteByColumn(tx, "repo_id", repoID, "label", "milestone"); err != nil {
			return err
		}
		// A base repository is locked before its forks, the order updating its
		// visibility and deleting it lock them in, so they cannot deadlock.
		if repo.IsFork {
			if err := tx.Exec("UPDATE repository SET num_forks = num_forks - 1 WHERE id = ?", repo.ForkID).Error; err != nil {
				return errors.Newf("decrease fork count: %v", err)
			}
		}
		if err := tx.Where("id = ?", repoID).Delete(new(Repository)).Error; err != nil {
			return errors.Wrap(err, "delete repository")
		}

		// Reset forks after the repository, the order updating its visibility
		// locks them in, so the two cannot deadlock. Forks are updated by primary
		// key, because fork_id has no index and updating by it would lock every
		// repository until commit on MySQL.
		var forkIDs []int64
		if err := tx.Model(new(Repository)).Where("fork_id = ?", repoID).Pluck("id", &forkIDs).Error; err != nil {
			return errors.Wrap(err, "list forks")
		}
		for ids := range slices.Chunk(forkIDs, 500) {
			if err := tx.Exec("UPDATE repository SET fork_id = 0, is_fork = ? WHERE id IN ?", false, ids).Error; err != nil {
				return errors.Wrap(err, "reset forks")
			}
		}

		// Teams are locked after repositories, the order forking and transferring
		// into the organization lock them in, so the two cannot deadlock.
		if org.IsOrganization() {
			teamsStore := newTeamsStore(tx)
			for _, t := range teams {
				if !teamsStore.HasRepo(ctx, org.ID, t.ID, repoID) {
					continue
				} else if err := teamsStore.removeRepo(ctx, t, repo, false); err != nil {
					return err
				}
			}
		}

		// Users are locked before the rows of the repository in other tables, the
		// order creating and transferring repositories lock them in, so they
		// cannot deadlock.
		if err := tx.Exec(dbx.Quote("UPDATE %s SET num_repos = num_repos - 1 WHERE id = ?", "user"), ownerID).Error; err != nil {
			return errors.Wrap(err, "decrease repository count of owner")
		}

		// The columns of pull requests, accesses and watches deleted by are
		// indexed, so that MySQL locks only the rows of this repository, after
		// the repository and its base were locked.
		if err := deleteByColumn(tx, "base_repo_id", repoID, "pull_request"); err != nil {
			return err
		}
		if err := deleteByColumn(tx, "repo_id", repoID,
			"access",
			"action",
			"watch",
			"star",
			"mirror",
			"issue_user",
			"release",
			"collaboration",
			"protect_branch",
			"webhook",
			"hook_task",
			"lfs_object",
		); err != nil {
			return err
		}

		// In chunks, because a statement can bind only so many parameters.
		for ids := range slices.Chunk(releaseIDs, 500) {
			var uuids []string
			if err := tx.Model(new(Attachment)).Where("release_id IN ?", ids).Pluck("uuid", &uuids).Error; err != nil {
				return errors.Wrap(err, "list attachments of releases")
			}
			for _, uuid := range uuids {
				attachmentPaths = append(attachmentPaths, AttachmentLocalPath(uuid))
			}
			if err := tx.Where("release_id IN ?", ids).Delete(new(Attachment)).Error; err != nil {
				return errors.Wrap(err, "delete attachments of releases")
			}
		}

		// Delete comments and attachments.
		var issueIDs []int64
		if err := tx.Model(new(Issue)).Where("repo_id = ?", repoID).Pluck("id", &issueIDs).Error; err != nil {
			return errors.Wrap(err, "list issues")
		}
		for _, issueID := range issueIDs {
			var uuids []string
			if err := tx.Model(new(Attachment)).Where("issue_id = ?", issueID).Pluck("uuid", &uuids).Error; err != nil {
				return errors.Wrap(err, "list attachments")
			}
			for _, uuid := range uuids {
				attachmentPaths = append(attachmentPaths, AttachmentLocalPath(uuid))
			}
			if err := deleteByColumn(tx, "issue_id", issueID, "comment", "attachment", "issue_label"); err != nil {
				return err
			}
		}
		if err := deleteByColumn(tx, "repo_id", repoID, "issue"); err != nil {
			return err
		}

		var deployKeyIDs []int64
		if err := tx.Model(new(DeployKey)).Where("repo_id = ?", repoID).Pluck("key_id", &deployKeyIDs).Error; err != nil {
			return errors.Wrap(err, "list deploy keys")
		}
		if err := lockPublicKeys(tx, deployKeyIDs...); err != nil {
			return errors.Wrap(err, "lock public keys")
		}
		if err := tx.Where("repo_id = ?", repoID).Delete(new(DeployKey)).Error; err != nil {
			return errors.Wrap(err, "delete deploy keys")
		}
		// Other repositories may use the same public keys.
		var err error
		if publicKeyDeleted, err = deleteUnusedPublicKeys(tx, deployKeyIDs...); err != nil {
			return err
		}

		return nil
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

	if publicKeyDeleted {
		if err = newPublicKeysStore(s.db).RewriteAuthorizedKeys(); err != nil {
			return errors.Wrap(err, "rewrite authorized keys")
		}
	}
	return nil
}

// GetByRef returns the repository of given reference in the form of
// "owner/name", with its attributes. It returns InvalidRepoReference when the
// reference has no owner.
func (s *RepositoriesStore) GetByRef(ctx context.Context, ref string) (*Repository, error) {
	n := strings.IndexByte(ref, byte('/'))
	if n < 2 {
		return nil, InvalidRepoReference{Ref: ref}
	}

	userName, repoName := ref[:n], ref[n+1:]
	user, err := newUsersStore(s.db).GetByUsername(ctx, userName)
	if err != nil {
		return nil, err
	}

	repo, err := s.GetByName(ctx, user.ID, repoName)
	if err != nil {
		return nil, err
	}
	return repo, s.LoadAttributes(ctx, repo)
}

// Count returns the number of repositories, of the owner with given ID when it
// is positive. Private repositories are counted only when includePrivate is
// true.
func (s *RepositoriesStore) Count(ctx context.Context, ownerID int64, includePrivate bool) (int64, error) {
	query := s.db.WithContext(ctx).Model(new(Repository))
	if ownerID > 0 {
		query = query.Where("owner_id = ?", ownerID)
	}
	if !includePrivate {
		query = query.Where("is_private = ?", false)
	}

	var count int64
	return count, query.Count(&count).Error
}

// ListByPage returns the repositories of given page, in the order they were
// created.
func (s *RepositoriesStore) ListByPage(ctx context.Context, page, pageSize int) ([]*Repository, error) {
	repos := make([]*Repository, 0, pageSize)
	return repos, s.db.WithContext(ctx).Order("id ASC").Limit(pageSize).Offset((max(page, 1) - 1) * pageSize).Find(&repos).Error
}

// FilterIDsWithIssues returns the IDs among given ones of the repositories
// that use the built-in issue tracker, in the order they were created, or nil
// when there are none.
func (s *RepositoriesStore) FilterIDsWithIssues(ctx context.Context, repoIDs []int64) ([]int64, error) {
	if len(repoIDs) == 0 {
		return nil, nil
	}

	var validIDs []int64
	if err := s.db.WithContext(ctx).Model(new(Repository)).
		Where("enable_issues = ? AND enable_external_tracker = ? AND id IN ?", true, false, repoIDs).
		Order("id ASC").
		Pluck("id", &validIDs).Error; err != nil {
		return nil, errors.Wrap(err, "filter repositories")
	}

	if len(validIDs) == 0 {
		return nil, nil
	}
	return validIDs, nil
}

// ListNonMirrors returns the repositories that are not mirrors, in the order
// they were created.
func (s *RepositoriesStore) ListNonMirrors(ctx context.Context) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, s.db.WithContext(ctx).Where("is_mirror = ?", false).Order("id ASC").Find(&repos).Error
}

type UserRepoOptions struct {
	UserID   int64
	Private  bool
	Page     int
	PageSize int
}

// ListByOwner returns the repositories of given page of the owner, most
// recently updated first. Private and unlisted repositories are included only
// when opts.Private is true. A page size that is not positive means no limit.
func (s *RepositoriesStore) ListByOwner(ctx context.Context, opts UserRepoOptions) ([]*Repository, error) {
	query := s.db.WithContext(ctx).Where("owner_id = ?", opts.UserID).Order("updated_unix DESC").Order("id DESC")
	if !opts.Private {
		query = query.Where("is_private = ? AND is_unlisted = ?", false, false)
	}

	// NOTE: A non-positive page size means no limit, e.g., callers pass the
	// cached number of repositories which can be zero.
	if opts.PageSize > 0 {
		query = query.Limit(opts.PageSize).Offset((max(opts.Page, 1) - 1) * opts.PageSize)
	}

	repos := make([]*Repository, 0, max(opts.PageSize, 0))
	return repos, query.Find(&repos).Error
}

// ListMirrorsByOwner returns the mirrors of the owner with given ID, in the
// order they were created.
func (s *RepositoriesStore) ListMirrorsByOwner(ctx context.Context, ownerID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, s.db.WithContext(ctx).Where("owner_id = ? AND is_mirror = ?", ownerID, true).Order("id ASC").Find(&repos).Error
}

// ListOwnedAndCollaborative returns the repositories that the user with given
// ID collaborates on, then those the user owns, each in the order they were
// created.
func (s *RepositoriesStore) ListOwnedAndCollaborative(ctx context.Context, userID int64) ([]*Repository, error) {
	db := s.db.WithContext(ctx)
	repos := make([]*Repository, 0, 10)
	if err := db.Table("repository AS repo").
		Select("repo.*").
		Joins("INNER JOIN collaboration ON collaboration.repo_id = repo.id").
		Where("collaboration.user_id = ?", userID).
		Order("repo.id ASC").
		Find(&repos).Error; err != nil {
		return nil, errors.Wrap(err, "list collaborative repositories")
	}

	ownRepos := make([]*Repository, 0, 10)
	if err := db.Where("owner_id = ?", userID).Order("id ASC").Find(&ownRepos).Error; err != nil {
		return nil, errors.Wrap(err, "list own repositories")
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

// Search returns the repositories of given page whose name or description
// contains the keyword, ignoring case, and the number of them.
func (s *RepositoriesStore) Search(ctx context.Context, opts SearchRepoOptions) (repos []*Repository, count int64, err error) {
	repos = make([]*Repository, 0, opts.PageSize)
	query := s.db.WithContext(ctx).Table("repository AS repo")
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
		Limit(opts.PageSize).Offset((max(opts.Page, 1) - 1) * opts.PageSize).
		Find(&repos).Error
}

// listUsersWithAccess returns the users who have at least given access mode to
// the repository, and its owner when it is a user. Organizations are left out,
// even when they have access.
func (s *RepositoriesStore) listUsersWithAccess(ctx context.Context, repo *Repository, mode AccessMode) (_ []*User, err error) {
	db := s.db.WithContext(ctx)
	if err = repo.getOwner(db); err != nil {
		return nil, err
	}

	accesses := make([]*Access, 0, 10)
	if err = db.Where("repo_id = ? AND mode >= ?", repo.ID, mode).Find(&accesses).Error; err != nil {
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

		if err = db.Where("id IN ? AND type = ?", userIDs, UserTypeIndividual).Order("id ASC").Find(&users).Error; err != nil {
			return nil, err
		}

		// TODO(unknwon): Rely on AfterFind hook to sanitize user full name.
		for _, u := range users {
			u.FullName = markup.Sanitize(u.FullName)
		}
	}
	if !repo.Owner.IsOrganization() {
		users = append(users, repo.Owner)
	}

	return users, nil
}

// ListAssignees returns the users who can be assigned to issues of the
// repository, i.e., those who can read it, and its owner when it is a user.
func (s *RepositoriesStore) ListAssignees(ctx context.Context, repo *Repository) ([]*User, error) {
	return s.listUsersWithAccess(ctx, repo, AccessModeRead)
}

// ListWriters returns the users who can write to the repository, and its owner
// when it is a user.
func (s *RepositoriesStore) ListWriters(ctx context.Context, repo *Repository) ([]*User, error) {
	return s.listUsersWithAccess(ctx, repo, AccessModeWrite)
}

// UpdateSize saves the size of the Git repository of the repository.
func (s *RepositoriesStore) UpdateSize(ctx context.Context, repo *Repository) error {
	countObject, err := git.RepoCountObjects(ctx, repo.RepoPath())
	if err != nil {
		return errors.Newf("count repository objects: %v", err)
	}

	repo.Size = countObject.Size + countObject.SizePack
	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err = s.db.WithContext(ctx).Model(repo).Select("size").UpdateColumns(repo).Error; err != nil {
		return errors.Newf("update size: %v", err)
	}
	return nil
}

// lockRepository locks the repository of given ID until the transaction ends,
// and reports whether it exists. Transactions that change a repository and
// rows of its organization's teams lock the repository first, so they cannot
// deadlock.
func lockRepository(tx *gorm.DB, id int64) (bool, error) {
	query := tx.Model(new(Repository)).Where("id = ?", id)
	if !conf.UseSQLite3 {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ids []int64
	if err := query.Pluck("id", &ids).Error; err != nil {
		return false, errors.Wrap(err, "lock repository")
	}
	return len(ids) > 0, nil
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

// initRepository performs initial commit with chosen setup files on behalf of doer.
func initRepository(ctx context.Context, logger *logx.Logger, tx *gorm.DB, repoPath string, doer *User, repo *Repository, opts CreateRepoOptionsLegacy) (err error) {
	if err = git.Init(ctx, repoPath, git.InitOptions{Bare: true, ObjectFormat: opts.ObjectFormat}); err != nil {
		return errors.Newf("init repository: %v", err)
	}

	// Set default branch.
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
	// override changes that were done earlier with SQL).
	saved, err := getRepositoryByID(tx, repo.ID)
	if err != nil {
		return errors.Newf("getRepositoryByID: %v", err)
	}

	if !opts.AutoInit {
		saved.IsEmpty = true
	}

	saved.DefaultBranch = conf.Repository.DefaultBranch
	if err = newReposStore(tx).update(ctx, logger, saved, false); err != nil {
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

// isRepoNameAllowed returns an error if given name is a reserved name or pattern for repositories.
func isRepoNameAllowed(name string) error {
	return isNameAllowed(reservedRepoNames, reservedRepoPatterns, name)
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

// RepoPath returns repository path by given user and repository name.
//
// Deprecated: Use repox.RepositoryPath instead.
func RepoPath(userName, repoName string) string {
	return repox.RepositoryPath(userName, repoName)
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
		if err := Handle.Repositories().Delete(context.TODO(), logger, repo.OwnerID, repo.ID); err != nil {
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

			if err = Handle.Repositories().Update(context.TODO(), logger, repo, false); err != nil {
				logger.Error("Failed to update repository", "repoID", id, "error", err)
				continue
			}
		}
	}
	// ***** END: Repository.NumForks *****
}

// Watch is connection request for receiving repository notification.
type Watch struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"uniqueIndex:UQE_watch_watch"`
	RepoID int64 `gorm:"uniqueIndex:UQE_watch_watch;index:idx_watch_repo_id"`
}

type Star struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"column:uid;uniqueIndex:UQE_star_s"`
	RepoID int64 `gorm:"uniqueIndex:UQE_star_s"`
}

// deleteByColumn deletes the rows of given tables whose column has the ID. A
// non-positive ID is refused, because a column with 0 means "none" in some
// tables, e.g., webhooks of organizations have repo_id 0.
func deleteByColumn(tx *gorm.DB, column string, id int64, tables ...string) error {
	if id <= 0 {
		return errors.Newf("delete by %s: invalid ID %d", column, id)
	}
	for _, table := range tables {
		// The names are quoted, because some are reserved words, e.g., "release"
		// in MySQL.
		err := tx.Exec("DELETE FROM ? WHERE ? = ?", clause.Table{Name: table}, clause.Column{Name: column}, id).Error
		if err != nil {
			return errors.Newf("delete %s by %s: %v", table, column, err)
		}
	}
	return nil
}
