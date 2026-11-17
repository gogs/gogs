package integrationtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-macaron/session"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

var (
	// serverURL is the URL of the server under test, without a trailing slash.
	serverURL string
	// sessions reads the sessions the server under test stores in files.
	sessions *session.Manager
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
		fmt.Fprintf(os.Stderr, "Server log:\n%s\nWork directory %q is left intact for inspection\n", serverLog, dir)
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
	dbName, dropDatabase, err := createPostgresDatabase()
	if err != nil {
		return 0, errors.Wrap(err, "create Postgres database")
	}
	configPath := filepath.Join(dir, "app.ini")
	if err = writeConfig(dir, configPath, port, dbName); err != nil {
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

	serverLog, err := os.Create(filepath.Join(dir, "server.log"))
	if err != nil {
		return 0, errors.Wrap(err, "create server log")
	}
	defer func() { _ = serverLog.Close() }()
	spans, err := newSpanReceiver()
	if err != nil {
		return 0, errors.Wrap(err, "start span receiver")
	}
	defer spans.close()
	server := exec.Command(binary, "web", "--config", configPath)
	server.Env = append(os.Environ(), "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="+spans.url)
	server.Stdout = serverLog
	server.Stderr = serverLog
	if err = server.Start(); err != nil {
		return 0, errors.Wrap(err, "start server")
	}
	exited := make(chan error, 1)
	go func() { exited <- server.Wait() }()
	stopped := false
	// The server exports the spans it has left when it stops.
	stopServer := func() {
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
	}
	defer stopServer()

	// Matches the external URL, so URLs the server builds are the ones the tests
	// request.
	serverURL = fmt.Sprintf("http://localhost:%d", port)
	if err = waitForServer(exited); err != nil {
		return 0, err
	}

	// Git hooks of seeded repositories call the binary at conf.AppPath(), which
	// is read from the first argument, so they call the server binary rather
	// than this test binary.
	os.Args[0] = binary
	if err = Init(logger, configPath); err != nil {
		return 0, errors.Wrap(err, "init")
	}
	sessions, err = session.NewManager("file", session.Options{
		ProviderConfig: conf.Session.ProviderConfig,
		Maxlifetime:    conf.Session.MaxLifeTime,
		IDLength:       16,
	})
	if err != nil {
		return 0, errors.Wrap(err, "create session manager")
	}
	code := m.Run()
	stopServer()
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

// writeConfig writes the configuration of the server under test, which uses the
// Postgres database of the name on the server the PG* environment variables
// point to.
func writeConfig(dir, path string, port int, dbName string) error {
	u, err := user.Current()
	if err != nil {
		return errors.Wrap(err, "get current user")
	}
	secretKey := make([]byte, 16)
	_, _ = rand.Read(secretKey)

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
EXTERNAL_URL = http://localhost:%[2]d/
DOMAIN = 127.0.0.1
HTTP_ADDR = 127.0.0.1
HTTP_PORT = %[2]d
OFFLINE_MODE = true
; Tests add SSH keys, which are written to the authorized_keys file in the work
; directory rather than the one of the user running the tests. The SSH server
; does not start.
SSH_ROOT_PATH = %[6]s

[repository]
ROOT = %[3]s

[database]
%[4]s

[security]
SECRET_KEY = %[5]s
; Tests send webhooks to and migrate from servers on the loopback interface.
LOCAL_NETWORK_ALLOWLIST = 127.0.0.1,localhost

[session]
PROVIDER = file

; Mail goes nowhere, port 1 refuses connections. Tests read the codes mailed,
; e.g., to reset a password, from the database.
[email]
ENABLED = true
HOST = 127.0.0.1:1

[auth]
DISABLE_REGISTRATION = false
ENABLE_REGISTRATION_CAPTCHA = true

[log]
MODE = console
LEVEL = Info
`, u.Username, port, filepath.Join(dir, "repositories"), database, hex.EncodeToString(secretKey), filepath.Join(dir, "ssh"))
	return os.WriteFile(path, []byte(config), 0o600)
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
	exec := func(query string) error {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return errors.Wrap(err, "open")
		}
		defer func() { _ = db.Close() }()
		_, err = db.Exec(query)
		return err
	}

	name = UniqueName("gogs_integration_")
	if err = exec(fmt.Sprintf("CREATE DATABASE %q", name)); err != nil {
		return "", nil, errors.Wrap(err, "create database")
	}
	// The test process still holds connections to the database.
	return name, func() error { return exec(fmt.Sprintf("DROP DATABASE %q WITH (FORCE)", name)) }, nil
}

func waitForServer(exited <-chan error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/healthcheck", nil)
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
