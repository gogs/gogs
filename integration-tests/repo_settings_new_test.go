package integrationtests

import (
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

const sshCloneAddrError = "SSH clone addresses are not supported, use the HTTP or HTTPS address with a username and password instead."

type repoSettingsCreated struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// repoSettingsMigrate migrates the public repository from the server under
// test over HTTP as the user, and returns the new repository and the address
// it was cloned from.
func repoSettingsMigrate(t *testing.T, c *client, u *User, source *Repository, mirror bool) (repo *Repository, cloneAddr string) {
	t.Helper()

	cloneAddr = serverURL + "/" + source.Owner + "/" + source.Name + ".git"
	var created repoSettingsCreated
	c.doJSON(http.MethodPost, "/api/web/repo/migrate", map[string]any{
		"cloneAddr":   cloneAddr,
		"name":        UniqueName("migrated-"),
		"mirror":      mirror,
		"description": "Migrated",
	}, http.StatusCreated, &created)
	require.Equal(t, u.Name, created.Owner)

	got, err := getRepositoryByName(t, u.ID, created.Name)
	require.NoError(t, err)
	return &Repository{ID: got.ID, Owner: created.Owner, Name: created.Name}, cloneAddr
}

// repoSettingsHead returns the commit the branch points to in the repository
// on disk.
func repoSettingsHead(t *testing.T, repo *Repository, branch string) string {
	t.Helper()
	return repoSettingsGit(t, database.RepoPath(repo.Owner, repo.Name), "rev-parse", "refs/heads/"+branch)
}

func TestRepoSettingsNewRepository(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, u)

	var options struct {
		Owners []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"owners"`
		OwnerID    int64    `json:"ownerID"`
		Gitignores []string `json:"gitignores"`
		Licenses   []string `json:"licenses"`
		Readmes    []string `json:"readmes"`
	}
	c.doJSON(http.MethodGet, "/api/web/repo/new", nil, http.StatusOK, &options)
	require.Len(t, options.Owners, 1)
	assert.Equal(t, u.ID, options.Owners[0].ID)
	assert.Equal(t, u.Name, options.Owners[0].Name)
	assert.Equal(t, u.ID, options.OwnerID)
	assert.Contains(t, options.Gitignores, "Go")
	assert.Contains(t, options.Licenses, "MIT License")
	assert.Contains(t, options.Readmes, "Default")

	// Repositories are only created for the viewer and the organizations they
	// own.
	otherOwner := "?org=" + strconv.FormatInt(other.ID, 10)
	status, body := c.do(http.MethodGet, "/api/web/repo/new"+otherOwner, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = newClient(t).do(http.MethodGet, "/api/web/repo/new", nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
}

func TestRepoSettingsCreateRepository(t *testing.T) {
	t.Parallel()

	u := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	c := authenticatedClient(t, u)
	name := UniqueName("created-")
	request := map[string]any{
		"name":        name,
		"description": "Created through the web API",
		"private":     true,
		"autoInit":    true,
		"gitignores":  []string{"Go"},
		"license":     "MIT License",
		"readme":      "Default",
	}

	status, body := c.do(http.MethodPost, "/api/web/repo/create", map[string]any{"name": name, "ownerID": other.ID})
	require.Equal(t, http.StatusForbidden, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "ownerID"))

	var created repoSettingsCreated
	c.doJSON(http.MethodPost, "/api/web/repo/create", request, http.StatusCreated, &created)
	assert.Equal(t, repoSettingsCreated{Owner: u.Name, Name: name}, created)

	got, err := getRepositoryByName(t, u.ID, name)
	require.NoError(t, err)
	assert.Equal(t, "Created through the web API", got.Description)
	assert.True(t, got.IsPrivate)
	assert.False(t, got.IsEmpty)
	files := repoSettingsGit(t, database.RepoPath(u.Name, name), "ls-tree", "--name-only", "HEAD")
	assert.ElementsMatch(t, []string{".gitignore", "LICENSE", "README.md"}, strings.Split(files, "\n"))

	status, body = c.do(http.MethodPost, "/api/web/repo/create", request)
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "name"))

	request["name"] = UniqueName("created-")
	request["license"] = "No Such License"
	status, body = c.do(http.MethodPost, "/api/web/repo/create", request)
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "license"))
	_, err = getRepositoryByName(t, u.ID, request["name"].(string))
	assert.True(t, database.IsErrRepoNotExist(err), "repository created with an unknown license: %v", err)
}

func TestRepoSettingsMigrateRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	u := seedUser(t, CreateUserOptions{})
	source := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	branch := repoSettingsDefaultBranch(t, source)
	c := authenticatedClient(t, u)

	repo, _ := repoSettingsMigrate(t, c, u, source, false)
	assert.Equal(t, repoSettingsHead(t, source, branch), repoSettingsHead(t, repo, branch))
	got, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	assert.False(t, got.IsMirror)
	assert.Equal(t, "Migrated", got.Description)

	// Importing from a path on the server takes a permission the user does not
	// have.
	name := UniqueName("migrated-")
	status, body := c.do(http.MethodPost, "/api/web/repo/migrate", map[string]any{
		"cloneAddr": database.RepoPath(source.Owner, source.Name),
		"name":      name,
	})
	require.Equal(t, http.StatusForbidden, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "cloneAddr"))
	_, err = getRepositoryByName(t, u.ID, name)
	assert.True(t, database.IsErrRepoNotExist(err), "repository migrated from a local path: %v", err)

	// An SSH address is not a path on the server, so it is not refused as one.
	status, body = c.do(http.MethodPost, "/api/web/repo/migrate", map[string]any{
		"cloneAddr": "git@gitlab.example.com:owner/repo.git",
		"name":      name,
	})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.Equal(t, sshCloneAddrError, repoSettingsFieldError(t, body, "cloneAddr"))
	_, err = getRepositoryByName(t, u.ID, name)
	assert.True(t, database.IsErrRepoNotExist(err), "repository migrated from an SSH address: %v", err)
}

func TestRepoSettingsMirror(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	u := seedUser(t, CreateUserOptions{})
	source := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	branch := repoSettingsDefaultBranch(t, source)
	c := authenticatedClient(t, u)
	mirror, cloneAddr := repoSettingsMigrate(t, c, u, source, true)
	assert.Equal(t, repoSettingsHead(t, source, branch), repoSettingsHead(t, mirror, branch))

	type mirrorSettings struct {
		Address     string `json:"address"`
		Interval    int    `json:"interval"`
		EnablePrune bool   `json:"enablePrune"`
	}
	var settings struct {
		Mirror         *mirrorSettings `json:"mirror"`
		CanEnablePulls bool            `json:"canEnablePulls"`
	}
	c.doJSON(http.MethodGet, repoSettingsAPIPath(mirror, "/settings"), nil, http.StatusOK, &settings)
	require.NotNil(t, settings.Mirror)
	assert.Equal(t, cloneAddr, settings.Mirror.Address)
	assert.Equal(t, conf.Mirror.DefaultInterval, settings.Mirror.Interval)
	assert.False(t, settings.CanEnablePulls)

	// Mirrors follow the branches of their source.
	status, body := c.do(http.MethodGet, repoSettingsAPIPath(mirror, "/settings/branches"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = c.do(http.MethodPut, repoSettingsAPIPath(mirror, "/settings/mirror"), map[string]any{
		"address":     cloneAddr,
		"interval":    24,
		"enablePrune": false,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	stored, err := database.Handle.Mirrors().GetByRepoID(t.Context(), mirror.ID)
	require.NoError(t, err)
	assert.Equal(t, 24, stored.Interval)
	assert.False(t, stored.EnablePrune)
	settings.Mirror = nil
	c.doJSON(http.MethodGet, repoSettingsAPIPath(mirror, "/settings"), nil, http.StatusOK, &settings)
	require.NotNil(t, settings.Mirror)
	assert.Equal(t, mirrorSettings{Address: cloneAddr, Interval: 24}, *settings.Mirror)

	status, body = c.do(http.MethodPut, repoSettingsAPIPath(mirror, "/settings/mirror"), map[string]any{
		"address":     "ssh://git@gitlab.example.com/owner/repo.git",
		"interval":    24,
		"enablePrune": false,
	})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.Equal(t, sshCloneAddrError, repoSettingsFieldError(t, body, "address"))
	settings.Mirror = nil
	c.doJSON(http.MethodGet, repoSettingsAPIPath(mirror, "/settings"), nil, http.StatusOK, &settings)
	require.NotNil(t, settings.Mirror)
	assert.Equal(t, cloneAddr, settings.Mirror.Address)

	mirrorPath := database.RepoPath(mirror.Owner, mirror.Name)
	sourcePath := database.RepoPath(source.Owner, source.Name)
	// sync adds a commit to the source and waits for the mirror to fetch it,
	// along with any other change made to the source before.
	sync := func(message string) {
		t.Helper()
		commit := repoSettingsAddCommit(t, source, branch, message)
		status, body := c.do(http.MethodPost, repoSettingsAPIPath(mirror, "/settings/mirror/sync"), repoSettingsNoBody)
		require.Equal(t, http.StatusAccepted, status, "response: %s", body)
		require.Eventually(t, func() bool {
			// Not repoSettingsHead, which cannot fail the test from the goroutine
			// the condition runs in.
			out, err := exec.Command("git", "-C", mirrorPath, "rev-parse", "refs/heads/"+branch).Output()
			return err == nil && strings.TrimSpace(string(out)) == commit
		}, 30*time.Second, 200*time.Millisecond, "mirror not synced")
	}
	hasBranch := func(name string) bool {
		return exec.Command("git", "-C", mirrorPath, "rev-parse", "--verify", "refs/heads/"+name).Run() == nil
	}

	// Saving the settings set the next sync time as well, so start in a later
	// second to tell the two apart.
	time.Sleep(time.Until(time.Unix(time.Now().Unix()+1, 0)))
	before := time.Now().Unix()
	sync("After the mirror")
	// The sync is recorded once the mirror is updated, and the next one is
	// scheduled the interval after it.
	require.Eventually(t, func() bool {
		stored, err = database.Handle.Mirrors().GetByRepoID(t.Context(), mirror.ID)
		return err == nil && stored.LastSyncUnix >= before
	}, 30*time.Second, 200*time.Millisecond, "sync not recorded")
	assert.LessOrEqual(t, stored.LastSyncUnix, time.Now().Unix())
	assert.GreaterOrEqual(t, stored.NextSyncUnix, before+24*60*60)
	assert.LessOrEqual(t, stored.NextSyncUnix, time.Now().Unix()+24*60*60)

	// Without pruning, branches deleted from the source stay.
	repoSettingsGit(t, sourcePath, "branch", "gone", branch)
	sync("Add a branch")
	require.True(t, hasBranch("gone"))
	repoSettingsGit(t, sourcePath, "branch", "-D", "gone")
	sync("Delete a branch")
	assert.True(t, hasBranch("gone"), "branch pruned while pruning is off")

	status, body = c.do(http.MethodPut, repoSettingsAPIPath(mirror, "/settings/mirror"), map[string]any{
		"address":     cloneAddr,
		"interval":    24,
		"enablePrune": true,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	sync("Prune")
	assert.False(t, hasBranch("gone"), "branch not pruned")

	regular := seedRepository(t, u.ID, CreateRepositoryOptions{AutoInit: true})
	status, body = c.do(http.MethodPost, repoSettingsAPIPath(regular, "/settings/mirror/sync"), repoSettingsNoBody)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = c.do(http.MethodPut, repoSettingsAPIPath(regular, "/settings/mirror"), map[string]any{"address": cloneAddr, "interval": 24})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = c.do(http.MethodDelete, repoSettingsAPIPath(mirror, ""), map[string]any{"repoName": mirror.Name})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	_, err = database.Handle.Mirrors().GetByRepoID(t.Context(), mirror.ID)
	assert.True(t, database.IsMirrorNotExist(err), "mirror of deleted repository left behind: %v", err)
}

func TestRepoSettingsConvertMirror(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	u := seedUser(t, CreateUserOptions{})
	admin := seedUser(t, CreateUserOptions{})
	source := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, u)
	mirror, _ := repoSettingsMigrate(t, c, u, source, true)
	repoSettingsAddCollaborator(t, mirror, admin, database.AccessModeAdmin)
	path := repoSettingsAPIPath(mirror, "/settings/convert")

	status, body := authenticatedClient(t, admin).do(http.MethodPost, path, map[string]any{"repoName": mirror.Name})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = c.do(http.MethodPost, path, map[string]any{"repoName": mirror.Name + "x"})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "repoName"))
	got, err := getRepository(t, mirror.ID)
	require.NoError(t, err)
	require.True(t, got.IsMirror)

	status, body = c.do(http.MethodPost, path, map[string]any{"repoName": mirror.Name})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	got, err = getRepository(t, mirror.ID)
	require.NoError(t, err)
	assert.False(t, got.IsMirror)
	_, err = database.Handle.Mirrors().GetByRepoID(t.Context(), mirror.ID)
	assert.Error(t, err, "mirror left behind")

	var settings struct {
		Mirror any `json:"mirror"`
	}
	c.doJSON(http.MethodGet, repoSettingsAPIPath(mirror, "/settings"), nil, http.StatusOK, &settings)
	assert.Nil(t, settings.Mirror)

	// Regular repositories cannot be converted.
	status, body = c.do(http.MethodPost, path, map[string]any{"repoName": mirror.Name})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestRepoSettingsFork(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	u := seedUser(t, CreateUserOptions{})
	outsider := seedUser(t, CreateUserOptions{})
	base := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	baseID := strconv.FormatInt(base.ID, 10)
	c := authenticatedClient(t, u)

	var options struct {
		Base struct {
			Owner     string `json:"owner"`
			Name      string `json:"name"`
			IsPrivate bool   `json:"isPrivate"`
		} `json:"base"`
		Owners []struct {
			Name string `json:"name"`
		} `json:"owners"`
	}
	c.doJSON(http.MethodGet, "/api/web/repo/fork/"+baseID, nil, http.StatusOK, &options)
	assert.Equal(t, owner.Name, options.Base.Owner)
	assert.Equal(t, base.Name, options.Base.Name)
	assert.False(t, options.Base.IsPrivate)
	require.Len(t, options.Owners, 1)
	assert.Equal(t, u.Name, options.Owners[0].Name)

	// Repositories the viewer cannot read are not found.
	outsiderClient := authenticatedClient(t, outsider)
	privateID := strconv.FormatInt(private.ID, 10)
	status, body := outsiderClient.do(http.MethodGet, "/api/web/repo/fork/"+privateID, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = outsiderClient.do(http.MethodPost, "/api/web/repo/fork/"+privateID, map[string]any{"name": UniqueName("fork-")})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	forkName := UniqueName("fork-")
	var created repoSettingsCreated
	c.doJSON(http.MethodPost, "/api/web/repo/fork/"+baseID, map[string]any{"name": forkName, "description": "My fork"}, http.StatusCreated, &created)
	assert.Equal(t, repoSettingsCreated{Owner: u.Name, Name: forkName}, created)

	fork, err := database.Handle.Repositories().GetForkBy(t.Context(), base.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, forkName, fork.Name)
	assert.True(t, fork.IsFork)
	assert.Equal(t, "My fork", fork.Description)
	branch := repoSettingsDefaultBranch(t, base)
	assert.Equal(t, repoSettingsHead(t, base, branch), repoSettingsHead(t, &Repository{Owner: u.Name, Name: forkName}, branch))
	gotBase, err := getRepository(t, base.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, gotBase.NumForks)

	// Forking again leads to the fork the viewer already has.
	created = repoSettingsCreated{}
	c.doJSON(http.MethodPost, "/api/web/repo/fork/"+baseID, map[string]any{"name": UniqueName("fork-")}, http.StatusOK, &created)
	assert.Equal(t, repoSettingsCreated{Owner: u.Name, Name: forkName}, created)

	status, body = authenticatedClient(t, owner).do(http.MethodPost, "/api/web/repo/fork/"+baseID, map[string]any{"name": UniqueName("fork-")})
	require.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "ownerID"))
}
