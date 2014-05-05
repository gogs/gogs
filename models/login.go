// Copyright github.com/juju2013. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/go-xorm/core"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/auth/ldap"
)

// Login types.
const (
	LT_PLAIN = iota + 1
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

type LoginSource struct {
	Id        int64
	Type      int
	Name      string          `xorm:"unique"`
	IsActived bool            `xorm:"not null default false"`
	Cfg       core.Conversion `xorm:"TEXT"`
	Created   time.Time       `xorm:"created"`
	Updated   time.Time       `xorm:"updated"`
}

func (source *LoginSource) TypeString() string {
	return LoginTypes[source.Type]
}

func (source *LoginSource) LDAP() *LDAPConfig {
	return source.Cfg.(*LDAPConfig)
}

// for xorm callback
func (source *LoginSource) BeforeSet(colName string, val xorm.Cell) {
	if colName == "type" {
		ty := (*val).(int64)
		switch ty {
		case LT_LDAP:
			source.Cfg = new(LDAPConfig)
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

func AddLDAPSource(name string, cfg *LDAPConfig) error {
	_, err := orm.Insert(&LoginSource{Type: LT_LDAP,
		Name:      name,
		IsActived: true,
		Cfg:       cfg,
	})
	return err
}

func UpdateLDAPSource(source *LoginSource) error {
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
