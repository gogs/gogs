package git

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateServerInfo(t *testing.T) {
	ctx := context.Background()
	err := os.RemoveAll(filepath.Join(repoPath, "info"))
	require.NoError(t, err)
	err = UpdateServerInfo(ctx, repoPath)
	require.NoError(t, err)
	assert.True(t, isFile(filepath.Join(repoPath, "info", "refs")))
}

func TestIsSupportedProtocol(t *testing.T) {
	tests := []struct {
		protocol string
		want     bool
	}{
		{protocol: "version=2", want: true},
		{protocol: "version=1", want: true},
		{protocol: ""},
		{protocol: "version=0"},
		{protocol: "version=3"},
		{protocol: "version=2:foo=bar"},
		{protocol: " version=2"},
		{protocol: "foo"},
	}
	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			assert.Equal(t, test.want, IsSupportedProtocol(test.protocol))
		})
	}
}

func TestRunService(t *testing.T) {
	ctx := context.Background()

	t.Run("advertise refs", func(t *testing.T) {
		for _, service := range []Service{ServiceUploadPack, ServiceReceivePack} {
			var stdout bytes.Buffer
			err := RunService(ctx, service, repoPath,
				ServiceOptions{
					StatelessRPC:  true,
					AdvertiseRefs: true,
					Stdout:        &stdout,
				},
			)
			require.NoError(t, err)
			assert.Contains(t, stdout.String(), "refs/heads/master", string(service))
		}
	})

	t.Run("environment variables", func(t *testing.T) {
		var stdout bytes.Buffer
		err := RunService(ctx, ServiceUploadPack, repoPath,
			ServiceOptions{
				StatelessRPC:  true,
				AdvertiseRefs: true,
				Stdout:        &stdout,
				Envs: []string{
					"GIT_CONFIG_COUNT=1",
					"GIT_CONFIG_KEY_0=uploadpack.allowFilter",
					"GIT_CONFIG_VALUE_0=true",
				},
			},
		)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), " filter")
	})

	t.Run("protocol", func(t *testing.T) {
		tests := []struct {
			name     string
			protocol string
			envs     []string
			inherit  string
			want     string
		}{
			{name: "v2", protocol: "version=2", want: "version 2"},
			{name: "v1", protocol: "version=1", want: "version 1"},
			{name: "empty", protocol: ""},
			{name: "v0", protocol: "version=0"},
			{name: "unknown version", protocol: "version=3"},
			{name: "extra keys", protocol: "version=2:foo=bar"},
			{name: "garbage", protocol: "foo"},
			{name: "inherited", inherit: "version=2"},
			{name: "through Envs", envs: []string{"GIT_PROTOCOL=version=2"}},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.inherit != "" {
					t.Setenv("GIT_PROTOCOL", test.inherit)
				}

				var stdout bytes.Buffer
				err := RunService(ctx, ServiceUploadPack, repoPath,
					ServiceOptions{
						StatelessRPC:  true,
						AdvertiseRefs: true,
						Stdout:        &stdout,
						Protocol:      test.protocol,
						Envs:          test.envs,
					},
				)
				require.NoError(t, err)
				if test.want != "" {
					assert.Contains(t, stdout.String(), test.want)
					return
				}
				assert.NotContains(t, stdout.String(), "version ")
				assert.Contains(t, stdout.String(), "refs/heads/master")
			})
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		var stderr bytes.Buffer
		err := RunService(ctx, ServiceUploadPack, filepath.Join(t.TempDir(), "404.git"),
			ServiceOptions{
				StatelessRPC:  true,
				AdvertiseRefs: true,
				Stderr:        &stderr,
			},
		)
		require.Error(t, err)
		assert.Contains(t, stderr.String(), "does not appear to be a git repository")
	})
}

func TestRunService_IdleTimeout(t *testing.T) {
	old := serviceIdleTimeout
	serviceIdleTimeout = 200 * time.Millisecond
	defer func() { serviceIdleTimeout = old }()

	// The client never sends anything after the advertisement.
	stdin, w := io.Pipe()
	defer func() { _ = w.Close() }()

	start := time.Now()
	err := RunService(context.Background(), ServiceUploadPack, repoPath, ServiceOptions{
		Stdin:  stdin,
		Stdout: io.Discard,
	})
	assert.Equal(t, ErrExecTimeout, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
