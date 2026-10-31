// Package gitdaemon provides a built-in Git protocol (git://) server that
// allows anonymous cloning of public repositories, the same way as the
// standalone "git daemon" command does.
package gitdaemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

// allowedServices are read-only Git services permitted for anonymous access.
var allowedServices = map[string]bool{
	"git-upload-pack":    true,
	"git-upload-archive": true,
}

// Protocol handshake and anonymous Git subprocesses need independent limits.
const (
	maxRequestLineLen        = 1024
	defaultMaxGitConnections = 32
	defaultGitSessionTimeout = 15 * time.Minute
)

// gitSessionLimits ensures callers with zero-valued options also have limits.
// Operators can override these defaults in the [server] configuration.
func gitSessionLimits(opts conf.GitProtocolOpts) (int, time.Duration) {
	maxConnections := opts.MaxConnections
	if maxConnections <= 0 {
		maxConnections = defaultMaxGitConnections
	}
	timeout := time.Duration(opts.SessionTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultGitSessionTimeout
	}
	return maxConnections, timeout
}

// gitSessionDeadlines anchors every connection to one overall wall-clock
// lifetime. The handshake keeps its stricter 30-second cap without ever
// outliving the configured session timeout.
func gitSessionDeadlines(now time.Time, sessionTimeout time.Duration) (time.Time, time.Time) {
	sessionDeadline := now.Add(sessionTimeout)
	handshakeDeadline := now.Add(30 * time.Second)
	if sessionDeadline.Before(handshakeDeadline) {
		handshakeDeadline = sessionDeadline
	}
	return handshakeDeadline, sessionDeadline
}

// tryAdmitGitSession never blocks accept while all backend process slots are occupied.
func tryAdmitGitSession(slots chan struct{}) bool {
	select {
	case slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// request is a parsed Git protocol service request.
type request struct {
	service     string // e.g. "git-upload-pack"
	path        string // repository path relative to the repository root, e.g. "owner/repo.git"
	owner       string // lowercased owner name for database lookup
	repo        string // lowercased repository name for database lookup
	wiki        bool   // whether the request targets the repository's wiki
	gitProtocol string // explicitly requested, supported protocol negotiation (v1 or v2)
}

// readPacketLine reads a single pkt-line from r and returns its payload.
func readPacketLine(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, errors.Wrap(err, "read pkt-line length")
	}
	n, err := strconv.ParseUint(string(lenBuf[:]), 16, 16)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid pkt-line length %q", string(lenBuf[:]))
	}
	if n <= 4 || n > maxRequestLineLen {
		return nil, errors.Errorf("pkt-line length %d out of range", n)
	}
	payload := make([]byte, n-4)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, errors.Wrap(err, "read pkt-line payload")
	}
	return payload, nil
}

// packetWrite returns str encoded as a pkt-line.
func packetWrite(str string) string {
	return fmt.Sprintf("%04x%s", len(str)+4, str)
}

// sendError reports a protocol error to the client using the ERR pkt-line
// convention used by "git daemon".
func sendError(w io.Writer, msg string) {
	_, _ = io.WriteString(w, packetWrite(fmt.Sprintf("ERR %s\n", msg)))
}

// requestedGitProtocol reads only the wire-defined "version" parameter, which
// comes after the host field and an empty NUL-delimited separator. Never pass
// arbitrary client-provided environment variables or unrecognized parameters
// through to Git. Git protocol v2 requires GIT_PROTOCOL=version=2 for upload-pack.
func requestedGitProtocol(payload []byte) string {
	parts := strings.Split(string(payload), "\x00")
	if len(parts) < 3 {
		return ""
	}

	// The host parameter is optional. Extra parameters are introduced by an
	// empty NUL-delimited field whether or not host= was sent.
	extraStart := 1
	if parts[extraStart] != "" {
		if !strings.HasPrefix(parts[extraStart], "host=") {
			return ""
		}
		extraStart++
	}
	if extraStart >= len(parts)-1 || parts[extraStart] != "" {
		return ""
	}
	extraStart++

	for _, part := range parts[extraStart : len(parts)-1] {
		if part == "version=1" || part == "version=2" {
			return part
		}
	}
	return ""
}

// parseRequest parses the initial service request of the Git protocol, which
// has the form "<service> <path>\x00<extra-parameters>...", e.g.
// "git-upload-pack /owner/repo.git\x00host=example.com\x00".
func parseRequest(payload []byte) (*request, error) {
	// Repository request precedes NUL-separated host and optional protocol version.
	// The pathname terminator is mandatory in the git transport grammar; without
	// it the client has not sent a complete service request.
	line, _, foundTerminator := strings.Cut(string(payload), "\x00")
	if !foundTerminator {
		return nil, errors.New("service request missing NUL terminator")
	}

	fields := strings.Fields(line)
	if len(fields) != 2 {
		return nil, errors.Errorf("malformed service request %q", line)
	}
	service, repoPath := fields[0], fields[1]
	if !allowedServices[service] {
		return nil, errors.Errorf("service %q is not allowed", service)
	}

	// The path must have the form "<owner>/<repo>.git" (or "<owner>/<repo>.wiki.git"
	// for wikis). Resolve the repository name and reject anything else,
	// including traversal attempts.
	repoPath = strings.ToLower(strings.TrimPrefix(repoPath, "/"))
	name := strings.TrimSuffix(repoPath, ".git")
	fields = strings.Split(name, "/")
	if len(fields) != 2 || strings.ContainsAny(repoPath, "\\") {
		return nil, errors.Errorf("invalid repository path %q", repoPath)
	}
	ownerName, repoName := fields[0], fields[1]
	repoName, wiki := strings.CutSuffix(repoName, ".wiki")
	if ownerName == "" || repoName == "" || ownerName == "." || repoName == "." ||
		strings.HasPrefix(ownerName, "..") || strings.HasPrefix(repoName, "..") {
		return nil, errors.Errorf("invalid repository path %q", repoPath)
	}

	// Hand Git a canonical path built from the validated names instead of the
	// client-supplied one. Git resolves "<repo>.wiki" to "<repo>.wiki.git" by
	// itself, so passing the raw path through would serve a wiki that was not
	// checked against the wiki access policy.
	gitPath := ownerName + "/" + repoName
	if wiki {
		gitPath += ".wiki"
	}
	gitPath += ".git"

	return &request{
		service:     service,
		path:        gitPath,
		owner:       ownerName,
		repo:        repoName,
		wiki:        wiki,
		gitProtocol: requestedGitProtocol(payload),
	}, nil
}

// canServeAnonymousGit enforces the repository visibility policy and prevents
// wiki Git URLs from bypassing disabled or externally hosted wiki settings.
func canServeAnonymousGit(repo *database.Repository, wiki, requireSignin bool) bool {
	if repo.IsPrivate || requireSignin {
		return false
	}
	return !wiki || (repo.EnableWiki && !repo.EnableExternalWiki)
}

// handleConn serves a single Git protocol connection.
func handleConn(conn net.Conn, sessionTimeout time.Duration) {
	defer func() {
		_ = conn.Close()
	}()
	// Bound handshake, database lookup, Git subprocess lifetime, and network
	// copies to one absolute session deadline. CommandContext kills Git if a
	// client stalls.
	now := time.Now()
	handshakeDeadline, sessionDeadline := gitSessionDeadlines(now, sessionTimeout)
	ctx, cancel := context.WithDeadline(context.Background(), sessionDeadline)
	defer cancel()
	remote := conn.RemoteAddr()
	log.Trace("Git protocol: connection from %s", remote)

	fail := func(msg string, err error) {
		if err != nil {
			log.Trace("Git protocol: %s [%s]: %v", msg, remote, err)
		}
		sendError(conn, msg)
	}

	// The client is expected to send the service request promptly. A short
	// configured session timeout is stricter than the ordinary 30-second cap.
	if err := conn.SetDeadline(handshakeDeadline); err != nil {
		log.Trace("Git protocol: failed to set handshake deadline [%s]: %v", remote, err)
		return
	}
	payload, err := readPacketLine(conn)
	if err != nil {
		fail("protocol error", err)
		return
	}
	req, err := parseRequest(payload)
	if err != nil {
		fail("no such repository", err)
		return
	}

	owner, err := database.Handle.Users().GetByUsername(ctx, req.owner)
	if err != nil {
		if !database.IsErrUserNotExist(err) {
			log.Error("Git protocol: failed to get owner %q: %v", req.owner, err)
		}
		fail("repository not found", nil)
		return
	}
	repo, err := database.Handle.Repositories().GetByName(ctx, owner.ID, req.repo)
	if err != nil {
		if !database.IsErrRepoNotExist(err) {
			log.Error("Git protocol: failed to get repository %q/%q: %v", req.owner, req.repo, err)
		}
		fail("repository not found", nil)
		return
	}

	// Public Git access must not expose a disabled or externally hosted wiki.
	if !canServeAnonymousGit(repo, req.wiki, conf.Auth.RequireSigninView) {
		fail("access denied", nil)
		return
	}

	// Reuse the original absolute deadline after authentication. Do not grant
	// the session a fresh timeout budget just because the handshake completed.
	if err := conn.SetDeadline(sessionDeadline); err != nil {
		log.Trace("Git protocol: failed to set session deadline [%s]: %v", remote, err)
		return
	}

	// Delegate the session to the Git backend, e.g. "git upload-pack owner/repo.git".
	cmd := exec.CommandContext(ctx, "git", strings.TrimPrefix(req.service, "git-"), req.path)
	cmd.Dir = conf.Repository.Root
	// Without this, Git always answers with the legacy advertisement even if
	// a modern git:// client requested protocol v2. Explicitly override any
	// inherited value, and forward only recognized, protocol-defined versions.
	if req.service == "git-upload-pack" {
		cmd.Env = append(os.Environ(), "GIT_PROTOCOL="+req.gitProtocol)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Error("Git protocol: StdinPipe: %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error("Git protocol: StdoutPipe: %v", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error("Git protocol: StderrPipe: %v", err)
		return
	}

	if err = cmd.Start(); err != nil {
		log.Error("Git protocol: failed to start %q [%s]: %v", req.service, remote, err)
		return
	}

	go func() {
		_, _ = io.Copy(stdin, conn)
		_ = stdin.Close()
	}()
	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()
	forwardGitOutput(conn, stdout, cancel)

	if err = cmd.Wait(); err != nil {
		log.Trace("Git protocol: %q exited for %s: %v", req.service, remote, err)
	}
}

// forwardGitOutput aborts Git when the client stops accepting output. A
// disconnected client must not pin a bounded session slot and upload-pack
// subprocess until the full session timeout. Successful transfers and
// ordinary client write-side half-closes do not cancel the process.
func forwardGitOutput(dst io.Writer, src io.Reader, cancel context.CancelFunc) {
	if _, err := io.Copy(dst, src); err != nil {
		cancel()
	}
}

// nextGitAcceptRetryDelay caps retries for persistent listener failures.
// Without a delay, exhausted file descriptors can spin and flood the log.
func nextGitAcceptRetryDelay(previous time.Duration) time.Duration {
	if previous <= 0 {
		return 5 * time.Millisecond
	}
	if previous >= time.Second/2 {
		return time.Second
	}
	return previous * 2
}

// Listen starts a Git protocol server listening on the given host and port.
func Listen(opts conf.GitProtocolOpts) {
	listener, err := net.Listen("tcp", net.JoinHostPort(opts.ListenHost, strconv.Itoa(opts.ListenPort)))
	if err != nil {
		log.Fatal("Git protocol: failed to start server: %v", err)
	}
	maxConnections, sessionTimeout := gitSessionLimits(opts)
	slots := make(chan struct{}, maxConnections)
	go func() {
		var retryDelay time.Duration
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					// An intentionally closed listener cannot accept again.
					return
				}
				retryDelay = nextGitAcceptRetryDelay(retryDelay)
				log.Error("Git protocol: error accepting incoming connection: %v; retrying in %s", err, retryDelay)
				// Avoid hot-spinning on repeated temporary/permanent accept errors.
				time.Sleep(retryDelay)
				continue
			}
			retryDelay = 0
			if !tryAdmitGitSession(slots) {
				// Close immediately: no unbounded rejection goroutines or
				// blocked error writes under a connection flood.
				_ = conn.Close()
				continue
			}
			go func() {
				defer func() { <-slots }()
				handleConn(conn, sessionTimeout)
			}()
		}
	}()
}
