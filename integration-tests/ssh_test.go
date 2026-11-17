package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

// sshKey is the private key file of an SSH key whose public key is registered
// with the server.
type sshKey struct {
	path string
}

// sshNewKey writes a new private key to a file and returns it with its public
// key in the authorized_keys format.
func sshNewKey(t *testing.T) (*sshKey, string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	block, err := ssh.MarshalPrivateKey(priv, "")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0o600))
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return &sshKey{path: path}, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

// sshUserKey returns a new key of the user.
func sshUserKey(t *testing.T, userID int64) *sshKey {
	t.Helper()

	key, content := sshNewKey(t)
	_, err := database.AddPublicKey(logger, userID, UniqueName("key-"), content)
	require.NoError(t, err)
	return key
}

// sshDeployKey returns a new read-only deploy key of the repository.
func sshDeployKey(t *testing.T, repo *Repository) *sshKey {
	t.Helper()

	key, content := sshNewKey(t)
	_, err := database.AddDeployKey(logger, repo.ID, UniqueName("deploy-"), content)
	require.NoError(t, err)
	return key
}

// sshRemoteURL returns the SSH URL of the repository on the builtin SSH server,
// in the form Gogs shows for cloning on a port other than 22.
func sshRemoteURL(repo *Repository) string {
	return fmt.Sprintf("ssh://git@127.0.0.1:%d/%s/%s.git", sshPort, repo.Owner, repo.Name)
}

// command returns a Git command that runs in the directory and connects over
// SSH with the key only.
func (k *sshKey) command(dir string, args ...string) *exec.Cmd {
	cmd := gitCommand(dir, args...)
	cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -i "+k.path+
		" -o IdentitiesOnly=yes -o IdentityAgent=none -o BatchMode=yes"+
		" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR")
	return cmd
}

// git returns the trimmed output of Git run in the directory over SSH, and
// whether it succeeded.
func (k *sshKey) git(dir string, args ...string) (string, error) {
	out, err := k.command(dir, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// clone clones the repository over SSH into a new directory and returns the
// directory.
func (k *sshKey) clone(t *testing.T, repo *Repository) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clone")
	out, err := k.git(filepath.Dir(dir), "clone", sshRemoteURL(repo), dir)
	require.NoError(t, err, "clone: %s", out)
	return dir
}

// cloneFails clones the repository over SSH, fails the test when the clone
// succeeds, and returns the output.
func (k *sshKey) cloneFails(t *testing.T, repo *Repository) string {
	t.Helper()

	out, err := k.git(t.TempDir(), "clone", sshRemoteURL(repo), "clone")
	require.Error(t, err, "clone succeeded: %s", out)
	return out
}

func TestSSHCloneAndPush(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerKey := sshUserKey(t, owner.ID)
	otherKey := sshUserKey(t, other.ID)

	dir := ownerKey.clone(t, repo)
	assert.Equal(t, gitMainOnDisk(t, repo), gitRun(t, dir, "rev-parse", "HEAD"))
	fileName := UniqueName("file-") + ".txt"
	want := gitCommitFile(t, dir, fileName)
	out, err := ownerKey.git(dir, "push", "origin", "main")
	require.NoError(t, err, "push: %s", out)
	assert.Equal(t, want, gitMainOnDisk(t, repo))

	// The server records the push in the news feed of the owner, as it does for
	// a push over HTTP.
	actions, err := database.Handle.Actions().ListByUser(t.Context(), owner.ID, owner.ID, 0, false)
	require.NoError(t, err)
	var pushed *database.Action
	for _, a := range actions {
		if a.OpType == database.ActionCommitRepo && a.RepoID == repo.ID {
			pushed = a
			break
		}
	}
	require.NotNil(t, pushed, "no push action recorded")
	assert.Equal(t, owner.ID, pushed.ActUserID)
	assert.Contains(t, pushed.Content, want)

	t.Run("protocol version 2", func(t *testing.T) {
		cmdOut, err := ownerKey.git(t.TempDir(), "-c", "protocol.version=2", "ls-remote", sshRemoteURL(repo), "refs/heads/main")
		require.NoError(t, err, "ls-remote: %s", cmdOut)
		assert.Equal(t, want+"\trefs/heads/main", cmdOut)

		// The server answers in the version the client asks for through the
		// GIT_PROTOCOL environment variable.
		cmd := ownerKey.command(t.TempDir(), "-c", "protocol.version=2", "ls-remote", sshRemoteURL(repo))
		cmd.Env = append(cmd.Env, "GIT_TRACE_PACKET=1")
		trace, err := cmd.CombinedOutput()
		require.NoError(t, err, "ls-remote: %s", trace)
		assert.Contains(t, string(trace), "< version 2")
	})

	t.Run("user without write access", func(t *testing.T) {
		dir := otherKey.clone(t, repo)
		gitCommitFile(t, dir, UniqueName("file-")+".txt")
		out, err := otherKey.git(dir, "push", "origin", "main")
		require.Error(t, err, "push succeeded: %s", out)
		assert.Contains(t, out, "You do not have sufficient authorization for this action")
		assert.Equal(t, want, gitMainOnDisk(t, repo))
	})

	t.Run("unregistered key", func(t *testing.T) {
		key, _ := sshNewKey(t)
		out := key.cloneFails(t, repo)
		assert.Contains(t, out, "Permission denied (publickey)")
	})
}

func TestSSHClonePrivate(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	head := gitMainOnDisk(t, repo)

	dir := sshUserKey(t, owner.ID).clone(t, repo)
	assert.Equal(t, head, gitRun(t, dir, "rev-parse", "HEAD"))

	t.Run("user without access", func(t *testing.T) {
		out := sshUserKey(t, other.ID).cloneFails(t, repo)
		assert.Contains(t, out, "Repository does not exist or you do not have access")
	})

	t.Run("missing repository", func(t *testing.T) {
		out := sshUserKey(t, other.ID).cloneFails(t, &Repository{Owner: owner.Name, Name: UniqueName("missing-")})
		assert.Contains(t, out, "Repository does not exist or you do not have access")
	})
}

func TestSSHDeployKey(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	head := gitMainOnDisk(t, repo)
	key := sshDeployKey(t, repo)

	dir := key.clone(t, repo)
	assert.Equal(t, head, gitRun(t, dir, "rev-parse", "HEAD"))

	t.Run("push", func(t *testing.T) {
		gitCommitFile(t, dir, UniqueName("file-")+".txt")
		out, err := key.git(dir, "push", "origin", "main")
		require.Error(t, err, "push succeeded: %s", out)
		assert.Contains(t, out, "Key permission denied")
		assert.Equal(t, head, gitMainOnDisk(t, repo))
	})

	t.Run("other repository", func(t *testing.T) {
		otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
		out := key.cloneFails(t, otherRepo)
		assert.Contains(t, out, "Key access denied")
	})
}

// servCommand returns a Git command that runs in the directory and connects
// through "gogs serv" with the key of the ID, like OpenSSH does with the
// command that Gogs writes to the authorized_keys file.
func servCommand(t *testing.T, keyID int64, dir string, args ...string) *exec.Cmd {
	t.Helper()

	sshProgram := filepath.Join(t.TempDir(), "ssh")
	script := fmt.Sprintf("#!/bin/sh\n# The command for the server is the last argument.\nfor command; do :; done\nSSH_ORIGINAL_COMMAND=\"$command\" exec '%s' serv key-%d --config='%s'\n",
		os.Args[0], keyID, conf.CustomConf)
	require.NoError(t, os.WriteFile(sshProgram, []byte(script), 0o700))
	cmd := gitCommand(dir, args...)
	cmd.Env = append(cmd.Env, "GIT_SSH="+sshProgram)
	return cmd
}

func TestSSHServ(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	remoteURL := fmt.Sprintf("ssh://git@localhost/%s/%s.git", repo.Owner, repo.Name)
	addKey := func(userID int64) int64 {
		_, content := sshNewKey(t)
		key, err := database.AddPublicKey(logger, userID, UniqueName("key-"), content)
		require.NoError(t, err)
		return key.ID
	}
	ownerKeyID := addKey(owner.ID)

	dir := filepath.Join(t.TempDir(), "clone")
	out, err := servCommand(t, ownerKeyID, filepath.Dir(dir), "clone", remoteURL, dir).CombinedOutput()
	require.NoError(t, err, "clone: %s", out)
	want := gitCommitFile(t, dir, UniqueName("file-")+".txt")
	out, err = servCommand(t, ownerKeyID, dir, "push", "origin", "main").CombinedOutput()
	require.NoError(t, err, "push: %s", out)
	assert.Equal(t, want, gitMainOnDisk(t, repo))

	actions := gitRepoActions(t, owner.ID, repo.ID)
	require.NotEmpty(t, actions)
	assert.Equal(t, database.ActionCommitRepo, actions[0].OpType)
	assert.Equal(t, owner.ID, actions[0].ActUserID)
	assert.Contains(t, actions[0].Content, want)

	t.Run("user without write access", func(t *testing.T) {
		gitCommitFile(t, dir, UniqueName("file-")+".txt")
		out, err := servCommand(t, addKey(other.ID), dir, "push", "origin", "main").CombinedOutput()
		require.Error(t, err, "push succeeded: %s", out)
		assert.Contains(t, string(out), "Gogs: You do not have sufficient authorization for this action")
		assert.Equal(t, want, gitMainOnDisk(t, repo))
	})
}
