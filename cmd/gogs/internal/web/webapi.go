package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"reflect"
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
	"gogs.io/gogs/internal/route/user"
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

// webAPIValidator is the shared validator instance used by every webapi
// binding. Registering the json-tag name function makes validation errors
// carry the wire field name (e.g. "recoveryCode") via ve.Field(), so the
// 400 payload keys match what the React client sends and reads.
var webAPIValidator = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}()

// bindJSON binds the request body to T. On binding or validation failure it
// short-circuits with a 400 carrying the standard renderBindingErrors payload,
// so downstream handlers can drop the `if len(bindErrs) > 0` boilerplate and
// the binding.Errors parameter entirely.
func bindJSON(model any) flamego.Handler {
	return binding.JSON(model, binding.Options{
		Validator: webAPIValidator,
		ErrorHandler: func(c flamego.Context, l i18n.Locale, errs binding.Errors) {
			w := c.ResponseWriter()
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(renderBindingErrors(l, errs))
		},
	})
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
			f.Combo("/reset-password").
				Get(getUserResetPassword).
				Post(bindJSON(userResetPasswordRequest{}), postUserResetPassword)
			f.Combo("/sign-in").
				Get(getUserSignIn).
				Post(bindJSON(userSignInRequest{}), postUserSignIn)
			f.Group("/mfa", func() {
				f.Combo("").
					Get(getUserMFA).
					Post(bindJSON(userMFARequest{}), postUserMFA)
				f.Post("/recovery", bindJSON(userMFARecoveryRequest{}), postUserMFARecovery)
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

// fieldErrors maps JSON field names to per-field localized messages. A non-nil
// value renders inline under the input. A nil value marks the input as
// invalid (highlight + focus eligibility) without duplicating text. Used in
// concert with bindingErrorResponse.Error to surface one banner message while
// highlighting multiple inputs.
type fieldErrors map[string]*string

// bindingErrorResponse carries form-validation failures. Error is the top-level
// message shown as a banner above the form (used when the failure is not tied
// to a specific input, e.g. malformed body, bad credentials).
type bindingErrorResponse struct {
	Error  string      `json:"error,omitempty"`
	Fields fieldErrors `json:"fields,omitempty"`
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

	out := make(fieldErrors)
	for _, e := range errs {
		var ves validator.ValidationErrors
		ok := errors.As(e.Err, &ves)
		if !ok {
			continue
		}
		for _, ve := range ves {
			field := ve.Field()
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
		log.Error("getUserSignIn: list activated login sources: %v", err)
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
}

type userResetPasswordPageResponse struct {
	Valid bool `json:"valid"`
}

func getUserResetPassword(r *http.Request) (statusCode int, resp *userResetPasswordPageResponse, err error) {
	code := r.URL.Query().Get("code")
	if code == "" {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusOK, &userResetPasswordPageResponse{Valid: user.VerifyUserActiveCode(code) != nil}, nil
}

type userResetPasswordRequest struct {
	Code     string `json:"code" validate:"required"`
	Password string `json:"password" validate:"required,min=6,max=255"`
}

func postUserResetPassword(r *http.Request, l i18n.Locale, req userResetPasswordRequest) (statusCode int, resp any, err error) {
	u := user.VerifyUserActiveCode(req.Code)
	if u == nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("auth.invalid_code")}, nil
	}

	if err := database.Handle.Users().Update(r.Context(), u.ID, database.UpdateUserOptions{Password: &req.Password}); err != nil {
		log.Error("postUserResetPassword: update password for user %q: %v", u.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}

	log.Trace("User password reset: %s", u.Name)
	return http.StatusNoContent, nil, nil
}

type userSignInResponse struct {
	// MFA is true when the account has MFA enabled and the password step
	// succeeded but a second factor is still required. The client should
	// navigate to /user/mfa to complete the challenge.
	MFA bool `json:"mfa,omitempty"`
}

func postUserSignIn(r *http.Request, sess session.Store, mc *macaron.Context, l i18n.Locale, req userSignInRequest) (statusCode int, resp any, err error) {
	u, err := database.Handle.Users().Authenticate(r.Context(), req.Username, req.Password, req.LoginSource)
	if err != nil {
		switch {
		case auth.IsErrBadCredentials(err):
			return http.StatusUnauthorized, &bindingErrorResponse{
				Error:  l.Tr("form.username_password_incorrect"),
				Fields: fieldErrors{"username": nil, "password": nil},
			}, nil
		case database.IsErrLoginSourceMismatch(err):
			return http.StatusUnprocessableEntity, nil, errors.New(l.Tr("form.auth_source_mismatch"))
		default:
			log.Error("postUserSignIn: authenticate user %q: %v", req.Username, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
		}
	}

	if database.Handle.TwoFactors().IsEnabled(r.Context(), u.ID) {
		_ = sess.Set("mfaUserID", u.ID)
		return http.StatusOK, &userSignInResponse{MFA: true}, nil
	}

	completeSignIn(sess, mc, u)
	return http.StatusOK, &userSignInResponse{}, nil
}

// completeSignIn finalizes the sign-in session for u: writes the auth session,
// clears any in-flight MFA state, and sets the login-status cookie. The
// caller is responsible for navigating to a post-login destination via
// /redirect?to=.
func completeSignIn(sess session.Store, mc *macaron.Context, u *database.User) {
	_ = sess.Set("uid", u.ID)
	_ = sess.Set("uname", u.Name)
	_ = sess.Delete("mfaUserID")

	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Security.EnableLoginStatusCookie {
		mc.SetCookie(conf.Security.LoginStatusCookieName, "true", 0, conf.Server.Subpath)
	}
}

func getUserMFA(sess session.Store) (statusCode int, resp any, err error) {
	if _, ok := sess.Get("mfaUserID").(int64); !ok {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusNoContent, nil, nil
}

type userMFARequest struct {
	Passcode string `json:"passcode" validate:"required,len=6"`
}

type userMFAResponse struct{}

func postUserMFA(r *http.Request, sess session.Store, mc *macaron.Context, ca cache.Cache, l i18n.Locale, req userMFARequest) (statusCode int, resp any, err error) {
	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth.mfa_session_expired")}, nil
	}

	t, err := database.Handle.TwoFactors().GetByUserID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFA: get two factor by user ID %d: %v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get two factor by user ID")
	}

	valid, err := t.ValidateTOTP(req.Passcode)
	if err != nil {
		log.Error("postUserMFA: validate TOTP for user %d: %v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "validate TOTP")
	}
	if !valid {
		msg := l.Tr("auth.mfa_invalid_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"passcode": &msg},
		}, nil
	}

	if ca.IsExist(userx.TwoFactorCacheKey(userID, req.Passcode)) {
		msg := l.Tr("auth.mfa_reused_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"passcode": &msg},
		}, nil
	}
	if err = ca.Put(userx.TwoFactorCacheKey(userID, req.Passcode), 1, 60); err != nil {
		log.Error("postUserMFA: cache two factor passcode for user %d: %v", userID, err)
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFA: get user by ID %d: %v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	completeSignIn(sess, mc, u)
	return http.StatusOK, &userMFAResponse{}, nil
}

type userMFARecoveryRequest struct {
	RecoveryCode string `json:"recoveryCode" validate:"required,len=11"`
}

func postUserMFARecovery(r *http.Request, sess session.Store, mc *macaron.Context, l i18n.Locale, req userMFARecoveryRequest) (statusCode int, resp any, err error) {
	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth.mfa_session_expired")}, nil
	}

	if err := database.Handle.TwoFactors().UseRecoveryCode(r.Context(), userID, req.RecoveryCode); err != nil {
		if database.IsTwoFactorRecoveryCodeNotFound(err) {
			msg := l.Tr("auth.mfa_invalid_recovery_code")
			return http.StatusUnauthorized, &bindingErrorResponse{
				Fields: fieldErrors{"recoveryCode": &msg},
			}, nil
		}
		log.Error("postUserMFARecovery: use recovery code for user %d: %v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "use recovery code")
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		log.Error("postUserMFARecovery: get user by ID %d: %v", userID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	completeSignIn(sess, mc, u)
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
	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	return http.StatusNoContent, nil, nil
}
