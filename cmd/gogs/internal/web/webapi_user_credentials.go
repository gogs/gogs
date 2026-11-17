package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/userx"
)

type userEmail struct {
	Email       string `json:"email"`
	IsPrimary   bool   `json:"isPrimary"`
	IsActivated bool   `json:"isActivated"`
}

type userEmails struct {
	Items []userEmail `json:"items"`
}

func getUserEmails(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userEmails, err error) {
	emails, err := database.Handle.Users().ListEmails(c.Request().Context(), viewer.ID)
	if err != nil {
		logger.Error("Failed to list emails", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list emails")
	}
	items := make([]userEmail, 0, len(emails))
	for _, e := range emails {
		item := userEmail{Email: e.Email, IsPrimary: e.IsPrimary, IsActivated: e.IsActivated}
		if item.IsPrimary {
			items = append([]userEmail{item}, items...)
		} else {
			items = append(items, item)
		}
	}
	return http.StatusOK, &userEmails{Items: items}, nil
}

type userEmailRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

type userEmailAddedResponse struct {
	// ConfirmationSent is true when the site requires confirming the address
	// through the link sent to it before it can be used.
	ConfirmationSent  bool `json:"confirmationSent"`
	CodeLifetimeHours int  `json:"codeLifetimeHours,omitempty"`
}

// postUserEmail adds an email address to the authenticated user, and sends it a
// confirmation link when the site requires one.
func postUserEmail(c flamego.Context, logger *logx.Logger, ca cache.Cache, l i18n.Locale, viewer *database.User, req userEmailRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	err = database.Handle.Users().AddEmail(ctx, viewer.ID, req.Email, !conf.Auth.RequireEmailConfirmation)
	if err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			msg := l.Tr("form.email_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
		}
		logger.Error("Failed to add email", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add email")
	}
	if !conf.Auth.RequireEmailConfirmation {
		return http.StatusCreated, &userEmailAddedResponse{}, nil
	}

	if err := email.SendActivateEmailMail(logger, l, database.NewMailerUser(viewer), req.Email); err != nil {
		logger.Error("Failed to send confirmation mail", "userID", viewer.ID, "error", err)
	}
	if err := ca.Set(ctx, userx.MailResendCacheKey(viewer.ID), 1, 180*time.Second); err != nil {
		logger.Error("Failed to put mail resend cache", "userID", viewer.ID, "error", err)
	}
	return http.StatusCreated, &userEmailAddedResponse{ConfirmationSent: true, CodeLifetimeHours: conf.Auth.ActivateCodeLives / 60}, nil
}

// putUserEmailPrimary makes a confirmed email address of the authenticated user
// their primary one.
func putUserEmailPrimary(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	address := c.Param("email")
	err = database.Handle.Users().MarkEmailPrimary(c.Request().Context(), viewer.ID, address)
	if err != nil {
		switch {
		case database.IsErrEmailAddressNotExist(err):
			return http.StatusNotFound, nil, errors.New("email address not found")
		case database.IsErrEmailNotVerified(err):
			return http.StatusConflict, nil, errors.New("email address not confirmed")
		}
		logger.Error("Failed to make email primary", "email", address, "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "make email primary")
	}
	return http.StatusNoContent, nil, nil
}

func deleteUserEmail(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User) (statusCode int, resp any, err error) {
	address := c.Param("email")
	if strings.EqualFold(address, viewer.Email) {
		return http.StatusConflict, nil, errors.New(l.Tr("settings.email_deletion_primary"))
	}
	if err = database.Handle.Users().DeleteEmail(c.Request().Context(), viewer.ID, address); err != nil {
		logger.Error("Failed to delete email", "email", address, "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete email")
	}
	return http.StatusNoContent, nil, nil
}

type userSSHKey struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"createdAt"`
	// LastUsedAt is only sent for keys that were used.
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	// IsRecentlyUsed is true for keys used in the last 7 days.
	IsRecentlyUsed bool `json:"isRecentlyUsed"`
}

type userSSHKeys struct {
	Items []userSSHKey `json:"items"`
}

func getUserSSHKeys(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userSSHKeys, err error) {
	keys, err := database.Handle.PublicKeys().List(c.Request().Context(), viewer.ID)
	if err != nil {
		logger.Error("Failed to list public keys", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list public keys")
	}
	items := make([]userSSHKey, 0, len(keys))
	for _, k := range keys {
		item := userSSHKey{ID: k.ID, Name: k.Name, Fingerprint: k.Fingerprint, CreatedAt: k.Created, IsRecentlyUsed: k.HasRecentActivity}
		if k.HasUsed {
			item.LastUsedAt = &k.Updated
		}
		items = append(items, item)
	}
	return http.StatusOK, &userSSHKeys{Items: items}, nil
}

type userSSHKeyRequest struct {
	SSHTitle string `json:"title" validate:"required,max=50"`
	Content  string `json:"content" validate:"required"`
}

func postUserSSHKey(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req userSSHKeyRequest) (statusCode int, resp any, err error) {
	content, err := database.CheckPublicKeyString(logger, req.Content)
	if err != nil {
		msg := l.Tr("form.invalid_ssh_key", err.Error())
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
	}
	if _, err = database.Handle.PublicKeys().Add(c.Request().Context(), logger, viewer.ID, req.SSHTitle, content); err != nil {
		var msg string
		switch {
		case database.IsErrKeyAlreadyExist(err):
			msg = l.Tr("settings.ssh_key_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}, nil
		case database.IsErrKeyNameAlreadyUsed(err):
			msg = l.Tr("settings.ssh_key_name_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}, nil
		}
		logger.Error("Failed to add public key", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add public key")
	}
	return http.StatusNoContent, nil, nil
}

// deleteUserSSHKey deletes an SSH key of the authenticated user. Keys of other
// users are not found, even for site admins.
func deleteUserSSHKey(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	key, err := database.Handle.PublicKeys().GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrKeyNotExist(err) {
			return http.StatusNotFound, nil, errors.New("key not found")
		}
		logger.Error("Failed to get public key", "keyID", id, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get public key")
	}
	if key.OwnerID != viewer.ID {
		return http.StatusNotFound, nil, errors.New("key not found")
	}
	if err = database.Handle.PublicKeys().Delete(c.Request().Context(), logger, viewer, key.ID); err != nil {
		logger.Error("Failed to delete public key", "keyID", key.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete public key")
	}
	return http.StatusNoContent, nil, nil
}

type userTwoFactor struct {
	Enabled bool `json:"enabled"`
}

func getUserTwoFactor(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userTwoFactor, err error) {
	return http.StatusOK, &userTwoFactor{Enabled: database.Handle.TwoFactors().IsEnabled(c.Request().Context(), logger, viewer.ID)}, nil
}

// Session keys of the secret a user is enrolling in two-factor authentication
// with, kept until they confirm it with a passcode.
const (
	twoFactorSecretSessionKey = "twoFactorSecret"
	twoFactorURLSessionKey    = "twoFactorURL"
)

type userTwoFactorEnrollment struct {
	Secret string `json:"secret"`
	// QRCodeURL is a data URL of the QR code image of the secret.
	QRCodeURL string `json:"qrCodeURL"`
}

// getUserTwoFactorEnrollment returns the secret to add to an authentication
// app. The same secret is returned until it is confirmed, so reloading the
// page does not invalidate a scanned code.
func getUserTwoFactorEnrollment(c flamego.Context, logger *logx.Logger, sess session.Session, viewer *database.User) (statusCode int, resp any, err error) {
	if database.Handle.TwoFactors().IsEnabled(c.Request().Context(), logger, viewer.ID) {
		return http.StatusNotFound, nil, errors.New("two-factor authentication is already enabled")
	}

	var key *otp.Key
	if keyURL, ok := sess.Get(twoFactorURLSessionKey).(string); ok {
		key, _ = otp.NewKeyFromURL(keyURL)
	}
	if key == nil {
		key, err = totp.Generate(totp.GenerateOpts{
			Issuer:      conf.App.BrandName,
			AccountName: viewer.Email,
		})
		if err != nil {
			logger.Error("Failed to generate TOTP", "userID", viewer.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "generate TOTP")
		}
	}

	img, err := key.Image(240, 240)
	if err != nil {
		logger.Error("Failed to generate TOTP image", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "generate image")
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, img); err != nil {
		logger.Error("Failed to encode TOTP image", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "encode image")
	}

	sess.Set(twoFactorSecretSessionKey, key.Secret())
	sess.Set(twoFactorURLSessionKey, key.String())
	return http.StatusOK, &userTwoFactorEnrollment{
		Secret:    key.Secret(),
		QRCodeURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

type userTwoFactorRequest struct {
	Passcode string `json:"passcode" validate:"required"`
}

// postUserTwoFactor enables two-factor authentication once the user enters a
// passcode for the secret they are enrolling with.
func postUserTwoFactor(c flamego.Context, logger *logx.Logger, sess session.Session, l i18n.Locale, viewer *database.User, req userTwoFactorRequest) (statusCode int, resp any, err error) {
	secret, ok := sess.Get(twoFactorSecretSessionKey).(string)
	if !ok {
		return http.StatusNotFound, nil, errors.New("no two-factor authentication secret to confirm")
	}
	if !totp.Validate(req.Passcode, secret) {
		msg := l.Tr("settings.two_factor_invalid_passcode")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"passcode": &msg}}, nil
	}
	if err = database.Handle.TwoFactors().Create(c.Request().Context(), viewer.ID, conf.Security.SecretKey, secret); err != nil {
		logger.Error("Failed to enable two-factor authentication", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create two factor")
	}
	sess.Delete(twoFactorSecretSessionKey)
	sess.Delete(twoFactorURLSessionKey)
	return http.StatusNoContent, nil, nil
}

func deleteUserTwoFactor(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	if !database.Handle.TwoFactors().IsEnabled(c.Request().Context(), logger, viewer.ID) {
		return http.StatusNotFound, nil, errors.New("two-factor authentication is not enabled")
	}
	if err = database.Handle.TwoFactors().DeleteByUserID(c.Request().Context(), viewer.ID); err != nil {
		logger.Error("Failed to disable two-factor authentication", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete two factor")
	}
	return http.StatusNoContent, nil, nil
}

type userRecoveryCode struct {
	Code   string `json:"code"`
	IsUsed bool   `json:"isUsed"`
}

type userRecoveryCodes struct {
	Items []userRecoveryCode `json:"items"`
}

func listUserRecoveryCodes(ctx context.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	codes, err := database.Handle.TwoFactors().ListRecoveryCodes(ctx, viewer.ID)
	if err != nil {
		logger.Error("Failed to get recovery codes", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get recovery codes")
	}
	items := make([]userRecoveryCode, 0, len(codes))
	for _, code := range codes {
		items = append(items, userRecoveryCode{Code: code.Code, IsUsed: code.IsUsed})
	}
	return http.StatusOK, &userRecoveryCodes{Items: items}, nil
}

func getUserRecoveryCodes(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	if !database.Handle.TwoFactors().IsEnabled(c.Request().Context(), logger, viewer.ID) {
		return http.StatusNotFound, nil, errors.New("two-factor authentication is not enabled")
	}
	return listUserRecoveryCodes(c.Request().Context(), logger, viewer)
}

// postUserRecoveryCodes replaces the recovery codes of the user with new ones
// and returns them.
func postUserRecoveryCodes(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	if !database.Handle.TwoFactors().IsEnabled(c.Request().Context(), logger, viewer.ID) {
		return http.StatusNotFound, nil, errors.New("two-factor authentication is not enabled")
	}
	if err = database.Handle.TwoFactors().RegenerateRecoveryCodes(c.Request().Context(), viewer.ID); err != nil {
		logger.Error("Failed to regenerate recovery codes", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "regenerate recovery codes")
	}
	return listUserRecoveryCodes(c.Request().Context(), logger, viewer)
}

type userAccessToken struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	// LastUsedAt is only sent for tokens that were used.
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	// IsRecentlyUsed is true for tokens used in the last 7 days.
	IsRecentlyUsed bool `json:"isRecentlyUsed"`
}

type userAccessTokens struct {
	Items []userAccessToken `json:"items"`
}

func getUserAccessTokens(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp *userAccessTokens, err error) {
	tokens, err := database.Handle.AccessTokens().List(c.Request().Context(), viewer.ID)
	if err != nil {
		logger.Error("Failed to list access tokens", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list access tokens")
	}
	items := make([]userAccessToken, 0, len(tokens))
	for _, t := range tokens {
		item := userAccessToken{ID: t.ID, Name: t.Name, CreatedAt: t.Created, IsRecentlyUsed: t.HasRecentActivity}
		if t.HasUsed {
			item.LastUsedAt = &t.Updated
		}
		items = append(items, item)
	}
	return http.StatusOK, &userAccessTokens{Items: items}, nil
}

type userAccessTokenRequest struct {
	TokenName string `json:"name" validate:"required,max=255"`
}

type userAccessTokenCreated struct {
	// Token is only ever shown here, right after it is created.
	Token string `json:"token"`
}

func postUserAccessToken(c flamego.Context, logger *logx.Logger, l i18n.Locale, viewer *database.User, req userAccessTokenRequest) (statusCode int, resp any, err error) {
	t, err := database.Handle.AccessTokens().Create(c.Request().Context(), viewer.ID, req.TokenName)
	if err != nil {
		if database.IsErrAccessTokenAlreadyExist(err) {
			msg := l.Tr("settings.token_name_exists")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
		}
		logger.Error("Failed to create access token", "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create access token")
	}
	return http.StatusCreated, &userAccessTokenCreated{Token: t.Sha1}, nil
}

func deleteUserAccessToken(c flamego.Context, logger *logx.Logger, viewer *database.User) (statusCode int, resp any, err error) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err = database.Handle.AccessTokens().DeleteByID(c.Request().Context(), viewer.ID, id); err != nil {
		logger.Error("Failed to delete access token", "tokenID", id, "userID", viewer.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete access token")
	}
	return http.StatusNoContent, nil, nil
}
