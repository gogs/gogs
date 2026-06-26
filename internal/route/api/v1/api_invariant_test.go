package v1_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea.com/gitea/gitea/internal/route/api/v1"
	"gitea.com/gitea/gitea/modules/setting"
	"gitea.com/gitea/gitea/modules/test"
	"gitea.com/gitea/gitea/modules/web"
	"github.com/stretchr/testify/assert"
)

func TestProtectedEndpointsRejectUnauthenticatedRequests(t *testing.T) {
	// Setup test environment
	setting.AppURL = "http://localhost:3000/"
	setting.AppSubURL = ""

	// Initialize router with the actual API routes
	m := web.NewMacaron()
	v1.RegisterRoutes(m)

	// Test payloads: missing, malformed, and valid authentication headers
	payloads := []struct {
		name   string
		header string
	}{
		{"missing_auth", ""},                      // No Authorization header
		{"malformed_token", "Bearer invalid"},     // Wrong auth scheme
		{"empty_basic_auth", "Basic "},            // Empty credentials
		{"malformed_basic_auth", "Basic invalid"}, // Invalid base64
		{"valid_auth", "Basic dXNlcjpwYXNz"},      // Valid base64 "user:pass"
	}

	for _, payload := range payloads {
		t.Run(payload.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/user", nil)
			if payload.header != "" {
				req.Header.Set("Authorization", payload.header)
			}
			resp := httptest.NewRecorder()

			m.ServeHTTP(resp, req)

			// Assert that unauthenticated requests are rejected with 401 or 403
			assert.True(t, resp.Code == http.StatusUnauthorized || resp.Code == http.StatusForbidden,
				"Expected 401 or 403 for unauthenticated request, got %d", resp.Code)
		})
	}
}