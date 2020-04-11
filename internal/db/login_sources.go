// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	jsoniter "github.com/json-iterator/go"

	"gogs.io/gogs/internal/errutil"
)

// LoginSourcesStore is the persistent interface for login sources.
//
// NOTE: All methods are sorted in alphabetical order.
type LoginSourcesStore interface {
	// Create creates a new login source and persist to database.
	// It returns ErrLoginSourceAlreadyExist when a login source with same name already exists.
	Create(opts CreateLoginSourceOpts) (*LoginSource, error)
	// GetByID returns the login source with given ID.
	// It returns ErrLoginSourceNotExist when not found.
	GetByID(id int64) (*LoginSource, error)
	// Save persists all values of given login source to database or local file.
	// The Updated field is set to current time automatically.
	Save(t *LoginSource) error
}

var LoginSources LoginSourcesStore

// NOTE: This is a GORM save hook.
func (s *LoginSource) BeforeSave() (err error) {
	s.RawConfig, err = jsoniter.MarshalToString(s.Config)
	return err
}

// NOTE: This is a GORM create hook.
func (s *LoginSource) BeforeCreate() {
	s.CreatedUnix = gorm.NowFunc().Unix()
	s.UpdatedUnix = s.CreatedUnix
}

// NOTE: This is a GORM update hook.
func (s *LoginSource) BeforeUpdate() {
	s.UpdatedUnix = gorm.NowFunc().Unix()
}

// NOTE: This is a GORM query hook.
func (s *LoginSource) AfterFind() error {
	s.Created = time.Unix(s.CreatedUnix, 0).Local()
	s.Updated = time.Unix(s.UpdatedUnix, 0).Local()

	switch s.Type {
	case LoginLDAP, LoginDLDAP:
		s.Config = new(LDAPConfig)
	case LoginSMTP:
		s.Config = new(SMTPConfig)
	case LoginPAM:
		s.Config = new(PAMConfig)
	case LoginGitHub:
		s.Config = new(GitHubConfig)
	default:
		return fmt.Errorf("unrecognized login source type: %v", s.Type)
	}
	return jsoniter.UnmarshalFromString(s.RawConfig, s.Config)
}

var _ LoginSourcesStore = (*loginSources)(nil)

type loginSources struct {
	*gorm.DB
}

type CreateLoginSourceOpts struct {
	Type      LoginType
	Name      string
	Activated bool
	Default   bool
	Config    interface{}
}

type ErrLoginSourceAlreadyExist struct {
	args errutil.Args
}

func IsErrLoginSourceAlreadyExist(err error) bool {
	_, ok := err.(ErrLoginSourceAlreadyExist)
	return ok
}

func (err ErrLoginSourceAlreadyExist) Error() string {
	return fmt.Sprintf("login source already exists: %v", err.args)
}

func (db *loginSources) Create(opts CreateLoginSourceOpts) (*LoginSource, error) {
	err := db.Where("name = ?", opts.Name).First(new(LoginSource)).Error
	if err == nil {
		return nil, ErrLoginSourceAlreadyExist{args: errutil.Args{"name": opts.Name}}
	} else if !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}

	source := &LoginSource{
		Type:      opts.Type,
		Name:      opts.Name,
		IsActived: opts.Activated,
		IsDefault: opts.Default,
		Config:    opts.Config,
	}
	return source, db.DB.Create(source).Error
}

func (db *loginSources) GetByID(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	err := db.Where("id = ?", id).First(source).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return localLoginSources.GetLoginSourceByID(id)
		}
		return nil, err
	}
	return source, nil
}

func (db *loginSources) Save(source *LoginSource) error {
	if source.LocalFile == nil {
		return db.DB.Save(source).Error
	}

	source.LocalFile.SetGeneral("name", source.Name)
	source.LocalFile.SetGeneral("is_activated", strconv.FormatBool(source.IsActived))
	source.LocalFile.SetGeneral("is_default", strconv.FormatBool(source.IsDefault))
	if err := source.LocalFile.SetConfig(source.Config); err != nil {
		return fmt.Errorf("LocalFile.SetConfig: %v", err)
	} else if err = source.LocalFile.Save(); err != nil {
		return fmt.Errorf("LocalFile.Save: %v", err)
	}
	return nil
}
