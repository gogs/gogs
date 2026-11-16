package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
)

func TestProviderAuthenticate(t *testing.T) {
	// Mimics GitHub, which identifies the user by the personal access token and
	// ignores the Basic auth username.
	tokenOwners := map[string]map[string]any{
		"alice-token":     {"login": "alice", "name": "Alice"},
		"uppercase-token": {"login": "Alice"},
		"no-login-token":  {"name": "Nobody"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, password, _ := r.BasicAuth()
		owner, ok := tokenOwners[password]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(owner)
	}))
	defer server.Close()

	provider := NewProvider(&Config{APIEndpoint: server.URL + "/"})

	tests := []struct {
		name      string
		login     string
		password  string
		wantLogin string
		wantErr   bool
	}{
		{
			name:      "matching login",
			login:     "alice",
			password:  "alice-token",
			wantLogin: "alice",
		},
		{
			name:      "matching login in different case",
			login:     "alice",
			password:  "uppercase-token",
			wantLogin: "Alice",
		},
		{
			name:      "login with surrounding whitespace",
			login:     " alice ",
			password:  "alice-token",
			wantLogin: "alice",
		},
		{
			name:     "token of another user",
			login:    "bob",
			password: "alice-token",
			wantErr:  true,
		},
		{
			name:     "email as login",
			login:    "alice@example.com",
			password: "alice-token",
			wantErr:  true,
		},
		{
			name:     "missing login in response",
			login:    "alice",
			password: "no-login-token",
			wantErr:  true,
		},
		{
			name:     "invalid token",
			login:    "alice",
			password: "bad-token",
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account, err := provider.Authenticate(test.login, test.password)
			if test.wantErr {
				assert.True(t, auth.IsErrBadCredentials(err), "want ErrBadCredentials but got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantLogin, account.Login)
			assert.Equal(t, test.wantLogin, account.Name)
		})
	}
}
