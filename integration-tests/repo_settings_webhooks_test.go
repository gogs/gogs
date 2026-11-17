package integrationtests

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// repoSettingsDelivery is a request a webhook receiver got.
type repoSettingsDelivery struct {
	Header http.Header
	Body   []byte
}

// repoSettingsWebhookReceiver starts a server that answers every request with
// 200 and passes it on to the channel.
func repoSettingsWebhookReceiver(t *testing.T) (url string, deliveries <-chan repoSettingsDelivery) {
	t.Helper()

	ch := make(chan repoSettingsDelivery, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ch <- repoSettingsDelivery{Header: r.Header.Clone(), Body: body}
		_, _ = w.Write([]byte("received"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/hook", ch
}

func repoSettingsNextDelivery(t *testing.T, deliveries <-chan repoSettingsDelivery) repoSettingsDelivery {
	t.Helper()

	select {
	case d := <-deliveries:
		return d
	case <-time.After(30 * time.Second):
		t.Fatal("no webhook delivered within 30 seconds")
		return repoSettingsDelivery{}
	}
}

// repoSettingsCreateWebhook adds a Gogs webhook that sends push events as JSON
// to the URL, and returns its ID.
func repoSettingsCreateWebhook(t *testing.T, c *client, repo *Repository, url, secret string) int64 {
	t.Helper()

	var created struct {
		ID int64 `json:"id"`
	}
	c.doJSON(http.MethodPost, repoSettingsAPIPath(repo, "/settings/hooks"), map[string]any{
		"type":        "gogs",
		"url":         url,
		"contentType": "json",
		"secret":      secret,
		"events":      "push_only",
		"active":      true,
	}, http.StatusCreated, &created)
	require.NotZero(t, created.ID)
	return created.ID
}

type repoSettingsWebhookDetail struct {
	ID       int64 `json:"id"`
	Settings struct {
		Type         string          `json:"type"`
		URL          string          `json:"url"`
		ContentType  string          `json:"contentType"`
		Secret       string          `json:"secret"`
		Events       string          `json:"events"`
		ChosenEvents map[string]bool `json:"chosenEvents"`
		Active       bool            `json:"active"`
	} `json:"settings"`
	Deliveries []struct {
		UUID      string `json:"uuid"`
		Succeeded bool   `json:"succeeded"`
		Payload   string `json:"payload"`
		Response  *struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		} `json:"response"`
	} `json:"deliveries"`
}

func TestRepoSettingsWebhooks(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	branch := repoSettingsDefaultBranch(t, repo)
	c := repoSettingsSignedIn(t, owner)
	url, deliveries := repoSettingsWebhookReceiver(t)
	hooksPath := repoSettingsAPIPath(repo, "/settings/hooks")

	status, body := repoSettingsSignedIn(t, other).do(http.MethodPost, hooksPath, map[string]any{
		"type": "gogs", "url": url, "events": "push_only", "active": true,
	})
	require.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = c.do(http.MethodPost, hooksPath, map[string]any{"type": "gogs", "url": "not a URL", "events": "push_only"})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.NotEmpty(t, repoSettingsFieldError(t, body, "url"))

	id := repoSettingsCreateWebhook(t, c, repo, url, "first secret")
	hookPath := hooksPath + "/" + strconv.FormatInt(id, 10)

	var list struct {
		Items []struct {
			ID         int64  `json:"id"`
			Type       string `json:"type"`
			URL        string `json:"url"`
			LastStatus string `json:"lastStatus"`
			Active     bool   `json:"active"`
		} `json:"items"`
	}
	c.doJSON(http.MethodGet, hooksPath, nil, http.StatusOK, &list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, id, list.Items[0].ID)
	assert.Equal(t, "gogs", list.Items[0].Type)
	assert.Equal(t, url, list.Items[0].URL)
	assert.Equal(t, "none", list.Items[0].LastStatus)
	assert.True(t, list.Items[0].Active)

	var detail repoSettingsWebhookDetail
	c.doJSON(http.MethodGet, hookPath, nil, http.StatusOK, &detail)
	assert.Equal(t, id, detail.ID)
	assert.Equal(t, "json", detail.Settings.ContentType)
	assert.Equal(t, "first secret", detail.Settings.Secret)
	assert.Equal(t, "push_only", detail.Settings.Events)
	assert.Empty(t, detail.Deliveries)

	// Webhooks of other repositories are not found.
	status, body = c.do(http.MethodGet, repoSettingsAPIPath(otherRepo, "/settings/hooks/"+strconv.FormatInt(id, 10)), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	const secret = "second secret"
	status, body = c.do(http.MethodPatch, hookPath, map[string]any{
		"url":          url,
		"contentType":  "json",
		"secret":       secret,
		"events":       "choose_events",
		"chosenEvents": map[string]bool{"push": true, "issues": true},
		"active":       true,
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	detail = repoSettingsWebhookDetail{}
	c.doJSON(http.MethodGet, hookPath, nil, http.StatusOK, &detail)
	assert.Equal(t, secret, detail.Settings.Secret)
	assert.Equal(t, "choose_events", detail.Settings.Events)
	assert.True(t, detail.Settings.ChosenEvents["push"])
	assert.True(t, detail.Settings.ChosenEvents["issues"])
	assert.False(t, detail.Settings.ChosenEvents["release"])
	stored, err := database.Handle.Webhooks().GetOfRepo(t.Context(), repo.ID, id)
	require.NoError(t, err)
	assert.Equal(t, secret, stored.Secret)
	assert.True(t, stored.ChooseEvents)
	assert.True(t, stored.Issues)

	t.Run("test delivery", func(t *testing.T) {
		status, body := c.do(http.MethodPost, hookPath+"/test", repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		d := repoSettingsNextDelivery(t, deliveries)
		assert.Equal(t, "push", d.Header.Get("X-Gogs-Event"))
		assert.Equal(t, "application/json", d.Header.Get("Content-Type"))
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(d.Body)
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), d.Header.Get("X-Gogs-Signature"))

		var payload struct {
			Ref   string `json:"ref"`
			After string `json:"after"`
			Repo  struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			Pusher struct {
				UserName string `json:"username"`
			} `json:"pusher"`
		}
		require.NoError(t, json.Unmarshal(d.Body, &payload), "payload: %s", d.Body)
		assert.Equal(t, "refs/heads/"+branch, payload.Ref)
		assert.Equal(t, repoSettingsHead(t, repo, branch), payload.After)
		assert.Equal(t, repo.Owner+"/"+repo.Name, payload.Repo.FullName)
		assert.Equal(t, owner.Name, payload.Pusher.UserName)

		uuid := d.Header.Get("X-Gogs-Delivery")
		require.NotEmpty(t, uuid)
		// The server records the delivery after the receiver answers.
		var delivered repoSettingsWebhookDetail
		require.Eventually(t, func() bool {
			var detail repoSettingsWebhookDetail
			status, body := c.do(http.MethodGet, hookPath, nil)
			if status != http.StatusOK || json.Unmarshal(body, &detail) != nil {
				return false
			}
			delivered = detail
			return len(detail.Deliveries) == 1 && detail.Deliveries[0].Response != nil
		}, 30*time.Second, 200*time.Millisecond, "delivery not recorded")
		assert.Equal(t, uuid, delivered.Deliveries[0].UUID)
		assert.True(t, delivered.Deliveries[0].Succeeded)
		assert.Equal(t, http.StatusOK, delivered.Deliveries[0].Response.Status)
		assert.Equal(t, "received", delivered.Deliveries[0].Response.Body)
		assert.JSONEq(t, string(d.Body), delivered.Deliveries[0].Payload)

		status, body = c.do(http.MethodPost, hookPath+"/deliveries/"+uuid+"/redeliver", repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		again := repoSettingsNextDelivery(t, deliveries)
		assert.Equal(t, uuid, again.Header.Get("X-Gogs-Delivery"))
		assert.Equal(t, string(d.Body), string(again.Body))

		status, body = c.do(http.MethodPost, hookPath+"/deliveries/"+UniqueName("no-such-delivery-")+"/redeliver", repoSettingsNoBody)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	status, body = c.do(http.MethodDelete, hookPath, repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = c.do(http.MethodGet, hookPath, nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	_, err = database.Handle.Webhooks().GetOfRepo(t.Context(), repo.ID, id)
	assert.True(t, database.IsErrWebhookNotExist(err), "webhook not deleted: %v", err)
}

func TestRepoSettingsTriggerTask(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	branch := repoSettingsDefaultBranch(t, repo)
	c := repoSettingsSignedIn(t, owner)
	url, deliveries := repoSettingsWebhookReceiver(t)
	id := repoSettingsCreateWebhook(t, c, repo, url, "")

	status, body := c.do(http.MethodPost, repoSettingsAPIPath(repo, "/settings/hooks/"+strconv.FormatInt(id, 10)+"/test"), repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	uuid := repoSettingsNextDelivery(t, deliveries).Header.Get("X-Gogs-Delivery")

	// Triggering the tasks of the repository delivers what is still pending,
	// which the delivery becomes again.
	var task *database.HookTask
	require.Eventually(t, func() bool {
		var err error
		task, err = database.Handle.HookTasks().GetByUUID(t.Context(), id, uuid)
		return err == nil && task.IsDelivered && task.Delivered > 0
	}, 30*time.Second, 200*time.Millisecond, "delivery not recorded")
	task.IsDelivered = false
	require.NoError(t, database.Handle.HookTasks().Update(t.Context(), task))

	u, err := database.Handle.Users().GetByID(t.Context(), owner.ID)
	require.NoError(t, err)
	sum := md5.Sum([]byte(u.Salt))
	trigger := func(secret string) (int, []byte) {
		status, _, body := newClient(t).doRaw(http.MethodGet, "/"+repo.Owner+"/"+repo.Name+"/tasks/trigger?branch="+branch+
			"&pusher="+strconv.FormatInt(owner.ID, 10)+"&secret="+secret, "", nil)
		return status, body
	}

	status, body = trigger(hex.EncodeToString(sum[:]) + "0")
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.Contains(t, string(body), "Invalid secret")

	status, body = trigger(hex.EncodeToString(sum[:]))
	require.Equal(t, http.StatusAccepted, status, "response: %s", body)
	assert.Equal(t, uuid, repoSettingsNextDelivery(t, deliveries).Header.Get("X-Gogs-Delivery"))
}

// A webhook is reachable only through the repository or organization it
// belongs to, even by the admin of another one.
func TestRepoSettingsWebhooksOfOthers(t *testing.T) {
	victim := seedUser(t, CreateUserOptions{})
	victimRepo := seedRepository(t, victim.ID, CreateRepositoryOptions{AutoInit: true})
	victimClient := repoSettingsSignedIn(t, victim)
	url, deliveries := repoSettingsWebhookReceiver(t)
	victimHookID := repoSettingsCreateWebhook(t, victimClient, victimRepo, url, "victim secret")
	status, body := victimClient.do(http.MethodPost, repoSettingsAPIPath(victimRepo, "/settings/hooks/"+strconv.FormatInt(victimHookID, 10)+"/test"), repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	uuid := repoSettingsNextDelivery(t, deliveries).Header.Get("X-Gogs-Delivery")
	require.Eventually(t, func() bool {
		task, err := database.Handle.HookTasks().GetByUUID(t.Context(), victimHookID, uuid)
		return err == nil && task.IsDelivered && task.Delivered > 0
	}, 30*time.Second, 200*time.Millisecond, "delivery not recorded")

	victimOrg := orgSeed(t, victim.ID)
	var victimOrgHook struct {
		ID int64 `json:"id"`
	}
	victimClient.doJSON(http.MethodPost, "/api/web/orgs/"+victimOrg.Name+"/settings/hooks", map[string]any{
		"type": "gogs", "url": url, "contentType": "json", "events": "push_only", "active": true,
	}, http.StatusCreated, &victimOrgHook)

	attacker := seedUser(t, CreateUserOptions{})
	attackerRepo := seedRepository(t, attacker.ID, CreateRepositoryOptions{AutoInit: true})
	attackerOrg := orgSeed(t, attacker.ID)
	c := repoSettingsSignedIn(t, attacker)
	api := apiV1Client(t, attacker)

	for _, id := range []int64{victimHookID, victimOrgHook.ID} {
		hookPath := repoSettingsAPIPath(attackerRepo, "/settings/hooks/"+strconv.FormatInt(id, 10))
		for _, req := range []struct {
			method string
			path   string
			body   any
		}{
			{http.MethodGet, hookPath, nil},
			{http.MethodPatch, hookPath, map[string]any{"url": "http://127.0.0.1:1/stolen", "contentType": "json", "events": "send_everything", "active": true}},
			{http.MethodDelete, hookPath, repoSettingsNoBody},
			{http.MethodPost, hookPath + "/test", repoSettingsNoBody},
			{http.MethodPost, hookPath + "/deliveries/" + uuid + "/redeliver", repoSettingsNoBody},
			{http.MethodPatch, apiV1RepoPath(attackerRepo, "/hooks/"+apiV1Itoa(id)), map[string]any{"config": map[string]string{"url": "http://127.0.0.1:1/stolen"}}},
		} {
			caller := c
			if strings.HasPrefix(req.path, "/api/v1/") {
				caller = api
			}
			status, body := caller.do(req.method, req.path, req.body)
			assert.Equal(t, http.StatusNotFound, status, "%s %s, response: %s", req.method, req.path, body)
		}
		// Deleting through the API answers the same whether or not the
		// webhook exists.
		status, body := api.do(http.MethodDelete, apiV1RepoPath(attackerRepo, "/hooks/"+apiV1Itoa(id)), nil)
		assert.Equal(t, http.StatusNoContent, status, "response: %s", body)

		orgHookPath := "/api/web/orgs/" + attackerOrg.Name + "/settings/hooks/" + strconv.FormatInt(id, 10)
		status, body = c.do(http.MethodGet, orgHookPath, nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = c.do(http.MethodPatch, orgHookPath, map[string]any{"url": "http://127.0.0.1:1/stolen", "contentType": "json", "events": "send_everything", "active": true})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = c.do(http.MethodDelete, orgHookPath, orgNoBody)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	}

	hook, err := database.Handle.Webhooks().GetOfRepo(t.Context(), victimRepo.ID, victimHookID)
	require.NoError(t, err)
	assert.Equal(t, url, hook.URL)
	assert.Equal(t, "victim secret", hook.Secret)
	assert.True(t, hook.PushOnly)
	orgHook, err := database.Handle.Webhooks().GetOfOrg(t.Context(), victimOrg.ID, victimOrgHook.ID)
	require.NoError(t, err)
	assert.Equal(t, url, orgHook.URL)
	task, err := database.Handle.HookTasks().GetByUUID(t.Context(), victimHookID, uuid)
	require.NoError(t, err)
	assert.True(t, task.IsDelivered, "delivery queued again")
	select {
	case d := <-deliveries:
		t.Fatalf("unexpected delivery %q", d.Header.Get("X-Gogs-Delivery"))
	default:
	}
}

// A redelivery that gets no response shows as failed, although the delivery
// succeeded before.
func TestRepoSettingsWebhookFailedRedelivery(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := repoSettingsSignedIn(t, owner)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("received"))
	}))
	t.Cleanup(srv.Close)
	id := repoSettingsCreateWebhook(t, c, repo, srv.URL+"/hook", "")
	hooksPath := repoSettingsAPIPath(repo, "/settings/hooks")
	hookPath := hooksPath + "/" + strconv.FormatInt(id, 10)

	lastStatus := func() string {
		var list struct {
			Items []struct {
				LastStatus string `json:"lastStatus"`
			} `json:"items"`
		}
		c.doJSON(http.MethodGet, hooksPath, nil, http.StatusOK, &list)
		require.Len(t, list.Items, 1)
		return list.Items[0].LastStatus
	}
	// waitForDelivery returns the webhook once its only delivery has a
	// response that satisfies the condition.
	waitForDelivery := func(cond func(status int, body string) bool) repoSettingsWebhookDetail {
		var detail repoSettingsWebhookDetail
		require.Eventually(t, func() bool {
			detail = repoSettingsWebhookDetail{}
			code, body := c.do(http.MethodGet, hookPath, nil)
			return code == http.StatusOK && json.Unmarshal(body, &detail) == nil &&
				len(detail.Deliveries) == 1 && detail.Deliveries[0].Response != nil &&
				cond(detail.Deliveries[0].Response.Status, detail.Deliveries[0].Response.Body)
		}, 30*time.Second, 200*time.Millisecond, "delivery not recorded")
		return detail
	}

	status, body := c.do(http.MethodPost, hookPath+"/test", repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	detail := waitForDelivery(func(status int, _ string) bool { return status == http.StatusOK })
	assert.True(t, detail.Deliveries[0].Succeeded)
	assert.Equal(t, "succeeded", lastStatus())

	srv.Close()
	status, body = c.do(http.MethodPost, hookPath+"/deliveries/"+detail.Deliveries[0].UUID+"/redeliver", repoSettingsNoBody)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	detail = waitForDelivery(func(_ int, body string) bool { return strings.HasPrefix(body, "Delivery: ") })
	assert.False(t, detail.Deliveries[0].Succeeded)
	assert.Equal(t, "failed", lastStatus())
}
