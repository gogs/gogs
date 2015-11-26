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

	"github.com/Unknwon/com"
	"github.com/go-xorm/core"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/auth/pam"
	"github.com/gogits/gogs/modules/log"
)

type LoginType int

// Note: new type must be added at the end of list to maintain compatibility.
const (
	NOTYPE LoginType = iota
	PLAIN
	LDAP
	SMTP
	PAM
	DLDAP
)

var (
	ErrAuthenticationAlreadyExist = errors.New("Authentication already exist")
	ErrAuthenticationNotExist     = errors.New("Authentication does not exist")
	ErrAuthenticationUserUsed     = errors.New("Authentication has been used by some users")
)

var LoginNames = map[LoginType]string{
	LDAP:  "LDAP (via BindDN)",
	DLDAP: "LDAP (simple auth)",
	SMTP:  "SMTP",
	PAM:   "PAM",
}

// Ensure structs implemented interface.
var (
	_ core.Conversion = &LDAPConfig{}
	_ core.Conversion = &SMTPConfig{}
	_ core.Conversion = &PAMConfig{}
)

type LDAPConfig struct {
	*ldap.Source
}

func (cfg *LDAPConfig) FromDB(bs []byte) error {
	return json.Unmarshal(bs, &cfg)
}

func (cfg *LDAPConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

type SMTPConfig struct {
	Auth           string
	Host           string
	Port           int
	AllowedDomains string `xorm:"TEXT"`
	TLS            bool
	SkipVerify     bool
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
	ID        int64 `xorm:"pk autoincr"`
	Type      LoginType
	Name      string          `xorm:"UNIQUE"`
	IsActived bool            `xorm:"NOT NULL DEFAULT false"`
	Cfg       core.Conversion `xorm:"TEXT"`
	Created   time.Time       `xorm:"CREATED"`
	Updated   time.Time       `xorm:"UPDATED"`
}

func (source *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	switch colName {
	case "type":
		switch LoginType((*val).(int64)) {
		case LDAP, DLDAP:
			source.Cfg = new(LDAPConfig)
		case SMTP:
			source.Cfg = new(SMTPConfig)
		case PAM:
			source.Cfg = new(PAMConfig)
		default:
			panic("unrecognized login source type: " + com.ToStr(*val))
		}
	}
}

func (source *LoginSource) TypeName() string {
	return LoginNames[source.Type]
}

func (source *LoginSource) IsLDAP() bool {
	return source.Type == LDAP
}

func (source *LoginSource) IsDLDAP() bool {
	return source.Type == DLDAP
}

func (source *LoginSource) IsSMTP() bool {
	return source.Type == SMTP
}

func (source *LoginSource) IsPAM() bool {
	return source.Type == PAM
}

func (source *LoginSource) UseTLS() bool {
	switch source.Type {
	case LDAP, DLDAP:
		return source.LDAP().UseSSL
	case SMTP:
		return source.SMTP().TLS
	}

	return false
}

func (source *LoginSource) SkipVerify() bool {
	switch source.Type {
	case LDAP, DLDAP:
		return source.LDAP().SkipVerify
	case SMTP:
		return source.SMTP().SkipVerify
	}

	return false
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

// CountLoginSources returns number of login sources.
func CountLoginSources() int64 {
	count, _ := x.Count(new(LoginSource))
	return count
}

func CreateSource(source *LoginSource) error {
	_, err := x.Insert(source)
	return err
}

func LoginSources() ([]*LoginSource, error) {
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

func DeleteSource(source *LoginSource) error {
	count, err := x.Count(&User{LoginSource: source.ID})
	if err != nil {
		return err
	} else if count > 0 {
		return ErrAuthenticationUserUsed
	}
	_, err = x.Id(source.ID).Delete(new(LoginSource))
	return err
}

// .____     ________      _____ __________
// |    |    \______ \    /  _  \\______   \
// |    |     |    |  \  /  /_\  \|     ___/
// |    |___  |    `   \/    |    \    |
// |_______ \/_______  /\____|__  /____|
//         \/        \/         \/

// LoginUserLDAPSource queries if name/passwd can login against the LDAP directory pool,
// and create a local user if success when enabled.
// It returns the same LoginUserPlain semantic.
func LoginUserLDAPSource(u *User, name, passwd string, source *LoginSource, autoRegister bool) (*User, error) {
	cfg := source.Cfg.(*LDAPConfig)
	directBind := (source.Type == DLDAP)
	fn, sn, mail, admin, logged := cfg.SearchEntry(name, passwd, directBind)
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
		FullName:    strings.TrimSpace(fn + " " + sn),
		LoginType:   source.Type,
		LoginSource: source.ID,
		LoginName:   name,
		Email:       mail,
		IsAdmin:     admin,
		IsActive:    true,
	}
	return u, CreateUser(u)
}

//   _________   __________________________
//  /   _____/  /     \__    ___/\______   \
//  \_____  \  /  \ /  \|    |    |     ___/
//  /        \/    Y    \    |    |    |
// /_______  /\____|__  /____|    |____|
//         \/         \/

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

var SMTPAuths = []string{SMTP_PLAIN, SMTP_LOGIN}

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
	// Verify allowed domains.
	if len(cfg.AllowedDomains) > 0 {
		idx := strings.Index(name, "@")
		if idx == -1 {
			return nil, ErrUserNotExist{0, name}
		} else if !com.IsSliceContainsStr(strings.Split(cfg.AllowedDomains, ","), name[idx+1:]) {
			return nil, ErrUserNotExist{0, name}
		}
	}

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
			return nil, ErrUserNotExist{0, name}
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

// __________  _____      _____
// \______   \/  _  \    /     \
//  |     ___/  /_\  \  /  \ /  \
//  |    |  /    |    \/    Y    \
//  |____|  \____|__  /\____|__  /
//                  \/         \/

// Query if name/passwd can login against PAM
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserPAMSource(u *User, name, passwd string, sourceId int64, cfg *PAMConfig, autoRegister bool) (*User, error) {
	if err := pam.PAMAuth(cfg.ServiceName, name, passwd); err != nil {
		if strings.Contains(err.Error(), "Authentication failure") {
			return nil, ErrUserNotExist{0, name}
		}
		return nil, err
	}

	if !autoRegister {
		return u, nil
	}

	// fake a local user creation
	u = &User{
		LowerName:   strings.ToLower(name),
		Name:        name,
		LoginType:   PAM,
		LoginSource: sourceId,
		LoginName:   name,
		IsActive:    true,
		Passwd:      passwd,
		Email:       name,
	}
	return u, CreateUser(u)
}

func ExternalUserLogin(u *User, name, passwd string, source *LoginSource, autoRegister bool) (*User, error) {
	if !source.IsActived {
		return nil, ErrLoginSourceNotActived
	}

	switch source.Type {
	case LDAP, DLDAP:
		return LoginUserLDAPSource(u, name, passwd, source, autoRegister)
	case SMTP:
		return LoginUserSMTPSource(u, name, passwd, source.ID, source.Cfg.(*SMTPConfig), autoRegister)
	case PAM:
		return LoginUserPAMSource(u, name, passwd, source.ID, source.Cfg.(*PAMConfig), autoRegister)
	}

	return nil, ErrUnsupportedLoginType
}

// UserSignIn validates user name and password.
func UserSignIn(uname, passwd string) (*User, error) {
	var u *User
	if strings.Contains(uname, "@") {
		u = &User{Email: strings.ToLower(uname)}
	} else {
		u = &User{LowerName: strings.ToLower(uname)}
	}

	userExists, err := x.Get(u)
	if err != nil {
		return nil, err
	}

	if userExists {
		switch u.LoginType {
		case NOTYPE, PLAIN:
			if u.ValidatePassword(passwd) {
				return u, nil
			}

			return nil, ErrUserNotExist{u.Id, u.Name}

		default:
			var source LoginSource
			hasSource, err := x.Id(u.LoginSource).Get(&source)
			if err != nil {
				return nil, err
			} else if !hasSource {
				return nil, ErrLoginSourceNotExist
			}

			return ExternalUserLogin(u, u.LoginName, passwd, &source, false)
		}
	}

	var sources []LoginSource
	if err = x.UseBool().Find(&sources, &LoginSource{IsActived: true}); err != nil {
		return nil, err
	}

	for _, source := range sources {
		u, err := ExternalUserLogin(nil, uname, passwd, &source, true)
		if err == nil {
			return u, nil
		}

		log.Warn("Failed to login '%s' via '%s': %v", uname, source.Name, err)
	}

	return nil, ErrUserNotExist{u.Id, u.Name}
}
