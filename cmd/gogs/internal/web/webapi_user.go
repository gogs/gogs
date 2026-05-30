package web

import (
	stdctx "context"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/session"
	"github.com/go-macaron/i18n"
	macaronsession "github.com/go-macaron/session"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

func parseUserFromCode(ctx stdctx.Context, code string) (user *database.User) {
	if len(code) <= tool.TimeLimitCodeLength {
		return nil
	}

	hexStr := code[tool.TimeLimitCodeLength:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = database.Handle.Users().GetByUsername(ctx, string(b)); user != nil {
			return user
		} else if !database.IsErrUserNotExist(err) {
			log.Error("parseUserFromCode: get user by name %q: %v", string(b), err)
		}
	}
	return nil
}

func verifyUserActiveCode(ctx stdctx.Context, code string) (user *database.User) {
	if user = parseUserFromCode(ctx, code); user != nil {
		prefix := code[:tool.TimeLimitCodeLength]
		data := strconv.FormatInt(user.ID, 10) + user.Email + user.LowerName + user.Password + user.Rands
		if tool.VerifyTimeLimitCode(data, conf.Auth.ActivateCodeLives, prefix) {
			return user
		}
	}
	return nil
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
		Valid:        code != "" && verifyUserActiveCode(r.Context(), code) != nil,
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
	u := verifyUserActiveCode(r.Context(), req.Code)
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

type getUserActivateResponse struct {
	Email             string `json:"email,omitempty"`
	CodeLifetimeHours int    `json:"codeLifetimeHours,omitempty"`
}

func getUserActivate(u *database.User) (statusCode int, resp any, err error) {
	if u == nil {
		return http.StatusUnauthorized, nil, nil
	}
	// An already-active and authenticated user has no business on the activation page.
	if u.IsActive {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusOK, &getUserActivateResponse{
		Email:             u.Email,
		CodeLifetimeHours: conf.Auth.ActivateCodeLives / 60,
	}, nil
}

type postUserActivateResponse struct {
	RateLimited       bool `json:"rateLimited,omitempty"`
	CodeLifetimeHours int  `json:"codeLifetimeHours,omitempty"`
}

func postUserActivate(r *http.Request, u *database.User, mc *macaron.Context, ca cache.Cache, l i18n.Locale) (statusCode int, resp any, err error) {
	if u == nil {
		return http.StatusUnauthorized, nil, nil
	}
	if u.IsActive {
		return http.StatusNotFound, nil, nil
	}
	if !conf.Auth.RequireEmailConfirmation {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth.disable_register_mail")}, nil
	}

	ctx := r.Context()
	if _, err := ca.Get(ctx, userx.MailResendCacheKey(u.ID)); err == nil {
		return http.StatusOK, &postUserActivateResponse{
			RateLimited:       true,
			CodeLifetimeHours: conf.Auth.ActivateCodeLives / 60,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Error("postUserActivate: get mail resend cache for user %q: %v", u.Name, err)
	}

	if err := email.SendActivateAccountMail(mc, database.NewMailerUser(u)); err != nil {
		log.Error("postUserActivate: send activation mail to user %q: %v", u.Name, err)
	}
	if err := ca.Set(ctx, userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
		log.Error("postUserActivate: put mail resend cache for user %q: %v", u.Name, err)
	}
	return http.StatusOK, &postUserActivateResponse{CodeLifetimeHours: conf.Auth.ActivateCodeLives / 60}, nil
}

type userActivateCompleteRequest struct {
	Code string `json:"code" validate:"required"`
}

func postUserActivateComplete(r *http.Request, sess session.Session, mc *macaron.Context, l i18n.Locale, req userActivateCompleteRequest) (statusCode int, resp any, err error) {
	target := verifyUserActiveCode(r.Context(), req.Code)
	if target == nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("auth.invalid_code")}, nil
	}

	v := true
	if err := database.Handle.Users().Update(
		r.Context(),
		target.ID,
		database.UpdateUserOptions{
			GenerateNewRands: true,
			IsActivated:      &v,
		},
	); err != nil {
		log.Error("postUserActivateComplete: update user %q: %v", target.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}

	log.Trace("User activated: %s", target.Name)
	completeSignIn(sess, mc, target)
	return http.StatusNoContent, nil, nil
}

type postUserSignOutResponse struct {
	RedirectTo string `json:"redirectTo,omitempty"`
}

func postUserSignOut(sess macaronsession.Store, mc *macaron.Context) (statusCode int, resp *postUserSignOutResponse, err error) {
	_ = sess.Flush()
	_ = sess.Destory(mc)
	if conf.Auth.CustomLogoutURL != "" {
		return http.StatusOK, &postUserSignOutResponse{RedirectTo: conf.Auth.CustomLogoutURL}, nil
	}
	return http.StatusNoContent, nil, nil
}
