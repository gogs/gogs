package integrationtests

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/session"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

var (
	// serverURL is the URL of the server under test, without a trailing slash.
	serverURL string
	// forcePrivateServerURL is the URL of another server under test, which only
	// differs by setting [repository] FORCE_PRIVATE and using the image captcha
	// for registration.
	forcePrivateServerURL string
	// portOverrideServerURL is the URL of another server under test, which is
	// started with `gogs web --port` while its HTTP_PORT is taken, and whose
	// external URL uses HTTP_PORT.
	portOverrideServerURL string
	// proxiedServerURL is the URL of a reverse proxy to another server under
	// test, which is started like the one at portOverrideServerURL and has the
	// URL of the proxy as its external URL.
	proxiedServerURL string
	// sshPort is the port of the builtin SSH server of the server under test.
	sshPort int
	// serverConfigPath is the configuration file of the server under test.
	serverConfigPath string
	// sessions reads the sessions the server under test stores in files.
	sessions session.Store
	logger   = logx.NewNoopLogger()
)

// TestMain builds Gogs, starts it with a configuration that only the tests use,
// and connects to its database, so tests seed data and read what the server
// stores, e.g., sessions and secrets, while they call it over HTTP.
func TestMain(m *testing.M) {
	if os.Getenv("GOGS_INTEGRATION_TEST") == "" {
		fmt.Println("Skipping integration tests, set GOGS_INTEGRATION_TEST=1 to run them")
		os.Exit(0)
	}

	dir, err := os.MkdirTemp("", "gogs-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create work directory:", err)
		os.Exit(1)
	}
	code, err := run(m, dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	if code == 0 {
		_ = os.RemoveAll(dir)
	} else {
		serverLog, _ := os.ReadFile(filepath.Join(dir, "server.log"))
		forcePrivateServerLog, _ := os.ReadFile(filepath.Join(dir, "server-force-private.log"))
		fmt.Fprintf(os.Stderr, "Server log:\n%s\nLog of the server with FORCE_PRIVATE:\n%s\nWork directory %q is left intact for inspection\n", serverLog, forcePrivateServerLog, dir)
	}
	os.Exit(code)
}

func run(m *testing.M, dir string) (int, error) {
	binary := filepath.Join(dir, "gogs")
	build := exec.Command("go", "build", "-o", binary, "gogs.io/gogs/cmd/gogs")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return 0, errors.Wrap(err, "build Gogs")
	}
	stopWebApp, err := startWebAppStub()
	if err != nil {
		return 0, errors.Wrap(err, "start web app stub")
	}
	defer stopWebApp()

	port, err := freePort()
	if err != nil {
		return 0, errors.Wrap(err, "find free port")
	}
	sshPort, err = freePort()
	if err != nil {
		return 0, errors.Wrap(err, "find free SSH port")
	}
	dbName, dropDatabase, err := createPostgresDatabase()
	if err != nil {
		return 0, errors.Wrap(err, "create Postgres database")
	}
	// Servers that share the database must share the key that encrypts what they
	// store.
	secretKey := make([]byte, 16)
	_, _ = rand.Read(secretKey)
	serverConfigPath = filepath.Join(dir, "app.ini")
	if err = writeConfig(dir, serverConfigPath, fmt.Sprintf("http://localhost:%d/", port), port, sshPort, dbName, hex.EncodeToString(secretKey), false, conf.AntiBotVerificationAltcha); err != nil {
		return 0, errors.Wrap(err, "write configuration")
	}

	// The server and this process resolve paths the same way, and neither
	// touches the Git configuration of the user running the tests.
	gitConfig := filepath.Join(dir, "gitconfig")
	if err = os.WriteFile(gitConfig, nil, 0o600); err != nil {
		return 0, errors.Wrap(err, "write Git configuration")
	}
	for k, v := range map[string]string{
		"GOGS_WORK_DIR":     dir,
		"GOGS_CUSTOM":       filepath.Join(dir, "custom"),
		"GIT_CONFIG_GLOBAL": gitConfig,
	} {
		if err = os.Setenv(k, v); err != nil {
			return 0, errors.Wrapf(err, "set %s", k)
		}
	}

	spans, err := newSpanReceiver()
	if err != nil {
		return 0, errors.Wrap(err, "start span receiver")
	}
	defer spans.close()
	// Matches the external URL, so URLs the server builds are the ones the tests
	// request.
	serverURL = fmt.Sprintf("http://localhost:%d", port)
	stopServer, err := startServer(binary, serverConfigPath, serverURL, filepath.Join(dir, "server.log"), spans.url)
	if err != nil {
		return 0, err
	}
	defer stopServer()

	// The server with FORCE_PRIVATE shares the database and the repositories, so
	// tests seed data once and call either server. It starts after the first one
	// has migrated the database and generated the SSH host keys.
	forcePrivatePort, err := freePort()
	if err != nil {
		return 0, errors.Wrap(err, "find free port")
	}
	forcePrivateSSHPort, err := freePort()
	if err != nil {
		return 0, errors.Wrap(err, "find free SSH port")
	}
	forcePrivateConfigPath := filepath.Join(dir, "app-force-private.ini")
	if err = writeConfig(dir, forcePrivateConfigPath, fmt.Sprintf("http://localhost:%d/", forcePrivatePort), forcePrivatePort, forcePrivateSSHPort, dbName, hex.EncodeToString(secretKey), true, conf.AntiBotVerificationCaptcha); err != nil {
		return 0, errors.Wrap(err, "write configuration")
	}
	forcePrivateServerURL = fmt.Sprintf("http://localhost:%d", forcePrivatePort)
	stopForcePrivateServer, err := startServer(binary, forcePrivateConfigPath, forcePrivateServerURL, filepath.Join(dir, "server-force-private.log"), spans.url)
	if err != nil {
		return 0, errors.Wrap(err, "start server with FORCE_PRIVATE")
	}
	defer stopForcePrivateServer()

	// The servers started with `gogs web --port` share the database and the
	// repositories as well. They start before the tests, because a server that
	// starts delivers pending webhooks and cleans up temporary data, which tests
	// that run meanwhile use.
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.Wrap(err, "listen on the port to take")
	}
	defer func() { _ = taken.Close() }()
	takenPort := taken.Addr().(*net.TCPAddr).Port
	startPortOverrideServer := func(name, externalURL string) (listenURL string, stop func(), _ error) {
		port, err := freePort()
		if err != nil {
			return "", nil, errors.Wrap(err, "find free port")
		}
		sshPort, err := freePort()
		if err != nil {
			return "", nil, errors.Wrap(err, "find free SSH port")
		}
		configPath := filepath.Join(dir, "app-"+name+".ini")
		if err = writeConfig(dir, configPath, externalURL, takenPort, sshPort, dbName, hex.EncodeToString(secretKey), false, conf.AntiBotVerificationAltcha); err != nil {
			return "", nil, errors.Wrap(err, "write configuration")
		}
		listenURL = fmt.Sprintf("http://localhost:%d", port)
		stop, err = startServer(binary, configPath, listenURL, filepath.Join(dir, "server-"+name+".log"), spans.url, "--port", strconv.Itoa(port))
		if err != nil {
			return "", nil, errors.Wrapf(err, "start server %s", name)
		}
		return listenURL, stop, nil
	}
	var stopPortOverrideServer func()
	portOverrideServerURL, stopPortOverrideServer, err = startPortOverrideServer("port-override", fmt.Sprintf("http://localhost:%d/", takenPort))
	if err != nil {
		return 0, err
	}
	defer stopPortOverrideServer()

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, errors.Wrap(err, "listen for reverse proxy")
	}
	proxiedServerURL = fmt.Sprintf("http://localhost:%d", proxyListener.Addr().(*net.TCPAddr).Port)
	proxiedListenURL, stopProxiedServer, err := startPortOverrideServer("proxied", proxiedServerURL+"/")
	if err != nil {
		_ = proxyListener.Close()
		return 0, err
	}
	defer stopProxiedServer()
	proxyTarget, err := url.Parse(proxiedListenURL)
	if err != nil {
		_ = proxyListener.Close()
		return 0, errors.Wrap(err, "parse reverse proxy target")
	}
	proxy := &http.Server{Handler: httputil.NewSingleHostReverseProxy(proxyTarget)}
	go func() { _ = proxy.Serve(proxyListener) }()
	defer func() { _ = proxy.Close() }()

	// Git hooks of seeded repositories call the binary at conf.AppPath(), which
	// is read from the first argument, so they call the server binary rather
	// than this test binary.
	os.Args[0] = binary
	if err = Init(logger, serverConfigPath); err != nil {
		return 0, errors.Wrap(err, "init")
	}
	sessions, err = session.FileIniter()(context.Background(),
		session.FileConfig{
			RootDir:  conf.Session.ProviderConfig,
			Lifetime: time.Duration(conf.Session.MaxLifeTime) * time.Second,
		},
		// Tests only read sessions, which never writes their IDs.
		session.IDWriter(func(http.ResponseWriter, *http.Request, string) {}),
	)
	if err != nil {
		return 0, errors.Wrap(err, "create session store")
	}
	code := m.Run()
	stopServer()
	stopForcePrivateServer()
	stopPortOverrideServer()
	stopProxiedServer()
	// Only a full run is held to covering every route. A run of some tests,
	// e.g., with -run, lists the routes left for information.
	if code == 0 {
		err = checkRouteCoverage(spans.spanNames())
		if err != nil && flag.Lookup("test.run").Value.String() == "" {
			return 1, err
		} else if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	// The database of a failed run is left for inspection, like the work
	// directory.
	if code == 0 {
		if err = dropDatabase(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to drop database:", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Database %q is left intact for inspection\n", dbName)
	}
	return code, nil
}

// startWebAppStub serves a minimal page shell where a development build of Gogs
// expects the Vite dev server, so the 404 and 500 pages the server renders
// through the web app have their status codes, without building the web app.
// When the port is taken, e.g., by `moon run web:dev`, the server uses what
// listens there.
func startWebAppStub() (stop func(), _ error) {
	l, err := net.Listen("tcp", "127.0.0.1:5173")
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return func() {}, nil
		}
		return nil, errors.Wrap(err, "listen")
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>{{.WebContext}}</head><body><div id="root"></div></body></html>`))
	})}
	go func() { _ = server.Serve(l) }()
	return func() { _ = server.Close() }, nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// writeConfig writes the configuration of a server under test, which uses the
// Postgres database of the name on the server the PG* environment variables
// point to.
func writeConfig(dir, path, externalURL string, port, sshPort int, dbName, secretKey string, forcePrivate bool, antiBotVerification string) error {
	u, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "get current user")
	}

	database := fmt.Sprintf(`TYPE = postgres
HOST = %s:%s
NAME = %s
USER = %s
PASSWORD = %s
SSL_MODE = %s`, os.Getenv("PGHOST"), os.Getenv("PGPORT"), dbName, os.Getenv("PGUSER"), os.Getenv("PGPASSWORD"), os.Getenv("PGSSLMODE"))

	config := fmt.Sprintf(`RUN_USER = %s
RUN_MODE = dev

[server]
; WebAuthn takes the host of the external URL as the relying party ID, which
; cannot be an IP address, so passkeys only work with a domain name.
EXTERNAL_URL = %[10]s
DOMAIN = 127.0.0.1
HTTP_ADDR = 127.0.0.1
HTTP_PORT = %[2]d
; Tests clone and push over the builtin SSH server. Rewriting the
; authorized_keys file, e.g., by an admin operation, writes to the work
; directory rather than the one of the user running the tests.
SSH_ROOT_PATH = %[6]s
START_SSH_SERVER = true
SSH_LISTEN_HOST = 127.0.0.1
SSH_PORT = %[7]d

[repository]
ROOT = %[3]s
FORCE_PRIVATE = %[8]t

[database]
%[4]s

[security]
SECRET_KEY = %[5]s
; Tests send webhooks to and migrate from servers on the loopback interface.
LOCAL_NETWORK_ALLOWLIST = 127.0.0.1,localhost

[session]
PROVIDER = file

; Mail goes nowhere because port 1 refuses connections. Tests read the codes mailed,
; e.g., to reset a password, from the database.
[email]
ENABLED = true
HOST = 127.0.0.1:1

[auth]
DISABLE_REGISTRATION = false
ANTI_BOT_VERIFICATION = %[9]s
; Tests call from the loopback interface, which is a trusted proxy by default,
; so only requests with the header are authenticated by it.
ENABLE_REVERSE_PROXY_AUTHENTICATION = true
ENABLE_REVERSE_PROXY_AUTO_REGISTRATION = true

[log]
MODE = console
LEVEL = Info
`, u.Username, port, filepath.Join(dir, "repositories"), database, secretKey, filepath.Join(dir, "ssh"), sshPort, forcePrivate, antiBotVerification, externalURL)
	return os.WriteFile(path, []byte(config), 0o600)
}

// createPostgresDatabase creates a database with a unique name on the server
// the PG* environment variables point to. The returned function drops it.
func createPostgresDatabase() (name string, drop func() error, _ error) {
	dsn, err := postgresDSN("")
	if err != nil {
		return "", nil, errors.Wrap(err, "new DSN")
	}
	exec := func(query string, args ...any) error {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return errors.Wrap(err, "open")
		}
		defer func() { _ = db.Close() }()
		_, err = db.Exec(query, args...)
		return err
	}

	name = UniqueName("gogs_integration_")
	if err = exec(fmt.Sprintf("CREATE DATABASE %q", name)); err != nil {
		return "", nil, errors.Wrap(err, "create database")
	}
	drop = func() error {
		// The test process still holds connections to the database.
		err := exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", name)
		if err != nil {
			return errors.Wrap(err, "terminate connections")
		}
		return exec(fmt.Sprintf("DROP DATABASE %q", name))
	}
	return name, drop, nil
}

// postgresDSN returns the DSN of the database of the name, or of the default
// database when the name is empty, on the server the PG* environment variables
// point to.
func postgresDSN(name string) (string, error) {
	return dbx.NewDSN(conf.DatabaseOpts{
		Type:     "postgres",
		Host:     os.ExpandEnv("$PGHOST:$PGPORT"),
		Name:     name,
		Schema:   "public",
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
		SSLMode:  os.Getenv("PGSSLMODE"),
	})
}

// startServer starts the server with the configuration and the extra arguments
// of `gogs web`, which logs to the file and exports spans to the URL, and waits
// until it serves on the URL. The returned function stops it, and does nothing
// once it has.
func startServer(binary, configPath, url, logPath, spansURL string, args ...string) (stop func(), _ error) {
	serverLog, err := os.Create(logPath)
	if err != nil {
		return nil, errors.Wrap(err, "create server log")
	}
	server := exec.Command(binary, append([]string{"web", "--config", configPath}, args...)...)
	server.Env = append(os.Environ(), "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="+spansURL)
	server.Stdout = serverLog
	server.Stderr = serverLog
	if err = server.Start(); err != nil {
		_ = serverLog.Close()
		return nil, errors.Wrap(err, "start server")
	}
	exited := make(chan error, 1)
	go func() { exited <- server.Wait() }()
	stopped := false
	// The server exports the spans it has left when it stops.
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		_ = server.Process.Signal(os.Interrupt)
		select {
		case err := <-exited:
			// Gogs stops cleanly on an interrupt, so an error means it crashed.
			if err != nil {
				fmt.Fprintln(os.Stderr, "Server exited with error:", err)
			}
		case <-time.After(10 * time.Second):
			_ = server.Process.Kill()
		}
		_ = serverLog.Close()
	}
	if err = waitForServer(url, exited); err != nil {
		stop()
		return nil, err
	}
	return stop, nil
}

func waitForServer(url string, exited <-chan error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/healthcheck", nil)
		if err != nil {
			return errors.Wrap(err, "new request")
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case err := <-exited:
			return errors.Wrap(err, "server exited before it was ready")
		case <-ctx.Done():
			return errors.New("server not ready within a minute")
		case <-time.After(200 * time.Millisecond):
		}
	}
}
