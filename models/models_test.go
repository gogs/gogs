package models

import (
	"fmt"
	"testing"

	"github.com/lunny/xorm"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	var err error
	orm, err = xorm.NewEngine("sqlite3", "./test.db")
	if err != nil {
		fmt.Println(err)
	}

	err = orm.Sync(&User{}, &Org{}, &Repo{})
	if err != nil {
		fmt.Println(err)
	}
}

func TestCreateRepository(t *testing.T) {
	user := User{Id: 1}
	err := CreateUserRepository("test", &user, "test")
	if err != nil {
		t.Error(err)
	}
}
