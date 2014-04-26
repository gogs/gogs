package models

import

// Login types.
"github.com/go-xorm/core"

/*const (
	LT_PLAIN = iota + 1
	LT_LDAP
	LT_SMTP
)*/

var _ core.Conversion = &LDAPConfig{}

type LDAPConfig struct {
}

// implement
func (cfg *LDAPConfig) FromDB(bs []byte) error {
	return nil
}

func (cfg *LDAPConfig) ToDB() ([]byte, error) {
	return nil, nil
}

type LoginSource struct {
	Id   int64
	Type int
	Name string
	Cfg  LDAPConfig
}
