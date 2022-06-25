// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	mockrequire "github.com/derision-test/go-mockgen/testutil/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/dbtest"
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
		require.NoError(t, err)
		assert.Empty(t, s.Config)
	})

	t.Run("Config has been set", func(t *testing.T) {
		s := &LoginSource{
			Provider: pam.NewProvider(&pam.Config{
				ServiceName: "pam_service",
			}),
		}
		err := s.BeforeSave(db)
		require.NoError(t, err)
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
		DB: dbtest.NewDB(t, "loginSources", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *loginSources)
	}{
		{"Create", loginSourcesCreate},
		{"Count", loginSourcesCount},
		{"DeleteByID", loginSourcesDeleteByID},
		{"GetByID", loginSourcesGetByID},
		{"List", loginSourcesList},
		{"ResetNonDefault", loginSourcesResetNonDefault},
		{"Save", loginSourcesSave},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				require.NoError(t, err)
			})
			tc.test(t, db)
		})
		if t.Failed() {
			break
		}
	}
}

func loginSourcesCreate(t *testing.T, db *loginSources) {
	ctx := context.Background()

	// Create first login source with name "GitHub"
	source, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		},
	)
	require.NoError(t, err)

	// Get it back and check the Created field
	source, err = db.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), source.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), source.Updated.UTC().Format(time.RFC3339))

	// Try create second login source with same name should fail
	_, err = db.Create(ctx, CreateLoginSourceOptions{Name: source.Name})
	wantErr := ErrLoginSourceAlreadyExist{args: errutil.Args{"name": source.Name}}
	assert.Equal(t, wantErr, err)
}

func loginSourcesCount(t *testing.T, db *loginSources) {
	ctx := context.Background()

	// Create two login sources, one in database and one as source file.
	_, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		},
	)
	require.NoError(t, err)

	mock := NewMockLoginSourceFilesStore()
	mock.LenFunc.SetDefaultReturn(2)
	setMockLoginSourceFilesStore(t, db, mock)

	assert.Equal(t, int64(3), db.Count(ctx))
}

func loginSourcesDeleteByID(t *testing.T, db *loginSources) {
	ctx := context.Background()

	t.Run("delete but in used", func(t *testing.T) {
		source, err := db.Create(ctx,
			CreateLoginSourceOptions{
				Type:      auth.GitHub,
				Name:      "GitHub",
				Activated: true,
				Default:   false,
				Config: &github.Config{
					APIEndpoint: "https://api.github.com",
				},
			},
		)
		require.NoError(t, err)

		// Create a user that uses this login source
		_, err = (&users{DB: db.DB}).Create(ctx, "alice", "",
			CreateUserOptions{
				LoginSource: source.ID,
			},
		)
		require.NoError(t, err)

		// Delete the login source will result in error
		err = db.DeleteByID(ctx, source.ID)
		wantErr := ErrLoginSourceInUse{args: errutil.Args{"id": source.ID}}
		assert.Equal(t, wantErr, err)
	})

	mock := NewMockLoginSourceFilesStore()
	mock.GetByIDFunc.SetDefaultHook(func(id int64) (*LoginSource, error) {
		return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
	})
	setMockLoginSourceFilesStore(t, db, mock)

	// Create a login source with name "GitHub2"
	source, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub2",
			Activated: true,
			Default:   false,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		},
	)
	require.NoError(t, err)

	// Delete a non-existent ID is noop
	err = db.DeleteByID(ctx, 9999)
	require.NoError(t, err)

	// We should be able to get it back
	_, err = db.GetByID(ctx, source.ID)
	require.NoError(t, err)

	// Now delete this login source with ID
	err = db.DeleteByID(ctx, source.ID)
	require.NoError(t, err)

	// We should get token not found error
	_, err = db.GetByID(ctx, source.ID)
	wantErr := ErrLoginSourceNotExist{args: errutil.Args{"id": source.ID}}
	assert.Equal(t, wantErr, err)
}

func loginSourcesGetByID(t *testing.T, db *loginSources) {
	ctx := context.Background()

	mock := NewMockLoginSourceFilesStore()
	mock.GetByIDFunc.SetDefaultHook(func(id int64) (*LoginSource, error) {
		if id != 101 {
			return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
		}
		return &LoginSource{ID: id}, nil
	})
	setMockLoginSourceFilesStore(t, db, mock)

	expConfig := &github.Config{
		APIEndpoint: "https://api.github.com",
	}

	// Create a login source with name "GitHub"
	source, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   false,
			Config:    expConfig,
		},
	)
	require.NoError(t, err)

	// Get the one in the database and test the read/write hooks
	source, err = db.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, expConfig, source.Provider.Config())

	// Get the one in source file store
	_, err = db.GetByID(ctx, 101)
	require.NoError(t, err)
}

func loginSourcesList(t *testing.T, db *loginSources) {
	ctx := context.Background()

	mock := NewMockLoginSourceFilesStore()
	mock.ListFunc.SetDefaultHook(func(opts ListLoginSourceOptions) []*LoginSource {
		if opts.OnlyActivated {
			return []*LoginSource{
				{ID: 1},
			}
		}
		return []*LoginSource{
			{ID: 1},
			{ID: 2},
		}
	})
	setMockLoginSourceFilesStore(t, db, mock)

	// Create two login sources in database, one activated and the other one not
	_, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type: auth.PAM,
			Name: "PAM",
			Config: &pam.Config{
				ServiceName: "PAM",
			},
		},
	)
	require.NoError(t, err)
	_, err = db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		},
	)
	require.NoError(t, err)

	// List all login sources
	sources, err := db.List(ctx, ListLoginSourceOptions{})
	require.NoError(t, err)
	assert.Equal(t, 4, len(sources), "number of sources")

	// Only list activated login sources
	sources, err = db.List(ctx, ListLoginSourceOptions{OnlyActivated: true})
	require.NoError(t, err)
	assert.Equal(t, 2, len(sources), "number of sources")
}

func loginSourcesResetNonDefault(t *testing.T, db *loginSources) {
	ctx := context.Background()

	mock := NewMockLoginSourceFilesStore()
	mock.ListFunc.SetDefaultHook(func(opts ListLoginSourceOptions) []*LoginSource {
		mockFile := NewMockLoginSourceFileStore()
		mockFile.SetGeneralFunc.SetDefaultHook(func(name, value string) {
			assert.Equal(t, "is_default", name)
			assert.Equal(t, "false", value)
		})
		return []*LoginSource{
			{
				File: mockFile,
			},
		}
	})
	setMockLoginSourceFilesStore(t, db, mock)

	// Create two login sources both have default on
	source1, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:    auth.PAM,
			Name:    "PAM",
			Default: true,
			Config: &pam.Config{
				ServiceName: "PAM",
			},
		},
	)
	require.NoError(t, err)
	source2, err := db.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.GitHub,
			Name:      "GitHub",
			Activated: true,
			Default:   true,
			Config: &github.Config{
				APIEndpoint: "https://api.github.com",
			},
		},
	)
	require.NoError(t, err)

	// Set source 1 as default
	err = db.ResetNonDefault(ctx, source1)
	require.NoError(t, err)

	// Verify the default state
	source1, err = db.GetByID(ctx, source1.ID)
	require.NoError(t, err)
	assert.True(t, source1.IsDefault)

	source2, err = db.GetByID(ctx, source2.ID)
	require.NoError(t, err)
	assert.False(t, source2.IsDefault)
}

func loginSourcesSave(t *testing.T, db *loginSources) {
	ctx := context.Background()

	t.Run("save to database", func(t *testing.T) {
		// Create a login source with name "GitHub"
		source, err := db.Create(ctx,
			CreateLoginSourceOptions{
				Type:      auth.GitHub,
				Name:      "GitHub",
				Activated: true,
				Default:   false,
				Config: &github.Config{
					APIEndpoint: "https://api.github.com",
				},
			},
		)
		require.NoError(t, err)

		source.IsActived = false
		source.Provider = github.NewProvider(&github.Config{
			APIEndpoint: "https://api2.github.com",
		})
		err = db.Save(ctx, source)
		require.NoError(t, err)

		source, err = db.GetByID(ctx, source.ID)
		require.NoError(t, err)
		assert.False(t, source.IsActived)
		assert.Equal(t, "https://api2.github.com", source.GitHub().APIEndpoint)
	})

	t.Run("save to file", func(t *testing.T) {
		mockFile := NewMockLoginSourceFileStore()
		source := &LoginSource{
			Provider: github.NewProvider(&github.Config{
				APIEndpoint: "https://api.github.com",
			}),
			File: mockFile,
		}
		err := db.Save(ctx, source)
		require.NoError(t, err)
		mockrequire.Called(t, mockFile.SaveFunc)
	})
}
