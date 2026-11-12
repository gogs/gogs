package git

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSHA256TestRepo returns a SHA-256 copy of the test repository. It rewrites
// the history with "git fast-export" and "git fast-import", so the copy has the
// same commits, trees, blobs and tags, but with SHA-256 IDs.
func newSHA256TestRepo(t *testing.T) *Repository {
	t.Helper()
	ctx := context.Background()

	dir := t.TempDir()
	path := filepath.Join(dir, "testrepo-sha256.git")
	err := Init(ctx, path, InitOptions{Bare: true, ObjectFormat: ObjectFormatSHA256})
	require.NoError(t, err)

	history, err := exec(ctx, testrepo.path, []string{"fast-export", "--all"}, nil)
	require.NoError(t, err)

	// The submodule commit is in another repository, so "git fast-import" needs
	// to be told its SHA-256 ID.
	fromMarks := filepath.Join(dir, "from-marks")
	require.NoError(t, os.WriteFile(fromMarks, []byte(":1 6b08f76a5313fa3d26859515b30aa17a5faa2807\n"), 0o600))
	toMarks := filepath.Join(dir, "to-marks")
	require.NoError(t, os.WriteFile(toMarks, []byte(":1 8fa8e7ed491f06aa84c52730587547852f91e8d1b0b9105660209f253edd2c41\n"), 0o600))
	_, err = execInput(ctx, path,
		[]string{
			"fast-import",
			"--rewrite-submodules-from=gogs/docs-api:" + fromMarks,
			"--rewrite-submodules-to=gogs/docs-api:" + toMarks,
		},
		nil,
		bytes.NewReader(history),
	)
	require.NoError(t, err)

	r, err := Open(path)
	require.NoError(t, err)
	return r
}

func TestSHA256Repository(t *testing.T) {
	ctx := context.Background()
	r := newSHA256TestRepo(t)

	// The expected IDs are from https://github.com/gogs/git-module/pull/140, which
	// converted the test repository the same way.
	const (
		headCommitID    = "949e74b2fd94b95da3492e613656fc7fda5f7a6e82184ea291a89bd2b028dd2e" // cfc3b29
		parentCommitID  = "e8983d273da9a8236d8566fdbf83a34499b2d5e89e5e60d8b3537d6cd1946026" // d58e3ef
		tagCommitID     = "d4043c6c6c5b313187a7ccfc0bc8f17dbe725374aa03a81bd6b49994e33f77bd" // 0eedd79
		tagID           = "264607f7e6128c06d1bab93857dbaf10c1ad98cd98ade780a9adcfd388774056" // v1.1.0
		readmeBlobID    = "dd66873b93ef9e7f3909bb919f45bbd91540d85ea171043f21b4f0d830e404bb"
		gogsTreeID      = "ba2cc18e595b5f70a47cea3ed71f3568addd55e7bb5581a8918b7885abb7888e"
		submoduleCommit = "8fa8e7ed491f06aa84c52730587547852f91e8d1b0b9105660209f253edd2c41"
	)

	t.Run("ObjectFormat", func(t *testing.T) {
		got, err := r.ObjectFormat(ctx)
		require.NoError(t, err)
		assert.Equal(t, ObjectFormatSHA256, got)
	})

	t.Run("RevParse", func(t *testing.T) {
		got, err := r.RevParse(ctx, "master")
		require.NoError(t, err)
		assert.Equal(t, headCommitID, got)
	})

	t.Run("CatFileCommit", func(t *testing.T) {
		c, err := r.CatFileCommit(ctx, "master")
		require.NoError(t, err)
		assert.Equal(t, headCommitID, c.ID.String())
		assert.Equal(t, "Add files with same SHA", c.Summary())

		parentID, err := c.ParentID(0)
		require.NoError(t, err)
		assert.Equal(t, parentCommitID, parentID.String())

		parent, err := c.Parent(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, parentCommitID, parent.ID.String())
	})

	t.Run("Entries", func(t *testing.T) {
		c, err := r.CatFileCommit(ctx, "master")
		require.NoError(t, err)

		entries, err := c.Entries(ctx)
		require.NoError(t, err)
		got := make(map[string]string, len(entries))
		for _, e := range entries {
			got[e.Name()] = e.ID().String()
		}
		assert.Equal(t, readmeBlobID, got["README.txt"])
		assert.Equal(t, gogsTreeID, got["gogs"])

		blob, err := c.Blob(ctx, "README.txt")
		require.NoError(t, err)
		assert.Equal(t, readmeBlobID, blob.ID().String())
	})

	t.Run("Submodule", func(t *testing.T) {
		c, err := r.CatFileCommit(ctx, "master")
		require.NoError(t, err)

		mod, err := c.Submodule(ctx, "gogs/docs-api")
		require.NoError(t, err)
		assert.Equal(t, submoduleCommit, mod.Commit)
	})

	t.Run("Tag", func(t *testing.T) {
		tag, err := r.Tag(ctx, "v1.1.0")
		require.NoError(t, err)
		assert.Equal(t, ObjectTag, tag.Type())
		assert.Equal(t, tagID, tag.ID().String())
		assert.Equal(t, tagCommitID, tag.CommitID().String())

		c, err := tag.Commit(ctx)
		require.NoError(t, err)
		assert.Equal(t, tagCommitID, c.ID.String())
	})

	t.Run("Diff", func(t *testing.T) {
		diff, err := r.Diff(ctx, headCommitID, 0, 0, 0)
		require.NoError(t, err)
		require.NotEmpty(t, diff.Files)
		for _, f := range diff.Files {
			assert.Len(t, f.Index, 64, f.Name)
		}
	})
}
