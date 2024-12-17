// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/mocks"
)

func Test_validateWebhook(t *testing.T) {
	l := &mocks.Locale{
		MockLang: "en",
		MockTr: func(s string, _ ...any) string {
			return s
		},
	}

	tests := []struct {
		name      string
		actor     *database.User
		webhook   *database.Webhook
		expField  string
		expMsg    string
		expStatus int
	}{
		{
			name:      "admin bypass local address check",
			webhook:   &database.Webhook{URL: "https://www.google.com"},
			expStatus: http.StatusOK,
		},

		{
			name:      "local address not allowed",
			webhook:   &database.Webhook{URL: "http://localhost:3306"},
			expField:  "PayloadURL",
			expMsg:    "repo.settings.webhook.url_resolved_to_blocked_local_address",
			expStatus: http.StatusForbidden,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, msg, status := validateWebhook(l, test.webhook)
			assert.Equal(t, test.expStatus, status)
			assert.Equal(t, test.expMsg, msg)
			assert.Equal(t, test.expField, field)
		})
	}
}
