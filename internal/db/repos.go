// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	api "github.com/gogs/go-gogs-client"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/repoutil"
)

// ReposStore is the persistent interface for repositories.
//
// NOTE: All methods are sorted in alphabetical order.
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
	// GetByName returns the repository with given owner and name. It returns
	// ErrRepoNotExist when not found.
	GetByName(ctx context.Context, ownerID int64, name string) (*Repository, error)
	// Touch updates the updated time to the current time and removes the bare state
	// of the given repository.
	Touch(ctx context.Context, id int64) error
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
	return repo, db.WithContext(ctx).Create(repo).Error
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
