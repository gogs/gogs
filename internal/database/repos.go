// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	api "github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/repoutil"
)

// ReposStore is the persistent interface for repositories.
type ReposStore interface {
	// Create creates a new repository record in the database. It returns
	// ErrNameNotAllowed when the repository name is not allowed, or
	// ErrRepoAlreadyExist when a repository with same name already exists for the
	// owner.
	Create(ctx context.Context, ownerID int64, opts CreateRepoOptions) (*Repository, error)
	// GetByCollaboratorID returns a list of repositories that the given
	// collaborator has access to. Results are limited to the given limit and sorted
	// by the given order (e.g. "updated_unix DESC"). Repositories that are owned
	// directly by the given collaborator are not included.
	GetByCollaboratorID(ctx context.Context, collaboratorID int64, limit int, orderBy string) ([]*Repository, error)
	// GetByCollaboratorIDWithAccessMode returns a list of repositories and
	// corresponding access mode that the given collaborator has access to.
	// Repositories that are owned directly by the given collaborator are not
	// included.
	GetByCollaboratorIDWithAccessMode(ctx context.Context, collaboratorID int64) (map[*Repository]AccessMode, error)
	// GetByID returns the repository with given ID. It returns ErrRepoNotExist when
	// not found.
	GetByID(ctx context.Context, id int64) (*Repository, error)
	// GetByName returns the repository with given owner and name. It returns
	// ErrRepoNotExist when not found.
	GetByName(ctx context.Context, ownerID int64, name string) (*Repository, error)
	// Star marks the user to star the repository.
	Star(ctx context.Context, userID, repoID int64) error
	// Touch updates the updated time to the current time and removes the bare state
	// of the given repository.
	Touch(ctx context.Context, id int64) error

	// ListWatches returns all watches of the given repository.
	ListWatches(ctx context.Context, repoID int64) ([]*Watch, error)
	// Watch marks the user to watch the repository.
	Watch(ctx context.Context, userID, repoID int64) error

	// HasForkedBy returns true if the given repository has forked by the given user.
	HasForkedBy(ctx context.Context, repoID, userID int64) bool
}

var Repos ReposStore

// BeforeCreate implements the GORM create hook.
func (r *Repository) BeforeCreate(tx *gorm.DB) error {
	if r.CreatedUnix == 0 {
		r.CreatedUnix = tx.NowFunc().Unix()
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
	r.Created = time.Unix(r.CreatedUnix, 0).Local()
	r.Updated = time.Unix(r.UpdatedUnix, 0).Local()
	return nil
}

type RepositoryAPIFormatOptions struct {
	Permission *api.Permission
	Parent     *api.Repository
}

// APIFormat returns the API format of a repository.
func (r *Repository) APIFormat(owner *User, opts ...RepositoryAPIFormatOptions) *api.Repository {
	var opt RepositoryAPIFormatOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cloneLink := repoutil.NewCloneLink(owner.Name, r.Name, false)
	return &api.Repository{
		ID:            r.ID,
		Owner:         owner.APIFormat(),
		Name:          r.Name,
		FullName:      owner.Name + "/" + r.Name,
		Description:   r.Description,
		Private:       r.IsPrivate,
		Fork:          r.IsFork,
		Parent:        opt.Parent,
		Empty:         r.IsBare,
		Mirror:        r.IsMirror,
		Size:          r.Size,
		HTMLURL:       repoutil.HTMLURL(owner.Name, r.Name),
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

var _ ReposStore = (*repos)(nil)

type repos struct {
	*gorm.DB
}

// NewReposStore returns a persistent interface for repositories with given
// database connection.
func NewReposStore(db *gorm.DB) ReposStore {
	return &repos{DB: db}
}

type ErrRepoAlreadyExist struct {
	args errutil.Args
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

func (db *repos) Create(ctx context.Context, ownerID int64, opts CreateRepoOptions) (*Repository, error) {
	err := isRepoNameAllowed(opts.Name)
	if err != nil {
		return nil, err
	}

	_, err = db.GetByName(ctx, ownerID, opts.Name)
	if err == nil {
		return nil, ErrRepoAlreadyExist{
			args: errutil.Args{
				"ownerID": ownerID,
				"name":    opts.Name,
			},
		}
	} else if !IsErrRepoNotExist(err) {
		return nil, err
	}

	repo := &Repository{
		OwnerID:       ownerID,
		LowerName:     strings.ToLower(opts.Name),
		Name:          opts.Name,
		Description:   opts.Description,
		DefaultBranch: opts.DefaultBranch,
		IsPrivate:     opts.Private,
		IsMirror:      opts.Mirror,
		EnableWiki:    opts.EnableWiki,
		EnableIssues:  opts.EnableIssues,
		EnablePulls:   opts.EnablePulls,
		IsFork:        opts.Fork,
		ForkID:        opts.ForkID,
	}
	return repo, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err = tx.Create(repo).Error
		if err != nil {
			return errors.Wrap(err, "create")
		}

		err = NewReposStore(tx).Watch(ctx, ownerID, repo.ID)
		if err != nil {
			return errors.Wrap(err, "watch")
		}
		return nil
	})
}

func (db *repos) GetByCollaboratorID(ctx context.Context, collaboratorID int64, limit int, orderBy string) ([]*Repository, error) {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM repository
		JOIN access ON access.repo_id = repository.id AND access.user_id = @collaboratorID
		WHERE access.mode >= @accessModeRead
		ORDER BY @orderBy
		LIMIT @limit
	*/
	var repos []*Repository
	return repos, db.WithContext(ctx).
		Joins("JOIN access ON access.repo_id = repository.id AND access.user_id = ?", collaboratorID).
		Where("access.mode >= ?", AccessModeRead).
		Order(orderBy).
		Limit(limit).
		Find(&repos).
		Error
}

func (db *repos) GetByCollaboratorIDWithAccessMode(ctx context.Context, collaboratorID int64) (map[*Repository]AccessMode, error) {
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
	err := db.WithContext(ctx).
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

var _ errutil.NotFound = (*ErrRepoNotExist)(nil)

type ErrRepoNotExist struct {
	args errutil.Args
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

func (db *repos) GetByID(ctx context.Context, id int64) (*Repository, error) {
	repo := new(Repository)
	err := db.WithContext(ctx).Where("id = ?", id).First(repo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRepoNotExist{errutil.Args{"repoID": id}}
		}
		return nil, err
	}
	return repo, nil
}

func (db *repos) GetByName(ctx context.Context, ownerID int64, name string) (*Repository, error) {
	repo := new(Repository)
	err := db.WithContext(ctx).
		Where("owner_id = ? AND lower_name = ?", ownerID, strings.ToLower(name)).
		First(repo).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrRepoNotExist{
				args: errutil.Args{
					"ownerID": ownerID,
					"name":    name,
				},
			}
		}
		return nil, err
	}
	return repo, nil
}

func (db *repos) recountStars(tx *gorm.DB, userID, repoID int64) error {
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

func (db *repos) Star(ctx context.Context, userID, repoID int64) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		s := &Star{
			UserID: userID,
			RepoID: repoID,
		}
		result := tx.FirstOrCreate(s, s)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}

		return db.recountStars(tx, userID, repoID)
	})
}

func (db *repos) Touch(ctx context.Context, id int64) error {
	return db.WithContext(ctx).
		Model(new(Repository)).
		Where("id = ?", id).
		Updates(map[string]any{
			"is_bare":      false,
			"updated_unix": db.NowFunc().Unix(),
		}).
		Error
}

func (db *repos) ListWatches(ctx context.Context, repoID int64) ([]*Watch, error) {
	var watches []*Watch
	return watches, db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&watches).Error
}

func (db *repos) recountWatches(tx *gorm.DB, repoID int64) error {
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

func (db *repos) Watch(ctx context.Context, userID, repoID int64) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		w := &Watch{
			UserID: userID,
			RepoID: repoID,
		}
		result := tx.FirstOrCreate(w, w)
		if result.Error != nil {
			return errors.Wrap(result.Error, "upsert")
		} else if result.RowsAffected <= 0 {
			return nil // Relation already exists
		}

		return db.recountWatches(tx, repoID)
	})
}

func (db *repos) HasForkedBy(ctx context.Context, repoID, userID int64) bool {
	var count int64
	db.WithContext(ctx).Model(new(Repository)).Where("owner_id = ? AND fork_id = ?", userID, repoID).Count(&count)
	return count > 0
}
