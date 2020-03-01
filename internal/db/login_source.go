// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// FIXME: Put this file into its own package and separate into different files based on login sources.
package db

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-macaron/binding"
	"github.com/json-iterator/go"
	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
	"xorm.io/core"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/errors"
)

type LoginType int

// Note: new type must append to the end of list to maintain compatibility.
const (
	LOGIN_NOTYPE LoginType = iota
	LOGIN_PLAIN            // 1
	LOGIN_LDAP             // 2
	LOGIN_SMTP             // 3
	LOGIN_PAM              // 4
	LOGIN_DLDAP            // 5
	LOGIN_GITHUB           // 6
)

var LoginNames = map[LoginType]string{
	LOGIN_LDAP:   "LDAP (via BindDN)",
	LOGIN_DLDAP:  "LDAP (simple auth)", // Via direct bind
	LOGIN_SMTP:   "SMTP",
	LOGIN_PAM:    "PAM",
	LOGIN_GITHUB: "GitHub",
}

var SecurityProtocolNames = map[ldap.SecurityProtocol]string{
	ldap.SECURITY_PROTOCOL_UNENCRYPTED: "Unencrypted",
	ldap.SECURITY_PROTOCOL_LDAPS:       "LDAPS",
	ldap.SECURITY_PROTOCOL_START_TLS:   "StartTLS",
}

// Ensure structs implemented interface.
var (
	_ core.Conversion = &LDAPConfig{}
	_ core.Conversion = &SMTPConfig{}
	_ core.Conversion = &PAMConfig{}
	_ core.Conversion = &GitHubConfig{}
)

type LDAPConfig struct {
	*ldap.Source `ini:"config"`
}

func (cfg *LDAPConfig) FromDB(bs []byte) error {
	return jsoniter.Unmarshal(bs, &cfg)
}

func (cfg *LDAPConfig) ToDB() ([]byte, error) {
	return jsoniter.Marshal(cfg)
}

func (cfg *LDAPConfig) SecurityProtocolName() string {
	return SecurityProtocolNames[cfg.SecurityProtocol]
}

type SMTPConfig struct {
	Auth           string
	Host           string
	Port           int
	AllowedDomains string `xorm:"TEXT"`
	TLS            bool   `ini:"tls"`
	SkipVerify     bool
}

func (cfg *SMTPConfig) FromDB(bs []byte) error {
	return jsoniter.Unmarshal(bs, cfg)
}

func (cfg *SMTPConfig) ToDB() ([]byte, error) {
	return jsoniter.Marshal(cfg)
}

type PAMConfig struct {
	ServiceName string // PAM service (e.g. system-auth)
}

func (cfg *PAMConfig) FromDB(bs []byte) error {
	return jsoniter.Unmarshal(bs, &cfg)
}

func (cfg *PAMConfig) ToDB() ([]byte, error) {
	return jsoniter.Marshal(cfg)
}

type GitHubConfig struct {
	APIEndpoint string // GitHub service (e.g. https://api.github.com/)
}

func (cfg *GitHubConfig) FromDB(bs []byte) error {
	return jsoniter.Unmarshal(bs, &cfg)
}

func (cfg *GitHubConfig) ToDB() ([]byte, error) {
	return jsoniter.Marshal(cfg)
}

// AuthSourceFile contains information of an authentication source file.
type AuthSourceFile struct {
	abspath string
	file    *ini.File
}

// SetGeneral sets new value to the given key in the general (default) section.
func (f *AuthSourceFile) SetGeneral(name, value string) {
	f.file.Section("").Key(name).SetValue(value)
}

// SetConfig sets new values to the "config" section.
func (f *AuthSourceFile) SetConfig(cfg core.Conversion) error {
	return f.file.Section("config").ReflectFrom(cfg)
}

// Save writes updates into file system.
func (f *AuthSourceFile) Save() error {
	return f.file.SaveTo(f.abspath)
}

// LoginSource represents an external way for authorizing users.
type LoginSource struct {
	ID        int64
	Type      LoginType
	Name      string          `xorm:"UNIQUE"`
	IsActived bool            `xorm:"NOT NULL DEFAULT false"`
	IsDefault bool            `xorm:"DEFAULT false"`
	Cfg       core.Conversion `xorm:"TEXT"`

	Created     time.Time `xorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" json:"-"`
	UpdatedUnix int64

	LocalFile *AuthSourceFile `xorm:"-" json:"-"`
}

func (s *LoginSource) BeforeInsert() {
	s.CreatedUnix = time.Now().Unix()
	s.UpdatedUnix = s.CreatedUnix
}

func (s *LoginSource) BeforeUpdate() {
	s.UpdatedUnix = time.Now().Unix()
}

// Cell2Int64 converts a xorm.Cell type to int64,
// and handles possible irregular cases.
func Cell2Int64(val xorm.Cell) int64 {
	switch (*val).(type) {
	case []uint8:
		log.Trace("Cell2Int64 ([]uint8): %v", *val)
		return com.StrTo(string((*val).([]uint8))).MustInt64()
	}
	return (*val).(int64)
}

func (s *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	switch colName {
	case "type":
		switch LoginType(Cell2Int64(val)) {
		case LOGIN_LDAP, LOGIN_DLDAP:
			s.Cfg = new(LDAPConfig)
		case LOGIN_SMTP:
			s.Cfg = new(SMTPConfig)
		case LOGIN_PAM:
			s.Cfg = new(PAMConfig)
		case LOGIN_GITHUB:
			s.Cfg = new(GitHubConfig)
		default:
			panic("unrecognized login source type: " + com.ToStr(*val))
		}
	}
}

func (s *LoginSource) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		s.Created = time.Unix(s.CreatedUnix, 0).Local()
	case "updated_unix":
		s.Updated = time.Unix(s.UpdatedUnix, 0).Local()
	}
}

func (s *LoginSource) TypeName() string {
	return LoginNames[s.Type]
}

func (s *LoginSource) IsLDAP() bool {
	return s.Type == LOGIN_LDAP
}

func (s *LoginSource) IsDLDAP() bool {
	return s.Type == LOGIN_DLDAP
}

func (s *LoginSource) IsSMTP() bool {
	return s.Type == LOGIN_SMTP
}

func (s *LoginSource) IsPAM() bool {
	return s.Type == LOGIN_PAM
}

func (s *LoginSource) IsGitHub() bool {
	return s.Type == LOGIN_GITHUB
}

func (s *LoginSource) HasTLS() bool {
	return ((s.IsLDAP() || s.IsDLDAP()) &&
		s.LDAP().SecurityProtocol > ldap.SECURITY_PROTOCOL_UNENCRYPTED) ||
		s.IsSMTP()
}

func (s *LoginSource) UseTLS() bool {
	switch s.Type {
	case LOGIN_LDAP, LOGIN_DLDAP:
		return s.LDAP().SecurityProtocol != ldap.SECURITY_PROTOCOL_UNENCRYPTED
	case LOGIN_SMTP:
		return s.SMTP().TLS
	}

	return false
}

func (s *LoginSource) SkipVerify() bool {
	switch s.Type {
	case LOGIN_LDAP, LOGIN_DLDAP:
		return s.LDAP().SkipVerify
	case LOGIN_SMTP:
		return s.SMTP().SkipVerify
	}

	return false
}

func (s *LoginSource) LDAP() *LDAPConfig {
	return s.Cfg.(*LDAPConfig)
}

func (s *LoginSource) SMTP() *SMTPConfig {
	return s.Cfg.(*SMTPConfig)
}

func (s *LoginSource) PAM() *PAMConfig {
	return s.Cfg.(*PAMConfig)
}

func (s *LoginSource) GitHub() *GitHubConfig {
	return s.Cfg.(*GitHubConfig)
}

func CreateLoginSource(source *LoginSource) error {
	has, err := x.Get(&LoginSource{Name: source.Name})
	if err != nil {
		return err
	} else if has {
		return ErrLoginSourceAlreadyExist{source.Name}
	}

	_, err = x.Insert(source)
	if err != nil {
		return err
	} else if source.IsDefault {
		return ResetNonDefaultLoginSources(source)
	}
	return nil
}

// LoginSources returns all login sources defined.
func LoginSources() ([]*LoginSource, error) {
	sources := make([]*LoginSource, 0, 2)
	if err := x.Find(&sources); err != nil {
		return nil, err
	}

	return append(sources, localLoginSources.List()...), nil
}

// ActivatedLoginSources returns login sources that are currently activated.
func ActivatedLoginSources() ([]*LoginSource, error) {
	sources := make([]*LoginSource, 0, 2)
	if err := x.Where("is_actived = ?", true).Find(&sources); err != nil {
		return nil, fmt.Errorf("find activated login sources: %v", err)
	}
	return append(sources, localLoginSources.ActivatedList()...), nil
}

// GetLoginSourceByID returns login source by given ID.
func GetLoginSourceByID(id int64) (*LoginSource, error) {
	source := new(LoginSource)
	has, err := x.Id(id).Get(source)
	if err != nil {
		return nil, err
	} else if !has {
		return localLoginSources.GetLoginSourceByID(id)
	}
	return source, nil
}

// ResetNonDefaultLoginSources clean other default source flag
func ResetNonDefaultLoginSources(source *LoginSource) error {
	// update changes to DB
	if _, err := x.NotIn("id", []int64{source.ID}).Cols("is_default").Update(&LoginSource{IsDefault: false}); err != nil {
		return err
	}
	// write changes to local authentications
	for i := range localLoginSources.sources {
		if localLoginSources.sources[i].LocalFile != nil && localLoginSources.sources[i].ID != source.ID {
			localLoginSources.sources[i].LocalFile.SetGeneral("is_default", "false")
			if err := localLoginSources.sources[i].LocalFile.SetConfig(source.Cfg); err != nil {
				return fmt.Errorf("LocalFile.SetConfig: %v", err)
			} else if err = localLoginSources.sources[i].LocalFile.Save(); err != nil {
				return fmt.Errorf("LocalFile.Save: %v", err)
			}
		}
	}
	// flush memory so that web page can show the same behaviors
	localLoginSources.UpdateLoginSource(source)
	return nil
}

// UpdateLoginSource updates information of login source to database or local file.
func UpdateLoginSource(source *LoginSource) error {
	if source.LocalFile == nil {
		if _, err := x.Id(source.ID).AllCols().Update(source); err != nil {
			return err
		} else {
			return ResetNonDefaultLoginSources(source)
		}

	}

	source.LocalFile.SetGeneral("name", source.Name)
	source.LocalFile.SetGeneral("is_activated", com.ToStr(source.IsActived))
	source.LocalFile.SetGeneral("is_default", com.ToStr(source.IsDefault))
	if err := source.LocalFile.SetConfig(source.Cfg); err != nil {
		return fmt.Errorf("LocalFile.SetConfig: %v", err)
	} else if err = source.LocalFile.Save(); err != nil {
		return fmt.Errorf("LocalFile.Save: %v", err)
	}
	return ResetNonDefaultLoginSources(source)
}

func DeleteSource(source *LoginSource) error {
	count, err := x.Count(&User{LoginSource: source.ID})
	if err != nil {
		return err
	} else if count > 0 {
		return ErrLoginSourceInUse{source.ID}
	}
	_, err = x.Id(source.ID).Delete(new(LoginSource))
	return err
}

// CountLoginSources returns total number of login sources.
func CountLoginSources() int64 {
	count, _ := x.Count(new(LoginSource))
	return count + int64(localLoginSources.Len())
}

// LocalLoginSources contains authentication sources configured and loaded from local files.
// Calling its methods is thread-safe; otherwise, please maintain the mutex accordingly.
type LocalLoginSources struct {
	sync.RWMutex
	sources []*LoginSource
}

func (s *LocalLoginSources) Len() int {
	return len(s.sources)
}

// List returns full clone of login sources.
func (s *LocalLoginSources) List() []*LoginSource {
	s.RLock()
	defer s.RUnlock()

	list := make([]*LoginSource, s.Len())
	for i := range s.sources {
		list[i] = &LoginSource{}
		*list[i] = *s.sources[i]
	}
	return list
}

// ActivatedList returns clone of activated login sources.
func (s *LocalLoginSources) ActivatedList() []*LoginSource {
	s.RLock()
	defer s.RUnlock()

	list := make([]*LoginSource, 0, 2)
	for i := range s.sources {
		if !s.sources[i].IsActived {
			continue
		}
		source := &LoginSource{}
		*source = *s.sources[i]
		list = append(list, source)
	}
	return list
}

// GetLoginSourceByID returns a clone of login source by given ID.
func (s *LocalLoginSources) GetLoginSourceByID(id int64) (*LoginSource, error) {
	s.RLock()
	defer s.RUnlock()

	for i := range s.sources {
		if s.sources[i].ID == id {
			source := &LoginSource{}
			*source = *s.sources[i]
			return source, nil
		}
	}

	return nil, errors.LoginSourceNotExist{ID: id}
}

// UpdateLoginSource updates in-memory copy of the authentication source.
func (s *LocalLoginSources) UpdateLoginSource(source *LoginSource) {
	s.Lock()
	defer s.Unlock()

	source.Updated = time.Now()
	for i := range s.sources {
		if s.sources[i].ID == source.ID {
			*s.sources[i] = *source
		} else if source.IsDefault {
			s.sources[i].IsDefault = false
		}
	}
}

var localLoginSources = &LocalLoginSources{}

// LoadAuthSources loads authentication sources from local files
// and converts them into login sources.
func LoadAuthSources() {
	authdPath := filepath.Join(conf.CustomDir(), "conf", "auth.d")
	if !com.IsDir(authdPath) {
		return
	}

	paths, err := com.GetFileListBySuffix(authdPath, ".conf")
	if err != nil {
		log.Fatal("Failed to list authentication sources: %v", err)
	}

	localLoginSources.sources = make([]*LoginSource, 0, len(paths))

	for _, fpath := range paths {
		authSource, err := ini.Load(fpath)
		if err != nil {
			log.Fatal("Failed to load authentication source: %v", err)
		}
		authSource.NameMapper = ini.TitleUnderscore

		// Set general attributes
		s := authSource.Section("")
		loginSource := &LoginSource{
			ID:        s.Key("id").MustInt64(),
			Name:      s.Key("name").String(),
			IsActived: s.Key("is_activated").MustBool(),
			IsDefault: s.Key("is_default").MustBool(),
			LocalFile: &AuthSourceFile{
				abspath: fpath,
				file:    authSource,
			},
		}

		fi, err := os.Stat(fpath)
		if err != nil {
			log.Fatal("Failed to load authentication source: %v", err)
		}
		loginSource.Updated = fi.ModTime()

		// Parse authentication source file
		authType := s.Key("type").String()
		switch authType {
		case "ldap_bind_dn":
			loginSource.Type = LOGIN_LDAP
			loginSource.Cfg = &LDAPConfig{}
		case "ldap_simple_auth":
			loginSource.Type = LOGIN_DLDAP
			loginSource.Cfg = &LDAPConfig{}
		case "smtp":
			loginSource.Type = LOGIN_SMTP
			loginSource.Cfg = &SMTPConfig{}
		case "pam":
			loginSource.Type = LOGIN_PAM
			loginSource.Cfg = &PAMConfig{}
		case "github":
			loginSource.Type = LOGIN_GITHUB
			loginSource.Cfg = &GitHubConfig{}
		default:
			log.Fatal("Failed to load authentication source: unknown type '%s'", authType)
		}

		if err = authSource.Section("config").MapTo(loginSource.Cfg); err != nil {
			log.Fatal("Failed to parse authentication source 'config': %v", err)
		}

		localLoginSources.sources = append(localLoginSources.sources, loginSource)
	}
}

// .____     ________      _____ __________
// |    |    \______ \    /  _  \\______   \
// |    |     |    |  \  /  /_\  \|     ___/
// |    |___  |    `   \/    |    \    |
// |_______ \/_______  /\____|__  /____|
//         \/        \/         \/

func composeFullName(firstname, surname, username string) string {
	switch {
	case len(firstname) == 0 && len(surname) == 0:
		return username
	case len(firstname) == 0:
		return surname
	case len(surname) == 0:
		return firstname
	default:
		return firstname + " " + surname
	}
}

// LoginViaLDAP queries if login/password is valid against the LDAP directory pool,
// and create a local user if success when enabled.
func LoginViaLDAP(user *User, login, password string, source *LoginSource, autoRegister bool) (*User, error) {
	username, fn, sn, mail, isAdmin, succeed := source.Cfg.(*LDAPConfig).SearchEntry(login, password, source.Type == LOGIN_DLDAP)
	if !succeed {
		// User not in LDAP, do nothing
		return nil, errors.UserNotExist{Name: login}
	}

	if !autoRegister {
		return user, nil
	}

	// Fallback.
	if len(username) == 0 {
		username = login
	}
	// Validate username make sure it satisfies requirement.
	if binding.AlphaDashDotPattern.MatchString(username) {
		return nil, fmt.Errorf("Invalid pattern for attribute 'username' [%s]: must be valid alpha or numeric or dash(-_) or dot characters", username)
	}

	if len(mail) == 0 {
		mail = fmt.Sprintf("%s@localhost", username)
	}

	user = &User{
		LowerName:   strings.ToLower(username),
		Name:        username,
		FullName:    composeFullName(fn, sn, username),
		Email:       mail,
		LoginType:   source.Type,
		LoginSource: source.ID,
		LoginName:   login,
		IsActive:    true,
		IsAdmin:     isAdmin,
	}

	ok, err := IsUserExist(0, user.Name)
	if err != nil {
		return user, err
	}

	if ok {
		return user, UpdateUser(user)
	}

	return user, CreateUser(user)
}

//   _________   __________________________
//  /   _____/  /     \__    ___/\______   \
//  \_____  \  /  \ /  \|    |    |     ___/
//  /        \/    Y    \    |    |    |
// /_______  /\____|__  /____|    |____|
//         \/         \/

type smtpLoginAuth struct {
	username, password string
}

func (auth *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(auth.username), nil
}

func (auth *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(auth.username), nil
		case "Password:":
			return []byte(auth.password), nil
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
	return errors.New("Unsupported SMTP authentication method")
}

// LoginViaSMTP queries if login/password is valid against the SMTP,
// and create a local user if success when enabled.
func LoginViaSMTP(user *User, login, password string, sourceID int64, cfg *SMTPConfig, autoRegister bool) (*User, error) {
	// Verify allowed domains.
	if len(cfg.AllowedDomains) > 0 {
		idx := strings.Index(login, "@")
		if idx == -1 {
			return nil, errors.UserNotExist{Name: login}
		} else if !com.IsSliceContainsStr(strings.Split(cfg.AllowedDomains, ","), login[idx+1:]) {
			return nil, errors.UserNotExist{Name: login}
		}
	}

	var auth smtp.Auth
	if cfg.Auth == SMTP_PLAIN {
		auth = smtp.PlainAuth("", login, password, cfg.Host)
	} else if cfg.Auth == SMTP_LOGIN {
		auth = &smtpLoginAuth{login, password}
	} else {
		return nil, errors.New("Unsupported SMTP authentication type")
	}

	if err := SMTPAuth(auth, cfg); err != nil {
		// Check standard error format first,
		// then fallback to worse case.
		tperr, ok := err.(*textproto.Error)
		if (ok && tperr.Code == 535) ||
			strings.Contains(err.Error(), "Username and Password not accepted") {
			return nil, errors.UserNotExist{0, login}
		}
		return nil, err
	}

	if !autoRegister {
		return user, nil
	}

	username := login
	idx := strings.Index(login, "@")
	if idx > -1 {
		username = login[:idx]
	}

	user = &User{
		LowerName:   strings.ToLower(username),
		Name:        strings.ToLower(username),
		Email:       login,
		Passwd:      password,
		LoginType:   LOGIN_SMTP,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
	}
	return user, CreateUser(user)
}

// __________  _____      _____
// \______   \/  _  \    /     \
//  |     ___/  /_\  \  /  \ /  \
//  |    |  /    |    \/    Y    \
//  |____|  \____|__  /\____|__  /
//                  \/         \/

// LoginViaPAM queries if login/password is valid against the PAM,
// and create a local user if success when enabled.
func LoginViaPAM(user *User, login, password string, sourceID int64, cfg *PAMConfig, autoRegister bool) (*User, error) {
	if err := pam.PAMAuth(cfg.ServiceName, login, password); err != nil {
		if strings.Contains(err.Error(), "Authentication failure") {
			return nil, errors.UserNotExist{0, login}
		}
		return nil, err
	}

	if !autoRegister {
		return user, nil
	}

	user = &User{
		LowerName:   strings.ToLower(login),
		Name:        login,
		Email:       login,
		Passwd:      password,
		LoginType:   LOGIN_PAM,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
	}
	return user, CreateUser(user)
}

//________.__  __     ___ ___      ___.
///  _____/|__|/  |_  /   |   \ __ _\_ |__
///   \  ___|  \   __\/    ~    \  |  \ __ \
//\    \_\  \  ||  |  \    Y    /  |  / \_\ \
//\______  /__||__|   \___|_  /|____/|___  /
//\/                 \/           \/

func LoginViaGitHub(user *User, login, password string, sourceID int64, cfg *GitHubConfig, autoRegister bool) (*User, error) {
	fullname, email, url, location, err := github.Authenticate(cfg.APIEndpoint, login, password)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			return nil, errors.UserNotExist{0, login}
		}
		return nil, err
	}

	if !autoRegister {
		return user, nil
	}
	user = &User{
		LowerName:   strings.ToLower(login),
		Name:        login,
		FullName:    fullname,
		Email:       email,
		Website:     url,
		Passwd:      password,
		LoginType:   LOGIN_GITHUB,
		LoginSource: sourceID,
		LoginName:   login,
		IsActive:    true,
		Location:    location,
	}
	return user, CreateUser(user)
}

func remoteUserLogin(user *User, login, password string, source *LoginSource, autoRegister bool) (*User, error) {
	if !source.IsActived {
		return nil, errors.LoginSourceNotActivated{SourceID: source.ID}
	}

	switch source.Type {
	case LOGIN_LDAP, LOGIN_DLDAP:
		return LoginViaLDAP(user, login, password, source, autoRegister)
	case LOGIN_SMTP:
		return LoginViaSMTP(user, login, password, source.ID, source.Cfg.(*SMTPConfig), autoRegister)
	case LOGIN_PAM:
		return LoginViaPAM(user, login, password, source.ID, source.Cfg.(*PAMConfig), autoRegister)
	case LOGIN_GITHUB:
		return LoginViaGitHub(user, login, password, source.ID, source.Cfg.(*GitHubConfig), autoRegister)
	}

	return nil, errors.InvalidLoginSourceType{Type: source.Type}
}

// UserLogin validates user name and password via given login source ID.
// If the loginSourceID is negative, it will abort login process if user is not found.
func UserLogin(username, password string, loginSourceID int64) (*User, error) {
	var user *User
	if strings.Contains(username, "@") {
		user = &User{Email: strings.ToLower(username)}
	} else {
		user = &User{LowerName: strings.ToLower(username)}
	}

	hasUser, err := x.Get(user)
	if err != nil {
		return nil, fmt.Errorf("get user record: %v", err)
	}

	if hasUser {
		// Note: This check is unnecessary but to reduce user confusion at login page
		// and make it more consistent at user's perspective.
		if loginSourceID >= 0 && user.LoginSource != loginSourceID {
			return nil, errors.LoginSourceMismatch{Expect: loginSourceID, Actual: user.LoginSource}
		}

		// Validate password hash fetched from database for local accounts
		if user.LoginType == LOGIN_NOTYPE ||
			user.LoginType == LOGIN_PLAIN {
			if user.ValidatePassword(password) {
				return user, nil
			}

			return nil, errors.UserNotExist{user.ID, user.Name}
		}

		// Remote login to the login source the user is associated with
		source, err := GetLoginSourceByID(user.LoginSource)
		if err != nil {
			return nil, err
		}

		return remoteUserLogin(user, user.LoginName, password, source, false)
	}

	// Non-local login source is always greater than 0
	if loginSourceID <= 0 {
		return nil, errors.UserNotExist{-1, username}
	}

	source, err := GetLoginSourceByID(loginSourceID)
	if err != nil {
		return nil, err
	}

	return remoteUserLogin(nil, username, password, source, true)
}
