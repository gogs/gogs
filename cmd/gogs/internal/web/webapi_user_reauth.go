package web

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/go-macaron/i18n"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/userx"
)

// recentAuthSessionKey is the session key of the Unix time until which the
// signed-in user may take actions that need them to have recently confirmed
// who they are, e.g., adding an SSH key or deleting a repository.
const recentAuthSessionKey = "recentAuthUntil"

// recentAuthDuration is how long confirming who you are, or signing in, lets
// you take such actions without being asked again.
var recentAuthDuration = 15 * time.Minute

// passkeyReauthSessionKey holds the passkey challenge of a confirmation in
// progress.
const passkeyReauthSessionKey = "passkeyReauth"

func grantRecentAuth(sess session.Session) {
	sess.Set(recentAuthSessionKey, time.Now().Add(recentAuthDuration).Unix())
}

func hasRecentAuth(sess session.Session) bool {
	until, ok := sess.Get(recentAuthSessionKey).(int64)
	return ok && time.Now().Unix() < until
}

type reauthRequiredResponse struct {
	Error          string `json:"error"`
	ReauthRequired bool   `json:"reauthRequired"`
}

// requireRecentAuth turns away actions that change how to sign in to or
// access an account or its data, unless the signed-in user confirmed who they
// are in the last recentAuthDuration. The client confirms through
// /api/web/user/reauth and sends the request again. Mount it after
// requireSignIn.
func requireRecentAuth(c flamego.Context, sess session.Session, l i18n.Locale) {
	if hasRecentAuth(sess) {
		return
	}
	w := c.ResponseWriter()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(reauthRequiredResponse{Error: l.Tr("auth.reauth_required"), ReauthRequired: true})
}

type userReauthMethods struct {
	Passkey  bool `json:"passkey"`
	Passcode bool `json:"passcode"`
	// Minutes is how long a confirmation lasts.
	Minutes int `json:"minutes"`
}

// getUserReauth returns the ways the signed-in user can confirm who they are.
// The password always works.
func getUserReauth(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	passkeys, err := database.Handle.Passkeys().List(ctx, user.ID)
	if err != nil {
		log.Error("getUserReauth: list passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list passkeys")
	}
	return http.StatusOK, &userReauthMethods{
		Passkey:  len(passkeys) > 0,
		Passcode: database.Handle.TwoFactors().IsEnabled(ctx, user.ID),
		Minutes:  int(recentAuthDuration / time.Minute),
	}, nil
}

type userReauthPasswordRequest struct {
	Password string `json:"password" validate:"required,max=255"`
}

func postUserReauth(c flamego.Context, sess session.Session, l i18n.Locale, user *database.User, req userReauthPasswordRequest) (statusCode int, resp any, err error) {
	_, err = database.Handle.Users().Authenticate(c.Request().Context(), user.Name, req.Password, user.LoginSource)
	if err != nil {
		if auth.IsErrBadCredentials(err) || database.IsErrLoginSourceMismatch(err) {
			msg := l.Tr("auth.reauth_password_incorrect")
			return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"password": &msg}}, nil
		}
		log.Error("postUserReauth: authenticate user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "authenticate user")
	}
	grantRecentAuth(sess)
	return http.StatusNoContent, nil, nil
}

type userReauthPasscodeRequest struct {
	Passcode string `json:"passcode" validate:"required,len=6"`
}

func postUserReauthPasscode(c flamego.Context, sess session.Session, ca cache.Cache, l i18n.Locale, user *database.User, req userReauthPasscodeRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	t, err := database.Handle.TwoFactors().GetByUserID(ctx, user.ID)
	if err != nil {
		if database.IsErrTwoFactorNotFound(err) {
			return http.StatusNotFound, nil, errors.New("two-factor authentication is not enabled")
		}
		log.Error("postUserReauthPasscode: get two factor of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get two factor")
	}
	valid, err := t.ValidateTOTP(req.Passcode)
	if err != nil {
		log.Error("postUserReauthPasscode: validate TOTP for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "validate TOTP")
	}
	if !valid {
		msg := l.Tr("auth.mfa_invalid_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"passcode": &msg}}, nil
	}

	// A passcode works once, like when signing in.
	cacheKey := userx.TwoFactorCacheKey(user.ID, req.Passcode)
	if _, err := ca.Get(ctx, cacheKey); err == nil {
		msg := l.Tr("auth.mfa_reused_passcode")
		return http.StatusUnauthorized, &bindingErrorResponse{Fields: fieldErrors{"passcode": &msg}}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Error("postUserReauthPasscode: get two factor passcode cache for user %d: %v", user.ID, err)
	}
	if err = ca.Set(ctx, cacheKey, 1, 60*time.Second); err != nil {
		log.Error("postUserReauthPasscode: cache two factor passcode for user %d: %v", user.ID, err)
	}

	grantRecentAuth(sess)
	return http.StatusNoContent, nil, nil
}

// postUserReauthPasskeyChallenge returns the options to ask the browser for
// one of the passkeys of the signed-in user with.
func postUserReauthPasskeyChallenge(c flamego.Context, sess session.Session, user *database.User) (statusCode int, resp any, err error) {
	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserReauthPasskeyChallenge: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	pu, err := newPasskeyUser(c, user)
	if err != nil {
		log.Error("postUserReauthPasskeyChallenge: get passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	if len(pu.passkeys) == 0 {
		return http.StatusNotFound, nil, errors.New("no passkeys")
	}
	assertion, data, err := wa.BeginLogin(pu, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		log.Error("postUserReauthPasskeyChallenge: begin login for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "begin login")
	}
	if err = setPasskeySession(sess, passkeyReauthSessionKey, data); err != nil {
		log.Error("postUserReauthPasskeyChallenge: save session for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, assertion, nil
}

// postUserReauthPasskey confirms the signed-in user with the passkey that
// answered the challenge in progress.
func postUserReauthPasskey(c flamego.Context, sess session.Session, l i18n.Locale, user *database.User) (statusCode int, resp any, err error) {
	failed := &bindingErrorResponse{Error: l.Tr("auth.reauth_passkey_failed")}
	data, ok := takePasskeySession(sess, passkeyReauthSessionKey)
	if !ok {
		return http.StatusUnauthorized, failed, nil
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(c.Request().Request.Body)
	if err != nil {
		return http.StatusUnauthorized, failed, nil
	}

	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserReauthPasskey: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	pu, err := newPasskeyUser(c, user)
	if err != nil {
		log.Error("postUserReauthPasskey: get passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	credential, err := wa.ValidateLogin(pu, *data, parsed)
	if err != nil || credential.Authenticator.CloneWarning {
		return http.StatusUnauthorized, failed, nil
	}
	passkey := pu.passkeyBy(credential.ID)
	if passkey == nil {
		return http.StatusUnauthorized, failed, nil
	}
	if err = database.Handle.Passkeys().MarkUsed(c.Request().Context(), passkey.ID, *credential); err != nil {
		log.Error("postUserReauthPasskey: mark passkey %d used: %v", passkey.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "mark passkey used")
	}

	grantRecentAuth(sess)
	return http.StatusNoContent, nil, nil
}
