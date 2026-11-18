package integrationtests

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	neturl "net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
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
	// differs by setting [repository] FORCE_PRIVATE.
	forcePrivateServerURL string
	// requireSigninViewServerURL is the URL of another server under test, which
	// only differs by setting [auth] REQUIRE_SIGNIN_VIEW.
	requireSigninViewServerURL string
	// requireSigninViewSSHPort is the port of the builtin SSH server of the
	// server at requireSigninViewServerURL.
	requireSigninViewSSHPort int
	// subpathServerURL is the URL of another server under test, which is served
	// under subpath behind a reverse proxy that strips it, without a trailing
	// slash.
	subpathServerURL string
	// sshPort is the port of the builtin SSH server of the server under test.
	sshPort int
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
		logs, _ := filepath.Glob(filepath.Join(dir, "server*.log"))
		for _, path := range logs {
			serverLog, _ := os.ReadFile(path)
			fmt.Fprintf(os.Stderr, "%s:\n%s\n", filepath.Base(path), serverLog)
		}
		fmt.Fprintf(os.Stderr, "Work directory %q is left intact for inspection\n", dir)
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
	configPath := filepath.Join(dir, "app.ini")
	if err = writeConfig(dir, configPath, port, sshPort, dbName, hex.EncodeToString(secretKey), fmt.Sprintf("http://localhost:%d", port), serverOptions{}); err != nil {
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
	stopServer, err := startServer(binary, configPath, serverURL, filepath.Join(dir, "server.log"), spans.url)
	if err != nil {
		return 0, err
	}
	defer stopServer()

	// The variants share the database and the repositories, so tests seed data
	// once, through the configuration of the first server, and call any server.
	// They start after the first server has migrated the database and generated
	// the SSH host keys.
	var stopVariants []func()
	defer func() {
		for _, stop := range stopVariants {
			stop()
		}
	}()
	for _, v := range []struct {
		name    string
		opts    serverOptions
		url     *string
		sshPort *int
	}{
		{name: "force-private", opts: serverOptions{ForcePrivate: true}, url: &forcePrivateServerURL},
		{name: "require-signin-view", opts: serverOptions{RequireSigninView: true}, url: &requireSigninViewServerURL, sshPort: &requireSigninViewSSHPort},
		{name: "subpath", opts: serverOptions{Subpath: "/gogs"}, url: &subpathServerURL},
	} {
		url, sshPort, stop, err := startVariant(binary, dir, dbName, hex.EncodeToString(secretKey), spans.url, v.name, v.opts)
		if err != nil {
			return 0, errors.Wrapf(err, "start server %q", v.name)
		}
		stopVariants = append(stopVariants, stop)
		*v.url = url
		if v.sshPort != nil {
			*v.sshPort = sshPort
		}
	}

	// Git hooks of seeded repositories call the binary at conf.AppPath(), which
	// is read from the first argument, so they call the server binary rather
	// than this test binary.
	os.Args[0] = binary
	if err = Init(logger, configPath); err != nil {
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
	for _, stop := range stopVariants {
		stop()
	}
	stopVariants = nil
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

// serverOptions are how the configuration of a server under test differs from
// the default.
type serverOptions struct {
	ForcePrivate      bool
	RequireSigninView bool
	// Subpath is the path the server is served under, e.g., "/gogs", which the
	// reverse proxy in front of it strips.
	Subpath string
}

// writeConfig writes the configuration of a server under test, which uses the
// Postgres database of the name on the server the PG* environment variables
// point to, and is reached at the external URL, without a trailing slash.
func writeConfig(dir, path string, port, sshPort int, dbName, secretKey, externalURL string, opts serverOptions) error {
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
EXTERNAL_URL = %[9]s/
DOMAIN = 127.0.0.1
HTTP_ADDR = 127.0.0.1
HTTP_PORT = %[2]d
OFFLINE_MODE = true
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
REQUIRE_SIGNIN_VIEW = %[10]t
DISABLE_REGISTRATION = false
ENABLE_REGISTRATION_CAPTCHA = true

[log]
MODE = console
LEVEL = Info
`, u.Username, port, filepath.Join(dir, "repositories"), database, secretKey, filepath.Join(dir, "ssh"), sshPort, opts.ForcePrivate,
		externalURL, opts.RequireSigninView)
	return os.WriteFile(path, []byte(config), 0o600)
}

// startVariant starts a server under test with the options, which shares the
// database of the name and the work directory with the first server. It
// returns the URL where tests call the server, without a trailing slash, and the
// port of its builtin SSH server. A server with a subpath is behind a reverse
// proxy that strips the subpath, like deployments under a subpath.
func startVariant(binary, dir, dbName, secretKey, spansURL, name string, opts serverOptions) (url string, sshPort int, stop func(), _ error) {
	// The proxy listens before the ports of the server are picked, so they
	// cannot be the same.
	var proxyListener net.Listener
	if opts.Subpath != "" {
		var err error
		proxyListener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", 0, nil, errors.Wrap(err, "listen for reverse proxy")
		}
	}
	port, err := freePort()
	if err == nil {
		sshPort, err = freePort()
	}
	if err != nil {
		if proxyListener != nil {
			_ = proxyListener.Close()
		}
		return "", 0, nil, errors.Wrap(err, "find free port")
	}
	stopProxy := func() {}
	url = fmt.Sprintf("http://localhost:%d", port)
	if proxyListener != nil {
		stopProxy, err = startSubpathProxy(proxyListener, opts.Subpath, fmt.Sprintf("http://127.0.0.1:%d", port))
		if err != nil {
			_ = proxyListener.Close()
			return "", 0, nil, errors.Wrap(err, "start reverse proxy")
		}
		url = fmt.Sprintf("http://localhost:%d%s", proxyListener.Addr().(*net.TCPAddr).Port, opts.Subpath)
	}

	configPath := filepath.Join(dir, "app-"+name+".ini")
	if err = writeConfig(dir, configPath, port, sshPort, dbName, secretKey, url, opts); err != nil {
		stopProxy()
		return "", 0, nil, errors.Wrap(err, "write configuration")
	}
	stopServer, err := startServer(binary, configPath, url, filepath.Join(dir, "server-"+name+".log"), spansURL)
	if err != nil {
		stopProxy()
		return "", 0, nil, err
	}
	return url, sshPort, func() {
		stopServer()
		stopProxy()
	}, nil
}

// startSubpathProxy serves the target under the subpath on the listener, and
// strips the subpath before passing requests on, like the reverse proxy
// configurations in the documentation. Like the NGINX one, it sends the host
// of the target, so the server builds URLs from its external URL rather than
// from the Host header.
func startSubpathProxy(l net.Listener, subpath, target string) (stop func(), _ error) {
	targetURL, err := neturl.Parse(target)
	if err != nil {
		return nil, errors.Wrap(err, "parse target")
	}
	proxy := &httputil.ReverseProxy{
		// The proxy fails to connect until the server listens, while the tests
		// wait for it.
		ErrorLog: log.New(io.Discard, "", 0),
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(targetURL)
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, subpath)
			r.Out.URL.RawPath = strings.TrimPrefix(r.In.URL.RawPath, subpath)
			if r.Out.URL.Path == "" {
				r.Out.URL.Path = "/"
			}
		},
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != subpath && !strings.HasPrefix(r.URL.Path, subpath+"/") {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	})}
	go func() { _ = server.Serve(l) }()
	return func() { _ = server.Close() }, nil
}

// createPostgresDatabase creates a database with a unique name on the server
// the PG* environment variables point to. The returned function drops it.
func createPostgresDatabase() (name string, drop func() error, _ error) {
	dsn, err := dbx.NewDSN(conf.DatabaseOpts{
		Type:     "postgres",
		Host:     os.ExpandEnv("$PGHOST:$PGPORT"),
		Schema:   "public",
		User:     os.Getenv("PGUSER"),
		Password: os.Getenv("PGPASSWORD"),
		SSLMode:  os.Getenv("PGSSLMODE"),
	})
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

// startServer starts the server with the configuration, which logs to the file
// and exports spans to the URL, and waits until it serves on the URL. The
// returned function stops it, and does nothing once it has.
func startServer(binary, configPath, url, logPath, spansURL string) (stop func(), _ error) {
	serverLog, err := os.Create(logPath)
	if err != nil {
		return nil, errors.Wrap(err, "create server log")
	}
	server := exec.Command(binary, "web", "--config", configPath)
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
