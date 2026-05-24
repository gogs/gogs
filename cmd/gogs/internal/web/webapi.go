package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/binding"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/flamego/validator"
	"github.com/go-macaron/i18n"
	macaronsession "github.com/go-macaron/session"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/route/user"
	"gogs.io/gogs/internal/userx"
)

type (
	webAPIUserKey    struct{}
	webAPISessionKey struct{}
	webAPIMacaronKey struct{}
	webAPILocaleKey  struct{}
)

func flamegoBridger(webHandler http.Handler) func(c *context.Context, l i18n.Locale) {
	return func(c *context.Context, l i18n.Locale) {
		ctx := c.Req.Context()
		ctx = stdctx.WithValue(ctx, webAPIUserKey{}, c.User)
		ctx = stdctx.WithValue(ctx, webAPISessionKey{}, c.Session)
		ctx = stdctx.WithValue(ctx, webAPIMacaronKey{}, c.Context)
		ctx = stdctx.WithValue(ctx, webAPILocaleKey{}, l)
		webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
	}
}

func flamegoInjector(c flamego.Context) {
	ctx := c.Request().Context()
	user, _ := ctx.Value(webAPIUserKey{}).(*database.User)
	sess, _ := ctx.Value(webAPISessionKey{}).(macaronsession.Store)
	mc, _ := ctx.Value(webAPIMacaronKey{}).(*macaron.Context)
	l, _ := ctx.Value(webAPILocaleKey{}).(i18n.Locale)
	c.Map(user, sess, mc, l)
	c.MapTo(flamegoSessionAdapter{sess: sess}, (*session.Session)(nil))
}

// flamegoSessionAdapter exposes the underlying Macaron session via the Flamego
// session interface so the captcha middleware (and any future Flamego-native
// session consumer) can read/write the same session store the rest of the app
// uses.
type flamegoSessionAdapter struct {
	sess macaronsession.Store
}

func (s flamegoSessionAdapter) ID() string                      { return s.sess.ID() }
func (s flamegoSessionAdapter) Get(key interface{}) interface{} { return s.sess.Get(key) }
func (s flamegoSessionAdapter) Set(key, val interface{})        { _ = s.sess.Set(key, val) }
func (s flamegoSessionAdapter) SetFlash(val interface{})        { _ = s.sess.Set("_flash", val) }
func (s flamegoSessionAdapter) Delete(key interface{})          { _ = s.sess.Delete(key) }
func (s flamegoSessionAdapter) Flush()                          { _ = s.sess.Flush() }
func (s flamegoSessionAdapter) Encode() ([]byte, error)         { return nil, nil }

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
	_ = v.RegisterValidation("alphadashdot", func(fl validator.FieldLevel) bool {
		return !alphaDashDotInvalid.MatchString(fl.Field().String())
	})
	return v
}()

var alphaDashDotInvalid = regexp.MustCompile(`[^\d\w\-_\.]`)

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
			f.Combo("/sign-up").
				Get(getUserSignUp).
				Post(bindJSON(userSignUpRequest{}), postUserSignUp)
			f.Group("/reset-password", func() {
				f.Combo("").
					Get(getUserResetPassword).
					Post(bindJSON(userResetPasswordEmailRequest{}), postUserResetPassword)
				f.Post("/complete", bindJSON(userResetPasswordCompleteRequest{}), postUserResetPasswordComplete)
			})
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
	}, webAPIBodyLimiter)
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
	"required":     "form.require_error",
	"max":          "form.max_size_error",
	"min":          "form.min_size_error",
	"len":          "form.size_error",
	"email":        "form.email_error",
	"url":          "form.url_error",
	"alphadashdot": "form.alpha_dash_dot_error",
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

type getUserSignInResponse struct {
	LoginSources []loginSource `json:"loginSources"`
}

type getUserSignUpResponse struct {
	RegistrationDisabled bool `json:"registrationDisabled"`
	CaptchaEnabled       bool `json:"captchaEnabled"`
}

func getUserSignUp() (statusCode int, resp *getUserSignUpResponse, err error) {
	return http.StatusOK, &getUserSignUpResponse{
		RegistrationDisabled: conf.Auth.DisableRegistration,
		CaptchaEnabled:       conf.Auth.EnableRegistrationCaptcha,
	}, nil
}

type userSignUpRequest struct {
	UserName string `json:"userName" validate:"required,alphadashdot,max=35"`
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,max=255"`
	Captcha  string `json:"captcha"`
}

type userSignUpResponse struct {
	EmailConfirmationRequired bool   `json:"emailConfirmationRequired,omitempty"`
	Email                     string `json:"email,omitempty"`
	Hours                     int    `json:"hours,omitempty"`
}

func postUserSignUp(r *http.Request, mc *macaron.Context, ca cache.Cache, l i18n.Locale, cpt captcha.Captcha, req userSignUpRequest) (statusCode int, resp any, err error) {
	if conf.Auth.DisableRegistration {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth.disable_register_prompt")}, nil
	}
	if conf.Auth.EnableRegistrationCaptcha && !cpt.ValidText(req.Captcha) {
		msg := l.Tr("form.captcha_incorrect")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"captcha": &msg},
		}, nil
	}
	u, err := database.Handle.Users().Create(
		r.Context(),
		req.UserName,
		req.Email,
		database.CreateUserOptions{
			Password:  req.Password,
			Activated: !conf.Auth.RequireEmailConfirmation,
		},
	)
	if err != nil {
		switch {
		case database.IsErrUserAlreadyExist(err):
			msg := l.Tr("form.username_been_taken")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"userName": &msg}}, nil
		case database.IsErrEmailAlreadyUsed(err):
			msg := l.Tr("form.email_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
		case database.IsErrNameNotAllowed(err):
			msg := l.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"userName": &msg}}, nil
		default:
			log.Error("postUserSignUp: create user %q: %v", req.UserName, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "create user")
		}
	}
	log.Trace("Account created: %s", u.Name)

	if database.Handle.Users().Count(r.Context()) == 1 {
		v := true
		err := database.Handle.Users().Update(
			r.Context(),
			u.ID,
			database.UpdateUserOptions{
				IsActivated: &v,
				IsAdmin:     &v,
			},
		)
		if err != nil {
			log.Error("postUserSignUp: update first user %q: %v", u.Name, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
		}
	}

	if conf.Auth.RequireEmailConfirmation && u.ID > 1 {
		if err := email.SendActivateAccountMail(mc, database.NewMailerUser(u)); err != nil {
			log.Error("postUserSignUp: send activation mail to user %q: %v", u.Name, err)
		}
		if err := ca.Set(r.Context(), userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
			log.Error("postUserSignUp: put mail resend cache for user %q: %v", u.Name, err)
		}
		return http.StatusOK, &userSignUpResponse{
			EmailConfirmationRequired: true,
			Email:                     u.Email,
			Hours:                     conf.Auth.ActivateCodeLives / 60,
		}, nil
	}

	return http.StatusOK, &userSignUpResponse{}, nil
}

func getUserSignIn(r *http.Request) (statusCode int, resp *getUserSignInResponse, err error) {
	sources, err := database.Handle.LoginSources().List(r.Context(), database.ListLoginSourceOptions{OnlyActivated: true})
	if err != nil {
		log.Error("getUserSignIn: list activated login sources: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list activated login sources")
	}
	loginSources := make([]loginSource, 0, len(sources))
	for _, s := range sources {
		loginSources = append(loginSources, loginSource{ID: s.ID, Name: s.Name, IsDefault: s.IsDefault})
	}
	return http.StatusOK, &getUserSignInResponse{LoginSources: loginSources}, nil
}

type userSignInRequest struct {
	Username    string `json:"username" validate:"required,max=254"`
	Password    string `json:"password" validate:"required,max=255"`
	LoginSource int64  `json:"loginSource"`
}

type getUserResetPasswordResponse struct {
	EmailEnabled bool `json:"emailEnabled"`
	Valid        bool `json:"valid"`
}

func getUserResetPassword(r *http.Request) (statusCode int, resp *getUserResetPasswordResponse, err error) {
	code := r.URL.Query().Get("code")
	return http.StatusOK, &getUserResetPasswordResponse{
		EmailEnabled: conf.Email.Enabled,
		Valid:        code != "" && user.VerifyUserActiveCode(code) != nil,
	}, nil
}

type userResetPasswordEmailRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

type userResetPasswordCompleteRequest struct {
	Code     string `json:"code" validate:"required"`
	Password string `json:"password" validate:"required,min=6,max=255"`
}

type userResetPasswordResponse struct {
	Hours         int  `json:"hours,omitempty"`
	ResendLimited bool `json:"resendLimited,omitempty"`
}

func postUserResetPassword(r *http.Request, ca cache.Cache, l i18n.Locale, req userResetPasswordEmailRequest) (statusCode int, resp any, err error) {
	if !conf.Email.Enabled {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth.disable_register_mail")}, nil
	}

	ctx := r.Context()
	u, err := database.Handle.Users().GetByEmail(ctx, req.Email)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusOK, &userResetPasswordResponse{Hours: conf.Auth.ActivateCodeLives / 60}, nil
		}
		log.Error("postUserResetPassword: get user by email %q: %v", req.Email, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by email")
	}

	if !u.IsLocal() {
		msg := l.Tr("auth.non_local_account")
		return http.StatusForbidden, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
	}

	if _, err := ca.Get(ctx, userx.MailResendCacheKey(u.ID)); err == nil {
		return http.StatusOK, &userResetPasswordResponse{
			Hours:         conf.Auth.ActivateCodeLives / 60,
			ResendLimited: true,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Error("postUserResetPassword: get mail resend cache for user %q: %v", u.Name, err)
	}

	if err = email.SendResetPasswordMail(l, database.NewMailerUser(u)); err != nil {
		log.Error("postUserResetPassword: send reset password mail to user %q: %v", u.Name, err)
	}
	if err = ca.Set(ctx, userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
		log.Error("postUserResetPassword: put mail resend cache for user %q: %v", u.Name, err)
	}

	return http.StatusOK, &userResetPasswordResponse{Hours: conf.Auth.ActivateCodeLives / 60}, nil
}

func postUserResetPasswordComplete(r *http.Request, l i18n.Locale, req userResetPasswordCompleteRequest) (statusCode int, resp any, err error) {
	u := user.VerifyUserActiveCode(req.Code)
	if u == nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("auth.invalid_code")}, nil
	}

	if err := database.Handle.Users().Update(r.Context(), u.ID, database.UpdateUserOptions{Password: &req.Password}); err != nil {
		log.Error("postUserResetPasswordComplete: update password for user %q: %v", u.Name, err)
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

func postUserSignIn(r *http.Request, sess session.Session, mc *macaron.Context, l i18n.Locale, req userSignInRequest) (statusCode int, resp any, err error) {
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
		sess.Set("mfaUserID", u.ID)
		return http.StatusOK, &userSignInResponse{MFA: true}, nil
	}

	completeSignIn(sess, mc, u)
	return http.StatusOK, &userSignInResponse{}, nil
}

// completeSignIn finalizes the sign-in session for u: writes the auth session,
// clears any in-flight MFA state, and sets the login-status cookie. The
// caller is responsible for navigating to a post-login destination via
// /redirect?to=.
func completeSignIn(sess session.Session, mc *macaron.Context, u *database.User) {
	sess.Set("uid", u.ID)
	sess.Set("uname", u.Name)
	sess.Delete("mfaUserID")

	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Security.EnableLoginStatusCookie {
		mc.SetCookie(conf.Security.LoginStatusCookieName, "true", 0, conf.Server.Subpath)
	}
}

func getUserMFA(sess session.Session) (statusCode int, resp any, err error) {
	if _, ok := sess.Get("mfaUserID").(int64); !ok {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusNoContent, nil, nil
}

type userMFARequest struct {
	Passcode string `json:"passcode" validate:"required,len=6"`
}

type userMFAResponse struct{}

func postUserMFA(r *http.Request, sess session.Session, mc *macaron.Context, ca cache.Cache, l i18n.Locale, req userMFARequest) (statusCode int, resp any, err error) {
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

	cacheKey := userx.TwoFactorCacheKey(userID, req.Passcode)
	if _, err := ca.Get(r.Context(), cacheKey); err == nil {
		msg := l.Tr("auth.mfa_reused_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"passcode": &msg},
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Error("postUserMFA: get two factor passcode cache for user %d: %v", userID, err)
	}
	if err = ca.Set(r.Context(), cacheKey, 1, 60*time.Second); err != nil {
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

func postUserMFARecovery(r *http.Request, sess session.Session, mc *macaron.Context, l i18n.Locale, req userMFARecoveryRequest) (statusCode int, resp any, err error) {
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

type postUserSignOutResponse struct {
	RedirectTo string `json:"redirectTo,omitempty"`
}

func postUserSignOut(sess macaronsession.Store, mc *macaron.Context) (statusCode int, resp *postUserSignOutResponse, err error) {
	_ = sess.Flush()
	_ = sess.Destory(mc)
	mc.SetCookie(conf.Session.CSRFCookieName, "", -1, conf.Server.Subpath)
	if conf.Auth.CustomLogoutURL != "" {
		return http.StatusOK, &postUserSignOutResponse{RedirectTo: conf.Auth.CustomLogoutURL}, nil
	}
	return http.StatusNoContent, nil, nil
}
