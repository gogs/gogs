// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
)

func Test_repos(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(Repository)}
	db := &repos{
		DB: initTestDB(t, "repos", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *repos)
	}{
		{"create", test_repos_create},
		{"GetByName", test_repos_GetByName},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, db)
		})
	}
}

func test_repos_create(t *testing.T, db *repos) {
	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.create(1, createRepoOpts{
			Name: "my.git",
		})
		expErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "pattern": "*.git"}}
		assert.Equal(t, expErr, err)
	})

	t.Run("already exists", func(t *testing.T) {
		_, err := db.create(1, createRepoOpts{
			Name: "repo1",
		})
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.create(1, createRepoOpts{
			Name: "repo1",
		})
		expErr := ErrRepoAlreadyExist{args: errutil.Args{"ownerID": int64(1), "name": "repo1"}}
		assert.Equal(t, expErr, err)
	})

	repo, err := db.create(1, createRepoOpts{
		Name: "repo2",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err = db.GetByName(repo.OwnerID, repo.Name)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, gorm.NowFunc().Format(time.RFC3339), repo.Created.Format(time.RFC3339))
}

func test_repos_GetByName(t *testing.T, db *repos) {
	repo, err := db.create(1, createRepoOpts{
		Name: "repo1",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.GetByName(repo.OwnerID, repo.Name)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.GetByName(1, "bad_name")
	expErr := ErrRepoNotExist{args: errutil.Args{"ownerID": int64(1), "name": "bad_name"}}
	assert.Equal(t, expErr, err)
}
