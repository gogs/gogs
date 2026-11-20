package database

import (
	"encoding/base64"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

func TestSSHParsePublicKey(t *testing.T) {
	tempPath := t.TempDir()
	tests := []struct {
		name      string
		content   string
		expType   string
		expLength int
	}{
		{
			name:      "rsa-2048",
			content:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDMZXh+1OBUwSH9D45wTaxErQIN9IoC9xl7MKJkqvTvv6O5RR9YW/IK9FbfjXgXsppYGhsCZo1hFOOsXHMnfOORqu/xMDx4yPuyvKpw4LePEcg4TDipaDFuxbWOqc/BUZRZcXu41QAWfDLrInwsltWZHSeG7hjhpacl4FrVv9V1pS6Oc5Q1NxxEzTzuNLS/8diZrTm/YAQQ/+B+mzWI3zEtF4miZjjAljWd1LTBPvU23d29DcBmmFahcZ441XZsTeAwGxG/Q6j8NgNXj9WxMeWwxXV2jeAX/EBSpZrCVlCQ1yJswT6xCp8TuBnTiGWYMBNTbOZvPC4e0WI2/yZW/s5F nocomment",
			expType:   "rsa",
			expLength: 2048,
		},
		{
			name:      "ecdsa-256",
			content:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFQacN3PrOll7PXmN5B/ZNVahiUIqI05nbBlZk1KXsO3d06ktAWqbNflv2vEmA38bTFTfJ2sbn2B5ksT52cDDbA= nocomment",
			expType:   "ecdsa",
			expLength: 256,
		},
		{
			name:      "ecdsa-384",
			content:   "ecdsa-sha2-nistp384 AAAAE2VjZHNhLXNoYTItbmlzdHAzODQAAAAIbmlzdHAzODQAAABhBINmioV+XRX1Fm9Qk2ehHXJ2tfVxW30ypUWZw670Zyq5GQfBAH6xjygRsJ5wWsHXBsGYgFUXIHvMKVAG1tpw7s6ax9oA+dJOJ7tj+vhn8joFqT+sg3LYHgZkHrfqryRasQ== nocomment",
			expType:   "ecdsa",
			expLength: 384,
		},
		{
			name:      "ecdsa-521",
			content:   "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBACGt3UG3EzRwNOI17QR84l6PgiAcvCE7v6aXPj/SC6UWKg4EL8vW9ZBcdYL9wzs4FZXh4MOV8jAzu3KRWNTwb4k2wFNUpGOt7l28MztFFEtH5BDDrtAJSPENPy8pvPLMfnPg5NhvWycqIBzNcHipem5wSJFN5PdpNOC2xMrPWKNqj+ZjQ== nocomment",
			expType:   "ecdsa",
			expLength: 521,
		},
		{
			name:      "ed25519-256",
			content:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICGYutovQfTewtcodVN1E1UUzMk4GQfiRI5ZoP/kTlDb nocomment",
			expType:   "ed25519",
			expLength: 256,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			typ, length, err := SSHNativeParsePublicKey(test.content)
			require.NoError(t, err)
			assert.Equal(t, test.expType, typ)
			assert.Equal(t, test.expLength, length)

			typ, length, err = SSHKeygenParsePublicKey(test.content, tempPath, "ssh-keygen")
			require.NoError(t, err)
			assert.Equal(t, test.expType, typ)
			assert.Equal(t, test.expLength, length)
		})
	}
}

func TestCheckPublicKeyStringUnknownAlgorithm(t *testing.T) {
	conf.SetMockSSH(t, conf.SSHOpts{
		StartBuiltinServer:  true,
		MinimumKeySizeCheck: true,
	})

	// A well-formed key line whose algorithm the SSH library does not know.
	key := base64.StdEncoding.EncodeToString(ssh.Marshal(struct{ Name string }{"ssh-unknown"}))
	_, err := CheckPublicKeyString(logx.NewNoopLogger(), "ssh-unknown "+key)
	assert.True(t, IsErrKeyUnableVerify(err), "error: %v", err)
}

func TestCheckPublicKeyStringWithSSHKeygen(t *testing.T) {
	keyTestPath := t.TempDir()
	conf.SetMockSSH(t, conf.SSHOpts{
		KeygenPath:          "ssh-keygen",
		KeyTestPath:         keyTestPath,
		MinimumKeySizeCheck: true,
	})
	// A key line whose type matches but whose key data ssh-keygen cannot read.
	truncated := "ssh-ed25519 " + base64.StdEncoding.EncodeToString(ssh.Marshal(struct{ Name string }{"ssh-ed25519"}))

	t.Run("key ssh-keygen rejects", func(t *testing.T) {
		for _, content := range []string{
			truncated,
			// ssh-keygen reads it as a private key and answers "is not a key file".
			truncated + " PRIVATE KEY",
		} {
			_, err := CheckPublicKeyString(logx.NewNoopLogger(), content)
			require.Error(t, err)
			assert.True(t, IsErrKeyUnableVerify(err), "error: %v", err)
			assert.Regexp(t, `\[result: key is not a (public )?key file\.\]$`, err.Error())
		}
	})

	t.Run("server fails to check", func(t *testing.T) {
		for name, opts := range map[string]conf.SSHOpts{
			"missing ssh-keygen":    {KeygenPath: filepath.Join(keyTestPath, "missing"), KeyTestPath: keyTestPath, MinimumKeySizeCheck: true},
			"missing key test path": {KeygenPath: "ssh-keygen", KeyTestPath: filepath.Join(keyTestPath, "missing"), MinimumKeySizeCheck: true},
		} {
			conf.SSH = opts
			_, err := CheckPublicKeyString(logx.NewNoopLogger(), truncated)
			assert.True(t, IsErrKeyCheckFailed(err), "%s, error: %v", name, err)
			assert.False(t, IsErrKeyUnableVerify(err), "%s, error: %v", name, err)
		}
	})
}
