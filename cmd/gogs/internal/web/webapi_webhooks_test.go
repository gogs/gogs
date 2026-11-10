package web

import (
	"net/http"
	"testing"

	"github.com/go-macaron/i18n"
	"github.com/stretchr/testify/assert"
	unknwoni18n "github.com/unknwon/i18n"
)

func TestValidateWebhookURL(t *testing.T) {
	l := i18n.Locale{Locale: unknwoni18n.Locale{Lang: "en-US"}}
	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{name: "public address", url: "https://www.google.com", wantStatus: http.StatusOK},
		{name: "local address", url: "http://localhost:3306", wantStatus: http.StatusForbidden},
		{name: "loopback address", url: "http://127.0.0.1:8080/hook", wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, _ := validateWebhookURL(l, test.url)
			assert.Equal(t, test.wantStatus, status)
		})
	}
}
