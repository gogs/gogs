package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

func TestBind(t *testing.T) {
	f := flamego.New()
	f.Use(func(c flamego.Context) {
		c.Map((*database.User)(nil), RequestInfo{Path: "/"})
	})
	f.Use(apiContexter)
	f.Post("/", bind(createRepoRequest{}), func(c *apiContext, form createRepoRequest) {
		c.PlainText(http.StatusOK, form.Name+" "+form.Description)
	})

	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantBody    string
	}{
		{
			name:       "empty content type",
			body:       `{"name":"r1"}`,
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:        "unsupported content type",
			contentType: "text/plain",
			body:        `{"name":"r1"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "malformed JSON",
			contentType: "application/json",
			body:        `{"name":`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing required field",
			contentType: "application/json",
			body:        `{"description":"d"}`,
			wantStatus:  http.StatusUnprocessableEntity,
		},
		{
			name:        "invalid characters",
			contentType: "application/json",
			body:        `{"name":"r 1"}`,
			wantStatus:  http.StatusUnprocessableEntity,
		},
		{
			name:        "exceeds max length in runes",
			contentType: "application/json",
			body:        `{"name":"r1","description":"` + strings.Repeat("é", 256) + `"}`,
			wantStatus:  http.StatusUnprocessableEntity,
		},
		{
			name:        "max length in runes",
			contentType: "application/json",
			body:        `{"name":"r1","description":"` + strings.Repeat("é", 255) + `"}`,
			wantStatus:  http.StatusOK,
			wantBody:    "r1 " + strings.Repeat("é", 255),
		},
		{
			name:        "form uses snake case keys",
			contentType: "application/x-www-form-urlencoded",
			body:        "name=r1&description=d&auto_init=on",
			wantStatus:  http.StatusOK,
			wantBody:    "r1 d",
		},
		{
			name:        "form with unparsable bool",
			contentType: "application/x-www-form-urlencoded",
			body:        "name=r1&private=maybe",
			wantStatus:  http.StatusUnprocessableEntity,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			require.NoError(t, err)
			if test.contentType != "" {
				req.Header.Set("Content-Type", test.contentType)
			}

			resp := httptest.NewRecorder()
			f.ServeHTTP(resp, req)

			assert.Equal(t, test.wantStatus, resp.Code)
			if test.wantBody != "" {
				assert.Equal(t, test.wantBody, resp.Body.String())
			}
		})
	}
}
