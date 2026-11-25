package conf

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"unknwon.dev/ini/v2"
)

func TestCheckInvalidOptions(t *testing.T) {
	cfg := ini.Empty()
	_, _ = cfg.Section("mailer").NewKey("ENABLED", "true")
	_, _ = cfg.Section("service").NewKey("START_TYPE", "true")
	_, _ = cfg.Section("security").NewKey("REVERSE_PROXY_AUTHENTICATION_USER", "true")
	_, _ = cfg.Section("auth").NewKey("ACTIVE_CODE_LIVE_MINUTES", "10")
	_, _ = cfg.Section("auth").NewKey("RESET_PASSWD_CODE_LIVE_MINUTES", "10")
	_, _ = cfg.Section("auth").NewKey("ENABLE_CAPTCHA", "true")
	_, _ = cfg.Section("auth").NewKey("ENABLE_REGISTRATION_CAPTCHA", "true")
	_, _ = cfg.Section("auth").NewKey("ENABLE_NOTIFY_MAIL", "true")
	_, _ = cfg.Section("auth").NewKey("REGISTER_EMAIL_CONFIRM", "true")
	_, _ = cfg.Section("session").NewKey("GC_INTERVAL_TIME", "10")
	_, _ = cfg.Section("session").NewKey("SESSION_LIFE_TIME", "10")
	_, _ = cfg.Section("server").NewKey("ROOT_URL", "true")
	_, _ = cfg.Section("server").NewKey("LANDING_PAGE", "true")
	_, _ = cfg.Section("database").NewKey("DB_TYPE", "true")
	_, _ = cfg.Section("database").NewKey("PASSWD", "true")
	_, _ = cfg.Section("other").NewKey("SHOW_FOOTER_TEMPLATE_LOAD_TIME", "true")
	_, _ = cfg.Section("email").NewKey("ENABLED", "true")
	_, _ = cfg.Section("server").NewKey("NONEXISTENT_OPTION", "true")

	wantWarnings := []string{
		"option [auth] ACTIVE_CODE_LIVE_MINUTES is invalid",
		"option [auth] ENABLE_CAPTCHA is invalid",
		"option [auth] ENABLE_REGISTRATION_CAPTCHA is deprecated, use [auth] ANTI_BOT_VERIFICATION_ON_SIGN_UP instead",
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
		"option [other] SHOW_FOOTER_TEMPLATE_LOAD_TIME is invalid",
	}

	gotWarnings := checkInvalidOptions(cfg)
	sort.Strings(wantWarnings)
	sort.Strings(gotWarnings)
	assert.Equal(t, wantWarnings, gotWarnings)
}

func TestHandleDeprecatedRegistrationCaptcha(t *testing.T) {
	oldFile, oldAuth := File, Auth
	t.Cleanup(func() { File, Auth = oldFile, oldAuth })

	tests := []struct {
		name       string
		enabled    string // Value of ENABLE_REGISTRATION_CAPTCHA, empty when not set.
		current    string // ANTI_BOT_VERIFICATION_STRATEGY after mapping the configuration.
		signUpOff  bool   // Whether ANTI_BOT_VERIFICATION_ON_SIGN_UP is false after mapping the configuration.
		want       string
		wantSignUp bool
	}{
		{name: "not set keeps altcha", current: "altcha", want: "altcha", wantSignUp: true},
		{name: "true keeps sign-up on", enabled: "true", current: "altcha", want: "altcha", wantSignUp: true},
		{name: "true turns empty to altcha", enabled: "true", current: "", want: "altcha", wantSignUp: true},
		{name: "true keeps captcha", enabled: "true", current: "captcha", want: "captcha", wantSignUp: true},
		{name: "true turns sign-up on", enabled: "true", current: "altcha", signUpOff: true, want: "altcha", wantSignUp: true},
		{name: "false turns sign-up off", enabled: "false", current: "altcha", want: "altcha", wantSignUp: false},
		{name: "false keeps captcha", enabled: "false", current: "captcha", want: "captcha", wantSignUp: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			File = ini.Empty()
			if test.enabled != "" {
				_, _ = File.Section("auth").NewKey("ENABLE_REGISTRATION_CAPTCHA", test.enabled)
			}
			Auth = AuthOpts{AntiBotVerificationStrategy: test.current, AntiBotVerificationOnSignUp: !test.signUpOff}
			handleDeprecated()
			assert.Equal(t, test.want, Auth.AntiBotVerificationStrategy)
			assert.Equal(t, test.wantSignUp, Auth.AntiBotVerificationOnSignUp)
		})
	}
}
