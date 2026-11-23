package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/osx"
)

func TestUploads(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &UploadsStore{
		db: newTestDB(t, "UploadsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *UploadsStore)
	}{
		{"New", uploadsNew},
		{"Get", uploadsGet},
		{"Delete", uploadsDelete},
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

func uploadsNew(t *testing.T, ctx context.Context, s *UploadsStore) {
	tempPath := t.TempDir()
	// 🚨 SECURITY: The name must not escape the directory it is committed to.
	upload, err := s.New(ctx, tempPath, "../../docs/README.md", strings.NewReader("# Hello"))
	require.NoError(t, err)
	assert.Equal(t, "docs/README.md", upload.Name)
	assert.True(t, strings.HasPrefix(upload.LocalPath(tempPath), tempPath), "stored at %q", upload.LocalPath(tempPath))
	data, err := os.ReadFile(upload.LocalPath(tempPath))
	require.NoError(t, err)
	assert.Equal(t, "# Hello", string(data))

	got, err := s.GetByUUID(ctx, upload.UUID)
	require.NoError(t, err)
	assert.Equal(t, upload.ID, got.ID)
	assert.Equal(t, "docs/README.md", got.Name)

	_, err = s.New(ctx, tempPath, "/../", strings.NewReader("x"))
	require.Error(t, err)
}

func uploadsGet(t *testing.T, ctx context.Context, s *UploadsStore) {
	tempPath := t.TempDir()
	// Malformed UUIDs are not found either.
	for _, uuid := range []string{"404", "00000000-0000-0000-0000-000000000000", "urn:uuid:00000000-0000-0000-0000-000000000000"} {
		_, err := s.GetByUUID(ctx, uuid)
		assert.True(t, IsErrUploadNotExist(err), "%q: got %v", uuid, err)
	}

	upload1, err := s.New(ctx, tempPath, "a.txt", strings.NewReader("a"))
	require.NoError(t, err)
	upload2, err := s.New(ctx, tempPath, "b.txt", strings.NewReader("b"))
	require.NoError(t, err)
	_, err = s.New(ctx, tempPath, "c.txt", strings.NewReader("c"))
	require.NoError(t, err)

	// Unknown and malformed UUIDs are skipped.
	got, err := s.ListByUUIDs(ctx, []string{upload2.UUID, "404", "00000000-0000-0000-0000-000000000000", upload1.UUID})
	require.NoError(t, err)
	var names []string
	for _, upload := range got {
		names = append(names, upload.Name)
	}
	assert.ElementsMatch(t, []string{"a.txt", "b.txt"}, names)

	got, err = s.ListByUUIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func uploadsDelete(t *testing.T, ctx context.Context, s *UploadsStore) {
	tempPath := t.TempDir()
	upload1, err := s.New(ctx, tempPath, "a.txt", strings.NewReader("a"))
	require.NoError(t, err)
	upload2, err := s.New(ctx, tempPath, "b.txt", strings.NewReader("b"))
	require.NoError(t, err)
	upload3, err := s.New(ctx, tempPath, "c.txt", strings.NewReader("c"))
	require.NoError(t, err)

	err = s.Delete(ctx, tempPath, upload1, upload2)
	require.NoError(t, err)
	for _, upload := range []*Upload{upload1, upload2} {
		_, err = s.GetByUUID(ctx, upload.UUID)
		assert.True(t, IsErrUploadNotExist(err), "got %v", err)
		assert.False(t, osx.Exist(upload.LocalPath(tempPath)), "file of %q not removed", upload.Name)
	}
	assert.True(t, osx.IsFile(upload3.LocalPath(tempPath)))

	err = s.DeleteByUUID(ctx, tempPath, upload3.UUID)
	require.NoError(t, err)
	_, err = s.GetByUUID(ctx, upload3.UUID)
	assert.True(t, IsErrUploadNotExist(err), "got %v", err)
	assert.False(t, osx.Exist(upload3.LocalPath(tempPath)))

	// Deleting an upload that does not exist is not an error.
	err = s.DeleteByUUID(ctx, tempPath, upload3.UUID)
	require.NoError(t, err)
	err = s.DeleteByUUID(ctx, tempPath, "404")
	require.NoError(t, err)
}
