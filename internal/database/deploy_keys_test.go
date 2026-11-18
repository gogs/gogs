package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

func TestDeployKeys(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &DeployKeysStore{
		db: newTestDB(t, "DeployKeysStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *DeployKeysStore)
	}{
		{"Add", deployKeysAdd},
		{"Get", deployKeysGet},
		{"List", deployKeysList},
		{"Touch", deployKeysTouch},
		{"Delete", deployKeysDelete},
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

func deployKeysAdd(t *testing.T, ctx context.Context, s *DeployKeysStore) {
	authorizedKeys := setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	content := newTestSSHKey(t)
	key, err := s.Add(ctx, logger, 1, "deploy", content)
	require.NoError(t, err)
	assert.Equal(t, content, key.Content)
	assert.NotEmpty(t, key.Fingerprint)

	pkey, err := newPublicKeysStore(s.db).GetByID(ctx, key.KeyID)
	require.NoError(t, err)
	assert.Equal(t, KeyTypeDeploy, int(pkey.Type))
	assert.Equal(t, AccessModeRead, pkey.Mode)
	assert.Equal(t, key.Fingerprint, pkey.Fingerprint)
	assert.Contains(t, readAuthorizedKeys(t, authorizedKeys), fmt.Sprintf("key-%d", pkey.ID))

	// Repositories that use the same content share its public key.
	key2, err := s.Add(ctx, logger, 2, "deploy", content)
	require.NoError(t, err)
	assert.Equal(t, key.KeyID, key2.KeyID)
	assert.NotEqual(t, key.ID, key2.ID)
	// So do they when the comments of the contents differ.
	key3, err := s.Add(ctx, logger, 3, "deploy", content+" ci@example.com")
	require.NoError(t, err)
	assert.Equal(t, key.KeyID, key3.KeyID)

	_, err = s.Add(ctx, logger, 1, "again", content)
	assert.True(t, IsErrDeployKeyAlreadyExist(err), "got %v", err)
	// A key rolled back leaves no line, whose key ID a later key may get.
	rolledBack := newTestSSHKey(t)
	_, err = s.Add(ctx, logger, 1, "deploy", rolledBack)
	assert.True(t, IsErrDeployKeyNameAlreadyUsed(err), "got %v", err)
	assert.NotContains(t, readAuthorizedKeys(t, authorizedKeys), rolledBack)

	// Content of a user key cannot be a deploy key.
	userContent := newTestSSHKey(t)
	_, err = newPublicKeysStore(s.db).Add(ctx, logger, 1, "laptop", userContent)
	require.NoError(t, err)
	_, err = s.Add(ctx, logger, 1, "user", userContent)
	assert.True(t, IsErrKeyAlreadyExist(err), "got %v", err)
}

func deployKeysGet(t *testing.T, ctx context.Context, s *DeployKeysStore) {
	setMockSSHForKeys(t)

	_, err := s.GetByID(ctx, 404)
	assert.True(t, IsErrDeployKeyNotExist(err), "got %v", err)

	content := newTestSSHKey(t)
	key, err := s.Add(ctx, logx.NewNoopLogger(), 1, "deploy", content)
	require.NoError(t, err)

	got, err := s.GetByID(ctx, key.ID)
	require.NoError(t, err)
	assert.Equal(t, content, got.Content)

	got, err = s.GetByRepoAndKey(ctx, 1, key.KeyID)
	require.NoError(t, err)
	assert.Equal(t, key.ID, got.ID)
	assert.Equal(t, content, got.Content)

	_, err = s.GetByRepoAndKey(ctx, 2, key.KeyID)
	assert.True(t, IsErrDeployKeyNotExist(err), "got %v", err)
}

func deployKeysList(t *testing.T, ctx context.Context, s *DeployKeysStore) {
	setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	content1, content2 := newTestSSHKey(t), newTestSSHKey(t)
	_, err := s.Add(ctx, logger, 1, "first", content1)
	require.NoError(t, err)
	_, err = s.Add(ctx, logger, 1, "second", content2)
	require.NoError(t, err)
	_, err = s.Add(ctx, logger, 2, "other", newTestSSHKey(t))
	require.NoError(t, err)

	keys, err := s.List(ctx, 1)
	require.NoError(t, err)
	got := make(map[string]string, len(keys))
	for _, key := range keys {
		got[key.Name] = key.Content
	}
	assert.Equal(t, map[string]string{"first": content1, "second": content2}, got)

	// A deploy key whose public key is gone is still listed.
	err = s.db.Where("content = ?", content2).Delete(new(PublicKey)).Error
	require.NoError(t, err)
	keys, err = s.List(ctx, 1)
	require.NoError(t, err)
	got = make(map[string]string, len(keys))
	for _, key := range keys {
		got[key.Name] = key.Content
	}
	assert.Equal(t, map[string]string{"first": content1, "second": ""}, got)
}

func deployKeysTouch(t *testing.T, ctx context.Context, s *DeployKeysStore) {
	setMockSSHForKeys(t)

	key, err := s.Add(ctx, logx.NewNoopLogger(), 1, "deploy", newTestSSHKey(t))
	require.NoError(t, err)
	err = s.db.Model(new(DeployKey)).Where("id = ?", key.ID).
		Updates(map[string]any{"created_unix": 1, "updated_unix": 1}).Error
	require.NoError(t, err)

	err = s.Touch(ctx, key.ID)
	require.NoError(t, err)

	got, err := s.GetByID(ctx, key.ID)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Unix(), got.UpdatedUnix)
	assert.True(t, got.HasUsed)
}

func deployKeysDelete(t *testing.T, ctx context.Context, s *DeployKeysStore) {
	setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	reposStore := newReposStore(s.db)
	repo1, err := reposStore.Create(ctx, alice.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo2, err := reposStore.Create(ctx, alice.ID, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)

	content := newTestSSHKey(t)
	key1, err := s.Add(ctx, logger, repo1.ID, "deploy", content)
	require.NoError(t, err)
	key2, err := s.Add(ctx, logger, repo2.ID, "deploy", content)
	require.NoError(t, err)

	// Users who are not admins of the repository cannot delete its keys.
	err = s.Delete(ctx, logger, bob, key1.ID)
	assert.True(t, IsErrKeyAccessDenied(err), "got %v", err)

	// The public key stays while another repository uses it.
	err = s.Delete(ctx, logger, alice, key1.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, key1.ID)
	assert.True(t, IsErrDeployKeyNotExist(err), "got %v", err)
	_, err = newPublicKeysStore(s.db).GetByID(ctx, key1.KeyID)
	require.NoError(t, err)

	// Site admins can delete keys of any repository, and the last one takes its
	// public key with it.
	err = s.Delete(ctx, logger, &User{ID: bob.ID, IsAdmin: true}, key2.ID)
	require.NoError(t, err)
	_, err = newPublicKeysStore(s.db).GetByID(ctx, key2.KeyID)
	assert.True(t, IsErrKeyNotExist(err), "got %v", err)

	// Deleting a key that does not exist is not an error.
	err = s.Delete(ctx, logger, alice, key1.ID)
	require.NoError(t, err)
}
