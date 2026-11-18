package database

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
)

func TestPublicKeys(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &PublicKeysStore{
		db: newTestDB(t, "PublicKeysStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *PublicKeysStore)
	}{
		{"Add", publicKeysAdd},
		{"GetByID", publicKeysGetByID},
		{"GetByContent", publicKeysGetByContent},
		{"List", publicKeysList},
		{"Touch", publicKeysTouch},
		{"Delete", publicKeysDelete},
		{"RewriteAuthorizedKeys", publicKeysRewriteAuthorizedKeys},
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

// newTestSSHKey returns the content of a new SSH public key, without a comment.
func newTestSSHKey(t *testing.T) string {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

// setMockSSHForKeys sets up the SSH options that adding keys needs, and returns
// the path of the "authorized_keys" file.
func setMockSSHForKeys(t *testing.T) string {
	t.Helper()

	rootPath := t.TempDir()
	conf.SetMockSSH(t, conf.SSHOpts{RootPath: rootPath, KeygenPath: "ssh-keygen"})
	return authorizedKeysPath()
}

func readAuthorizedKeys(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

func publicKeysAdd(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	authorizedKeys := setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	content := newTestSSHKey(t)
	key, err := s.Add(ctx, logger, 1, "laptop", content)
	require.NoError(t, err)
	assert.NotZero(t, key.ID)
	assert.Equal(t, KeyTypeUser, int(key.Type))
	assert.Equal(t, AccessModeWrite, key.Mode)
	assert.True(t, strings.HasPrefix(key.Fingerprint, "SHA256:"), "fingerprint %q", key.Fingerprint)
	assert.Contains(t, readAuthorizedKeys(t, authorizedKeys), fmt.Sprintf("key-%d", key.ID))

	// The same key cannot be added again, even by another user or with another
	// comment.
	_, err = s.Add(ctx, logger, 2, "desktop", content)
	assert.True(t, IsErrKeyAlreadyExist(err), "got %v", err)
	_, err = s.Add(ctx, logger, 1, "desktop", content+" alice@example.com")
	assert.True(t, IsErrKeyAlreadyExist(err), "got %v", err)

	// Names are unique for each user only.
	_, err = s.Add(ctx, logger, 1, "laptop", newTestSSHKey(t))
	assert.True(t, IsErrKeyNameAlreadyUsed(err), "got %v", err)
	_, err = s.Add(ctx, logger, 2, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
}

func publicKeysGetByID(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	setMockSSHForKeys(t)

	_, err := s.GetByID(ctx, 404)
	assert.True(t, IsErrKeyNotExist(err), "got %v", err)

	key, err := s.Add(ctx, logx.NewNoopLogger(), 1, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
	got, err := s.GetByID(ctx, key.ID)
	require.NoError(t, err)
	assert.Equal(t, key.Content, got.Content)
	assert.Equal(t, key.Fingerprint, got.Fingerprint)
}

func publicKeysGetByContent(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	setMockSSHForKeys(t)

	content := newTestSSHKey(t)
	key, err := s.Add(ctx, logx.NewNoopLogger(), 1, "laptop", content+" alice@example.com")
	require.NoError(t, err)

	// The comment of the stored key is ignored.
	got, err := s.GetByContent(ctx, content)
	require.NoError(t, err)
	assert.Equal(t, key.ID, got.ID)

	_, err = s.GetByContent(ctx, newTestSSHKey(t))
	assert.True(t, IsErrKeyNotExist(err), "got %v", err)
}

func publicKeysList(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	key1, err := s.Add(ctx, logger, 1, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
	key2, err := s.Add(ctx, logger, 1, "desktop", newTestSSHKey(t))
	require.NoError(t, err)
	_, err = s.Add(ctx, logger, 2, "laptop", newTestSSHKey(t))
	require.NoError(t, err)

	keys, err := s.List(ctx, 1)
	require.NoError(t, err)
	var ids []int64
	for _, key := range keys {
		ids = append(ids, key.ID)
	}
	assert.ElementsMatch(t, []int64{key1.ID, key2.ID}, ids)

	keys, err = s.List(ctx, 3)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

func publicKeysTouch(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	key, err := s.Add(ctx, logger, 1, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
	other, err := s.Add(ctx, logger, 1, "desktop", newTestSSHKey(t))
	require.NoError(t, err)
	err = s.db.Model(new(PublicKey)).Where("id IN ?", []int64{key.ID, other.ID}).
		Updates(map[string]any{"created_unix": 1, "updated_unix": 1}).Error
	require.NoError(t, err)

	err = s.Touch(ctx, key.ID)
	require.NoError(t, err)

	got, err := s.GetByID(ctx, key.ID)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Unix(), got.UpdatedUnix)
	assert.True(t, got.HasUsed)
	assert.True(t, got.HasRecentActivity)

	got, err = s.GetByID(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got.UpdatedUnix)
	assert.False(t, got.HasUsed)
}

func publicKeysDelete(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	authorizedKeys := setMockSSHForKeys(t)
	logger := logx.NewNoopLogger()

	key, err := s.Add(ctx, logger, 1, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
	other, err := s.Add(ctx, logger, 1, "desktop", newTestSSHKey(t))
	require.NoError(t, err)

	// Other users cannot delete the key.
	err = s.Delete(ctx, logger, &User{ID: 2}, key.ID)
	assert.True(t, IsErrKeyAccessDenied(err), "got %v", err)
	_, err = s.GetByID(ctx, key.ID)
	require.NoError(t, err)

	err = s.Delete(ctx, logger, &User{ID: 1}, key.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, key.ID)
	assert.True(t, IsErrKeyNotExist(err), "got %v", err)
	got := readAuthorizedKeys(t, authorizedKeys)
	assert.NotContains(t, got, fmt.Sprintf("key-%d ", key.ID))
	assert.NotContains(t, got, key.Content)
	assert.Contains(t, got, other.Content)

	// Site admins can delete keys of others.
	err = s.Delete(ctx, logger, &User{ID: 2, IsAdmin: true}, other.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, other.ID)
	assert.True(t, errx.IsNotFound(err), "got %v", err)

	// Deleting a key that does not exist is not an error.
	err = s.Delete(ctx, logger, &User{ID: 1}, key.ID)
	require.NoError(t, err)
}

func publicKeysRewriteAuthorizedKeys(t *testing.T, ctx context.Context, s *PublicKeysStore) {
	authorizedKeys := setMockSSHForKeys(t)

	key, err := s.Add(ctx, logx.NewNoopLogger(), 1, "laptop", newTestSSHKey(t))
	require.NoError(t, err)
	err = os.WriteFile(authorizedKeys, []byte("ssh-ed25519 AAAA stale\n"), 0o600)
	require.NoError(t, err)

	err = s.RewriteAuthorizedKeys()
	require.NoError(t, err)

	got := readAuthorizedKeys(t, authorizedKeys)
	assert.Equal(t, key.AuthorizedString(), got)

	// The builtin SSH server does not read the file, which is left alone.
	conf.SSH.StartBuiltinServer = true
	err = os.WriteFile(authorizedKeys, []byte("ssh-ed25519 AAAA admin\n"), 0o600)
	require.NoError(t, err)
	err = s.RewriteAuthorizedKeys()
	require.NoError(t, err)
	assert.Equal(t, "ssh-ed25519 AAAA admin\n", readAuthorizedKeys(t, authorizedKeys))
}
