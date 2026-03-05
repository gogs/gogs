package user

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

const (
	passkeyLoginSessionDataKey        = "passkeyLoginSessionData"
	passkeyRegistrationSessionDataKey = "passkeyRegistrationSessionData"
)

// webAuthnUser adapts a database user and passkey set to the WebAuthn user
// interface expected by the go-webauthn library.
type webAuthnUser struct {
	user                    *database.User
	credentials             []webauthn.Credential
	passkeyIDByCredentialID map[string]int64
}

// newWebAuthnUser builds a WebAuthn-compatible user and precomputes a
// credential ID to passkey ID lookup for assertion updates.
func newWebAuthnUser(user *database.User, passkeys []*database.Passkey) (*webAuthnUser, error) {
	credentials := make([]webauthn.Credential, 0, len(passkeys))
	passkeyIDByCredentialID := make(map[string]int64, len(passkeys))
	for _, passkey := range passkeys {
		credential, err := passkey.CredentialStruct()
		if err != nil {
			return nil, errors.Wrapf(err, "decode credential [passkey_id: %d]", passkey.ID)
		}
		credentials = append(credentials, credential)
		passkeyIDByCredentialID[base64.RawURLEncoding.EncodeToString(credential.ID)] = passkey.ID
	}
	return &webAuthnUser{
		user:                    user,
		credentials:             credentials,
		passkeyIDByCredentialID: passkeyIDByCredentialID,
	}, nil
}

// WebAuthnID returns the stable user handle used by authenticators.
func (u *webAuthnUser) WebAuthnID() []byte {
	return []byte(strconv.FormatInt(u.user.ID, 10))
}

// WebAuthnName returns the account name for discoverable credentials.
func (u *webAuthnUser) WebAuthnName() string {
	return u.user.Name
}

// WebAuthnDisplayName returns a user-friendly name shown by authenticators.
func (u *webAuthnUser) WebAuthnDisplayName() string {
	if u.user.FullName != "" {
		return u.user.FullName
	}
	return u.user.Name
}

// WebAuthnCredentials returns all registered passkey credentials for the user.
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

// passkeyIDByCredential resolves a credential ID back to the persisted passkey
// record identifier.
func (u *webAuthnUser) passkeyIDByCredential(rawCredentialID []byte) (int64, bool) {
	passkeyID, ok := u.passkeyIDByCredentialID[base64.RawURLEncoding.EncodeToString(rawCredentialID)]
	return passkeyID, ok
}

// newWebAuthn creates a WebAuthn instance using server URL/domain settings as
// RP ID and origin constraints.
func newWebAuthn() (*webauthn.WebAuthn, error) {
	var rpID string
	if conf.Server.URL != nil {
		rpID = conf.Server.URL.Hostname()
	}
	if rpID == "" {
		rpID = conf.Server.Domain
	}
	if rpID == "" {
		return nil, errors.New("empty relying party ID")
	}

	rpDisplayName := conf.App.BrandName
	if rpDisplayName == "" {
		rpDisplayName = "Gogs"
	}

	origin := strings.TrimRight(conf.Server.ExternalURL, "/")
	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpDisplayName,
		RPOrigins:     []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationPreferred,
		},
	})
}

// saveWebAuthnSession stores WebAuthn session data as JSON in the user session.
func saveWebAuthnSession(c *context.Context, key string, sessionData *webauthn.SessionData) error {
	raw, err := json.Marshal(sessionData)
	if err != nil {
		return errors.Wrap(err, "marshal session data")
	}
	return c.Session.Set(key, string(raw))
}

// loadWebAuthnSession loads and decodes WebAuthn session data from the user
// session. The boolean return indicates whether the key existed.
func loadWebAuthnSession(c *context.Context, key string) (*webauthn.SessionData, bool, error) {
	raw, ok := c.Session.Get(key).(string)
	if !ok || raw == "" {
		return nil, false, nil
	}

	var sessionData webauthn.SessionData
	err := json.Unmarshal([]byte(raw), &sessionData)
	if err != nil {
		return nil, true, errors.Wrap(err, "unmarshal session data")
	}
	return &sessionData, true, nil
}

// passkeyRegistrationFailedMessage builds a localized flash message for
// registration failures and appends the underlying reason when available.
func passkeyRegistrationFailedMessage(c *context.Context, err error) string {
	if err == nil {
		return c.Tr("settings.passkey_register_failed")
	}
	return c.Tr("settings.passkey_register_failed_reason", err)
}

// LoginPasskeyOptions starts the passkey authentication ceremony and returns
// credential request options for browser-side WebAuthn APIs.
func LoginPasskeyOptions(c *context.Context) {
	webAuthn, err := newWebAuthn()
	if err != nil {
		c.Error(err, "create webauthn")
		return
	}

	assertion, sessionData, err := webAuthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		c.Error(err, "begin discoverable login")
		return
	}

	if err = saveWebAuthnSession(c, passkeyLoginSessionDataKey, sessionData); err != nil {
		c.Error(err, "save passkey login session data")
		return
	}

	c.JSONSuccess(assertion)
}

// LoginPasskeyPost verifies the passkey assertion and signs the user in when
// the assertion is valid.
func LoginPasskeyPost(c *context.Context) {
	sessionData, ok, err := loadWebAuthnSession(c, passkeyLoginSessionDataKey)
	if err != nil {
		if ok {
			_ = c.Session.Delete(passkeyLoginSessionDataKey)
		}
		c.Error(err, "load passkey login session data")
		return
	} else if !ok {
		c.Flash.Error(c.Tr("auth.passkey_session_expired"))
		c.RedirectSubpath("/user/login")
		return
	}
	defer func() { _ = c.Session.Delete(passkeyLoginSessionDataKey) }()

	parsedResponse, err := protocol.ParseCredentialRequestResponseBytes([]byte(c.Query("credential")))
	if err != nil {
		c.Flash.Error(c.Tr("auth.passkey_login_failed"))
		c.RedirectSubpath("/user/login")
		return
	}

	webAuthn, err := newWebAuthn()
	if err != nil {
		c.Error(err, "create webauthn")
		return
	}

	findUserByHandle := func(_ []byte, userHandle []byte) (webauthn.User, error) {
		userID, err := strconv.ParseInt(string(userHandle), 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "parse user handle")
		}

		user, err := database.Handle.Users().GetByID(c.Req.Context(), userID)
		if err != nil {
			return nil, errors.Wrap(err, "get user by ID")
		}

		passkeys, err := database.Handle.Passkeys().ListByUserID(c.Req.Context(), user.ID)
		if err != nil {
			return nil, errors.Wrap(err, "list passkeys by user ID")
		}

		return newWebAuthnUser(user, passkeys)
	}

	wUser, credential, err := webAuthn.ValidatePasskeyLogin(findUserByHandle, *sessionData, parsedResponse)
	if err != nil {
		c.Flash.Error(c.Tr("auth.passkey_login_failed"))
		c.RedirectSubpath("/user/login")
		return
	}

	user, ok := wUser.(*webAuthnUser)
	if !ok {
		c.Error(errors.New("invalid webauthn user type"), "type assert webauthn user")
		return
	}

	passkeyID, ok := user.passkeyIDByCredential(credential.ID)
	if !ok {
		c.Flash.Error(c.Tr("auth.passkey_login_failed"))
		c.RedirectSubpath("/user/login")
		return
	}
	if err = database.Handle.Passkeys().UpdateCredential(c.Req.Context(), user.user.ID, passkeyID, *credential); err != nil {
		c.Error(err, "update passkey credential")
		return
	}

	afterLogin(c, user.user, c.QueryBool("remember"))
}

// SettingsPasskeyRegister starts a new passkey registration ceremony for the
// current signed-in user.
func SettingsPasskeyRegister(c *context.Context) {
	passkeys, err := database.Handle.Passkeys().ListByUserID(c.Req.Context(), c.UserID())
	if err != nil {
		c.Error(err, "list passkeys by user ID")
		return
	}

	webAuthnUser, err := newWebAuthnUser(c.User, passkeys)
	if err != nil {
		c.Error(err, "create webauthn user")
		return
	}

	webAuthn, err := newWebAuthn()
	if err != nil {
		c.Error(err, "create webauthn")
		return
	}

	options := []webauthn.RegistrationOption{
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(webAuthnUser.WebAuthnCredentials()).CredentialDescriptors()),
	}

	creation, sessionData, err := webAuthn.BeginRegistration(webAuthnUser, options...)
	if err != nil {
		c.Error(err, "begin passkey registration")
		return
	}

	if err = saveWebAuthnSession(c, passkeyRegistrationSessionDataKey, sessionData); err != nil {
		c.Error(err, "save passkey registration session data")
		return
	}

	c.JSONSuccess(creation)
}

// SettingsPasskeyCreate verifies registration attestation payload and stores
// the created passkey credential.
func SettingsPasskeyCreate(c *context.Context) {
	sessionData, ok, err := loadWebAuthnSession(c, passkeyRegistrationSessionDataKey)
	if err != nil {
		if ok {
			_ = c.Session.Delete(passkeyRegistrationSessionDataKey)
		}
		c.Error(err, "load passkey registration session data")
		return
	} else if !ok {
		c.Flash.Error(c.Tr("settings.passkey_session_expired"))
		c.RedirectSubpath("/user/settings/security")
		return
	}
	defer func() { _ = c.Session.Delete(passkeyRegistrationSessionDataKey) }()

	passkeys, err := database.Handle.Passkeys().ListByUserID(c.Req.Context(), c.UserID())
	if err != nil {
		c.Error(err, "list passkeys by user ID")
		return
	}

	webAuthnUser, err := newWebAuthnUser(c.User, passkeys)
	if err != nil {
		c.Error(err, "create webauthn user")
		return
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBytes([]byte(c.Query("credential")))
	if err != nil {
		c.Flash.Error(passkeyRegistrationFailedMessage(c, err))
		c.RedirectSubpath("/user/settings/security")
		return
	}

	webAuthn, err := newWebAuthn()
	if err != nil {
		c.Error(err, "create webauthn")
		return
	}

	credential, err := webAuthn.CreateCredential(webAuthnUser, *sessionData, parsedResponse)
	if err != nil {
		c.Flash.Error(passkeyRegistrationFailedMessage(c, err))
		c.RedirectSubpath("/user/settings/security")
		return
	}

	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		name = c.Tr("settings.passkey_default_name", time.Now().Format("2006-01-02 15:04"))
	}

	_, err = database.Handle.Passkeys().Create(c.Req.Context(), c.UserID(), name, *credential)
	if err != nil {
		if database.IsErrPasskeyAlreadyExist(err) {
			c.Flash.Error(c.Tr("settings.passkey_already_exists"))
			c.RedirectSubpath("/user/settings/security")
		} else {
			c.Error(err, "create passkey")
		}
		return
	}

	c.Flash.Success(c.Tr("settings.passkey_register_success"))
	c.RedirectSubpath("/user/settings/security")
}

// SettingsPasskeyDelete removes a stored passkey from the current user.
func SettingsPasskeyDelete(c *context.Context) {
	passkeyID := c.QueryInt64("id")
	err := database.Handle.Passkeys().DeleteByID(c.Req.Context(), c.UserID(), passkeyID)
	if err != nil {
		if database.IsErrPasskeyNotFound(err) {
			c.Flash.Error(c.Tr("settings.passkey_not_found"))
			c.RedirectSubpath("/user/settings/security")
		} else {
			c.Error(err, "delete passkey")
		}
		return
	}

	c.Flash.Success(c.Tr("settings.passkey_delete_success"))
	c.RedirectSubpath("/user/settings/security")
}
