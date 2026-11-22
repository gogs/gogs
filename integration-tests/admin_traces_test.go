package integrationtests

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminTraces(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	// No other request reaches the overview before this one, so it is the first
	// of its route in the minute, which the server keeps. The server gives the
	// request a trace of its own, whatever trace the request carries.
	raw := make([]byte, 16)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	carriedTraceID := hex.EncodeToString(raw)
	admin.header.Set("Traceparent", "00-"+carriedTraceID+"-0123456789abcdef-01")
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
	var search struct {
		Total  int `json:"total"`
		Traces []struct {
			ID string `json:"id"`
		} `json:"traces"`
	}
	// The server records the trace once the request ended, which may be after
	// the client got the response.
	require.Eventually(t, func() bool {
		admin.doJSON(http.MethodGet, "/api/web/admin/traces?route="+url.QueryEscape("GET /api/web/admin/traces/overview"), nil, http.StatusOK, &search)
		return search.Total == 1
	}, 5*time.Second, 50*time.Millisecond, "trace of the request not kept")
	traceID := search.Traces[0].ID
	assert.NotEqual(t, carriedTraceID, traceID)
	status, body := admin.do(http.MethodGet, "/api/web/admin/traces/"+carriedTraceID, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	admin.doJSON(http.MethodGet, "/api/web/admin/traces/"+traceID, nil, http.StatusOK, &trace)
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

	admin.doJSON(http.MethodGet, "/api/web/admin/traces?q="+traceID[:8], nil, http.StatusOK, &search)
	require.Equal(t, 1, search.Total)
	assert.Equal(t, traceID, search.Traces[0].ID)

	status, body = admin.do(http.MethodGet, "/api/web/admin/traces/0123456789abcdef0123456789abcdef", nil)
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
		keys := make([]string, 0, len(health.Series))
		for _, series := range health.Series {
			keys = append(keys, series.Key)
		}
		// Measurements of the requests, the Go runtime and the server itself.
		assert.Subset(t, keys, []string{"requests", "failedRequests", "heap", "dbConnections", "hookQueue"})
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
