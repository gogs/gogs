package models

import (
	"strings"
	"time"
)

const (
	Readable = iota + 1
	Writable
)

type Access struct {
	Id       int64
	UserName string    `xorm:"unique(s)"`
	RepoName string    `xorm:"unique(s)"`
	Mode     int       `xorm:"unique(s)"`
	Created  time.Time `xorm:"created"`
}

func AddAccess(access *Access) error {
	_, err := orm.Insert(access)
	return err
}

// if one user can read or write one repository
func HasAccess(userName, repoName, mode string) (bool, error) {
	return orm.Get(&Access{0, strings.ToLower(userName), strings.ToLower(repoName), mode})
}
