package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginSessions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &LoginSessionsStore{
		db: newTestDB(t, "LoginSessionsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *LoginSessionsStore)
	}{
		{"CreateAndGetByToken", loginSessionsCreateAndGetByToken},
		{"Touch", loginSessionsTouch},
		{"DeleteByID", loginSessionsDeleteByID},
		{"DeleteAll", loginSessionsDeleteAll},
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

func loginSessionsCreateAndGetByToken(t *testing.T, ctx context.Context, s *LoginSessionsStore) {
	created, token, err := s.Create(ctx, 1, "Firefox", "203.0.113.7")
	require.NoError(t, err)
	assert.Len(t, token, 64)
	assert.NotEqual(t, token, created.TokenHash, "The token itself should not be stored")

	got, err := s.GetByToken(ctx, 1, token)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "Firefox", got.UserAgent)
	assert.Equal(t, "203.0.113.7", got.IP)
	assert.Equal(t, got.CreatedUnix, got.LastSeenUnix)

	// The token of one user does not sign in another.
	_, err = s.GetByToken(ctx, 2, token)
	assert.True(t, IsErrLoginSessionNotExist(err))
	_, err = s.GetByToken(ctx, 1, "")
	assert.True(t, IsErrLoginSessionNotExist(err))
	_, err = s.GetByToken(ctx, 1, token+"0")
	assert.True(t, IsErrLoginSessionNotExist(err))
}

func loginSessionsTouch(t *testing.T, ctx context.Context, s *LoginSessionsStore) {
	created, token, err := s.Create(ctx, 1, "", "")
	require.NoError(t, err)

	// Seen a moment ago: not written again.
	require.NoError(t, s.Touch(ctx, created))
	got, err := s.GetByToken(ctx, 1, token)
	require.NoError(t, err)
	assert.Equal(t, created.LastSeenUnix, got.LastSeenUnix)

	// Seen over a minute ago: written.
	got.LastSeenUnix -= 61
	require.NoError(t, s.Touch(ctx, got))
	got, err = s.GetByToken(ctx, 1, token)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Unix(), got.LastSeenUnix)
}

func loginSessionsDeleteByID(t *testing.T, ctx context.Context, s *LoginSessionsStore) {
	created, token, err := s.Create(ctx, 1, "", "")
	require.NoError(t, err)

	// Deleting a login session of another user is a no-op.
	require.NoError(t, s.DeleteByID(ctx, 2, created.ID))
	_, err = s.GetByToken(ctx, 1, token)
	require.NoError(t, err)

	require.NoError(t, s.DeleteByID(ctx, 1, created.ID))
	_, err = s.GetByToken(ctx, 1, token)
	assert.True(t, IsErrLoginSessionNotExist(err))
}

func loginSessionsDeleteAll(t *testing.T, ctx context.Context, s *LoginSessionsStore) {
	kept, _, err := s.Create(ctx, 1, "", "")
	require.NoError(t, err)
	_, _, err = s.Create(ctx, 1, "", "")
	require.NoError(t, err)
	_, _, err = s.Create(ctx, 1, "", "")
	require.NoError(t, err)
	other, _, err := s.Create(ctx, 2, "", "")
	require.NoError(t, err)

	require.NoError(t, s.DeleteAll(ctx, 1, kept.ID))
	got, err := s.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, kept.ID, got[0].ID)

	require.NoError(t, s.DeleteAll(ctx, 1, 0))
	got, err = s.List(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = s.List(ctx, 2)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, other.ID, got[0].ID)
}
