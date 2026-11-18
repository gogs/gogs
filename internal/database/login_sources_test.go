package database

import (
	"context"
	"testing"
	"time"

	mockrequire "github.com/derision-test/go-mockgen/v2/testutil/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/errx"
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
		s := &LoginSource{
			CreatedUnix: 1,
		}
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

func TestLoginSource_BeforeUpdate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	s := &LoginSource{}
	_ = s.BeforeUpdate(db)
	assert.Equal(t, db.NowFunc().Unix(), s.UpdatedUnix)
}

func TestLoginSource_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	tests := []struct {
		name     string
		authType auth.Type
		wantType any
	}{
		{
			name:     "LDAP",
			authType: auth.LDAP,
			wantType: &ldap.Provider{},
		},
		{
			name:     "DLDAP",
			authType: auth.DLDAP,
			wantType: &ldap.Provider{},
		},
		{
			name:     "SMTP",
			authType: auth.SMTP,
			wantType: &smtp.Provider{},
		},
		{
			name:     "PAM",
			authType: auth.PAM,
			wantType: &pam.Provider{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := LoginSource{
				Type:        test.authType,
				Config:      `{}`,
				CreatedUnix: now.Unix(),
				UpdatedUnix: now.Unix(),
			}
			err := s.AfterFind(db)
			require.NoError(t, err)

			assert.Equal(t, s.CreatedUnix, s.Created.Unix())
			assert.Equal(t, s.UpdatedUnix, s.Updated.Unix())
			assert.IsType(t, test.wantType, s.Provider)
		})
	}
}

func TestLoginSources(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &LoginSourcesStore{
		db: newTestDB(t, "LoginSourcesStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *LoginSourcesStore)
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
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func loginSourcesCreate(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
	source, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM",
			Activated: true,
			Default:   false,
			Config: &pam.Config{
				ServiceName: "login",
			},
		},
	)
	require.NoError(t, err)

	source, err = s.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), source.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), source.Updated.UTC().Format(time.RFC3339))

	// A second login source with the same name cannot be created.
	_, err = s.Create(ctx, CreateLoginSourceOptions{Name: source.Name})
	wantErr := ErrLoginSourceAlreadyExist{args: errx.Args{"name": source.Name}}
	assert.Equal(t, wantErr, err)
}

func setMockLoginSourceFilesStore(t *testing.T, s *LoginSourcesStore, mock loginSourceFilesStore) {
	before := s.files
	s.files = mock
	t.Cleanup(func() {
		s.files = before
	})
}

func loginSourcesCount(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
	// One login source is in the database, and two are source files.
	_, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM",
			Activated: true,
			Default:   false,
			Config: &pam.Config{
				ServiceName: "login",
			},
		},
	)
	require.NoError(t, err)

	mock := NewMockLoginSourceFilesStore()
	mock.LenFunc.SetDefaultReturn(2)
	setMockLoginSourceFilesStore(t, s, mock)

	assert.Equal(t, int64(3), s.Count(ctx))
}

func loginSourcesDeleteByID(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
	t.Run("delete but in used", func(t *testing.T) {
		source, err := s.Create(ctx,
			CreateLoginSourceOptions{
				Type:      auth.PAM,
				Name:      "Corp PAM",
				Activated: true,
				Default:   false,
				Config: &pam.Config{
					ServiceName: "login",
				},
			},
		)
		require.NoError(t, err)

		_, err = newUsersStore(s.db).Create(ctx, "alice", "",
			CreateUserOptions{
				LoginSource: source.ID,
			},
		)
		require.NoError(t, err)

		// A login source that a user uses cannot be deleted.
		err = s.DeleteByID(ctx, source.ID)
		wantErr := ErrLoginSourceInUse{args: errx.Args{"id": source.ID}}
		assert.Equal(t, wantErr, err)
	})

	mock := NewMockLoginSourceFilesStore()
	mock.GetByIDFunc.SetDefaultHook(func(id int64) (*LoginSource, error) {
		return nil, ErrLoginSourceNotExist{args: errx.Args{"id": id}}
	})
	setMockLoginSourceFilesStore(t, s, mock)

	source, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM2",
			Activated: true,
			Default:   false,
			Config: &pam.Config{
				ServiceName: "login",
			},
		},
	)
	require.NoError(t, err)

	// Deleting a non-existent ID is a no-op.
	err = s.DeleteByID(ctx, 9999)
	require.NoError(t, err)

	_, err = s.GetByID(ctx, source.ID)
	require.NoError(t, err)

	err = s.DeleteByID(ctx, source.ID)
	require.NoError(t, err)

	_, err = s.GetByID(ctx, source.ID)
	wantErr := ErrLoginSourceNotExist{args: errx.Args{"id": source.ID}}
	assert.Equal(t, wantErr, err)
}

func loginSourcesGetByID(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
	mock := NewMockLoginSourceFilesStore()
	mock.GetByIDFunc.SetDefaultHook(func(id int64) (*LoginSource, error) {
		if id != 101 {
			return nil, ErrLoginSourceNotExist{args: errx.Args{"id": id}}
		}
		return &LoginSource{ID: id}, nil
	})
	setMockLoginSourceFilesStore(t, s, mock)

	expConfig := &pam.Config{
		ServiceName: "login",
	}

	source, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM",
			Activated: true,
			Default:   false,
			Config:    expConfig,
		},
	)
	require.NoError(t, err)

	// Get the one in the database and test the read/write hooks.
	source, err = s.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, expConfig, source.Provider.Config())

	// Get the one in source file store.
	_, err = s.GetByID(ctx, 101)
	require.NoError(t, err)
}

func loginSourcesList(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
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
	setMockLoginSourceFilesStore(t, s, mock)

	// Create two login sources in database, one activated and the other one not.
	_, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type: auth.PAM,
			Name: "PAM",
			Config: &pam.Config{
				ServiceName: "PAM",
			},
		},
	)
	require.NoError(t, err)
	_, err = s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM",
			Activated: true,
			Config: &pam.Config{
				ServiceName: "login",
			},
		},
	)
	require.NoError(t, err)
	// A GitHub source stored before its support was removed is left out.
	err = s.db.Exec(`INSERT INTO login_source (type, name, is_actived, cfg) VALUES (?, ?, ?, ?)`,
		auth.Removed, "GitHub", true, `{"APIEndpoint":"https://api.github.com/"}`).Error
	require.NoError(t, err)

	sources, err := s.List(ctx, ListLoginSourceOptions{})
	require.NoError(t, err)
	assert.Equal(t, 4, len(sources), "number of sources")

	sources, err = s.List(ctx, ListLoginSourceOptions{OnlyActivated: true})
	require.NoError(t, err)
	assert.Equal(t, 2, len(sources), "number of sources")
}

func loginSourcesResetNonDefault(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
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
	setMockLoginSourceFilesStore(t, s, mock)

	// Create two login sources that both are default.
	source1, err := s.Create(ctx,
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
	source2, err := s.Create(ctx,
		CreateLoginSourceOptions{
			Type:      auth.PAM,
			Name:      "Corp PAM",
			Activated: true,
			Default:   true,
			Config: &pam.Config{
				ServiceName: "login",
			},
		},
	)
	require.NoError(t, err)

	err = s.ResetNonDefault(ctx, source1)
	require.NoError(t, err)

	source1, err = s.GetByID(ctx, source1.ID)
	require.NoError(t, err)
	assert.True(t, source1.IsDefault)

	source2, err = s.GetByID(ctx, source2.ID)
	require.NoError(t, err)
	assert.False(t, source2.IsDefault)
}

func loginSourcesSave(t *testing.T, ctx context.Context, s *LoginSourcesStore) {
	t.Run("save to database", func(t *testing.T) {
		source, err := s.Create(ctx,
			CreateLoginSourceOptions{
				Type:      auth.PAM,
				Name:      "Corp PAM",
				Activated: true,
				Default:   false,
				Config: &pam.Config{
					ServiceName: "login",
				},
			},
		)
		require.NoError(t, err)

		source.IsActived = false
		source.Provider = pam.NewProvider(&pam.Config{
			ServiceName: "system-auth",
		})
		err = s.Save(ctx, source)
		require.NoError(t, err)

		source, err = s.GetByID(ctx, source.ID)
		require.NoError(t, err)
		assert.False(t, source.IsActived)
		assert.Equal(t, "system-auth", source.Provider.Config().(*pam.Config).ServiceName)
	})

	t.Run("save to file", func(t *testing.T) {
		mockFile := NewMockLoginSourceFileStore()
		source := &LoginSource{
			Provider: pam.NewProvider(&pam.Config{
				ServiceName: "login",
			}),
			File: mockFile,
		}
		err := s.Save(ctx, source)
		require.NoError(t, err)
		mockrequire.Called(t, mockFile.SaveFunc)
	})
}
