package lfsx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// Token grants a user access to the LFS objects of one repository until it
// expires, e.g., to a client that authenticated over SSH.
type Token struct {
	UserID int64 `json:"u"`
	RepoID int64 `json:"r"`
	// Upload is true when the token also grants uploading objects.
	Upload bool `json:"w"`
	// Expires is the Unix time after which the token is no longer valid.
	Expires int64 `json:"e"`
}

func tokenMAC(secret string, payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte("lfs-token:"+secret))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

// SignToken returns the token signed with the secret.
func SignToken(secret string, t Token) string {
	payload, _ := json.Marshal(t)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(payload) + "." + enc.EncodeToString(tokenMAC(secret, payload))
}

// ParseToken returns the token that SignToken signed with the secret, when it
// has not expired at the given time.
func ParseToken(secret, signed string, now time.Time) (*Token, error) {
	enc := base64.RawURLEncoding
	encodedPayload, encodedMAC, ok := strings.Cut(signed, ".")
	if !ok {
		return nil, errors.New("malformed token")
	}
	payload, err := enc.DecodeString(encodedPayload)
	if err != nil {
		return nil, errors.Wrap(err, "decode payload")
	}
	mac, err := enc.DecodeString(encodedMAC)
	if err != nil {
		return nil, errors.Wrap(err, "decode signature")
	}
	if !hmac.Equal(mac, tokenMAC(secret, payload)) {
		return nil, errors.New("invalid signature")
	}

	var t Token
	if err = json.Unmarshal(payload, &t); err != nil {
		return nil, errors.Wrap(err, "unmarshal payload")
	}
	if now.Unix() >= t.Expires {
		return nil, errors.New("token expired")
	}
	return &t, nil
}
