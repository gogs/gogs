package integrationtests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
)

// userAssertPage asserts that the path serves the HTML shell of the web app
// with the status, and returns the shell.
func userAssertPage(t *testing.T, c *client, path string, wantStatus int) string {
	t.Helper()

	status, header, body := c.doRaw(http.MethodGet, path, "", nil)
	require.Equal(t, wantStatus, status, "GET %s, response: %s", path, body)
	assert.Equal(t, "text/html; charset=utf-8", header.Get("Content-Type"), "GET %s", path)
	assert.Contains(t, string(body), "window.__webContext=", "GET %s", path)
	return string(body)
}

// userAssertRedirect asserts that the path redirects to the location.
func userAssertRedirect(t *testing.T, c *client, path string, wantStatus int, wantLocation string) {
	t.Helper()

	status, header, body := c.doRaw(http.MethodGet, path, "", nil)
	require.Equal(t, wantStatus, status, "GET %s, response: %s", path, body)
	assert.Equal(t, wantLocation, header.Get("Location"), "GET %s", path)
}

func TestUserActivateEmail(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	address := UniqueName("confirm-") + "@example.com"
	require.NoError(t, database.Handle.Users().AddEmail(t.Context(), u.ID, address, false))
	stored := userGet(t, u.ID)
	code := userx.GenerateConfirmEmailCode(stored.ID, address, stored.LowerName, stored.Password, stored.Rands)
	activated := func() bool {
		e, err := database.Handle.Users().GetEmail(t.Context(), u.ID, address, false)
		require.NoError(t, err)
		return e.IsActivated
	}
	activatePath := func(code, address string) string {
		return "/user/activate_email?" + url.Values{"code": {code}, "email": {address}}.Encode()
	}

	c := newClient(t)
	// A code is only good for the address it was mailed to.
	otherAddress := UniqueName("other-") + "@example.com"
	require.NoError(t, database.Handle.Users().AddEmail(t.Context(), u.ID, otherAddress, false))
	userAssertRedirect(t, c, activatePath(code, otherAddress), http.StatusFound, "/user/settings/email")
	userAssertRedirect(t, c, activatePath(code+"0", address), http.StatusFound, "/user/settings/email")
	// Nor can a code that activates an account confirm an address.
	activateAccountCode := userx.GenerateActivateCode(stored.ID, address, stored.LowerName, stored.Password, stored.Rands)
	userAssertRedirect(t, c, activatePath(activateAccountCode, address), http.StatusFound, "/user/settings/email")
	assert.False(t, activated())

	t.Run("confirmed on the settings page", func(t *testing.T) {
		userAssertRedirect(t, c, activatePath(code, address), http.StatusFound, "/user/settings/email?activated=1")
		assert.True(t, activated())
	})

	t.Run("activated", func(t *testing.T) {
		userAssertRedirect(t, c, activatePath(code, address), http.StatusFound, "/user/settings/email?activated=1")
		assert.True(t, activated())
	})
}

func TestUserActivateEmailSeveral(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	addresses := []string{UniqueName("first-") + "@example.com", UniqueName("second-") + "@example.com"}
	// Both links are mailed before either is opened, like when a user adds
	// several addresses at once.
	stored := userGet(t, u.ID)
	codes := make([]string, len(addresses))
	for i, address := range addresses {
		require.NoError(t, database.Handle.Users().AddEmail(t.Context(), u.ID, address, false))
		codes[i] = userx.GenerateConfirmEmailCode(stored.ID, address, stored.LowerName, stored.Password, stored.Rands)
	}

	c := newClient(t)
	for i, address := range addresses {
		path := "/user/activate_email?" + url.Values{"code": {codes[i]}, "email": {address}}.Encode()
		userAssertRedirect(t, c, path, http.StatusFound, "/user/settings/email?activated=1")
		e, err := database.Handle.Users().GetEmail(t.Context(), u.ID, address, false)
		require.NoError(t, err)
		assert.True(t, e.IsActivated, "%s not activated", address)
	}
}

func TestUserEmail2User(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})

	t.Run("unknown email", func(t *testing.T) {
		userAssertPage(t, newClient(t), "/user/email2user?email="+url.QueryEscape(UniqueName("nobody-")+"@example.com"), http.StatusNotFound)
	})

	t.Run("profile", func(t *testing.T) {
		c := newClient(t)
		userAssertRedirect(t, c, "/user/email2user?email="+url.QueryEscape(u.Email), http.StatusFound, "/"+u.Name)
	})
}

func TestUserPublicKeys(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	_, content := userSeedSSHKey(t, u.ID, "laptop")

	t.Run("keys", func(t *testing.T) {
		status, header, body := newClient(t).doRaw(http.MethodGet, "/"+u.Name+".keys", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, "text/plain; charset=UTF-8", header.Get("Content-Type"))
		// The comment of the key, often an email address, is left out.
		assert.Equal(t, content+"\n", string(body))
	})

	t.Run("unknown user", func(t *testing.T) {
		userAssertPage(t, newClient(t), "/"+UniqueName("nobody-")+".keys", http.StatusNotFound)
	})
}

var userLocaleURL = regexp.MustCompile(`"localeURL":"([^"]+)"`)

func TestUserSiteLocale(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	page := userAssertPage(t, c, "/", http.StatusOK)
	m := userLocaleURL.FindStringSubmatch(page)
	require.NotNil(t, m, "no localeURL in page")
	localeURL := m[1]
	require.True(t, strings.HasPrefix(localeURL, "/assets/locale/en-US."), localeURL)

	status, header, body := c.doRaw(http.MethodGet, localeURL, "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, "application/json; charset=utf-8", header.Get("Content-Type"))
	assert.Contains(t, header.Get("Cache-Control"), "immutable")
	// The URL carries a hash of the content, so a change gets a new URL.
	sum := sha256.Sum256(body)
	assert.Equal(t, "/assets/locale/en-US."+hex.EncodeToString(sum[:6])+".json", localeURL)

	var messages map[string]string
	require.NoError(t, json.Unmarshal(body, &messages))
	assert.Equal(t, "Sign in", messages["sign_in"])
	assert.Equal(t, "My repositories", messages["home.my_repos"])
}

func TestUserSiteCaptchaImage(t *testing.T) {
	t.Parallel()

	// The server with FORCE_PRIVATE uses the image captcha.
	c := newClient(t)
	c.baseURL = forcePrivateServerURL
	status, header, body := c.doRaw(http.MethodGet, "/-/captcha/image.png", "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "image/png", header.Get("Content-Type"))
	assert.True(t, bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")), "not a PNG")
	text, ok := c.sessionValue(captchaSessionKey).(string)
	require.True(t, ok, "no captcha text in session")
	assert.NotEmpty(t, text)

	// The server with altcha does not serve captcha images.
	_, header, _ = newClient(t).doRaw(http.MethodGet, "/-/captcha/image.png", "", nil)
	assert.NotEqual(t, "image/png", header.Get("Content-Type"))
}

func TestUserSiteAltchaWorker(t *testing.T) {
	t.Parallel()

	status, header, body := newClient(t).doRaw(http.MethodGet, "/-/altcha/worker.js", "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "text/javascript; charset=utf-8", header.Get("Content-Type"))
	assert.Contains(t, string(body), "self.onmessage")

	// The server with the image captcha does not serve altcha.
	c := newClient(t)
	c.baseURL = forcePrivateServerURL
	_, header, _ = c.doRaw(http.MethodGet, "/-/altcha/challenge.json", "", nil)
	assert.NotEqual(t, "application/json", header.Get("Content-Type"))
}

func TestUserSiteRedirect(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	userAssertRedirect(t, c, "/redirect?to="+url.QueryEscape("/explore/repos?q=x"), http.StatusSeeOther, "/explore/repos?q=x")
	for _, to := range []string{"https://example.com/", "//example.com/", `/\example.com`, ""} {
		userAssertRedirect(t, c, "/redirect?to="+url.QueryEscape(to), http.StatusSeeOther, "/")
	}
}

func TestUserSiteRobotsTxt(t *testing.T) {
	t.Parallel()

	// The server under test has no custom robots.txt.
	status, _, body := newClient(t).doRaw(http.MethodGet, "/robots.txt", "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestUserSiteHealthCheck(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	status, header, body := c.doRaw(http.MethodGet, "/healthcheck", "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))
	assert.Equal(t, "* Database connection: OK\n", string(body))

	status, header, body = c.doRaw(http.MethodHead, "/healthcheck", "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))
	assert.Empty(t, body)
}

func TestUserSiteMetrics(t *testing.T) {
	t.Parallel()

	// Metrics are enabled without basic authentication by default.
	status, header, body := newClient(t).doRaw(http.MethodGet, "/-/metrics", "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.True(t, strings.HasPrefix(header.Get("Content-Type"), "text/plain"), header.Get("Content-Type"))
	assert.Contains(t, string(body), "\ngo_goroutines ")
}
