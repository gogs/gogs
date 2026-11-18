package integrationtests

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

// healthcheckRun runs "gogs healthcheck" without flags, like the HEALTHCHECK of
// the Docker images, with the configuration of the server under test as the
// custom configuration, with the given keys replaced. It returns the output and
// the error of the command.
func healthcheckRun(t *testing.T, replace map[string]string) (string, error) {
	t.Helper()

	config, err := os.ReadFile(conf.CustomConf)
	require.NoError(t, err)
	for key, value := range replace {
		pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + ` = .*$`)
		require.True(t, pattern.Match(config), "configuration has no %s", key)
		config = pattern.ReplaceAll(config, []byte(key+" = "+value))
	}
	customDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(customDir, "conf"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(customDir, "conf", "app.ini"), config, 0o600))

	// TestMain points the first argument to the binary of the server.
	cmd := exec.Command(os.Args[0], "healthcheck")
	cmd.Env = append(os.Environ(), "GOGS_CUSTOM="+customDir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestHealthcheck(t *testing.T) {
	t.Run("healthy server at the configured address", func(t *testing.T) {
		out, err := healthcheckRun(t, nil)
		assert.NoError(t, err, out)
	})

	t.Run("wildcard address reaches the server on loopback", func(t *testing.T) {
		out, err := healthcheckRun(t, map[string]string{"HTTP_ADDR": "0.0.0.0"})
		assert.NoError(t, err, out)
	})

	// The Docker image runs the health check as root while RUN_USER is "git".
	t.Run("run user other than the current user", func(t *testing.T) {
		out, err := healthcheckRun(t, map[string]string{"RUN_USER": "gogs-healthcheck-nobody"})
		assert.NoError(t, err, out)
	})

	t.Run("nothing listening at the configured port", func(t *testing.T) {
		port, err := freePort()
		require.NoError(t, err)
		out, err := healthcheckRun(t, map[string]string{"HTTP_PORT": strconv.Itoa(port)})
		require.Error(t, err)
		assert.Contains(t, out, "connection refused")
	})

	t.Run("server responding other than OK", func(t *testing.T) {
		unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "database is down", http.StatusServiceUnavailable)
		}))
		t.Cleanup(unhealthy.Close)
		_, port, err := net.SplitHostPort(unhealthy.Listener.Addr().String())
		require.NoError(t, err)

		out, err := healthcheckRun(t, map[string]string{"HTTP_PORT": port})
		require.Error(t, err)
		assert.Contains(t, out, "503 Service Unavailable: database is down")
	})
}
