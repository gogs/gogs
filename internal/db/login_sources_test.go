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

	db := &loginSources{
		DB: initTestDB(t, "loginSources", new(LoginSource)),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *loginSources)
	}{
		{"Create", test_loginSources_Create},
		// {"Count", test_loginSources_Count},
		// {"GetByID", test_loginSources_GetByID},
		// {"List", test_loginSources_List},
		// {"ResetNonDefault", test_loginSources_ResetNonDefault},
		// {"Save", test_loginSources_Save},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := deleteTables(db.DB, new(LoginSource))
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
