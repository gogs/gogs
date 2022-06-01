// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/errutil"
)

func TestLoginSource_BeforeSave(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("Config has not been set", func(t *testing.T) {
		s := &LoginSource{}
		err := s.BeforeSave(db)
		if err != nil {
			t.Fatal(err)
		}
		assert.Empty(t, s.Config)
	})

	t.Run("Config has been set", func(t *testing.T) {
		s := &LoginSource{
			Provider: pam.NewProvider(&pam.Config{
				ServiceName: "pam_service",
			}),
		}
		err := s.BeforeSave(db)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, `{"ServiceName":"pam_service"}`, s.Config)
	})
}

func TestLoginSource_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		s := &LoginSource{CreatedUnix: 1}
		_ = s.BeforeCreate(db)
		assert.Equal(t, int64(1), s.CreatedUnix)
		assert.Equal(t, int64(0), s.UpdatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		s := &LoginSource{}
		_ = s.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), s.CreatedUnix)
		assert.Equal(t, db.NowFunc().Unix(), s.UpdatedUnix)
	})
}

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
		{"ResetNonDefault", test_loginSources_ResetNonDefault},
		{"Save", test_loginSources_Save},
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
		if t.Failed() {
			break
		}
	}
}

func test_loginSources_Create(t *testing.T, db *loginSources) {
	// Create first login source with name "GitHub"
	source, err := db.Create(CreateLoginSourceOpts{
		Type:      auth.GitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config: &github.Config{
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
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), source.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), source.Updated.UTC().Format(time.RFC3339))

	// Try create second login source with same name should fail
	_, err = db.Create(CreateLoginSourceOpts{Name: source.Name})
	expErr := ErrLoginSourceAlreadyExist{args: errutil.Args{"name": source.Name}}
	assert.Equal(t, expErr, err)
}

func test_loginSources_Count(t *testing.T, db *loginSources) {
	// Create two login sources, one in database and one as source file.
	_, err := db.Create(CreateLoginSourceOpts{
		Type:      auth.GitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config: &github.Config{
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
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Create a user that uses this login source
		_, err = (&users{DB: db.DB}).Create("alice", "", CreateUserOpts{
			LoginSource: source.ID,
		})
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
		Type:      auth.GitHub,
		Name:      "GitHub2",
		Activated: true,
		Default:   false,
		Config: &github.Config{
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
	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockGetByID: func(id int64) (*LoginSource, error) {
			if id != 101 {
				return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
			}
			return &LoginSource{ID: id}, nil
		},
	})

	expConfig := &github.Config{
		APIEndpoint: "https://api.github.com",
	}

	// Create a login source with name "GitHub"
	source, err := db.Create(CreateLoginSourceOpts{
		Type:      auth.GitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   false,
		Config:    expConfig,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get the one in the database and test the read/write hooks
	source, err = db.GetByID(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, expConfig, source.Provider.Config())

	// Get the one in source file store
	_, err = db.GetByID(101)
	if err != nil {
		t.Fatal(err)
	}
}

func test_loginSources_List(t *testing.T, db *loginSources) {
	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockList: func(opts ListLoginSourceOpts) []*LoginSource {
			if opts.OnlyActivated {
				return []*LoginSource{
					{ID: 1},
				}
			}
			return []*LoginSource{
				{ID: 1},
				{ID: 2},
			}
		},
	})

	// Create two login sources in database, one activated and the other one not
	_, err := db.Create(CreateLoginSourceOpts{
		Type: auth.PAM,
		Name: "PAM",
		Config: &pam.Config{
			ServiceName: "PAM",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Create(CreateLoginSourceOpts{
		Type:      auth.GitHub,
		Name:      "GitHub",
		Activated: true,
		Config: &github.Config{
			APIEndpoint: "https://api.github.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// List all login sources
	sources, err := db.List(ListLoginSourceOpts{})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 4, len(sources), "number of sources")

	// Only list activated login sources
	sources, err = db.List(ListLoginSourceOpts{OnlyActivated: true})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(sources), "number of sources")
}

func test_loginSources_ResetNonDefault(t *testing.T, db *loginSources) {
	setMockLoginSourceFilesStore(t, db, &mockLoginSourceFilesStore{
		MockList: func(opts ListLoginSourceOpts) []*LoginSource {
			return []*LoginSource{
				{
					File: &mockLoginSourceFileStore{
						MockSetGeneral: func(name, value string) {
							assert.Equal(t, "is_default", name)
							assert.Equal(t, "false", value)
						},
						MockSave: func() error {
							return nil
						},
					},
				},
			}
		},
		MockUpdate: func(source *LoginSource) {},
	})

	// Create two login sources both have default on
	source1, err := db.Create(CreateLoginSourceOpts{
		Type:    auth.PAM,
		Name:    "PAM",
		Default: true,
		Config: &pam.Config{
			ServiceName: "PAM",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	source2, err := db.Create(CreateLoginSourceOpts{
		Type:      auth.GitHub,
		Name:      "GitHub",
		Activated: true,
		Default:   true,
		Config: &github.Config{
			APIEndpoint: "https://api.github.com",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Set source 1 as default
	err = db.ResetNonDefault(source1)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the default state
	source1, err = db.GetByID(source1.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, source1.IsDefault)

	source2, err = db.GetByID(source2.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.False(t, source2.IsDefault)
}

func test_loginSources_Save(t *testing.T, db *loginSources) {
	t.Run("save to database", func(t *testing.T) {
		// Create a login source with name "GitHub"
		source, err := db.Create(CreateLoginSourceOpts{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		source.IsActived = false
		source.Provider = github.NewProvider(&github.Config{
			APIEndpoint: "https://api2.github.com",
		})
		err = db.Save(source)
		if err != nil {
			t.Fatal(err)
		}

		source, err = db.GetByID(source.ID)
		if err != nil {
			t.Fatal(err)
		}
		assert.False(t, source.IsActived)
		assert.Equal(t, "https://api2.github.com", source.GitHub().APIEndpoint)
	})

	t.Run("save to file", func(t *testing.T) {
		calledSave := false
		source := &LoginSource{
			Provider: github.NewProvider(&github.Config{
				APIEndpoint: "https://api.github.com",
			}),
			File: &mockLoginSourceFileStore{
				MockSetGeneral: func(name, value string) {},
				MockSetConfig:  func(cfg interface{}) error { return nil },
				MockSave: func() error {
					calledSave = true
					return nil
				},
			},
		}
		err := db.Save(source)
		if err != nil {
			t.Fatal(err)
		}
		assert.True(t, calledSave)
	})
}
