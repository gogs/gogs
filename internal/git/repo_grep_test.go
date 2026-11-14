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
		"ctx.txt":      "one\ntwo\nhello 3\nfour\nfive\nsix\nhello 7\neight\n",
	})

	t.Run("matches literally and ignores case", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "hello")
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "ctx.txt", Line: 3, Text: "hello 3", IsMatch: true},
			{Path: "ctx.txt", Line: 7, Text: "hello 7", IsMatch: true},
			{Path: "dir/a: b.txt", Line: 1, Text: "Hello world", IsMatch: true},
			{Path: "dir/a: b.txt", Line: 3, Text: "say HELLO again", IsMatch: true},
			{Path: "main.go", Line: 3, Text: "hello", IsMatch: true},
		}, results)

		results, _, err = r.Grep(ctx, "search", "a.c")
		require.NoError(t, err)
		assert.Equal(t, []*GrepResult{{Path: "main.go", Line: 2, Text: "a.c", IsMatch: true}}, results)
	})

	t.Run("no match", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "nothing like this")
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Empty(t, results)
	})

	t.Run("context", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "hello", GrepOptions{ContextLines: 1, Pathspecs: []string{"ctx.txt"}})
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "ctx.txt", Line: 2, Text: "two"},
			{Path: "ctx.txt", Line: 3, Text: "hello 3", IsMatch: true},
			{Path: "ctx.txt", Line: 4, Text: "four"},
			{Path: "ctx.txt", Line: 6, Text: "six"},
			{Path: "ctx.txt", Line: 7, Text: "hello 7", IsMatch: true},
			{Path: "ctx.txt", Line: 8, Text: "eight"},
		}, results)

		// The line after the last match counted for the file is context, even
		// though it contains the keyword.
		results, _, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResultsPerFile: 1, ContextLines: 4, Pathspecs: []string{"ctx.txt"}})
		require.NoError(t, err)
		require.Len(t, results, 7)
		assert.Equal(t, &GrepResult{Path: "ctx.txt", Line: 7, Text: "hello 7"}, results[6])

		// The lines before the match that is left out are left out with it.
		results, truncated, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResults: 1, ContextLines: 1, Pathspecs: []string{"ctx.txt"}})
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "ctx.txt", Line: 2, Text: "two"},
			{Path: "ctx.txt", Line: 3, Text: "hello 3", IsMatch: true},
			{Path: "ctx.txt", Line: 4, Text: "four"},
		}, results)
	})

	t.Run("pathspecs", func(t *testing.T) {
		results, _, err := r.Grep(ctx, commitID, "hello", GrepOptions{Pathspecs: []string{":(glob)**/*.go", "dir/"}})
		require.NoError(t, err)
		var paths []string
		for _, result := range results {
			paths = append(paths, result.Path)
		}
		assert.Equal(t, []string{"dir/a: b.txt", "dir/a: b.txt", "main.go"}, paths)
	})

	t.Run("limits", func(t *testing.T) {
		results, truncated, err := r.Grep(ctx, commitID, "hello", GrepOptions{MaxResultsPerFile: 1})
		require.NoError(t, err)
		assert.False(t, truncated)
		assert.Equal(t, []*GrepResult{
			{Path: "ctx.txt", Line: 3, Text: "hello 3", IsMatch: true},
			{Path: "dir/a: b.txt", Line: 1, Text: "Hello world", IsMatch: true},
			{Path: "main.go", Line: 3, Text: "hello", IsMatch: true},
		}, results)

		results, truncated, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResults: 2})
		require.NoError(t, err)
		assert.True(t, truncated)
		assert.Len(t, results, 2)

		_, truncated, err = r.Grep(ctx, commitID, "hello", GrepOptions{MaxResults: 5})
		require.NoError(t, err)
		assert.False(t, truncated)
	})
}
