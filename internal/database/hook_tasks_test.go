package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

func TestHookTasks(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &HookTasksStore{
		db: newTestDB(t, "HookTasksStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *HookTasksStore)
	}{
		{"ListByWebhook", hookTasksListByWebhook},
		{"GetByUUID", hookTasksGetByUUID},
		{"Update", hookTasksUpdate},
		{"listUndelivered", hookTasksListUndelivered},
		{"deliver", hookTasksDeliver},
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

func newTestHookTask(t *testing.T, s *HookTasksStore, repoID, hookID int64, uuid string) *HookTask {
	t.Helper()

	task := &HookTask{RepoID: repoID, HookID: hookID, UUID: uuid}
	err := s.db.Create(task).Error
	require.NoError(t, err)
	return task
}

func hookTasksListByWebhook(t *testing.T, ctx context.Context, s *HookTasksStore) {
	var ids []int64
	for i := range 5 {
		ids = append(ids, newTestHookTask(t, s, 1, 1, fmt.Sprintf("task-%d", i)).ID)
	}
	newTestHookTask(t, s, 1, 2, "other")

	taskIDs := func(tasks []*HookTask) []int64 {
		var ids []int64
		for _, task := range tasks {
			ids = append(ids, task.ID)
		}
		return ids
	}

	// Newest first, in pages.
	got, err := s.ListByWebhook(ctx, 1, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{ids[4], ids[3]}, taskIDs(got))
	got, err = s.ListByWebhook(ctx, 1, 3, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{ids[0]}, taskIDs(got))
	got, err = s.ListByWebhook(ctx, 1, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{ids[4], ids[3]}, taskIDs(got))
	// A page size of 0 lists all deliveries, as XORM did for `[webhook] PAGING_NUM = 0`.
	got, err = s.ListByWebhook(ctx, 1, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{ids[4], ids[3], ids[2], ids[1], ids[0]}, taskIDs(got))

	got, err = s.ListByWebhook(ctx, 3, 1, 2)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func hookTasksGetByUUID(t *testing.T, ctx context.Context, s *HookTasksStore) {
	task := newTestHookTask(t, s, 1, 1, "uuid-1")

	got, err := s.GetByUUID(ctx, 1, "uuid-1")
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)

	// Deliveries are only found through their own webhook.
	_, err = s.GetByUUID(ctx, 2, "uuid-1")
	assert.True(t, IsHookTaskNotExist(err), "got %v", err)
	_, err = s.GetByUUID(ctx, 1, "uuid-2")
	assert.True(t, IsHookTaskNotExist(err), "got %v", err)
}

func hookTasksUpdate(t *testing.T, ctx context.Context, s *HookTasksStore) {
	task := newTestHookTask(t, s, 1, 1, "uuid-1")

	task.IsDelivered = true
	task.IsSucceed = true
	task.RequestInfo = &HookRequest{Headers: map[string]string{"X-Gogs-Event": "push"}}
	task.ResponseInfo = &HookResponse{Status: 202, Body: "accepted"}
	err := s.Update(ctx, task)
	require.NoError(t, err)

	got, err := s.GetByUUID(ctx, 1, "uuid-1")
	require.NoError(t, err)
	assert.True(t, got.IsDelivered)
	assert.True(t, got.IsSucceed)
	require.NotNil(t, got.RequestInfo)
	assert.Equal(t, "push", got.RequestInfo.Headers["X-Gogs-Event"])
	require.NotNil(t, got.ResponseInfo)
	assert.Equal(t, 202, got.ResponseInfo.Status)
	assert.Equal(t, "accepted", got.ResponseInfo.Body)
}

func hookTasksListUndelivered(t *testing.T, ctx context.Context, s *HookTasksStore) {
	pending1 := newTestHookTask(t, s, 1, 1, "pending-1")
	pending2 := newTestHookTask(t, s, 2, 2, "pending-2")
	delivered := newTestHookTask(t, s, 1, 1, "delivered")
	delivered.IsDelivered = true
	require.NoError(t, s.Update(ctx, delivered))

	taskIDs := func(tasks []*HookTask) []int64 {
		var ids []int64
		for _, task := range tasks {
			ids = append(ids, task.ID)
		}
		return ids
	}

	got, err := s.listUndelivered(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{pending1.ID, pending2.ID}, taskIDs(got))

	got, err = s.listUndelivered(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{pending1.ID}, taskIDs(got))
}

func hookTasksDeliver(t *testing.T, ctx context.Context, s *HookTasksStore) {
	webhooks := &WebhooksStore{db: s.db}
	for _, tc := range []struct {
		url      string
		wantBody string
	}{
		{"://bad", "Cannot parse payload URL: "},
		{"http://127.0.0.1:1/hook", "Payload URL resolved to a local network address that is implicitly blocked."},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := newTestWebhook(t, ctx, webhooks, 1, 0, HookEvents{})
			task := newTestHookTask(t, s, 1, w.ID, "uuid-"+tc.url)
			// A redelivery of a delivery that succeeded before.
			task.URL = tc.url
			task.IsSucceed = true
			task.ResponseInfo = &HookResponse{Status: 200, Body: "received"}
			require.NoError(t, s.Update(ctx, task))
			require.NoError(t, webhooks.setLastStatus(ctx, w.ID, HookStatusSucceed))
			task, err := s.GetByUUID(ctx, w.ID, task.UUID)
			require.NoError(t, err)
			task.IsDelivered = false

			task.deliver(ctx, logx.NewNoopLogger(), webhooks)
			require.NoError(t, s.Update(ctx, task))

			got, err := s.GetByUUID(ctx, w.ID, task.UUID)
			require.NoError(t, err)
			assert.True(t, got.IsDelivered, "a failed attempt is not retried")
			assert.False(t, got.IsSucceed)
			assert.NotZero(t, got.Delivered)
			require.NotNil(t, got.ResponseInfo)
			assert.Zero(t, got.ResponseInfo.Status)
			assert.Contains(t, got.ResponseInfo.Body, tc.wantBody)
			undelivered, err := s.listUndelivered(ctx, 1)
			require.NoError(t, err)
			assert.Empty(t, undelivered)
			gotWebhook, err := webhooks.GetOfRepo(ctx, 1, w.ID)
			require.NoError(t, err)
			assert.Equal(t, HookStatus(HookStatusFailed), gotWebhook.LastStatus)
		})
	}
}
