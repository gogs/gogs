package integrationtests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/repox"
)

// adminClients returns clients authenticated as a new site admin and as a new
// user who is not one, and a client that is unauthenticated.
func adminClients(t *testing.T) (admin, nonAdmin, anonymous *client) {
	t.Helper()

	admin = authenticatedClient(t, seedUser(t, CreateUserOptions{Admin: true}))
	nonAdmin = authenticatedClient(t, seedUser(t, CreateUserOptions{}))
	return admin, nonAdmin, newClient(t)
}

// adminAssertTurnedAway asserts that the API route turns away an authenticated
// user who is not an admin and an unauthenticated visitor.
func adminAssertTurnedAway(t *testing.T, nonAdmin, anonymous *client, method, path string, body any) {
	t.Helper()

	status, respBody := nonAdmin.do(method, path, body)
	assert.Equal(t, http.StatusForbidden, status, "non-admin %s %s, response: %s", method, path, respBody)
	status, respBody = anonymous.do(method, path, body)
	assert.Equal(t, http.StatusUnauthorized, status, "anonymous %s %s, response: %s", method, path, respBody)
}

// adminNoBody is the body of requests that send no data. The web API only
// accepts requests that change something when they are sent as JSON.
var adminNoBody = map[string]any{}

// adminFieldErrors decodes the field errors of a response to an invalid
// request.
func adminFieldErrors(t *testing.T, body []byte) map[string]string {
	t.Helper()

	var resp struct {
		Fields map[string]string `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
	return resp.Fields
}

type adminReportRow struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	On    bool   `json:"on"`
}

type adminReportSection struct {
	ID     string             `json:"id"`
	Groups [][]adminReportRow `json:"groups"`
}

// adminReportValues returns the values of the rows of the section with the ID.
func adminReportValues(t *testing.T, sections []adminReportSection, id string) []string {
	t.Helper()

	for _, s := range sections {
		if s.ID != id {
			continue
		}
		var values []string
		for _, group := range s.Groups {
			for _, row := range group {
				values = append(values, row.Value)
			}
		}
		return values
	}
	t.Fatalf("no section %q", id)
	return nil
}

func TestAdminDashboard(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/dashboard", nil)

	var resp struct {
		AppVersion    string    `json:"appVersion"`
		GoVersion     string    `json:"goVersion"`
		StartedAt     time.Time `json:"startedAt"`
		UptimeSeconds int64     `json:"uptimeSeconds"`
		Statistics    []struct {
			Title string `json:"title"`
			Items []struct {
				Key   string `json:"key"`
				Label string `json:"label"`
				Count int64  `json:"count"`
			} `json:"items"`
		} `json:"statistics"`
		SystemStatus adminReportSection `json:"systemStatus"`
	}
	admin.doJSON(http.MethodGet, "/api/web/admin/dashboard", nil, http.StatusOK, &resp)
	// TestMain points the first argument to the binary of the server.
	version, err := exec.Command(os.Args[0], "--version").Output()
	require.NoError(t, err)
	assert.Equal(t, "Gogs version "+resp.AppVersion, strings.TrimSpace(string(version)))
	// The server is built by the same toolchain as the tests.
	assert.Equal(t, runtime.Version(), resp.GoVersion)
	assert.False(t, resp.StartedAt.IsZero())
	assert.True(t, resp.StartedAt.Before(time.Now()), "the server started at %s", resp.StartedAt)
	assert.InDelta(t, time.Since(resp.StartedAt).Seconds(), resp.UptimeSeconds, 5)
	assert.Equal(t, "system-status", resp.SystemStatus.ID)
	assert.NotEmpty(t, resp.SystemStatus.Groups)

	// Other tests add and delete data at the same time, so the counts are only
	// checked to be there, with at least the users of this test.
	require.NotEmpty(t, resp.Statistics)
	people := resp.Statistics[0]
	assert.Equal(t, "People", people.Title)
	require.NotEmpty(t, people.Items)
	assert.Equal(t, "users", people.Items[0].Key)
	assert.Equal(t, "Users", people.Items[0].Label)
	assert.GreaterOrEqual(t, people.Items[0].Count, int64(2))
	labels := make(map[string]string)
	for _, group := range resp.Statistics {
		for _, statistic := range group.Items {
			// A label without a translation would come back as its locale key.
			assert.NotContains(t, statistic.Label, "statistic_")
			labels[statistic.Key] = statistic.Label
		}
	}
	assert.Len(t, labels, 22)
	assert.Equal(t, "Organizations", labels["organizations"])
	assert.Equal(t, "Repositories", labels["repositories"])
	assert.Equal(t, "Login sources", labels["login_sources"])
}

// adminRunOperation runs the site-wide operation as the admin, after checking
// that it is turned away for others, and returns the message of its success.
func adminRunOperation(t *testing.T, admin, nonAdmin, anonymous *client, name string) string {
	t.Helper()

	path := "/api/web/admin/operations/" + name
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPost, path, adminNoBody)
	var resp struct {
		Message string `json:"message"`
	}
	admin.doJSON(http.MethodPost, path, adminNoBody, http.StatusOK, &resp)
	return resp.Message
}

// TestAdminOperations runs the operations that act on repositories of the
// whole site, which only fix up or tidy what other tests leave behind, and
// checks their effect on repositories seeded here. Deleting inactive users and
// resyncing SSH keys are left out.
//
// It does not run in parallel, like TestBackupRestore, because the operations
// act on every repository, including the ones other tests push to or download
// archives of at the same time. Go runs it before the parallel tests start.
func TestAdminOperations(t *testing.T) {
	admin, nonAdmin, anonymous := adminClients(t)
	owner := seedUser(t, CreateUserOptions{})

	t.Run("git gc", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		packs := filepath.Join(repox.RepositoryPath(repo.Owner, repo.Name), "objects", "pack", "*.pack")
		// The commit of a new repository is pushed as loose objects, which
		// garbage collection packs.
		before, err := filepath.Glob(packs)
		require.NoError(t, err)
		require.Empty(t, before, "repository already packed")

		msg := adminRunOperation(t, admin, nonAdmin, anonymous, "git-gc-repos")
		assert.Equal(t, "Collected garbage in all repositories.", msg)
		after, err := filepath.Glob(packs)
		require.NoError(t, err)
		assert.NotEmpty(t, after, "repository not packed")
	})

	t.Run("resync hooks", func(t *testing.T) {
		// A repository gets the delegate hooks it needs, e.g., pre-receive for a
		// protected branch, and loses the others.
		unprotected := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		protected := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		require.NoError(t, database.Handle.ProtectBranches().Save(t.Context(), &database.ProtectBranch{RepoID: protected.ID, Name: "main", Protected: true}))
		hook := func(repo *Repository) string {
			return filepath.Join(repox.RepositoryPath(repo.Owner, repo.Name), "hooks", "pre-receive")
		}
		for _, repo := range []*Repository{unprotected, protected} {
			require.NoError(t, os.WriteFile(hook(repo), []byte("#!/bin/sh\nexit 1\n"), 0o755))
		}

		msg := adminRunOperation(t, admin, nonAdmin, anonymous, "resync-repo-hooks")
		assert.Equal(t, "Resynced the hooks of all repositories.", msg)
		assert.NoFileExists(t, hook(unprotected))
		content, err := os.ReadFile(hook(protected))
		require.NoError(t, err)
		// TestMain points the first argument to the binary of the server.
		assert.Contains(t, string(content), `"`+os.Args[0]+`" hook`)
		assert.Contains(t, string(content), "pre-receive")
	})

	t.Run("delete archives", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		repoPath := repox.RepositoryPath(repo.Owner, repo.Name)
		archive := filepath.Join(repoPath, "archives", "zip", "main.zip")
		require.NoError(t, os.MkdirAll(filepath.Dir(archive), 0o755))
		require.NoError(t, os.WriteFile(archive, []byte("archive"), 0o644))

		msg := adminRunOperation(t, admin, nonAdmin, anonymous, "delete-repo-archives")
		assert.Equal(t, "Deleted the archives of all repositories.", msg)
		_, err := os.Stat(filepath.Join(repoPath, "archives"))
		assert.True(t, os.IsNotExist(err), "archives not deleted: %v", err)
		_, err = os.Stat(filepath.Join(repoPath, "HEAD"))
		assert.NoError(t, err, "repository deleted with its archives")
	})

	t.Run("reinit missing", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		repoPath := repox.RepositoryPath(repo.Owner, repo.Name)
		require.NoError(t, os.RemoveAll(repoPath))

		msg := adminRunOperation(t, admin, nonAdmin, anonymous, "reinit-missing-repos")
		assert.Equal(t, "Reinitialized the repositories that lost their Git files.", msg)
		_, err := os.Stat(filepath.Join(repoPath, "HEAD"))
		assert.NoError(t, err, "repository not reinitialized")
		_, err = getRepository(t, repo.ID)
		assert.NoError(t, err)
	})

	t.Run("delete missing", func(t *testing.T) {
		kept := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		missing := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		require.NoError(t, os.RemoveAll(repox.RepositoryPath(missing.Owner, missing.Name)))

		msg := adminRunOperation(t, admin, nonAdmin, anonymous, "delete-missing-repos")
		assert.Equal(t, "Deleted the records of repositories that lost their Git files.", msg)
		_, err := getRepository(t, missing.ID)
		assert.True(t, database.IsErrRepoNotExist(err), "repository not deleted: %v", err)
		_, err = getRepository(t, kept.ID)
		assert.NoError(t, err)
	})

	status, body := admin.do(http.MethodPost, "/api/web/admin/operations/no-such-operation", adminNoBody)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestAdminConfig(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/config", nil)

	var resp struct {
		Sections     []adminReportSection `json:"sections"`
		EmailEnabled bool                 `json:"emailEnabled"`
	}
	status, body := admin.do(http.MethodGet, "/api/web/admin/config", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	require.NoError(t, json.Unmarshal(body, &resp))

	// The expected values are the ones writeConfig sets.
	u, err := url.Parse(serverURL)
	require.NoError(t, err)
	assert.True(t, resp.EmailEnabled)
	server := adminReportValues(t, resp.Sections, "server")
	assert.Contains(t, server, serverURL+"/")
	assert.Contains(t, server, u.Port())
	assert.Contains(t, adminReportValues(t, resp.Sections, "email"), "127.0.0.1:1")
	assert.Contains(t, adminReportValues(t, resp.Sections, "session"), "file")
	assert.Contains(t, adminReportValues(t, resp.Sections, "repository"), conf.Repository.Root)
	assert.NotContains(t, string(body), conf.Security.SecretKey, "secret key disclosed")
}

func TestAdminTestMail(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	path := "/api/web/admin/config/test-mail"
	address := UniqueName("mail-") + "@example.com"
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodPost, path, map[string]any{"email": address})

	// The mail server of the tests refuses connections, which the admin is
	// told about to fix the settings.
	var resp struct {
		Error string `json:"error"`
	}
	admin.doJSON(http.MethodPost, path, map[string]any{"email": address}, http.StatusBadGateway, &resp)
	assert.Contains(t, resp.Error, "Failed to send test email to '"+address+"'")

	status, body := admin.do(http.MethodPost, path, map[string]any{"email": "not an email"})
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
}

func TestAdminMonitor(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/monitor", nil)

	var resp struct {
		CronTasks []struct {
			Description string `json:"description"`
			Schedule    string `json:"schedule"`
		} `json:"cronTasks"`
	}
	admin.doJSON(http.MethodGet, "/api/web/admin/monitor", nil, http.StatusOK, &resp)
	schedules := make(map[string]string)
	for _, task := range resp.CronTasks {
		schedules[task.Description] = task.Schedule
	}
	// The default schedules of conf/app.ini, which the tests do not change.
	assert.Equal(t, map[string]string{
		"Update mirrors":              "@every 10m",
		"Repository health check":     "@every 24h",
		"Check repository statistics": "@every 24h",
		"Repository archive cleanup":  "@every 24h",
	}, schedules)
}

// adminSeedOrganization creates an organization owned by the user.
func adminSeedOrganization(t *testing.T, ownerID int64) *database.User {
	t.Helper()

	owner, err := database.Handle.Users().GetByID(t.Context(), ownerID)
	require.NoError(t, err)
	org := &database.User{
		Name:     UniqueName("org-"),
		IsActive: true,
		Type:     database.UserTypeOrganization,
	}
	require.NoError(t, database.Handle.Organizations().Create(t.Context(), org, owner))
	return org
}

func TestAdminOrgs(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	owner := seedUser(t, CreateUserOptions{})
	org := adminSeedOrganization(t, owner.ID)
	seedRepository(t, org.ID, CreateRepositoryOptions{})

	path := "/api/web/admin/orgs?q=" + url.QueryEscape(org.Name)
	adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, path, nil)

	type item struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		TeamCount   int    `json:"teamCount"`
		MemberCount int    `json:"memberCount"`
		RepoCount   int    `json:"repoCount"`
	}
	var resp struct {
		Items []item `json:"items"`
		Total int64  `json:"total"`
	}
	admin.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
	// A new organization has the owners team with its creator in it.
	assert.Equal(t, []item{{ID: org.ID, Name: org.Name, TeamCount: 1, MemberCount: 1, RepoCount: 1}}, resp.Items)
	assert.Equal(t, int64(1), resp.Total)

	// Users of the same name are not organizations.
	resp.Items = nil
	admin.doJSON(http.MethodGet, "/api/web/admin/orgs?q="+url.QueryEscape(owner.Name), nil, http.StatusOK, &resp)
	assert.Empty(t, resp.Items)
}

func TestAdminRepos(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true, AutoInit: true})
	repoPath := repox.RepositoryPath(repo.Owner, repo.Name)

	t.Run("list", func(t *testing.T) {
		path := "/api/web/admin/repos?q=" + url.QueryEscape(repo.Name)
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, path, nil)

		type item struct {
			ID        int64  `json:"id"`
			Owner     string `json:"owner"`
			Name      string `json:"name"`
			IsPrivate bool   `json:"isPrivate"`
		}
		var resp struct {
			Items []item `json:"items"`
			Total int64  `json:"total"`
		}
		// Admins see private repositories of everyone.
		admin.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		assert.Equal(t, []item{{ID: repo.ID, Owner: repo.Owner, Name: repo.Name, IsPrivate: true}}, resp.Items)
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("delete", func(t *testing.T) {
		path := "/api/web/admin/repos/" + strconv.FormatInt(repo.ID, 10)
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodDelete, path, adminNoBody)
		_, err := getRepository(t, repo.ID)
		require.NoError(t, err, "repository deleted by a request that was turned away")

		status, body := admin.do(http.MethodDelete, path, adminNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err = getRepository(t, repo.ID)
		assert.True(t, database.IsErrRepoNotExist(err), "repository not deleted: %v", err)
		_, err = os.Stat(repoPath)
		assert.True(t, os.IsNotExist(err), "Git directory not deleted: %v", err)

		status, body = admin.do(http.MethodDelete, path, adminNoBody)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

// adminNoticeIDs returns the IDs of all system notices whose description has
// the prefix.
func adminNoticeIDs(t *testing.T, prefix string) []int64 {
	t.Helper()

	notices, err := database.Handle.Notices().List(t.Context(), 1, 1<<20)
	require.NoError(t, err)
	var ids []int64
	for _, n := range notices {
		if strings.HasPrefix(n.Description, prefix) {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

func TestAdminNotices(t *testing.T) {
	t.Parallel()

	admin, nonAdmin, anonymous := adminClients(t)
	prefix := UniqueName("notice-")
	for _, suffix := range []string{"-first", "-second"} {
		require.NoError(t, database.Handle.Notices().Create(t.Context(), database.NoticeTypeRepository, prefix+suffix))
	}
	ids := adminNoticeIDs(t, prefix)
	require.Len(t, ids, 2)
	// Notices are listed newest first.
	second, first := ids[0], ids[1]

	t.Run("list", func(t *testing.T) {
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodGet, "/api/web/admin/notices", nil)

		type notice struct {
			ID          int64  `json:"id"`
			Type        string `json:"type"`
			Description string `json:"description"`
		}
		// Other tests may add notices at the same time, which pushes these to
		// later pages.
		var seeded []notice
		for page := 1; len(seeded) < 2; page++ {
			var resp struct {
				Items []notice `json:"items"`
			}
			admin.doJSON(http.MethodGet, "/api/web/admin/notices?page="+strconv.Itoa(page), nil, http.StatusOK, &resp)
			for _, n := range resp.Items {
				if strings.HasPrefix(n.Description, prefix) {
					seeded = append(seeded, n)
				}
			}
			if len(resp.Items) == 0 {
				break
			}
		}
		assert.Equal(t, []notice{
			{ID: second, Type: "Repository", Description: prefix + "-second"},
			{ID: first, Type: "Repository", Description: prefix + "-first"},
		}, seeded)
	})

	t.Run("delete selected", func(t *testing.T) {
		body := map[string]any{"ids": []int64{first}}
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodDelete, "/api/web/admin/notices", body)
		require.Equal(t, []int64{second, first}, adminNoticeIDs(t, prefix), "deleted by a request that was turned away")

		status, respBody := admin.do(http.MethodDelete, "/api/web/admin/notices", map[string]any{"ids": []int64{}})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", respBody)

		status, respBody = admin.do(http.MethodDelete, "/api/web/admin/notices", body)
		require.Equal(t, http.StatusNoContent, status, "response: %s", respBody)
		assert.Equal(t, []int64{second}, adminNoticeIDs(t, prefix))
	})

	t.Run("delete all", func(t *testing.T) {
		require.NoError(t, database.Handle.Notices().Create(t.Context(), database.NoticeTypeRepository, prefix+"-third"))
		adminAssertTurnedAway(t, nonAdmin, anonymous, http.MethodDelete, "/api/web/admin/notices/all", adminNoBody)
		require.NotEmpty(t, adminNoticeIDs(t, prefix), "deleted by a request that was turned away")

		// Deleting all notices also deletes the ones of other tests, so only
		// the ones of this test are checked to be gone.
		status, body := admin.do(http.MethodDelete, "/api/web/admin/notices/all", adminNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		assert.Empty(t, adminNoticeIDs(t, prefix))
	})
}
