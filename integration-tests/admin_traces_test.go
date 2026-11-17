package integrationtests

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminTraces(t *testing.T) {
	admin, nonAdmin, anonymous := adminClients(t)
	// The server continues the trace a request carries, so the test knows the ID
	// of the trace it looks for. No other request reaches the overview before
	// this one, so it is the first of its route in the minute, which the server
	// keeps.
	raw := make([]byte, 16)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	traceID := hex.EncodeToString(raw)
	admin.header.Set("Traceparent", "00-"+traceID+"-0123456789abcdef-01")
	var overview struct {
		Enabled  bool `json:"enabled"`
		Requests int  `json:"requests"`
	}
	admin.doJSON(http.MethodGet, "/api/web/admin/traces/overview", nil, http.StatusOK, &overview)
	admin.header.Del("Traceparent")
	assert.True(t, overview.Enabled)
	assert.Positive(t, overview.Requests)

	var trace struct {
		ID    string `json:"id"`
		Route struct {
			Name string `json:"name"`
		} `json:"route"`
		Status int `json:"status"`
		Spans  []struct {
			Kind string `json:"kind"`
		} `json:"spans"`
	}
	// The server records the trace once the request ended, which may be after
	// the client got the response.
	require.Eventually(t, func() bool {
		status, body := admin.do(http.MethodGet, "/api/web/admin/traces/"+traceID, nil)
		return status == http.StatusOK && json.Unmarshal(body, &trace) == nil
	}, 5*time.Second, 50*time.Millisecond, "trace %s not kept", traceID)
	assert.Equal(t, traceID, trace.ID)
	assert.Equal(t, "GET /api/web/admin/traces/overview", trace.Route.Name)
	assert.Equal(t, http.StatusOK, trace.Status)
	require.NotEmpty(t, trace.Spans)
	assert.Equal(t, "request", trace.Spans[0].Kind)
	// Signing in as the admin looked them up in the database.
	kinds := make(map[string]bool)
	for _, span := range trace.Spans {
		kinds[span.Kind] = true
	}
	assert.True(t, kinds["db"], "spans of the trace: %+v", trace.Spans)

	var search struct {
		Total  int `json:"total"`
		Traces []struct {
			ID string `json:"id"`
		} `json:"traces"`
	}
	admin.doJSON(http.MethodGet, "/api/web/admin/traces?q="+traceID[:8], nil, http.StatusOK, &search)
	require.Equal(t, 1, search.Total)
	assert.Equal(t, traceID, search.Traces[0].ID)

	status, body := admin.do(http.MethodGet, "/api/web/admin/traces/0123456789abcdef0123456789abcdef", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = admin.do(http.MethodGet, "/api/web/admin/traces?status=3xx", nil)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	for _, rangeName := range []string{"1h", "24h"} {
		var health struct {
			Range  string `json:"range"`
			Series []struct {
				Key string `json:"key"`
			} `json:"series"`
		}
		admin.doJSON(http.MethodGet, "/api/web/admin/health?range="+rangeName, nil, http.StatusOK, &health)
		assert.Equal(t, rangeName, health.Range)
		assert.NotEmpty(t, health.Series)
	}
	status, body = admin.do(http.MethodGet, "/api/web/admin/health?range=7d", nil)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	for _, path := range []string{
		"/api/web/admin/health",
		"/api/web/admin/traces",
		"/api/web/admin/traces/overview",
		"/api/web/admin/traces/0123456789abcdef0123456789abcdef",
	} {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, path, nil)
	}
}
