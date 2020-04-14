// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"gogs.io/gogs/internal/lfsutil"
)

// NOTE: Mocks are sorted in alphabetical order.

var _ AccessTokensStore = (*MockAccessTokensStore)(nil)

type MockAccessTokensStore struct {
	MockCreate     func(userID int64, name string) (*AccessToken, error)
	MockDeleteByID func(userID, id int64) error
	MockGetBySHA   func(sha string) (*AccessToken, error)
	MockList       func(userID int64) ([]*AccessToken, error)
	MockSave       func(t *AccessToken) error
}

func (m *MockAccessTokensStore) Create(userID int64, name string) (*AccessToken, error) {
	return m.MockCreate(userID, name)
}

func (m *MockAccessTokensStore) DeleteByID(userID, id int64) error {
	return m.MockDeleteByID(userID, id)
}

func (m *MockAccessTokensStore) GetBySHA(sha string) (*AccessToken, error) {
	return m.MockGetBySHA(sha)
}

func (m *MockAccessTokensStore) List(userID int64) ([]*AccessToken, error) {
	return m.MockList(userID)
}

func (m *MockAccessTokensStore) Save(t *AccessToken) error {
	return m.MockSave(t)
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
	MockCreateObject     func(repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error
	MockGetObjectByOID   func(repoID int64, oid lfsutil.OID) (*LFSObject, error)
	MockGetObjectsByOIDs func(repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error)
}

func (m *MockLFSStore) CreateObject(repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error {
	return m.MockCreateObject(repoID, oid, size, storage)
}

func (m *MockLFSStore) GetObjectByOID(repoID int64, oid lfsutil.OID) (*LFSObject, error) {
	return m.MockGetObjectByOID(repoID, oid)
}

func (m *MockLFSStore) GetObjectsByOIDs(repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error) {
	return m.MockGetObjectsByOIDs(repoID, oids...)
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
	MockAccessMode   func(userID int64, repo *Repository) AccessMode
	MockAuthorize    func(userID int64, repo *Repository, desired AccessMode) bool
	MockSetRepoPerms func(repoID int64, accessMap map[int64]AccessMode) error
}

func (m *MockPermsStore) AccessMode(userID int64, repo *Repository) AccessMode {
	return m.MockAccessMode(userID, repo)
}

func (m *MockPermsStore) Authorize(userID int64, repo *Repository, desired AccessMode) bool {
	return m.MockAuthorize(userID, repo, desired)
}

func (m *MockPermsStore) SetRepoPerms(repoID int64, accessMap map[int64]AccessMode) error {
	return m.MockSetRepoPerms(repoID, accessMap)
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
	MockGetByName func(ownerID int64, name string) (*Repository, error)
}

func (m *MockReposStore) GetByName(ownerID int64, name string) (*Repository, error) {
	return m.MockGetByName(ownerID, name)
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
	MockCreate        func(userID int64, key, secret string) error
	MockGetByUserID   func(userID int64) (*TwoFactor, error)
	MockIsUserEnabled func(userID int64) bool
}

func (m *MockTwoFactorsStore) Create(userID int64, key, secret string) error {
	return m.MockCreate(userID, key, secret)
}

func (m *MockTwoFactorsStore) GetByUserID(userID int64) (*TwoFactor, error) {
	return m.MockGetByUserID(userID)
}

func (m *MockTwoFactorsStore) IsUserEnabled(userID int64) bool {
	return m.MockIsUserEnabled(userID)
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
	MockAuthenticate  func(username, password string, loginSourceID int64) (*User, error)
	MockGetByID       func(id int64) (*User, error)
	MockGetByUsername func(username string) (*User, error)
}

func (m *MockUsersStore) Authenticate(username, password string, loginSourceID int64) (*User, error) {
	return m.MockAuthenticate(username, password, loginSourceID)
}

func (m *MockUsersStore) GetByID(id int64) (*User, error) {
	return m.MockGetByID(id)
}

func (m *MockUsersStore) GetByUsername(username string) (*User, error) {
	return m.MockGetByUsername(username)
}

func SetMockUsersStore(t *testing.T, mock UsersStore) {
	before := Users
	Users = mock
	t.Cleanup(func() {
		Users = before
	})
}
