package conf

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/ini.v1"
)

func Test_i18n_DateLang(t *testing.T) {
	c := &i18nConf{
		dateLangs: map[string]string{
			"en-US": "en",
			"zh-CN": "zh",
		},
	}

	tests := []struct {
		lang string
		want string
	}{
		{lang: "en-US", want: "en"},
		{lang: "zh-CN", want: "zh"},

		{lang: "jp-JP", want: "en"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, c.DateLang(test.lang))
		})
	}
}

func TestCheckInvalidOptions(t *testing.T) {
	cfg := ini.Empty()
	_, _ = cfg.Section("mailer").NewKey("ENABLED", "true")
	_, _ = cfg.Section("service").NewKey("START_TYPE", "true")
	_, _ = cfg.Section("security").NewKey("REVERSE_PROXY_AUTHENTICATION_USER", "true")
	_, _ = cfg.Section("auth").NewKey("ACTIVE_CODE_LIVE_MINUTES", "10")
	_, _ = cfg.Section("auth").NewKey("RESET_PASSWD_CODE_LIVE_MINUTES", "10")
	_, _ = cfg.Section("auth").NewKey("ENABLE_CAPTCHA", "true")
	_, _ = cfg.Section("auth").NewKey("ENABLE_NOTIFY_MAIL", "true")
	_, _ = cfg.Section("auth").NewKey("REGISTER_EMAIL_CONFIRM", "true")
	_, _ = cfg.Section("session").NewKey("GC_INTERVAL_TIME", "10")
	_, _ = cfg.Section("session").NewKey("SESSION_LIFE_TIME", "10")
	_, _ = cfg.Section("server").NewKey("ROOT_URL", "true")
	_, _ = cfg.Section("server").NewKey("LANDING_PAGE", "true")
	_, _ = cfg.Section("database").NewKey("DB_TYPE", "true")
	_, _ = cfg.Section("database").NewKey("PASSWD", "true")
	_, _ = cfg.Section("other").NewKey("SHOW_FOOTER_BRANDING", "true")
	_, _ = cfg.Section("other").NewKey("SHOW_FOOTER_TEMPLATE_LOAD_TIME", "true")
	_, _ = cfg.Section("email").NewKey("ENABLED", "true")
	_, _ = cfg.Section("server").NewKey("NONEXISTENT_OPTION", "true")

	wantWarnings := []string{
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is invalid",
		"option [auth] ENABLE_CAPTCHA is invalid",
		"option [auth] ENABLE_NOTIFY_MAIL is invalid",
		"option [auth] REGISTER_EMAIL_CONFIRM is invalid",
		"option [auth] RESET_PASSWD_CODE_LIVE_MINUTES is invalid",
		"option [database] DB_TYPE is invalid",
		"option [database] PASSWD is invalid",
		"option [security] REVERSE_PROXY_AUTHENTICATION_USER is invalid",
		"option [session] GC_INTERVAL_TIME is invalid",
		"option [session] SESSION_LIFE_TIME is invalid",
		"section [mailer] is invalid, use [email] instead",
		"section [service] is invalid, use [auth] instead",
		"option [server] ROOT_URL is invalid",
		"option [server] LANDING_PAGE is invalid",
		"option [server] NONEXISTENT_OPTION is invalid",
	}

	gotWarnings := checkInvalidOptions(cfg)
	sort.Strings(wantWarnings)
	sort.Strings(gotWarnings)
	assert.Equal(t, wantWarnings, gotWarnings)
}
