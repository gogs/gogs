package database

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

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
	Permission *apiv1types.RepositoryPermission
	Parent     *apiv1types.Repository
}

// APIFormat returns the API format of a repository.
func (r *Repository) APIFormat(owner *User, opts ...RepositoryAPIFormatOptions) *apiv1types.Repository {
	var opt RepositoryAPIFormatOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cloneLink := repox.NewCloneLink(owner.Name, r.Name, false)
	return &apiv1types.Repository{
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
// by the given order (e.g. "updated_unix DESC"). Repositories that are owned
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

func (s *RepositoriesStore) recountStars(tx *gorm.DB, userID, repoID int64) error {
	/*
		Equivalent SQL for PostgreSQL:

		UPDATE repository
		SET num_stars = (
			SELECT COUNT(*) FROM star WHERE repo_id = @repoID
		)
		WHERE id = @repoID
	*/
	err := tx.Model(&Repository{}).
		Where("id = ?", repoID).
		Update(
			"num_stars",
			tx.Model(&Star{}).Select("COUNT(*)").Where("repo_id = ?", repoID),
		).
		Error
	if err != nil {
		return errors.Wrap(err, `update "repository.num_stars"`)
	}

	/*
		Equivalent SQL for PostgreSQL:

		UPDATE "user"
		SET num_stars = (
			SELECT COUNT(*) FROM star WHERE uid = @userID
		)
		WHERE id = @userID
	*/
	err = tx.Model(&User{}).
		Where("id = ?", userID).
		Update(
			"num_stars",
			tx.Model(&Star{}).Select("COUNT(*)").Where("uid = ?", userID),
		).
		Error
	if err != nil {
		return errors.Wrap(err, `update "user.num_stars"`)
	}
	return nil
}

// Star marks the user to star the repository.
func (s *RepositoriesStore) Star(ctx context.Context, userID, repoID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		star := &Star{
			UserID: userID,
			RepoID: repoID,
		}
		result := tx.FirstOrCreate(star, star)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}

		return s.recountStars(tx, userID, repoID)
	})
}

// Unstar marks the user to no longer star the repository. Unstarring a
// repository that the user does not star is a no-op.
func (s *RepositoriesStore) Unstar(ctx context.Context, userID, repoID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("uid = ? AND repo_id = ?", userID, repoID).Delete(new(Star))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete")
		} else if result.RowsAffected <= 0 {
			return nil
		}

		return s.recountStars(tx, userID, repoID)
	})
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

// listUsersOf returns the users of given page who relate to the repository
// through the table, whose column of user IDs is given, in the order they
// started to.
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

func (s *RepositoriesStore) recountWatches(tx *gorm.DB, repoID int64) error {
	/*
		Equivalent SQL for PostgreSQL:

		UPDATE repository
		SET num_watches = (
			SELECT COUNT(*) FROM watch WHERE repo_id = @repoID
		)
		WHERE id = @repoID
	*/
	return tx.Model(&Repository{}).
		Where("id = ?", repoID).
		Update(
			"num_watches",
			tx.Model(&Watch{}).Select("COUNT(*)").Where("repo_id = ?", repoID),
		).
		Error
}

// Watch marks the user to watch the repository.
func (s *RepositoriesStore) Watch(ctx context.Context, userID, repoID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newReposStore(tx).watch(ctx, userID, repoID)
	})
}

// watch is Watch without a transaction of its own.
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

	return s.recountWatches(db, repoID)
}

// Unwatch marks the user to no longer watch the repository. Unwatching a
// repository that the user does not watch is a no-op.
func (s *RepositoriesStore) Unwatch(ctx context.Context, userID, repoID int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newReposStore(tx).unwatch(ctx, userID, repoID)
	})
}

// unwatch is Unwatch without a transaction of its own.
func (s *RepositoriesStore) unwatch(ctx context.Context, userID, repoID int64) error {
	db := s.db.WithContext(ctx)
	result := db.Where("user_id = ? AND repo_id = ?", userID, repoID).Delete(new(Watch))
	if result.Error != nil {
		return errors.Wrap(result.Error, "delete")
	} else if result.RowsAffected <= 0 {
		return nil
	}

	return s.recountWatches(db, repoID)
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

// HasForkedBy returns true if the given repository has forked by the given user.
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

	// Remember visibility preference
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
			return repo, errors.Newf("InsertOne: %v", err)
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
		IsPrivate:     baseRepo.IsPrivate,
		IsUnlisted:    baseRepo.IsUnlisted,
		IsFork:        true,
		ForkID:        baseRepo.ID,
		// xorm inserts false for a zero bool instead of the column default.
		PullsAllowMerge: true,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := newReposStore(tx).insert(ctx, doer, owner, repo); err != nil {
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
	err = newUsersStore(s.db).Update(ctx, owner.ID, UpdateUserOptions{LastRepoVisibility: &repo.IsPrivate})
	if err != nil {
		return nil, errors.Wrap(err, "update user")
	}

	if err = s.UpdateSize(ctx, repo); err != nil {
		logger.Error("Failed to update repository size", "repoID", repo.ID, "error", err)
	}
	if err = newWebhooksStore(s.db).Prepare(ctx, logger, baseRepo, HookEventTypeFork, &apiv1types.WebhookForkPayload{
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

		// Update repository.
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

	// Change repository directory name
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

	if len(repo.Description) > 512 {
		repo.Description = repo.Description[:512]
	}
	if len(repo.Website) > 255 {
		repo.Website = repo.Website[:255]
	}

	db := s.db.WithContext(ctx)
	// NOTE: Use UpdateColumns to skip hooks so the update time stays unchanged.
	if err = db.Model(repo).Select("*").UpdateColumns(repo).Error; err != nil {
		return errors.Newf("update: %v", err)
	}

	if visibilityChanged {
		if err = repo.getOwner(db); err != nil {
			return errors.Newf("getOwner: %v", err)
		}
		if repo.Owner.IsOrganization() {
			// Organization repository need to recalculate access table when visibility is changed
			if err = newPermissionsStore(s.db).recalculateTeamAccesses(ctx, repo, 0); err != nil {
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

		forkRepos := make([]*Repository, 0, 10)
		if err = db.Where("fork_id = ?", repo.ID).Find(&forkRepos).Error; err != nil {
			return errors.Wrap(err, "list forks")
		}
		for i := range forkRepos {
			forkRepos[i].IsPrivate = repo.IsPrivate
			forkRepos[i].IsUnlisted = repo.IsUnlisted
			if err = s.update(ctx, logger, forkRepos[i], true); err != nil {
				return errors.Newf("updateRepository[%d]: %v", forkRepos[i].ID, err)
			}
		}

		// Change visibility of generated actions
		err = db.Model(new(Action)).Where("repo_id = ?", repo.ID).UpdateColumn("is_private", repo.IsPrivate || repo.IsUnlisted).Error
		if err != nil {
			return errors.Newf("change action visibility of repository: %v", err)
		}
	}

	return nil
}

// Delete deletes the repository with given ID of the owner with given ID,
// with everything in it, and its Git repository, wiki, and attachments. It
// returns ErrRepoNotExist when not found.
func (s *RepositoriesStore) Delete(ctx context.Context, logger *logx.Logger, ownerID, repoID int64) error {
	repo := new(Repository)
	err := s.db.WithContext(ctx).Where(&Repository{ID: repoID, OwnerID: ownerID}).First(repo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRepoNotExist{args: map[string]any{"ownerID": ownerID, "repoID": repoID}}
		}
		return err
	}

	// In case is a organization.
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

	attachmentPaths := make([]string, 0, 25)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if err = s.db.WithContext(ctx).Exec("UPDATE repository SET fork_id = 0, is_fork = ? WHERE fork_id = ?", false, repo.ID).Error; err != nil {
			logger.Error("Failed to reset fork status of forks", "repoID", repo.ID, "error", err)
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
// the repository, and its owner when it is a user.
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

		if err = db.Where("id IN ?", userIDs).Order("id ASC").Find(&users).Error; err != nil {
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
