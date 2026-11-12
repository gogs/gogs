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
				Envs:          []string{"GIT_PROTOCOL=version=2"},
			},
		)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "version 2")
		assert.NotContains(t, stdout.String(), "refs/heads/master")
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
