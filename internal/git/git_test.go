package git

import (
	"bytes"
	"context"
	"encoding/json"
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
	// TestRunReceivePack has Git run this binary as its receive-pack.
	if out := os.Getenv("GOGS_TEST_RECEIVE_PACK_OUT"); out != "" {
		runReceivePackForTest(out)
		return
	}

	flag.Parse()

	if testing.Verbose() {
		SetOutput(os.Stdout)
	}

	ctx := context.Background()

	// Set up the test repository.
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

// runReceivePackForTest serves the push for the repository in the last argument
// over stdin and stdout, and writes the applied updates to out as JSON.
func runReceivePackForTest(out string) {
	applied, err := RunReceivePack(context.Background(), os.Args[len(os.Args)-1], ServiceOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		stdlog.Fatal(err)
	}
	p, err := json.Marshal(applied)
	if err != nil {
		stdlog.Fatal(err)
	}
	if err = os.WriteFile(out, p, 0o644); err != nil {
		stdlog.Fatal(err)
	}
}
