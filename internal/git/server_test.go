package git

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRunReceivePack(t *testing.T) {
	ctx := context.Background()

	remotePath := filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, Init(ctx, remotePath, InitOptions{Bare: true}))
	// Rejects only the update of this one branch, so the rest of the push goes on.
	err := os.WriteFile(filepath.Join(remotePath, "hooks", "update"),
		[]byte("#!/bin/sh\ntest \"$1\" != refs/heads/rejected\n"), 0o755)
	require.NoError(t, err)

	var applied []RefUpdate
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info/refs") {
			w.Header().Set("Content-Type", "application/x-git-receive-pack-advertisement")
			_, _ = w.Write([]byte("001f# service=git-receive-pack\n0000"))
			assert.NoError(t, RunService(ctx, ServiceReceivePack, remotePath, ServiceOptions{
				StatelessRPC:  true,
				AdvertiseRefs: true,
				Stdout:        w,
			}))
			return
		}

		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		var err error
		applied, err = RunReceivePack(ctx, remotePath, ServiceOptions{
			StatelessRPC: true,
			Stdin:        r.Body,
			Stdout:       w,
		})
		assert.NoError(t, err)
	}))
	defer server.Close()

	localPath := t.TempDir()
	git := func(args ...string) string {
		args = append([]string{"-c", "user.name=alice", "-c", "user.email=alice@example.com"}, args...)
		stdout, _ := exec(ctx, localPath, args, nil)
		return strings.TrimSpace(string(stdout))
	}
	empty := ObjectFormatSHA1.EmptyID()
	git("init", "--initial-branch=main")
	git("commit", "--allow-empty", "-m", "first")
	first := git("rev-parse", "HEAD")
	git("branch", "rejected")
	git("tag", "v1")

	git("push", server.URL, "main", "rejected", "v1")
	assert.ElementsMatch(t,
		[]RefUpdate{
			{OldID: empty, NewID: first, RefName: "refs/heads/main"},
			{OldID: empty, NewID: first, RefName: "refs/tags/v1"},
		},
		applied,
	)

	git("commit", "--allow-empty", "-m", "second")
	second := git("rev-parse", "HEAD")
	git("push", server.URL, "main", ":refs/tags/v1")
	assert.ElementsMatch(t,
		[]RefUpdate{
			{OldID: first, NewID: second, RefName: "refs/heads/main"},
			{OldID: first, NewID: empty, RefName: "refs/tags/v1"},
		},
		applied,
	)
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
