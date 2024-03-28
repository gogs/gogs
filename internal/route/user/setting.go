// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	gocontext "context"
	"encoding/base64"
	"fmt"
	"html/template"
	"image/png"
	"io"

	"github.com/pkg/errors"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userutil"
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
	SETTINGS_PROFILE                   = "user/settings/profile"
	SETTINGS_AVATAR                    = "user/settings/avatar"
	SETTINGS_PASSWORD                  = "user/settings/password"
	SETTINGS_EMAILS                    = "user/settings/email"
	SETTINGS_SSH_KEYS                  = "user/settings/sshkeys"
	SETTINGS_SECURITY                  = "user/settings/security"
	SETTINGS_TWO_FACTOR_ENABLE         = "user/settings/two_factor_enable"
	SETTINGS_TWO_FACTOR_RECOVERY_CODES = "user/settings/two_factor_recovery_codes"
	SETTINGS_REPOSITORIES              = "user/settings/repositories"
	SETTINGS_ORGANIZATIONS             = "user/settings/organizations"
	SETTINGS_APPLICATIONS              = "user/settings/applications"
	SETTINGS_DELETE                    = "user/settings/delete"
	NOTIFICATION                       = "user/notification"
)

func Settings(c *context.Context) {
	c.Title("settings.profile")
	c.PageIs("SettingsProfile")
	c.Data["origin_name"] = c.User.Name
	c.Data["name"] = c.User.Name
	c.Data["full_name"] = c.User.FullName
	c.Data["email"] = c.User.Email
	c.Data["website"] = c.User.Website
	c.Data["location"] = c.User.Location
	c.Success(SETTINGS_PROFILE)
}

func SettingsPost(c *context.Context, f form.UpdateProfile) {
	c.Title("settings.profile")
	c.PageIs("SettingsProfile")
	c.Data["origin_name"] = c.User.Name

	if c.HasError() {
		c.Success(SETTINGS_PROFILE)
		return
	}

	// Non-local users are not allowed to change their username
	if c.User.IsLocal() {
		// Check if the username (including cases) had been changed
		if c.User.Name != f.Name {
			err := database.Users.ChangeUsername(c.Req.Context(), c.User.ID, f.Name)
			if err != nil {
				c.FormErr("Name")
				var msg string
				switch {
				case database.IsErrUserAlreadyExist(errors.Cause(err)):
					msg = c.Tr("form.username_been_taken")
				case database.IsErrNameNotAllowed(errors.Cause(err)):
					msg = c.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
				default:
					c.Error(err, "change user name")
					return
				}

				c.RenderWithErr(msg, SETTINGS_PROFILE, &f)
				return
			}

			log.Trace("Username changed: %s -> %s", c.User.Name, f.Name)
		}
	}

	err := database.Users.Update(
		c.Req.Context(),
		c.User.ID,
		database.UpdateUserOptions{
			FullName: &f.FullName,
			Website:  &f.Website,
			Location: &f.Location,
		},
	)
	if err != nil {
		c.Error(err, "update user")
		return
	}

	c.Flash.Success(c.Tr("settings.update_profile_success"))
	c.RedirectSubpath("/user/settings")
}

// FIXME: limit upload size
func UpdateAvatarSetting(c *context.Context, f form.Avatar, ctxUser *database.User) error {
	if f.Source == form.AvatarLookup && f.Gravatar != "" {
		avatar := cryptoutil.MD5(f.Gravatar)
		err := database.Users.Update(
			c.Req.Context(),
			ctxUser.ID,
			database.UpdateUserOptions{
				Avatar:      &avatar,
				AvatarEmail: &f.Gravatar,
			},
		)
		if err != nil {
			return errors.Wrap(err, "update user")
		}

		err = database.Users.DeleteCustomAvatar(c.Req.Context(), c.User.ID)
		if err != nil {
			return errors.Wrap(err, "delete custom avatar")
		}
		return nil
	}

	if f.Avatar != nil && f.Avatar.Filename != "" {
		r, err := f.Avatar.Open()
		if err != nil {
			return fmt.Errorf("open avatar reader: %v", err)
		}
		defer func() { _ = r.Close() }()

		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read avatar content: %v", err)
		}
		if !tool.IsImageFile(data) {
			return errors.New(c.Tr("settings.uploaded_avatar_not_a_image"))
		}

		err = database.Users.UseCustomAvatar(c.Req.Context(), ctxUser.ID, data)
		if err != nil {
			return errors.Wrap(err, "save avatar")
		}
		return nil
	}
	return nil
}

func SettingsAvatar(c *context.Context) {
	c.Title("settings.avatar")
	c.PageIs("SettingsAvatar")
	c.Success(SETTINGS_AVATAR)
}

func SettingsAvatarPost(c *context.Context, f form.Avatar) {
	if err := UpdateAvatarSetting(c, f, c.User); err != nil {
		c.Flash.Error(err.Error())
	} else {
		c.Flash.Success(c.Tr("settings.update_avatar_success"))
	}

	c.RedirectSubpath("/user/settings/avatar")
}

func SettingsDeleteAvatar(c *context.Context) {
	err := database.Users.DeleteCustomAvatar(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Flash.Error(fmt.Sprintf("Failed to delete avatar: %v", err))
	}

	c.RedirectSubpath("/user/settings/avatar")
}

func SettingsPassword(c *context.Context) {
	c.Title("settings.password")
	c.PageIs("SettingsPassword")
	c.Success(SETTINGS_PASSWORD)
}

func SettingsPasswordPost(c *context.Context, f form.ChangePassword) {
	c.Title("settings.password")
	c.PageIs("SettingsPassword")

	if c.HasError() {
		c.Success(SETTINGS_PASSWORD)
		return
	}

	if !userutil.ValidatePassword(c.User.Password, c.User.Salt, f.OldPassword) {
		c.Flash.Error(c.Tr("settings.password_incorrect"))
	} else if f.Password != f.Retype {
		c.Flash.Error(c.Tr("form.password_not_match"))
	} else {
		err := database.Users.Update(
			c.Req.Context(),
			c.User.ID,
			database.UpdateUserOptions{
				Password: &f.Password,
			},
		)
		if err != nil {
			c.Errorf(err, "update user")
			return
		}
		c.Flash.Success(c.Tr("settings.change_password_success"))
	}

	c.RedirectSubpath("/user/settings/password")
}

func SettingsEmails(c *context.Context) {
	c.Title("settings.emails")
	c.PageIs("SettingsEmails")

	emails, err := database.Users.ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Errorf(err, "get email addresses")
		return
	}
	c.Data["Emails"] = emails

	c.Success(SETTINGS_EMAILS)
}

func SettingsEmailPost(c *context.Context, f form.AddEmail) {
	c.Title("settings.emails")
	c.PageIs("SettingsEmails")

	if c.Query("_method") == "PRIMARY" {
		err := database.Users.MarkEmailPrimary(c.Req.Context(), c.User.ID, c.Query("email"))
		if err != nil {
			c.Errorf(err, "make email primary")
			return
		}

		c.RedirectSubpath("/user/settings/email")
		return
	}

	// Add Email address.
	emails, err := database.Users.ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Errorf(err, "get email addresses")
		return
	}
	c.Data["Emails"] = emails

	if c.HasError() {
		c.Success(SETTINGS_EMAILS)
		return
	}

	err = database.Users.AddEmail(c.Req.Context(), c.User.ID, f.Email, !conf.Auth.RequireEmailConfirmation)
	if err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			c.RenderWithErr(c.Tr("form.email_been_used"), SETTINGS_EMAILS, &f)
		} else {
			c.Errorf(err, "add email address")
		}
		return
	}

	// Send confirmation email
	if conf.Auth.RequireEmailConfirmation {
		email.SendActivateEmailMail(c.Context, database.NewMailerUser(c.User), f.Email)

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

	err := database.Users.DeleteEmail(c.Req.Context(), c.User.ID, email)
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

	c.Success(SETTINGS_SSH_KEYS)
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
		c.Success(SETTINGS_SSH_KEYS)
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
			c.RenderWithErr(c.Tr("settings.ssh_key_been_used"), SETTINGS_SSH_KEYS, &f)
		case database.IsErrKeyNameAlreadyUsed(err):
			c.FormErr("Title")
			c.RenderWithErr(c.Tr("settings.ssh_key_name_used"), SETTINGS_SSH_KEYS, &f)
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

	c.Success(SETTINGS_SECURITY)
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
	c.Success(SETTINGS_TWO_FACTOR_ENABLE)
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

	c.Success(SETTINGS_TWO_FACTOR_RECOVERY_CODES)
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

func SettingsRepos(c *context.Context) {
	c.Title("settings.repos")
	c.PageIs("SettingsRepositories")

	repos, err := database.GetUserAndCollaborativeRepositories(c.User.ID)
	if err != nil {
		c.Errorf(err, "get user and collaborative repositories")
		return
	}
	if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
		c.Errorf(err, "load attributes")
		return
	}
	c.Data["Repos"] = repos

	c.Success(SETTINGS_REPOSITORIES)
}

func SettingsLeaveRepo(c *context.Context) {
	repo, err := database.GetRepositoryByID(c.QueryInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by ID")
		return
	}

	if err = repo.DeleteCollaboration(c.User.ID); err != nil {
		c.Errorf(err, "delete collaboration")
		return
	}

	c.Flash.Success(c.Tr("settings.repos.leave_success", repo.FullName()))
	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/user/settings/repositories",
	})
}

func SettingsOrganizations(c *context.Context) {
	c.Title("settings.orgs")
	c.PageIs("SettingsOrganizations")

	orgs, err := database.GetOrgsByUserID(c.User.ID, true)
	if err != nil {
		c.Errorf(err, "get organizations by user ID")
		return
	}
	c.Data["Orgs"] = orgs

	c.Success(SETTINGS_ORGANIZATIONS)
}

func SettingsLeaveOrganization(c *context.Context) {
	if err := database.RemoveOrgUser(c.QueryInt64("id"), c.User.ID); err != nil {
		if database.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
		} else {
			c.Errorf(err, "remove organization user")
			return
		}
	}

	c.JSONSuccess(map[string]any{
		"redirect": conf.Server.Subpath + "/user/settings/organizations",
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

		c.Success(SETTINGS_APPLICATIONS)
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
			c.Success(SETTINGS_APPLICATIONS)
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

func SettingsDelete(c *context.Context) {
	c.Title("settings.delete")
	c.PageIs("SettingsDelete")

	if c.Req.Method == "POST" {
		if _, err := database.Users.Authenticate(c.Req.Context(), c.User.Name, c.Query("password"), c.User.LoginSource); err != nil {
			if auth.IsErrBadCredentials(err) {
				c.RenderWithErr(c.Tr("form.enterred_invalid_password"), SETTINGS_DELETE, nil)
			} else {
				c.Errorf(err, "authenticate user")
			}
			return
		}

		if err := database.Users.DeleteByID(c.Req.Context(), c.User.ID, false); err != nil {
			switch {
			case database.IsErrUserOwnRepos(err):
				c.Flash.Error(c.Tr("form.still_own_repo"))
				c.Redirect(conf.Server.Subpath + "/user/settings/delete")
			case database.IsErrUserHasOrgs(err):
				c.Flash.Error(c.Tr("form.still_has_org"))
				c.Redirect(conf.Server.Subpath + "/user/settings/delete")
			default:
				c.Errorf(err, "delete user")
			}
		} else {
			log.Trace("Account deleted: %s", c.User.Name)
			c.Redirect(conf.Server.Subpath + "/")
		}
		return
	}

	c.Success(SETTINGS_DELETE)
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
