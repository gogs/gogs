// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/errutil"
)

// LoginSourcesStore is the persistent interface for login sources.
//
// NOTE: All methods are sorted in alphabetical order.
type LoginSourcesStore interface {
	// Create creates a new login source and persist to database.
	// It returns ErrLoginSourceAlreadyExist when a login source with same name already exists.
	Create(opts CreateLoginSourceOpts) (*LoginSource, error)
	// Count returns the total number of login sources.
	Count() int64
	// DeleteByID deletes a login source by given ID.
	// It returns ErrLoginSourceInUse if at least one user is associated with the login source.
	DeleteByID(id int64) error
	// GetByID returns the login source with given ID.
	// It returns ErrLoginSourceNotExist when not found.
	GetByID(id int64) (*LoginSource, error)
	// List returns a list of login sources filtered by options.
	List(opts ListLoginSourceOpts) ([]*LoginSource, error)
	// ResetNonDefault clears default flag for all the other login sources.
	ResetNonDefault(source *LoginSource) error
	// Save persists all values of given login source to database or local file.
	// The Updated field is set to current time automatically.
	Save(t *LoginSource) error
}

var LoginSources LoginSourcesStore

// LoginSource represents an external way for authorizing users.
type LoginSource struct {
	ID        int64
	Type      auth.Type
	Name      string        `xorm:"UNIQUE" gorm:"UNIQUE"`
	IsActived bool          `xorm:"NOT NULL DEFAULT false" gorm:"NOT NULL"`
	IsDefault bool          `xorm:"DEFAULT false"`
	Provider  auth.Provider `xorm:"-" gorm:"-"`
	Config    string        `xorm:"TEXT cfg" gorm:"COLUMN:cfg;TYPE:TEXT" json:"RawConfig"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64

	File loginSourceFileStore `xorm:"-" gorm:"-" json:"-"`
}

// NOTE: This is a GORM save hook.
func (s *LoginSource) BeforeSave(_ *gorm.DB) (err error) {
	if s.Provider == nil {
		return nil
	}
	s.Config, err = jsoniter.MarshalToString(s.Provider.Config())
	return err
}

// NOTE: This is a GORM create hook.
func (s *LoginSource) BeforeCreate(tx *gorm.DB) error {
	if s.CreatedUnix == 0 {
		s.CreatedUnix = tx.NowFunc().Unix()
		s.UpdatedUnix = s.CreatedUnix
	}
	return nil
}

// NOTE: This is a GORM update hook.
func (s *LoginSource) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// NOTE: This is a GORM query hook.
func (s *LoginSource) AfterFind(_ *gorm.DB) error {
	s.Created = time.Unix(s.CreatedUnix, 0).Local()
	s.Updated = time.Unix(s.UpdatedUnix, 0).Local()

	switch s.Type {
	case auth.LDAP:
		var cfg ldap.Config
		err := jsoniter.UnmarshalFromString(s.Config, &cfg)
		if err != nil {
			return err
		}
		s.Provider = ldap.NewProvider(false, &cfg)

	case auth.DLDAP:
		var cfg ldap.Config
		err := jsoniter.UnmarshalFromString(s.Config, &cfg)
		if err != nil {
			return err
		}
		s.Provider = ldap.NewProvider(true, &cfg)

	case auth.SMTP:
		var cfg smtp.Config
		err := jsoniter.UnmarshalFromString(s.Config, &cfg)
		if err != nil {
			return err
		}
		s.Provider = smtp.NewProvider(&cfg)

	case auth.PAM:
		var cfg pam.Config
		err := jsoniter.UnmarshalFromString(s.Config, &cfg)
		if err != nil {
			return err
		}
		s.Provider = pam.NewProvider(&cfg)

	case auth.GitHub:
		var cfg github.Config
		err := jsoniter.UnmarshalFromString(s.Config, &cfg)
		if err != nil {
			return err
		}
		s.Provider = github.NewProvider(&cfg)

	default:
		return fmt.Errorf("unrecognized login source type: %v", s.Type)
	}
	return nil
}

func (s *LoginSource) TypeName() string {
	return auth.Name(s.Type)
}

func (s *LoginSource) IsLDAP() bool {
	return s.Type == auth.LDAP
}

func (s *LoginSource) IsDLDAP() bool {
	return s.Type == auth.DLDAP
}

func (s *LoginSource) IsSMTP() bool {
	return s.Type == auth.SMTP
}

func (s *LoginSource) IsPAM() bool {
	return s.Type == auth.PAM
}

func (s *LoginSource) IsGitHub() bool {
	return s.Type == auth.GitHub
}

func (s *LoginSource) LDAP() *ldap.Config {
	return s.Provider.Config().(*ldap.Config)
}

func (s *LoginSource) SMTP() *smtp.Config {
	return s.Provider.Config().(*smtp.Config)
}

func (s *LoginSource) PAM() *pam.Config {
	return s.Provider.Config().(*pam.Config)
}

func (s *LoginSource) GitHub() *github.Config {
	return s.Provider.Config().(*github.Config)
}

var _ LoginSourcesStore = (*loginSources)(nil)

type loginSources struct {
	*gorm.DB
	files loginSourceFilesStore
}

type CreateLoginSourceOpts struct {
	Type      auth.Type
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
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	source := &LoginSource{
		Type:      opts.Type,
		Name:      opts.Name,
		IsActived: opts.Activated,
		IsDefault: opts.Default,
	}
	source.Config, err = jsoniter.MarshalToString(opts.Config)
	if err != nil {
		return nil, err
	}
	return source, db.DB.Create(source).Error
}

func (db *loginSources) Count() int64 {
	var count int64
	db.Model(new(LoginSource)).Count(&count)
	return count + int64(db.files.Len())
}

type ErrLoginSourceInUse struct {
	args errutil.Args
}

func IsErrLoginSourceInUse(err error) bool {
	_, ok := err.(ErrLoginSourceInUse)
	return ok
}

func (err ErrLoginSourceInUse) Error() string {
	return fmt.Sprintf("login source is still used by some users: %v", err.args)
}

func (db *loginSources) DeleteByID(id int64) error {
	var count int64
	err := db.Model(new(User)).Where("login_source = ?", id).Count(&count).Error
	if err != nil {
		return err
	} else if count > 0 {
		return ErrLoginSourceInUse{args: errutil.Args{"id": id}}
	}

	return db.Where("id = ?", id).Delete(new(LoginSource)).Error
}

func (db *loginSources) GetByID(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	err := db.Where("id = ?", id).First(source).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return db.files.GetByID(id)
		}
		return nil, err
	}
	return source, nil
}

type ListLoginSourceOpts struct {
	// Whether to only include activated login sources.
	OnlyActivated bool
}

func (db *loginSources) List(opts ListLoginSourceOpts) ([]*LoginSource, error) {
	var sources []*LoginSource
	query := db.Order("id ASC")
	if opts.OnlyActivated {
		query = query.Where("is_actived = ?", true)
	}
	err := query.Find(&sources).Error
	if err != nil {
		return nil, err
	}

	return append(sources, db.files.List(opts)...), nil
}

func (db *loginSources) ResetNonDefault(dflt *LoginSource) error {
	err := db.Model(new(LoginSource)).Where("id != ?", dflt.ID).Updates(map[string]interface{}{"is_default": false}).Error
	if err != nil {
		return err
	}

	for _, source := range db.files.List(ListLoginSourceOpts{}) {
		if source.File != nil && source.ID != dflt.ID {
			source.File.SetGeneral("is_default", "false")
			if err = source.File.Save(); err != nil {
				return errors.Wrap(err, "save file")
			}
		}
	}

	db.files.Update(dflt)
	return nil
}

func (db *loginSources) Save(source *LoginSource) error {
	if source.File == nil {
		return db.DB.Save(source).Error
	}

	source.File.SetGeneral("name", source.Name)
	source.File.SetGeneral("is_activated", strconv.FormatBool(source.IsActived))
	source.File.SetGeneral("is_default", strconv.FormatBool(source.IsDefault))
	if err := source.File.SetConfig(source.Provider.Config()); err != nil {
		return errors.Wrap(err, "set config")
	} else if err = source.File.Save(); err != nil {
		return errors.Wrap(err, "save file")
	}
	return nil
}
