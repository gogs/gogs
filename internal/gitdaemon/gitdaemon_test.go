package gitdaemon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func pktLine(payload string) string {
	return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}

func TestReadPacketLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expErr    bool
		expOutput string
	}{
		{
			name:      "normal request",
			input:     pktLine("git-upload-pack /alice/repo.git\x00"),
			expOutput: "git-upload-pack /alice/repo.git\x00",
		},
		{
			name:   "truncated length",
			input:  "003",
			expErr: true,
		},
		{
			name:   "non-hex length",
			input:  "zzzz" + pktLine("git-upload-pack /a/b.git\x00"),
			expErr: true,
		},
		{
			name:   "zero length",
			input:  "0000",
			expErr: true,
		},
		{
			name:   "declared length shorter than pkt-line header",
			input:  "0004" + pktLine("git-upload-pack /a/b.git\x00"),
			expErr: true,
		},
		{
			name:   "oversized length",
			input:  "ffff" + pktLine("git-upload-pack /a/b.git\x00"),
			expErr: true,
		},
		{
			name:   "truncated payload",
			input:  pktLine("git-upload-pack /alice/repo.git\x00")[:10],
			expErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := readPacketLine(bytes.NewReader([]byte(test.input)))
			if test.expErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expOutput, string(got))
		})
	}
}

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		expErr      bool
		expService  string
		expPath     string
		expOwner    string
		expRepo     string
		expWiki     bool
		expProtocol string
	}{
		{
			name:       "upload-pack with host parameter",
			payload:    "git-upload-pack /Alice/Repo.git\x00host=example.com\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.git",
			expOwner:   "alice",
			expRepo:    "repo",
		},
		{
			name:        "upload-pack with protocol version",
			payload:     "git-upload-pack /alice/repo.git\x00host=example.com\x00\x00version=1\x00",
			expProtocol: "version=1",
			expService:  "git-upload-pack",
			expPath:     "alice/repo.git",
			expOwner:    "alice",
			expRepo:     "repo",
		},
		{
			name:        "upload-pack Git protocol v2",
			payload:     "git-upload-pack /alice/repo.git\x00host=example.com\x00\x00version=2\x00",
			expService:  "git-upload-pack",
			expPath:     "alice/repo.git",
			expOwner:    "alice",
			expRepo:     "repo",
			expProtocol: "version=2",
		},
		{
			name:        "upload-pack Git protocol v2 without host parameter",
			payload:     "git-upload-pack /alice/repo.git\x00\x00version=2\x00",
			expService:  "git-upload-pack",
			expPath:     "alice/repo.git",
			expOwner:    "alice",
			expRepo:     "repo",
			expProtocol: "version=2",
		},
		{
			name:       "unrecognized protocol extra is ignored",
			payload:    "git-upload-pack /alice/repo.git\x00host=example.com\x00\x00GIT_PROTOCOL=version=2\x00version=3\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.git",
			expOwner:   "alice",
			expRepo:    "repo",
		},
		{
			name:       "unseparated version in host is ignored",
			payload:    "git-upload-pack /alice/repo.git\x00host=example.com\x00version=2\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.git",
			expOwner:   "alice",
			expRepo:    "repo",
		},
		{
			name:       "upload-archive",
			payload:    "git-upload-archive /alice/repo.git\x00",
			expService: "git-upload-archive",
			expPath:    "alice/repo.git",
			expOwner:   "alice",
			expRepo:    "repo",
		},
		{
			name:       "wiki repository",
			payload:    "git-upload-pack /alice/repo.wiki.git\x00host=example.com\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.wiki.git",
			expOwner:   "alice",
			expRepo:    "repo",
			expWiki:    true,
		},
		{
			name:       "wiki repository without .git suffix",
			payload:    "git-upload-pack /alice/repo.wiki\x00host=example.com\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.wiki.git",
			expOwner:   "alice",
			expRepo:    "repo",
			expWiki:    true,
		},
		{
			name:       "mixed-case wiki repository",
			payload:    "git-upload-pack /Alice/Repo.WIKI.git\x00host=example.com\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.wiki.git",
			expOwner:   "alice",
			expRepo:    "repo",
			expWiki:    true,
		},
		{
			name:       "path without .git suffix",
			payload:    "git-upload-pack /alice/repo\x00host=example.com\x00",
			expService: "git-upload-pack",
			expPath:    "alice/repo.git",
			expOwner:   "alice",
			expRepo:    "repo",
		},
		{
			name:    "empty wiki repository name",
			payload: "git-upload-pack /alice/.wiki.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "receive-pack is rejected",
			payload: "git-receive-pack /alice/repo.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "unknown service",
			payload: "git-whatever /alice/repo.git\x00",
			expErr:  true,
		},
		{
			name:    "missing path",
			payload: "git-upload-pack\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "missing NUL terminator",
			payload: "git-upload-pack /alice/repo.git",
			expErr:  true,
		},

		{
			name:    "path traversal",
			payload: "git-upload-pack /../../secret.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "dot segment",
			payload: "git-upload-pack /./repo.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "extra path segments",
			payload: "git-upload-pack /a/b/c.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "empty owner",
			payload: "git-upload-pack //repo.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "backslash traversal",
			payload: "git-upload-pack /..\\secret.git\x00host=example.com\x00",
			expErr:  true,
		},
		{
			name:    "empty payload",
			payload: "",
			expErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseRequest([]byte(test.payload))
			if test.expErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expService, got.service)
			assert.Equal(t, test.expPath, got.path)
			assert.Equal(t, test.expOwner, got.owner)
			assert.Equal(t, test.expRepo, got.repo)
			assert.Equal(t, test.expWiki, got.wiki)
			assert.Equal(t, test.expProtocol, got.gitProtocol)
		})
	}
}

func TestAnonymousGitReadPolicy(t *testing.T) {
	tests := []struct {
		name          string
		repo          database.Repository
		wiki          bool
		requireSignin bool
		want          bool
	}{
		{"public repository", database.Repository{}, false, false, true},
		{"public repository despite externally hosted wiki", database.Repository{EnableExternalWiki: true}, false, false, true},
		{"enabled internal wiki", database.Repository{EnableWiki: true}, true, false, true},
		{"disabled wiki", database.Repository{EnableWiki: false}, true, false, false},
		{"externally hosted wiki", database.Repository{EnableWiki: true, EnableExternalWiki: true}, true, false, false},
		{"private repository", database.Repository{IsPrivate: true}, false, false, false},
		{"private wiki", database.Repository{IsPrivate: true, EnableWiki: true}, true, false, false},
		{"sign-in required for repository", database.Repository{}, false, true, false},
		{"sign-in required for wiki", database.Repository{EnableWiki: true}, true, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canServeAnonymousGit(&tc.repo, tc.wiki, tc.requireSignin))
		})
	}
}

func TestSendError(t *testing.T) {
	var buf bytes.Buffer
	sendError(&buf, "repository not found")

	// ERR message is sent as a single pkt-line: "<4-hex-len>ERR <msg>\n".
	got := buf.String()
	payload := fmt.Sprintf("ERR %s\n", "repository not found")
	assert.Equal(t, fmt.Sprintf("%04x%s", len(payload)+4, payload), got)
}

func TestGitSessionLimits(t *testing.T) {
	maxConnections, timeout := gitSessionLimits(conf.GitProtocolOpts{})
	assert.Equal(t, 32, maxConnections)
	assert.Equal(t, 15*time.Minute, timeout)

	maxConnections, timeout = gitSessionLimits(conf.GitProtocolOpts{
		MaxConnections:        2,
		SessionTimeoutSeconds: 45,
	})
	assert.Equal(t, 2, maxConnections)
	assert.Equal(t, 45*time.Second, timeout)
}

func TestGitSessionDeadlines(t *testing.T) {
	now := time.Unix(123, 0)

	// A configured timeout shorter than the ordinary handshake cap owns the
	// whole connection lifetime, including the initial pkt-line.
	handshake, session := gitSessionDeadlines(now, 5*time.Second)
	assert.Equal(t, now.Add(5*time.Second), handshake)
	assert.Equal(t, now.Add(5*time.Second), session)

	// Longer sessions retain the 30-second handshake cap, but both deadlines
	// remain anchored to the same connection start time.
	handshake, session = gitSessionDeadlines(now, 45*time.Second)
	assert.Equal(t, now.Add(30*time.Second), handshake)
	assert.Equal(t, now.Add(45*time.Second), session)
}

func TestGitAcceptRetryDelay(t *testing.T) {
	// Persistent accept errors must never hot-spin or grow sleep indefinitely.
	delay := time.Duration(0)
	for i := 0; i < 20; i++ {
		next := nextGitAcceptRetryDelay(delay)
		assert.Greater(t, next, time.Duration(0))
		assert.LessOrEqual(t, next, time.Second)
		if delay > 0 {
			assert.GreaterOrEqual(t, next, delay)
		}
		delay = next
	}
	assert.Equal(t, time.Second, delay)
	// The successful-Accept branch resets retryDelay to zero.
	assert.Equal(t, 5*time.Millisecond, nextGitAcceptRetryDelay(0))
}

func TestGitSessionAdmission(t *testing.T) {
	slots := make(chan struct{}, 1)
	assert.True(t, tryAdmitGitSession(slots))
	assert.False(t, tryAdmitGitSession(slots), "an occupied Git subprocess slot must reject new sessions")
	<-slots
	assert.True(t, tryAdmitGitSession(slots), "releasing a Git subprocess slot permits a new session")
}

type failedGitClientWriter struct{}

func (failedGitClientWriter) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestForwardGitOutputCancelsOnClientFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A network write failure must stop upload-pack promptly instead of
	// consuming a bounded anonymous Git session slot until its timeout.
	forwardGitOutput(failedGitClientWriter{}, strings.NewReader("git payload"), cancel)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)

	// A successful response does not cancel the Git process early; normal
	// Git client half-closes and completed upload-pack sessions stay valid.
	goodCtx, goodCancel := context.WithCancel(context.Background())
	defer goodCancel()
	var output bytes.Buffer
	forwardGitOutput(&output, strings.NewReader("git payload"), goodCancel)
	assert.NoError(t, goodCtx.Err())
	assert.Equal(t, "git payload", output.String())
}
