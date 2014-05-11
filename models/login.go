// Copyright github.com/juju2013. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/go-xorm/core"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/auth/ldap"
)

// Login types.
const (
	LT_NOTYPE = iota
	LT_PLAIN
	LT_LDAP
	LT_SMTP
)

var (
	ErrAuthenticationAlreadyExist = errors.New("Authentication already exist")
	ErrAuthenticationNotExist     = errors.New("Authentication does not exist")
	ErrAuthenticationUserUsed     = errors.New("Authentication has been used by some users")
)

var LoginTypes = map[int]string{
	LT_LDAP: "LDAP",
	LT_SMTP: "SMTP",
}

var _ core.Conversion = &LDAPConfig{}
var _ core.Conversion = &SMTPConfig{}

type LDAPConfig struct {
	ldap.Ldapsource
}

// implement
func (cfg *LDAPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, &cfg.Ldapsource)
}

func (cfg *LDAPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg.Ldapsource)
}

type SMTPConfig struct {
	Auth string
	Host string
	Port int
	TLS  bool
}

// implement
func (cfg *SMTPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, cfg)
}

func (cfg *SMTPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

type LoginSource struct {
	Id                int64
	Type              int
	Name              string          `xorm:"unique"`
	IsActived         bool            `xorm:"not null default false"`
	Cfg               core.Conversion `xorm:"TEXT"`
	Created           time.Time       `xorm:"created"`
	Updated           time.Time       `xorm:"updated"`
	AllowAutoRegister bool            `xorm:"not null default false"`
}

func (source *LoginSource) TypeString() string {
	return LoginTypes[source.Type]
}

func (source *LoginSource) LDAP() *LDAPConfig {
	return source.Cfg.(*LDAPConfig)
}

func (source *LoginSource) SMTP() *SMTPConfig {
	return source.Cfg.(*SMTPConfig)
}

// for xorm callback
func (source *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	if colName == "type" {
		ty := (*val).(int64)
		switch ty {
		case LT_LDAP:
			source.Cfg = new(LDAPConfig)
		case LT_SMTP:
			source.Cfg = new(SMTPConfig)
		}
	}
}

func GetAuths() ([]*LoginSource, error) {
	var auths = make([]*LoginSource, 0)
	err := orm.Find(&auths)
	return auths, err
}

func GetLoginSourceById(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	has, err := orm.Id(id).Get(source)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, ErrAuthenticationNotExist
	}
	return source, nil
}

func AddSource(source *LoginSource) error {
	_, err := orm.Insert(source)
	return err
}

func UpdateSource(source *LoginSource) error {
	_, err := orm.AllCols().Id(source.Id).Update(source)
	return err
}

func DelLoginSource(source *LoginSource) error {
	cnt, err := orm.Count(&User{LoginSource: source.Id})
	if err != nil {
		return err
	}
	if cnt > 0 {
		return ErrAuthenticationUserUsed
	}
	_, err = orm.Id(source.Id).Delete(&LoginSource{})
	return err
}

// login a user
func LoginUser(uname, passwd string) (*User, error) {
	var u *User
	if strings.Contains(uname, "@") {
		u = &User{Email: uname}
	} else {
		u = &User{LowerName: strings.ToLower(uname)}
	}

	has, err := orm.Get(u)
	if err != nil {
		return nil, err
	}

	if u.LoginType == LT_NOTYPE {
		if has {
			u.LoginType = LT_PLAIN
		}
	}

	// for plain login, user must have existed.
	if u.LoginType == LT_PLAIN {
		if !has {
			return nil, ErrUserNotExist
		}

		newUser := &User{Passwd: passwd, Salt: u.Salt}
		newUser.EncodePasswd()
		if u.Passwd != newUser.Passwd {
			return nil, ErrUserNotExist
		}
		return u, nil
	} else {
		if !has {
			var sources []LoginSource
			cond := &LoginSource{IsActived: true, AllowAutoRegister: true}
			err = orm.UseBool().Find(&sources, cond)
			if err != nil {
				return nil, err
			}

			for _, source := range sources {
				if source.Type == LT_LDAP {
					u, err := LoginUserLdapSource(nil, uname, passwd,
						source.Id, source.Cfg.(*LDAPConfig), true)
					if err == nil {
						return u, err
					}
				} else if source.Type == LT_SMTP {
					u, err := LoginUserSMTPSource(nil, uname, passwd,
						source.Id, source.Cfg.(*SMTPConfig), true)

					if err == nil {
						return u, err
					}
				}
			}

			return nil, ErrUserNotExist
		}

		var source LoginSource
		hasSource, err := orm.Id(u.LoginSource).Get(&source)
		if err != nil {
			return nil, err
		}
		if !hasSource {
			return nil, ErrLoginSourceNotExist
		}

		if !source.IsActived {
			return nil, ErrLoginSourceNotActived
		}

		switch u.LoginType {
		case LT_LDAP:
			return LoginUserLdapSource(u, u.LoginName, passwd,
				source.Id, source.Cfg.(*LDAPConfig), false)
		case LT_SMTP:
			return LoginUserSMTPSource(u, u.LoginName, passwd,
				source.Id, source.Cfg.(*SMTPConfig), false)
		}
		return nil, ErrUnsupportedLoginType
	}
}

// Query if name/passwd can login against the LDAP direcotry pool
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserLdapSource(user *User, name, passwd string, sourceId int64, cfg *LDAPConfig, autoRegister bool) (*User, error) {
	mail, logged := cfg.Ldapsource.SearchEntry(name, passwd)
	if !logged {
		// user not in LDAP, do nothing
		return nil, ErrUserNotExist
	}
	if !autoRegister {
		return user, nil
	}

	// fake a local user creation
	user = &User{
		LowerName:   strings.ToLower(name),
		Name:        strings.ToLower(name),
		LoginType:   LT_LDAP,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       mail,
	}

	return RegisterUser(user)
}

type loginAuth struct {
	username, password string
}

func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		}
	}
	return nil, nil
}

var (
	SMTP_PLAIN = "PLAIN"
	SMTP_LOGIN = "LOGIN"
	SMTPAuths  = []string{SMTP_PLAIN, SMTP_LOGIN}
)

func SmtpAuth(addr string, a smtp.Auth, tls bool) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()

	if tls {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(nil); err != nil {
				return err
			}
		} else {
			return errors.New("smtp server unsupported tls")
		}
	}

	if ok, _ := c.Extension("AUTH"); ok {
		if err = c.Auth(a); err != nil {
			return err
		}
		return nil
	} else {
		return ErrUnsupportedLoginType
	}
}

// Query if name/passwd can login against the LDAP direcotry pool
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserSMTPSource(user *User, name, passwd string, sourceId int64, cfg *SMTPConfig, autoRegister bool) (*User, error) {
	var auth smtp.Auth
	if cfg.Auth == SMTP_PLAIN {
		auth = smtp.PlainAuth("", name, passwd, cfg.Host)
	} else if cfg.Auth == SMTP_LOGIN {
		auth = LoginAuth(name, passwd)
	} else {
		return nil, errors.New("Unsupported smtp auth type")
	}

	err := SmtpAuth(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), auth, cfg.TLS)
	if err != nil {
		return nil, err
	}

	if !autoRegister {
		return user, nil
	}

	var loginName = name
	idx := strings.Index(name, "@")
	if idx > -1 {
		loginName = name[:idx]
	}
	// fake a local user creation
	user = &User{
		LowerName:   strings.ToLower(loginName),
		Name:        strings.ToLower(loginName),
		LoginType:   LT_SMTP,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       name,
	}

	return RegisterUser(user)
}
