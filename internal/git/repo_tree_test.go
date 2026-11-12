package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnescapeChars(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no escapes",
			in:   "normal-filename.txt",
			want: "normal-filename.txt",
		},
		{
			name: "escaped quote",
			in:   `Test \"Word\".md`,
			want: `Test "Word".md`,
		},
		{
			name: "escaped backslash",
			in:   `path\\to\\file.txt`,
			want: `path\to\file.txt`,
		},
		{
			name: "escaped tab",
			in:   `file\twith\ttabs.txt`,
			want: "file\twith\ttabs.txt",
		},
		{
			name: "mixed escapes",
			in:   `\"quoted\\path\t.md`,
			want: "\"quoted\\path\t.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnescapeChars([]byte(tt.in))
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestRepository_LsTree(t *testing.T) {
	ctx := context.Background()
	if runtime.GOOS == "windows" {
		t.Skip(`Windows does not allow '"' in filenames`)
	}

	path := tempPath()
	defer os.RemoveAll(path)

	err := Init(ctx, path)
	require.NoError(t, err)

	specialName := `Test "Wiki" Page.md`
	err = os.WriteFile(filepath.Join(path, specialName), []byte("content"), 0o644)
	require.NoError(t, err)

	repo, err := Open(path)
	require.NoError(t, err)

	err = repo.Add(ctx, AddOptions{All: true})
	require.NoError(t, err)

	err = repo.Commit(ctx, &Signature{Name: "test", Email: "test@test.com"}, "initial commit")
	require.NoError(t, err)

	commit, err := repo.CatFileCommit(ctx, "HEAD")
	require.NoError(t, err)

	// Without Verbatim, Git quotes and escapes the filename.
	entries, err := commit.Entries(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, specialName, entries[0].Name())

	// With Verbatim, Git outputs the filename as-is.
	entries, err = commit.Entries(ctx, LsTreeOptions{Verbatim: true})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, specialName, entries[0].Name())
}
