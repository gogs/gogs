package database

import (
	"context"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{"DeleteByID", passkeysDeleteByID},
		{"MarkUsed", passkeysMarkUsed},
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
	laptop, err := s.Create(ctx, 1, "Laptop", webauthn.Credential{ID: []byte("laptop"), PublicKey: []byte("laptop-key")})
	require.NoError(t, err)
	_, err = s.Create(ctx, 2, "Phone of someone else", webauthn.Credential{ID: []byte("other")})
	require.NoError(t, err)
	_, err = s.Create(ctx, 1, "Phone", webauthn.Credential{ID: []byte("phone")})
	require.NoError(t, err)

	got, err := s.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, laptop.ID, got[0].ID)
	assert.Equal(t, "Laptop", got[0].Name)
	assert.Equal(t, []byte("laptop-key"), got[0].Credential.PublicKey)
	assert.Equal(t, s.db.NowFunc().Unix(), got[0].Created.Unix())
	assert.False(t, got[0].HasUsed)
	assert.Equal(t, "Phone", got[1].Name)
}

func passkeysDeleteByID(t *testing.T, ctx context.Context, s *PasskeysStore) {
	p, err := s.Create(ctx, 1, "Laptop", webauthn.Credential{ID: []byte("laptop")})
	require.NoError(t, err)

	// Deleting a passkey of another user is a no-op.
	err = s.DeleteByID(ctx, 2, p.ID)
	require.NoError(t, err)
	got, err := s.List(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)

	err = s.DeleteByID(ctx, 1, p.ID)
	require.NoError(t, err)
	got, err = s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func passkeysMarkUsed(t *testing.T, ctx context.Context, s *PasskeysStore) {
	p, err := s.Create(ctx, 1, "Laptop", webauthn.Credential{ID: []byte("laptop"), Authenticator: webauthn.Authenticator{SignCount: 1}})
	require.NoError(t, err)

	credential := p.Credential
	credential.Authenticator.SignCount = 2
	err = s.MarkUsed(ctx, p.ID, credential)
	require.NoError(t, err)

	got, err := s.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, uint32(2), got[0].Credential.Authenticator.SignCount)
	assert.Equal(t, []byte("laptop"), got[0].Credential.ID)
	assert.Equal(t, s.db.NowFunc().Unix(), got[0].UpdatedUnix)
	assert.True(t, got[0].HasUsed)
	assert.True(t, got[0].HasRecentActivity)
}
