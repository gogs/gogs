package integrationtests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// userAuthenticator is a software WebAuthn authenticator that holds a single
// passkey, which verifies the user every time it is used.
type userAuthenticator struct {
	key          *ecdsa.PrivateKey
	credentialID []byte
	userHandle   []byte
	signCount    uint32
}

func newUserAuthenticator(t *testing.T) *userAuthenticator {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &userAuthenticator{key: key, credentialID: id}
}

var userB64 = base64.RawURLEncoding

// Flags of the authenticator data: user present, user verified and attested
// credential data included.
const (
	userFlagUP = 0x01
	userFlagUV = 0x04
	userFlagAT = 0x40
)

func (a *userAuthenticator) authData(t *testing.T, flags byte, attested []byte) []byte {
	t.Helper()

	rpIDHash := sha256.Sum256([]byte(cfg.Server.URL.Hostname()))
	data := append([]byte{}, rpIDHash[:]...)
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, a.signCount)
	return append(data, attested...)
}

func (*userAuthenticator) clientData(t *testing.T, typ, challenge string) []byte {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   challenge,
		"origin":      cfg.Server.URL.Scheme + "://" + cfg.Server.URL.Host,
		"crossOrigin": false,
	})
	require.NoError(t, err)
	return b
}

// create answers the creation options the server returned with a new passkey
// and the "none" attestation.
func (a *userAuthenticator) create(t *testing.T, options []byte) json.RawMessage {
	t.Helper()

	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			User      struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(options, &opts))
	var err error
	a.userHandle, err = userB64.DecodeString(opts.PublicKey.User.ID)
	require.NoError(t, err)

	publicKey, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{
			KeyType:   int64(webauthncose.EllipticKey),
			Algorithm: int64(webauthncose.AlgES256),
		},
		Curve:  int64(webauthncose.P256),
		XCoord: a.key.X.FillBytes(make([]byte, 32)),
		YCoord: a.key.Y.FillBytes(make([]byte, 32)),
	})
	require.NoError(t, err)
	attested := make([]byte, 16) // A zero AAGUID.
	attested = binary.BigEndian.AppendUint16(attested, uint16(len(a.credentialID)))
	attested = append(attested, a.credentialID...)
	attested = append(attested, publicKey...)

	attestation, err := webauthncbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": a.authData(t, userFlagUP|userFlagUV|userFlagAT, attested),
	})
	require.NoError(t, err)

	b, err := json.Marshal(map[string]any{
		"id":    userB64.EncodeToString(a.credentialID),
		"rawId": userB64.EncodeToString(a.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    userB64.EncodeToString(a.clientData(t, "webauthn.create", opts.PublicKey.Challenge)),
			"attestationObject": userB64.EncodeToString(attestation),
		},
	})
	require.NoError(t, err)
	return b
}

// get answers the request options the server returned with a signature of the
// passkey.
func (a *userAuthenticator) get(t *testing.T, options []byte) map[string]any {
	t.Helper()

	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(options, &opts))

	a.signCount++
	authData := a.authData(t, userFlagUP|userFlagUV, nil)
	clientData := a.clientData(t, "webauthn.get", opts.PublicKey.Challenge)
	clientDataHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	require.NoError(t, err)

	return map[string]any{
		"id":    userB64.EncodeToString(a.credentialID),
		"rawId": userB64.EncodeToString(a.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    userB64.EncodeToString(clientData),
			"authenticatorData": userB64.EncodeToString(authData),
			"signature":         userB64.EncodeToString(signature),
			"userHandle":        userB64.EncodeToString(a.userHandle),
		},
	}
}

type userPasskeyItem struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	LastUsedAt *string `json:"lastUsedAt"`
}

func userListPasskeys(c *client) []userPasskeyItem {
	c.t.Helper()

	var resp struct {
		Items []userPasskeyItem `json:"items"`
	}
	c.doJSON(http.MethodGet, "/api/web/user/settings/passkeys", nil, http.StatusOK, &resp)
	return resp.Items
}

func TestUserPasskeys(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, u)
	assert.Empty(t, userListPasskeys(c))

	t.Run("registration options", func(t *testing.T) {
		var options struct {
			PublicKey struct {
				Challenge string `json:"challenge"`
				RP        struct {
					ID string `json:"id"`
				} `json:"rp"`
				User struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"user"`
				AuthenticatorSelection struct {
					ResidentKey      string `json:"residentKey"`
					UserVerification string `json:"userVerification"`
				} `json:"authenticatorSelection"`
			} `json:"publicKey"`
		}
		c.doJSON(http.MethodPost, "/api/web/user/settings/passkeys/registration", map[string]any{"name": "Laptop"}, http.StatusOK, &options)
		assert.NotEmpty(t, options.PublicKey.Challenge)
		assert.Equal(t, "localhost", options.PublicKey.RP.ID)
		assert.Equal(t, u.Name, options.PublicKey.User.Name)
		assert.Equal(t, userB64.EncodeToString(binary.BigEndian.AppendUint64(nil, uint64(u.ID))), options.PublicKey.User.ID)
		assert.Equal(t, "required", options.PublicKey.AuthenticatorSelection.ResidentKey)
		assert.Equal(t, "required", options.PublicKey.AuthenticatorSelection.UserVerification)

		status, body := c.do(http.MethodPost, "/api/web/user/settings/passkeys/registration", map[string]any{"name": ""})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	})

	t.Run("add without a registration in progress", func(t *testing.T) {
		other := authenticatedClient(t, seedUser(t, CreateUserOptions{}))
		status, body := other.do(http.MethodPost, "/api/web/user/settings/passkeys", map[string]any{
			"name":       "Laptop",
			"credential": map[string]any{"id": "x"},
		})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	})

	t.Run("add a malformed credential", func(t *testing.T) {
		status, body := c.do(http.MethodPost, "/api/web/user/settings/passkeys/registration", map[string]any{"name": "Laptop"})
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = c.do(http.MethodPost, "/api/web/user/settings/passkeys", map[string]any{
			"name":       "Laptop",
			"credential": map[string]any{"id": "x", "type": "public-key"},
		})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Empty(t, userListPasskeys(c))
	})

	t.Run("signed out", func(t *testing.T) {
		anonymous := newClient(t)
		for _, r := range []struct{ method, path string }{
			{http.MethodGet, "/api/web/user/settings/passkeys"},
			{http.MethodPost, "/api/web/user/settings/passkeys/registration"},
			{http.MethodPost, "/api/web/user/settings/passkeys"},
			{http.MethodDelete, "/api/web/user/settings/passkeys/1"},
		} {
			status, body := anonymous.do(r.method, r.path, map[string]any{})
			assert.Equal(t, http.StatusUnauthorized, status, "%s %s, response: %s", r.method, r.path, body)
		}
	})

	t.Run("sign-in without a challenge", func(t *testing.T) {
		anonymous := newClient(t)
		status, body := anonymous.do(http.MethodPost, "/api/web/user/sign-in/passkey", map[string]any{"id": "x"})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, anonymous.username())
	})

	t.Run("register, sign in and delete", func(t *testing.T) {
		authenticator := newUserAuthenticator(t)
		name := UniqueName("passkey-")
		status, options := c.do(http.MethodPost, "/api/web/user/settings/passkeys/registration", map[string]any{"name": name})
		require.Equal(t, http.StatusOK, status, "response: %s", options)
		status, body := c.do(http.MethodPost, "/api/web/user/settings/passkeys", map[string]any{
			"name":       name,
			"credential": authenticator.create(t, options),
		})
		require.Equal(t, http.StatusCreated, status, "response: %s", body)

		passkeys := userListPasskeys(c)
		require.Len(t, passkeys, 1)
		assert.Equal(t, name, passkeys[0].Name)
		assert.Nil(t, passkeys[0].LastUsedAt)

		// A challenge is only answered once.
		status, body = c.do(http.MethodPost, "/api/web/user/settings/passkeys", map[string]any{
			"name":       name,
			"credential": authenticator.create(t, options),
		})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

		visitor := newClient(t)
		var challenge json.RawMessage
		visitor.doJSON(http.MethodPost, "/api/web/user/sign-in/passkey/challenge", map[string]any{}, http.StatusOK, &challenge)
		var challengeOpts struct {
			PublicKey struct {
				Challenge        string `json:"challenge"`
				RPID             string `json:"rpId"`
				UserVerification string `json:"userVerification"`
			} `json:"publicKey"`
		}
		require.NoError(t, json.Unmarshal(challenge, &challengeOpts))
		assert.NotEmpty(t, challengeOpts.PublicKey.Challenge)
		assert.Equal(t, "localhost", challengeOpts.PublicKey.RPID)
		assert.Equal(t, "required", challengeOpts.PublicKey.UserVerification)

		status, body = visitor.do(http.MethodPost, "/api/web/user/sign-in/passkey", authenticator.get(t, challenge))
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, u.Name, visitor.username())
		passkeys = userListPasskeys(c)
		require.Len(t, passkeys, 1)
		assert.NotNil(t, passkeys[0].LastUsedAt)

		// Another user cannot delete the passkey.
		other := authenticatedClient(t, seedUser(t, CreateUserOptions{}))
		status, body = other.do(http.MethodDelete, "/api/web/user/settings/passkeys/"+strconv.FormatInt(passkeys[0].ID, 10), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		stored, err := database.Handle.Passkeys().List(t.Context(), u.ID)
		require.NoError(t, err)
		assert.Len(t, stored, 1)

		status, body = c.do(http.MethodDelete, "/api/web/user/settings/passkeys/"+strconv.FormatInt(passkeys[0].ID, 10), map[string]any{})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, userListPasskeys(c))

		// A deleted passkey no longer signs in.
		late := newClient(t)
		late.doJSON(http.MethodPost, "/api/web/user/sign-in/passkey/challenge", map[string]any{}, http.StatusOK, &challenge)
		status, body = late.do(http.MethodPost, "/api/web/user/sign-in/passkey", authenticator.get(t, challenge))
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, late.username())
	})
}
