package web

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/go-macaron/i18n"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
)

// Session keys of the WebAuthn ceremonies in progress, each holding the JSON of
// the webauthn.SessionData of its challenge until it is answered.
const (
	passkeyRegistrationSessionKey = "passkeyRegistration"
	passkeySignInSessionKey       = "passkeySignIn"
)

// passkeySignInTimeout is how long a sign-in with a passkey waits for the
// authenticator. The sign-in page offers other ways to sign in once it passes.
const passkeySignInTimeout = time.Minute

// newWebAuthn returns the WebAuthn relying party of the site, which is
// identified by the host of the external URL. Passkeys therefore only work on
// the external URL, and stop working when its host changes.
func newWebAuthn() (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPID:          conf.Server.URL.Hostname(),
		RPDisplayName: conf.App.BrandName,
		RPOrigins:     []string{conf.Server.URL.Scheme + "://" + conf.Server.URL.Host},
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: passkeySignInTimeout,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce: true,
				Timeout: 5 * time.Minute,
			},
		},
	})
}

// passkeyUser is a user with their passkeys as a WebAuthn user.
type passkeyUser struct {
	*database.User
	passkeys []*database.Passkey
}

func newPasskeyUser(c flamego.Context, user *database.User) (*passkeyUser, error) {
	passkeys, err := database.Handle.Passkeys().List(c.Request().Context(), user.ID)
	if err != nil {
		return nil, errors.Wrap(err, "list passkeys")
	}
	return &passkeyUser{User: user, passkeys: passkeys}, nil
}

// passkeyUserHandle returns the user handle of the user with the given ID,
// which authenticators return when signing in so the user can be found without
// a username.
func passkeyUserHandle(userID int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(userID))
}

func (u *passkeyUser) WebAuthnID() []byte          { return passkeyUserHandle(u.ID) }
func (u *passkeyUser) WebAuthnName() string        { return u.Name }
func (u *passkeyUser) WebAuthnDisplayName() string { return u.DisplayName() }

func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	credentials := make([]webauthn.Credential, 0, len(u.passkeys))
	for _, p := range u.passkeys {
		credentials = append(credentials, p.Credential)
	}
	return credentials
}

// passkeyBy returns the passkey of the user with the given credential ID.
func (u *passkeyUser) passkeyBy(credentialID []byte) *database.Passkey {
	for _, p := range u.passkeys {
		if bytes.Equal(p.Credential.ID, credentialID) {
			return p
		}
	}
	return nil
}

// setPasskeySession keeps the WebAuthn session data under the key until the
// challenge is answered.
func setPasskeySession(sess session.Session, key string, data *webauthn.SessionData) error {
	b, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "marshal session data")
	}
	sess.Set(key, string(b))
	return nil
}

// takePasskeySession returns the WebAuthn session data under the key and
// removes it, so each challenge can only be answered once.
func takePasskeySession(sess session.Session, key string) (*webauthn.SessionData, bool) {
	raw, ok := sess.Get(key).(string)
	if !ok {
		return nil, false
	}
	sess.Delete(key)
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, false
	}
	return &data, true
}

type userPasskey struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	// LastUsedAt is only sent for passkeys that were used.
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	// IsRecentlyUsed is true for passkeys used in the last 7 days.
	IsRecentlyUsed bool `json:"isRecentlyUsed"`
}

type userPasskeys struct {
	Items []userPasskey `json:"items"`
}

func getUserPasskeys(c flamego.Context, user *database.User) (statusCode int, resp *userPasskeys, err error) {
	passkeys, err := database.Handle.Passkeys().List(c.Request().Context(), user.ID)
	if err != nil {
		log.Error("getUserPasskeys: list passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list passkeys")
	}
	items := make([]userPasskey, 0, len(passkeys))
	for _, p := range passkeys {
		item := userPasskey{ID: p.ID, Name: p.Name, CreatedAt: p.Created, IsRecentlyUsed: p.HasRecentActivity}
		if p.HasUsed {
			item.LastUsedAt = &p.Updated
		}
		items = append(items, item)
	}
	return http.StatusOK, &userPasskeys{Items: items}, nil
}

type userPasskeyRegistrationRequest struct {
	PasskeyName string `json:"name" validate:"required,max=255"`
}

// postUserPasskeyRegistration starts adding a passkey to the signed-in user,
// and returns the options to create it with in the browser. It takes the name
// of the passkey only to reject an invalid one before the browser asks for
// the passkey.
func postUserPasskeyRegistration(c flamego.Context, sess session.Session, user *database.User, _ userPasskeyRegistrationRequest) (statusCode int, resp any, err error) {
	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserPasskeyRegistration: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	pu, err := newPasskeyUser(c, user)
	if err != nil {
		log.Error("postUserPasskeyRegistration: get passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}

	creation, data, err := wa.BeginRegistration(
		pu,
		// Only passkeys that can be found without a username and that verify
		// the user can sign in without a password and a second factor.
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(webauthn.Credentials(pu.WebAuthnCredentials()).CredentialDescriptors()),
	)
	if err != nil {
		log.Error("postUserPasskeyRegistration: begin registration for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "begin registration")
	}
	if err = setPasskeySession(sess, passkeyRegistrationSessionKey, data); err != nil {
		log.Error("postUserPasskeyRegistration: save session for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, creation, nil
}

type userPasskeyRequest struct {
	PasskeyName string          `json:"name" validate:"required,max=255"`
	Credential  json.RawMessage `json:"credential" validate:"required"`
}

// postUserPasskey adds the passkey the browser created for the registration
// in progress to the signed-in user.
func postUserPasskey(c flamego.Context, sess session.Session, mc *macaron.Context, l i18n.Locale, user *database.User, req userPasskeyRequest) (statusCode int, resp any, err error) {
	data, ok := takePasskeySession(sess, passkeyRegistrationSessionKey)
	if !ok {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("settings.passkey_add_failed")}, nil
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(req.Credential)
	if err != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("settings.passkey_add_failed")}, nil
	}

	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserPasskey: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	pu, err := newPasskeyUser(c, user)
	if err != nil {
		log.Error("postUserPasskey: get passkeys of user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	credential, err := wa.CreateCredential(pu, *data, parsed)
	if err != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Error: l.Tr("settings.passkey_add_failed")}, nil
	}

	if _, err = database.Handle.Passkeys().Create(c.Request().Context(), user.ID, req.PasskeyName, *credential); err != nil {
		log.Error("postUserPasskey: create passkey for user %d: %v", user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create passkey")
	}
	alertUser(mc, user, user.Email, email.AlertPasskeyAdded, req.PasskeyName)
	return http.StatusCreated, nil, nil
}

func deleteUserPasskey(c flamego.Context, user *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err = database.Handle.Passkeys().DeleteByID(c.Request().Context(), user.ID, id); err != nil {
		log.Error("deleteUserPasskey: delete passkey %d of user %d: %v", id, user.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete passkey")
	}
	return http.StatusNoContent, nil, nil
}

// postUserSignInPasskeyChallenge starts signing in with a passkey, and returns
// the options to ask the browser for one with. Any passkey of the site can
// answer, so the visitor does not enter a username.
func postUserSignInPasskeyChallenge(sess session.Session) (statusCode int, resp any, err error) {
	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserSignInPasskeyChallenge: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	assertion, data, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		log.Error("postUserSignInPasskeyChallenge: begin login: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "begin login")
	}
	if err = setPasskeySession(sess, passkeySignInSessionKey, data); err != nil {
		log.Error("postUserSignInPasskeyChallenge: save session: %v", err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusOK, assertion, nil
}

// postUserSignInPasskey signs in the owner of the passkey that answered the
// challenge in progress. The passkey verified the user, so neither a password
// nor a second factor is asked for.
func postUserSignInPasskey(c flamego.Context, sess session.Session, mc *macaron.Context, l i18n.Locale) (statusCode int, resp any, err error) {
	failed := &bindingErrorResponse{Error: l.Tr("auth.passkey_sign_in_failed")}
	data, ok := takePasskeySession(sess, passkeySignInSessionKey)
	if !ok {
		return http.StatusUnauthorized, failed, nil
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(c.Request().Request.Body)
	if err != nil {
		return http.StatusUnauthorized, failed, nil
	}

	wa, err := newWebAuthn()
	if err != nil {
		log.Error("postUserSignInPasskey: create WebAuthn relying party: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create WebAuthn relying party")
	}
	ctx := c.Request().Context()
	var owner *passkeyUser
	credential, err := wa.ValidateDiscoverableLogin(
		func(_, userHandle []byte) (webauthn.User, error) {
			if len(userHandle) != 8 {
				return nil, errors.New("malformed user handle")
			}
			u, err := database.Handle.Users().GetByID(ctx, int64(binary.BigEndian.Uint64(userHandle)))
			if err != nil {
				return nil, errors.Wrap(err, "get user")
			}
			owner, err = newPasskeyUser(c, u)
			if err != nil {
				return nil, err
			}
			return owner, nil
		},
		*data,
		parsed,
	)
	if err != nil {
		return http.StatusUnauthorized, failed, nil
	}
	// A signature counter that went backwards means the passkey may have been
	// cloned.
	if credential.Authenticator.CloneWarning {
		log.Warn("postUserSignInPasskey: refused a passkey of user %d with a signature counter that went backwards", owner.ID)
		return http.StatusUnauthorized, failed, nil
	}

	passkey := owner.passkeyBy(credential.ID)
	if passkey == nil {
		return http.StatusUnauthorized, failed, nil
	}
	if err = database.Handle.Passkeys().MarkUsed(ctx, passkey.ID, *credential); err != nil {
		log.Error("postUserSignInPasskey: mark passkey %d used: %v", passkey.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "mark passkey used")
	}

	if err = completeSignIn(sess, mc, owner.User, true); err != nil {
		log.Error("postUserSignInPasskey: complete sign-in of user %d: %v", owner.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	return http.StatusNoContent, nil, nil
}
