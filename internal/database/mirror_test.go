package database

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			assert.Equal(t, test.expResults, parseRemoteUpdateOutput(test.output))
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
