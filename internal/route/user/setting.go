package user

import (
	"bytes"
	gocontext "context"
	"encoding/base64"
	"encoding/hex"
	"html/template"
	"image/png"
	"net/http"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userx"
)

// SettingsHandler is the handler for users settings endpoints.
type SettingsHandler struct {
	store SettingsStore
}

// NewSettingsHandler returns a new SettingsHandler for users settings endpoints.
func NewSettingsHandler(s SettingsStore) *SettingsHandler {
	return &SettingsHandler{
		store: s,
	}
}

const (
	tmplUserSettingsEmail                  = "user/settings/email"
	tmplUserSettingsSSHKeys                = "user/settings/sshkeys"
	tmplUserSettingsSecurity               = "user/settings/security"
	tmplUserSettingsTwoFactorEnable        = "user/settings/two_factor_enable"
	tmplUserSettingsTwoFactorRecoveryCodes = "user/settings/two_factor_recovery_codes"
	tmplUserSettingsApplications           = "user/settings/applications"
	tmplUserNotification                   = "user/notification"
)

func SettingsEmails(c *context.Context) {
	c.Title("settings.emails")
	c.PageIs("SettingsEmails")

	emails, err := database.Handle.Users().ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Errorf(err, "get email addresses")
		return
	}
	c.Data["Emails"] = emails

	c.Success(tmplUserSettingsEmail)
}

func parseUserFromCode(ctx gocontext.Context, code string) (user *database.User) {
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

func verifyActiveEmailCode(ctx gocontext.Context, code, email string) *database.EmailAddress {
	if user := parseUserFromCode(ctx, code); user != nil {
		if userx.VerifyActivateCode(code, user.ID, email, user.LowerName, user.Password, user.Rands) {
			emailAddress, err := database.Handle.Users().GetEmail(ctx, user.ID, email, false)
			if err == nil {
				return emailAddress
			}
		}
	}
	return nil
}

func ActivateEmail(c *context.Context) {
	code := c.Query("code")
	emailAddr := c.Query("email")

	if email := verifyActiveEmailCode(c.Req.Context(), code, emailAddr); email != nil {
		err := database.Handle.Users().MarkEmailActivated(c.Req.Context(), email.UserID, email.Email)
		if err != nil {
			c.Error(err, "activate email")
			return
		}

		log.Trace("Email activated: %s", email.Email)
		c.Flash.Success(c.Tr("settings.add_email_success"))
	}

	c.RedirectSubpath("/user/settings/email")
}

func SettingsEmailPost(c *context.Context, f form.AddEmail) {
	c.Title("settings.emails")
	c.PageIs("SettingsEmails")

	if c.Query("_method") == "PRIMARY" {
		err := database.Handle.Users().MarkEmailPrimary(c.Req.Context(), c.User.ID, c.Query("email"))
		if err != nil {
			c.Errorf(err, "make email primary")
			return
		}

		c.RedirectSubpath("/user/settings/email")
		return
	}

	// Add Email address.
	emails, err := database.Handle.Users().ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Errorf(err, "get email addresses")
		return
	}
	c.Data["Emails"] = emails

	if c.HasError() {
		c.HTML(http.StatusBadRequest, tmplUserSettingsEmail)
		return
	}

	err = database.Handle.Users().AddEmail(c.Req.Context(), c.User.ID, f.Email, !conf.Auth.RequireEmailConfirmation)
	if err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			c.RenderWithErr(c.Tr("form.email_been_used"), http.StatusUnprocessableEntity, tmplUserSettingsEmail, &f)
		} else {
			c.Errorf(err, "add email address")
		}
		return
	}

	// Send confirmation email
	if conf.Auth.RequireEmailConfirmation {
		if err := email.SendActivateEmailMail(c.Context, database.NewMailerUser(c.User), f.Email); err != nil {
			log.Error("Failed to send activate email mail: %v", err)
		}

		if err := c.Cache.Put("MailResendLimit_"+c.User.LowerName, c.User.LowerName, 180); err != nil {
			log.Error("Set cache 'MailResendLimit' failed: %v", err)
		}
		c.Flash.Info(c.Tr("settings.add_email_confirmation_sent", f.Email, conf.Auth.ActivateCodeLives/60))
	} else {
		c.Flash.Success(c.Tr("settings.add_email_success"))
	}

	c.RedirectSubpath("/user/settings/email")
}

func DeleteEmail(c *context.Context) {
	email := c.Query("id") // The "id" here is the actual email address
	if c.User.Email == email {
		c.Flash.Error(c.Tr("settings.email_deletion_primary"))
		c.JSONSuccess(map[string]any{
			"redirect": conf.Server.Subpath + "/user/settings/email",
		})
		return
	}

	err := database.Handle.Users().DeleteEmail(c.Req.Context(), c.User.ID, email)
	if err != nil {
		c.Error(err, "delete email address")
		return
	}

	c.Flash.Success(c.Tr("settings.email_deletion_success"))
	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/user/settings/email",
	})
}

func SettingsSSHKeys(c *context.Context) {
	c.Title("settings.ssh_keys")
	c.PageIs("SettingsSSHKeys")

	keys, err := database.ListPublicKeys(c.User.ID)
	if err != nil {
		c.Errorf(err, "list public keys")
		return
	}
	c.Data["Keys"] = keys

	c.Success(tmplUserSettingsSSHKeys)
}

func SettingsSSHKeysPost(c *context.Context, f form.AddSSHKey) {
	c.Title("settings.ssh_keys")
	c.PageIs("SettingsSSHKeys")

	keys, err := database.ListPublicKeys(c.User.ID)
	if err != nil {
		c.Errorf(err, "list public keys")
		return
	}
	c.Data["Keys"] = keys

	if c.HasError() {
		c.HTML(http.StatusBadRequest, tmplUserSettingsSSHKeys)
		return
	}

	content, err := database.CheckPublicKeyString(f.Content)
	if err != nil {
		if database.IsErrKeyUnableVerify(err) {
			c.Flash.Info(c.Tr("form.unable_verify_ssh_key"))
		} else {
			c.Flash.Error(c.Tr("form.invalid_ssh_key", err.Error()))
			c.RedirectSubpath("/user/settings/ssh")
			return
		}
	}

	if _, err = database.AddPublicKey(c.User.ID, f.Title, content); err != nil {
		c.Data["HasError"] = true
		switch {
		case database.IsErrKeyAlreadyExist(err):
			c.FormErr("Content")
			c.RenderWithErr(c.Tr("settings.ssh_key_been_used"), http.StatusUnprocessableEntity, tmplUserSettingsSSHKeys, &f)
		case database.IsErrKeyNameAlreadyUsed(err):
			c.FormErr("Title")
			c.RenderWithErr(c.Tr("settings.ssh_key_name_used"), http.StatusUnprocessableEntity, tmplUserSettingsSSHKeys, &f)
		default:
			c.Errorf(err, "add public key")
		}
		return
	}

	c.Flash.Success(c.Tr("settings.add_key_success", f.Title))
	c.RedirectSubpath("/user/settings/ssh")
}

func DeleteSSHKey(c *context.Context) {
	if err := database.DeletePublicKey(c.User, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeletePublicKey: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("settings.ssh_key_deletion_success"))
	}

	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/user/settings/ssh",
	})
}

func SettingsSecurity(c *context.Context) {
	c.Title("settings.security")
	c.PageIs("SettingsSecurity")

	t, err := database.Handle.TwoFactors().GetByUserID(c.Req.Context(), c.UserID())
	if err != nil && !database.IsErrTwoFactorNotFound(err) {
		c.Errorf(err, "get two factor by user ID")
		return
	}
	c.Data["TwoFactor"] = t

	c.Success(tmplUserSettingsSecurity)
}

func SettingsTwoFactorEnable(c *context.Context) {
	if database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
		c.NotFound()
		return
	}

	c.Title("settings.two_factor_enable_title")
	c.PageIs("SettingsSecurity")

	var key *otp.Key
	var err error
	keyURL := c.Session.Get("twoFactorURL")
	if keyURL != nil {
		key, _ = otp.NewKeyFromURL(keyURL.(string))
	}
	if key == nil {
		key, err = totp.Generate(totp.GenerateOpts{
			Issuer:      conf.App.BrandName,
			AccountName: c.User.Email,
		})
		if err != nil {
			c.Errorf(err, "generate TOTP")
			return
		}
	}
	c.Data["TwoFactorSecret"] = key.Secret()

	img, err := key.Image(240, 240)
	if err != nil {
		c.Errorf(err, "generate image")
		return
	}

	var buf bytes.Buffer
	if err = png.Encode(&buf, img); err != nil {
		c.Errorf(err, "encode image")
		return
	}
	c.Data["QRCode"] = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()))

	_ = c.Session.Set("twoFactorSecret", c.Data["TwoFactorSecret"])
	_ = c.Session.Set("twoFactorURL", key.String())
	c.Success(tmplUserSettingsTwoFactorEnable)
}

func SettingsTwoFactorEnablePost(c *context.Context) {
	secret, ok := c.Session.Get("twoFactorSecret").(string)
	if !ok {
		c.NotFound()
		return
	}

	if !totp.Validate(c.Query("passcode"), secret) {
		c.Flash.Error(c.Tr("settings.two_factor_invalid_passcode"))
		c.RedirectSubpath("/user/settings/security/two_factor_enable")
		return
	}

	if err := database.Handle.TwoFactors().Create(c.Req.Context(), c.UserID(), conf.Security.SecretKey, secret); err != nil {
		c.Flash.Error(c.Tr("settings.two_factor_enable_error", err))
		c.RedirectSubpath("/user/settings/security/two_factor_enable")
		return
	}

	_ = c.Session.Delete("twoFactorSecret")
	_ = c.Session.Delete("twoFactorURL")
	c.Flash.Success(c.Tr("settings.two_factor_enable_success"))
	c.RedirectSubpath("/user/settings/security/two_factor_recovery_codes")
}

func SettingsTwoFactorRecoveryCodes(c *context.Context) {
	if !database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
		c.NotFound()
		return
	}

	c.Title("settings.two_factor_recovery_codes_title")
	c.PageIs("SettingsSecurity")

	recoveryCodes, err := database.GetRecoveryCodesByUserID(c.UserID())
	if err != nil {
		c.Errorf(err, "get recovery codes by user ID")
		return
	}
	c.Data["RecoveryCodes"] = recoveryCodes

	c.Success(tmplUserSettingsTwoFactorRecoveryCodes)
}

func SettingsTwoFactorRecoveryCodesPost(c *context.Context) {
	if !database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
		c.NotFound()
		return
	}

	if err := database.RegenerateRecoveryCodes(c.UserID()); err != nil {
		c.Flash.Error(c.Tr("settings.two_factor_regenerate_recovery_codes_error", err))
	} else {
		c.Flash.Success(c.Tr("settings.two_factor_regenerate_recovery_codes_success"))
	}

	c.RedirectSubpath("/user/settings/security/two_factor_recovery_codes")
}

func SettingsTwoFactorDisable(c *context.Context) {
	if !database.Handle.TwoFactors().IsEnabled(c.Req.Context(), c.User.ID) {
		c.NotFound()
		return
	}

	if err := database.DeleteTwoFactor(c.UserID()); err != nil {
		c.Errorf(err, "delete two factor")
		return
	}

	c.Flash.Success(c.Tr("settings.two_factor_disable_success"))
	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/user/settings/security",
	})
}

func (h *SettingsHandler) Applications() macaron.Handler {
	return func(c *context.Context) {
		c.Title("settings.applications")
		c.PageIs("SettingsApplications")

		tokens, err := h.store.ListAccessTokens(c.Req.Context(), c.User.ID)
		if err != nil {
			c.Errorf(err, "list access tokens")
			return
		}
		c.Data["Tokens"] = tokens

		c.Success(tmplUserSettingsApplications)
	}
}

func (h *SettingsHandler) ApplicationsPost() macaron.Handler {
	return func(c *context.Context, f form.NewAccessToken) {
		c.Title("settings.applications")
		c.PageIs("SettingsApplications")

		if c.HasError() {
			tokens, err := h.store.ListAccessTokens(c.Req.Context(), c.User.ID)
			if err != nil {
				c.Errorf(err, "list access tokens")
				return
			}

			c.Data["Tokens"] = tokens
			c.HTML(http.StatusBadRequest, tmplUserSettingsApplications)
			return
		}

		t, err := h.store.CreateAccessToken(c.Req.Context(), c.User.ID, f.Name)
		if err != nil {
			if database.IsErrAccessTokenAlreadyExist(err) {
				c.Flash.Error(c.Tr("settings.token_name_exists"))
				c.RedirectSubpath("/user/settings/applications")
			} else {
				c.Errorf(err, "new access token")
			}
			return
		}

		c.Flash.Success(c.Tr("settings.generate_token_succees"))
		c.Flash.Info(t.Sha1)
		c.RedirectSubpath("/user/settings/applications")
	}
}

func (h *SettingsHandler) DeleteApplication() macaron.Handler {
	return func(c *context.Context) {
		if err := h.store.DeleteAccessTokenByID(c.Req.Context(), c.User.ID, c.QueryInt64("id")); err != nil {
			c.Flash.Error("DeleteAccessTokenByID: " + err.Error())
		} else {
			c.Flash.Success(c.Tr("settings.delete_token_success"))
		}

		c.JSONSuccess(map[string]any{
			"redirect": conf.Server.Subpath + "/user/settings/applications",
		})
	}
}

// SettingsStore is the data layer carrier for user settings endpoints. This
// interface is meant to abstract away and limit the exposure of the underlying
// data layer to the handler through a thin-wrapper.
type SettingsStore interface {
	// CreateAccessToken creates a new access token and persist to database. It
	// returns database.ErrAccessTokenAlreadyExist when an access token with same
	// name already exists for the user.
	CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error)
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx gocontext.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx gocontext.Context, id int64) error
	// ListAccessTokens returns all access tokens belongs to given user.
	ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error)
	// DeleteAccessTokenByID deletes the access token by given ID.
	DeleteAccessTokenByID(ctx gocontext.Context, userID, id int64) error
}

type settingsStore struct{}

// NewSettingsStore returns a new SettingsStore using the global database
// handle.
func NewSettingsStore() SettingsStore {
	return &settingsStore{}
}

func (*settingsStore) CreateAccessToken(ctx gocontext.Context, userID int64, name string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().Create(ctx, userID, name)
}

func (*settingsStore) GetAccessTokenBySHA1(ctx gocontext.Context, sha1 string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().GetBySHA1(ctx, sha1)
}

func (*settingsStore) TouchAccessTokenByID(ctx gocontext.Context, id int64) error {
	return database.Handle.AccessTokens().Touch(ctx, id)
}

func (*settingsStore) ListAccessTokens(ctx gocontext.Context, userID int64) ([]*database.AccessToken, error) {
	return database.Handle.AccessTokens().List(ctx, userID)
}

func (*settingsStore) DeleteAccessTokenByID(ctx gocontext.Context, userID, id int64) error {
	return database.Handle.AccessTokens().DeleteByID(ctx, userID, id)
}
