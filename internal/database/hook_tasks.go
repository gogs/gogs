package database

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// HookTasksStore is the storage layer for deliveries of webhooks.
type HookTasksStore struct {
	db *gorm.DB
}

func newHookTasksStore(db *gorm.DB) *HookTasksStore {
	return &HookTasksStore{db: db}
}

// ListByWebhook returns the deliveries of the webhook with given ID, newest
// first, on given page of given size.
func (s *HookTasksStore) ListByWebhook(ctx context.Context, webhookID int64, page, pageSize int) ([]*HookTask, error) {
	if page < 1 {
		page = 1
	}
	tasks := make([]*HookTask, 0, pageSize)
	return tasks, s.db.WithContext(ctx).
		Where("hook_id = ?", webhookID).
		Order("id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tasks).
		Error
}

// GetByUUID returns the delivery of the webhook with given UUID. It returns
// ErrHookTaskNotExist when not found.
func (s *HookTasksStore) GetByUUID(ctx context.Context, webhookID int64, uuid string) (*HookTask, error) {
	task := new(HookTask)
	err := s.db.WithContext(ctx).Where("hook_id = ? AND uuid = ?", webhookID, uuid).First(task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHookTaskNotExist{args: map[string]any{"webhookID": webhookID, "uuid": uuid}}
		}
		return nil, err
	}
	return task, nil
}

// Update saves all fields of the delivery.
func (s *HookTasksStore) Update(ctx context.Context, t *HookTask) error {
	return s.db.WithContext(ctx).Model(t).Select("*").Updates(t).Error
}

// listUndelivered returns the deliveries that have not been made, of given
// repository, or of all repositories when repoID is 0.
func (s *HookTasksStore) listUndelivered(ctx context.Context, repoID int64) ([]*HookTask, error) {
	db := s.db.WithContext(ctx).Where("is_delivered = ?", false)
	if repoID > 0 {
		db = db.Where("repo_id = ?", repoID)
	}
	var tasks []*HookTask
	return tasks, db.Order("id ASC").Find(&tasks).Error
}

// Prepare creates a delivery of given event for each active webhook of the
// repository and of its owner, when the owner is an organization, that is
// subscribed to the event, and queues them.
func (s *WebhooksStore) Prepare(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader) error {
	webhooks, err := s.listActive(ctx, repo)
	if err != nil {
		return err
	}
	return s.prepareHookTasks(ctx, logger, repo, event, p, webhooks)
}

// Test creates a delivery of given event for the webhook with given ID of the
// repository when it is subscribed to the event, even if it is not active, and
// queues it. It returns ErrWebhookNotExist when the repository has no such
// webhook.
func (s *WebhooksStore) Test(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader, webhookID int64) error {
	webhook, err := s.GetOfRepo(ctx, repo.ID, webhookID)
	if err != nil {
		return err
	}
	return s.prepareHookTasks(ctx, logger, repo, event, p, []*Webhook{webhook})
}

// hasEvent returns true if the webhook is subscribed to given event.
func (w *Webhook) hasEvent(event HookEventType) bool {
	switch event {
	case HookEventTypeCreate:
		return w.HasCreateEvent()
	case HookEventTypeDelete:
		return w.HasDeleteEvent()
	case HookEventTypeFork:
		return w.HasForkEvent()
	case HookEventTypePush:
		return w.HasPushEvent()
	case HookEventTypeIssues:
		return w.HasIssuesEvent()
	case HookEventTypePullRequest:
		return w.HasPullRequestEvent()
	case HookEventTypeIssueComment:
		return w.HasIssueCommentEvent()
	case HookEventTypeRelease:
		return w.HasReleaseEvent()
	}
	return true
}

// prepareHookTasks creates a delivery of given event for each of given
// webhooks that is subscribed to the event, and queues them.
func (s *WebhooksStore) prepareHookTasks(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader, webhooks []*Webhook) (err error) {
	if len(webhooks) == 0 {
		return nil
	}

	db := s.db.WithContext(ctx)
	for _, w := range webhooks {
		if !w.hasEvent(event) {
			continue
		}

		// Use separate objects so modifications won't be made on payload on
		// non-Gogs type hooks.
		var payloader apiv1types.WebhookPayloader
		switch w.HookTaskType {
		case SLACK:
			payloader, err = GetSlackPayload(p, event, w.Meta)
			if err != nil {
				return errors.Wrap(err, "get Slack payload")
			}
		case DISCORD:
			payloader, err = GetDiscordPayload(p, event, w.Meta)
			if err != nil {
				return errors.Wrap(err, "get Discord payload")
			}
		case DINGTALK:
			payloader, err = GetDingtalkPayload(p, event)
			if err != nil {
				return errors.Wrap(err, "get DingTalk payload")
			}
		default:
			payloader = p
		}

		data, err := payloader.JSONPayload()
		if err != nil {
			return errors.Wrap(err, "encode payload")
		}
		var signature string
		if len(w.Secret) > 0 {
			sig := hmac.New(sha256.New, []byte(w.Secret))
			_, _ = sig.Write(data)
			signature = hex.EncodeToString(sig.Sum(nil))
		}

		err = db.Create(&HookTask{
			RepoID:         repo.ID,
			HookID:         w.ID,
			UUID:           uuid.New().String(),
			Type:           w.HookTaskType,
			URL:            w.URL,
			Signature:      signature,
			PayloadContent: string(data),
			ContentType:    w.ContentType,
			EventType:      event,
			IsSSL:          w.IsSSL,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create hook task")
		}
	}

	// It's safe to fail when the whole function is called during hook execution
	// because resource released after exit. Also, there is no process started to
	// consume this input during hook execution.
	go HookQueue.Add(repo.ID)
	logger.Debug("Prepared webhook deliveries", "repoID", repo.ID, "event", event)
	return nil
}
