package repo

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/mocks"
)

func TestValidateWebhook(t *testing.T) {
	l := &mocks.Locale{
		MockLang: "en",
		MockTr: func(s string, _ ...any) string {
			return s
		},
	}

	tests := []struct {
		name       string
		actor      *database.User
		webhook    *database.Webhook
		wantField  string
		wantMsg    string
		wantStatus int
	}{
		{
			name:       "admin bypass local address check",
			webhook:    &database.Webhook{URL: "https://www.google.com"},
			wantStatus: http.StatusOK,
		},

		{
			name:       "local address not allowed",
			webhook:    &database.Webhook{URL: "http://localhost:3306"},
			wantField:  "PayloadURL",
			wantMsg:    "repo.settings.webhook.url_resolved_to_blocked_local_address",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, msg, status := validateWebhook(l, test.webhook)
			assert.Equal(t, test.wantStatus, status)
			assert.Equal(t, test.wantMsg, msg)
			assert.Equal(t, test.wantField, field)
		})
	}
}
