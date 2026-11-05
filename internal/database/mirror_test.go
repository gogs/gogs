package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseRemoteUpdateOutput(t *testing.T) {
	tests := []struct {
		output     string
		expResults []*mirrorSyncResult
	}{
		{
			`
From https://try.gogs.io/unknwon/upsteam
 * [new branch]      develop    -> develop
   b0bb24f..1d85a4f  master     -> master
 - [deleted]         (none)     -> bugfix
`,
			[]*mirrorSyncResult{
				{"develop", gitShortEmptyID, ""},
				{"master", "b0bb24f", "1d85a4f"},
				{"bugfix", "", gitShortEmptyID},
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expResults, parseRemoteUpdateOutput(test.output))
		})
	}
}

func TestIsMigrationCertificateError(t *testing.T) {
	tests := []struct {
		name    string
		message string
		expVal  bool
	}{
		{
			name:    "libcurl issuer not recognized",
			message: "fatal: unable to access 'https://example.com/repo.git/': Peer's Certificate issuer is not recognized.",
			expVal:  true,
		},
		{
			name:    "cannot authenticate known ca",
			message: "fatal: unable to access 'https://example.com/repo.git/': Peer certificate cannot be authenticated with known CA certificates",
			expVal:  true,
		},
		{
			name:    "go unknown authority",
			message: "x509: certificate signed by unknown authority",
			expVal:  true,
		},
		{
			name:    "non-certificate fatal",
			message: "fatal: repository 'https://example.com/repo.git/' not found",
			expVal:  false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsMigrationCertificateError(test.message))
		})
	}
}

func TestHandleMirrorCredentialsAddsMigrationCertificateHint(t *testing.T) {
	message := HandleMirrorCredentials("clone: fatal: unable to access 'https://alice:secret@example.com/repo.git/': Peer's Certificate issuer is not recognized.", true)

	assert.Contains(t, message, "https://<credentials>@example.com/repo.git/")
	assert.NotContains(t, message, "secret")
	assert.Contains(t, message, "remote Git server certificate is not trusted")
	assert.Contains(t, message, "git config --global http.sslCAInfo")
}
