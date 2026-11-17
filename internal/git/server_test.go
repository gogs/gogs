package git

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	t.Run("unsupported service", func(t *testing.T) {
		for _, service := range []Service{"version", "--version", "rev-parse"} {
			var stdout bytes.Buffer
			err := RunService(ctx, service, repoPath, ServiceOptions{Stdout: &stdout})
			assert.ErrorContains(t, err, "unsupported service", service)
			assert.Empty(t, stdout.String(), service)
		}
	})
}

func TestRunReceivePack(t *testing.T) {
	ctx := context.Background()

	// newRemote returns a repository whose update hook rejects one branch, so the
	// rest of the push goes on, and whose post-receive hook deletes another, like
	// a push that comes right after, so the references no longer show the update.
	newRemote := func(t *testing.T) string {
		remotePath := filepath.Join(t.TempDir(), "remote.git")
		require.NoError(t, Init(ctx, remotePath, InitOptions{Bare: true}))
		hooks := map[string]string{
			"update":       "#!/bin/sh\ntest \"$1\" != refs/heads/rejected\n",
			"post-receive": "#!/bin/sh\ngit update-ref -d refs/heads/moved 2>/dev/null\ntrue\n",
		}
		for name, content := range hooks {
			require.NoError(t, os.WriteFile(filepath.Join(remotePath, "hooks", name), []byte(content), 0o755))
		}
		return remotePath
	}

	// overHTTP pushes like the smart HTTP protocol, where Git runs stateless.
	overHTTP := func(t *testing.T, remotePath string) (string, func() []RefUpdate) {
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
		t.Cleanup(server.Close)
		return server.URL, func() []RefUpdate { return applied }
	}

	// overStream pushes like SSH, where Git first advertises the references on the
	// same stream. Git runs this test binary as its receive-pack, see TestMain.
	overStream := func(t *testing.T, remotePath string) (string, func() []RefUpdate) {
		exe, err := os.Executable()
		require.NoError(t, err)
		out := filepath.Join(t.TempDir(), "applied.json")
		t.Setenv("GOGS_TEST_RECEIVE_PACK_OUT", out)
		return "--receive-pack=" + exe + " " + remotePath, func() []RefUpdate {
			p, err := os.ReadFile(out)
			require.NoError(t, err)
			var applied []RefUpdate
			require.NoError(t, json.Unmarshal(p, &applied))
			return applied
		}
	}

	transports := map[string]func(t *testing.T, remotePath string) (string, func() []RefUpdate){
		"http":   overHTTP,
		"stream": overStream,
	}
	for name, transport := range transports {
		t.Run(name, func(t *testing.T) {
			remote, applied := transport(t, newRemote(t))
			localPath := t.TempDir()
			git := func(args ...string) string {
				args = append([]string{"-c", "user.name=alice", "-c", "user.email=alice@example.com"}, args...)
				stdout, _ := exec(ctx, localPath, args, nil)
				return strings.TrimSpace(string(stdout))
			}
			push := func(refspecs ...string) {
				args := []string{"push"}
				if receivePack, ok := strings.CutPrefix(remote, "--receive-pack="); ok {
					exe, remotePath, _ := strings.Cut(receivePack, " ")
					// Git runs the receive-pack through a shell, which would drop the
					// backslashes of a Windows path that is not quoted.
					args = append(args, "--receive-pack='"+exe+"'", remotePath)
				} else {
					args = append(args, remote)
				}
				git(append(args, refspecs...)...)
			}

			empty := ObjectFormatSHA1.EmptyID()
			git("init", "--initial-branch=main")
			git("commit", "--allow-empty", "-m", "first")
			first := git("rev-parse", "HEAD")
			git("branch", "rejected")
			git("branch", "moved")
			git("tag", "v1")

			push("main", "rejected", "moved", "v1")
			assert.ElementsMatch(t,
				[]RefUpdate{
					{OldID: empty, NewID: first, RefName: "refs/heads/main"},
					{OldID: empty, NewID: first, RefName: "refs/heads/moved"},
					{OldID: empty, NewID: first, RefName: "refs/tags/v1"},
				},
				applied(),
			)

			git("commit", "--allow-empty", "-m", "second")
			second := git("rev-parse", "HEAD")
			push("main", ":refs/tags/v1")
			assert.ElementsMatch(t,
				[]RefUpdate{
					{OldID: first, NewID: second, RefName: "refs/heads/main"},
					{OldID: first, NewID: empty, RefName: "refs/tags/v1"},
				},
				applied(),
			)
		})
	}
}

func TestAppliedRefUpdates(t *testing.T) {
	ctx := context.Background()
	// Looks up the references one at a time, so the result spans batches.
	oldBatchSize := refUpdatesBatchSize
	refUpdatesBatchSize = 1
	defer func() { refUpdatesBatchSize = oldBatchSize }()

	path := t.TempDir()
	git := func(args ...string) string {
		args = append([]string{"-c", "user.name=alice", "-c", "user.email=alice@example.com"}, args...)
		stdout, err := exec(ctx, path, args, nil)
		require.NoError(t, err)
		return strings.TrimSpace(string(stdout))
	}
	empty := ObjectFormatSHA1.EmptyID()
	git("init", "--initial-branch=main")
	git("commit", "--allow-empty", "-m", "first")
	first := git("rev-parse", "HEAD")
	git("branch", "missing/below")

	applied := RefUpdate{OldID: empty, NewID: first, RefName: "refs/heads/main"}
	deleted := RefUpdate{OldID: first, NewID: empty, RefName: "refs/heads/gone"}
	// "refs/heads/missing" matches "refs/heads/missing/below" as a pattern.
	notApplied := RefUpdate{OldID: empty, NewID: first, RefName: "refs/heads/missing"}
	got, err := appliedRefUpdates(ctx, path, []RefUpdate{applied, notApplied, deleted})
	require.NoError(t, err)
	assert.Equal(t, []RefUpdate{applied, deleted}, got)
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

func TestRunReceivePack_DeleteWithMissingOldObject(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "remote.git")
	require.NoError(t, Init(ctx, path, InitOptions{Bare: true}))
	stdout, err := exec(ctx, path, []string{"-c", "user.name=a", "-c", "user.email=a@example.com", "commit-tree", "-m", "first", ObjectFormatSHA1.EmptyTreeID()}, nil)
	require.NoError(t, err, string(stdout))
	first := strings.TrimSpace(string(stdout))
	stdout, err = exec(ctx, path, []string{"update-ref", "refs/heads/existing", first}, nil)
	require.NoError(t, err, string(stdout))

	// Git deletes an existing reference even when the old ID is not an object in
	// the repository, and reports the deletion of a missing reference without
	// passing it to the post-receive hook.
	missingOldID := strings.Repeat("ab", 20)
	empty := ObjectFormatSHA1.EmptyID()
	var request strings.Builder
	for i, name := range []string{"refs/heads/existing", "refs/heads/never-existed"} {
		command := missingOldID + " " + empty + " " + name
		if i == 0 {
			command += "\x00report-status"
		}
		_, _ = fmt.Fprintf(&request, "%04x%s", len(command)+4, command)
	}
	request.WriteString("0000")

	var response strings.Builder
	applied, err := RunReceivePack(ctx, path, ServiceOptions{
		StatelessRPC: true,
		Stdin:        strings.NewReader(request.String()),
		Stdout:       &response,
	})
	require.NoError(t, err)
	require.Contains(t, response.String(), "ok refs/heads/never-existed")
	assert.Equal(t, []RefUpdate{{OldID: missingOldID, NewID: empty, RefName: "refs/heads/existing"}}, applied)
}

func TestReceivePackPush_SidebandCapability(t *testing.T) {
	id := strings.Repeat("ab", 20)
	empty := ObjectFormatSHA1.EmptyID()
	command := id + " " + empty + " refs/heads/gone\x00report-status side-band"
	request := fmt.Sprintf("%04x%s0000", len(command)+4, command)
	// "git receive-pack" ignores "side-band" and sends the report as is.
	response := "000eunpack ok\n0017ok refs/heads/gone\n0000"

	push := &receivePackPush{}
	push.parseRequest([]byte(request))
	push.parseResponse([]byte(response))
	_, applied, reported := push.result()
	assert.True(t, reported)
	assert.Equal(t, []RefUpdate{{OldID: id, NewID: empty, RefName: "refs/heads/gone"}}, applied)
}
