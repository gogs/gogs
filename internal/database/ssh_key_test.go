package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
