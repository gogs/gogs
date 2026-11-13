package database

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/logx"
)

func TestParseRemoteUpdateOutput(t *testing.T) {
	tests := []struct {
		output     string
		expResults []*mirrorSyncResult
	}{
		{
			`
From https://try.gogs.io/unknwon/upsteam
 * [new branch]      develop    -> develop
   b0bb24f..1d85a4f  master     -> master
 - [deleted]         (none)     -> bugfix
`,
			[]*mirrorSyncResult{
				{"develop", gitShortEmptyID, ""},
				{"master", "b0bb24f", "1d85a4f"},
				{"bugfix", "", gitShortEmptyID},
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expResults, parseRemoteUpdateOutput(logx.NewNoop(), test.output))
		})
	}
}

func TestMirrorGitEnv(t *testing.T) {
	gitConfigGet := func(t *testing.T, key string) string {
		cmd := exec.Command("git", "config", "--get", key)
		cmd.Env = append(os.Environ(), mirrorGitEnv()...)
		output, err := cmd.Output()
		require.NoError(t, err, key)
		return strings.TrimSpace(string(output))
	}

	t.Run("no inherited configuration", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_COUNT", "")
		assert.Equal(t, "false", gitConfigGet(t, "http.followRedirects"))
	})

	t.Run("keeps inherited configuration", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_COUNT", "1")
		t.Setenv("GIT_CONFIG_KEY_0", "http.proxy")
		t.Setenv("GIT_CONFIG_VALUE_0", "http://proxy.example.com:3128")
		assert.Equal(t, "false", gitConfigGet(t, "http.followRedirects"))
		assert.Equal(t, "http://proxy.example.com:3128", gitConfigGet(t, "http.proxy"))
	})
}

func TestRemoteDefaultBranch(t *testing.T) {
	git := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}

	source := t.TempDir()
	git(t, source, "init", "--initial-branch=trunk")
	git(t, source, "-c", "user.name=alice", "-c", "user.email=alice@example.com", "commit", "--allow-empty", "--message=initial")
	git(t, source, "branch", "release/v1")
	git(t, source, "symbolic-ref", "HEAD", "refs/heads/release/v1")

	got, err := remoteDefaultBranch(time.Minute, source)
	require.NoError(t, err)
	assert.Equal(t, "release/v1", got)
}
