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
	"github.com/gogits/gogs/modules/log"
)

type LoginType int

const (
	NOTYPE LoginType = iota
	PLAIN
	LDAP
	SMTP
)

var (
	ErrAuthenticationAlreadyExist = errors.New("Authentication already exist")
	ErrAuthenticationNotExist     = errors.New("Authentication does not exist")
	ErrAuthenticationUserUsed     = errors.New("Authentication has been used by some users")
)

var LoginTypes = map[LoginType]string{
	LDAP: "LDAP",
	SMTP: "SMTP",
}

// Ensure structs implmented interface.
var (
	_ core.Conversion = &LDAPConfig{}
	_ core.Conversion = &SMTPConfig{}
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
	Auth string
	Host string
	Port int
	TLS  bool
}

func (cfg *SMTPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, cfg)
}

func (cfg *SMTPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

type LoginSource struct {
	Id                int64
	Type              LoginType
	Name              string          `xorm:"UNIQUE"`
	IsActived         bool            `xorm:"NOT NULL DEFAULT false"`
	Cfg               core.Conversion `xorm:"TEXT"`
	AllowAutoRegister bool            `xorm:"NOT NULL DEFAULT false"`
	Created           time.Time       `xorm:"CREATED"`
	Updated           time.Time       `xorm:"UPDATED"`
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

func (source *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	if colName == "type" {
		ty := (*val).(int64)
		switch LoginType(ty) {
		case LDAP:
			source.Cfg = new(LDAPConfig)
		case SMTP:
			source.Cfg = new(SMTPConfig)
		}
	}
}

func CreateSource(source *LoginSource) error {
	_, err := x.Insert(source)
	return err
}

func GetAuths() ([]*LoginSource, error) {
	var auths = make([]*LoginSource, 0, 5)
	err := x.Find(&auths)
	return auths, err
}

func GetLoginSourceById(id int64) (*LoginSource, error) {
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
	_, err := x.Id(source.Id).AllCols().Update(source)
	return err
}

func DelLoginSource(source *LoginSource) error {
	cnt, err := x.Count(&User{LoginSource: source.Id})
	if err != nil {
		return err
	}
	if cnt > 0 {
		return ErrAuthenticationUserUsed
	}
	_, err = x.Id(source.Id).Delete(&LoginSource{})
	return err
}

// UserSignIn validates user name and password.
func UserSignIn(uname, passwd string) (*User, error) {
	var u *User
	if strings.Contains(uname, "@") {
		u = &User{Email: uname}
	} else {
		u = &User{LowerName: strings.ToLower(uname)}
	}

	has, err := x.Get(u)
	if err != nil {
		return nil, err
	}

	if u.LoginType == NOTYPE {
		if has {
			u.LoginType = PLAIN
		}
	}

	// for plain login, user must have existed.
	if u.LoginType == PLAIN {
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
			if err = x.UseBool().Find(&sources,
				&LoginSource{IsActived: true, AllowAutoRegister: true}); err != nil {
				return nil, err
			}

			for _, source := range sources {
				if source.Type == LDAP {
					u, err := LoginUserLdapSource(nil, uname, passwd,
						source.Id, source.Cfg.(*LDAPConfig), true)
					if err == nil {
						return u, nil
					}
					log.Warn("Fail to login(%s) by LDAP(%s): %v", uname, source.Name, err)
				} else if source.Type == SMTP {
					u, err := LoginUserSMTPSource(nil, uname, passwd,
						source.Id, source.Cfg.(*SMTPConfig), true)
					if err == nil {
						return u, nil
					}
					log.Warn("Fail to login(%s) by SMTP(%s): %v", uname, source.Name, err)
				}
			}

			return nil, ErrUserNotExist
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
			return LoginUserLdapSource(u, u.LoginName, passwd,
				source.Id, source.Cfg.(*LDAPConfig), false)
		case SMTP:
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
		LoginType:   LDAP,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       mail,
	}

	return CreateUser(user)
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

func SmtpAuth(host string, port int, a smtp.Auth, useTls bool) error {
	c, err := smtp.Dial(fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	defer c.Close()

	if err = c.Hello("gogs"); err != nil {
		return err
	}

	if useTls {
		if ok, _ := c.Extension("STARTTLS"); ok {
			config := &tls.Config{ServerName: host}
			if err = c.StartTLS(config); err != nil {
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
		return nil, errors.New("Unsupported SMTP auth type")
	}

	if err := SmtpAuth(cfg.Host, cfg.Port, auth, cfg.TLS); err != nil {
		if strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, ErrUserNotExist
		}
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
		LoginType:   SMTP,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       name,
	}
	return CreateUser(user)
}
