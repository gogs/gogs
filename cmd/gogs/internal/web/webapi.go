package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/binding"
	"github.com/flamego/flamego"
	"github.com/flamego/validator"
	"github.com/go-macaron/cache"
	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/urlx"
	"gogs.io/gogs/internal/userx"
)

type (
	webAPIUserKey    struct{}
	webAPISessionKey struct{}
	webAPIMacaronKey struct{}
	webAPILocaleKey  struct{}
	webAPICacheKey   struct{}
)

func bridgeToWebAPI(webHandler http.Handler) func(c *context.Context, l i18n.Locale, ca cache.Cache) {
	return func(c *context.Context, l i18n.Locale, ca cache.Cache) {
		ctx := c.Req.Context()
		ctx = stdctx.WithValue(ctx, webAPIUserKey{}, c.User)
		ctx = stdctx.WithValue(ctx, webAPISessionKey{}, c.Session)
		ctx = stdctx.WithValue(ctx, webAPIMacaronKey{}, c.Context)
		ctx = stdctx.WithValue(ctx, webAPILocaleKey{}, l)
		ctx = stdctx.WithValue(ctx, webAPICacheKey{}, ca)
		webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
	}
}

func webAPIInjector(c flamego.Context) {
	ctx := c.Request().Context()
	user, _ := ctx.Value(webAPIUserKey{}).(*database.User)
	sess, _ := ctx.Value(webAPISessionKey{}).(session.Store)
	mc, _ := ctx.Value(webAPIMacaronKey{}).(*macaron.Context)
	l, _ := ctx.Value(webAPILocaleKey{}).(i18n.Locale)
	ca, _ := ctx.Value(webAPICacheKey{}).(cache.Cache)
	c.Map(user, sess, mc, l, ca)
}

func webAPIBodyLimiter(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 4*1024) // 4 KiB
}

func mountWebAPIRoutes(f *flamego.Flame) {
	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		w := c.ResponseWriter()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			msg := err.Error()
			if statusCode >= http.StatusInternalServerError && conf.IsProdMode() {
				msg = "Internal server error"
			}
			resp = map[string]any{"error": msg}
		}
		if resp == nil {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})

	f.Group("/api/web", func() {
		f.Group("/user", func() {
			f.Get("/info", getUserInfo)
			f.Combo("/sign-in").
				Get(getUserSignIn).
				Post(binding.JSON(userSignInRequest{}), postUserSignIn)
			f.Group("/mfa", func() {
				f.Combo("").
					Get(getUserMFA).
					Post(binding.JSON(userMFARequest{}), postUserMFA)
				f.Post("/recovery", binding.JSON(userMFARecoveryRequest{}), postUserMFARecovery)
			})
			f.Post("/sign-out", postUserSignOut)
		})
	}, webAPIBodyLimiter, webAPIInjector)

	f.Get("/redirect", getRedirect)
}

func getRedirect(c flamego.Context) {
	to := c.Request().URL.Query().Get("to")
	if !urlx.IsSameSite(to) {
		to = conf.Server.Subpath + "/"
	}
	c.Redirect(to, http.StatusSeeOther)
}

// bindingErrorResponse carries form-validation failures. Error is the top-level
// message shown as a banner above the form (used when the failure is not tied to
// a specific input, e.g. malformed body, bad credentials). Fields maps JSON
// field names to per-field localized messages. A non-nil value renders inline
// under the input. nil marks the input as invalid (highlight + focus
// eligibility) without duplicating text. Pair Error with nil entries in Fields
// to surface one banner message while highlighting multiple inputs.
type bindingErrorResponse struct {
	Error  string             `json:"error,omitempty"`
	Fields map[string]*string `json:"fields,omitempty"`
}

// ruleSuffixKeys maps a validator tag to the shared "form.*_error" suffix key
// (e.g. "max" -> "form.max_size_error"). Messages are composed as
// <field label> + <suffix>, mirroring the legacy Macaron binding behavior.
var ruleSuffixKeys = map[string]string{
	"required": "form.require_error",
	"max":      "form.max_size_error",
	"min":      "form.min_size_error",
	"len":      "form.size_error",
	"email":    "form.email_error",
	"url":      "form.url_error",
}

// renderBindingErrors maps binding.Errors to the response shape, looking up
// localized messages via the request's locale. The per-field label comes from
// "form.<StructField>" (e.g. "form.UserName"); the rule suffix comes from
// ruleSuffixKeys. Rule parameters (e.g. "254" for `max=254`) are passed
// through to the suffix translation for %s expansion. Always HTTP 400.
func renderBindingErrors(l i18n.Locale, errs binding.Errors) *bindingErrorResponse {
	for _, e := range errs {
		if e.Category == binding.ErrorCategoryDeserialization {
			return &bindingErrorResponse{Error: l.Tr("form.invalid_request") + ": " + e.Err.Error()}
		}
	}

	out := make(map[string]*string)
	for _, e := range errs {
		var ves validator.ValidationErrors
		ok := errors.As(e.Err, &ves)
		if !ok {
			continue
		}
		for _, ve := range ves {
			field := strings.ToLower(ve.StructField())
			if _, exists := out[field]; exists {
				// Keep the first rule that failed for a given field so the client renders one
				// message per input. Subsequent rules surface only after the first is fixed.
				continue
			}
			label := l.Tr("form." + ve.StructField())
			suffixKey, known := ruleSuffixKeys[ve.Tag()]
			var msg string
			switch {
			case !known:
				msg = l.Tr("form.unknown_error") + " " + ve.Tag()
			case ve.Param() != "":
				msg = label + l.Tr(suffixKey, ve.Param())
			default:
				msg = label + l.Tr(suffixKey)
			}
			out[field] = &msg
		}
	}
	return &bindingErrorResponse{Fields: out}
}

type loginSource struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type userSignInPageResponse struct {
	LoginSources []loginSource `json:"loginSources"`
}

func getUserSignIn(r *http.Request) (statusCode int, resp *userSignInPageResponse, err error) {
	sources, err := database.Handle.LoginSources().List(r.Context(), database.ListLoginSourceOptions{OnlyActivated: true})
	if err != nil {
		log.Error("getUserSignIn: list activated login sources: %+v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list activated login sources")
	}
	loginSources := make([]loginSource, 0, len(sources))
	for _, s := range sources {
		loginSources = append(loginSources, loginSource{ID: s.ID, Name: s.Name, IsDefault: s.IsDefault})
	}
	return http.StatusOK, &userSignInPageResponse{LoginSources: loginSources}, nil
}

type userSignInRequest struct {
	Username    string `json:"username" validate:"required,max=254"`
	Password    string `json:"password" validate:"required,max=255"`
	LoginSource int64  `json:"loginSource"`
	Remember    bool   `json:"remember"`
}

type userSignInResponse struct {
	// MFA is true when the account has MFA enabled and the password step
	// succeeded but a second factor is still required. The client should
	// navigate to /user/mfa to complete the challenge.
	MFA bool `json:"mfa,omitempty"`
}

func postUserSignIn(r *http.Request, sess session.Store, mc *macaron.Context, l i18n.Locale, req userSignInRequest, bindErrs binding.Errors) (statusCode int, resp any, err error) {
	if len(bindErrs) > 0 {
		return http.StatusBadRequest, renderBindingErrors(l, bindErrs), nil
	}

	u, err := database.Handle.Users().Authenticate(r.Context(), req.Username, req.Password, req.LoginSource)
	if err != nil {
		switch {
		case auth.IsErrBadCredentials(err):
			return http.StatusUnauthorized, &bindingErrorResponse{
				Error:  l.Tr("form.username_password_incorrect"),
				Fields: map[string]*string{"username": nil, "password": nil},
			}, nil
		case database.IsErrLoginSourceMismatch(err):
			return http.StatusUnprocessableEntity, nil, errors.New(l.Tr("form.auth_source_mismatch"))
		default:
			log.Error("postUserSignIn: authenticate user %q: %+v", req.Username, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
		}
	}

	if database.Handle.TwoFactors().IsEnabled(r.Context(), u.ID) {
		_ = sess.Set("mfaRemember", req.Remember)
		_ = sess.Set("mfaUserID", u.ID)
		return http.StatusOK, &userSignInResponse{MFA: true}, nil
	}

	completeSignIn(sess, mc, u, req.Remember)
	return http.StatusOK, &userSignInResponse{}, nil
}

// completeSignIn finalizes the sign-in session for u: writes the auth session,
// clears any in-flight MFA session, and sets remember-me / login-status
// cookies. The caller is responsible for navigating to a post-login
// destination via /redirect?to=.
func completeSignIn(sess session.Store, mc *macaron.Context, u *database.User, remember bool) {
	if remember {
		days := 86400 * conf.Security.LoginRememberDays
		mc.SetCookie(conf.Security.CookieUsername, u.Name, days, conf.Server.Subpath, "", conf.Security.CookieSecure, true)
		mc.SetSuperSecureCookie(u.Rands+u.Password, conf.Security.CookieRememberName, u.Name, days, conf.Server.Subpath, "", conf.Security.CookieSecure, true)
	}

	_ = sess.Set("uid", u.ID)
	_ = sess.Set("uname", u.Name)
	_ = sess.Delete("mfaRemember")
	_ = sess.Delete("mfaUserID")

	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Security.EnableLoginStatusCookie {
		mc.SetCookie(conf.Security.LoginStatusCookieName, "true", 0, conf.Server.Subpath)
	}
}

type userMFAPageResponse struct {
	Active bool `json:"active"`
}

func getUserMFA(sess session.Store) (statusCode int, resp *userMFAPageResponse, err error) {
	_, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusOK, &userMFAPageResponse{Active: true}, nil
}

type userMFARequest struct {
	Passcode string `json:"passcode" validate:"required,max=16"`
}

type userMFAResponse struct{}

func postUserMFA(r *http.Request, sess session.Store, mc *macaron.Context, ca cache.Cache, l i18n.Locale, req userMFARequest, bindErrs binding.Errors) (statusCode int, resp any, err error) {
	if len(bindErrs) > 0 {
		return http.StatusBadRequest, renderBindingErrors(l, bindErrs), nil
	}

	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth.login_two_factor_session_expired")}, nil
	}

	t, err := database.Handle.TwoFactors().GetByUserID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFA: get two factor by user ID %d: %+v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get two factor by user ID")
	}

	valid, err := t.ValidateTOTP(req.Passcode)
	if err != nil {
		log.Error("postUserMFA: validate TOTP for user %d: %+v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "validate TOTP")
	}
	if !valid {
		return http.StatusUnauthorized, &bindingErrorResponse{
			Error:  l.Tr("settings.two_factor_invalid_passcode"),
			Fields: map[string]*string{"passcode": nil},
		}, nil
	}

	if ca.IsExist(userx.TwoFactorCacheKey(userID, req.Passcode)) {
		return http.StatusUnauthorized, &bindingErrorResponse{
			Error:  l.Tr("settings.two_factor_reused_passcode"),
			Fields: map[string]*string{"passcode": nil},
		}, nil
	}
	if err = ca.Put(userx.TwoFactorCacheKey(userID, req.Passcode), 1, 60); err != nil {
		log.Error("postUserMFA: cache two factor passcode for user %d: %v", userID, err)
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFA: get user by ID %d: %+v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	remember, _ := sess.Get("mfaRemember").(bool)
	completeSignIn(sess, mc, u, remember)
	return http.StatusOK, &userMFAResponse{}, nil
}

type userMFARecoveryRequest struct {
	RecoveryCode string `json:"recoveryCode" validate:"required,max=64"`
}

func postUserMFARecovery(r *http.Request, sess session.Store, mc *macaron.Context, l i18n.Locale, req userMFARecoveryRequest, bindErrs binding.Errors) (statusCode int, resp any, err error) {
	if len(bindErrs) > 0 {
		return http.StatusBadRequest, renderBindingErrors(l, bindErrs), nil
	}

	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth.login_two_factor_session_expired")}, nil
	}

	if err := database.Handle.TwoFactors().UseRecoveryCode(r.Context(), userID, req.RecoveryCode); err != nil {
		if database.IsTwoFactorRecoveryCodeNotFound(err) {
			return http.StatusUnauthorized, &bindingErrorResponse{
				Error:  l.Tr("auth.login_two_factor_invalid_recovery_code"),
				Fields: map[string]*string{"recoveryCode": nil},
			}, nil
		}
		log.Error("postUserMFARecovery: use recovery code for user %d: %+v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "use recovery code")
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFARecovery: get user by ID %d: %+v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	remember, _ := sess.Get("mfaRemember").(bool)
	completeSignIn(sess, mc, u, remember)
	return http.StatusOK, &userMFAResponse{}, nil
}

type userInfo struct {
	Username              string `json:"username"`
	AvatarURL             string `json:"avatarURL"`
	IsAdmin               bool   `json:"isAdmin"`
	CanCreateOrganization bool   `json:"canCreateOrganization"`
}

func getUserInfo(user *database.User) (statusCode int, resp *userInfo, err error) {
	if user == nil {
		return http.StatusNoContent, nil, nil
	}
	return http.StatusOK,
		&userInfo{
			Username:              user.Name,
			AvatarURL:             user.AvatarURL(),
			IsAdmin:               user.IsAdmin,
			CanCreateOrganization: user.CanCreateOrganization(),
		},
		nil
}

func postUserSignOut(sess session.Store, mc *macaron.Context) (statusCode int, resp any, err error) {
	_ = sess.Flush()
	_ = sess.Destory(mc)
	mc.SetCookie(conf.Security.CookieUsername, "", -1, conf.Server.Subpath)
	mc.SetCookie(conf.Security.CookieRememberName, "", -1, conf.Server.Subpath)
	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	return http.StatusNoContent, nil, nil
}
