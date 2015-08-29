// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/go-xorm/core"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/auth/pam"
	"github.com/gogits/gogs/modules/log"
)

type LoginType int

const (
	NOTYPE LoginType = iota
	PLAIN
	LDAP
	SMTP
	PAM
)

var (
	ErrAuthenticationAlreadyExist = errors.New("Authentication already exist")
	ErrAuthenticationNotExist     = errors.New("Authentication does not exist")
	ErrAuthenticationUserUsed     = errors.New("Authentication has been used by some users")
)

var LoginTypes = map[LoginType]string{
	LDAP: "LDAP",
	SMTP: "SMTP",
	PAM:  "PAM",
}

// Ensure structs implemented interface.
var (
	_ core.Conversion = &LDAPConfig{}
	_ core.Conversion = &SMTPConfig{}
	_ core.Conversion = &PAMConfig{}
)

type LDAPConfig struct {
	ldap.Ldapsource
}

func (cfg *LDAPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, &cfg.Ldapsource)
}

func (cfg *LDAPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg.Ldapsource)
}

type SMTPConfig struct {
	Auth       string
	Host       string
	Port       int
	TLS        bool
	SkipVerify bool
}

func (cfg *SMTPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, cfg)
}

func (cfg *SMTPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

type PAMConfig struct {
	ServiceName string // pam service (e.g. system-auth)
}

func (cfg *PAMConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, &cfg)
}

func (cfg *PAMConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

type LoginSource struct {
	ID                int64 `xorm:"pk autoincr"`
	Type              LoginType
	Name              string          `xorm:"UNIQUE"`
	IsActived         bool            `xorm:"NOT NULL DEFAULT false"`
	Cfg               core.Conversion `xorm:"TEXT"`
	AllowAutoRegister bool            `xorm:"NOT NULL DEFAULT false"`
	Created           time.Time       `xorm:"CREATED"`
	Updated           time.Time       `xorm:"UPDATED"`
}

func (source *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	switch colName {
	case "type":
		switch LoginType((*val).(int64)) {
		case LDAP:
			source.Cfg = new(LDAPConfig)
		case SMTP:
			source.Cfg = new(SMTPConfig)
		case PAM:
			source.Cfg = new(PAMConfig)
		}
	}
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

func (source *LoginSource) PAM() *PAMConfig {
	return source.Cfg.(*PAMConfig)
}

func CreateSource(source *LoginSource) error {
	_, err := x.Insert(source)
	return err
}

func GetAuths() ([]*LoginSource, error) {
	auths := make([]*LoginSource, 0, 5)
	return auths, x.Find(&auths)
}

func GetLoginSourceByID(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	has, err := x.Id(id).Get(source)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAuthenticationNotExist
	}
	return source, nil
}

func UpdateSource(source *LoginSource) error {
	_, err := x.Id(source.ID).AllCols().Update(source)
	return err
}

func DelLoginSource(source *LoginSource) error {
	cnt, err := x.Count(&User{LoginSource: source.ID})
	if err != nil {
		return err
	}
	if cnt > 0 {
		return ErrAuthenticationUserUsed
	}
	_, err = x.Id(source.ID).Delete(&LoginSource{})
	return err
}

// UserSignIn validates user name and password.
func UserSignIn(uname, passwd string) (*User, error) {
	u := new(User)
	if strings.Contains(uname, "@") {
		u = &User{Email: uname}
	} else {
		u = &User{LowerName: strings.ToLower(uname)}
	}

	has, err := x.Get(u)
	if err != nil {
		return nil, err
	}

	if u.LoginType == NOTYPE && has {
		u.LoginType = PLAIN
	}

	// For plain login, user must exist to reach this line.
	// Now verify password.
	if u.LoginType == PLAIN {
		if !u.ValidatePassword(passwd) {
			return nil, ErrUserNotExist{u.Id, u.Name}
		}
		return u, nil
	}

	if !has {
		var sources []LoginSource
		if err = x.UseBool().Find(&sources,
			&LoginSource{IsActived: true, AllowAutoRegister: true}); err != nil {
			return nil, err
		}

		for _, source := range sources {
			if source.Type == LDAP {
				u, err := LoginUserLdapSource(nil, uname, passwd,
					source.ID, source.Cfg.(*LDAPConfig), true)
				if err == nil {
					return u, nil
				}
				log.Warn("Fail to login(%s) by LDAP(%s): %v", uname, source.Name, err)
			} else if source.Type == SMTP {
				u, err := LoginUserSMTPSource(nil, uname, passwd,
					source.ID, source.Cfg.(*SMTPConfig), true)
				if err == nil {
					return u, nil
				}
				log.Warn("Fail to login(%s) by SMTP(%s): %v", uname, source.Name, err)
			} else if source.Type == PAM {
				u, err := LoginUserPAMSource(nil, uname, passwd,
					source.ID, source.Cfg.(*PAMConfig), true)
				if err == nil {
					return u, nil
				}
				log.Warn("Fail to login(%s) by PAM(%s): %v", uname, source.Name, err)
			}
		}

		return nil, ErrUserNotExist{u.Id, u.Name}
	}

	var source LoginSource
	hasSource, err := x.Id(u.LoginSource).Get(&source)
	if err != nil {
		return nil, err
	} else if !hasSource {
		return nil, ErrLoginSourceNotExist
	} else if !source.IsActived {
		return nil, ErrLoginSourceNotActived
	}

	switch u.LoginType {
	case LDAP:
		return LoginUserLdapSource(u, u.LoginName, passwd, source.ID, source.Cfg.(*LDAPConfig), false)
	case SMTP:
		return LoginUserSMTPSource(u, u.LoginName, passwd, source.ID, source.Cfg.(*SMTPConfig), false)
	case PAM:
		return LoginUserPAMSource(u, u.LoginName, passwd, source.ID, source.Cfg.(*PAMConfig), false)
	}
	return nil, ErrUnsupportedLoginType
}

// Query if name/passwd can login against the LDAP directory pool
// Create a local user if success
// Return the same LoginUserPlain semantic
// FIXME: https://github.com/gogits/gogs/issues/672
func LoginUserLdapSource(u *User, name, passwd string, sourceId int64, cfg *LDAPConfig, autoRegister bool) (*User, error) {
	fn, sn, mail, admin, logged := cfg.Ldapsource.SearchEntry(name, passwd)
	if !logged {
		// User not in LDAP, do nothing
		return nil, ErrUserNotExist{0, name}
	}

	if !autoRegister {
		return u, nil
	}

	// Fallback.
	if len(mail) == 0 {
		mail = fmt.Sprintf("%s@localhost", name)
	}

	u = &User{
		LowerName:   strings.ToLower(name),
		Name:        name,
		FullName:    fn + " " + sn,
		LoginType:   LDAP,
		LoginSource: sourceId,
		LoginName:   name,
		Passwd:      passwd,
		Email:       mail,
		IsAdmin:     admin,
		IsActive:    true,
	}
	return u, CreateUser(u)
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

const (
	SMTP_PLAIN = "PLAIN"
	SMTP_LOGIN = "LOGIN"
)

var (
	SMTPAuths = []string{SMTP_PLAIN, SMTP_LOGIN}
)

func SMTPAuth(a smtp.Auth, cfg *SMTPConfig) error {
	c, err := smtp.Dial(fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Hello("gogs"); err != nil {
		return err
	}

	if cfg.TLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(&tls.Config{
				InsecureSkipVerify: cfg.SkipVerify,
				ServerName:         cfg.Host,
			}); err != nil {
				return err
			}
		} else {
			return errors.New("SMTP server unsupports TLS")
		}
	}

	if ok, _ := c.Extension("AUTH"); ok {
		if err = c.Auth(a); err != nil {
			return err
		}
		return nil
	}
	return ErrUnsupportedLoginType
}

// Query if name/passwd can login against the LDAP directory pool
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserSMTPSource(u *User, name, passwd string, sourceId int64, cfg *SMTPConfig, autoRegister bool) (*User, error) {
	var auth smtp.Auth
	if cfg.Auth == SMTP_PLAIN {
		auth = smtp.PlainAuth("", name, passwd, cfg.Host)
	} else if cfg.Auth == SMTP_LOGIN {
		auth = LoginAuth(name, passwd)
	} else {
		return nil, errors.New("Unsupported SMTP auth type")
	}

	if err := SMTPAuth(auth, cfg); err != nil {
		if strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, ErrUserNotExist{u.Id, u.Name}
		}
		return nil, err
	}

	if !autoRegister {
		return u, nil
	}

	var loginName = name
	idx := strings.Index(name, "@")
	if idx > -1 {
		loginName = name[:idx]
	}
	// fake a local user creation
	u = &User{
		LowerName:   strings.ToLower(loginName),
		Name:        strings.ToLower(loginName),
		LoginType:   SMTP,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       name,
	}
	err := CreateUser(u)
	return u, err
}

// Query if name/passwd can login against PAM
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserPAMSource(u *User, name, passwd string, sourceId int64, cfg *PAMConfig, autoRegister bool) (*User, error) {
	if err := pam.PAMAuth(cfg.ServiceName, name, passwd); err != nil {
		if strings.Contains(err.Error(), "Authentication failure") {
			return nil, ErrUserNotExist{u.Id, u.Name}
		}
		return nil, err
	}

	if !autoRegister {
		return u, nil
	}

	// fake a local user creation
	u = &User{
		LowerName:   strings.ToLower(name),
		Name:        strings.ToLower(name),
		LoginType:   PAM,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       name,
	}
	err := CreateUser(u)
	return u, err
}
