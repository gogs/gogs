package integrationtests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

// repoSettingsNoBody is the body of requests that carry no data, which the web
// API still only accepts as JSON.
var repoSettingsNoBody = struct{}{}

// repoSettingsAPIPath returns the path of the web API of the repository
// followed by the suffix, e.g., "/settings".
func repoSettingsAPIPath(repo *Repository, suffix string) string {
	return "/api/web/" + repo.Owner + "/" + repo.Name + suffix
}

func repoSettingsSignedIn(t *testing.T, u *User) *client {
	t.Helper()

	c := newClient(t)
	c.signIn(u)
	return c
}

// repoSettingsGit runs Git in the directory and returns its trimmed output.
func repoSettingsGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// repoSettingsDefaultBranch returns the branch HEAD points to in the
// repository on disk.
func repoSettingsDefaultBranch(t *testing.T, repo *Repository) string {
	t.Helper()
	return repoSettingsGit(t, database.RepoPath(repo.Owner, repo.Name), "symbolic-ref", "--short", "HEAD")
}

// repoSettingsAddCommit commits on top of the branch of the repository on disk,
// without going through the server, and returns the ID of the commit.
func repoSettingsAddCommit(t *testing.T, repo *Repository, branch, message string) string {
	t.Helper()

	dir := database.RepoPath(repo.Owner, repo.Name)
	parent := repoSettingsGit(t, dir, "rev-parse", "refs/heads/"+branch)
	commit := repoSettingsGit(t, dir, "commit-tree", parent+"^{tree}", "-p", parent, "-m", message)
	repoSettingsGit(t, dir, "update-ref", "refs/heads/"+branch, commit, parent)
	return commit
}

// repoSettingsAddCollaborator gives the user the access to the repository
// through the stores, e.g., to sign in as an admin who does not own it.
func repoSettingsAddCollaborator(t *testing.T, repo *Repository, u *User, mode database.AccessMode) {
	t.Helper()

	r, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	collaborator, err := database.Handle.Users().GetByID(t.Context(), u.ID)
	require.NoError(t, err)
	require.NoError(t, r.AddCollaborator(collaborator))
	require.NoError(t, r.ChangeCollaborationAccessMode(database.AccessModeOwner, u.ID, mode))
}

// repoSettingsFieldError returns the message of the field in a response of
// failed validation.
func repoSettingsFieldError(t *testing.T, body []byte, field string) string {
	t.Helper()

	var resp struct {
		Fields map[string]*string `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
	require.Contains(t, resp.Fields, field, "response: %s", body)
	if resp.Fields[field] == nil {
		return ""
	}
	return *resp.Fields[field]
}

func TestRepoSettingsGeneral(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	writer := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, repo, writer, database.AccessModeWrite)

	ownerClient := repoSettingsSignedIn(t, owner)
	var settings struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Website       string `json:"website"`
		Private       bool   `json:"private"`
		IsFork        bool   `json:"isFork"`
		ViewerIsOwner bool   `json:"viewerIsOwner"`
		Mirror        any    `json:"mirror"`
	}
	ownerClient.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
	assert.Equal(t, repo.Name, settings.Name)
	assert.False(t, settings.Private)
	assert.False(t, settings.IsFork)
	assert.True(t, settings.ViewerIsOwner)
	assert.Nil(t, settings.Mirror)

	for name, c := range map[string]*client{
		"writer": repoSettingsSignedIn(t, writer),
		"other":  repoSettingsSignedIn(t, other),
	} {
		status, body := c.do(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil)
		assert.Equal(t, http.StatusNotFound, status, "%s, response: %s", name, body)
	}
	status, body := newClient(t).do(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

	newName := UniqueName("renamed-")
	var renamed struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}
	ownerClient.doJSON(http.MethodPatch, repoSettingsAPIPath(repo, "/settings"), map[string]any{
		"name":        newName,
		"description": "A renamed repository",
		"website":     "https://example.com",
		"private":     true,
	}, http.StatusOK, &renamed)
	assert.Equal(t, owner.Name, renamed.Owner)
	assert.Equal(t, newName, renamed.Name)

	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, newName, got.Name)
	assert.Equal(t, "A renamed repository", got.Description)
	assert.Equal(t, "https://example.com", got.Website)
	assert.True(t, got.IsPrivate)
	assert.DirExists(t, database.RepoPath(owner.Name, newName))
	assert.NoDirExists(t, database.RepoPath(owner.Name, repo.Name))

	renamedRepo := &Repository{ID: repo.ID, Owner: owner.Name, Name: newName}
	ownerClient.doJSON(http.MethodGet, repoSettingsAPIPath(renamedRepo, "/settings"), nil, http.StatusOK, &settings)
	assert.Equal(t, newName, settings.Name)
	assert.Equal(t, "A renamed repository", settings.Description)
	assert.True(t, settings.Private)

	// Another repository of the owner already has the name.
	taken := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	status, body = ownerClient.do(http.MethodPatch, repoSettingsAPIPath(renamedRepo, "/settings"), map[string]any{"name": taken.Name})
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "name"))
}

func TestRepoSettingsAdvanced(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)

	advanced := map[string]any{
		"enableWiki":            false,
		"enableIssues":          true,
		"allowPublicIssues":     true,
		"enableExternalTracker": false,
		"enablePulls":           true,
		"pullsAllowMerge":       false,
		"pullsAllowSquash":      true,
		"pullsAllowRebase":      false,
		"pullsDeleteBranch":     true,
		"pullsSquashMessage":    "title_commits",
	}
	status, body := c.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), advanced)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.False(t, got.EnableWiki)
	assert.True(t, got.EnableIssues)
	assert.True(t, got.AllowPublicIssues)
	assert.True(t, got.EnablePulls)
	assert.False(t, got.PullsAllowMerge)
	assert.True(t, got.PullsAllowSquash)
	assert.True(t, got.PullsDeleteBranch)
	assert.Equal(t, "title_commits", got.PullsSquashMessage)

	var settings struct {
		Advanced struct {
			EnableWiki         bool   `json:"enableWiki"`
			PullsAllowSquash   bool   `json:"pullsAllowSquash"`
			PullsSquashMessage string `json:"pullsSquashMessage"`
		} `json:"advanced"`
	}
	c.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
	assert.False(t, settings.Advanced.EnableWiki)
	assert.True(t, settings.Advanced.PullsAllowSquash)
	assert.Equal(t, "title_commits", settings.Advanced.PullsSquashMessage)

	// Pull requests need a way to be merged.
	advanced["pullsAllowSquash"] = false
	advanced["enableWiki"] = true
	status, body = c.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), advanced)
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	got, err = getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.False(t, got.EnableWiki, "settings changed by a refused request")
}

func TestRepoSettingsAvatar(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	c := repoSettingsSignedIn(t, owner)

	upload := func(c *client, filename string, data []byte) (int, []byte) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := w.CreateFormFile("avatar", filename)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		status, _, body := c.doRaw(http.MethodPut, repoSettingsAPIPath(repo, "/settings/avatar"), w.FormDataContentType(), &buf)
		return status, body
	}
	var img bytes.Buffer
	require.NoError(t, png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 4, 4))))

	status, body := upload(repoSettingsSignedIn(t, other), "avatar.png", img.Bytes())
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = upload(c, "avatar.txt", []byte("not an image"))
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	status, body = upload(c, "avatar.png", img.Bytes())
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.True(t, got.UseCustomAvatar)
	avatarPath := filepath.Join(conf.Picture.RepositoryAvatarUploadPath, strconv.FormatInt(repo.ID, 10))
	assert.FileExists(t, avatarPath)

	type avatarSettings struct {
		AvatarURL       string `json:"avatarURL"`
		HasCustomAvatar bool   `json:"hasCustomAvatar"`
	}
	var settings avatarSettings
	c.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
	assert.True(t, settings.HasCustomAvatar)
	assert.Equal(t, "/-/repo-avatars/"+strconv.FormatInt(repo.ID, 10), settings.AvatarURL)

	status, _, body = c.doRaw(http.MethodDelete, repoSettingsAPIPath(repo, "/settings/avatar"), "", nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.NoFileExists(t, avatarPath)
	settings = avatarSettings{}
	c.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings"), nil, http.StatusOK, &settings)
	assert.False(t, settings.HasCustomAvatar)
	assert.NotEqual(t, "/-/repo-avatars/"+strconv.FormatInt(repo.ID, 10), settings.AvatarURL)
}

func TestRepoAvatarAccess(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	reader := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{Admin: true})
	disabled := seedUser(t, CreateUserOptions{})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	repoSettingsAddCollaborator(t, private, reader, database.AccessModeRead)
	repoSettingsAddCollaborator(t, private, disabled, database.AccessModeRead)

	// The disabled collaborator stays authenticated by the session they had
	// before their account was disabled.
	disabledClient := repoSettingsSignedIn(t, disabled)
	prohibitLogin := true
	require.NoError(t, database.Handle.Users().Update(t.Context(), disabled.ID, database.UpdateUserOptions{ProhibitLogin: &prohibitLogin}))

	avatar := []byte("avatar")
	require.NoError(t, os.MkdirAll(conf.Picture.RepositoryAvatarUploadPath, 0o755))
	for _, repo := range []*Repository{private, public} {
		require.NoError(t, os.WriteFile(filepath.Join(conf.Picture.RepositoryAvatarUploadPath, strconv.FormatInt(repo.ID, 10)), avatar, 0o644))
	}
	avatarPath := func(repo *Repository) string {
		return "/-/repo-avatars/" + strconv.FormatInt(repo.ID, 10)
	}

	tests := []struct {
		name       string
		client     *client
		repo       *Repository
		wantStatus int
	}{
		{name: "owner of a private repository", client: repoSettingsSignedIn(t, owner), repo: private, wantStatus: http.StatusOK},
		{name: "reader of a private repository", client: repoSettingsSignedIn(t, reader), repo: private, wantStatus: http.StatusOK},
		{name: "site admin of a private repository", client: repoSettingsSignedIn(t, admin), repo: private, wantStatus: http.StatusOK},
		{name: "other user of a private repository", client: repoSettingsSignedIn(t, other), repo: private, wantStatus: http.StatusNotFound},
		{name: "disabled reader of a private repository", client: disabledClient, repo: private, wantStatus: http.StatusNotFound},
		{name: "anonymous of a private repository", client: newClient(t), repo: private, wantStatus: http.StatusNotFound},
		{name: "anonymous of a public repository", client: newClient(t), repo: public, wantStatus: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, header, body := test.client.doRaw(http.MethodGet, avatarPath(test.repo), "", nil)
			require.Equal(t, test.wantStatus, status, "response: %s", body)
			if test.wantStatus == http.StatusOK {
				assert.Equal(t, avatar, body)
				assert.Equal(t, "private", header.Get("Cache-Control"))
			} else {
				assert.NotEqual(t, avatar, body)
			}
		})
	}

	status, _, body := newClient(t).doRaw(http.MethodGet, "/-/repo-avatars/0", "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	// The old path is the page of a user named "repo-avatars", not the avatar.
	_, _, body = newClient(t).doRaw(http.MethodGet, "/repo-avatars/"+strconv.FormatInt(public.ID, 10), "", nil)
	assert.NotEqual(t, avatar, body)
}

func TestRepoSettingsDelete(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, repo, admin, database.AccessModeAdmin)
	path := repoSettingsAPIPath(repo, "")

	// Admins who do not own the repository manage its settings but cannot
	// delete it.
	status, body := repoSettingsSignedIn(t, admin).do(http.MethodDelete, path, map[string]any{"repoName": repo.Name})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)

	c := repoSettingsSignedIn(t, owner)
	status, body = c.do(http.MethodDelete, path, map[string]any{"repoName": repo.Name + "x"})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "repoName"))
	_, err := getRepository(t, repo.ID)
	require.NoError(t, err, "deleted with the wrong name")

	status, body = c.do(http.MethodDelete, path, map[string]any{"repoName": repo.Name})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = getRepository(t, repo.ID)
	assert.True(t, database.IsErrRepoNotExist(err), "repository not deleted: %v", err)
	assert.NoDirExists(t, database.RepoPath(repo.Owner, repo.Name))
}

func TestRepoSettingsTransfer(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{})
	newOwner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, repo, admin, database.AccessModeAdmin)
	repoSettingsAddCollaborator(t, repo, newOwner, database.AccessModeWrite)
	path := repoSettingsAPIPath(repo, "/settings/transfer")

	status, body := repoSettingsSignedIn(t, admin).do(http.MethodPost, path, map[string]any{"repoName": repo.Name, "newOwner": newOwner.Name})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)

	c := repoSettingsSignedIn(t, owner)
	status, body = c.do(http.MethodPost, path, map[string]any{"repoName": repo.Name, "newOwner": UniqueName("nobody-")})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "newOwner"))

	var resp struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}
	c.doJSON(http.MethodPost, path, map[string]any{"repoName": repo.Name, "newOwner": newOwner.Name}, http.StatusOK, &resp)
	assert.Equal(t, newOwner.Name, resp.Owner)
	assert.Equal(t, repo.Name, resp.Name)

	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.Equal(t, newOwner.ID, got.OwnerID)
	assert.DirExists(t, database.RepoPath(newOwner.Name, repo.Name))
	assert.NoDirExists(t, database.RepoPath(owner.Name, repo.Name))
	// The new owner stops being a collaborator, the others stay.
	collaborators, err := got.GetCollaborators()
	require.NoError(t, err)
	var names []string
	for _, c := range collaborators {
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{admin.Name}, names)
}

func TestRepoSettingsDeleteWiki(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, repo, admin, database.AccessModeAdmin)
	r, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	require.NoError(t, r.InitWiki(t.Context()))
	require.True(t, r.EnableWiki)
	path := repoSettingsAPIPath(repo, "/settings/wiki")

	status, body := repoSettingsSignedIn(t, admin).do(http.MethodDelete, path, map[string]any{"repoName": repo.Name})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	assert.True(t, r.HasWiki())

	status, body = repoSettingsSignedIn(t, owner).do(http.MethodDelete, path, map[string]any{"repoName": repo.Name})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.False(t, r.HasWiki())
	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.False(t, got.EnableWiki)
}

func TestRepoSettingsCollaborators(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	collaborator := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)
	path := repoSettingsAPIPath(repo, "/settings/collaborators")

	type collaborators struct {
		Items []struct {
			Name string `json:"name"`
			Mode string `json:"mode"`
		} `json:"items"`
	}
	modes := func() map[string]string {
		var resp collaborators
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		m := make(map[string]string)
		for _, item := range resp.Items {
			m[item.Name] = item.Mode
		}
		return m
	}
	assert.Empty(t, modes())

	status, body := c.do(http.MethodPost, path, map[string]any{"username": UniqueName("nobody-")})
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "username"))

	status, body = c.do(http.MethodPost, path, map[string]any{"username": collaborator.Name})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	assert.Equal(t, map[string]string{collaborator.Name: "write"}, modes())

	// Collaborators who are not admins do not see the settings.
	collaboratorClient := repoSettingsSignedIn(t, collaborator)
	status, body = collaboratorClient.do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = c.do(http.MethodPatch, path+"/"+collaborator.Name, map[string]any{"mode": "read"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, map[string]string{collaborator.Name: "read"}, modes())
	r, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	got, err := r.GetCollaborators()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, database.AccessModeRead, got[0].Collaboration.Mode)

	status, body = c.do(http.MethodDelete, path+"/"+collaborator.Name, repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Empty(t, modes())
	got, err = r.GetCollaborators()
	require.NoError(t, err)
	assert.Empty(t, got)
}

// repoSettingsPublicKey returns a new SSH public key in the format of
// authorized_keys.
func repoSettingsPublicKey(t *testing.T) string {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

// repoSettingsSeedDeployKey adds a deploy key to the repository through the
// stores.
func repoSettingsSeedDeployKey(t *testing.T, repo *Repository, name string) *database.DeployKey {
	t.Helper()

	key, err := database.Handle.DeployKeys().Add(t.Context(), logger, repo.ID, name, repoSettingsPublicKey(t))
	require.NoError(t, err)
	return key
}

func TestRepoSettingsDeployKeys(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)
	path := repoSettingsAPIPath(repo, "/settings/keys")

	content := repoSettingsPublicKey(t)
	status, body := repoSettingsSignedIn(t, other).do(http.MethodPost, path, map[string]any{"title": "deploy", "content": content})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = c.do(http.MethodPost, path, map[string]any{"title": "deploy", "content": content})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	// The same key cannot be added twice.
	status, body = c.do(http.MethodPost, path, map[string]any{"title": "again", "content": content})
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	keysOfRepo, err := database.Handle.DeployKeys().List(t.Context(), repo.ID)
	require.NoError(t, err)
	require.Len(t, keysOfRepo, 1)
	key := keysOfRepo[0]
	assert.Equal(t, "deploy", key.Name)
	stored, err := database.Handle.PublicKeys().GetByID(t.Context(), key.KeyID)
	require.NoError(t, err)
	assert.Equal(t, content, strings.TrimSpace(stored.Content))

	otherKey := repoSettingsSeedDeployKey(t, otherRepo, "other")

	type keys struct {
		Items []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Fingerprint string `json:"fingerprint"`
		} `json:"items"`
	}
	var resp keys
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, key.ID, resp.Items[0].ID)
	assert.Equal(t, "deploy", resp.Items[0].Name)
	assert.Equal(t, key.Fingerprint, resp.Items[0].Fingerprint)

	status, body = repoSettingsSignedIn(t, other).do(http.MethodGet, path, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	// Keys of other repositories are not found.
	status, body = c.do(http.MethodDelete, path+"/"+strconv.FormatInt(otherKey.ID, 10), repoSettingsNoBody)
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	_, err = database.Handle.DeployKeys().GetByID(t.Context(), otherKey.ID)
	require.NoError(t, err)

	status, body = c.do(http.MethodDelete, path+"/"+strconv.FormatInt(key.ID, 10), repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.DeployKeys().GetByID(t.Context(), key.ID)
	assert.True(t, database.IsErrDeployKeyNotExist(err), "key not deleted: %v", err)
	resp = keys{}
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
	assert.Empty(t, resp.Items)
}

func TestRepoSettingsBranches(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	writer := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, repo, writer, database.AccessModeWrite)
	defaultBranch := repoSettingsDefaultBranch(t, repo)
	const branch = "release/v1"
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	repoSettingsGit(t, repoPath, "branch", branch, defaultBranch)
	c := repoSettingsSignedIn(t, owner)

	type branchSettings struct {
		IsEmpty           bool     `json:"isEmpty"`
		DefaultBranch     string   `json:"defaultBranch"`
		Branches          []string `json:"branches"`
		ProtectedBranches []string `json:"protectedBranches"`
	}
	var settings branchSettings
	c.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings/branches"), nil, http.StatusOK, &settings)
	assert.False(t, settings.IsEmpty)
	assert.Equal(t, defaultBranch, settings.DefaultBranch)
	assert.ElementsMatch(t, []string{defaultBranch, branch}, settings.Branches)
	assert.Empty(t, settings.ProtectedBranches)

	status, body := repoSettingsSignedIn(t, writer).do(http.MethodGet, repoSettingsAPIPath(repo, "/settings/branches"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	t.Run("protected branch", func(t *testing.T) {
		path := repoSettingsAPIPath(repo, "/settings/branches/"+branch)
		type protection struct {
			Name               string `json:"name"`
			Protected          bool   `json:"protected"`
			RequirePullRequest bool   `json:"requirePullRequest"`
		}
		var got protection
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &got)
		assert.Equal(t, protection{Name: branch}, got)

		status, body := c.do(http.MethodPut, path, map[string]any{"protected": true, "requirePullRequest": true})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &got)
		assert.Equal(t, protection{Name: branch, Protected: true, RequirePullRequest: true}, got)

		stored, err := database.Handle.ProtectBranches().GetByName(t.Context(), repo.ID, branch)
		require.NoError(t, err)
		assert.True(t, stored.Protected)
		assert.True(t, stored.RequirePullRequest)

		var settings branchSettings
		c.doJSON(http.MethodGet, repoSettingsAPIPath(repo, "/settings/branches"), nil, http.StatusOK, &settings)
		assert.Equal(t, []string{branch}, settings.ProtectedBranches)

		status, body = c.do(http.MethodGet, repoSettingsAPIPath(repo, "/settings/branches/no-such-branch"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("default branch", func(t *testing.T) {
		path := repoSettingsAPIPath(repo, "/settings/default-branch")
		status, body := c.do(http.MethodPut, path, map[string]any{"branch": "no-such-branch"})
		require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		assert.NotEmpty(t, repoSettingsFieldError(t, body, "branch"))

		status, body = c.do(http.MethodPut, path, map[string]any{"branch": branch})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Equal(t, branch, repoSettingsDefaultBranch(t, repo))
		got, err := getRepository(t, repo.ID)
		require.NoError(t, err)
		assert.Equal(t, branch, got.DefaultBranch)
	})
}

func TestRepoSettingsGitHooks(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)
	path := repoSettingsAPIPath(repo, "/settings/githooks")

	// Git hooks run code on the server, which takes the permission of a site
	// admin.
	status, body := c.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	allow := true
	require.NoError(t, database.Handle.Users().Update(t.Context(), owner.ID, database.UpdateUserOptions{AllowGitHook: &allow}))

	type hook struct {
		Name     string `json:"name"`
		IsActive bool   `json:"isActive"`
	}
	var hooks struct {
		Items []hook `json:"items"`
	}
	c.doJSON(http.MethodGet, path, nil, http.StatusOK, &hooks)
	assert.ElementsMatch(t, []hook{{Name: "pre-receive"}, {Name: "update"}, {Name: "post-receive"}}, hooks.Items)

	type hookContent struct {
		Name     string `json:"name"`
		IsActive bool   `json:"isActive"`
		Content  string `json:"content"`
	}
	var got hookContent
	c.doJSON(http.MethodGet, path+"/pre-receive", nil, http.StatusOK, &got)
	assert.Equal(t, "pre-receive", got.Name)
	assert.False(t, got.IsActive)

	status, body = c.do(http.MethodGet, path+"/pre-commit", nil)
	assert.Equal(t, http.StatusNotFound, status, "client-side hook, response: %s", body)
	status, body = c.do(http.MethodPut, path+"/pre-receive", map[string]any{"content": "  \n"})
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	const content = "#!/bin/sh\nexit 0"
	status, body = c.do(http.MethodPut, path+"/pre-receive", map[string]any{"content": content})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	hookPath := filepath.Join(database.RepoPath(repo.Owner, repo.Name), "custom_hooks", "pre-receive")
	onDisk, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(onDisk))
	c.doJSON(http.MethodGet, path+"/pre-receive", nil, http.StatusOK, &got)
	assert.True(t, got.IsActive)
	assert.Equal(t, content, got.Content)

	status, body = c.do(http.MethodDelete, path+"/pre-receive", repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.NoFileExists(t, hookPath)
	c.doJSON(http.MethodGet, path+"/pre-receive", nil, http.StatusOK, &got)
	assert.False(t, got.IsActive)
}

func TestRepoSettingsGitHooksRunOnPush(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	allow := true
	require.NoError(t, database.Handle.Users().Update(t.Context(), owner.ID, database.UpdateUserOptions{AllowGitHook: &allow}))
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)
	path := repoSettingsAPIPath(repo, "/settings/githooks")
	hooksDir := filepath.Join(database.RepoPath(repo.Owner, repo.Name), "hooks")

	// The update hook rejects tags, and the post-receive hook keeps what Git
	// passes it.
	received := filepath.Join(t.TempDir(), "post-receive")
	hooks := map[string]string{
		"update":       "#!/bin/sh\ncase \"$1\" in refs/tags/*) echo 'tags are not allowed' >&2; exit 1;; esac\n",
		"post-receive": "#!/bin/sh\ncat >> '" + received + "'\n",
	}
	for name, content := range hooks {
		status, body := c.do(http.MethodPut, path+"/"+name, map[string]any{"content": content})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}

	dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	want := gitCommitFile(t, dir, UniqueName("file-")+".txt")
	gitRun(t, dir, "tag", "v1")
	before := len(gitRepoActions(t, owner.ID, repo.ID))
	out, err := gitCommand(dir, "push", "origin", "HEAD:refs/heads/feature", "v1").CombinedOutput()
	require.Error(t, err, "push succeeded: %s", out)
	assert.Contains(t, string(out), "tags are not allowed")

	// Only the branch that the update hook accepted is pushed and recorded.
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	assert.Equal(t, want, gitRun(t, t.TempDir(), "--git-dir", repoPath, "rev-parse", "refs/heads/feature"))
	_, err = gitCommand(t.TempDir(), "--git-dir", repoPath, "rev-parse", "--verify", "refs/tags/v1").CombinedOutput()
	assert.Error(t, err, "rejected tag was pushed")
	assert.ElementsMatch(t, []gitRefAction{
		{OpType: database.ActionCreateBranch, RefName: "feature"},
		{OpType: database.ActionCommitRepo, RefName: "feature"},
	}, gitNewRefActions(t, owner.ID, repo.ID, before))
	got, err := os.ReadFile(received)
	require.NoError(t, err)
	fields := strings.Fields(string(got))
	require.Len(t, fields, 3, "post-receive input: %s", got)
	assert.Equal(t, []string{want, "refs/heads/feature"}, fields[1:])

	// Without custom hooks, pushes run no Gogs hook again.
	for name := range hooks {
		status, body := c.do(http.MethodDelete, path+"/"+name, repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}
	for _, hook := range []string{"pre-receive", "update", "post-receive"} {
		assert.NoFileExists(t, filepath.Join(hooksDir, hook))
	}
	before = len(gitRepoActions(t, owner.ID, repo.ID))
	gitRun(t, dir, "push", "origin", "v1")
	assert.Equal(t, []gitRefAction{{OpType: database.ActionPushTag, RefName: "v1"}}, gitNewRefActions(t, owner.ID, repo.ID, before))
}
