package migrations

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbtest"
)

func TestAddPublicKeyDeployFingerprintUniqueIndex(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addPublicKeyDeployFingerprintUniqueIndex")
	require.NoError(t, Migrate(logx.NewNoopLogger(), db))
	require.NoError(t, DropDeployFingerprintIndex(db))

	newKey := func() (string, string) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		key, err := ssh.NewPublicKey(pub)
		require.NoError(t, err)
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key)
	}
	shared, sharedFingerprint := newKey()
	other, otherFingerprint := newKey()
	addPublicKey := func(id int64, keyType int, content, fingerprint string) {
		err := db.Table("public_key").Create(map[string]any{
			"id": id, "owner_id": 0, "name": "", "fingerprint": fingerprint, "content": content, "mode": 1, "type": keyType,
		}).Error
		require.NoError(t, err)
	}
	// The same key with another comment and the MD5 fingerprint of an old
	// "ssh-keygen", and the same key as a user key, which is left alone.
	addPublicKey(1, keyTypeDeploy, shared+" laptop", sharedFingerprint)
	addPublicKey(2, keyTypeDeploy, shared+" desktop", "3b:0c:aa:11")
	addPublicKey(3, keyTypeDeploy, other, "9f:21:bb:22")
	addPublicKey(4, 1, shared, sharedFingerprint)
	addDeployKey := func(id, keyID, repoID int64, fingerprint string) {
		err := db.Table("deploy_key").Create(map[string]any{
			"id": id, "key_id": keyID, "repo_id": repoID, "name": fmt.Sprintf("key-%d", id), "fingerprint": fingerprint,
		}).Error
		require.NoError(t, err)
	}
	addDeployKey(1, 1, 1, sharedFingerprint)
	// Repository 1 has the key twice, repository 2 only through the duplicate.
	addDeployKey(2, 2, 1, "3b:0c:aa:11")
	addDeployKey(3, 2, 2, "3b:0c:aa:11")
	addDeployKey(4, 3, 3, "9f:21:bb:22")

	rootPath := t.TempDir()
	conf.SetMockSSH(t, conf.SSHOpts{RootPath: rootPath})
	authorizedKeys := filepath.Join(rootPath, "authorized_keys")
	line := func(id int64, content string) string {
		return fmt.Sprintf("command=\"gogs serv key-%d --config=''\",no-pty %s\n", id, content)
	}
	require.NoError(t, os.WriteFile(authorizedKeys,
		[]byte(line(2, shared+" desktop")+line(1, shared+" laptop")+line(3, other)+line(4, shared)+line(12, other)+"ssh-ed25519 AAAA admin\n"),
		0o600))

	require.NoError(t, addPublicKeyDeployFingerprintUniqueIndex(db))

	type publicKey struct {
		ID          int64
		Fingerprint string
	}
	var pkeys []publicKey
	require.NoError(t, db.Table("public_key").Order("id").Find(&pkeys).Error)
	assert.Equal(t, []publicKey{{1, sharedFingerprint}, {3, otherFingerprint}, {4, sharedFingerprint}}, pkeys)

	type deployKey struct {
		ID          int64
		KeyID       int64
		RepoID      int64
		Fingerprint string
	}
	var keys []deployKey
	require.NoError(t, db.Table("deploy_key").Order("id").Find(&keys).Error)
	assert.Equal(t, []deployKey{
		{1, 1, 1, sharedFingerprint},
		{3, 1, 2, sharedFingerprint},
		{4, 3, 3, otherFingerprint},
	}, keys)

	// Only the line of the merged key is gone, not the one of key 12 or the
	// line Gogs did not write.
	data, err := os.ReadFile(authorizedKeys)
	require.NoError(t, err)
	assert.Equal(t, line(1, shared+" laptop")+line(3, other)+line(4, shared)+line(12, other)+"ssh-ed25519 AAAA admin\n", string(data))

	// Only deploy keys are unique by their fingerprints.
	err = db.Table("public_key").Create(map[string]any{
		"id": 10, "owner_id": 0, "name": "", "fingerprint": sharedFingerprint, "content": shared, "mode": 1, "type": keyTypeDeploy,
	}).Error
	assert.Error(t, err)
	addPublicKey(11, 1, shared+" again", sharedFingerprint)

	assert.Equal(t, errMigrationSkipped, addPublicKeyDeployFingerprintUniqueIndex(db))
}
