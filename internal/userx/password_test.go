package userx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
)

func TestCheckPasswordPolicy(t *testing.T) {
	// SHA-1 of "password123" is CBFDAC6008F9CAB4083784CBD1874F76618D2A97.
	var gotPath, gotPadding string
	breached := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotPadding = r.URL.Path, r.Header.Get("Add-Padding")
		count := 0
		if breached {
			count = 251682
		}
		_, _ = fmt.Fprintf(w, "0018A45C4D1DEF81644B54AB7F969B88D65:3\r\nC6008F9CAB4083784CBD1874F76618D2A97:%d\r\nFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:0\r\n", count)
	}))
	defer server.Close()
	defer func(old string) { pwnedPasswordsURL = old }(pwnedPasswordsURL)
	pwnedPasswordsURL = server.URL + "/range/"

	ctx := context.Background()
	conf.SetMockAuth(t, conf.AuthOpts{MinPasswordLength: 8})

	t.Run("length counts characters, not bytes", func(t *testing.T) {
		assert.ErrorIs(t, CheckPasswordPolicy(ctx, "1234567"), ErrPasswordTooShort)
		assert.NoError(t, CheckPasswordPolicy(ctx, "12345678"))
		// 7 characters in 21 bytes.
		assert.ErrorIs(t, CheckPasswordPolicy(ctx, "密码密码密码密"), ErrPasswordTooShort)
	})

	t.Run("breach list is off by default", func(t *testing.T) {
		gotPath = ""
		assert.NoError(t, CheckPasswordPolicy(ctx, "password123"))
		assert.Empty(t, gotPath)
	})

	conf.SetMockAuth(t, conf.AuthOpts{MinPasswordLength: 8, CheckLeakedPasswords: true})

	t.Run("leaked password", func(t *testing.T) {
		assert.ErrorIs(t, CheckPasswordPolicy(ctx, "password123"), ErrPasswordLeaked)
		// Only the first 5 characters of the hash are sent.
		assert.Equal(t, "/range/CBFDA", gotPath)
		assert.Equal(t, "true", gotPadding)
	})

	t.Run("padding lines do not count", func(t *testing.T) {
		breached = false
		defer func() { breached = true }()
		assert.NoError(t, CheckPasswordPolicy(ctx, "password123"))
	})

	t.Run("unknown password", func(t *testing.T) {
		assert.NoError(t, CheckPasswordPolicy(ctx, "correct horse battery staple"))
	})

	t.Run("short passwords are not sent", func(t *testing.T) {
		gotPath = ""
		assert.ErrorIs(t, CheckPasswordPolicy(ctx, "short"), ErrPasswordTooShort)
		assert.Empty(t, gotPath)
	})

	t.Run("unreachable list lets the password through", func(t *testing.T) {
		pwnedPasswordsURL = "http://127.0.0.1:1/range/"
		defer func() { pwnedPasswordsURL = server.URL + "/range/" }()
		assert.NoError(t, CheckPasswordPolicy(ctx, "password123"))
	})
}
