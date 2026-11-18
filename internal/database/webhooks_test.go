package database

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

func TestWebhooks(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &WebhooksStore{
		db: newTestDB(t, "WebhooksStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *WebhooksStore)
	}{
		{"Get", webhooksGet},
		{"List", webhooksList},
		{"Update", webhooksUpdate},
		{"Delete", webhooksDelete},
		{"Prepare", webhooksPrepare},
		{"Prepare to Discord without settings", webhooksPrepareDiscordWithoutSettings},
		{"Test", webhooksTest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

// newTestWebhook creates a webhook of given repository or organization that is
// subscribed to given events, or only to pushes when there are none.
func newTestWebhook(t *testing.T, ctx context.Context, s *WebhooksStore, repoID, orgID int64, events HookEvents) *Webhook {
	t.Helper()

	w := &Webhook{
		RepoID:       repoID,
		OrgID:        orgID,
		URL:          "https://example.com/hook",
		ContentType:  JSON,
		IsActive:     true,
		HookTaskType: GOGS,
		HookEvent:    &HookEvent{PushOnly: true},
	}
	if events != (HookEvents{}) {
		w.HookEvent = &HookEvent{ChooseEvents: true, HookEvents: events}
	}
	require.NoError(t, w.UpdateEvent())
	require.NoError(t, s.Create(ctx, w))
	return w
}

func webhooksGet(t *testing.T, ctx context.Context, s *WebhooksStore) {
	repoHook := newTestWebhook(t, ctx, s, 1, 0, HookEvents{})
	orgHook := newTestWebhook(t, ctx, s, 0, 2, HookEvents{Issues: true})

	got, err := s.GetOfRepo(ctx, 1, repoHook.ID)
	require.NoError(t, err)
	assert.Equal(t, repoHook.URL, got.URL)
	assert.True(t, got.HasPushEvent())
	assert.False(t, got.HasIssuesEvent())

	got, err = s.GetOfOrg(ctx, 2, orgHook.ID)
	require.NoError(t, err)
	assert.True(t, got.HasIssuesEvent())
	assert.False(t, got.HasPushEvent())

	// Webhooks are only found through their own repository or organization.
	for _, get := range []func() (*Webhook, error){
		func() (*Webhook, error) { return s.GetOfRepo(ctx, 2, repoHook.ID) },
		func() (*Webhook, error) { return s.GetOfRepo(ctx, 1, orgHook.ID) },
		func() (*Webhook, error) { return s.GetOfOrg(ctx, 1, repoHook.ID) },
		func() (*Webhook, error) { return s.GetOfOrg(ctx, 2, 404) },
		func() (*Webhook, error) { return s.GetOfRepo(ctx, 1, 0) },
		func() (*Webhook, error) { return s.GetOfOrg(ctx, 2, 0) },
	} {
		_, err = get()
		assert.True(t, IsErrWebhookNotExist(err), "got %v", err)
	}
}

func webhooksList(t *testing.T, ctx context.Context, s *WebhooksStore) {
	hook1 := newTestWebhook(t, ctx, s, 1, 0, HookEvents{})
	hook2 := newTestWebhook(t, ctx, s, 1, 0, HookEvents{})
	newTestWebhook(t, ctx, s, 2, 0, HookEvents{})
	orgHook := newTestWebhook(t, ctx, s, 0, 3, HookEvents{})

	ids := func(webhooks []*Webhook) []int64 {
		var ids []int64
		for _, w := range webhooks {
			ids = append(ids, w.ID)
		}
		return ids
	}

	got, err := s.ListByRepo(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{hook1.ID, hook2.ID}, ids(got))

	got, err = s.ListByOrg(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, []int64{orgHook.ID}, ids(got))

	got, err = s.ListByOrg(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func webhooksUpdate(t *testing.T, ctx context.Context, s *WebhooksStore) {
	w := newTestWebhook(t, ctx, s, 1, 0, HookEvents{})

	w.URL = "https://example.com/new"
	w.IsActive = false
	w.HookEvent = &HookEvent{SendEverything: true}
	require.NoError(t, w.UpdateEvent())
	err := s.Update(ctx, w)
	require.NoError(t, err)

	got, err := s.GetOfRepo(ctx, 1, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/new", got.URL)
	assert.False(t, got.IsActive)
	assert.True(t, got.HasReleaseEvent())
}

func webhooksDelete(t *testing.T, ctx context.Context, s *WebhooksStore) {
	hook1 := newTestWebhook(t, ctx, s, 1, 0, HookEvents{})
	hook2 := newTestWebhook(t, ctx, s, 2, 0, HookEvents{})
	orgHook := newTestWebhook(t, ctx, s, 0, 3, HookEvents{})
	for _, w := range []*Webhook{hook1, hook2, orgHook} {
		err := s.db.Create(&HookTask{RepoID: w.RepoID, HookID: w.ID, UUID: "task-" + w.URL}).Error
		require.NoError(t, err)
	}
	countTasks := func(hookID int64) int64 {
		var count int64
		err := s.db.Model(new(HookTask)).Where("hook_id = ?", hookID).Count(&count).Error
		require.NoError(t, err)
		return count
	}

	// Webhooks of other repositories are neither deleted nor lose their
	// deliveries, and an ID of 0 deletes nothing.
	err := s.DeleteOfRepo(ctx, 1, hook2.ID)
	require.NoError(t, err)
	_, err = s.GetOfRepo(ctx, 2, hook2.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countTasks(hook2.ID))
	require.NoError(t, s.DeleteOfRepo(ctx, 2, 0))
	require.NoError(t, s.DeleteOfOrg(ctx, 3, 0))
	_, err = s.GetOfRepo(ctx, 2, hook2.ID)
	require.NoError(t, err)
	_, err = s.GetOfOrg(ctx, 3, orgHook.ID)
	require.NoError(t, err)

	err = s.DeleteOfRepo(ctx, 1, hook1.ID)
	require.NoError(t, err)
	_, err = s.GetOfRepo(ctx, 1, hook1.ID)
	assert.True(t, IsErrWebhookNotExist(err), "got %v", err)
	assert.Zero(t, countTasks(hook1.ID))

	err = s.DeleteOfOrg(ctx, 1, orgHook.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countTasks(orgHook.ID))
	err = s.DeleteOfOrg(ctx, 3, orgHook.ID)
	require.NoError(t, err)
	_, err = s.GetOfOrg(ctx, 3, orgHook.ID)
	assert.True(t, IsErrWebhookNotExist(err), "got %v", err)
	assert.Zero(t, countTasks(orgHook.ID))

	// Deleting a webhook that does not exist is not an error.
	err = s.DeleteOfRepo(ctx, 1, hook1.ID)
	require.NoError(t, err)
}

// newTestOrgRepo creates an organization and a repository it owns.
func newTestOrgRepo(t *testing.T, ctx context.Context, s *WebhooksStore) (org *User, repo *Repository) {
	t.Helper()

	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org, err := newUsersStore(s.db).Create(ctx, "wh-org", "wh-org@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(dbx.Quote("UPDATE %s SET type = ? WHERE id = ?", "user"), UserTypeOrganization, org.ID).Error
	require.NoError(t, err)
	repo, err = newReposStore(s.db).Create(ctx, org.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	return org, repo
}

func listTestHookTasks(t *testing.T, s *WebhooksStore) []*HookTask {
	t.Helper()

	var tasks []*HookTask
	err := s.db.Order("id ASC").Find(&tasks).Error
	require.NoError(t, err)
	return tasks
}

func webhooksPrepare(t *testing.T, ctx context.Context, s *WebhooksStore) {
	logger := logx.NewNoopLogger()
	org, repo := newTestOrgRepo(t, ctx, s)

	pushHook := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{})
	pushHook.Secret = "s3cret"
	require.NoError(t, s.Update(ctx, pushHook))
	newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{Issues: true})
	inactive := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{})
	inactive.IsActive = false
	require.NoError(t, s.Update(ctx, inactive))
	newTestWebhook(t, ctx, s, 9999, 0, HookEvents{})
	orgHook := newTestWebhook(t, ctx, s, 0, org.ID, HookEvents{Push: true})
	newTestWebhook(t, ctx, s, 0, 9999, HookEvents{})

	payload := &apiv1types.WebhookPushPayload{Ref: "refs/heads/main"}
	err := s.Prepare(ctx, logger, repo, HookEventTypePush, payload)
	require.NoError(t, err)

	// Only active webhooks of the repository and its organization that are
	// subscribed to pushes get a delivery.
	tasks := listTestHookTasks(t, s)
	require.Len(t, tasks, 2)
	assert.Equal(t, pushHook.ID, tasks[0].HookID)
	assert.Equal(t, orgHook.ID, tasks[1].HookID)

	data, err := payload.JSONPayload()
	require.NoError(t, err)
	mac := hmac.New(sha256.New, []byte("s3cret"))
	_, _ = mac.Write(data)
	task := tasks[0]
	assert.Equal(t, repo.ID, task.RepoID)
	assert.Equal(t, HookEventTypePush, task.EventType)
	assert.Equal(t, string(data), task.PayloadContent)
	assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), task.Signature)
	assert.NotEmpty(t, task.UUID)
	assert.NotEqual(t, task.UUID, tasks[1].UUID)
	assert.False(t, task.IsDelivered)
	assert.Empty(t, tasks[1].Signature)

	// Webhooks of organizations are left out for repositories of users.
	userRepo, err := newReposStore(s.db).Create(ctx, 404, CreateRepoOptions{Name: "repo2"})
	require.NoError(t, err)
	userRepo.Owner = &User{ID: 404}
	newTestWebhook(t, ctx, s, userRepo.ID, 0, HookEvents{})
	newTestWebhook(t, ctx, s, 0, 404, HookEvents{})
	err = s.Prepare(ctx, logger, userRepo, HookEventTypePush, payload)
	require.NoError(t, err)
	tasks = listTestHookTasks(t, s)
	require.Len(t, tasks, 3)
	assert.Equal(t, userRepo.ID, tasks[2].RepoID)
}

func webhooksTest(t *testing.T, ctx context.Context, s *WebhooksStore) {
	logger := logx.NewNoopLogger()
	_, repo := newTestOrgRepo(t, ctx, s)

	inactive := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{})
	inactive.IsActive = false
	require.NoError(t, s.Update(ctx, inactive))
	issuesHook := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{Issues: true})
	otherHook := newTestWebhook(t, ctx, s, 9999, 0, HookEvents{})
	payload := &apiv1types.WebhookPushPayload{Ref: "refs/heads/main"}

	// Inactive webhooks can be tested.
	err := s.Test(ctx, logger, repo, HookEventTypePush, payload, inactive.ID)
	require.NoError(t, err)
	tasks := listTestHookTasks(t, s)
	require.Len(t, tasks, 1)
	assert.Equal(t, inactive.ID, tasks[0].HookID)

	// Webhooks not subscribed to the event get no delivery.
	err = s.Test(ctx, logger, repo, HookEventTypePush, payload, issuesHook.ID)
	require.NoError(t, err)
	assert.Len(t, listTestHookTasks(t, s), 1)

	err = s.Test(ctx, logger, repo, HookEventTypePush, payload, otherHook.ID)
	assert.True(t, IsErrWebhookNotExist(err), "got %v", err)
	assert.Len(t, listTestHookTasks(t, s), 1)
}

func webhooksPrepareDiscordWithoutSettings(t *testing.T, ctx context.Context, s *WebhooksStore) {
	// Discord payloads have links, which read the server settings that other
	// suites mock.
	conf.SetMockServer(t, conf.ServerOpts{})
	_, repo := newTestOrgRepo(t, ctx, s)
	// API v1 creates Discord webhooks without settings.
	discord := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{})
	discord.HookTaskType = DISCORD
	discord.Meta = ""
	require.NoError(t, s.Update(ctx, discord))
	after := newTestWebhook(t, ctx, s, repo.ID, 0, HookEvents{})

	payload := &apiv1types.WebhookPushPayload{
		Ref:    "refs/heads/main",
		Repo:   &apiv1types.Repository{Name: "repo", HTMLURL: "https://example.com/org/repo"},
		Sender: &apiv1types.User{UserName: "alice", AvatarURL: "https://example.com/alice.png"},
	}
	err := s.Prepare(ctx, logx.NewNoopLogger(), repo, HookEventTypePush, payload)
	require.NoError(t, err)

	tasks := listTestHookTasks(t, s)
	require.Len(t, tasks, 2)
	assert.Equal(t, discord.ID, tasks[0].HookID)
	assert.Contains(t, tasks[0].PayloadContent, "Pushed 0 new commits")
	assert.NotContains(t, tasks[0].PayloadContent, `"username"`)
	assert.NotContains(t, tasks[0].PayloadContent, `"avatar_url"`)
	assert.Equal(t, after.ID, tasks[1].HookID)
}
