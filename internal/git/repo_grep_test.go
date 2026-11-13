package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_Grep(t *testing.T) {
	ctx := context.Background()
	r := setupBareRepoWithAlternates(t)
	commitID := commitFiles(t, r, "search", map[string]string{
		"dir/a: b.txt": "Hello world\r\nfoo\nsay HELLO again\n",
		"main.go":      "abc\na.c\nhello\n",
		"image.bin":    "hello\x00binary",
	})

	t.Run("matches literally and ignores case", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "hello")
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "dir/a: b.txt", Line: 1, Text: "Hello world"},
			{Path: "dir/a: b.txt", Line: 3, Text: "say HELLO again"},
			{Path: "main.go", Line: 3, Text: "hello"},
		}, results)

		results, _, err = r.Grep(ctx, "search", "a.c")
		require.NoError(t, err)
		assert.Equal(t, []*GrepResult{{Path: "main.go", Line: 2, Text: "a.c"}}, results)
	})

	t.Run("no match", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "nothing like this")
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, results)
	})

	t.Run("limits", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "hello", GrepOptions{MaxResultsPerFile: 1})
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "dir/a: b.txt", Line: 1, Text: "Hello world"},
			{Path: "main.go", Line: 3, Text: "hello"},
		}, results)

		results, truncated, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResults: 2})
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Len(t, results, 2)

		_, truncated, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResults: 3})
		require.NoError(t, err)
		assert.False(t, truncated)
	})
}
