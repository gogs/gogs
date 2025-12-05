// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

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

// TestWarnDeprecated tests the warnDeprecated function.
// It creates a new ini.File, adds some sections and keys,
// and then checks if the expected warnings are returned by the function.
func TestWarnDeprecated(t *testing.T) {
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

	expectedDeprecatedWarning := []string{
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is deprecated, use [auth] ACTIVATE_CODE_LIVES instead",
		"option [auth] ENABLE_CAPTCHA is deprecated, use [auth] ENABLE_REGISTRATION_CAPTCHA instead",
		"option [auth] ENABLE_NOTIFY_MAIL is deprecated, use [user] ENABLE_EMAIL_NOTIFICATION instead",
		"option [auth] REGISTER_EMAIL_CONFIRM is deprecated, use [auth] REQUIRE_EMAIL_CONFIRMATION instead",
		"option [auth] RESET_PASSWD_CODE_LIVE_MINUTES is deprecated, use [auth] RESET_PASSWORD_CODE_LIVES instead",
		"option [database] DB_TYPE is deprecated, use [database] TYPE instead",
		"option [database] PASSWD is deprecated, use [database] PASSWORD instead",
		"option [security] REVERSE_PROXY_AUTHENTICATION_USER is deprecated, use [auth] REVERSE_PROXY_AUTHENTICATION_HEADER instead",
		"option [session] GC_INTERVAL_TIME is deprecated, use [session] GC_INTERVAL instead",
		"option [session] SESSION_LIFE_TIME is deprecated, use [session] MAX_LIFE_TIME instead",
		"section mailer is deprecated, use email instead",
		"section service is deprecated, use auth instead",
		"option [server] ROOT_URL is deprecated, use [server] EXTERNAL_URL instead",
		"option [server] LANDING_PAGE is deprecated, use [server] LANDING_URL instead",
	}
	expectedUnusedWarning := []string{
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is not in the available config",
		"option [auth] ENABLE_CAPTCHA is not in the available config",
		"option [auth] ENABLE_NOTIFY_MAIL is not in the available config",
		"option [auth] REGISTER_EMAIL_CONFIRM is not in the available config",
		"option [auth] RESET_PASSWD_CODE_LIVE_MINUTES is not in the available config",
		"option [database] DB_TYPE is not in the available config",
		"option [database] PASSWD is not in the available config",
		"option [email] key_not_exist is not in the available config",
		"option [mailer] ENABLED is not in the available config",
		"option [security] REVERSE_PROXY_AUTHENTICATION_USER is not in the available config",
		"option [server] LANDING_PAGE is not in the available config",
		"option [server] ROOT_URL is not in the available config",
		"option [service] START_TYPE is not in the available config",
		"option [session] GC_INTERVAL_TIME is not in the available config",
		"option [session] SESSION_LIFE_TIME is not in the available config",
	}

	expectedWarning := append(expectedDeprecatedWarning, expectedUnusedWarning...)
	actualWarning := warnDeprecated(cfg)
	sort.Strings(expectedWarning)
	sort.Strings(actualWarning)
	assert.Equal(t, expectedWarning, actualWarning)

}
