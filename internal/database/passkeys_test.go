package database

import (
	"context"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPasskey_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("timestamps have been set", func(t *testing.T) {
		passkey := &Passkey{
			CreatedUnix: 1,
			UpdatedUnix: 2,
		}
		_ = passkey.BeforeCreate(db)
		assert.Equal(t, int64(1), passkey.CreatedUnix)
		assert.Equal(t, int64(2), passkey.UpdatedUnix)
	})

	t.Run("timestamps have not been set", func(t *testing.T) {
		passkey := &Passkey{}
		_ = passkey.BeforeCreate(db)
		assert.Equal(t, now.Unix(), passkey.CreatedUnix)
		assert.Equal(t, now.Unix(), passkey.UpdatedUnix)
	})
}

func TestPasskey_AfterFind(t *testing.T) {
	now := time.Now()

	passkey := &Passkey{
		CreatedUnix:  now.Unix(),
		UpdatedUnix:  now.Add(time.Minute).Unix(),
		LastUsedUnix: now.Add(2 * time.Minute).Unix(),
	}
	_ = passkey.AfterFind(nil)

	assert.Equal(t, passkey.CreatedUnix, passkey.Created.Unix())
	assert.Equal(t, passkey.UpdatedUnix, passkey.Updated.Unix())
	assert.Equal(t, passkey.LastUsedUnix, passkey.LastUsed.Unix())
}

func TestPasskey_CredentialStruct(t *testing.T) {
	t.Run("valid credential", func(t *testing.T) {
		passkey := &Passkey{
			Credential: `{"id":"Y3JlZGVudGlhbC0x","publicKey":"cHVibGljLWtleQ==","attestationType":"none","transport":null,"flags":{"userPresent":false,"userVerified":false,"backupEligible":false,"backupState":false},"authenticator":{"AAGUID":null,"signCount":1,"cloneWarning":false,"attachment":""},"attestation":{"clientDataJSON":null,"clientDataHash":null,"authenticatorData":null,"publicKeyAlgorithm":0,"object":null}}`,
		}
		credential, err := passkey.CredentialStruct()
		require.NoError(t, err)
		assert.Equal(t, []byte("credential-1"), credential.ID)
	})

	t.Run("invalid credential", func(t *testing.T) {
		passkey := &Passkey{
			Credential: "{not-json",
		}
		_, err := passkey.CredentialStruct()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal credential")
	})
}

func TestPasskeys(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &PasskeysStore{
		db: newTestDB(t, "PasskeysStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *PasskeysStore)
	}{
		{"CreateAndList", passkeysCreateAndList},
		{"CreateDuplicate", passkeysCreateDuplicate},
		{"UpdateCredential", passkeysUpdateCredential},
		{"UpdateCredentialNotFound", passkeysUpdateCredentialNotFound},
		{"DeleteByID", passkeysDeleteByID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func passkeysCreateAndList(t *testing.T, ctx context.Context, s *PasskeysStore) {
	passkey, err := s.Create(ctx, 1, "macbook", testCredential(1))
	require.NoError(t, err)
	assert.Equal(t, int64(1), passkey.UserID)
	assert.Equal(t, "macbook", passkey.Name)

	passkeys, err := s.ListByUserID(ctx, 1)
	require.NoError(t, err)
	require.Len(t, passkeys, 1)

	credential, err := passkeys[0].CredentialStruct()
	require.NoError(t, err)
	assert.Equal(t, []byte("credential-1"), credential.ID)
	assert.Equal(t, uint32(1), credential.Authenticator.SignCount)
}

func passkeysCreateDuplicate(t *testing.T, ctx context.Context, s *PasskeysStore) {
	_, err := s.Create(ctx, 1, "macbook", testCredential(1))
	require.NoError(t, err)

	_, err = s.Create(ctx, 2, "iphone", testCredential(1))
	assert.True(t, IsErrPasskeyAlreadyExist(err))
}

func passkeysUpdateCredential(t *testing.T, ctx context.Context, s *PasskeysStore) {
	created, err := s.Create(ctx, 1, "macbook", testCredential(1))
	require.NoError(t, err)

	updated := testCredential(5)
	err = s.UpdateCredential(ctx, 1, created.ID, updated)
	require.NoError(t, err)

	passkeys, err := s.ListByUserID(ctx, 1)
	require.NoError(t, err)
	require.Len(t, passkeys, 1)
	assert.NotZero(t, passkeys[0].LastUsedUnix)

	stored, err := passkeys[0].CredentialStruct()
	require.NoError(t, err)
	assert.Equal(t, uint32(5), stored.Authenticator.SignCount)
}

func passkeysDeleteByID(t *testing.T, ctx context.Context, s *PasskeysStore) {
	created, err := s.Create(ctx, 1, "macbook", testCredential(1))
	require.NoError(t, err)

	err = s.DeleteByID(ctx, 2, created.ID)
	assert.True(t, IsErrPasskeyNotFound(err))

	err = s.DeleteByID(ctx, 1, created.ID)
	require.NoError(t, err)

	passkeys, err := s.ListByUserID(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, passkeys)
}

func passkeysUpdateCredentialNotFound(t *testing.T, ctx context.Context, s *PasskeysStore) {
	err := s.UpdateCredential(ctx, 1, 999, testCredential(2))
	assert.True(t, IsErrPasskeyNotFound(err))
}

func testCredential(signCount uint32) webauthn.Credential {
	return webauthn.Credential{
		ID:              []byte("credential-1"),
		PublicKey:       []byte("public-key"),
		AttestationType: "none",
		Authenticator: webauthn.Authenticator{
			SignCount: signCount,
		},
	}
}
