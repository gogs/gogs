package email

import (
	"strings"
	"sync"
	"testing"

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
		assert.Contains(t, m.Text, "https://example.test/user/activate?code=a&b")
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
