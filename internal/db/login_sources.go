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
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/auth/ldap"
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
	Type      LoginType
	Name      string      `xorm:"UNIQUE" gorm:"UNIQUE"`
	IsActived bool        `xorm:"NOT NULL DEFAULT false" gorm:"NOT NULL"`
	IsDefault bool        `xorm:"DEFAULT false"`
	Config    interface{} `xorm:"-" gorm:"-"`
	RawConfig string      `xorm:"TEXT cfg" gorm:"COLUMN:cfg;TYPE:TEXT"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" gorm:"-" json:"-"`
	UpdatedUnix int64

	File loginSourceFileStore `xorm:"-" gorm:"-" json:"-"`
}

// NOTE: This is a GORM save hook.
func (s *LoginSource) BeforeSave() (err error) {
	if s.Config == nil {
		return nil
	}
	s.RawConfig, err = jsoniter.MarshalToString(s.Config)
	return err
}

// NOTE: This is a GORM create hook.
func (s *LoginSource) BeforeCreate() {
	if s.CreatedUnix > 0 {
		return
	}
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

func (s *LoginSource) TypeName() string {
	return LoginNames[s.Type]
}

func (s *LoginSource) IsLDAP() bool {
	return s.Type == LoginLDAP
}

func (s *LoginSource) IsDLDAP() bool {
	return s.Type == LoginDLDAP
}

func (s *LoginSource) IsSMTP() bool {
	return s.Type == LoginSMTP
}

func (s *LoginSource) IsPAM() bool {
	return s.Type == LoginPAM
}

func (s *LoginSource) IsGitHub() bool {
	return s.Type == LoginGitHub
}

func (s *LoginSource) HasTLS() bool {
	return ((s.IsLDAP() || s.IsDLDAP()) &&
		s.LDAP().SecurityProtocol > ldap.SecurityProtocolUnencrypted) ||
		s.IsSMTP()
}

func (s *LoginSource) UseTLS() bool {
	switch s.Type {
	case LoginLDAP, LoginDLDAP:
		return s.LDAP().SecurityProtocol != ldap.SecurityProtocolUnencrypted
	case LoginSMTP:
		return s.SMTP().TLS
	}

	return false
}

func (s *LoginSource) SkipVerify() bool {
	switch s.Type {
	case LoginLDAP, LoginDLDAP:
		return s.LDAP().SkipVerify
	case LoginSMTP:
		return s.SMTP().SkipVerify
	}

	return false
}

func (s *LoginSource) LDAP() *LDAPConfig {
	return s.Config.(*LDAPConfig)
}

func (s *LoginSource) SMTP() *SMTPConfig {
	return s.Config.(*SMTPConfig)
}

func (s *LoginSource) PAM() *PAMConfig {
	return s.Config.(*PAMConfig)
}

func (s *LoginSource) GitHub() *GitHubConfig {
	return s.Config.(*GitHubConfig)
}

var _ LoginSourcesStore = (*loginSources)(nil)

type loginSources struct {
	*gorm.DB
	files loginSourceFilesStore
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
		if gorm.IsRecordNotFoundError(err) {
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
	if err := source.File.SetConfig(source.Config); err != nil {
		return errors.Wrap(err, "set config")
	} else if err = source.File.Save(); err != nil {
		return errors.Wrap(err, "save file")
	}
	return nil
}
