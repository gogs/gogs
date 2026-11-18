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

func TestAddPublicKeyOwnerIDFingerprintUniqueIndex(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "addPublicKeyOwnerIDFingerprintUniqueIndex")
	require.NoError(t, Migrate(logx.NewNoopLogger(), db))
	require.NoError(t, AllowDuplicatePublicKeys(db))

	newKey := func() (string, string) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		key, err := ssh.NewPublicKey(pub)
		require.NoError(t, err)
		return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), ssh.FingerprintSHA256(key)
	}
	shared, sharedFingerprint := newKey()
	other, otherFingerprint := newKey()
	const keyTypeUser = 1
	addPublicKey := func(id, ownerID int64, keyType int, content, fingerprint string) error {
		return db.Table("public_key").Create(map[string]any{
			"id": id, "owner_id": ownerID, "name": fmt.Sprintf("key-%d", id), "fingerprint": fingerprint, "content": content, "mode": 1, "type": keyType,
		}).Error
	}
	// Deploy keys: the same key with another comment and the MD5 fingerprint of
	// an old "ssh-keygen", and another key with an MD5 fingerprint.
	require.NoError(t, addPublicKey(1, 0, keyTypeDeploy, shared+" laptop", sharedFingerprint))
	require.NoError(t, addPublicKey(2, 0, keyTypeDeploy, shared+" desktop", "3b:0c:aa:11"))
	require.NoError(t, addPublicKey(3, 0, keyTypeDeploy, other, "9f:21:bb:22"))
	// User keys: user 7 has the key twice, user 8 has it once and keeps it.
	require.NoError(t, addPublicKey(4, 7, keyTypeUser, other+" alice@laptop", otherFingerprint))
	require.NoError(t, addPublicKey(5, 7, keyTypeUser, other+" alice@desktop", otherFingerprint))
	require.NoError(t, addPublicKey(6, 8, keyTypeUser, other, otherFingerprint))
	addDeployKey := func(id, keyID, repoID int64, fingerprint string) {
		err := db.Table("deploy_key").Create(map[string]any{
			"id": id, "key_id": keyID, "repo_id": repoID, "name": fmt.Sprintf("key-%d", id), "fingerprint": fingerprint,
		}).Error
		require.NoError(t, err)
	}
	for repoID := int64(1); repoID <= 3; repoID++ {
		require.NoError(t, db.Exec("INSERT INTO repository (id, owner_id, lower_name, name) VALUES (?, 1, ?, ?)",
			repoID, fmt.Sprintf("repo-%d", repoID), fmt.Sprintf("repo-%d", repoID)).Error)
	}
	addDeployKey(1, 1, 1, sharedFingerprint)
	// Repository 1 has the key twice, repository 2 only through the duplicate.
	addDeployKey(2, 2, 1, "3b:0c:aa:11")
	addDeployKey(3, 2, 2, "3b:0c:aa:11")
	addDeployKey(4, 3, 3, "9f:21:bb:22")
	// Left behind by deleting repository 9: a key only it used, and the shared
	// key, which repository 1 keeps. Key 14 has no deploy key at all.
	orphan, orphanFingerprint := newKey()
	require.NoError(t, addPublicKey(13, 0, keyTypeDeploy, orphan, orphanFingerprint))
	require.NoError(t, addPublicKey(14, 0, keyTypeDeploy, orphan+" again", "aa:bb"))
	addDeployKey(5, 13, 9, orphanFingerprint)
	addDeployKey(6, 1, 9, sharedFingerprint)

	rootPath := t.TempDir()
	conf.SetMockSSH(t, conf.SSHOpts{RootPath: rootPath})
	authorizedKeys := filepath.Join(rootPath, "authorized_keys")
	line := func(id int64, content string) string {
		return fmt.Sprintf("command=\"gogs serv key-%d --config=''\",no-pty %s\n", id, content)
	}
	require.NoError(t, os.WriteFile(authorizedKeys,
		[]byte(line(2, shared+" desktop")+line(1, shared+" laptop")+line(3, other)+line(5, other+" alice@desktop")+line(4, other+" alice@laptop")+line(6, other)+line(52, other)+line(13, orphan)+line(14, orphan+" again")+"ssh-ed25519 AAAA admin\n"),
		0o600))

	require.NoError(t, addPublicKeyOwnerIDFingerprintUniqueIndex(db))

	type publicKey struct {
		ID          int64
		OwnerID     int64
		Fingerprint string
	}
	var pkeys []publicKey
	require.NoError(t, db.Table("public_key").Order("id").Find(&pkeys).Error)
	assert.Equal(t, []publicKey{
		{1, 0, sharedFingerprint},
		{3, 0, otherFingerprint},
		{4, 7, otherFingerprint},
		{6, 8, otherFingerprint},
	}, pkeys)

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

	// Only the lines of the merged and orphan keys are gone, not the one of key 52, whose
	// ID only starts with that of key 5, or the line Gogs did not write.
	data, err := os.ReadFile(authorizedKeys)
	require.NoError(t, err)
	assert.Equal(t, line(1, shared+" laptop")+line(3, other)+line(4, other+" alice@laptop")+line(6, other)+line(52, other)+"ssh-ed25519 AAAA admin\n", string(data))

	// Each owner has a key once, while different owners may have the same key.
	assert.Error(t, addPublicKey(10, 0, keyTypeDeploy, shared, sharedFingerprint))
	assert.Error(t, addPublicKey(11, 7, keyTypeUser, other+" again", otherFingerprint))
	require.NoError(t, addPublicKey(12, 9, keyTypeUser, other, otherFingerprint))

	assert.Equal(t, errMigrationSkipped, addPublicKeyOwnerIDFingerprintUniqueIndex(db))
}
