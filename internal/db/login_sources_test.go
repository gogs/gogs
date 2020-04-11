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

func Test_loginSources(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(LoginSource), new(User)}
	db := &loginSources{
		DB: initTestDB(t, "loginSources", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *loginSources)
	}{
		{"Create", test_loginSources_Create},
		{"Count", test_loginSources_Count},
		{"DeleteByID", test_loginSources_DeleteByID},
		{"GetByID", test_loginSources_GetByID},
		{"List", test_loginSources_List},
		// {"ResetNonDefault", test_loginSources_ResetNonDefault},
		// {"Save", test_loginSources_Save},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(db.DB, tables...)
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, db)
		})
	}
}

func test_loginSources_Create(t *testing.T, db *loginSources) {
	// Create first login source with name "GitHub"
	source, err := db.Create(CreateLoginSourceOpts{
		Type:      LoginGitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config: &GitHubConfig{
			APIEndpoint: "https://api.github.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get it back and check the Created field
	source, err = db.GetByID(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, gorm.NowFunc().Format(time.RFC3339), source.Created.Format(time.RFC3339))

	// Try create second login source with same name should fail
	_, err = db.Create(CreateLoginSourceOpts{Name: source.Name})
	expErr := ErrLoginSourceAlreadyExist{args: errutil.Args{"name": source.Name}}
	assert.Equal(t, expErr, err)
}

func test_loginSources_Count(t *testing.T, db *loginSources) {
	// Create two login sources, one in database and one as source file.
	_, err := db.Create(CreateLoginSourceOpts{
		Type:      LoginGitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config: &GitHubConfig{
			APIEndpoint: "https://api.github.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockLen: func() int {
			return 2
		},
	})

	assert.Equal(t, int64(3), db.Count())
}

func test_loginSources_DeleteByID(t *testing.T, db *loginSources) {
	t.Run("delete but in used", func(t *testing.T) {
		source, err := db.Create(CreateLoginSourceOpts{
			Type:      LoginGitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config: &GitHubConfig{
				APIEndpoint: "https://api.github.com",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Create a user that uses this login source
		user := &User{
			LoginSource: source.ID,
		}
		err = db.DB.Create(user).Error
		if err != nil {
			t.Fatal(err)
		}

		// Delete the login source will result in error
		err = db.DeleteByID(source.ID)
		expErr := ErrLoginSourceInUse{args: errutil.Args{"id": source.ID}}
		assert.Equal(t, expErr, err)
	})

	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockGetByID: func(id int64) (*LoginSource, error) {
			return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
		},
	})

	// Create a login source with name "GitHub2"
	source, err := db.Create(CreateLoginSourceOpts{
		Type:      LoginGitHub,
		Name:      "GitHub2",
		Activated: true,
		Default:   false,
		Config: &GitHubConfig{
			APIEndpoint: "https://api.github.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Delete a non-existent ID is noop
	err = db.DeleteByID(9999)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetByID(source.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Now delete this login source with ID
	err = db.DeleteByID(source.ID)
	if err != nil {
		t.Fatal(err)
	}

	// We should get token not found error
	_, err = db.GetByID(source.ID)
	expErr := ErrLoginSourceNotExist{args: errutil.Args{"id": source.ID}}
	assert.Equal(t, expErr, err)
}

func test_loginSources_GetByID(t *testing.T, db *loginSources) {
	expConfig := &GitHubConfig{
		APIEndpoint: "https://api.github.com",
	}

	// Create a login source with name "GitHub"
	source, err := db.Create(CreateLoginSourceOpts{
		Type:      LoginGitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config:    expConfig,
	})
	if err != nil {
		t.Fatal(err)
	}

	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockGetByID: func(id int64) (*LoginSource, error) {
			if id != 101 {
				return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
			}
			return &LoginSource{ID: id}, nil
		},
	})

	// Get the one in the database and test the read/write hooks
	source, err = db.GetByID(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expConfig, source.Config)

	// Get the one in source file store
	_, err = db.GetByID(101)
	if err != nil {
		t.Fatal(err)
	}
}

func test_loginSources_List(t *testing.T, db *loginSources) {

}
