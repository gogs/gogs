package lfsx

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToken(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	want := Token{UserID: 2, RepoID: 7, Upload: true, Expires: now.Add(time.Minute).Unix()}
	signed := SignToken("secret", want)

	got, err := ParseToken("secret", signed, now)
	require.NoError(t, err)
	assert.Equal(t, want, *got)

	_, err = ParseToken("secret", signed, now.Add(time.Minute))
	assert.Error(t, err, "expired")
	_, err = ParseToken("other", signed, now)
	assert.Error(t, err, "signed with another secret")

	// A token whose payload is changed, e.g., to another repository or to grant
	// uploading, no longer matches its signature.
	_, mac, _ := strings.Cut(signed, ".")
	forged, _, _ := strings.Cut(SignToken("other", Token{UserID: 2, RepoID: 8, Upload: true, Expires: want.Expires}), ".")
	_, err = ParseToken("secret", forged+"."+mac, now)
	assert.Error(t, err, "payload of another token")

	for _, malformed := range []string{"", "abc", "abc.def", "." + mac} {
		_, err = ParseToken("secret", malformed, now)
		assert.Error(t, err, malformed)
	}
}
