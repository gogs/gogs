package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthcheckTarget(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthcheck" {
			http.NotFound(w, r)
		}
	})
	port := func(t *testing.T, s *httptest.Server) int {
		_, p, err := net.SplitHostPort(s.Listener.Addr().String())
		require.NoError(t, err)
		n, err := strconv.Atoi(p)
		require.NoError(t, err)
		return n
	}
	get := func(t *testing.T, client *http.Client, url string) {
		resp, err := client.Get(url)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	plain := httptest.NewServer(handler)
	t.Cleanup(plain.Close)
	plainPort := strconv.Itoa(port(t, plain))
	// Dialing a wildcard address reaches local listeners on Linux, so the URL is
	// checked as well.
	for addr, host := range map[string]string{
		"":          "127.0.0.1",
		"0.0.0.0":   "127.0.0.1",
		"[::]":      "127.0.0.1",
		"127.0.0.1": "127.0.0.1",
		"localhost": "localhost",
	} {
		t.Run("http with address "+strconv.Quote(addr), func(t *testing.T) {
			client, url, err := healthcheckTarget("http", addr, port(t, plain))
			require.NoError(t, err)
			assert.Equal(t, "http://"+host+":"+plainPort+"/healthcheck", url)
			get(t, client, url)
		})
	}

	t.Run("http with an IPv6 address", func(t *testing.T) {
		listener, err := net.Listen("tcp", "[::1]:0")
		if err != nil {
			t.Skip("IPv6 loopback is not available:", err)
		}
		s := httptest.NewUnstartedServer(handler)
		s.Listener = listener
		s.Start()
		t.Cleanup(s.Close)

		client, url, err := healthcheckTarget("http", "[::1]", port(t, s))
		require.NoError(t, err)
		assert.Equal(t, "http://[::1]:"+strconv.Itoa(port(t, s))+"/healthcheck", url)
		get(t, client, url)
	})

	t.Run("https with a self-signed certificate", func(t *testing.T) {
		s := httptest.NewTLSServer(handler)
		t.Cleanup(s.Close)
		client, url, err := healthcheckTarget("https", "0.0.0.0", port(t, s))
		require.NoError(t, err)
		get(t, client, url)
	})

	t.Run("unix", func(t *testing.T) {
		socket := filepath.Join(t.TempDir(), "gogs.sock")
		listener, err := net.Listen("unix", socket)
		require.NoError(t, err)
		s := httptest.NewUnstartedServer(handler)
		s.Listener = listener
		s.Start()
		t.Cleanup(s.Close)

		client, url, err := healthcheckTarget("unix", socket, 0)
		require.NoError(t, err)
		get(t, client, url)
	})

	t.Run("fcgi", func(t *testing.T) {
		_, _, err := healthcheckTarget("fcgi", "", 3000)
		assert.EqualError(t, err, `health check is not supported for server protocol "fcgi"`)
	})
}
