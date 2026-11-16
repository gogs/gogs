package git

import (
	"context"
	"os"
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
			got := unescapeChars([]byte(tt.in))
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

	err := Init(ctx, path, InitOptions{Bare: true})
	require.NoError(t, err)
	repo, err := Open(path)
	require.NoError(t, err)

	// Names that "git ls-tree" would quote without the -z flag.
	specialNames := []string{`"Leading" quote.md`, `Test "Wiki" Page.md`}
	files := make(map[string]string)
	for _, name := range specialNames {
		files[name] = "content"
	}
	commitID := commitFiles(t, repo, "master", files)

	commit, err := repo.CatFileCommit(ctx, commitID)
	require.NoError(t, err)

	entries, err := commit.Entries(ctx)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, specialNames, names)
}

func TestRepository_TreePaths(t *testing.T) {
	ctx := context.Background()
	paths, err := testrepo.TreePaths(ctx, "master")
	require.NoError(t, err)

	// Files of nested trees are listed by their full paths, and the
	// submodule "gogs/docs-api" is left out.
	assert.Len(t, paths, 21)
	assert.Equal(t, ".DS_Store", paths[0])
	assert.Contains(t, paths, "src/main/java/com/github/App.java")
	assert.Contains(t, paths, "symlink.run.sh")
	assert.NotContains(t, paths, "gogs/docs-api")

	_, err = testrepo.TreePaths(ctx, "404")
	assert.Error(t, err)
}
