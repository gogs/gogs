package email

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unknwon/i18n"

	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/conf"
)

func setupTemplates(t *testing.T) {
	t.Helper()
	conf.SetMockApp(t, conf.AppOpts{BrandName: "Gogs"})
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL:        "https://example.test/",
		LoadAssetsFromDisk: false,
	})
	tplSet = nil
	tplSetErr = nil
	tplSetOnce = sync.Once{}
}

// TestPreviews composes every email the way the admin panel previews it, so a
// broken template, a missing data field or a translation with the wrong
// arguments is caught before the first email goes out.
func TestPreviews(t *testing.T) {
	setupTemplates(t)

	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			m, err := Preview(name, "en-US")
			require.NoError(t, err)
			assert.NotEmpty(t, m.Subject)
			assert.Contains(t, m.HTML, `lang="en-US"`)
			for part, body := range map[string]string{"HTML": m.HTML, "text": m.Text} {
				assert.NotContains(t, body, "<no value>", "%s references a missing data field", part)
				assert.NotContains(t, body, "%!", "%s has a translation with the wrong arguments", part)
				assert.NotContains(t, body, "{{", "%s has an action left over", part)
			}
			assert.NotContains(t, m.Text, "<p", "text has HTML in it")
			// A page cannot show cid: images.
			assert.Contains(t, m.HTML, `src="data:image/png;base64,`)
			assert.NotContains(t, m.HTML, "cid:")
		})
	}

	_, err := Preview("does/not/exist", "en-US")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCompose(t *testing.T) {
	setupTemplates(t)
	data := userCodeData{Username: `<b>bob</b>`, ActiveCodeLives: 3, Code: "a&b"}

	t.Run("escapes data in HTML only", func(t *testing.T) {
		m, err := activateAccountMail.compose("en-US", data)
		require.NoError(t, err)
		assert.Contains(t, m.HTML, "Hi &lt;b&gt;bob&lt;/b&gt;,")
		assert.NotContains(t, m.HTML, "<b>bob</b>")
		assert.Contains(t, m.HTML, `href="https://example.test/user/activate?code=a%26b"`)
		assert.Contains(t, m.Text, "Hi <b>bob</b>,")
		assert.Contains(t, m.Text, "within 3 hours")
		assert.Contains(t, m.Text, "https://example.test/user/activate?code=a%26b")
	})

	t.Run("in the language of the recipient", func(t *testing.T) {
		loadLocales()
		data, err := embedconf.Files.ReadFile("locale/locale_zh-CN.ini")
		require.NoError(t, err)
		err = i18n.SetMessage("zh-CN", data)
		if err != nil {
			require.ErrorIs(t, err, i18n.ErrLangAlreadyExist)
		}

		m, err := activateAccountMail.compose("zh-CN", userCodeData{Username: "bob"})
		require.NoError(t, err)
		assert.Equal(t, "请激活您的帐户", m.Subject)
		assert.Contains(t, m.HTML, `lang="zh-CN"`)
		// Keys missing from a language fall back to English.
		assert.True(t, strings.HasPrefix(m.Text, "Gogs"), m.Text)
		assert.Contains(t, m.Text, "Hi bob,")
	})
}

func TestSecurityAlert(t *testing.T) {
	setupTemplates(t)
	u := sampleUser{}
	at := time.Date(2026, 11, 12, 23, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))

	edge := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36 Edg/146.0.0.0"
	m, err := AlertSSHKeyAdded.compose("en-US", newSecurityAlertData("en-US", u, Activity{UserAgent: edge, IP: "198.51.100.7", Time: at}, `<i>laptop</i>`))
	require.NoError(t, err)
	assert.Equal(t, "An SSH key was added to your account", m.Subject)
	assert.Contains(t, m.Text, `The SSH key "<i>laptop</i>" was added to your account on Gogs.`)
	assert.NotContains(t, m.HTML, "<i>laptop</i>")
	assert.Contains(t, m.Text, "Edge on Windows")
	assert.Contains(t, m.Text, "2026-11-12 15:30 UTC")
	assert.Contains(t, m.Text, "198.51.100.7")

	// Whoever signs in chooses the user agent, so it is never shown as is.
	m, err = AlertNewSignIn.compose("en-US", newSecurityAlertData("en-US", u, Activity{UserAgent: "Click https://evil.example", Time: at}, ""))
	require.NoError(t, err)
	assert.Contains(t, m.Text, "Unknown browser")
	assert.NotContains(t, m.Text, "evil.example")
	assert.NotContains(t, m.HTML, "evil.example")
}

func TestMessageLogo(t *testing.T) {
	setupTemplates(t)
	conf.Email.From = "Gogs <gogs@example.test>"
	t.Cleanup(func() { conf.Email.From = "" })
	render := func(t *testing.T, m *Mail) string {
		t.Helper()
		msg, err := newMessage([]string{"bob@example.test"}, m)
		require.NoError(t, err)
		var buf strings.Builder
		_, err = msg.msg.WriteTo(&buf)
		require.NoError(t, err)
		return buf.String()
	}

	m, err := activateAccountMail.compose("en-US", userCodeData{Username: "bob"})
	require.NoError(t, err)
	require.NotEmpty(t, m.Logo)
	assert.Contains(t, m.HTML, `src="cid:logo.png"`)

	t.Run("travels inside the email", func(t *testing.T) {
		out := render(t, m)
		assert.Contains(t, out, "multipart/related")
		assert.Contains(t, out, "Content-Id: <logo.png>")
		assert.Contains(t, out, "Content-Type: image/png")
	})

	t.Run("not in plain text emails", func(t *testing.T) {
		conf.Email.UsePlainText = true
		t.Cleanup(func() { conf.Email.UsePlainText = false })
		assert.NotContains(t, render(t, m), "logo.png")
	})

	t.Run("not when the HTML does not show it", func(t *testing.T) {
		assert.NotContains(t, render(t, &Mail{Subject: "x", HTML: "<p>x</p>", Text: "x"}), "logo.png")
	})
}
