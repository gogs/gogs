package integrationtests

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

// sshKey is the private key file of an SSH key whose public key is registered
// with the server.
type sshKey struct {
	path string
	// id is the ID of a key of a user, which "gogs serv" takes.
	id int64
	// agentSocket is the socket of the SSH agent that holds the private key,
	// in which case path is the public key file.
	agentSocket string
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
	added, err := database.Handle.PublicKeys().Add(t.Context(), logger, userID, UniqueName("key-"), content)
	require.NoError(t, err)
	key.id = added.ID
	return key
}

// sshDeployKey returns a new read-only deploy key of the repository.
func sshDeployKey(t *testing.T, repo *Repository) *sshKey {
	t.Helper()

	key, content := sshNewKey(t)
	_, err := database.Handle.DeployKeys().Add(t.Context(), logger, repo.ID, UniqueName("deploy-"), content)
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
	agentSocket := "none"
	if k.agentSocket != "" {
		agentSocket = k.agentSocket
	}
	cmd := gitCommand(dir, args...)
	cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -i "+k.path+
		" -o IdentitiesOnly=yes -o IdentityAgent="+agentSocket+" -o BatchMode=yes"+
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

// TestSSHCloneAndPush covers what is particular to the builtin SSH server.
// TestGitPush and TestGitPushWithoutWriteAccess cover pushing over it.
func TestSSHCloneAndPush(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerKey := sshUserKey(t, owner.ID)
	head := gitMainOnDisk(t, repo)

	t.Run("protocol version 2", func(t *testing.T) {
		cmdOut, err := ownerKey.git(t.TempDir(), "-c", "protocol.version=2", "ls-remote", sshRemoteURL(repo), "refs/heads/main")
		require.NoError(t, err, "ls-remote: %s", cmdOut)
		assert.Equal(t, head+"\trefs/heads/main", cmdOut)

		// The server answers in the version the client asks for through the
		// GIT_PROTOCOL environment variable.
		cmd := ownerKey.command(t.TempDir(), "-c", "protocol.version=2", "ls-remote", sshRemoteURL(repo))
		cmd.Env = append(cmd.Env, "GIT_TRACE_PACKET=1")
		trace, err := cmd.CombinedOutput()
		require.NoError(t, err, "ls-remote: %s", trace)
		assert.Contains(t, string(trace), "< version 2")
	})

	t.Run("unregistered key", func(t *testing.T) {
		key, _ := sshNewKey(t)
		out := key.cloneFails(t, repo)
		assert.Contains(t, out, "Permission denied (publickey)")
	})
}

func TestSSHClonePrivate(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

// sshServLsRemote lists the main branch of the repository through "gogs serv"
// with the key ID, as OpenSSH does with the line of the key in the
// authorized_keys file.
func sshServLsRemote(t *testing.T, keyID int64, repo *Repository) (string, error) {
	t.Helper()

	cmd := gitCommand(t.TempDir(), "ls-remote", "ssh://git@localhost/"+repo.Owner+"/"+repo.Name+".git", "refs/heads/main")
	cmd.Env = append(cmd.Env, "GIT_SSH="+servProgram(t, keyID))
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestSSHDeleteDeployKey(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	_, content := sshNewKey(t)
	key, err := database.Handle.DeployKeys().Add(t.Context(), logger, repo.ID, UniqueName("deploy-"), content)
	require.NoError(t, err)
	otherKey, err := database.Handle.DeployKeys().Add(t.Context(), logger, otherRepo.ID, UniqueName("deploy-"), content)
	require.NoError(t, err)

	c := authenticatedClient(t, owner)
	deleteKey := func(repo *Repository, key *database.DeployKey) {
		t.Helper()
		status, body := c.do(http.MethodDelete, repoSettingsAPIPath(repo, "/settings/keys/"+strconv.FormatInt(key.ID, 10)), repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}

	// The other repository still uses the key, so it still gets in.
	deleteKey(repo, key)
	out, err := sshServLsRemote(t, key.KeyID, otherRepo)
	require.NoError(t, err, "ls-remote: %s", out)
	assert.Equal(t, gitMainOnDisk(t, otherRepo)+"\trefs/heads/main", out)
	out, err = sshServLsRemote(t, key.KeyID, repo)
	require.Error(t, err, "ls-remote succeeded: %s", out)

	// The last repository takes the public key with it.
	deleteKey(otherRepo, otherKey)
	_, err = database.Handle.PublicKeys().GetByID(t.Context(), key.KeyID)
	assert.True(t, database.IsErrKeyNotExist(err), "key not deleted: %v", err)
}

func TestSSHDeleteRepositoryWithDeployKeys(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	_, ownContent := sshNewKey(t)
	ownKey, err := database.Handle.DeployKeys().Add(t.Context(), logger, repo.ID, UniqueName("deploy-"), ownContent)
	require.NoError(t, err)
	_, sharedContent := sshNewKey(t)
	sharedKey, err := database.Handle.DeployKeys().Add(t.Context(), logger, repo.ID, UniqueName("deploy-"), sharedContent)
	require.NoError(t, err)
	_, err = database.Handle.DeployKeys().Add(t.Context(), logger, otherRepo.ID, UniqueName("deploy-"), sharedContent)
	require.NoError(t, err)

	status, body := apiV1Client(t, owner).do(http.MethodDelete, apiV1RepoPath(repo, ""), nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	keys, err := database.Handle.DeployKeys().List(t.Context(), repo.ID)
	require.NoError(t, err)
	assert.Empty(t, keys)

	_, err = database.Handle.PublicKeys().GetByID(t.Context(), ownKey.KeyID)
	assert.True(t, database.IsErrKeyNotExist(err), "key not deleted: %v", err)

	// The key of the other repository keeps working.
	_, err = database.Handle.PublicKeys().GetByID(t.Context(), sharedKey.KeyID)
	require.NoError(t, err)
	out, err := sshServLsRemote(t, sharedKey.KeyID, otherRepo)
	require.NoError(t, err, "ls-remote: %s", out)
	assert.Equal(t, gitMainOnDisk(t, otherRepo)+"\trefs/heads/main", out)
}

// servProgram writes a program that Git runs in place of ssh, which connects
// through "gogs serv" with the key of the ID, like OpenSSH does with the
// command that Gogs writes to the authorized_keys file, and returns its path.
func servProgram(t *testing.T, keyID int64) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "ssh")
	script := fmt.Sprintf("#!/bin/sh\n# The command for the server is the last argument.\nfor command; do :; done\nSSH_ORIGINAL_COMMAND=\"$command\" exec '%s' serv key-%d --config='%s'\n",
		os.Args[0], keyID, conf.CustomConf)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

// sshSecurityKeyAgent is an SSH agent that holds the key of a FIDO security
// key, e.g., a YubiKey, and signs as the security key does when touched.
type sshSecurityKeyAgent struct {
	// OpenSSH only lists the keys of the agent and asks it to sign.
	agent.Agent
	key  ssh.PublicKey
	sign func(message []byte) ([]byte, error)
}

func (a *sshSecurityKeyAgent) List() ([]*agent.Key, error) {
	return []*agent.Key{{Format: a.key.Type(), Blob: a.key.Marshal()}}, nil
}

// Sign signs the data as described in PROTOCOL.u2f of OpenSSH.
func (a *sshSecurityKeyAgent) Sign(_ ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	const userPresent = 0x01
	counter := []byte{0, 0, 0, 1}
	application := sha256.Sum256([]byte(sshSecurityKeyApplication))
	digest := sha256.Sum256(data)
	blob, err := a.sign(slices.Concat(application[:], []byte{userPresent}, counter, digest[:]))
	if err != nil {
		return nil, err
	}
	return &ssh.Signature{Format: a.key.Type(), Blob: blob, Rest: slices.Concat([]byte{userPresent}, counter)}, nil
}

// sshSecurityKeyApplication is the application that ssh-keygen creates keys of
// security keys for by default.
const sshSecurityKeyApplication = "ssh:"

// sshNewSecurityKey starts an SSH agent that holds a new key of a security key
// of the algorithm, and returns the key with its public key in the
// authorized_keys format.
func sshNewSecurityKey(t *testing.T, algorithm string) (*sshKey, string) {
	t.Helper()

	a := &sshSecurityKeyAgent{}
	var wire []byte
	switch algorithm {
	case ssh.KeyAlgoSKED25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		wire = ssh.Marshal(struct {
			Name        string
			Key         []byte
			Application string
		}{algorithm, pub, sshSecurityKeyApplication})
		a.sign = func(message []byte) ([]byte, error) { return ed25519.Sign(priv, message), nil }
	case ssh.KeyAlgoSKECDSA256:
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		pub, err := priv.PublicKey.ECDH()
		require.NoError(t, err)
		wire = ssh.Marshal(struct {
			Name        string
			Curve       string
			Key         []byte
			Application string
		}{algorithm, "nistp256", pub.Bytes(), sshSecurityKeyApplication})
		a.sign = func(message []byte) ([]byte, error) {
			digest := sha256.Sum256(message)
			r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
			if err != nil {
				return nil, err
			}
			return ssh.Marshal(struct{ R, S *big.Int }{r, s}), nil
		}
	default:
		t.Fatalf("unsupported algorithm %q", algorithm)
	}
	var err error
	a.key, err = ssh.ParsePublicKey(wire)
	require.NoError(t, err)

	// The path of a Unix socket is limited to about 100 bytes, which the
	// directory of t.TempDir() exceeds for long test names.
	dir, err := os.MkdirTemp("", "agent-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "agent.sock")
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = agent.ServeAgent(a, conn)
			}()
		}
	}()

	content := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(a.key)))
	path := filepath.Join(dir, "id.pub")
	require.NoError(t, os.WriteFile(path, []byte(content+"\n"), 0o600))
	return &sshKey{path: path, agentSocket: socket}, content
}

func TestSSHSecurityKey(t *testing.T) {
	t.Parallel()

	u, c := userSignedIn(t)
	repo := seedRepository(t, u.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	head := gitMainOnDisk(t, repo)
	addKey := func(content string) (int, []byte) {
		t.Helper()
		return c.do(http.MethodPost, "/api/web/user/settings/ssh-keys", map[string]any{
			"title":   UniqueName("key-"),
			"content": content,
		})
	}

	for _, algorithm := range []string{ssh.KeyAlgoSKED25519, ssh.KeyAlgoSKECDSA256} {
		t.Run(algorithm, func(t *testing.T) {
			key, content := sshNewSecurityKey(t, algorithm)
			status, body := addKey(content + " user@example.com")
			require.Equal(t, http.StatusNoContent, status, "response: %s", body)

			dir := key.clone(t, repo)
			assert.Equal(t, head, gitRun(t, dir, "rev-parse", "HEAD"))
		})
	}

	// Keys too short for their type are rejected, so the size check is on.
	t.Run("short RSA key", func(t *testing.T) {
		priv, err := rsa.GenerateKey(rand.Reader, 1536)
		require.NoError(t, err)
		pub, err := ssh.NewPublicKey(&priv.PublicKey)
		require.NoError(t, err)
		status, body := addKey(strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))))
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), "key length is not enough")
	})
}
