package web

import (
	"cmp"
	stdctx "context"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/session"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/userx"
)

func parseUserFromCode(ctx stdctx.Context, logger *logx.Logger, code string) (user *database.User) {
	if len(code) <= userx.TimeLimitCodeLength {
		return nil
	}

	hexStr := code[userx.TimeLimitCodeLength:]
	if b, err := hex.DecodeString(hexStr); err == nil {
		if user, err = database.Handle.Users().GetByUsername(ctx, string(b)); user != nil {
			return user
		} else if !database.IsErrUserNotExist(err) {
			logger.Error("Failed to get user by username", "username", string(b), "error", err)
		}
	}
	return nil
}

// verifyUserActivateCode returns the user that the activation code is for, or
// nil when the code is not signed with secretKey or is older than minutes.
func verifyUserActivateCode(ctx stdctx.Context, logger *logx.Logger, secretKey string, minutes int, code string) *database.User {
	user := parseUserFromCode(ctx, logger, code)
	if user != nil && userx.VerifyActivateCode(secretKey, minutes, code, user.ID, user.Email, user.LowerName, user.Password, user.Rands) {
		return user
	}
	return nil
}

// verifyUserResetPasswordCode returns the user that the code to reset the
// password is for, or nil when the code is not signed with secretKey or is older
// than minutes.
func verifyUserResetPasswordCode(ctx stdctx.Context, logger *logx.Logger, secretKey string, minutes int, code string) *database.User {
	user := parseUserFromCode(ctx, logger, code)
	if user != nil && userx.VerifyResetPasswordCode(secretKey, minutes, code, user.ID, user.Email, user.LowerName, user.Password, user.Rands) {
		return user
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
	// AntiBotVerification is empty when sign-ups are not verified.
	AntiBotVerification string `json:"antiBotVerification"`
}

func getUserSignUp(auth conf.AuthOpts) (statusCode int, resp *getUserSignUpResponse, err error) {
	return http.StatusOK, &getUserSignUpResponse{
		RegistrationDisabled: auth.DisableRegistration,
		AntiBotVerification:  auth.AntiBotVerification,
	}, nil
}

type userSignUpRequest struct {
	UserName string `json:"userName" validate:"required,alphadashdot,max=35"`
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,max=255"`
	// AntiBotPayload is the text of the captcha image, or the solution of the altcha
	// challenge, depending on [auth] ANTI_BOT_VERIFICATION.
	AntiBotPayload string `json:"antiBotPayload"`
}

type userSignUpResponse struct {
	EmailConfirmationRequired bool   `json:"emailConfirmationRequired,omitempty"`
	Email                     string `json:"email,omitempty"`
	Hours                     int    `json:"hours,omitempty"`
}

func postUserSignUp(logger *logx.Logger, emailOpts email.Options, sec conf.SecurityOpts, auth conf.AuthOpts, r *http.Request, ca cache.Cache, l i18n.Locale, verifyAntiBot antiBotVerifier, req userSignUpRequest) (statusCode int, resp any, err error) {
	if auth.DisableRegistration {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth::disable_register_prompt")}, nil
	}
	if !verifyAntiBot(req.AntiBotPayload) {
		msg := l.Tr("form::captcha_incorrect")
		if auth.AntiBotVerification == conf.AntiBotVerificationAltcha {
			msg = l.Tr("form::anti_bot_verification_failed")
		}
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"antiBotPayload": &msg},
		}, nil
	}
	u, err := database.Handle.Users().Create(
		r.Context(),
		req.UserName,
		req.Email,
		database.CreateUserOptions{
			Password:  req.Password,
			Activated: !auth.RequireEmailConfirmation,
		},
	)
	if err != nil {
		switch {
		case database.IsErrUserAlreadyExist(err):
			msg := l.Tr("form::username_been_taken")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"userName": &msg}}, nil
		case database.IsErrEmailAlreadyUsed(err):
			msg := l.Tr("form::email_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
		case database.IsErrNameNotAllowed(err):
			msg := l.Tr("user::form.name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value())
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"userName": &msg}}, nil
		default:
			logger.Error("Failed to create user", "username", req.UserName, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "create user")
		}
	}
	logger.Debug("Account created", "username", u.Name)

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
			logger.Error("Failed to activate first user as admin", "username", u.Name, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
		}
	}

	if auth.RequireEmailConfirmation && u.ID > 1 {
		if err := sendActivateAccountMail(logger, emailOpts, sec.SecretKey, l, u); err != nil {
			logger.Error("Failed to send activation mail", "username", u.Name, "error", err)
		}
		if err := ca.Set(r.Context(), userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
			logger.Error("Failed to set mail resend cache", "username", u.Name, "error", err)
		}
		return http.StatusOK, &userSignUpResponse{
			EmailConfirmationRequired: true,
			Email:                     u.Email,
			Hours:                     auth.ActivateCodeLives / 60,
		}, nil
	}

	return http.StatusOK, &userSignUpResponse{}, nil
}

// sendActivateAccountMail sends the user the code that activates the account,
// which is signed with secretKey.
func sendActivateAccountMail(logger *logx.Logger, emailOpts email.Options, secretKey string, l i18n.Locale, u *database.User) error {
	code := userx.GenerateActivateCode(secretKey, emailOpts.ActivateCodeLives, u.ID, u.Email, u.Name, u.Password, u.Rands)
	return email.SendActivateAccountMail(logger, emailOpts, l, database.NewMailerUser(u), code)
}

func getUserSignIn(logger *logx.Logger, r *http.Request) (statusCode int, resp *getUserSignInResponse, err error) {
	sources, err := database.Handle.LoginSources().List(r.Context(), database.ListLoginSourceOptions{OnlyActivated: true})
	if err != nil {
		logger.Error("Failed to list activated login sources", "error", err)
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

func getUserResetPassword(logger *logx.Logger, sec conf.SecurityOpts, auth conf.AuthOpts, emailOpts conf.EmailOpts, r *http.Request) (statusCode int, resp *getUserResetPasswordResponse, err error) {
	code := r.URL.Query().Get("code")
	return http.StatusOK, &getUserResetPasswordResponse{
		EmailEnabled: emailOpts.Enabled,
		Valid:        code != "" && verifyUserResetPasswordCode(r.Context(), logger, sec.SecretKey, auth.ResetPasswordCodeLives, code) != nil,
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

func postUserResetPassword(logger *logx.Logger, sec conf.SecurityOpts, auth conf.AuthOpts, emailOpts email.Options, r *http.Request, ca cache.Cache, l i18n.Locale, req userResetPasswordEmailRequest) (statusCode int, resp any, err error) {
	if !emailOpts.Email.Enabled {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth::disable_register_mail")}, nil
	}

	ctx := r.Context()
	u, err := database.Handle.Users().GetByEmail(ctx, req.Email)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return http.StatusOK, &userResetPasswordResponse{Hours: auth.ResetPasswordCodeLives / 60}, nil
		}
		logger.Error("Failed to get user by email", "email", req.Email, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by email")
	}

	if !u.IsLocal() {
		msg := l.Tr("auth::non_local_account")
		return http.StatusForbidden, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
	}

	if _, err := ca.Get(ctx, userx.MailResendCacheKey(u.ID)); err == nil {
		return http.StatusOK, &userResetPasswordResponse{
			Hours:         auth.ResetPasswordCodeLives / 60,
			ResendLimited: true,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		logger.Error("Failed to get mail resend cache", "username", u.Name, "error", err)
	}

	code := userx.GenerateResetPasswordCode(sec.SecretKey, auth.ResetPasswordCodeLives, u.ID, u.Email, u.Name, u.Password, u.Rands)
	if err = email.SendResetPasswordMail(logger, emailOpts, l, database.NewMailerUser(u), code); err != nil {
		logger.Error("Failed to send reset password mail", "username", u.Name, "error", err)
	}
	if err = ca.Set(ctx, userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
		logger.Error("Failed to set mail resend cache", "username", u.Name, "error", err)
	}

	return http.StatusOK, &userResetPasswordResponse{Hours: auth.ResetPasswordCodeLives / 60}, nil
}

func postUserResetPasswordComplete(logger *logx.Logger, sec conf.SecurityOpts, auth conf.AuthOpts, r *http.Request, l i18n.Locale, req userResetPasswordCompleteRequest) (statusCode int, resp any, err error) {
	u := verifyUserResetPasswordCode(r.Context(), logger, sec.SecretKey, auth.ResetPasswordCodeLives, req.Code)
	if u == nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("auth::invalid_code")}, nil
	}

	if err := database.Handle.Users().Update(r.Context(), u.ID, database.UpdateUserOptions{Password: &req.Password}); err != nil {
		logger.Error("Failed to update user password", "username", u.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}

	logger.Debug("User password reset", "username", u.Name)
	return http.StatusNoContent, nil, nil
}

type userSignInResponse struct {
	// MFA is true when the account has MFA enabled and the password step
	// succeeded but a second factor is still required. The client should
	// navigate to /user/mfa to complete the challenge.
	MFA bool `json:"mfa,omitempty"`
}

func postUserSignIn(logger *logx.Logger, cookies signInCookies, r *http.Request, w http.ResponseWriter, store session.Store, sess session.Session, l i18n.Locale, req userSignInRequest) (statusCode int, resp any, err error) {
	u, err := database.Handle.Users().Authenticate(r.Context(), logger, req.Username, req.Password, req.LoginSource)
	if err != nil {
		switch {
		case auth.IsErrBadCredentials(err):
			return http.StatusUnauthorized, &bindingErrorResponse{
				Error:  l.Tr("form::username_password_incorrect"),
				Fields: fieldErrors{"username": nil, "password": nil},
			}, nil
		case database.IsErrLoginSourceMismatch(err):
			return http.StatusUnprocessableEntity, nil, errors.New(l.Tr("form::auth_source_mismatch"))
		case database.IsErrNameNotAllowed(err):
			// The authentication source gave a name that cannot be used for the new
			// account.
			return http.StatusUnprocessableEntity, nil, errors.New(l.Tr("user::form.name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value()))
		default:
			logger.Error("Failed to authenticate user", "username", req.Username, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
		}
	}

	if database.Handle.TwoFactors().IsEnabled(r.Context(), logger, u.ID) {
		sess.Set("mfaUserID", u.ID)
		return http.StatusOK, &userSignInResponse{MFA: true}, nil
	}

	if err = completeSignIn(w, r, cookies, store, sess, u); err != nil {
		logger.Error("Failed to complete sign-in", "userID", u.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "complete sign-in")
	}
	return http.StatusOK, &userSignInResponse{}, nil
}

// signInCookies are the cookies that signing in and out set.
type signInCookies struct {
	// path is where the cookies are sent, i.e., the subpath of the site.
	path string
	// session is the name of the session cookie.
	session string
	// loginStatus is the name of the cookie that tells whether the viewer is
	// authenticated, or empty when it is not set.
	loginStatus string
}

// newSignInCookies returns the cookies of a site under subpath.
func newSignInCookies(sessionOpts conf.SessionOpts, sec conf.SecurityOpts, subpath string) signInCookies {
	cookies := signInCookies{path: cmp.Or(subpath, "/"), session: sessionOpts.CookieName}
	if sec.EnableLoginStatusCookie {
		cookies.loginStatus = sec.LoginStatusCookieName
	}
	return cookies
}

// completeSignIn signs in u under a new session, which drops any in-flight MFA
// state, and sets the login-status cookie when it is enabled. The caller is
// responsible for navigating to a post-login destination via /redirect?to=.
func completeSignIn(w http.ResponseWriter, r *http.Request, cookies signInCookies, store session.Store, sess session.Session, u *database.User) error {
	// Sign in under a new session ID, so a session ID planted in the browser
	// before signing in (session fixation) is not authenticated too. The old
	// session is deleted rather than carried over.
	if err := store.Destroy(r.Context(), sess.ID()); err != nil {
		return errors.Wrap(err, "destroy session")
	}
	sess.Flush()
	// The session middleware has set the cookie when it started a new session,
	// which would leave two session cookies on the response.
	header := w.Header()
	setCookies := header.Values("Set-Cookie")
	header.Del("Set-Cookie")
	for _, c := range setCookies {
		if !strings.HasPrefix(c, cookies.session+"=") {
			header.Add("Set-Cookie", c)
		}
	}
	if err := sess.RegenerateID(w, r); err != nil {
		return errors.Wrap(err, "regenerate session ID")
	}
	sess.Set("uid", u.ID)
	sess.Set("uname", u.Name)

	if cookies.loginStatus != "" {
		http.SetCookie(w, &http.Cookie{
			Name:  cookies.loginStatus,
			Value: "true",
			Path:  cookies.path,
		})
	}
	return nil
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

func postUserMFA(logger *logx.Logger, sec conf.SecurityOpts, cookies signInCookies, r *http.Request, w http.ResponseWriter, store session.Store, sess session.Session, ca cache.Cache, l i18n.Locale, req userMFARequest) (statusCode int, resp any, err error) {
	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth::mfa_session_expired")}, nil
	}

	t, err := database.Handle.TwoFactors().GetByUserID(r.Context(), userID)
	if err != nil {
		logger.Error("Failed to get two factor by user ID", "userID", userID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get two factor by user ID")
	}

	valid, err := t.ValidateTOTP(sec.SecretKey, req.Passcode)
	if err != nil {
		logger.Error("Failed to validate TOTP", "userID", userID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "validate TOTP")
	}
	if !valid {
		msg := l.Tr("auth::mfa_invalid_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"passcode": &msg},
		}, nil
	}

	cacheKey := userx.TwoFactorCacheKey(userID, req.Passcode)
	if _, err := ca.Get(r.Context(), cacheKey); err == nil {
		msg := l.Tr("auth::mfa_reused_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{
			Fields: fieldErrors{"passcode": &msg},
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		logger.Error("Failed to get two factor passcode cache", "userID", userID, "error", err)
	}
	if err = ca.Set(r.Context(), cacheKey, 1, 60*time.Second); err != nil {
		logger.Error("Failed to cache two factor passcode", "userID", userID, "error", err)
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		logger.Error("Failed to get user by ID", "userID", userID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	if err = completeSignIn(w, r, cookies, store, sess, u); err != nil {
		logger.Error("Failed to complete sign-in", "userID", u.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "complete sign-in")
	}
	return http.StatusOK, &userMFAResponse{}, nil
}

type userMFARecoveryRequest struct {
	RecoveryCode string `json:"recoveryCode" validate:"required,len=11"`
}

func postUserMFARecovery(logger *logx.Logger, cookies signInCookies, r *http.Request, w http.ResponseWriter, store session.Store, sess session.Session, l i18n.Locale, req userMFARecoveryRequest) (statusCode int, resp any, err error) {
	userID, ok := sess.Get("mfaUserID").(int64)
	if !ok {
		return http.StatusUnauthorized, &bindingErrorResponse{Error: l.Tr("auth::mfa_session_expired")}, nil
	}

	if err := database.Handle.TwoFactors().UseRecoveryCode(r.Context(), userID, req.RecoveryCode); err != nil {
		if database.IsTwoFactorRecoveryCodeNotFound(err) {
			msg := l.Tr("auth::mfa_invalid_recovery_code")
			return http.StatusUnauthorized, &bindingErrorResponse{
				Fields: fieldErrors{"recoveryCode": &msg},
			}, nil
		}
		logger.Error("Failed to use recovery code", "userID", userID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "use recovery code")
	}

	u, err := database.Handle.Users().GetByID(r.Context(), userID)
	if err != nil {
		logger.Error("Failed to get user by ID", "userID", userID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get user by ID")
	}

	if err = completeSignIn(w, r, cookies, store, sess, u); err != nil {
		logger.Error("Failed to complete sign-in", "userID", u.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "complete sign-in")
	}
	return http.StatusOK, &userMFAResponse{}, nil
}

type userInfo struct {
	Username              string `json:"username"`
	AvatarURL             string `json:"avatarURL"`
	IsAdmin               bool   `json:"isAdmin"`
	CanCreateOrganization bool   `json:"canCreateOrganization"`
}

func getUserInfo(admin conf.AdminOpts, links database.LinkOptions, user *database.User) (statusCode int, resp *userInfo, err error) {
	if user == nil {
		return http.StatusNoContent, nil, nil
	}
	return http.StatusOK,
		&userInfo{
			Username:              user.Name,
			AvatarURL:             user.AvatarURL(links),
			IsAdmin:               user.IsAdmin,
			CanCreateOrganization: user.CanCreateOrganization(admin.DisableRegularOrgCreation),
		},
		nil
}

type getUserActivateResponse struct {
	Email             string `json:"email,omitempty"`
	CodeLifetimeHours int    `json:"codeLifetimeHours,omitempty"`
}

func getUserActivate(auth conf.AuthOpts, u *database.User) (statusCode int, resp any, err error) {
	if u == nil {
		return http.StatusUnauthorized, nil, nil
	}
	// An already-active and authenticated user has no business on the activation page.
	if u.IsActive {
		return http.StatusNotFound, nil, nil
	}
	return http.StatusOK, &getUserActivateResponse{
		Email:             u.Email,
		CodeLifetimeHours: auth.ActivateCodeLives / 60,
	}, nil
}

type postUserActivateResponse struct {
	RateLimited       bool `json:"rateLimited,omitempty"`
	CodeLifetimeHours int  `json:"codeLifetimeHours,omitempty"`
}

func postUserActivate(logger *logx.Logger, emailOpts email.Options, sec conf.SecurityOpts, auth conf.AuthOpts, r *http.Request, u *database.User, ca cache.Cache, l i18n.Locale) (statusCode int, resp any, err error) {
	if u == nil {
		return http.StatusUnauthorized, nil, nil
	}
	if u.IsActive {
		return http.StatusNotFound, nil, nil
	}
	if !auth.RequireEmailConfirmation {
		return http.StatusForbidden, &bindingErrorResponse{Error: l.Tr("auth::disable_register_mail")}, nil
	}

	ctx := r.Context()
	if _, err := ca.Get(ctx, userx.MailResendCacheKey(u.ID)); err == nil {
		return http.StatusOK, &postUserActivateResponse{
			RateLimited:       true,
			CodeLifetimeHours: auth.ActivateCodeLives / 60,
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		logger.Error("Failed to get mail resend cache", "username", u.Name, "error", err)
	}

	if err := sendActivateAccountMail(logger, emailOpts, sec.SecretKey, l, u); err != nil {
		logger.Error("Failed to send activation mail", "username", u.Name, "error", err)
	}
	if err := ca.Set(ctx, userx.MailResendCacheKey(u.ID), 1, 180*time.Second); err != nil {
		logger.Error("Failed to set mail resend cache", "username", u.Name, "error", err)
	}
	return http.StatusOK, &postUserActivateResponse{CodeLifetimeHours: auth.ActivateCodeLives / 60}, nil
}

type userActivateCompleteRequest struct {
	Code string `json:"code" validate:"required"`
}

func postUserActivateComplete(logger *logx.Logger, sec conf.SecurityOpts, auth conf.AuthOpts, cookies signInCookies, r *http.Request, w http.ResponseWriter, store session.Store, sess session.Session, l i18n.Locale, req userActivateCompleteRequest) (statusCode int, resp any, err error) {
	target := verifyUserActivateCode(r.Context(), logger, sec.SecretKey, auth.ActivateCodeLives, req.Code)
	if target == nil || target.IsActive {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("auth::invalid_code")}, nil
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
		logger.Error("Failed to activate user", "username", target.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}

	logger.Debug("User activated", "username", target.Name)

	// Holding the activation code only proves access to the mailbox, which is not
	// enough to skip the second factor.
	if !database.Handle.TwoFactors().IsEnabled(r.Context(), logger, target.ID) {
		if err := completeSignIn(w, r, cookies, store, sess, target); err != nil {
			logger.Error("Failed to complete sign-in", "userID", target.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "complete sign-in")
		}
	}
	return http.StatusNoContent, nil, nil
}

type postUserSignOutResponse struct {
	RedirectTo string `json:"redirectTo,omitempty"`
}

func postUserSignOut(w http.ResponseWriter, auth conf.AuthOpts, cookies signInCookies, sess session.Session) (statusCode int, resp *postUserSignOutResponse, err error) {
	sess.Flush()
	http.SetCookie(w, &http.Cookie{
		Name:     cookies.session,
		Path:     cookies.path,
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	if auth.CustomLogoutURL != "" {
		return http.StatusOK, &postUserSignOutResponse{RedirectTo: auth.CustomLogoutURL}, nil
	}
	return http.StatusNoContent, nil, nil
}
