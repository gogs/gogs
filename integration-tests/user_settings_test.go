package integrationtests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gogs.io/gogs/internal/database"
)

func userSignedIn(t *testing.T) (*User, *client) {
	t.Helper()

	u := seedUser(t, CreateUserOptions{})
	return u, authenticatedClient(t, u)
}

func userGet(t *testing.T, id int64) *database.User {
	t.Helper()

	u, err := database.Handle.Users().GetByID(t.Context(), id)
	require.NoError(t, err)
	return u
}

// userAssertSignInRequired asserts that every route answers an unauthenticated
// visitor with 401.
func userAssertSignInRequired(t *testing.T, routes ...string) {
	t.Helper()

	anonymous := newClient(t)
	for _, r := range routes {
		method, path, _ := strings.Cut(r, " ")
		status, body := anonymous.do(method, path, map[string]any{})
		assert.Equal(t, http.StatusUnauthorized, status, "%s, response: %s", r, body)
	}
}

func TestUserSettingsProfile(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t, "GET /api/web/user/settings", "PATCH /api/web/user/settings")

	u, c := userSignedIn(t)
	type settings struct {
		Name            string  `json:"name"`
		FullName        string  `json:"fullName"`
		Website         string  `json:"website"`
		Location        string  `json:"location"`
		IsLocal         bool    `json:"isLocal"`
		AvatarURL       string  `json:"avatarURL"`
		HasCustomAvatar bool    `json:"hasCustomAvatar"`
		AvatarEmail     *string `json:"avatarEmail"`
	}
	var got settings
	c.doJSON(http.MethodGet, "/api/web/user/settings", nil, http.StatusOK, &got)
	assert.Equal(t, u.Name, got.Name)
	assert.True(t, got.IsLocal)
	assert.False(t, got.HasCustomAvatar)
	assert.NotEmpty(t, got.AvatarURL)
	// The server under test is in offline mode, which disables Gravatar.
	assert.Nil(t, got.AvatarEmail)

	taken := seedUser(t, CreateUserOptions{})
	status, body := c.do(http.MethodPatch, "/api/web/user/settings", map[string]any{"name": taken.Name})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	status, body = c.do(http.MethodPatch, "/api/web/user/settings", map[string]any{"name": u.Name, "website": "not a URL"})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	newName := UniqueName("renamed-")
	var renamed struct {
		Name string `json:"name"`
	}
	c.doJSON(http.MethodPatch, "/api/web/user/settings", map[string]any{
		"name":     newName,
		"fullName": "Alice Example",
		"website":  "https://example.com/alice",
		"location": "Earth",
	}, http.StatusOK, &renamed)
	assert.Equal(t, newName, renamed.Name)

	stored := userGet(t, u.ID)
	assert.Equal(t, newName, stored.Name)
	assert.Equal(t, "Alice Example", stored.FullName)
	assert.Equal(t, "https://example.com/alice", stored.Website)
	assert.Equal(t, "Earth", stored.Location)
	assert.Equal(t, newName, c.username())
}

func TestUserSettingsPassword(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t, "PUT /api/web/user/settings/password")

	u, c := userSignedIn(t)
	put := func(oldPassword, password, retype string) (int, []byte) {
		return c.do(http.MethodPut, "/api/web/user/settings/password", map[string]any{
			"oldPassword": oldPassword,
			"password":    password,
			"retype":      retype,
		})
	}
	status, body := put(u.Password+"x", "new-password", "new-password")
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	status, body = put(u.Password, "new-password", "other-password")
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	status, body = put(u.Password, "new-password", "new-password")
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	newClient(t).signIn(&User{Name: u.Name, Password: "new-password"})
	status, body = newClient(t).do(http.MethodPost, "/api/web/user/sign-in", map[string]any{"username": u.Name, "password": u.Password})
	assert.Equal(t, http.StatusUnauthorized, status, "old password still works, response: %s", body)
}

func userAvatarForm(t *testing.T, data []byte) (string, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("avatar", "avatar.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return w.FormDataContentType(), &buf
}

func TestUserSettingsAvatar(t *testing.T) {
	t.Parallel()

	anonymous := newClient(t)
	contentType, form := userAvatarForm(t, []byte("not an image"))
	status, _, body := anonymous.doRaw(http.MethodPut, "/api/web/user/settings/avatar", contentType, form)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	status, _, body = anonymous.doRaw(http.MethodDelete, "/api/web/user/settings/avatar", "", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

	u, c := userSignedIn(t)
	contentType, form = userAvatarForm(t, []byte("not an image"))
	status, _, body = c.doRaw(http.MethodPut, "/api/web/user/settings/avatar", contentType, form)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.False(t, userGet(t, u.ID).UseCustomAvatar)

	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 16, 16))))
	contentType, form = userAvatarForm(t, img.Bytes())
	status, _, body = c.doRaw(http.MethodPut, "/api/web/user/settings/avatar", contentType, form)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, userGet(t, u.ID).UseCustomAvatar)
	var settings struct {
		HasCustomAvatar bool `json:"hasCustomAvatar"`
	}
	c.doJSON(http.MethodGet, "/api/web/user/settings", nil, http.StatusOK, &settings)
	assert.True(t, settings.HasCustomAvatar)

	status, header, body := anonymous.doRaw(http.MethodGet, "/-/avatars/"+strconv.FormatInt(u.ID, 10), "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, "image/png", header.Get("Content-Type"))
	assert.Equal(t, "private", header.Get("Cache-Control"))
	status, _, body = anonymous.doRaw(http.MethodGet, "/-/avatars/0", "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, _, body = c.doRaw(http.MethodDelete, "/api/web/user/settings/avatar", "", nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, userGet(t, u.ID).UseCustomAvatar)
}

func TestUserSettingsGravatar(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t, "PUT /api/web/user/settings/gravatar")

	u, c := userSignedIn(t)
	// The server under test is in offline mode, which disables Gravatar.
	status, body := c.do(http.MethodPut, "/api/web/user/settings/gravatar", map[string]any{"email": "avatar@example.com"})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	assert.Equal(t, u.Email, userGet(t, u.ID).AvatarEmail, "avatar email changed")
}

func TestUserSettingsRepositories(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t, "GET /api/web/user/settings/repositories", "DELETE /api/web/user/settings/repositories/1")

	u, c := userSignedIn(t)
	owner := seedUser(t, CreateUserOptions{})
	own := seedRepository(t, u.ID, CreateRepositoryOptions{Private: true})
	shared := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	unrelated := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})
	sharedRepo, err := getRepository(t, shared.ID)
	require.NoError(t, err)
	require.NoError(t, sharedRepo.AddCollaborator(userGet(t, u.ID)))

	type repo struct {
		ID        int64  `json:"id"`
		Owner     string `json:"owner"`
		Name      string `json:"name"`
		IsPrivate bool   `json:"isPrivate"`
		IsOwner   bool   `json:"isOwner"`
	}
	list := func() []repo {
		var resp struct {
			Items []repo `json:"items"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/settings/repositories", nil, http.StatusOK, &resp)
		return resp.Items
	}
	assert.ElementsMatch(t, []repo{
		{ID: own.ID, Owner: u.Name, Name: own.Name, IsPrivate: true, IsOwner: true},
		{ID: shared.ID, Owner: owner.Name, Name: shared.Name},
	}, list())

	leave := func(id int64) (int, []byte) {
		return c.do(http.MethodDelete, "/api/web/user/settings/repositories/"+strconv.FormatInt(id, 10), map[string]any{})
	}
	// Only repositories the user collaborates on can be left.
	for _, id := range []int64{own.ID, unrelated.ID} {
		status, body := leave(id)
		assert.Equal(t, http.StatusNotFound, status, "repository %d, response: %s", id, body)
	}

	status, body := leave(shared.ID)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, database.IsCollaborator(logger, shared.ID, u.ID))
	assert.Equal(t, []repo{{ID: own.ID, Owner: u.Name, Name: own.Name, IsPrivate: true, IsOwner: true}}, list())
}

func TestUserSettingsOrganizations(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t, "GET /api/web/user/settings/organizations")

	u, c := userSignedIn(t)
	org := &database.User{
		Name:     UniqueName("org-"),
		IsActive: true,
		Type:     database.UserTypeOrganization,
	}
	require.NoError(t, database.CreateOrganization(org, userGet(t, u.ID)))

	type orgItem struct {
		Name string `json:"name"`
	}
	var resp struct {
		Items []orgItem `json:"items"`
	}
	c.doJSON(http.MethodGet, "/api/web/user/settings/organizations", nil, http.StatusOK, &resp)
	assert.Equal(t, []orgItem{{Name: org.Name}}, resp.Items)

	_, other := userSignedIn(t)
	other.doJSON(http.MethodGet, "/api/web/user/settings/organizations", nil, http.StatusOK, &resp)
	assert.Empty(t, resp.Items)
}

func TestUserSettingsEmails(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t,
		"GET /api/web/user/settings/emails",
		"POST /api/web/user/settings/emails",
		"DELETE /api/web/user/settings/emails/a@example.com",
		"PUT /api/web/user/settings/emails/a@example.com/primary",
	)

	u, c := userSignedIn(t)
	type email struct {
		Email       string `json:"email"`
		IsPrimary   bool   `json:"isPrimary"`
		IsActivated bool   `json:"isActivated"`
	}
	list := func() []email {
		var resp struct {
			Items []email `json:"items"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/settings/emails", nil, http.StatusOK, &resp)
		return resp.Items
	}
	emailPath := func(address string) string {
		return "/api/web/user/settings/emails/" + url.PathEscape(address)
	}
	assert.Equal(t, []email{{Email: u.Email, IsPrimary: true, IsActivated: true}}, list())

	// The server under test does not require confirming emails.
	added := UniqueName("added-") + "@example.com"
	var resp struct {
		ConfirmationSent bool `json:"confirmationSent"`
	}
	c.doJSON(http.MethodPost, "/api/web/user/settings/emails", map[string]any{"email": added}, http.StatusCreated, &resp)
	assert.False(t, resp.ConfirmationSent)
	assert.Equal(t, []email{
		{Email: u.Email, IsPrimary: true, IsActivated: true},
		{Email: added, IsActivated: true},
	}, list())

	other := seedUser(t, CreateUserOptions{})
	status, body := c.do(http.MethodPost, "/api/web/user/settings/emails", map[string]any{"email": other.Email})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	unconfirmed := UniqueName("unconfirmed-") + "@example.com"
	require.NoError(t, database.Handle.Users().AddEmail(t.Context(), u.ID, unconfirmed, false))
	status, body = c.do(http.MethodPut, emailPath(unconfirmed)+"/primary", map[string]any{})
	assert.Equal(t, http.StatusConflict, status, "response: %s", body)
	status, body = c.do(http.MethodPut, emailPath(other.Email)+"/primary", map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = c.do(http.MethodPut, emailPath(added)+"/primary", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, added, userGet(t, u.ID).Email)

	status, body = c.do(http.MethodDelete, emailPath(added), map[string]any{})
	assert.Equal(t, http.StatusConflict, status, "primary email deleted, response: %s", body)
	status, body = c.do(http.MethodDelete, emailPath(u.Email), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = c.do(http.MethodDelete, emailPath(unconfirmed), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, []email{{Email: added, IsPrimary: true, IsActivated: true}}, list())
}

// userSeedSSHKey adds an SSH key to the user, which the server under test
// cannot do with SSH disabled. Like a key from ssh-keygen, it ends with a
// comment. The key is also returned without the comment.
func userSeedSSHKey(t *testing.T, userID int64, name string) (*database.PublicKey, string) {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	content := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	key, err := database.Handle.PublicKeys().Add(t.Context(), logger, userID, name, content+" "+name+"@example.com")
	require.NoError(t, err)
	return key, content
}

func TestUserSettingsSSHKeys(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t,
		"GET /api/web/user/settings/ssh-keys",
		"POST /api/web/user/settings/ssh-keys",
		"DELETE /api/web/user/settings/ssh-keys/1",
	)

	u, c := userSignedIn(t)
	repo := seedRepository(t, u.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	privateKey, content := sshNewKey(t)
	sshPub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(content))
	require.NoError(t, err)
	status, body := c.do(http.MethodPost, "/api/web/user/settings/ssh-keys", map[string]any{
		"title":   "Laptop",
		"content": content,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	// The same key cannot be added twice, even under another name.
	status, body = c.do(http.MethodPost, "/api/web/user/settings/ssh-keys", map[string]any{
		"title":   "Again",
		"content": content,
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	userKeys, err := database.Handle.PublicKeys().List(t.Context(), u.ID)
	require.NoError(t, err)
	require.Len(t, userKeys, 1)
	key, fingerprint := userKeys[0], ssh.FingerprintSHA256(sshPub)
	assert.Equal(t, content, strings.TrimSpace(key.Content))
	// The key added signs in to the SSH server.
	privateKey.clone(t, repo)
	type sshKey struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Fingerprint string `json:"fingerprint"`
	}
	list := func() []sshKey {
		var resp struct {
			Items []sshKey `json:"items"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/settings/ssh-keys", nil, http.StatusOK, &resp)
		return resp.Items
	}
	assert.Equal(t, []sshKey{{ID: key.ID, Name: "Laptop", Fingerprint: fingerprint}}, list())

	other := seedUser(t, CreateUserOptions{})
	otherKey, _ := userSeedSSHKey(t, other.ID, "Desktop")
	status, body = c.do(http.MethodDelete, "/api/web/user/settings/ssh-keys/"+strconv.FormatInt(otherKey.ID, 10), map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	_, err = database.Handle.PublicKeys().GetByID(t.Context(), otherKey.ID)
	assert.NoError(t, err, "key of another user deleted")

	status, body = c.do(http.MethodDelete, "/api/web/user/settings/ssh-keys/"+strconv.FormatInt(key.ID, 10), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, list())
	_, err = database.Handle.PublicKeys().GetByID(t.Context(), key.ID)
	assert.True(t, database.IsErrKeyNotExist(err), "key not deleted: %v", err)
	out := privateKey.cloneFails(t, repo)
	assert.Contains(t, out, "Permission denied (publickey)")
}

func TestUserSettingsTwoFactor(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t,
		"GET /api/web/user/settings/two-factor",
		"POST /api/web/user/settings/two-factor",
		"DELETE /api/web/user/settings/two-factor",
		"GET /api/web/user/settings/two-factor/enrollment",
		"GET /api/web/user/settings/two-factor/recovery-codes",
		"POST /api/web/user/settings/two-factor/recovery-codes",
	)

	u, c := userSignedIn(t)
	enabled := func() bool {
		var resp struct {
			Enabled bool `json:"enabled"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/settings/two-factor", nil, http.StatusOK, &resp)
		assert.Equal(t, database.Handle.TwoFactors().IsEnabled(t.Context(), logger, u.ID), resp.Enabled)
		return resp.Enabled
	}
	require.False(t, enabled())

	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/web/user/settings/two-factor/recovery-codes"},
		{http.MethodPost, "/api/web/user/settings/two-factor/recovery-codes"},
		{http.MethodDelete, "/api/web/user/settings/two-factor"},
	} {
		status, body := c.do(r.method, r.path, map[string]any{})
		assert.Equal(t, http.StatusNotFound, status, "%s %s while disabled, response: %s", r.method, r.path, body)
	}
	// A passcode only confirms a secret that was handed out.
	status, body := c.do(http.MethodPost, "/api/web/user/settings/two-factor", map[string]any{"passcode": "123456"})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	type enrollment struct {
		Secret    string `json:"secret"`
		QRCodeURL string `json:"qrCodeURL"`
	}
	var first, second enrollment
	c.doJSON(http.MethodGet, "/api/web/user/settings/two-factor/enrollment", nil, http.StatusOK, &first)
	assert.NotEmpty(t, first.Secret)
	assert.True(t, strings.HasPrefix(first.QRCodeURL, "data:image/png;base64,"), first.QRCodeURL)
	c.doJSON(http.MethodGet, "/api/web/user/settings/two-factor/enrollment", nil, http.StatusOK, &second)
	assert.Equal(t, first.Secret, second.Secret, "secret changed before it was confirmed")

	passcode, err := totp.GenerateCode(first.Secret, time.Now())
	require.NoError(t, err)
	wrong := "000000"
	if passcode == wrong {
		wrong = "111111"
	}
	status, body = c.do(http.MethodPost, "/api/web/user/settings/two-factor", map[string]any{"passcode": wrong})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.False(t, enabled())

	status, body = c.do(http.MethodPost, "/api/web/user/settings/two-factor", map[string]any{"passcode": passcode})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.True(t, enabled())
	status, body = c.do(http.MethodGet, "/api/web/user/settings/two-factor/enrollment", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	type recoveryCode struct {
		Code   string `json:"code"`
		IsUsed bool   `json:"isUsed"`
	}
	storedCodes := func() []recoveryCode {
		codes, err := database.Handle.TwoFactors().ListRecoveryCodes(t.Context(), u.ID)
		require.NoError(t, err)
		var items []recoveryCode
		for _, rc := range codes {
			items = append(items, recoveryCode{Code: rc.Code, IsUsed: rc.IsUsed})
		}
		return items
	}
	var codes struct {
		Items []recoveryCode `json:"items"`
	}
	c.doJSON(http.MethodGet, "/api/web/user/settings/two-factor/recovery-codes", nil, http.StatusOK, &codes)
	before := storedCodes()
	require.NotEmpty(t, before)
	assert.Equal(t, before, codes.Items)

	c.doJSON(http.MethodPost, "/api/web/user/settings/two-factor/recovery-codes", map[string]any{}, http.StatusOK, &codes)
	after := storedCodes()
	assert.Equal(t, after, codes.Items)
	assert.Len(t, after, len(before))
	for _, rc := range before {
		assert.NotContains(t, after, rc, "recovery code kept after regenerating")
	}

	status, body = c.do(http.MethodDelete, "/api/web/user/settings/two-factor", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, enabled())
}

func TestUserSettingsAccessTokens(t *testing.T) {
	t.Parallel()

	userAssertSignInRequired(t,
		"GET /api/web/user/settings/tokens",
		"POST /api/web/user/settings/tokens",
		"DELETE /api/web/user/settings/tokens/1",
	)

	u, c := userSignedIn(t)
	type token struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	list := func() []token {
		var resp struct {
			Items []token `json:"items"`
		}
		c.doJSON(http.MethodGet, "/api/web/user/settings/tokens", nil, http.StatusOK, &resp)
		return resp.Items
	}
	assert.Empty(t, list())

	var created struct {
		Token string `json:"token"`
	}
	c.doJSON(http.MethodPost, "/api/web/user/settings/tokens", map[string]any{"name": "CI"}, http.StatusCreated, &created)
	stored, err := database.Handle.AccessTokens().GetBySHA1(t.Context(), created.Token)
	require.NoError(t, err)
	assert.Equal(t, u.ID, stored.UserID)
	assert.Equal(t, "CI", stored.Name)
	assert.Equal(t, []token{{ID: stored.ID, Name: "CI"}}, list())

	status, body := c.do(http.MethodPost, "/api/web/user/settings/tokens", map[string]any{"name": "CI"})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	// Deleting a token of another user does not delete it.
	other := seedUser(t, CreateUserOptions{})
	otherToken, err := CreateAccessToken(t.Context(), other.ID)
	require.NoError(t, err)
	otherStored, err := database.Handle.AccessTokens().GetBySHA1(t.Context(), otherToken)
	require.NoError(t, err)
	status, body = c.do(http.MethodDelete, "/api/web/user/settings/tokens/"+strconv.FormatInt(otherStored.ID, 10), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.AccessTokens().GetBySHA1(t.Context(), otherToken)
	assert.NoError(t, err, "token of another user deleted")

	status, body = c.do(http.MethodDelete, "/api/web/user/settings/tokens/"+strconv.FormatInt(stored.ID, 10), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, list())
	_, err = database.Handle.AccessTokens().GetBySHA1(t.Context(), created.Token)
	assert.True(t, database.IsErrAccessTokenNotExist(err), "token not deleted: %v", err)
}
