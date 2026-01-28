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
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is invalid, use [auth] ACTIVATE_CODE_LIVES instead",
		"option [auth] ENABLE_CAPTCHA is invalid, use [auth] ENABLE_REGISTRATION_CAPTCHA instead",
		"option [auth] ENABLE_NOTIFY_MAIL is invalid, use [user] ENABLE_EMAIL_NOTIFICATION instead",
		"option [auth] REGISTER_EMAIL_CONFIRM is invalid, use [auth] REQUIRE_EMAIL_CONFIRMATION instead",
		"option [auth] RESET_PASSWD_CODE_LIVE_MINUTES is invalid, use [auth] RESET_PASSWORD_CODE_LIVES instead",
		"option [database] DB_TYPE is invalid, use [database] TYPE instead",
		"option [database] PASSWD is invalid, use [database] PASSWORD instead",
		"option [security] REVERSE_PROXY_AUTHENTICATION_USER is invalid, use [auth] REVERSE_PROXY_AUTHENTICATION_HEADER instead",
		"option [session] GC_INTERVAL_TIME is invalid, use [session] GC_INTERVAL instead",
		"option [session] SESSION_LIFE_TIME is invalid, use [session] MAX_LIFE_TIME instead",
		"section [mailer] is invalid, use [email] instead",
		"section [service] is invalid, use [auth] instead",
		"option [server] ROOT_URL is invalid, use [server] EXTERNAL_URL instead",
		"option [server] LANDING_PAGE is invalid, use [server] LANDING_URL instead",

		"option [server] NONEXISTENT_OPTION is invalid",
	}

	gotWarnings := checkInvalidOptions(cfg)
	sort.Strings(wantWarnings)
	sort.Strings(gotWarnings)
	assert.Equal(t, wantWarnings, gotWarnings)
}
