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

func TestWarnInvalidOptions(t *testing.T) {
	cfg := ini.Empty()
	cfg.Section("mailer").NewKey("ENABLED", "true")
	cfg.Section("service").NewKey("START_TYPE", "true")
	cfg.Section("security").NewKey("REVERSE_PROXY_AUTHENTICATION_USER", "true")
	cfg.Section("auth").NewKey("ACTIVE_CODE_LIVE_MINUTES", "10")
	cfg.Section("auth").NewKey("RESET_PASSWD_CODE_LIVE_MINUTES", "10")
	cfg.Section("auth").NewKey("ENABLE_CAPTCHA", "true")
	cfg.Section("auth").NewKey("ENABLE_NOTIFY_MAIL", "true")
	cfg.Section("auth").NewKey("REGISTER_EMAIL_CONFIRM", "true")
	cfg.Section("session").NewKey("GC_INTERVAL_TIME", "10")
	cfg.Section("session").NewKey("SESSION_LIFE_TIME", "10")
	cfg.Section("server").NewKey("ROOT_URL", "true")
	cfg.Section("server").NewKey("LANDING_PAGE", "true")
	cfg.Section("database").NewKey("DB_TYPE", "true")
	cfg.Section("database").NewKey("PASSWD", "true")
	cfg.Section("other").NewKey("SHOW_FOOTER_BRANDING", "true")
	cfg.Section("other").NewKey("SHOW_FOOTER_TEMPLATE_LOAD_TIME", "true")
	cfg.Section("email").NewKey("ENABLED", "true")
	cfg.Section("email").NewKey("key_not_exist", "true")

	expectedRenamedWarnings := []string{
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
	}
	expectedUnknownWarnings := []string{
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is invalid",
		"option [auth] ENABLE_CAPTCHA is invalid",
		"option [auth] ENABLE_NOTIFY_MAIL is invalid",
		"option [auth] REGISTER_EMAIL_CONFIRM is invalid",
		"option [auth] RESET_PASSWD_CODE_LIVE_MINUTES is invalid",
		"option [database] DB_TYPE is invalid",
		"option [database] PASSWD is invalid",
		"option [email] key_not_exist is invalid",
		"option [mailer] ENABLED is invalid",
		"option [security] REVERSE_PROXY_AUTHENTICATION_USER is invalid",
		"option [server] LANDING_PAGE is invalid",
		"option [server] ROOT_URL is invalid",
		"option [service] START_TYPE is invalid",
		"option [session] GC_INTERVAL_TIME is invalid",
		"option [session] SESSION_LIFE_TIME is invalid",
	}

	expectedWarnings := append(expectedRenamedWarnings, expectedUnknownWarnings...)
	actualWarnings := warnInvalidOptions(cfg)
	sort.Strings(expectedWarnings)
	sort.Strings(actualWarnings)
	assert.Equal(t, expectedWarnings, actualWarnings)
}
