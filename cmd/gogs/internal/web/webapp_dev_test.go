//go:build !prod

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewViteReverseProxy(t *testing.T) {
	viteURL, err := url.Parse("http://localhost:5173")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "https://example.test:3000/src/main.tsx", nil)
	proxy := newViteReverseProxy(viteURL)
	proxy.Director(req)

	assert.Equal(t, "http", req.URL.Scheme)
	assert.Equal(t, "localhost:5173", req.URL.Host)
	assert.Equal(t, "localhost:5173", req.Host)
	assert.Equal(t, "/src/main.tsx", req.URL.Path)
}
