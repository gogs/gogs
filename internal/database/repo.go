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
	RepoID int64 `gorm:"uniqueIndex:UQE_watch_watch"`
}

type Star struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"column:uid;uniqueIndex:UQE_star_s"`
	RepoID int64 `gorm:"uniqueIndex:UQE_star_s"`
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
