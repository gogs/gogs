package user

import (
	gocontext "context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-macaron/session"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

type testLocale struct{}

func (testLocale) Language() string {
	return "en-US"
}

func (testLocale) Tr(key string, _ ...interface{}) string {
	return key
}

type testSessionStore struct {
	values map[interface{}]interface{}
}

type testGORMLoggerWriter struct{}

func (testGORMLoggerWriter) Printf(string, ...interface{}) {}

func newTestSessionStore() *testSessionStore {
	return &testSessionStore{
		values: make(map[interface{}]interface{}),
	}
}

func (s *testSessionStore) Set(key, val interface{}) error {
	s.values[key] = val
	return nil
}

func (s *testSessionStore) Get(key interface{}) interface{} {
	return s.values[key]
}

func (s *testSessionStore) Delete(key interface{}) error {
	delete(s.values, key)
	return nil
}

func (*testSessionStore) ID() string {
	return "test-session"
}

func (*testSessionStore) Release() error {
	return nil
}

func (s *testSessionStore) Flush() error {
	clear(s.values)
	return nil
}

func (s *testSessionStore) Read(_ string) (session.RawStore, error) {
	return s, nil
}

func (s *testSessionStore) Destory(_ *macaron.Context) error {
	return s.Flush()
}

//nolint:staticcheck // go-macaron/session uses RegenerateId in its Store interface.
func (s *testSessionStore) RegenerateId(_ *macaron.Context) (session.RawStore, error) {
	return s, nil
}

func (s *testSessionStore) Count() int {
	return len(s.values)
}

func (*testSessionStore) GC() {}

func newTestRouteContext(t *testing.T, method, target string, body url.Values) (*context.Context, *httptest.ResponseRecorder, *testSessionStore) {
	t.Helper()

	var requestBody strings.Reader
	if body != nil {
		requestBody = *strings.NewReader(body.Encode())
	} else {
		requestBody = *strings.NewReader("")
	}

	req, err := http.NewRequest(method, target, &requestBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	recorder := httptest.NewRecorder()
	macaronContext := &macaron.Context{
		Req:    macaron.Request{Request: req},
		Resp:   macaron.NewResponseWriter(method, recorder),
		Data:   map[string]interface{}{},
		Locale: testLocale{},
	}

	sessionStore := newTestSessionStore()
	return &context.Context{
		Context: macaronContext,
		Flash:   &session.Flash{Values: url.Values{}},
		Session: sessionStore,
	}, recorder, sessionStore
}

func performRouteRequest(
	t *testing.T,
	route string,
	form url.Values,
	setup func(c *context.Context, sess *testSessionStore),
	handler func(c *context.Context),
) (*httptest.ResponseRecorder, *context.Context, *testSessionStore) {
	t.Helper()

	m := macaron.New()
	m.Use(macaron.Renderer())

	var (
		capturedContext *context.Context
		capturedSession *testSessionStore
	)
	m.Use(func(mc *macaron.Context) {
		if mc.Data == nil {
			mc.Data = make(map[string]interface{})
		}
		mc.Locale = testLocale{}

		capturedSession = newTestSessionStore()
		capturedContext = &context.Context{
			Context: mc,
			Flash:   &session.Flash{Values: url.Values{}},
			Session: capturedSession,
			Link:    conf.Server.Subpath + strings.TrimSuffix(mc.Req.URL.Path, "/"),
		}
		capturedContext.Data["Link"] = capturedContext.Link
		if setup != nil {
			setup(capturedContext, capturedSession)
		}
		mc.Map(capturedContext)
	})

	m.Post(route, handler)

	var req *http.Request
	var err error
	if form != nil {
		req, err = http.NewRequest(http.MethodPost, route, strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(http.MethodPost, route, http.NoBody)
		require.NoError(t, err)
	}

	recorder := httptest.NewRecorder()
	m.ServeHTTP(recorder, req)
	return recorder, capturedContext, capturedSession
}

func setupDatabaseHandle(t *testing.T) {
	t.Helper()

	beforeHandle := database.Handle
	beforeDatabase := conf.Database
	beforeUseMySQL := conf.UseMySQL
	beforeUsePostgreSQL := conf.UsePostgreSQL
	beforeUseSQLite3 := conf.UseSQLite3

	conf.Database = conf.DatabaseOpts{
		Type:         "sqlite3",
		Path:         filepath.Join(t.TempDir(), "gogs-test.db"),
		MaxOpenConns: 10,
		MaxIdleConns: 10,
	}

	db, err := database.NewConnection(testGORMLoggerWriter{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sqlDB.Close()
		database.Handle = beforeHandle
		conf.Database = beforeDatabase
		conf.UseMySQL = beforeUseMySQL
		conf.UsePostgreSQL = beforeUsePostgreSQL
		conf.UseSQLite3 = beforeUseSQLite3
	})
}

func marshalCredential(t *testing.T, credential webauthn.Credential) string {
	t.Helper()
	data, err := json.Marshal(credential)
	require.NoError(t, err)
	return string(data)
}

func TestNewWebAuthnUser(t *testing.T) {
	passkeys := []*database.Passkey{
		{
			ID:         11,
			Credential: marshalCredential(t, webauthn.Credential{ID: []byte("credential-1")}),
		},
		{
			ID:         22,
			Credential: marshalCredential(t, webauthn.Credential{ID: []byte("credential-2")}),
		},
	}

	u := &database.User{ID: 42, Name: "alice", FullName: "Alice Example"}
	webAuthnUser, err := newWebAuthnUser(u, passkeys)
	require.NoError(t, err)

	assert.Equal(t, []byte("42"), webAuthnUser.WebAuthnID())
	assert.Equal(t, "alice", webAuthnUser.WebAuthnName())
	assert.Equal(t, "Alice Example", webAuthnUser.WebAuthnDisplayName())
	require.Len(t, webAuthnUser.WebAuthnCredentials(), 2)
	assert.Equal(t, int64(11), webAuthnUser.passkeyIDByCredentialID["Y3JlZGVudGlhbC0x"])
	assert.Equal(t, int64(22), webAuthnUser.passkeyIDByCredentialID["Y3JlZGVudGlhbC0y"])

	passkeyID, ok := webAuthnUser.passkeyIDByCredential([]byte("credential-1"))
	require.True(t, ok)
	assert.Equal(t, int64(11), passkeyID)

	_, ok = webAuthnUser.passkeyIDByCredential([]byte("credential-3"))
	assert.False(t, ok)
}

func TestNewWebAuthnUser_BadCredential(t *testing.T) {
	_, err := newWebAuthnUser(
		&database.User{ID: 1, Name: "alice"},
		[]*database.Passkey{
			{
				ID:         1,
				Credential: "{bad-json",
			},
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode credential")
}

func TestNewWebAuthn(t *testing.T) {
	t.Run("with URL host", func(t *testing.T) {
		parsedURL, err := url.Parse("https://git.example.com/")
		require.NoError(t, err)
		conf.SetMockServer(t, conf.ServerOpts{
			ExternalURL: "https://git.example.com/",
			URL:         parsedURL,
		})
		conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs Test"})

		webAuthn, err := newWebAuthn()
		require.NoError(t, err)
		assert.Equal(t, "git.example.com", webAuthn.Config.RPID)
		assert.Equal(t, []string{"https://git.example.com"}, webAuthn.Config.RPOrigins)
		assert.Equal(t, "Gogs Test", webAuthn.Config.RPDisplayName)
	})

	t.Run("fallback to domain", func(t *testing.T) {
		conf.SetMockServer(t, conf.ServerOpts{
			ExternalURL: "https://fallback.example.com/",
			Domain:      "fallback.example.com",
		})
		conf.SetMockApp(t, conf.AppOpts{})

		webAuthn, err := newWebAuthn()
		require.NoError(t, err)
		assert.Equal(t, "fallback.example.com", webAuthn.Config.RPID)
		assert.Equal(t, "Gogs", webAuthn.Config.RPDisplayName)
	})

	t.Run("missing RP ID", func(t *testing.T) {
		conf.SetMockServer(t, conf.ServerOpts{
			ExternalURL: "https://example.com/",
		})
		conf.SetMockApp(t, conf.AppOpts{})

		_, err := newWebAuthn()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty relying party ID")
	})
}

func TestWebAuthnSessionData(t *testing.T) {
	c, _, _ := newTestRouteContext(t, http.MethodGet, "/", nil)
	sessionData := &webauthn.SessionData{
		Challenge:      "challenge-1",
		RelyingPartyID: "git.example.com",
	}

	err := saveWebAuthnSession(c, passkeyLoginSessionDataKey, sessionData)
	require.NoError(t, err)

	got, ok, err := loadWebAuthnSession(c, passkeyLoginSessionDataKey)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, sessionData.Challenge, got.Challenge)
	assert.Equal(t, sessionData.RelyingPartyID, got.RelyingPartyID)
}

func TestWebAuthnSessionData_BadPayload(t *testing.T) {
	c, _, sessionStore := newTestRouteContext(t, http.MethodGet, "/", nil)
	err := sessionStore.Set(passkeyLoginSessionDataKey, "{bad-json")
	require.NoError(t, err)

	_, ok, err := loadWebAuthnSession(c, passkeyLoginSessionDataKey)
	require.Error(t, err)
	assert.True(t, ok)
	assert.Contains(t, err.Error(), "unmarshal session data")
}

func TestLoginPasskeyPost_MissingSession(t *testing.T) {
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})
	c, recorder, _ := newTestRouteContext(t, http.MethodPost, "/user/login/passkey", nil)

	LoginPasskeyPost(c)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/login", recorder.Header().Get("Location"))
	assert.Equal(t, "auth.passkey_session_expired", c.Flash.ErrorMsg)
}

func TestLoginPasskeyOptions(t *testing.T) {
	parsedURL, err := url.Parse("https://git.example.com/")
	require.NoError(t, err)
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL: "https://git.example.com/",
		URL:         parsedURL,
		Subpath:     "",
	})
	conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs Test"})

	recorder, _, sessionStore := performRouteRequest(t, "/user/login/passkey/options", nil, nil, LoginPasskeyOptions)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"publicKey"`)
	assert.NotNil(t, sessionStore.Get(passkeyLoginSessionDataKey))
}

func TestLoginPasskeyPost_InvalidCredential(t *testing.T) {
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})
	body := url.Values{
		"credential": []string{"not-json"},
	}
	c, recorder, sessionStore := newTestRouteContext(t, http.MethodPost, "/user/login/passkey", body)
	err := saveWebAuthnSession(c, passkeyLoginSessionDataKey, &webauthn.SessionData{Challenge: "challenge"})
	require.NoError(t, err)

	LoginPasskeyPost(c)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/login", recorder.Header().Get("Location"))
	assert.Equal(t, "auth.passkey_login_failed", c.Flash.ErrorMsg)
	assert.Nil(t, sessionStore.Get(passkeyLoginSessionDataKey))
}

func TestSettingsPasskeyRegister(t *testing.T) {
	setupDatabaseHandle(t)

	parsedURL, err := url.Parse("https://git.example.com/")
	require.NoError(t, err)
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL: "https://git.example.com/",
		URL:         parsedURL,
		Subpath:     "",
	})
	conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs Test"})

	recorder, _, sessionStore := performRouteRequest(t, "/user/settings/security/passkeys/register", nil, func(c *context.Context, _ *testSessionStore) {
		c.IsLogged = true
		c.User = &database.User{
			ID:       1,
			Name:     "alice",
			FullName: "Alice",
			Email:    "alice@example.com",
		}
	}, SettingsPasskeyRegister)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"publicKey"`)
	assert.NotNil(t, sessionStore.Get(passkeyRegistrationSessionDataKey))
}

func TestSettingsPasskeyCreate_MissingSession(t *testing.T) {
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})
	c, recorder, _ := newTestRouteContext(t, http.MethodPost, "/user/settings/security/passkeys", nil)

	SettingsPasskeyCreate(c)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/settings/security", recorder.Header().Get("Location"))
	assert.Equal(t, "settings.passkey_session_expired", c.Flash.ErrorMsg)
}

func TestSettingsPasskeyCreate_InvalidCredential_ClearsSession(t *testing.T) {
	setupDatabaseHandle(t)
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})

	form := url.Values{
		"credential": []string{"not-json"},
	}
	recorder, c, sessionStore := performRouteRequest(t, "/user/settings/security/passkeys", form, func(c *context.Context, sess *testSessionStore) {
		c.IsLogged = true
		c.User = &database.User{
			ID:       1,
			Name:     "alice",
			FullName: "Alice",
			Email:    "alice@example.com",
		}
		err := saveWebAuthnSession(c, passkeyRegistrationSessionDataKey, &webauthn.SessionData{Challenge: "challenge"})
		require.NoError(t, err)
		assert.NotNil(t, sess.Get(passkeyRegistrationSessionDataKey))
	}, SettingsPasskeyCreate)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/settings/security", recorder.Header().Get("Location"))
	assert.Equal(t, "settings.passkey_register_failed_reason", c.Flash.ErrorMsg)
	assert.Nil(t, sessionStore.Get(passkeyRegistrationSessionDataKey))
}

func TestSettingsPasskeyDelete(t *testing.T) {
	setupDatabaseHandle(t)
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})

	credential := marshalCredential(t, webauthn.Credential{ID: []byte("credential-1")})
	passkey := &database.Passkey{
		UserID:       1,
		Name:         "alice-macbook",
		CredentialID: "Y3JlZGVudGlhbC0x",
		Credential:   credential,
	}
	_, err := database.Handle.Passkeys().Create(gocontext.Background(), passkey.UserID, passkey.Name, webauthn.Credential{ID: []byte("credential-1")})
	require.NoError(t, err)

	passkeys, err := database.Handle.Passkeys().ListByUserID(gocontext.Background(), 1)
	require.NoError(t, err)
	require.Len(t, passkeys, 1)

	form := url.Values{
		"id": []string{strconv.FormatInt(passkeys[0].ID, 10)},
	}
	recorder, c, _ := performRouteRequest(t, "/user/settings/security/passkeys/delete", form, func(c *context.Context, _ *testSessionStore) {
		c.IsLogged = true
		c.User = &database.User{ID: 1, Name: "alice"}
	}, SettingsPasskeyDelete)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/settings/security", recorder.Header().Get("Location"))
	assert.Equal(t, "settings.passkey_delete_success", c.Flash.SuccessMsg)

	passkeys, err = database.Handle.Passkeys().ListByUserID(gocontext.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, passkeys)
}

func TestSettingsPasskeyDelete_NotFound(t *testing.T) {
	setupDatabaseHandle(t)
	conf.SetMockServer(t, conf.ServerOpts{Subpath: ""})

	form := url.Values{
		"id": []string{"999"},
	}
	recorder, c, _ := performRouteRequest(t, "/user/settings/security/passkeys/delete", form, func(c *context.Context, _ *testSessionStore) {
		c.IsLogged = true
		c.User = &database.User{ID: 1, Name: "alice"}
	}, SettingsPasskeyDelete)

	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Equal(t, "/user/settings/security", recorder.Header().Get("Location"))
	assert.Equal(t, "settings.passkey_not_found", c.Flash.ErrorMsg)
}
