// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	"gogs.io/gogs/internal/lfsutil"
)

// NOTE: Mocks are sorted in alphabetical order.

var _ AccessTokensStore = (*MockAccessTokensStore)(nil)

type MockAccessTokensStore struct {
	MockCreate     func(ctx context.Context, userID int64, name string) (*AccessToken, error)
	MockDeleteByID func(ctx context.Context, userID, id int64) error
	MockGetBySHA   func(ctx context.Context, sha string) (*AccessToken, error)
	MockList       func(ctx context.Context, userID int64) ([]*AccessToken, error)
	MockSave       func(ctx context.Context, t *AccessToken) error
}

func (m *MockAccessTokensStore) Create(ctx context.Context, userID int64, name string) (*AccessToken, error) {
	return m.MockCreate(ctx, userID, name)
}

func (m *MockAccessTokensStore) DeleteByID(ctx context.Context, userID, id int64) error {
	return m.MockDeleteByID(ctx, userID, id)
}

func (m *MockAccessTokensStore) GetBySHA(ctx context.Context, sha string) (*AccessToken, error) {
	return m.MockGetBySHA(ctx, sha)
}

func (m *MockAccessTokensStore) List(ctx context.Context, userID int64) ([]*AccessToken, error) {
	return m.MockList(ctx, userID)
}

func (m *MockAccessTokensStore) Save(ctx context.Context, t *AccessToken) error {
	return m.MockSave(ctx, t)
}

func SetMockAccessTokensStore(t *testing.T, mock AccessTokensStore) {
	before := AccessTokens
	AccessTokens = mock
	t.Cleanup(func() {
		AccessTokens = before
	})
}

var _ LFSStore = (*MockLFSStore)(nil)

type MockLFSStore struct {
	MockCreateObject     func(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error
	MockGetObjectByOID   func(ctx context.Context, repoID int64, oid lfsutil.OID) (*LFSObject, error)
	MockGetObjectsByOIDs func(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error)
}

func (m *MockLFSStore) CreateObject(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error {
	return m.MockCreateObject(ctx, repoID, oid, size, storage)
}

func (m *MockLFSStore) GetObjectByOID(ctx context.Context, repoID int64, oid lfsutil.OID) (*LFSObject, error) {
	return m.MockGetObjectByOID(ctx, repoID, oid)
}

func (m *MockLFSStore) GetObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error) {
	return m.MockGetObjectsByOIDs(ctx, repoID, oids...)
}

func SetMockLFSStore(t *testing.T, mock LFSStore) {
	before := LFS
	LFS = mock
	t.Cleanup(func() {
		LFS = before
	})
}

var _ loginSourceFilesStore = (*mockLoginSourceFilesStore)(nil)

type mockLoginSourceFilesStore struct {
	MockGetByID func(id int64) (*LoginSource, error)
	MockLen     func() int
	MockList    func(opts ListLoginSourceOpts) []*LoginSource
	MockUpdate  func(source *LoginSource)
}

func (m *mockLoginSourceFilesStore) GetByID(id int64) (*LoginSource, error) {
	return m.MockGetByID(id)
}

func (m *mockLoginSourceFilesStore) Len() int {
	return m.MockLen()
}

func (m *mockLoginSourceFilesStore) List(opts ListLoginSourceOpts) []*LoginSource {
	return m.MockList(opts)
}

func (m *mockLoginSourceFilesStore) Update(source *LoginSource) {
	m.MockUpdate(source)
}

func setMockLoginSourceFilesStore(t *testing.T, db *loginSources, mock loginSourceFilesStore) {
	before := db.files
	db.files = mock
	t.Cleanup(func() {
		db.files = before
	})
}

var _ loginSourceFileStore = (*mockLoginSourceFileStore)(nil)

type mockLoginSourceFileStore struct {
	MockSetGeneral func(name, value string)
	MockSetConfig  func(cfg interface{}) error
	MockSave       func() error
}

func (m *mockLoginSourceFileStore) SetGeneral(name, value string) {
	m.MockSetGeneral(name, value)
}

func (m *mockLoginSourceFileStore) SetConfig(cfg interface{}) error {
	return m.MockSetConfig(cfg)
}

func (m *mockLoginSourceFileStore) Save() error {
	return m.MockSave()
}

var _ PermsStore = (*MockPermsStore)(nil)

type MockPermsStore struct {
	MockAccessMode   func(ctx context.Context, userID, repoID int64, opts AccessModeOptions) AccessMode
	MockAuthorize    func(ctx context.Context, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool
	MockSetRepoPerms func(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error
}

func (m *MockPermsStore) AccessMode(ctx context.Context, userID, repoID int64, opts AccessModeOptions) AccessMode {
	return m.MockAccessMode(ctx, userID, repoID, opts)
}

func (m *MockPermsStore) Authorize(ctx context.Context, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool {
	return m.MockAuthorize(ctx, userID, repoID, desired, opts)
}

func (m *MockPermsStore) SetRepoPerms(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error {
	return m.MockSetRepoPerms(ctx, repoID, accessMap)
}

func SetMockPermsStore(t *testing.T, mock PermsStore) {
	before := Perms
	Perms = mock
	t.Cleanup(func() {
		Perms = before
	})
}

var _ ReposStore = (*MockReposStore)(nil)

type MockReposStore struct {
	MockGetByName func(ctx context.Context, ownerID int64, name string) (*Repository, error)
}

func (m *MockReposStore) GetByName(ctx context.Context, ownerID int64, name string) (*Repository, error) {
	return m.MockGetByName(ctx, ownerID, name)
}

func SetMockReposStore(t *testing.T, mock ReposStore) {
	before := Repos
	Repos = mock
	t.Cleanup(func() {
		Repos = before
	})
}

var _ TwoFactorsStore = (*MockTwoFactorsStore)(nil)

type MockTwoFactorsStore struct {
	MockCreate        func(ctx context.Context, userID int64, key, secret string) error
	MockGetByUserID   func(ctx context.Context, userID int64) (*TwoFactor, error)
	MockIsUserEnabled func(ctx context.Context, userID int64) bool
}

func (m *MockTwoFactorsStore) Create(ctx context.Context, userID int64, key, secret string) error {
	return m.MockCreate(ctx, userID, key, secret)
}

func (m *MockTwoFactorsStore) GetByUserID(ctx context.Context, userID int64) (*TwoFactor, error) {
	return m.MockGetByUserID(ctx, userID)
}

func (m *MockTwoFactorsStore) IsUserEnabled(ctx context.Context, userID int64) bool {
	return m.MockIsUserEnabled(ctx, userID)
}

func SetMockTwoFactorsStore(t *testing.T, mock TwoFactorsStore) {
	before := TwoFactors
	TwoFactors = mock
	t.Cleanup(func() {
		TwoFactors = before
	})
}

var _ UsersStore = (*MockUsersStore)(nil)

type MockUsersStore struct {
	MockAuthenticate  func(ctx context.Context, username, password string, loginSourceID int64) (*User, error)
	MockCreate        func(ctx context.Context, username, email string, opts CreateUserOpts) (*User, error)
	MockGetByEmail    func(ctx context.Context, email string) (*User, error)
	MockGetByID       func(ctx context.Context, id int64) (*User, error)
	MockGetByUsername func(ctx context.Context, username string) (*User, error)
}

func (m *MockUsersStore) Authenticate(ctx context.Context, username, password string, loginSourceID int64) (*User, error) {
	return m.MockAuthenticate(ctx, username, password, loginSourceID)
}

func (m *MockUsersStore) Create(ctx context.Context, username, email string, opts CreateUserOpts) (*User, error) {
	return m.MockCreate(ctx, username, email, opts)
}

func (m *MockUsersStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	return m.MockGetByEmail(ctx, email)
}

func (m *MockUsersStore) GetByID(ctx context.Context, id int64) (*User, error) {
	return m.MockGetByID(ctx, id)
}

func (m *MockUsersStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	return m.MockGetByUsername(ctx, username)
}

func SetMockUsersStore(t *testing.T, mock UsersStore) {
	before := Users
	Users = mock
	t.Cleanup(func() {
		Users = before
	})
}
