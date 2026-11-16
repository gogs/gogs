package git

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepository_Blame(t *testing.T) {
	ctx := context.Background()

	type hunk struct {
		commit    string
		startLine int
		lines     int
	}
	// The first commit owns two separate runs of lines, with another commit's
	// lines between them.
	want := []hunk{
		{commit: "755fd577edcfd9209d0ac072eed3b022cbe4d39b", startLine: 1, lines: 2},
		{commit: "a13dba1e469944772490909daa58c53ac8fa4b0d", startLine: 3, lines: 2},
		{commit: "755fd577edcfd9209d0ac072eed3b022cbe4d39b", startLine: 5, lines: 8},
		{commit: "8d2636da55da593c421e1cb09eea502a05556a69", startLine: 13, lines: 3},
	}

	hunks, err := testrepo.Blame(ctx, "master", "README.txt")
	require.NoError(t, err)
	require.Len(t, hunks, len(want))
	for i, w := range want {
		assert.Equal(t, w.commit, hunks[i].Commit.ID.String(), "hunk %d", i)
		assert.Equal(t, w.startLine, hunks[i].StartLine, "hunk %d", i)
		assert.Len(t, hunks[i].Lines, w.lines, "hunk %d", i)
	}
	assert.Same(t, hunks[0].Commit, hunks[2].Commit)
	assert.Equal(t, "Here is an addition by me", hunks[1].Lines[0])
	assert.Equal(t, "", hunks[1].Lines[1])

	commit := hunks[1].Commit
	assert.Equal(t, "Even I can change the readme file", commit.Summary)
	assert.Equal(t, "Peter Bell", commit.Author.Name)
	assert.Equal(t, "peter@pbell.com", commit.Author.Email)
	assert.True(t, commit.Author.When.Equal(time.Unix(1371584078, 0)))
	_, offset := commit.Author.When.Zone()
	assert.Equal(t, -4*3600, offset)
	require.NotNil(t, commit.PreviousID)
	assert.Equal(t, "8d80474ffc3747e3375651cc2a0032c8599befd3", commit.PreviousID.String())
	assert.Equal(t, "README.txt", commit.PreviousPath)

	// The first commit of the file has nothing before it.
	assert.Nil(t, hunks[0].Commit.PreviousID)

	t.Run("SHA-256", func(t *testing.T) {
		hunks, err := newSHA256TestRepo(t).Blame(ctx, "master", "README.txt")
		require.NoError(t, err)
		require.Len(t, hunks, len(want))
		for i, w := range want {
			assert.Len(t, hunks[i].Commit.ID.String(), 64, "hunk %d", i)
			assert.Equal(t, w.startLine, hunks[i].StartLine, "hunk %d", i)
		}
		assert.Len(t, hunks[1].Commit.PreviousID.String(), 64)
	})

	t.Run("path does not exist", func(t *testing.T) {
		_, err := testrepo.Blame(ctx, "master", "404.txt")
		assert.Equal(t, ErrRevisionNotExist, err)
	})
}

func TestParseBlame_QuotedPreviousPath(t *testing.T) {
	data := []byte(`a13dba1e469944772490909daa58c53ac8fa4b0d 1 1 1
author Peter Bell
summary Rename
previous 8d80474ffc3747e3375651cc2a0032c8599befd3 "caf\303\251 menu.txt"
filename "caf\303\251.txt"
	espresso
`)
	hunks, err := parseBlame(data)
	require.NoError(t, err)
	require.Len(t, hunks, 1)
	assert.Equal(t, "café menu.txt", hunks[0].Commit.PreviousPath)
	assert.Equal(t, []string{"espresso"}, hunks[0].Lines)
}
