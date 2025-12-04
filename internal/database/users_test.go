// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/repoutil"
	"gogs.io/gogs/internal/userutil"
	"gogs.io/gogs/public"
)

func TestUser_BeforeCreate(t *testing.T) {
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
		user := &User{
			CreatedUnix: 1,
		}
		_ = user.BeforeCreate(db)
		assert.Equal(t, int64(1), user.CreatedUnix)
		assert.Equal(t, int64(0), user.UpdatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		user := &User{}
		_ = user.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), user.CreatedUnix)
		assert.Equal(t, db.NowFunc().Unix(), user.UpdatedUnix)
	})
}

func TestUser_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	user := &User{
		FullName:    "user1<script src=http://localhost:8181/xss.js>",
		CreatedUnix: now.Unix(),
		UpdatedUnix: now.Unix(),
	}
	_ = user.AfterFind(db)
	assert.Equal(t, "user1", user.FullName)
	assert.Equal(t, user.CreatedUnix, user.Created.Unix())
	assert.Equal(t, user.UpdatedUnix, user.Updated.Unix())
}

func TestUsers(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &UsersStore{
		db: newTestDB(t, "UsersStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *UsersStore)
	}{
		{"Authenticate", usersAuthenticate},
		{"ChangeUsername", usersChangeUsername},
		{"Count", usersCount},
		{"Create", usersCreate},
		{"DeleteCustomAvatar", usersDeleteCustomAvatar},
		{"DeleteByID", usersDeleteByID},
		{"DeleteInactivated", usersDeleteInactivated},
		{"GetByEmail", usersGetByEmail},
		{"GetByID", usersGetByID},
		{"GetByUsername", usersGetByUsername},
		{"GetByKeyID", usersGetByKeyID},
		{"GetMailableEmailsByUsernames", usersGetMailableEmailsByUsernames},
		{"IsUsernameUsed", usersIsUsernameUsed},
		{"List", usersList},
		{"ListFollowers", usersListFollowers},
		{"ListFollowings", usersListFollowings},
		{"SearchByName", usersSearchByName},
		{"Update", usersUpdate},
		{"UseCustomAvatar", usersUseCustomAvatar},
		{"AddEmail", usersAddEmail},
		{"GetEmail", usersGetEmail},
		{"ListEmails", usersListEmails},
		{"MarkEmailActivated", usersMarkEmailActivated},
		{"MarkEmailPrimary", usersMarkEmailPrimary},
		{"DeleteEmail", usersDeleteEmail},
		{"Follow", usersFollow},
		{"IsFollowing", usersIsFollowing},
		{"Unfollow", usersUnfollow},
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

func usersAuthenticate(t *testing.T, ctx context.Context, s *UsersStore) {
	password := "pa$$word"
	alice, err := s.Create(ctx, "alice", "alice@example.com",
		CreateUserOptions{
			Password: password,
		},
	)
	require.NoError(t, err)

	t.Run("user not found", func(t *testing.T) {
		_, err := s.Authenticate(ctx, "bob", password, -1)
		wantErr := auth.ErrBadCredentials{Args: map[string]any{"login": "bob"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("invalid password", func(t *testing.T) {
		_, err := s.Authenticate(ctx, alice.Name, "bad_password", -1)
		wantErr := auth.ErrBadCredentials{Args: map[string]any{"login": alice.Name, "userID": alice.ID}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("via email and password", func(t *testing.T) {
		user, err := s.Authenticate(ctx, alice.Email, password, -1)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("via username and password", func(t *testing.T) {
		user, err := s.Authenticate(ctx, alice.Name, password, -1)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("login source mismatch", func(t *testing.T) {
		_, err := s.Authenticate(ctx, alice.Email, password, 1)
		gotErr := fmt.Sprintf("%v", err)
		wantErr := ErrLoginSourceMismatch{args: map[string]any{"actual": 0, "expect": 1}}.Error()
		assert.Equal(t, wantErr, gotErr)
	})

	t.Run("via login source", func(t *testing.T) {
		loginSourcesStore := newLoginSourcesStore(s.db, NewMockLoginSourceFilesStore())
		loginSource, err := loginSourcesStore.Create(
			ctx,
			CreateLoginSourceOptions{
				Type:      auth.Mock,
				Name:      "mock-1",
				Activated: true,
				Config: mockProviderConfig{
					ExternalAccount: &auth.ExternalAccount{},
				},
			},
		)
		require.NoError(t, err)

		bob, err := s.Create(ctx, "bob", "bob@example.com",
			CreateUserOptions{
				Password:    password,
				LoginSource: 1,
			},
		)
		require.NoError(t, err)

		user, err := s.Authenticate(ctx, bob.Email, password, loginSource.ID)
		require.NoError(t, err)
		assert.Equal(t, bob.Name, user.Name)
	})

	t.Run("new user via login source", func(t *testing.T) {
		loginSourcesStore := newLoginSourcesStore(s.db, NewMockLoginSourceFilesStore())
		loginSource, err := loginSourcesStore.Create(
			ctx,
			CreateLoginSourceOptions{
				Type:      auth.Mock,
				Name:      "mock-2",
				Activated: true,
				Config: mockProviderConfig{
					ExternalAccount: &auth.ExternalAccount{
						Name:  "cindy",
						Email: "cindy@example.com",
					},
				},
			},
		)
		require.NoError(t, err)

		user, err := s.Authenticate(ctx, "cindy", password, loginSource.ID)
		require.NoError(t, err)
		assert.Equal(t, "cindy", user.Name)

		user, err = s.GetByUsername(ctx, "cindy")
		require.NoError(t, err)
		assert.Equal(t, "cindy@example.com", user.Email)
	})
}

func usersChangeUsername(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(
		ctx,
		"alice",
		"alice@example.com",
		CreateUserOptions{
			Activated: true,
		},
	)
	require.NoError(t, err)

	t.Run("name not allowed", func(t *testing.T) {
		err := s.ChangeUsername(ctx, alice.ID, "-")
		wantErr := ErrNameNotAllowed{
			args: errutil.Args{
				"reason": "reserved",
				"name":   "-",
			},
		}
		assert.Equal(t, wantErr, err)
	})

	t.Run("name already exists", func(t *testing.T) {
		bob, err := s.Create(
			ctx,
			"bob",
			"bob@example.com",
			CreateUserOptions{
				Activated: true,
			},
		)
		require.NoError(t, err)

		err = s.ChangeUsername(ctx, alice.ID, bob.Name)
		wantErr := ErrUserAlreadyExist{
			args: errutil.Args{
				"name": bob.Name,
			},
		}
		assert.Equal(t, wantErr, err)
	})

	tempRepositoryRoot := filepath.Join(os.TempDir(), "usersChangeUsername-tempRepositoryRoot")
	conf.SetMockRepository(
		t,
		conf.RepositoryOpts{
			Root: tempRepositoryRoot,
		},
	)
	err = os.RemoveAll(tempRepositoryRoot)
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempRepositoryRoot) }()

	tempServerAppDataPath := filepath.Join(os.TempDir(), "usersChangeUsername-tempServerAppDataPath")
	conf.SetMockServer(
		t,
		conf.ServerOpts{
			AppDataPath: tempServerAppDataPath,
		},
	)
	err = os.RemoveAll(tempServerAppDataPath)
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempServerAppDataPath) }()

	repo, err := newReposStore(s.db).Create(
		ctx,
		alice.ID,
		CreateRepoOptions{
			Name: "test-repo-1",
		},
	)
	require.NoError(t, err)

	// TODO: Use PullRequests.Create to replace SQL hack when the method is available.
	err = s.db.Exec(`INSERT INTO pull_request (head_user_name) VALUES (?)`, alice.Name).Error
	require.NoError(t, err)

	err = s.db.Model(&User{}).Where("id = ?", alice.ID).Update("updated_unix", 0).Error
	require.NoError(t, err)

	err = os.MkdirAll(repoutil.UserPath(alice.Name), os.ModePerm)
	require.NoError(t, err)
	err = os.MkdirAll(repoutil.RepositoryLocalPath(repo.ID), os.ModePerm)
	require.NoError(t, err)
	err = os.MkdirAll(repoutil.RepositoryLocalWikiPath(repo.ID), os.ModePerm)
	require.NoError(t, err)

	// Make sure mock data is set up correctly
	// TODO: Use PullRequests.GetByID to replace SQL hack when the method is available.
	var headUserName string
	err = s.db.Model(&PullRequest{}).Select("head_user_name").Row().Scan(&headUserName)
	require.NoError(t, err)
	assert.Equal(t, headUserName, alice.Name)

	var updatedUnix int64
	err = s.db.Model(&User{}).Select("updated_unix").Where("id = ?", alice.ID).Row().Scan(&updatedUnix)
	require.NoError(t, err)
	assert.Equal(t, int64(0), updatedUnix)

	assert.True(t, osutil.IsExist(repoutil.UserPath(alice.Name)))
	assert.True(t, osutil.IsExist(repoutil.RepositoryLocalPath(repo.ID)))
	assert.True(t, osutil.IsExist(repoutil.RepositoryLocalWikiPath(repo.ID)))

	const newUsername = "alice-new"
	err = s.ChangeUsername(ctx, alice.ID, newUsername)
	require.NoError(t, err)

	// TODO: Use PullRequests.GetByID to replace SQL hack when the method is available.
	err = s.db.Model(&PullRequest{}).Select("head_user_name").Row().Scan(&headUserName)
	require.NoError(t, err)
	assert.Equal(t, headUserName, newUsername)

	assert.True(t, osutil.IsExist(repoutil.UserPath(newUsername)))
	assert.False(t, osutil.IsExist(repoutil.UserPath(alice.Name)))
	assert.False(t, osutil.IsExist(repoutil.RepositoryLocalPath(repo.ID)))
	assert.False(t, osutil.IsExist(repoutil.RepositoryLocalWikiPath(repo.ID)))

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, newUsername, alice.Name)
	assert.Equal(t, s.db.NowFunc().Unix(), alice.UpdatedUnix)

	// Change the cases of the username should just be fine
	err = s.ChangeUsername(ctx, alice.ID, strings.ToUpper(newUsername))
	require.NoError(t, err)
	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, strings.ToUpper(newUsername), alice.Name)
}

func usersCount(t *testing.T, ctx context.Context, s *UsersStore) {
	// Has no user initially
	got := s.Count(ctx)
	assert.Equal(t, int64(0), got)

	_, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	got = s.Count(ctx)
	assert.Equal(t, int64(1), got)

	// Create an organization shouldn't count
	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org1, err := s.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(
		dbutil.Quote("UPDATE %s SET type = ? WHERE id = ?", "user"),
		UserTypeOrganization, org1.ID,
	).Error
	require.NoError(t, err)
	got = s.Count(ctx)
	assert.Equal(t, int64(1), got)
}

func usersCreate(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(
		ctx,
		"alice",
		"alice@example.com",
		CreateUserOptions{
			Activated: true,
		},
	)
	require.NoError(t, err)

	t.Run("name not allowed", func(t *testing.T) {
		_, err := s.Create(ctx, "-", "", CreateUserOptions{})
		wantErr := ErrNameNotAllowed{
			args: errutil.Args{
				"reason": "reserved",
				"name":   "-",
			},
		}
		assert.Equal(t, wantErr, err)
	})

	t.Run("name already exists", func(t *testing.T) {
		_, err := s.Create(ctx, alice.Name, "", CreateUserOptions{})
		wantErr := ErrUserAlreadyExist{
			args: errutil.Args{
				"name": alice.Name,
			},
		}
		assert.Equal(t, wantErr, err)
	})

	t.Run("email already exists", func(t *testing.T) {
		_, err := s.Create(ctx, "bob", alice.Email, CreateUserOptions{})
		wantErr := ErrEmailAlreadyUsed{
			args: errutil.Args{
				"email": alice.Email,
			},
		}
		assert.Equal(t, wantErr, err)
	})

	user, err := s.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), user.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), user.Updated.UTC().Format(time.RFC3339))
}

func usersDeleteCustomAvatar(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	avatar, err := public.Files.ReadFile("img/avatar_default.png")
	require.NoError(t, err)

	avatarPath := userutil.CustomAvatarPath(alice.ID)
	_ = os.Remove(avatarPath)
	defer func() { _ = os.Remove(avatarPath) }()

	err = s.UseCustomAvatar(ctx, alice.ID, avatar)
	require.NoError(t, err)

	// Make sure avatar is saved and the user flag is updated.
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, alice.UseCustomAvatar)

	// Delete avatar should remove the file and revert the user flag.
	err = s.DeleteCustomAvatar(ctx, alice.ID)
	require.NoError(t, err)

	got = osutil.IsFile(avatarPath)
	assert.False(t, got)

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.False(t, alice.UseCustomAvatar)
}

func usersDeleteByID(t *testing.T, ctx context.Context, s *UsersStore) {
	reposStore := newReposStore(s.db)

	t.Run("user still has repository ownership", func(t *testing.T) {
		alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
		require.NoError(t, err)

		_, err = reposStore.Create(ctx, alice.ID, CreateRepoOptions{Name: "repo1"})
		require.NoError(t, err)

		err = s.DeleteByID(ctx, alice.ID, false)
		wantErr := ErrUserOwnRepos{errutil.Args{"userID": alice.ID}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("user still has organization membership", func(t *testing.T) {
		bob, err := s.Create(ctx, "bob", "bob@exmaple.com", CreateUserOptions{})
		require.NoError(t, err)

		// TODO: Use Orgs.Create to replace SQL hack when the method is available.
		org1, err := s.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
		require.NoError(t, err)
		err = s.db.Exec(
			dbutil.Quote("UPDATE %s SET type = ? WHERE id IN (?)", "user"),
			UserTypeOrganization, org1.ID,
		).Error
		require.NoError(t, err)

		// TODO: Use Orgs.Join to replace SQL hack when the method is available.
		err = s.db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, bob.ID, org1.ID).Error
		require.NoError(t, err)

		err = s.DeleteByID(ctx, bob.ID, false)
		wantErr := ErrUserHasOrgs{errutil.Args{"userID": bob.ID}}
		assert.Equal(t, wantErr, err)
	})

	cindy, err := s.Create(ctx, "cindy", "cindy@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)
	frank, err := s.Create(ctx, "frank", "frank@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)
	repo2, err := reposStore.Create(ctx, cindy.ID, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)

	testUser, err := s.Create(ctx, "testUser", "testUser@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	// Mock watches, stars and follows
	err = reposStore.Watch(ctx, testUser.ID, repo2.ID)
	require.NoError(t, err)
	err = reposStore.Star(ctx, testUser.ID, repo2.ID)
	require.NoError(t, err)
	err = s.Follow(ctx, testUser.ID, cindy.ID)
	require.NoError(t, err)
	err = s.Follow(ctx, frank.ID, testUser.ID)
	require.NoError(t, err)

	// Mock "authorized_keys" file
	// TODO: Use PublicKeys.Add to replace SQL hack when the method is available.
	publicKey := &PublicKey{
		OwnerID:     testUser.ID,
		Name:        "test-key",
		Fingerprint: "12:f8:7e:78:61:b4:bf:e2:de:24:15:96:4e:d4:72:53",
		Content:     "test-key-content",
	}
	err = s.db.Create(publicKey).Error
	require.NoError(t, err)
	tempSSHRootPath := filepath.Join(os.TempDir(), "usersDeleteByID-tempSSHRootPath")
	conf.SetMockSSH(t, conf.SSHOpts{RootPath: tempSSHRootPath})
	err = newPublicKeysStore(s.db).RewriteAuthorizedKeys()
	require.NoError(t, err)

	// Mock issue assignee
	// TODO: Use Issues.Assign to replace SQL hack when the method is available.
	issue := &Issue{
		RepoID:     repo2.ID,
		Index:      1,
		PosterID:   cindy.ID,
		Title:      "test-issue",
		AssigneeID: testUser.ID,
	}
	err = s.db.Create(issue).Error
	require.NoError(t, err)

	// Mock random entries in related tables
	for _, table := range []any{
		&AccessToken{UserID: testUser.ID},
		&Collaboration{UserID: testUser.ID},
		&Access{UserID: testUser.ID},
		&Action{UserID: testUser.ID},
		&IssueUser{UserID: testUser.ID},
		&EmailAddress{UserID: testUser.ID},
	} {
		err = s.db.Create(table).Error
		require.NoError(t, err, "table for %T", table)
	}

	// Mock user directory
	tempRepositoryRoot := filepath.Join(os.TempDir(), "usersDeleteByID-tempRepositoryRoot")
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: tempRepositoryRoot})
	tempUserPath := repoutil.UserPath(testUser.Name)
	err = os.MkdirAll(tempUserPath, os.ModePerm)
	require.NoError(t, err)

	// Mock user custom avatar
	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "usersDeleteByID-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})
	err = os.MkdirAll(tempPictureAvatarUploadPath, os.ModePerm)
	require.NoError(t, err)
	tempCustomAvatarPath := userutil.CustomAvatarPath(testUser.ID)
	err = os.WriteFile(tempCustomAvatarPath, []byte("test"), 0600)
	require.NoError(t, err)

	// Verify mock data
	repo2, err = reposStore.GetByID(ctx, repo2.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, repo2.NumWatches) // The owner is watching the repo by default.
	assert.Equal(t, 1, repo2.NumStars)

	cindy, err = s.GetByID(ctx, cindy.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, cindy.NumFollowers)
	frank, err = s.GetByID(ctx, frank.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, frank.NumFollowing)

	authorizedKeys, err := os.ReadFile(authorizedKeysPath())
	require.NoError(t, err)
	assert.Contains(t, string(authorizedKeys), fmt.Sprintf("key-%d", publicKey.ID))
	assert.Contains(t, string(authorizedKeys), publicKey.Content)

	// TODO: Use Issues.GetByID to replace SQL hack when the method is available.
	err = s.db.First(issue, issue.ID).Error
	require.NoError(t, err)
	assert.Equal(t, testUser.ID, issue.AssigneeID)

	relatedTables := []any{
		&Watch{UserID: testUser.ID},
		&Star{UserID: testUser.ID},
		&Follow{UserID: testUser.ID},
		&PublicKey{OwnerID: testUser.ID},
		&AccessToken{UserID: testUser.ID},
		&Collaboration{UserID: testUser.ID},
		&Access{UserID: testUser.ID},
		&Action{UserID: testUser.ID},
		&IssueUser{UserID: testUser.ID},
		&EmailAddress{UserID: testUser.ID},
	}
	for _, table := range relatedTables {
		var count int64
		err = s.db.Model(table).Where(table).Count(&count).Error
		require.NoError(t, err, "table for %T", table)
		assert.NotZero(t, count, "table for %T", table)
	}

	assert.True(t, osutil.IsExist(tempUserPath))
	assert.True(t, osutil.IsExist(tempCustomAvatarPath))

	// Pull the trigger
	err = s.DeleteByID(ctx, testUser.ID, false)
	require.NoError(t, err)

	// Verify after-the-fact data
	repo2, err = reposStore.GetByID(ctx, repo2.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, repo2.NumWatches) // The owner is watching the repo by default.
	assert.Equal(t, 0, repo2.NumStars)

	cindy, err = s.GetByID(ctx, cindy.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, cindy.NumFollowers)
	frank, err = s.GetByID(ctx, frank.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, frank.NumFollowing)

	authorizedKeys, err = os.ReadFile(authorizedKeysPath())
	require.NoError(t, err)
	assert.Empty(t, authorizedKeys)

	// TODO: Use Issues.GetByID to replace SQL hack when the method is available.
	err = s.db.First(issue, issue.ID).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), issue.AssigneeID)

	for _, table := range []any{
		&Watch{UserID: testUser.ID},
		&Star{UserID: testUser.ID},
		&Follow{UserID: testUser.ID},
		&PublicKey{OwnerID: testUser.ID},
		&AccessToken{UserID: testUser.ID},
		&Collaboration{UserID: testUser.ID},
		&Access{UserID: testUser.ID},
		&Action{UserID: testUser.ID},
		&IssueUser{UserID: testUser.ID},
		&EmailAddress{UserID: testUser.ID},
	} {
		var count int64
		err = s.db.Model(table).Where(table).Count(&count).Error
		require.NoError(t, err, "table for %T", table)
		assert.Equal(t, int64(0), count, "table for %T", table)
	}

	assert.False(t, osutil.IsExist(tempUserPath))
	assert.False(t, osutil.IsExist(tempCustomAvatarPath))

	_, err = s.GetByID(ctx, testUser.ID)
	wantErr := ErrUserNotExist{errutil.Args{"userID": testUser.ID}}
	assert.Equal(t, wantErr, err)
}

func usersDeleteInactivated(t *testing.T, ctx context.Context, s *UsersStore) {
	// User with repository ownership should be skipped
	alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)
	reposStore := newReposStore(s.db)
	_, err = reposStore.Create(ctx, alice.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	// User with organization membership should be skipped
	bob, err := s.Create(ctx, "bob", "bob@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)
	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org1, err := s.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(
		dbutil.Quote("UPDATE %s SET type = ? WHERE id IN (?)", "user"),
		UserTypeOrganization, org1.ID,
	).Error
	require.NoError(t, err)
	// TODO: Use Orgs.Join to replace SQL hack when the method is available.
	err = s.db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, bob.ID, org1.ID).Error
	require.NoError(t, err)

	// User activated state should be skipped
	_, err = s.Create(ctx, "cindy", "cindy@exmaple.com", CreateUserOptions{Activated: true})
	require.NoError(t, err)

	// User meant to be deleted
	david, err := s.Create(ctx, "david", "david@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	tempSSHRootPath := filepath.Join(os.TempDir(), "usersDeleteInactivated-tempSSHRootPath")
	conf.SetMockSSH(t, conf.SSHOpts{RootPath: tempSSHRootPath})

	err = s.DeleteInactivated()
	require.NoError(t, err)

	_, err = s.GetByID(ctx, david.ID)
	wantErr := ErrUserNotExist{errutil.Args{"userID": david.ID}}
	assert.Equal(t, wantErr, err)

	users, err := s.List(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, users, 3)
}

func usersGetByEmail(t *testing.T, ctx context.Context, s *UsersStore) {
	t.Run("empty email", func(t *testing.T) {
		_, err := s.GetByEmail(ctx, "")
		wantErr := ErrUserNotExist{args: errutil.Args{"email": ""}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("ignore organization", func(t *testing.T) {
		// TODO: Use Orgs.Create to replace SQL hack when the method is available.
		org, err := s.Create(ctx, "gogs", "gogs@exmaple.com", CreateUserOptions{})
		require.NoError(t, err)

		err = s.db.Model(&User{}).Where("id", org.ID).UpdateColumn("type", UserTypeOrganization).Error
		require.NoError(t, err)

		_, err = s.GetByEmail(ctx, org.Email)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": org.Email}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("by primary email", func(t *testing.T) {
		alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
		require.NoError(t, err)

		_, err = s.GetByEmail(ctx, alice.Email)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, wantErr, err)

		// Mark user as activated
		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = s.db.Model(&User{}).Where("id", alice.ID).UpdateColumn("is_active", true).Error
		require.NoError(t, err)

		user, err := s.GetByEmail(ctx, alice.Email)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("by secondary email", func(t *testing.T) {
		bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
		require.NoError(t, err)

		// TODO: Use UserEmails.Create to replace SQL hack when the method is available.
		email2 := "bob2@exmaple.com"
		err = s.db.Exec(`INSERT INTO email_address (uid, email) VALUES (?, ?)`, bob.ID, email2).Error
		require.NoError(t, err)

		_, err = s.GetByEmail(ctx, email2)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": email2}}
		assert.Equal(t, wantErr, err)

		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = s.db.Exec(`UPDATE email_address SET is_activated = ? WHERE email = ?`, true, email2).Error
		require.NoError(t, err)

		user, err := s.GetByEmail(ctx, email2)
		require.NoError(t, err)
		assert.Equal(t, bob.Name, user.Name)
	})
}

func usersGetByID(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	user, err := s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = s.GetByID(ctx, 404)
	wantErr := ErrUserNotExist{args: errutil.Args{"userID": int64(404)}}
	assert.Equal(t, wantErr, err)
}

func usersGetByUsername(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	user, err := s.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = s.GetByUsername(ctx, "bad_username")
	wantErr := ErrUserNotExist{args: errutil.Args{"name": "bad_username"}}
	assert.Equal(t, wantErr, err)
}

func usersGetByKeyID(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	// TODO: Use PublicKeys.Create to replace SQL hack when the method is available.
	publicKey := &PublicKey{
		OwnerID:     alice.ID,
		Name:        "test-key",
		Fingerprint: "12:f8:7e:78:61:b4:bf:e2:de:24:15:96:4e:d4:72:53",
		Content:     "test-key-content",
		CreatedUnix: s.db.NowFunc().Unix(),
		UpdatedUnix: s.db.NowFunc().Unix(),
	}
	err = s.db.WithContext(ctx).Create(publicKey).Error
	require.NoError(t, err)

	user, err := s.GetByKeyID(ctx, publicKey.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = s.GetByKeyID(ctx, publicKey.ID+1)
	wantErr := ErrUserNotExist{args: errutil.Args{"keyID": publicKey.ID + 1}}
	assert.Equal(t, wantErr, err)
}

func usersGetMailableEmailsByUsernames(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := s.Create(ctx, "bob", "bob@exmaple.com", CreateUserOptions{Activated: true})
	require.NoError(t, err)
	_, err = s.Create(ctx, "cindy", "cindy@exmaple.com", CreateUserOptions{Activated: true})
	require.NoError(t, err)

	got, err := s.GetMailableEmailsByUsernames(ctx, []string{alice.Name, bob.Name, "ignore-non-exist"})
	require.NoError(t, err)
	want := []string{bob.Email}
	assert.Equal(t, want, got)
}

func usersIsUsernameUsed(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	tests := []struct {
		name          string
		username      string
		excludeUserID int64
		want          bool
	}{
		{
			name:          "no change",
			username:      alice.Name,
			excludeUserID: alice.ID,
			want:          false,
		},
		{
			name:          "change case",
			username:      strings.ToUpper(alice.Name),
			excludeUserID: alice.ID,
			want:          false,
		},
		{
			name:          "not used",
			username:      "bob",
			excludeUserID: alice.ID,
			want:          false,
		},
		{
			name:          "not used when not excluded",
			username:      "bob",
			excludeUserID: 0,
			want:          false,
		},

		{
			name:          "used when not excluded",
			username:      alice.Name,
			excludeUserID: 0,
			want:          true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := s.IsUsernameUsed(ctx, test.username, test.excludeUserID)
			assert.Equal(t, test.want, got)
		})
	}
}

func usersList(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	// Create an organization shouldn't count
	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org1, err := s.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(
		dbutil.Quote("UPDATE %s SET type = ? WHERE id = ?", "user"),
		UserTypeOrganization, org1.ID,
	).Error
	require.NoError(t, err)

	got, err := s.List(ctx, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, alice.ID, got[0].ID)

	got, err = s.List(ctx, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bob.ID, got[0].ID)

	got, err = s.List(ctx, 1, 3)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, alice.ID, got[0].ID)
	assert.Equal(t, bob.ID, got[1].ID)
}

func usersListFollowers(t *testing.T, ctx context.Context, s *UsersStore) {
	john, err := s.Create(ctx, "john", "john@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got, err := s.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	assert.Empty(t, got)

	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.Follow(ctx, alice.ID, john.ID)
	require.NoError(t, err)
	err = s.Follow(ctx, bob.ID, john.ID)
	require.NoError(t, err)

	// First page only has bob
	got, err = s.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bob.ID, got[0].ID)

	// Second page only has alice
	got, err = s.ListFollowers(ctx, john.ID, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, alice.ID, got[0].ID)
}

func usersListFollowings(t *testing.T, ctx context.Context, s *UsersStore) {
	john, err := s.Create(ctx, "john", "john@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got, err := s.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	assert.Empty(t, got)

	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.Follow(ctx, john.ID, alice.ID)
	require.NoError(t, err)
	err = s.Follow(ctx, john.ID, bob.ID)
	require.NoError(t, err)

	// First page only has bob
	got, err = s.ListFollowings(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bob.ID, got[0].ID)

	// Second page only has alice
	got, err = s.ListFollowings(ctx, john.ID, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, alice.ID, got[0].ID)
}

func usersSearchByName(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{FullName: "Alice Jordan"})
	require.NoError(t, err)
	bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{FullName: "Bob Jordan"})
	require.NoError(t, err)

	t.Run("search for username alice", func(t *testing.T) {
		users, count, err := s.SearchByName(ctx, "Li", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, users, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, alice.ID, users[0].ID)
	})

	t.Run("search for username bob", func(t *testing.T) {
		users, count, err := s.SearchByName(ctx, "oB", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, users, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, bob.ID, users[0].ID)
	})

	t.Run("search for full name jordan", func(t *testing.T) {
		users, count, err := s.SearchByName(ctx, "Jo", 1, 10, "")
		require.NoError(t, err)
		require.Len(t, users, int(count))
		assert.Equal(t, int64(2), count)
	})

	t.Run("search for full name jordan ORDER BY id DESC LIMIT 1", func(t *testing.T) {
		users, count, err := s.SearchByName(ctx, "Jo", 1, 1, "id DESC")
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.Equal(t, int64(2), count)
		assert.Equal(t, bob.ID, users[0].ID)
	})
}

func usersUpdate(t *testing.T, ctx context.Context, s *UsersStore) {
	const oldPassword = "Password"
	alice, err := s.Create(
		ctx,
		"alice",
		"alice@example.com",
		CreateUserOptions{
			FullName:    "FullName",
			Password:    oldPassword,
			LoginSource: 9,
			LoginName:   "LoginName",
			Location:    "Location",
			Website:     "Website",
			Activated:   false,
			Admin:       false,
		},
	)
	require.NoError(t, err)

	t.Run("update password", func(t *testing.T) {
		got := userutil.ValidatePassword(alice.Password, alice.Salt, oldPassword)
		require.True(t, got)

		newPassword := "NewPassword"
		err = s.Update(ctx, alice.ID, UpdateUserOptions{Password: &newPassword})
		require.NoError(t, err)
		alice, err = s.GetByID(ctx, alice.ID)
		require.NoError(t, err)

		got = userutil.ValidatePassword(alice.Password, alice.Salt, oldPassword)
		assert.False(t, got, "Old password should stop working")

		got = userutil.ValidatePassword(alice.Password, alice.Salt, newPassword)
		assert.True(t, got, "New password should work")
	})

	t.Run("update email but already used", func(t *testing.T) {
		bob, err := s.Create(
			ctx,
			"bob",
			"bob@example.com",
			CreateUserOptions{
				Activated: true,
			},
		)
		require.NoError(t, err)

		got := s.Update(ctx, alice.ID, UpdateUserOptions{Email: &bob.Email})
		want := ErrEmailAlreadyUsed{args: errutil.Args{"email": bob.Email}}
		assert.Equal(t, want, got)
	})

	loginSource := int64(1)
	maxRepoCreation := 99
	lastRepoVisibility := true
	overLimitStr := strings.Repeat("a", 2050)
	opts := UpdateUserOptions{
		LoginSource: &loginSource,
		LoginName:   &alice.Name,

		FullName:    &overLimitStr,
		Website:     &overLimitStr,
		Location:    &overLimitStr,
		Description: &overLimitStr,

		MaxRepoCreation:    &maxRepoCreation,
		LastRepoVisibility: &lastRepoVisibility,

		IsActivated:      &lastRepoVisibility,
		IsAdmin:          &lastRepoVisibility,
		AllowGitHook:     &lastRepoVisibility,
		AllowImportLocal: &lastRepoVisibility,
		ProhibitLogin:    &lastRepoVisibility,

		Avatar:      &overLimitStr,
		AvatarEmail: &overLimitStr,
	}
	err = s.Update(ctx, alice.ID, opts)
	require.NoError(t, err)

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)

	assertValues := func() {
		assert.Equal(t, loginSource, alice.LoginSource)
		assert.Equal(t, alice.Name, alice.LoginName)
		wantStr255 := strings.Repeat("a", 255)
		assert.Equal(t, wantStr255, alice.FullName)
		assert.Equal(t, wantStr255, alice.Website)
		assert.Equal(t, wantStr255, alice.Location)
		assert.Equal(t, wantStr255, alice.Description)
		assert.Equal(t, maxRepoCreation, alice.MaxRepoCreation)
		assert.Equal(t, lastRepoVisibility, alice.LastRepoVisibility)
		assert.Equal(t, lastRepoVisibility, alice.IsActive)
		assert.Equal(t, lastRepoVisibility, alice.IsAdmin)
		assert.Equal(t, lastRepoVisibility, alice.AllowGitHook)
		assert.Equal(t, lastRepoVisibility, alice.AllowImportLocal)
		assert.Equal(t, lastRepoVisibility, alice.ProhibitLogin)
		wantStr2048 := strings.Repeat("a", 2048)
		assert.Equal(t, wantStr2048, alice.Avatar)
		assert.Equal(t, wantStr255, alice.AvatarEmail)
	}
	assertValues()

	// Test ignored values
	err = s.Update(ctx, alice.ID, UpdateUserOptions{})
	require.NoError(t, err)

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assertValues()
}

func usersUseCustomAvatar(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	avatar, err := public.Files.ReadFile("img/avatar_default.png")
	require.NoError(t, err)

	avatarPath := userutil.CustomAvatarPath(alice.ID)
	_ = os.Remove(avatarPath)
	defer func() { _ = os.Remove(avatarPath) }()

	err = s.UseCustomAvatar(ctx, alice.ID, avatar)
	require.NoError(t, err)

	// Make sure avatar is saved and the user flag is updated.
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)

	alice, err = s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, alice.UseCustomAvatar)
}

func TestIsUsernameAllowed(t *testing.T) {
	for name := range reservedUsernames {
		t.Run(name, func(t *testing.T) {
			assert.True(t, IsErrNameNotAllowed(isUsernameAllowed(name)))
		})
	}

	for _, pattern := range reservedUsernamePatterns {
		t.Run(pattern, func(t *testing.T) {
			username := strings.ReplaceAll(pattern, "*", "alice")
			assert.True(t, IsErrNameNotAllowed(isUsernameAllowed(username)))
		})
	}
}

func usersAddEmail(t *testing.T, ctx context.Context, s *UsersStore) {
	t.Run("multiple users can add the same unverified email", func(t *testing.T) {
		alice, err := s.Create(ctx, "alice", "unverified@example.com", CreateUserOptions{})
		require.NoError(t, err)
		err = s.AddEmail(ctx, alice.ID+1, "unverified@example.com", false)
		require.NoError(t, err)
	})

	t.Run("only one user can add the same verified email", func(t *testing.T) {
		bob, err := s.Create(ctx, "bob", "verified@example.com", CreateUserOptions{Activated: true})
		require.NoError(t, err)
		got := s.AddEmail(ctx, bob.ID+1, "verified@example.com", true)
		want := ErrEmailAlreadyUsed{args: errutil.Args{"email": "verified@example.com"}}
		require.Equal(t, want, got)
	})
}

func usersGetEmail(t *testing.T, ctx context.Context, s *UsersStore) {
	const testUserID = 1
	const testEmail = "alice@example.com"
	_, err := s.GetEmail(ctx, testUserID, testEmail, false)
	wantErr := ErrEmailNotExist{
		args: errutil.Args{
			"email": testEmail,
		},
	}
	assert.Equal(t, wantErr, err)

	err = s.AddEmail(ctx, testUserID, testEmail, false)
	require.NoError(t, err)
	got, err := s.GetEmail(ctx, testUserID, testEmail, false)
	require.NoError(t, err)
	assert.Equal(t, testEmail, got.Email)

	// Should not return if we ask for a different user
	_, err = s.GetEmail(ctx, testUserID+1, testEmail, false)
	assert.Equal(t, wantErr, err)

	// Should not return if we only want activated emails
	_, err = s.GetEmail(ctx, testUserID, testEmail, true)
	assert.Equal(t, wantErr, err)

	err = s.MarkEmailActivated(ctx, testUserID, testEmail)
	require.NoError(t, err)
	got, err = s.GetEmail(ctx, testUserID, testEmail, true)
	require.NoError(t, err)
	assert.Equal(t, testEmail, got.Email)
}

func usersListEmails(t *testing.T, ctx context.Context, s *UsersStore) {
	t.Run("list emails with primary email", func(t *testing.T) {
		alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
		require.NoError(t, err)
		err = s.AddEmail(ctx, alice.ID, "alice2@example.com", true)
		require.NoError(t, err)
		err = s.MarkEmailPrimary(ctx, alice.ID, "alice2@example.com")
		require.NoError(t, err)

		emails, err := s.ListEmails(ctx, alice.ID)
		require.NoError(t, err)
		got := make([]string, 0, len(emails))
		for _, email := range emails {
			got = append(got, email.Email)
		}
		want := []string{"alice2@example.com", "alice@example.com"}
		assert.Equal(t, want, got)
	})

	t.Run("list emails without primary email", func(t *testing.T) {
		bob, err := s.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
		require.NoError(t, err)
		err = s.AddEmail(ctx, bob.ID, "bob2@example.com", false)
		require.NoError(t, err)

		emails, err := s.ListEmails(ctx, bob.ID)
		require.NoError(t, err)
		got := make([]string, 0, len(emails))
		for _, email := range emails {
			got = append(got, email.Email)
		}
		want := []string{"bob2@example.com", "bob@example.com"}
		assert.Equal(t, want, got)
	})
}

func usersMarkEmailActivated(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.AddEmail(ctx, alice.ID, "alice2@example.com", false)
	require.NoError(t, err)
	err = s.MarkEmailActivated(ctx, alice.ID, "alice2@example.com")
	require.NoError(t, err)

	gotEmail, err := s.GetEmail(ctx, alice.ID, "alice2@example.com", true)
	require.NoError(t, err)
	assert.True(t, gotEmail.IsActivated)

	gotAlice, err := s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.NotEqual(t, alice.Rands, gotAlice.Rands)
}

func usersMarkEmailPrimary(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.AddEmail(ctx, alice.ID, "alice2@example.com", false)
	require.NoError(t, err)

	// Should fail because email not verified
	gotError := s.MarkEmailPrimary(ctx, alice.ID, "alice2@example.com")
	wantError := ErrEmailNotVerified{args: errutil.Args{"email": "alice2@example.com"}}
	assert.Equal(t, wantError, gotError)

	// Mark email as verified and should succeed
	err = s.MarkEmailActivated(ctx, alice.ID, "alice2@example.com")
	require.NoError(t, err)
	err = s.MarkEmailPrimary(ctx, alice.ID, "alice2@example.com")
	require.NoError(t, err)
	gotAlice, err := s.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice2@example.com", gotAlice.Email)

	// Former primary email should be preserved
	gotEmail, err := s.GetEmail(ctx, alice.ID, "alice@example.com", false)
	require.NoError(t, err)
	assert.False(t, gotEmail.IsActivated)
}

func usersDeleteEmail(t *testing.T, ctx context.Context, s *UsersStore) {
	alice, err := s.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.AddEmail(ctx, alice.ID, "alice2@example.com", false)
	require.NoError(t, err)
	_, err = s.GetEmail(ctx, alice.ID, "alice2@example.com", false)
	require.NoError(t, err)

	err = s.DeleteEmail(ctx, alice.ID, "alice2@example.com")
	require.NoError(t, err)
	_, got := s.GetEmail(ctx, alice.ID, "alice2@example.com", false)
	want := ErrEmailNotExist{args: errutil.Args{"email": "alice2@example.com"}}
	require.Equal(t, want, got)
}

func usersFollow(t *testing.T, ctx context.Context, s *UsersStore) {
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	// It is OK to follow multiple times and just be noop.
	err = s.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	alice, err = usersStore.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, alice.NumFollowing)

	bob, err = usersStore.GetByID(ctx, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, bob.NumFollowers)
}

func usersIsFollowing(t *testing.T, ctx context.Context, s *UsersStore) {
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got := s.IsFollowing(ctx, alice.ID, bob.ID)
	assert.False(t, got)

	err = s.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	got = s.IsFollowing(ctx, alice.ID, bob.ID)
	assert.True(t, got)

	err = s.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	got = s.IsFollowing(ctx, alice.ID, bob.ID)
	assert.False(t, got)
}

func usersUnfollow(t *testing.T, ctx context.Context, s *UsersStore) {
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = s.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	// It is OK to unfollow multiple times and just be noop.
	err = s.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	err = s.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	alice, err = usersStore.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, alice.NumFollowing)

	bob, err = usersStore.GetByID(ctx, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, bob.NumFollowers)
}
