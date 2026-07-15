package database

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleMirrorCredentials(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		mosaics bool
		want    string
	}{
		{
			name:    "masks HTTP credentials",
			rawURL:  "https://alice:secret@example.com/repo.git",
			mosaics: true,
			want:    "https://<credentials>@example.com/repo.git",
		},
		{
			name:    "strips HTTP credentials",
			rawURL:  "https://alice:secret@example.com/repo.git",
			mosaics: false,
			want:    "https://example.com/repo.git",
		},
		{
			name:    "leaves SSH SCP syntax unchanged",
			rawURL:  "git@example.com:owner/repo.git",
			mosaics: true,
			want:    "git@example.com:owner/repo.git",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, HandleMirrorCredentials(test.rawURL, test.mosaics))
		})
	}
}

func TestIsMirrorTLSVerificationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "git peer certificate error",
			err:  errors.New("clone: exit status 128 - fatal: unable to access 'https://example.com/repo.git/': Peer's Certificate issuer is not recognized."),
			want: true,
		},
		{
			name: "git known CA certificates error",
			err:  errors.New("clone: exit status 128 - fatal: unable to access 'https://example.com/repo.git/': Peer certificate cannot be authenticated with known CA certificates"),
			want: true,
		},
		{
			name: "x509 unknown authority error",
			err:  errors.New("Get \"https://example.com\": x509: certificate signed by unknown authority"),
			want: true,
		},
		{
			name: "non certificate error",
			err:  errors.New("clone: exit status 128 - fatal: repository 'https://example.com/repo.git/' not found"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, IsMirrorTLSVerificationError(test.err))
		})
	}
}

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
