package models

import (
	"encoding/json"
	"time"

	"github.com/go-xorm/core"
	"github.com/gogits/gogs/modules/auth/ldap"
)

/*const (
	LT_PLAIN = iota + 1
	LT_LDAP
	LT_SMTP
)*/

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
	Name      string
	IsActived bool
	Cfg       core.Conversion `xorm:"TEXT"`
	Created   time.Time       `xorm:"created"`
	Updated   time.Time       `xorm:"updated"`
}

func GetAuths() ([]*LoginSource, error) {
	var auths = make([]*LoginSource, 0)
	err := orm.Find(&auths)
	return auths, err
}

func AddLDAPSource(name string, cfg *LDAPConfig) error {
	_, err := orm.Insert(&LoginSource{Type: LT_LDAP,
		Name:      name,
		IsActived: true,
		Cfg:       cfg,
	})
	return err
}

func UpdateLDAPSource(id int64, name string, cfg *LDAPConfig) error {
	_, err := orm.AllCols().Id(id).Update(&LoginSource{
		Id:   id,
		Type: LT_LDAP,
		Name: name,
		Cfg:  cfg,
	})
	return err
}

func DelLoginSource(id int64) error {
	_, err := orm.Id(id).Delete(&LoginSource{})
	return err
}
