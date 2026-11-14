package git

import (
	"bytes"
	"context"
	"flag"
	stdlog "log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoPath = "testdata/testrepo.git"

var testrepo *Repository

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Verbose() {
		SetOutput(os.Stdout)
	}

	ctx := context.Background()

	// Set up the test repository
	if !isDir(repoPath) {
		if err := Clone(ctx, "https://github.com/gogs/git-module-testrepo.git", repoPath, CloneOptions{
			Bare: true,
		}); err != nil {
			stdlog.Fatal(err)
		}
	}

	var err error
	testrepo, err = Open(repoPath)
	if err != nil {
		stdlog.Fatal(err)
	}

	os.Exit(m.Run())
}

func TestLog(t *testing.T) {
	old := logOutput
	defer SetOutput(old)

	tests := []struct {
		format    string
		args      []any
		expOutput string
	}{
		{
			format:    "",
			expOutput: "[git] \n",
		},
		{
			format:    "something",
			expOutput: "[git] something\n",
		},
		{
			format:    "val: %v",
			args:      []any{123},
			expOutput: "[git] val: 123\n",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			var buf bytes.Buffer
			SetOutput(&buf)

			logf(test.format, test.args...)
			assert.Equal(t, test.expOutput, buf.String())
		})
	}
}

func TestConfig(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))

	got, err := ConfigGet(ctx, "gogs.test")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, SetGlobalConfig(ctx, "gogs.test", "some value"))
	got, err = ConfigGet(ctx, "gogs.test")
	require.NoError(t, err)
	assert.Equal(t, "some value", got)
}
