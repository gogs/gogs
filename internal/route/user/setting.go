// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"image/png"
	"io"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cryptoutil"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
	"gogs.io/gogs/internal/userutil"
)

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
		// Check if username characters have been changed
		if c.User.LowerName != strings.ToLower(f.Name) {
			if err := db.ChangeUserName(c.User, f.Name); err != nil {
				c.FormErr("Name")
				var msg string
				switch {
				case db.IsErrUserAlreadyExist(err):
					msg = c.Tr("form.username_been_taken")
				case db.IsErrNameNotAllowed(err):
					msg = c.Tr("user.form.name_not_allowed", err.(db.ErrNameNotAllowed).Value())
				default:
					c.Error(err, "change user name")
					return
				}

				c.RenderWithErr(msg, SETTINGS_PROFILE, &f)
				return
			}

			log.Trace("Username changed: %s -> %s", c.User.Name, f.Name)
		}

		// In case it's just a case change
		c.User.Name = f.Name
		c.User.LowerName = strings.ToLower(f.Name)
	}

	c.User.FullName = f.FullName
	c.User.Email = f.Email
	c.User.Website = f.Website
	c.User.Location = f.Location
	if err := db.UpdateUser(c.User); err != nil {
		if db.IsErrEmailAlreadyUsed(err) {
			msg := c.Tr("form.email_been_used")
			c.RenderWithErr(msg, SETTINGS_PROFILE, &f)
			return
		}
		c.Errorf(err, "update user")
		return
	}

	c.Flash.Success(c.Tr("settings.update_profile_success"))
	c.RedirectSubpath("/user/settings")
}

// FIXME: limit upload size
func UpdateAvatarSetting(c *context.Context, f form.Avatar, ctxUser *db.User) error {
	ctxUser.UseCustomAvatar = f.Source == form.AVATAR_LOCAL
	if len(f.Gravatar) > 0 {
		ctxUser.Avatar = cryptoutil.MD5(f.Gravatar)
		ctxUser.AvatarEmail = f.Gravatar
	}

	if f.Avatar != nil && f.Avatar.Filename != "" {
		r, err := f.Avatar.Open()
		if err != nil {
			return fmt.Errorf("open avatar reader: %v", err)
		}
		defer func() {
			_ = r.Close()
		}()

		data, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read avatar content: %v", err)
		}
		if !tool.IsImageFile(data) {
			return errors.New(c.Tr("settings.uploaded_avatar_not_a_image"))
		}
		if err = ctxUser.UploadAvatar(data); err != nil {
			return fmt.Errorf("upload avatar: %v", err)
		}
	} else {
		// No avatar is uploaded but setting has been changed to enable,
		// generate a random one when needed.
		if ctxUser.UseCustomAvatar && !com.IsFile(userutil.CustomAvatarPath(ctxUser.ID)) {
			if err := userutil.GenerateRandomAvatar(ctxUser.ID, ctxUser.Name, ctxUser.Email); err != nil {
				log.Error("generate random avatar [%d]: %v", ctxUser.ID, err)
			}
		}
	}

	if err := db.UpdateUser(ctxUser); err != nil {
		return fmt.Errorf("update user: %v", err)
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
	if err := c.User.DeleteAvatar(); err != nil {
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
		c.User.Password = f.Password
		var err error
		if c.User.Salt, err = db.GetUserSalt(); err != nil {
			c.Errorf(err, "get user salt")
			return
		}
		c.User.Password = userutil.EncodePassword(c.User.Password, c.User.Salt)
		if err := db.UpdateUser(c.User); err != nil {
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

	emails, err := db.GetEmailAddresses(c.User.ID)
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

	// Make emailaddress primary.
	if c.Query("_method") == "PRIMARY" {
		if err := db.MakeEmailPrimary(c.UserID(), &db.EmailAddress{ID: c.QueryInt64("id")}); err != nil {
			c.Errorf(err, "make email primary")
			return
		}

		c.RedirectSubpath("/user/settings/email")
		return
	}

	// Add Email address.
	emails, err := db.GetEmailAddresses(c.User.ID)
	if err != nil {
		c.Errorf(err, "get email addresses")
		return
	}
	c.Data["Emails"] = emails

	if c.HasError() {
		c.Success(SETTINGS_EMAILS)
		return
	}

	emailAddr := &db.EmailAddress{
		UID:         c.User.ID,
		Email:       f.Email,
		IsActivated: !conf.Auth.RequireEmailConfirmation,
	}
	if err := db.AddEmailAddress(emailAddr); err != nil {
		if db.IsErrEmailAlreadyUsed(err) {
			c.RenderWithErr(c.Tr("form.email_been_used"), SETTINGS_EMAILS, &f)
		} else {
			c.Errorf(err, "add email address")
		}
		return
	}

	// Send confirmation email
	if conf.Auth.RequireEmailConfirmation {
		email.SendActivateEmailMail(c.Context, db.NewMailerUser(c.User), emailAddr.Email)

		if err := c.Cache.Put("MailResendLimit_"+c.User.LowerName, c.User.LowerName, 180); err != nil {
			log.Error("Set cache 'MailResendLimit' failed: %v", err)
		}
		c.Flash.Info(c.Tr("settings.add_email_confirmation_sent", emailAddr.Email, conf.Auth.ActivateCodeLives/60))
	} else {
		c.Flash.Success(c.Tr("settings.add_email_success"))
	}

	c.RedirectSubpath("/user/settings/email")
}

func DeleteEmail(c *context.Context) {
	if err := db.DeleteEmailAddress(&db.EmailAddress{
		ID:  c.QueryInt64("id"),
		UID: c.User.ID,
	}); err != nil {
		c.Errorf(err, "delete email address")
		return
	}

	c.Flash.Success(c.Tr("settings.email_deletion_success"))
	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/email",
	})
}

func SettingsSSHKeys(c *context.Context) {
	c.Title("settings.ssh_keys")
	c.PageIs("SettingsSSHKeys")

	keys, err := db.ListPublicKeys(c.User.ID)
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

	keys, err := db.ListPublicKeys(c.User.ID)
	if err != nil {
		c.Errorf(err, "list public keys")
		return
	}
	c.Data["Keys"] = keys

	if c.HasError() {
		c.Success(SETTINGS_SSH_KEYS)
		return
	}

	content, err := db.CheckPublicKeyString(f.Content)
	if err != nil {
		if db.IsErrKeyUnableVerify(err) {
			c.Flash.Info(c.Tr("form.unable_verify_ssh_key"))
		} else {
			c.Flash.Error(c.Tr("form.invalid_ssh_key", err.Error()))
			c.RedirectSubpath("/user/settings/ssh")
			return
		}
	}

	if _, err = db.AddPublicKey(c.User.ID, f.Title, content); err != nil {
		c.Data["HasError"] = true
		switch {
		case db.IsErrKeyAlreadyExist(err):
			c.FormErr("Content")
			c.RenderWithErr(c.Tr("settings.ssh_key_been_used"), SETTINGS_SSH_KEYS, &f)
		case db.IsErrKeyNameAlreadyUsed(err):
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
	if err := db.DeletePublicKey(c.User, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeletePublicKey: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("settings.ssh_key_deletion_success"))
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/ssh",
	})
}

func SettingsSecurity(c *context.Context) {
	c.Title("settings.security")
	c.PageIs("SettingsSecurity")

	t, err := db.TwoFactors.GetByUserID(c.Req.Context(), c.UserID())
	if err != nil && !db.IsErrTwoFactorNotFound(err) {
		c.Errorf(err, "get two factor by user ID")
		return
	}
	c.Data["TwoFactor"] = t

	c.Success(SETTINGS_SECURITY)
}

func SettingsTwoFactorEnable(c *context.Context) {
	if c.User.IsEnabledTwoFactor() {
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

	if err := db.TwoFactors.Create(c.Req.Context(), c.UserID(), conf.Security.SecretKey, secret); err != nil {
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
	if !c.User.IsEnabledTwoFactor() {
		c.NotFound()
		return
	}

	c.Title("settings.two_factor_recovery_codes_title")
	c.PageIs("SettingsSecurity")

	recoveryCodes, err := db.GetRecoveryCodesByUserID(c.UserID())
	if err != nil {
		c.Errorf(err, "get recovery codes by user ID")
		return
	}
	c.Data["RecoveryCodes"] = recoveryCodes

	c.Success(SETTINGS_TWO_FACTOR_RECOVERY_CODES)
}

func SettingsTwoFactorRecoveryCodesPost(c *context.Context) {
	if !c.User.IsEnabledTwoFactor() {
		c.NotFound()
		return
	}

	if err := db.RegenerateRecoveryCodes(c.UserID()); err != nil {
		c.Flash.Error(c.Tr("settings.two_factor_regenerate_recovery_codes_error", err))
	} else {
		c.Flash.Success(c.Tr("settings.two_factor_regenerate_recovery_codes_success"))
	}

	c.RedirectSubpath("/user/settings/security/two_factor_recovery_codes")
}

func SettingsTwoFactorDisable(c *context.Context) {
	if !c.User.IsEnabledTwoFactor() {
		c.NotFound()
		return
	}

	if err := db.DeleteTwoFactor(c.UserID()); err != nil {
		c.Errorf(err, "delete two factor")
		return
	}

	c.Flash.Success(c.Tr("settings.two_factor_disable_success"))
	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/security",
	})
}

func SettingsRepos(c *context.Context) {
	c.Title("settings.repos")
	c.PageIs("SettingsRepositories")

	repos, err := db.GetUserAndCollaborativeRepositories(c.User.ID)
	if err != nil {
		c.Errorf(err, "get user and collaborative repositories")
		return
	}
	if err = db.RepositoryList(repos).LoadAttributes(); err != nil {
		c.Errorf(err, "load attributes")
		return
	}
	c.Data["Repos"] = repos

	c.Success(SETTINGS_REPOSITORIES)
}

func SettingsLeaveRepo(c *context.Context) {
	repo, err := db.GetRepositoryByID(c.QueryInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by ID")
		return
	}

	if err = repo.DeleteCollaboration(c.User.ID); err != nil {
		c.Errorf(err, "delete collaboration")
		return
	}

	c.Flash.Success(c.Tr("settings.repos.leave_success", repo.FullName()))
	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/repositories",
	})
}

func SettingsOrganizations(c *context.Context) {
	c.Title("settings.orgs")
	c.PageIs("SettingsOrganizations")

	orgs, err := db.GetOrgsByUserID(c.User.ID, true)
	if err != nil {
		c.Errorf(err, "get organizations by user ID")
		return
	}
	c.Data["Orgs"] = orgs

	c.Success(SETTINGS_ORGANIZATIONS)
}

func SettingsLeaveOrganization(c *context.Context) {
	if err := db.RemoveOrgUser(c.QueryInt64("id"), c.User.ID); err != nil {
		if db.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
		} else {
			c.Errorf(err, "remove organization user")
			return
		}
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/organizations",
	})
}

func SettingsApplications(c *context.Context) {
	c.Title("settings.applications")
	c.PageIs("SettingsApplications")

	tokens, err := db.AccessTokens.List(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Errorf(err, "list access tokens")
		return
	}
	c.Data["Tokens"] = tokens

	c.Success(SETTINGS_APPLICATIONS)
}

func SettingsApplicationsPost(c *context.Context, f form.NewAccessToken) {
	c.Title("settings.applications")
	c.PageIs("SettingsApplications")

	if c.HasError() {
		tokens, err := db.AccessTokens.List(c.Req.Context(), c.User.ID)
		if err != nil {
			c.Errorf(err, "list access tokens")
			return
		}

		c.Data["Tokens"] = tokens
		c.Success(SETTINGS_APPLICATIONS)
		return
	}

	t, err := db.AccessTokens.Create(c.Req.Context(), c.User.ID, f.Name)
	if err != nil {
		if db.IsErrAccessTokenAlreadyExist(err) {
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

func SettingsDeleteApplication(c *context.Context) {
	if err := db.AccessTokens.DeleteByID(c.Req.Context(), c.User.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteAccessTokenByID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("settings.delete_token_success"))
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/user/settings/applications",
	})
}

func SettingsDelete(c *context.Context) {
	c.Title("settings.delete")
	c.PageIs("SettingsDelete")

	if c.Req.Method == "POST" {
		if _, err := db.Users.Authenticate(c.Req.Context(), c.User.Name, c.Query("password"), c.User.LoginSource); err != nil {
			if auth.IsErrBadCredentials(err) {
				c.RenderWithErr(c.Tr("form.enterred_invalid_password"), SETTINGS_DELETE, nil)
			} else {
				c.Errorf(err, "authenticate user")
			}
			return
		}

		if err := db.DeleteUser(c.User); err != nil {
			switch {
			case db.IsErrUserOwnRepos(err):
				c.Flash.Error(c.Tr("form.still_own_repo"))
				c.Redirect(conf.Server.Subpath + "/user/settings/delete")
			case db.IsErrUserHasOrgs(err):
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
