package integrationtests

import (
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
	code := userx.GenerateActivateCode(stored.ID, address, stored.LowerName, stored.Password, stored.Rands)
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
	assert.False(t, activated())

	t.Run("confirmed on the settings page", func(t *testing.T) {
		userAssertRedirect(t, c, activatePath(code, address), http.StatusFound, "/user/settings/email?activated=1")
		assert.True(t, activated())
	})

	t.Run("activated", func(t *testing.T) {
		status, header, body := c.doRaw(http.MethodGet, activatePath(code, address), "", nil)
		require.Equal(t, http.StatusFound, status, "response: %s", body)
		assert.True(t, strings.HasPrefix(header.Get("Location"), "/user/settings/email"), header.Get("Location"))
		assert.True(t, activated())
	})
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
	assert.Equal(t, "Username or email", messages["home.uname_holder"])
}

func TestUserSiteCaptchaImage(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	status, header, body := c.doRaw(http.MethodGet, "/captcha/image.jpeg", "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "image/jpeg", header.Get("Content-Type"))
	assert.True(t, len(body) > 2 && body[0] == 0xFF && body[1] == 0xD8, "not a JPEG")
	text, ok := c.sessionValue(captchaSessionKey).(string)
	require.True(t, ok, "no captcha text in session")
	assert.NotEmpty(t, text)
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
